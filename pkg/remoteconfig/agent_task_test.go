// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testFlareTaskPath = "datadog/2/AGENT_TASK/flare-task/config"

// applyStatusRecorder records the apply statuses reported for each config.
type applyStatusRecorder struct {
	mu       sync.Mutex
	statuses map[string][]state.ApplyStatus
}

func newApplyStatusRecorder() *applyStatusRecorder {
	return &applyStatusRecorder{statuses: map[string][]state.ApplyStatus{}}
}

func (a *applyStatusRecorder) record(path string, status state.ApplyStatus) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.statuses[path] = append(a.statuses[path], status)
}

func (a *applyStatusRecorder) get(path string) []state.ApplyStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]state.ApplyStatus(nil), a.statuses[path]...)
}

func (a *applyStatusRecorder) last(path string) state.ApplyStatus {
	statuses := a.get(path)
	if len(statuses) == 0 {
		return state.ApplyStatus{}
	}
	return statuses[len(statuses)-1]
}

type fakeFlareRunner struct {
	mu    sync.Mutex
	err   error
	tasks []FlareTask
}

func (f *fakeFlareRunner) Run(_ context.Context, task FlareTask) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tasks = append(f.tasks, task)
	return f.err
}

func (f *fakeFlareRunner) calls() []FlareTask {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]FlareTask(nil), f.tasks...)
}

func agentTaskConfig(t *testing.T, taskType, uuid string, args map[string]string) state.RawConfig {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"task_type": taskType, "uuid": uuid, "args": args})
	require.NoError(t, err)
	return state.RawConfig{Config: raw, Metadata: state.Metadata{Product: state.ProductAgentTask, ID: "flare-task"}}
}

func flareArgs() map[string]string {
	return map[string]string{
		"case_id":          "12345",
		"user_handle":      "user@example.com",
		"source":           "fleet",
		"tags":             `["env:test","team:containers"]`,
		"enable_profiling": "false",
	}
}

func TestParseFlareTask(t *testing.T) {
	tests := []struct {
		name    string
		args    map[string]string
		want    FlareTask
		wantErr string
	}{
		{
			name: "all arguments",
			args: flareArgs(),
			want: FlareTask{
				UUID:       "task-uuid",
				ConfigPath: testFlareTaskPath,
				CaseID:     "12345",
				UserHandle: "user@example.com",
				Source:     "fleet",
				Tags:       []string{"env:test", "team:containers"},
				Args:       flareArgs(),
			},
		},
		{
			name: "required arguments only",
			args: map[string]string{"case_id": "12345", "user_handle": "user@example.com"},
			want: FlareTask{
				UUID:       "task-uuid",
				ConfigPath: testFlareTaskPath,
				CaseID:     "12345",
				UserHandle: "user@example.com",
				Args:       map[string]string{"case_id": "12345", "user_handle": "user@example.com"},
			},
		},
		{
			name: "invalid tags are ignored",
			args: map[string]string{"case_id": "12345", "user_handle": "user@example.com", "tags": "not-json"},
			want: FlareTask{
				UUID:       "task-uuid",
				ConfigPath: testFlareTaskPath,
				CaseID:     "12345",
				UserHandle: "user@example.com",
				Args:       map[string]string{"case_id": "12345", "user_handle": "user@example.com", "tags": "not-json"},
			},
		},
		{
			name:    "missing case_id",
			args:    map[string]string{"user_handle": "user@example.com"},
			wantErr: "case_id not provided",
		},
		{
			name:    "missing user_handle",
			args:    map[string]string{"case_id": "12345"},
			wantErr: "user_handle not provided",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := agentTaskConfig(t, flareTaskType, "task-uuid", test.args)
			task, err := state.ParseConfigAgentTask(raw.Config, raw.Metadata)
			require.NoError(t, err)

			got, err := parseFlareTask(testFlareTaskPath, task)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestAgentTaskFlareStatus(t *testing.T) {
	tests := []struct {
		name      string
		runner    *fakeFlareRunner
		wantFinal state.ApplyStatus
	}{
		{
			name:      "default runner reports not implemented",
			wantFinal: state.ApplyStatus{State: state.ApplyStateError, Error: ErrFlareNotImplemented.Error()},
		},
		{
			name:      "flare sent",
			runner:    &fakeFlareRunner{},
			wantFinal: state.ApplyStatus{State: state.ApplyStateAcknowledged},
		},
		{
			name:      "flare failed",
			runner:    &fakeFlareRunner{err: errors.New("upload failed")},
			wantFinal: state.ApplyStatus{State: state.ApplyStateError, Error: "upload failed"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var options []RemoteConfigUpdaterOption
			if test.runner != nil {
				options = append(options, WithFlareRunner(test.runner))
			}
			updater := NewRemoteConfigUpdater(nil, logr.Discard(), options...)
			recorder := newApplyStatusRecorder()

			updater.agentTaskUpdateCallback(map[string]state.RawConfig{
				testFlareTaskPath: agentTaskConfig(t, flareTaskType, "task-uuid", flareArgs()),
			}, recorder.record)
			updater.tasksWG.Wait()

			assert.Equal(t, []state.ApplyStatus{
				{State: state.ApplyStateUnacknowledged},
				test.wantFinal,
			}, recorder.get(testFlareTaskPath))

			if test.runner != nil {
				calls := test.runner.calls()
				require.Len(t, calls, 1)
				assert.Equal(t, "task-uuid", calls[0].UUID)
				assert.Equal(t, "12345", calls[0].CaseID)
				assert.Equal(t, "user@example.com", calls[0].UserHandle)
			}
		})
	}
}

func TestAgentTaskRejectedTasks(t *testing.T) {
	tests := []struct {
		name    string
		config  func(t *testing.T) state.RawConfig
		wantErr string
	}{
		{
			name: "unsupported task type",
			config: func(t *testing.T) state.RawConfig {
				return agentTaskConfig(t, "ndm-device-scan", "task-uuid", map[string]string{"ip_address": "10.0.0.1"})
			},
			wantErr: `unsupported agent task type "ndm-device-scan"`,
		},
		{
			name: "invalid task",
			config: func(*testing.T) state.RawConfig {
				return state.RawConfig{Config: []byte("{"), Metadata: state.Metadata{Product: state.ProductAgentTask}}
			},
			wantErr: "Unexpected AGENT_TASK",
		},
		{
			name: "flare task without case_id",
			config: func(t *testing.T) state.RawConfig {
				return agentTaskConfig(t, flareTaskType, "task-uuid", map[string]string{"user_handle": "user@example.com"})
			},
			wantErr: "case_id not provided",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &fakeFlareRunner{}
			updater := NewRemoteConfigUpdater(nil, logr.Discard(), WithFlareRunner(runner))
			recorder := newApplyStatusRecorder()

			updater.agentTaskUpdateCallback(map[string]state.RawConfig{testFlareTaskPath: test.config(t)}, recorder.record)
			updater.tasksWG.Wait()

			statuses := recorder.get(testFlareTaskPath)
			require.Len(t, statuses, 1, "a rejected task is never reported as accepted")
			assert.Equal(t, state.ApplyStateError, statuses[0].State)
			assert.Contains(t, statuses[0].Error, test.wantErr)
			assert.Empty(t, runner.calls())
		})
	}
}

func TestAgentTaskLeaderOnly(t *testing.T) {
	leader := false
	runner := &fakeFlareRunner{}
	updater := NewRemoteConfigUpdater(nil, logr.Discard(),
		WithFlareRunner(runner),
		WithLeaderCheck(func() bool { return leader }),
	)
	recorder := newApplyStatusRecorder()
	updates := map[string]state.RawConfig{
		testFlareTaskPath: agentTaskConfig(t, flareTaskType, "task-uuid", flareArgs()),
	}

	updater.agentTaskUpdateCallback(updates, recorder.record)
	updater.tasksWG.Wait()
	assert.Empty(t, runner.calls(), "a non-leader must not run the flare")
	assert.Empty(t, recorder.get(testFlareTaskPath), "a non-leader must not report a status")

	leader = true
	updater.agentTaskUpdateCallback(updates, recorder.record)
	updater.tasksWG.Wait()
	assert.Len(t, runner.calls(), 1, "the task runs once this replica is the leader")
	assert.Equal(t, state.ApplyStatus{State: state.ApplyStateAcknowledged}, recorder.last(testFlareTaskPath))
}

func TestAgentTaskWithoutLeaderCheckRuns(t *testing.T) {
	runner := &fakeFlareRunner{}
	updater := NewRemoteConfigUpdater(nil, logr.Discard(), WithFlareRunner(runner))
	recorder := newApplyStatusRecorder()

	updater.agentTaskUpdateCallback(map[string]state.RawConfig{
		testFlareTaskPath: agentTaskConfig(t, flareTaskType, "task-uuid", flareArgs()),
	}, recorder.record)
	updater.tasksWG.Wait()

	assert.Len(t, runner.calls(), 1)
	assert.Equal(t, state.ApplyStatus{State: state.ApplyStateAcknowledged}, recorder.last(testFlareTaskPath))
}

func TestAgentTaskRunsOncePerUUID(t *testing.T) {
	runner := &fakeFlareRunner{err: errors.New("upload failed")}
	updater := NewRemoteConfigUpdater(nil, logr.Discard(), WithFlareRunner(runner))
	updates := map[string]state.RawConfig{
		testFlareTaskPath: agentTaskConfig(t, flareTaskType, "task-uuid", flareArgs()),
	}

	first := newApplyStatusRecorder()
	updater.agentTaskUpdateCallback(updates, first.record)
	updater.tasksWG.Wait()

	// Remote Configuration delivers active tasks again, for example to the
	// client that replaces the current one when updater tags change.
	second := newApplyStatusRecorder()
	updater.agentTaskUpdateCallback(updates, second.record)
	updater.tasksWG.Wait()

	assert.Len(t, runner.calls(), 1)
	assert.Equal(t, []state.ApplyStatus{
		{State: state.ApplyStateError, Error: "upload failed"},
	}, second.get(testFlareTaskPath), "a redelivered task reports its known status")
}

func TestStopCancelsRunningFlare(t *testing.T) {
	started := make(chan struct{})
	updater := NewRemoteConfigUpdater(nil, logr.Discard(), WithFlareRunner(flareRunnerFunc(func(ctx context.Context, _ FlareTask) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})))
	recorder := newApplyStatusRecorder()

	updater.agentTaskUpdateCallback(map[string]state.RawConfig{
		testFlareTaskPath: agentTaskConfig(t, flareTaskType, "task-uuid", flareArgs()),
	}, recorder.record)
	<-started
	require.NoError(t, updater.Stop())
	updater.tasksWG.Wait()

	assert.Equal(t, state.ApplyStatus{State: state.ApplyStateError, Error: context.Canceled.Error()}, recorder.last(testFlareTaskPath))
}

type flareRunnerFunc func(context.Context, FlareTask) error

func (f flareRunnerFunc) Run(ctx context.Context, task FlareTask) error {
	return f(ctx, task)
}
