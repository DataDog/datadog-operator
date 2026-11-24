// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

const (
	// flareTaskType and flareTaskTimeout match the Datadog Agent's
	// types.TaskFlare and agentTaskTimeout.
	flareTaskType    = "flare"
	flareTaskTimeout = 5 * time.Minute
)

// ErrFlareNotImplemented is the error of flare tasks when no FlareRunner is
// configured.
var ErrFlareNotImplemented = errors.New("remote flare is not implemented yet in this version of the Datadog Operator")

var errFlareRunning = errors.New("another flare is running: request the flare again once it is done")

// FlareTask is a flare request received through a Remote Configuration
// AGENT_TASK config, with the arguments the Datadog Agent reads in
// pkg/flare/clusteragent.HandleRCFlareTask.
type FlareTask struct {
	// UUID is sent as rc_task_uuid so Fleet Automation can match the flare
	// to the task.
	UUID       string
	ConfigPath string
	CaseID     string
	// UserHandle is the email of the user who requested the flare.
	UserHandle string
	Source     string
	Tags       []string
}

// FlareRunner creates and sends an Operator flare for a FlareTask. Run returns
// nil only once the flare was sent.
type FlareRunner interface {
	Run(ctx context.Context, task FlareTask) error
}

// parseFlareTask validates the arguments of a flare task.
func parseFlareTask(configPath string, task state.AgentTaskConfig) (FlareTask, error) {
	args := task.Config.TaskArgs
	caseID := args["case_id"]
	if caseID == "" {
		return FlareTask{}, errors.New("case_id not provided in flare agent task")
	}
	userHandle := args["user_handle"]
	if userHandle == "" {
		return FlareTask{}, errors.New("user_handle not provided in flare agent task")
	}

	var tags []string
	if rawTags := args["tags"]; rawTags != "" {
		if err := json.Unmarshal([]byte(rawTags), &tags); err != nil {
			// Same as the Datadog Agent: invalid tags don't fail the flare.
			tags = nil
		}
	}

	return FlareTask{
		UUID:       task.Config.UUID,
		ConfigPath: configPath,
		CaseID:     caseID,
		UserHandle: userHandle,
		Source:     args["source"],
		Tags:       tags,
	}, nil
}

// agentTaskUpdateCallback handles AGENT_TASK configs. Only flare tasks are
// supported. A flare task is unacknowledged while it runs, acknowledged once
// the flare was sent, and error if the task is invalid or the flare failed.
func (r *RemoteConfigUpdater) agentTaskUpdateCallback(updates map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) {
	for configPath, rawConfig := range updates {
		r.handleAgentTask(configPath, rawConfig, applyStatus)
	}
}

func (r *RemoteConfigUpdater) handleAgentTask(configPath string, rawConfig state.RawConfig, applyStatus func(string, state.ApplyStatus)) {
	// The Remote Configuration client doesn't recover panics in callbacks.
	defer func() {
		if p := recover(); p != nil {
			r.logger.Error(fmt.Errorf("%v", p), "Panic while handling a Remote Configuration agent task", "path", configPath, "stack", string(debug.Stack()))
			applyStatus(configPath, state.ApplyStatus{State: state.ApplyStateError, Error: fmt.Sprintf("the agent task panicked: %v", p)})
		}
	}()
	// Tasks run once per cluster, on the leader. A non-leader neither runs
	// the task nor reports a status for it.
	if !r.isLeader() {
		r.logger.Info("Ignoring Remote Configuration agent task: this Operator replica is not the leader", "path", configPath)
		return
	}

	task, err := state.ParseConfigAgentTask(rawConfig.Config, rawConfig.Metadata)
	if err != nil {
		r.logger.Error(err, "Invalid Remote Configuration agent task", "path", configPath)
		applyStatus(configPath, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
		return
	}

	key := task.Config.UUID
	if key == "" {
		key = configPath
	}

	// Remote Configuration delivers every active task again whenever the
	// AGENT_TASK product changes, and to a new client when updater tags are
	// refreshed. Report the known status instead of running a task twice.
	r.tasksMu.Lock()
	if r.taskStatuses == nil {
		r.taskStatuses = make(map[string]state.ApplyStatus)
	}
	if status, seen := r.taskStatuses[key]; seen {
		r.tasksMu.Unlock()
		applyStatus(configPath, status)
		return
	}
	r.taskStatuses[key] = state.ApplyStatus{State: state.ApplyStateUnacknowledged}
	r.tasksMu.Unlock()

	if task.Config.TaskType != flareTaskType {
		unsupportedErr := fmt.Errorf("unsupported agent task type %q: the Datadog Operator only supports %q tasks", task.Config.TaskType, flareTaskType)
		r.logger.Info("Ignoring unsupported Remote Configuration agent task", "path", configPath, "taskType", task.Config.TaskType)
		r.reportTaskStatus(key, configPath, state.ApplyStatus{State: state.ApplyStateError, Error: unsupportedErr.Error()}, applyStatus)
		return
	}

	flareTask, err := parseFlareTask(configPath, task)
	if err != nil {
		r.logger.Error(err, "Invalid Remote Configuration flare task", "path", configPath, "uuid", task.Config.UUID)
		r.reportTaskStatus(key, configPath, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()}, applyStatus)
		return
	}

	// One flare at a time, so that flares can't add up in the Operator memory.
	r.tasksMu.Lock()
	busy := r.flareRunning
	r.flareRunning = true
	r.tasksMu.Unlock()
	if busy {
		r.logger.Info("Rejecting Remote Configuration flare task: another flare is running", "uuid", flareTask.UUID, "caseID", flareTask.CaseID)
		r.reportTaskStatus(key, configPath, state.ApplyStatus{State: state.ApplyStateError, Error: errFlareRunning.Error()}, applyStatus)
		return
	}

	r.logger.Info("Received Remote Configuration flare task", "uuid", flareTask.UUID, "caseID", flareTask.CaseID)
	applyStatus(configPath, state.ApplyStatus{State: state.ApplyStateUnacknowledged})

	// Run the flare in the background: Remote Configuration calls every
	// product callback from its polling loop, under a lock.
	baseCtx := r.tasksContext()
	r.tasksWG.Go(func() {
		ctx, cancel := context.WithTimeout(baseCtx, flareTaskTimeout)
		defer cancel()

		err := r.runFlare(ctx, flareTask)
		r.tasksMu.Lock()
		r.flareRunning = false
		r.tasksMu.Unlock()
		status := state.ApplyStatus{State: state.ApplyStateAcknowledged}
		if err != nil {
			r.logger.Error(err, "Remote Configuration flare task failed", "uuid", flareTask.UUID, "caseID", flareTask.CaseID)
			status = state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()}
		}
		r.reportTaskStatus(key, configPath, status, applyStatus)
	})
}

// runFlare runs a flare task. A panic in the flare is returned as an error,
// so that a flare can't crash the Operator.
func (r *RemoteConfigUpdater) runFlare(ctx context.Context, task FlareTask) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("the flare panicked: %v", p)
			r.logger.Error(err, "Recovered a panic in a flare task", "uuid", task.UUID, "stack", string(debug.Stack()))
		}
	}()
	if r.flareRunner == nil {
		return ErrFlareNotImplemented
	}
	return r.flareRunner.Run(ctx, task)
}

func (r *RemoteConfigUpdater) reportTaskStatus(key, configPath string, status state.ApplyStatus, applyStatus func(string, state.ApplyStatus)) {
	r.tasksMu.Lock()
	r.taskStatuses[key] = status
	r.tasksMu.Unlock()
	applyStatus(configPath, status)
}

func (r *RemoteConfigUpdater) isLeader() bool {
	return r.leaderCheck == nil || r.leaderCheck()
}

// tasksContext returns the context Stop cancels to cancel running agent tasks.
func (r *RemoteConfigUpdater) tasksContext() context.Context {
	r.tasksMu.Lock()
	defer r.tasksMu.Unlock()
	if r.tasksCtx == nil {
		r.tasksCtx, r.cancelTasks = context.WithCancel(context.Background())
	}
	return r.tasksCtx
}
