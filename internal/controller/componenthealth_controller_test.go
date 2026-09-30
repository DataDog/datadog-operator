// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-operator/internal/controller/metrics"
	"github.com/DataDog/datadog-operator/pkg/componenthealth"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

const testComponent = constants.DefaultClusterAgentResourceSuffix

// recordingEmitter captures the snapshots handed to the emitter.
type recordingEmitter struct {
	snapshots [][]componenthealth.ComponentIssue
}

func (e *recordingEmitter) Snapshot(_ context.Context, issues []componenthealth.ComponentIssue) {
	e.snapshots = append(e.snapshots, issues)
}

func newTestReconciler(e componentHealthEmitter) *ComponentHealthReconciler {
	return &ComponentHealthReconciler{
		log:              logr.Discard(),
		restartThreshold: defaultRestartThreshold,
		emitter:          e,
		snapshotInterval: defaultSnapshotInterval,
		reported:         map[componentIssueKey]*componentIssueState{},
	}
}

func detected(issueType, pod string) componenthealth.DetectedIssue {
	return componenthealth.DetectedIssue{
		Component: testComponent,
		IssueType: issueType,
		Severity:  componenthealth.SeverityForIssueType(issueType),
		Namespace: "datadog",
		PodName:   pod,
	}
}

func podKey(name string) types.NamespacedName {
	return types.NamespacedName{Namespace: "datadog", Name: name}
}

// activeIssue returns the active component-level issue of the given type from the
// reconciler's current snapshot, or nil if none is active.
func activeIssue(r *ComponentHealthReconciler, issueType string) *componenthealth.ComponentIssue {
	for _, issue := range r.snapshot() {
		if issue.IssueType == issueType {
			return &issue
		}
	}
	return nil
}

func detectedCount(t *testing.T, issueType string) float64 {
	t.Helper()
	return testutil.ToFloat64(metrics.ComponentHealthIssuesDetected.WithLabelValues(testComponent, issueType))
}

// TestComponentHealth_AggregatesAcrossPods verifies that the same issue type on
// several pods of a component is a single component-level issue: it appears once
// in the snapshot (counted once), stays a single active issue while any pod
// remains affected, and clears from the snapshot only when the last recovers.
func TestComponentHealth_AggregatesAcrossPods(t *testing.T) {
	metrics.ComponentHealthIssuesDetected.Reset()
	r := newTestReconciler(&recordingEmitter{})

	// pod-a hits an OOMKill: first affected pod -> one active component-level issue.
	r.reconcilePod(podKey("pod-a"), testComponent, []componenthealth.DetectedIssue{detected(componenthealth.IssueOOMKilled, "pod-a")})
	issue := activeIssue(r, componenthealth.IssueOOMKilled)
	if assert.NotNil(t, issue, "first affected pod should surface the issue") {
		assert.ElementsMatch(t, []string{"pod-a"}, issue.AffectedPods)
		assert.Equal(t, testComponent, issue.Component)
	}
	assert.Equal(t, float64(1), detectedCount(t, componenthealth.IssueOOMKilled), "first affected pod counts once")

	// pod-b hits the same issue: already active -> still one issue, both pods.
	r.reconcilePod(podKey("pod-b"), testComponent, []componenthealth.DetectedIssue{detected(componenthealth.IssueOOMKilled, "pod-b")})
	assert.Len(t, r.snapshot(), 1, "second affected pod must not create a second component issue")
	assert.ElementsMatch(t, []string{"pod-a", "pod-b"}, activeIssue(r, componenthealth.IssueOOMKilled).AffectedPods)
	assert.Equal(t, float64(1), detectedCount(t, componenthealth.IssueOOMKilled), "second affected pod must not re-count")

	// pod-a recovers but pod-b still affected -> issue stays active.
	r.reconcilePod(podKey("pod-a"), testComponent, nil)
	issue = activeIssue(r, componenthealth.IssueOOMKilled)
	if assert.NotNil(t, issue, "issue is still active on pod-b") {
		assert.ElementsMatch(t, []string{"pod-b"}, issue.AffectedPods)
	}

	// pod-b recovers: last affected pod -> issue clears from the snapshot.
	r.reconcilePod(podKey("pod-b"), testComponent, nil)
	assert.Nil(t, activeIssue(r, componenthealth.IssueOOMKilled), "issue clears once the last pod recovers")
	assert.Empty(t, r.reported, "state should be cleared once resolved")
}

// TestComponentHealth_DistinctIssueTypes verifies each issue type on a component
// is tracked as its own instance.
func TestComponentHealth_DistinctIssueTypes(t *testing.T) {
	metrics.ComponentHealthIssuesDetected.Reset()
	r := newTestReconciler(&recordingEmitter{})

	r.reconcilePod(podKey("pod-a"), testComponent, []componenthealth.DetectedIssue{
		detected(componenthealth.IssueOOMKilled, "pod-a"),
		detected(componenthealth.IssueCrashLooping, "pod-a"),
	})
	assert.Len(t, r.snapshot(), 2, "distinct issue types should each be tracked")

	// Only the crash loop clears -> OOM stays active.
	r.reconcilePod(podKey("pod-a"), testComponent, []componenthealth.DetectedIssue{
		detected(componenthealth.IssueOOMKilled, "pod-a"),
	})
	assert.Nil(t, activeIssue(r, componenthealth.IssueCrashLooping))
	assert.NotNil(t, activeIssue(r, componenthealth.IssueOOMKilled))
}

// TestComponentHealth_SeverityFromIssueType verifies the component-level issue
// carries the static, catalog severity for its issue type, independent of the
// per-pod detections folded into it.
func TestComponentHealth_SeverityFromIssueType(t *testing.T) {
	r := newTestReconciler(&recordingEmitter{})

	r.reconcilePod(podKey("pod-a"), testComponent, []componenthealth.DetectedIssue{
		detected(componenthealth.IssueImagePullFailure, "pod-a"),
	})
	issue := activeIssue(r, componenthealth.IssueImagePullFailure)
	if assert.NotNil(t, issue) {
		assert.Equal(t, componenthealth.SeverityMedium, issue.Severity)
	}
}

// TestComponentHealth_SameIssueTypeMultipleContainers verifies that when a
// single pod detects the same issue type on more than one container (e.g. two
// crash-looping containers), the pod is registered once and the component
// issue only clears once every contributing container has recovered.
func TestComponentHealth_SameIssueTypeMultipleContainers(t *testing.T) {
	r := newTestReconciler(&recordingEmitter{})

	r.reconcilePod(podKey("pod-a"), testComponent, []componenthealth.DetectedIssue{
		detected(componenthealth.IssueCrashLooping, "pod-a"),
		detected(componenthealth.IssueCrashLooping, "pod-a"),
	})
	assert.Len(t, r.snapshot(), 1)

	// Only one of the two containers still reports the issue: the pod is still
	// affected, so the component issue must stay active.
	r.reconcilePod(podKey("pod-a"), testComponent, []componenthealth.DetectedIssue{
		detected(componenthealth.IssueCrashLooping, "pod-a"),
	})
	assert.NotNil(t, activeIssue(r, componenthealth.IssueCrashLooping), "pod is still affected via the other container")

	r.reconcilePod(podKey("pod-a"), testComponent, nil)
	assert.Nil(t, activeIssue(r, componenthealth.IssueCrashLooping), "clears once every container on the pod has recovered")
}

// TestComponentHealth_ForgetPodResolves verifies a disappearing pod is dropped
// from its component issues, clearing those it was the last pod of.
func TestComponentHealth_ForgetPodResolves(t *testing.T) {
	r := newTestReconciler(&recordingEmitter{})

	r.reconcilePod(podKey("pod-a"), testComponent, []componenthealth.DetectedIssue{detected(componenthealth.IssueOOMKilled, "pod-a")})
	assert.NotNil(t, activeIssue(r, componenthealth.IssueOOMKilled))

	r.forgetPod(podKey("pod-a"))
	assert.Nil(t, activeIssue(r, componenthealth.IssueOOMKilled), "removing the only affected pod clears the issue")
	assert.Empty(t, r.reported)
}

// TestComponentHealth_Snapshot verifies snapshot() returns every active
// component-level issue with its aggregated affected pods, in a stable order.
func TestComponentHealth_Snapshot(t *testing.T) {
	r := newTestReconciler(&recordingEmitter{})

	assert.Empty(t, r.snapshot(), "no issues -> empty snapshot")

	r.reconcilePod(podKey("pod-a"), testComponent, []componenthealth.DetectedIssue{
		detected(componenthealth.IssueOOMKilled, "pod-a"),
		detected(componenthealth.IssueCrashLooping, "pod-a"),
	})
	r.reconcilePod(podKey("pod-b"), testComponent, []componenthealth.DetectedIssue{
		detected(componenthealth.IssueOOMKilled, "pod-b"),
	})

	snap := r.snapshot()
	assert.Len(t, snap, 2)
	// Sorted by (component, namespace, issue_type): component_crash_looping < component_oomkilled.
	assert.Equal(t, componenthealth.IssueCrashLooping, snap[0].IssueType)
	assert.ElementsMatch(t, []string{"pod-a"}, snap[0].AffectedPods)
	assert.Equal(t, componenthealth.IssueOOMKilled, snap[1].IssueType)
	assert.ElementsMatch(t, []string{"pod-a", "pod-b"}, snap[1].AffectedPods)
}

// TestComponentHealth_EmitSnapshot verifies the snapshot loop hands the current
// active-issue set to the emitter.
func TestComponentHealth_EmitSnapshot(t *testing.T) {
	e := &recordingEmitter{}
	r := newTestReconciler(e)

	// No active issues: still emits (empty snapshot doubles as the liveness signal).
	r.emitSnapshot(context.Background())
	if assert.Len(t, e.snapshots, 1) {
		assert.Empty(t, e.snapshots[0])
	}

	r.reconcilePod(podKey("pod-a"), testComponent, []componenthealth.DetectedIssue{detected(componenthealth.IssueOOMKilled, "pod-a")})
	r.emitSnapshot(context.Background())
	if assert.Len(t, e.snapshots, 2) {
		assert.Len(t, e.snapshots[1], 1)
		assert.Equal(t, componenthealth.IssueOOMKilled, e.snapshots[1][0].IssueType)
	}
}

// TestComponentHealth_ActiveGauge verifies the active gauge tracks the number of
// pods currently exhibiting an issue and returns to 0 when it clears.
func TestComponentHealth_ActiveGauge(t *testing.T) {
	metrics.ComponentHealthIssuesActive.Reset()
	r := newTestReconciler(&recordingEmitter{})

	gauge := func() float64 {
		return testutil.ToFloat64(metrics.ComponentHealthIssuesActive.WithLabelValues(testComponent, componenthealth.IssueOOMKilled))
	}

	r.reconcilePod(podKey("pod-a"), testComponent, []componenthealth.DetectedIssue{detected(componenthealth.IssueOOMKilled, "pod-a")})
	assert.Equal(t, float64(1), gauge())

	r.reconcilePod(podKey("pod-b"), testComponent, []componenthealth.DetectedIssue{detected(componenthealth.IssueOOMKilled, "pod-b")})
	assert.Equal(t, float64(2), gauge())

	r.reconcilePod(podKey("pod-a"), testComponent, nil)
	assert.Equal(t, float64(1), gauge())

	r.reconcilePod(podKey("pod-b"), testComponent, nil)
	assert.Equal(t, float64(0), gauge())
}

// TestComponentHealth_ActiveGaugeAggregatesAcrossNamespaces verifies that the
// active gauge, which is not labeled by namespace, sums affected pods across
// every watched namespace rather than being clobbered by whichever namespace
// last reconciled.
func TestComponentHealth_ActiveGaugeAggregatesAcrossNamespaces(t *testing.T) {
	metrics.ComponentHealthIssuesActive.Reset()
	r := newTestReconciler(&recordingEmitter{})

	gauge := func() float64 {
		return testutil.ToFloat64(metrics.ComponentHealthIssuesActive.WithLabelValues(testComponent, componenthealth.IssueOOMKilled))
	}

	nsA := types.NamespacedName{Namespace: "namespace-a", Name: "pod-a"}
	nsB := types.NamespacedName{Namespace: "namespace-b", Name: "pod-b"}

	r.reconcilePod(nsA, testComponent, []componenthealth.DetectedIssue{detected(componenthealth.IssueOOMKilled, "pod-a")})
	assert.Equal(t, float64(1), gauge())

	r.reconcilePod(nsB, testComponent, []componenthealth.DetectedIssue{detected(componenthealth.IssueOOMKilled, "pod-b")})
	assert.Equal(t, float64(2), gauge())

	// Resolving namespace B's pod must not clobber namespace A's still-active issue.
	r.reconcilePod(nsB, testComponent, nil)
	assert.Equal(t, float64(1), gauge())

	r.reconcilePod(nsA, testComponent, nil)
	assert.Equal(t, float64(0), gauge())
}
