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
	"maps"
	"time"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

const (
	// flareTaskType is the AGENT_TASK task_type Fleet Automation sends to
	// request a flare. It matches the Datadog Agent's types.TaskFlare.
	flareTaskType = "flare"
	// flareTaskTimeout bounds a single flare run. It matches the Datadog
	// Agent's agentTaskTimeout.
	flareTaskTimeout = 5 * time.Minute
)

// ErrFlareNotImplemented is returned by the default FlareRunner. Fleet
// Automation sees it as the error of the flare task, so a flare is never
// reported as sent before the Operator can send one.
var ErrFlareNotImplemented = errors.New("remote flare is not implemented yet in this version of the Datadog Operator")

// FlareTask is a flare request received through a Remote Configuration
// AGENT_TASK config. Its fields mirror the arguments the Datadog Agent reads
// in pkg/flare/clusteragent.HandleRCFlareTask.
type FlareTask struct {
	// UUID identifies the task. The flare upload must send it as
	// rc_task_uuid so Fleet Automation can match the flare to the task.
	UUID string
	// ConfigPath is the Remote Configuration path of the task config.
	ConfigPath string
	// CaseID is the support case the flare is attached to.
	CaseID string
	// UserHandle is the email of the user who requested the flare.
	UserHandle string
	// Source is the optional flare source sent by Fleet Automation.
	Source string
	// Tags are the optional flare tags sent by Fleet Automation.
	Tags []string
	// Args holds every task argument, including the ones above, for
	// arguments the Operator does not interpret yet (such as
	// enable_profiling).
	Args map[string]string
}

// FlareRunner creates and sends an Operator flare for a FlareTask. Run returns
// nil only once the flare was sent.
type FlareRunner interface {
	Run(ctx context.Context, task FlareTask) error
}

// notImplementedFlareRunner is the default FlareRunner until the Operator can
// collect and send flares.
type notImplementedFlareRunner struct{}

func (notImplementedFlareRunner) Run(context.Context, FlareTask) error {
	return ErrFlareNotImplemented
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
		Args:       maps.Clone(args),
	}, nil
}

// agentTaskUpdateCallback handles AGENT_TASK configs. Only flare tasks are
// supported. A flare task is reported as:
//   - unacknowledged once the Operator accepted it and started the flare,
//   - acknowledged once the flare was sent,
//   - error, with a message, if the task is invalid or the flare failed.
//
// Remote Configuration has no separate "in progress" state: unacknowledged
// means received but not applied yet, and acknowledged means applied.
func (r *RemoteConfigUpdater) agentTaskUpdateCallback(updates map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) {
	for configPath, rawConfig := range updates {
		r.handleAgentTask(configPath, rawConfig, applyStatus)
	}
}

func (r *RemoteConfigUpdater) handleAgentTask(configPath string, rawConfig state.RawConfig, applyStatus func(string, state.ApplyStatus)) {
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

	r.logger.Info("Received Remote Configuration flare task", "uuid", flareTask.UUID, "caseID", flareTask.CaseID)
	applyStatus(configPath, state.ApplyStatus{State: state.ApplyStateUnacknowledged})

	// Run the flare in the background: Remote Configuration calls every
	// product callback from its polling loop, under a lock.
	runner := r.flareRunner
	if runner == nil {
		runner = notImplementedFlareRunner{}
	}
	stop := r.tasksStopChannel()
	r.tasksWG.Go(func() {
		ctx, cancel := context.WithTimeout(context.Background(), flareTaskTimeout)
		defer cancel()
		go func() {
			select {
			case <-stop:
				cancel()
			case <-ctx.Done():
			}
		}()

		status := state.ApplyStatus{State: state.ApplyStateAcknowledged}
		if err := runner.Run(ctx, flareTask); err != nil {
			r.logger.Error(err, "Remote Configuration flare task failed", "uuid", flareTask.UUID, "caseID", flareTask.CaseID)
			status = state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()}
		} else {
			r.logger.Info("Remote Configuration flare task completed", "uuid", flareTask.UUID, "caseID", flareTask.CaseID)
		}
		r.reportTaskStatus(key, configPath, status, applyStatus)
	})
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

// tasksStopChannel returns the channel Stop closes to cancel running agent
// tasks.
func (r *RemoteConfigUpdater) tasksStopChannel() <-chan struct{} {
	r.tasksMu.Lock()
	defer r.tasksMu.Unlock()
	if r.tasksStop == nil {
		r.tasksStop = make(chan struct{})
	}
	return r.tasksStop
}
