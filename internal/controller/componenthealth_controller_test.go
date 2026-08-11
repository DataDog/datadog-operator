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

// recordingEmitter captures the transitions the reconciler emits.
type recordingEmitter struct {
	emits    []componenthealth.ComponentIssue
	resolves []componenthealth.ComponentIssue
}

func (e *recordingEmitter) Emit(_ context.Context, issue componenthealth.ComponentIssue) {
	e.emits = append(e.emits, issue)
}

func (e *recordingEmitter) Resolve(_ context.Context, issue componenthealth.ComponentIssue) {
	e.resolves = append(e.resolves, issue)
}

func newTestReconciler(e componentHealthEmitter) *ComponentHealthReconciler {
	return &ComponentHealthReconciler{
		log:              logr.Discard(),
		restartThreshold: defaultRestartThreshold,
		emitter:          e,
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

// TestComponentHealth_AggregatesAcrossPods verifies that the same issue type on
// several pods of a component is a single component-level issue: one Emit on the
// first affected pod, no churn while any pod remains affected, one Resolve when
// the last recovers.
func TestComponentHealth_AggregatesAcrossPods(t *testing.T) {
	e := &recordingEmitter{}
	r := newTestReconciler(e)
	ctx := context.Background()

	// pod-a hits an OOMKill: first affected pod -> one component-level Emit.
	r.reconcilePod(ctx, podKey("pod-a"), testComponent, []componenthealth.DetectedIssue{detected(componenthealth.IssueOOMKilled, "pod-a")})
	assert.Len(t, e.emits, 1, "first affected pod should emit once")
	assert.Empty(t, e.resolves)
	assert.ElementsMatch(t, []string{"pod-a"}, e.emits[0].AffectedPods)
	assert.Equal(t, testComponent, e.emits[0].Component)

	// pod-b hits the same issue: already active -> no new Emit.
	r.reconcilePod(ctx, podKey("pod-b"), testComponent, []componenthealth.DetectedIssue{detected(componenthealth.IssueOOMKilled, "pod-b")})
	assert.Len(t, e.emits, 1, "second affected pod must not re-emit the component issue")
	assert.Empty(t, e.resolves)

	// pod-a recovers but pod-b still affected -> no Resolve yet.
	r.reconcilePod(ctx, podKey("pod-a"), testComponent, nil)
	assert.Empty(t, e.resolves, "issue is still active on pod-b")

	// pod-b recovers: last affected pod -> one component-level Resolve.
	r.reconcilePod(ctx, podKey("pod-b"), testComponent, nil)
	assert.Len(t, e.resolves, 1, "last recovering pod should resolve once")
	assert.Equal(t, componenthealth.IssueOOMKilled, e.resolves[0].IssueType)
	assert.Empty(t, r.reported, "state should be cleared once resolved")
}

// TestComponentHealth_DistinctIssueTypes verifies each issue type on a component
// is tracked as its own instance.
func TestComponentHealth_DistinctIssueTypes(t *testing.T) {
	e := &recordingEmitter{}
	r := newTestReconciler(e)
	ctx := context.Background()

	r.reconcilePod(ctx, podKey("pod-a"), testComponent, []componenthealth.DetectedIssue{
		detected(componenthealth.IssueOOMKilled, "pod-a"),
		detected(componenthealth.IssueCrashLooping, "pod-a"),
	})
	assert.Len(t, e.emits, 2, "distinct issue types should each emit")

	// Only the crash loop clears -> exactly one Resolve, OOM stays active.
	r.reconcilePod(ctx, podKey("pod-a"), testComponent, []componenthealth.DetectedIssue{
		detected(componenthealth.IssueOOMKilled, "pod-a"),
	})
	assert.Len(t, e.resolves, 1)
	assert.Equal(t, componenthealth.IssueCrashLooping, e.resolves[0].IssueType)
}

// TestComponentHealth_SeverityFromIssueType verifies the component-level issue
// carries the static, catalog severity for its issue type, independent of the
// per-pod detections folded into it.
func TestComponentHealth_SeverityFromIssueType(t *testing.T) {
	e := &recordingEmitter{}
	r := newTestReconciler(e)
	ctx := context.Background()

	r.reconcilePod(ctx, podKey("pod-a"), testComponent, []componenthealth.DetectedIssue{
		detected(componenthealth.IssueImagePullFailure, "pod-a"),
	})
	assert.Equal(t, componenthealth.SeverityMedium, e.emits[0].Severity)

	key := componentIssueKey{namespace: "datadog", component: testComponent, issueType: componenthealth.IssueImagePullFailure}
	assert.Equal(t, componenthealth.SeverityMedium, componenthealth.SeverityForIssueType(key.issueType))
}

// TestComponentHealth_SameIssueTypeMultipleContainers verifies that when a
// single pod detects the same issue type on more than one container (e.g. two
// crash-looping containers), the pod is registered once and the component
// issue only clears once every contributing container has recovered.
func TestComponentHealth_SameIssueTypeMultipleContainers(t *testing.T) {
	e := &recordingEmitter{}
	r := newTestReconciler(e)
	ctx := context.Background()

	r.reconcilePod(ctx, podKey("pod-a"), testComponent, []componenthealth.DetectedIssue{
		detected(componenthealth.IssueCrashLooping, "pod-a"),
		detected(componenthealth.IssueCrashLooping, "pod-a"),
	})
	assert.Len(t, e.emits, 1)

	// Only one of the two containers still reports the issue: the pod is still
	// affected, so the component issue must stay active.
	r.reconcilePod(ctx, podKey("pod-a"), testComponent, []componenthealth.DetectedIssue{
		detected(componenthealth.IssueCrashLooping, "pod-a"),
	})
	assert.Empty(t, e.resolves, "pod is still affected via the other container")

	r.reconcilePod(ctx, podKey("pod-a"), testComponent, nil)
	assert.Len(t, e.resolves, 1, "resolves once every container on the pod has recovered")
}

// TestComponentHealth_ForgetPodResolves verifies a disappearing pod is dropped
// from its component issues, resolving those it was the last pod of.
func TestComponentHealth_ForgetPodResolves(t *testing.T) {
	e := &recordingEmitter{}
	r := newTestReconciler(e)
	ctx := context.Background()

	r.reconcilePod(ctx, podKey("pod-a"), testComponent, []componenthealth.DetectedIssue{detected(componenthealth.IssueOOMKilled, "pod-a")})
	assert.Len(t, e.emits, 1)

	r.forgetPod(ctx, podKey("pod-a"))
	assert.Len(t, e.resolves, 1, "removing the only affected pod resolves the issue")
	assert.Empty(t, r.reported)
}

// TestComponentHealth_ActiveGauge verifies the active gauge tracks the number of
// pods currently exhibiting an issue and returns to 0 when it clears.
func TestComponentHealth_ActiveGauge(t *testing.T) {
	metrics.ComponentHealthIssuesActive.Reset()
	e := &recordingEmitter{}
	r := newTestReconciler(e)
	ctx := context.Background()

	gauge := func() float64 {
		return testutil.ToFloat64(metrics.ComponentHealthIssuesActive.WithLabelValues(testComponent, componenthealth.IssueOOMKilled))
	}

	r.reconcilePod(ctx, podKey("pod-a"), testComponent, []componenthealth.DetectedIssue{detected(componenthealth.IssueOOMKilled, "pod-a")})
	assert.Equal(t, float64(1), gauge())

	r.reconcilePod(ctx, podKey("pod-b"), testComponent, []componenthealth.DetectedIssue{detected(componenthealth.IssueOOMKilled, "pod-b")})
	assert.Equal(t, float64(2), gauge())

	r.reconcilePod(ctx, podKey("pod-a"), testComponent, nil)
	assert.Equal(t, float64(1), gauge())

	r.reconcilePod(ctx, podKey("pod-b"), testComponent, nil)
	assert.Equal(t, float64(0), gauge())
}

// TestComponentHealth_ActiveGaugeAggregatesAcrossNamespaces verifies that the
// active gauge, which is not labeled by namespace, sums affected pods across
// every watched namespace rather than being clobbered by whichever namespace
// last reconciled.
func TestComponentHealth_ActiveGaugeAggregatesAcrossNamespaces(t *testing.T) {
	metrics.ComponentHealthIssuesActive.Reset()
	e := &recordingEmitter{}
	r := newTestReconciler(e)
	ctx := context.Background()

	gauge := func() float64 {
		return testutil.ToFloat64(metrics.ComponentHealthIssuesActive.WithLabelValues(testComponent, componenthealth.IssueOOMKilled))
	}

	nsA := types.NamespacedName{Namespace: "namespace-a", Name: "pod-a"}
	nsB := types.NamespacedName{Namespace: "namespace-b", Name: "pod-b"}

	r.reconcilePod(ctx, nsA, testComponent, []componenthealth.DetectedIssue{detected(componenthealth.IssueOOMKilled, "pod-a")})
	assert.Equal(t, float64(1), gauge())

	r.reconcilePod(ctx, nsB, testComponent, []componenthealth.DetectedIssue{detected(componenthealth.IssueOOMKilled, "pod-b")})
	assert.Equal(t, float64(2), gauge())

	// Resolving namespace B's pod must not clobber namespace A's still-active issue.
	r.reconcilePod(ctx, nsB, testComponent, nil)
	assert.Equal(t, float64(1), gauge())

	r.reconcilePod(ctx, nsA, testComponent, nil)
	assert.Equal(t, float64(0), gauge())
}
