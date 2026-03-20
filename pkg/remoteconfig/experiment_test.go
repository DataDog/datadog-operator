// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
)

func init() {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
}

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = v2alpha1.AddToScheme(s)
	return s
}

func newTestDDA() *v2alpha1.DatadogAgent {
	return &v2alpha1.DatadogAgent{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogAgent",
			APIVersion: "datadoghq.com/v2alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "datadog-agent",
			Namespace: "datadog",
		},
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				APM: &v2alpha1.APMFeatureConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
				NPM: &v2alpha1.NPMFeatureConfig{
					Enabled: apiutils.NewBoolPointer(true),
				},
			},
		},
	}
}

func newTestDDAWithExperiment(phase v2alpha1.ExperimentPhase) *v2alpha1.DatadogAgent {
	dda := newTestDDA()
	dda.Status = v2alpha1.DatadogAgentStatus{
		CurrentRevision: "datadog-agent-abc123",
		Experiment: &v2alpha1.ExperimentStatus{
			Phase:            phase,
			BaselineRevision: "datadog-agent-baseline",
			ID:               "exp-001",
		},
	}
	return dda
}

func newUpdater(objs ...runtime.Object) *RemoteConfigUpdater {
	s := testScheme()
	builder := fake.NewClientBuilder().WithScheme(s)
	for _, obj := range objs {
		builder = builder.WithRuntimeObjects(obj)
	}
	builder = builder.WithStatusSubresource(&v2alpha1.DatadogAgent{})
	c := builder.Build()
	return &RemoteConfigUpdater{
		kubeClient: c,
		logger:     logr.New(logf.NullLogSink{}),
	}
}

func getDDA(t *testing.T, r *RemoteConfigUpdater) *v2alpha1.DatadogAgent {
	t.Helper()
	dda := &v2alpha1.DatadogAgent{}
	err := r.kubeClient.Get(context.TODO(), types.NamespacedName{
		Name:      "datadog-agent",
		Namespace: "datadog",
	}, dda)
	require.NoError(t, err)
	return dda
}

// ============================================================================
// parseExperimentSignal tests
// ============================================================================

func TestParseExperimentSignal_StartExperiment(t *testing.T) {
	spec := v2alpha1.DatadogAgentSpec{
		Features: &v2alpha1.DatadogFeatures{
			APM: &v2alpha1.APMFeatureConfig{
				Enabled: apiutils.NewBoolPointer(true),
			},
		},
	}
	payload := ExperimentSignal{
		Action:       ExperimentActionStart,
		ExperimentID: "exp-001",
		Config:       &spec,
	}
	data, _ := json.Marshal(payload)

	signal, err := parseExperimentSignal(data)

	require.NoError(t, err)
	require.NotNil(t, signal)
	assert.Equal(t, ExperimentActionStart, signal.Action)
	assert.Equal(t, "exp-001", signal.ExperimentID)
	assert.NotNil(t, signal.Config)
	assert.True(t, apiutils.BoolValue(signal.Config.Features.APM.Enabled))
}

func TestParseExperimentSignal_StopExperiment(t *testing.T) {
	payload := ExperimentSignal{
		Action:       ExperimentActionStop,
		ExperimentID: "exp-001",
	}
	data, _ := json.Marshal(payload)

	signal, err := parseExperimentSignal(data)

	require.NoError(t, err)
	require.NotNil(t, signal)
	assert.Equal(t, ExperimentActionStop, signal.Action)
	assert.Nil(t, signal.Config)
}

func TestParseExperimentSignal_PromoteExperiment(t *testing.T) {
	payload := ExperimentSignal{
		Action:       ExperimentActionPromote,
		ExperimentID: "exp-001",
	}
	data, _ := json.Marshal(payload)

	signal, err := parseExperimentSignal(data)

	require.NoError(t, err)
	require.NotNil(t, signal)
	assert.Equal(t, ExperimentActionPromote, signal.Action)
}

func TestParseExperimentSignal_RegularAgentConfig(t *testing.T) {
	// Regular agent config (no "action" field) should return nil
	payload := DatadogAgentRemoteConfig{
		ID:   "config-001",
		Name: "my-config",
		CoreAgent: &CoreAgentFeaturesConfig{
			SBOM: &SbomConfig{Enabled: apiutils.NewBoolPointer(true)},
		},
	}
	data, _ := json.Marshal(payload)

	signal, err := parseExperimentSignal(data)

	require.NoError(t, err)
	assert.Nil(t, signal, "regular config should not be parsed as experiment signal")
}

func TestParseExperimentSignal_UnknownAction(t *testing.T) {
	data := []byte(`{"action": "unknownAction", "experiment_id": "exp-001"}`)

	signal, err := parseExperimentSignal(data)

	require.Error(t, err, "unknown action should return error to prevent fallthrough")
	assert.Nil(t, signal)
}

func TestParseExperimentSignal_InvalidJSON(t *testing.T) {
	data := []byte(`not json`)

	signal, err := parseExperimentSignal(data)

	require.Error(t, err)
	assert.Nil(t, signal)
}

// ============================================================================
// handleStartExperiment tests
// ============================================================================

func TestHandleStartExperiment_Success(t *testing.T) {
	dda := newTestDDA()
	dda.Status.CurrentRevision = "datadog-agent-baseline"
	r := newUpdater(dda)

	newSpec := v2alpha1.DatadogAgentSpec{
		Features: &v2alpha1.DatadogFeatures{
			APM: &v2alpha1.APMFeatureConfig{
				Enabled: apiutils.NewBoolPointer(true),
			},
		},
	}

	signal := &ExperimentSignal{
		Action:       ExperimentActionStart,
		ExperimentID: "exp-001",
		Config:       &newSpec,
	}

	err := r.handleExperimentSignal(context.TODO(), signal)
	require.NoError(t, err)

	// Verify status
	updated := getDDA(t, r)
	require.NotNil(t, updated.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, updated.Status.Experiment.Phase)
	assert.Equal(t, "datadog-agent-baseline", updated.Status.Experiment.BaselineRevision)
	assert.Equal(t, "exp-001", updated.Status.Experiment.ID)

	// Verify ExpectedSpecHash is set (computed from defaulted spec)
	assert.NotEmpty(t, updated.Status.Experiment.ExpectedSpecHash,
		"ExpectedSpecHash should be set for first-reconcile validation")

	// Verify spec was merged (not replaced)
	assert.True(t, apiutils.BoolValue(updated.Spec.Features.APM.Enabled),
		"APM should be enabled after startExperiment")
	assert.True(t, apiutils.BoolValue(updated.Spec.Features.NPM.Enabled),
		"NPM should be preserved from original spec (merge, not replace)")
}

func TestHandleStartExperiment_MissingConfig(t *testing.T) {
	dda := newTestDDA()
	r := newUpdater(dda)

	signal := &ExperimentSignal{
		Action:       ExperimentActionStart,
		ExperimentID: "exp-001",
		Config:       nil, // Missing
	}

	err := r.handleExperimentSignal(context.TODO(), signal)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing config payload")
}

func TestHandleStartExperiment_NoCurrentRevision(t *testing.T) {
	dda := newTestDDA()
	// No CurrentRevision set — DDA hasn't been reconciled yet
	r := newUpdater(dda)

	newSpec := v2alpha1.DatadogAgentSpec{
		Features: &v2alpha1.DatadogFeatures{
			APM: &v2alpha1.APMFeatureConfig{
				Enabled: apiutils.NewBoolPointer(true),
			},
		},
	}

	signal := &ExperimentSignal{
		Action:       ExperimentActionStart,
		ExperimentID: "exp-001",
		Config:       &newSpec,
	}

	err := r.handleExperimentSignal(context.TODO(), signal)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no currentRevision set")
}

func TestHandleStartExperiment_AlreadyRunning(t *testing.T) {
	dda := newTestDDAWithExperiment(v2alpha1.ExperimentPhaseRunning)
	r := newUpdater(dda)

	newSpec := v2alpha1.DatadogAgentSpec{
		Features: &v2alpha1.DatadogFeatures{
			APM: &v2alpha1.APMFeatureConfig{
				Enabled: apiutils.NewBoolPointer(true),
			},
		},
	}

	signal := &ExperimentSignal{
		Action:       ExperimentActionStart,
		ExperimentID: "exp-002",
		Config:       &newSpec,
	}

	err := r.handleExperimentSignal(context.TODO(), signal)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "active (phase=running)")

	// Verify original experiment is untouched
	updated := getDDA(t, r)
	assert.Equal(t, "exp-001", updated.Status.Experiment.ID,
		"original experiment ID should be preserved")
	assert.Equal(t, "datadog-agent-baseline", updated.Status.Experiment.BaselineRevision,
		"original baseline should be preserved")
}

func TestHandleStartExperiment_RetryAfterPartialFailure(t *testing.T) {
	// Simulate: prior start set status (phase=running, expectedSpecHash set)
	// but spec patch failed. ExpectedSpecHash still set = retry allowed.
	dda := newTestDDA()
	dda.Status = v2alpha1.DatadogAgentStatus{
		CurrentRevision: "datadog-agent-baseline",
		Experiment: &v2alpha1.ExperimentStatus{
			Phase:            v2alpha1.ExperimentPhaseRunning,
			BaselineRevision: "datadog-agent-baseline",
			ID:               "exp-001",
			ExpectedSpecHash: "abc123", // Still set = spec wasn't validated yet
		},
	}
	r := newUpdater(dda)

	newSpec := v2alpha1.DatadogAgentSpec{
		Features: &v2alpha1.DatadogFeatures{
			APM: &v2alpha1.APMFeatureConfig{
				Enabled: apiutils.NewBoolPointer(true),
			},
		},
	}

	signal := &ExperimentSignal{
		Action:       ExperimentActionStart,
		ExperimentID: "exp-001",
		Config:       &newSpec,
	}

	err := r.handleExperimentSignal(context.TODO(), signal)
	require.NoError(t, err, "should allow retry when ExpectedSpecHash is still set")

	// Verify spec was patched on retry
	updated := getDDA(t, r)
	assert.True(t, apiutils.BoolValue(updated.Spec.Features.APM.Enabled),
		"APM should be enabled after retry")
}

func TestHandleStartExperiment_DifferentIDDuringRetryWindow(t *testing.T) {
	// Prior start for exp-001 partially failed (ExpectedSpecHash still set).
	// A different experiment (exp-002) tries to start — should be rejected.
	dda := newTestDDA()
	dda.Status = v2alpha1.DatadogAgentStatus{
		CurrentRevision: "datadog-agent-baseline",
		Experiment: &v2alpha1.ExperimentStatus{
			Phase:            v2alpha1.ExperimentPhaseRunning,
			BaselineRevision: "datadog-agent-baseline",
			ID:               "exp-001",
			ExpectedSpecHash: "abc123",
		},
	}
	r := newUpdater(dda)

	signal := &ExperimentSignal{
		Action:       ExperimentActionStart,
		ExperimentID: "exp-002", // Different ID
		Config: &v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				APM: &v2alpha1.APMFeatureConfig{
					Enabled: apiutils.NewBoolPointer(true),
				},
			},
		},
	}

	err := r.handleExperimentSignal(context.TODO(), signal)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "active (phase=running)")

	// Original experiment should be untouched
	updated := getDDA(t, r)
	assert.Equal(t, "exp-001", updated.Status.Experiment.ID)
}

func TestHandleStartExperiment_DuringRollback(t *testing.T) {
	// Stop was received (phase=rollback) but reconciler hasn't restored yet.
	// A new start should be rejected to avoid overwriting the pending rollback.
	dda := newTestDDAWithExperiment(v2alpha1.ExperimentPhaseRollback)
	r := newUpdater(dda)

	signal := &ExperimentSignal{
		Action:       ExperimentActionStart,
		ExperimentID: "exp-002",
		Config: &v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				APM: &v2alpha1.APMFeatureConfig{
					Enabled: apiutils.NewBoolPointer(true),
				},
			},
		},
	}

	err := r.handleExperimentSignal(context.TODO(), signal)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "active (phase=rollback)")

	// Original experiment should be untouched — rollback still pending
	updated := getDDA(t, r)
	assert.Equal(t, v2alpha1.ExperimentPhaseRollback, updated.Status.Experiment.Phase)
	assert.Equal(t, "exp-001", updated.Status.Experiment.ID)
}

func TestHandleStartExperiment_AfterAborted(t *testing.T) {
	// A prior experiment was aborted (external edit). A new start should
	// clear the aborted state and start fresh — not get stuck forever.
	dda := newTestDDAWithExperiment(v2alpha1.ExperimentPhaseAborted)
	r := newUpdater(dda)

	signal := &ExperimentSignal{
		Action:       ExperimentActionStart,
		ExperimentID: "exp-002",
		Config: &v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				APM: &v2alpha1.APMFeatureConfig{
					Enabled: apiutils.NewBoolPointer(true),
				},
			},
		},
	}

	err := r.handleExperimentSignal(context.TODO(), signal)
	require.NoError(t, err, "should allow new experiment after aborted")

	updated := getDDA(t, r)
	require.NotNil(t, updated.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, updated.Status.Experiment.Phase)
	assert.Equal(t, "exp-002", updated.Status.Experiment.ID)
}

// ============================================================================
// handleStopExperiment tests
// ============================================================================

func TestHandleStopExperiment_RunningExperiment(t *testing.T) {
	dda := newTestDDAWithExperiment(v2alpha1.ExperimentPhaseRunning)
	r := newUpdater(dda)

	signal := &ExperimentSignal{
		Action:       ExperimentActionStop,
		ExperimentID: "exp-001",
	}

	err := r.handleExperimentSignal(context.TODO(), signal)
	require.NoError(t, err)

	updated := getDDA(t, r)
	require.NotNil(t, updated.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseRollback, updated.Status.Experiment.Phase)
	assert.Equal(t, "datadog-agent-baseline", updated.Status.Experiment.BaselineRevision,
		"baseline should be preserved for rollback")
}

func TestHandleStopExperiment_NoRunningExperiment(t *testing.T) {
	dda := newTestDDA() // No experiment
	r := newUpdater(dda)

	signal := &ExperimentSignal{
		Action:       ExperimentActionStop,
		ExperimentID: "exp-001",
	}

	err := r.handleExperimentSignal(context.TODO(), signal)
	require.NoError(t, err, "stale signal should be silently ignored")
}

func TestHandleStopExperiment_AbortedExperiment(t *testing.T) {
	dda := newTestDDAWithExperiment(v2alpha1.ExperimentPhaseAborted)
	r := newUpdater(dda)

	signal := &ExperimentSignal{
		Action:       ExperimentActionStop,
		ExperimentID: "exp-001",
	}

	err := r.handleExperimentSignal(context.TODO(), signal)
	require.NoError(t, err, "stale signal should be silently ignored")
}

func TestHandleStopExperiment_IDMismatch(t *testing.T) {
	dda := newTestDDAWithExperiment(v2alpha1.ExperimentPhaseRunning) // ID = "exp-001"
	r := newUpdater(dda)

	signal := &ExperimentSignal{
		Action:       ExperimentActionStop,
		ExperimentID: "exp-999", // Wrong ID
	}

	err := r.handleExperimentSignal(context.TODO(), signal)
	require.NoError(t, err, "mismatched signal should be silently ignored")

	// Original experiment should be untouched
	updated := getDDA(t, r)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, updated.Status.Experiment.Phase)
}

// ============================================================================
// handlePromoteExperiment tests
// ============================================================================

func TestHandlePromoteExperiment_RunningExperiment(t *testing.T) {
	dda := newTestDDAWithExperiment(v2alpha1.ExperimentPhaseRunning)
	r := newUpdater(dda)

	signal := &ExperimentSignal{
		Action:       ExperimentActionPromote,
		ExperimentID: "exp-001",
	}

	err := r.handleExperimentSignal(context.TODO(), signal)
	require.NoError(t, err)

	updated := getDDA(t, r)
	require.NotNil(t, updated.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhasePromoted, updated.Status.Experiment.Phase)
	assert.Empty(t, updated.Status.Experiment.BaselineRevision,
		"baseline should be cleared on promote")
}

func TestHandlePromoteExperiment_NoRunningExperiment(t *testing.T) {
	dda := newTestDDA()
	r := newUpdater(dda)

	signal := &ExperimentSignal{
		Action:       ExperimentActionPromote,
		ExperimentID: "exp-001",
	}

	err := r.handleExperimentSignal(context.TODO(), signal)
	require.NoError(t, err, "stale signal should be silently ignored")
}

func TestHandlePromoteExperiment_AbortedExperiment(t *testing.T) {
	dda := newTestDDAWithExperiment(v2alpha1.ExperimentPhaseAborted)
	r := newUpdater(dda)

	signal := &ExperimentSignal{
		Action:       ExperimentActionPromote,
		ExperimentID: "exp-001",
	}

	err := r.handleExperimentSignal(context.TODO(), signal)
	require.NoError(t, err, "stale signal should be silently ignored")
}

func TestHandlePromoteExperiment_IDMismatch(t *testing.T) {
	dda := newTestDDAWithExperiment(v2alpha1.ExperimentPhaseRunning) // ID = "exp-001"
	r := newUpdater(dda)

	signal := &ExperimentSignal{
		Action:       ExperimentActionPromote,
		ExperimentID: "exp-999", // Wrong ID
	}

	err := r.handleExperimentSignal(context.TODO(), signal)
	require.NoError(t, err, "mismatched signal should be silently ignored")

	// Original experiment should still be running
	updated := getDDA(t, r)
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, updated.Status.Experiment.Phase)
}

// ============================================================================
// experimentPhase helper tests
// ============================================================================

func TestExperimentPhase_NoExperiment(t *testing.T) {
	dda := newTestDDA()
	assert.Equal(t, "none", experimentPhase(dda))
}

func TestExperimentPhase_Running(t *testing.T) {
	dda := newTestDDAWithExperiment(v2alpha1.ExperimentPhaseRunning)
	assert.Equal(t, "running", experimentPhase(dda))
}
