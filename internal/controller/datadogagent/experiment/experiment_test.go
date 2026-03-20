// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package experiment

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
)

const (
	testNamespace = "datadog"
	testDDAName   = "datadog-agent"
)

// --- Test helpers ---

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = v2alpha1.AddToScheme(s)
	_ = appsv1.AddToScheme(s)
	return s
}

func newTestDDA(spec v2alpha1.DatadogAgentSpec) *v2alpha1.DatadogAgent {
	return &v2alpha1.DatadogAgent{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogAgent",
			APIVersion: "datadoghq.com/v2alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      testDDAName,
			Namespace: testNamespace,
			UID:       types.UID("test-uid-123"),
		},
		Spec: spec,
	}
}

func newTestDDAWithStatus(spec v2alpha1.DatadogAgentSpec, status v2alpha1.DatadogAgentStatus) *v2alpha1.DatadogAgent {
	dda := newTestDDA(spec)
	dda.Status = status
	return dda
}

func specWithAPM(enabled bool) v2alpha1.DatadogAgentSpec {
	return v2alpha1.DatadogAgentSpec{
		Features: &v2alpha1.DatadogFeatures{
			APM: &v2alpha1.APMFeatureConfig{
				Enabled: apiutils.NewBoolPointer(enabled),
			},
		},
	}
}

func specWithNPM(enabled bool) v2alpha1.DatadogAgentSpec {
	return v2alpha1.DatadogAgentSpec{
		Features: &v2alpha1.DatadogFeatures{
			NPM: &v2alpha1.NPMFeatureConfig{
				Enabled: apiutils.NewBoolPointer(enabled),
			},
		},
	}
}

func buildFakeClient(s *runtime.Scheme, objs ...client.Object) client.Client {
	return fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(objs...).
		WithStatusSubresource(&v2alpha1.DatadogAgent{}).
		Build()
}

func makeControllerRevision(name, namespace string, owner *v2alpha1.DatadogAgent, spec v2alpha1.DatadogAgentSpec) *appsv1.ControllerRevision {
	data, _ := serializeSpec(&spec)
	cr := &appsv1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: runtime.RawExtension{Raw: data},
	}
	if owner != nil {
		cr.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: "datadoghq.com/v2alpha1",
				Kind:       "DatadogAgent",
				Name:       owner.Name,
				UID:        owner.UID,
				Controller: apiutils.NewBoolPointer(true),
			},
		}
	}
	return cr
}

func timePtr(t time.Time) *metav1.Time {
	mt := metav1.NewTime(t)
	return &mt
}

// ============================================================================
// ComputeSpecHash tests
// ============================================================================

func TestComputeSpecHash_Deterministic(t *testing.T) {
	spec := specWithAPM(true)
	hash1, err1 := ComputeSpecHash(&spec)
	hash2, err2 := ComputeSpecHash(&spec)

	require.NoError(t, err1)
	require.NoError(t, err2)
	assert.Equal(t, hash1, hash2, "same spec should produce same hash")
	assert.Len(t, hash1, RevisionHashLength, "hash should be truncated to RevisionHashLength")
}

func TestComputeSpecHash_DifferentSpecs(t *testing.T) {
	spec1 := specWithAPM(true)
	spec2 := specWithAPM(false)

	hash1, err1 := ComputeSpecHash(&spec1)
	hash2, err2 := ComputeSpecHash(&spec2)

	require.NoError(t, err1)
	require.NoError(t, err2)
	assert.NotEqual(t, hash1, hash2, "different specs should produce different hashes")
}

// ============================================================================
// RevisionName tests
// ============================================================================

func TestRevisionName_Format(t *testing.T) {
	name := RevisionName("datadog-agent", "abcdef1234")
	assert.Equal(t, "datadog-agent-abcdef1234", name)
}

// ============================================================================
// CreateControllerRevision tests
// ============================================================================

func TestCreateControllerRevision_New(t *testing.T) {
	s := testScheme()
	dda := newTestDDA(specWithAPM(true))
	c := buildFakeClient(s, dda)

	name, created, err := CreateControllerRevision(context.TODO(), c, dda, s)

	require.NoError(t, err)
	assert.True(t, created, "should have created a new revision")
	assert.NotEmpty(t, name)

	// Verify the ControllerRevision exists
	cr := &appsv1.ControllerRevision{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: testNamespace}, cr)
	require.NoError(t, err)
	assert.Equal(t, name, cr.Name)
	assert.NotEmpty(t, cr.Data.Raw, "revision should contain serialized spec data")
}

func TestCreateControllerRevision_AlreadyExists(t *testing.T) {
	s := testScheme()
	dda := newTestDDA(specWithAPM(true))

	// Pre-create the ControllerRevision
	hash, _ := ComputeSpecHash(&dda.Spec)
	existingName := RevisionName(testDDAName, hash)
	existingCR := makeControllerRevision(existingName, testNamespace, dda, dda.Spec)
	c := buildFakeClient(s, dda, existingCR)

	name, created, err := CreateControllerRevision(context.TODO(), c, dda, s)

	require.NoError(t, err)
	assert.False(t, created, "should not create a duplicate revision")
	assert.Equal(t, existingName, name)
}

func TestCreateControllerRevision_OwnerRef(t *testing.T) {
	s := testScheme()
	dda := newTestDDA(specWithAPM(true))
	c := buildFakeClient(s, dda)

	name, _, err := CreateControllerRevision(context.TODO(), c, dda, s)
	require.NoError(t, err)

	cr := &appsv1.ControllerRevision{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: name, Namespace: testNamespace}, cr)
	require.NoError(t, err)

	require.Len(t, cr.OwnerReferences, 1)
	ownerRef := cr.OwnerReferences[0]
	assert.Equal(t, dda.Name, ownerRef.Name)
	assert.Equal(t, dda.UID, ownerRef.UID)
	assert.Equal(t, "DatadogAgent", ownerRef.Kind)
	assert.NotNil(t, ownerRef.Controller)
	assert.True(t, *ownerRef.Controller)
}

// ============================================================================
// GetControllerRevision tests
// ============================================================================

func TestGetControllerRevision_Exists(t *testing.T) {
	s := testScheme()
	dda := newTestDDA(specWithAPM(true))
	existingCR := makeControllerRevision("test-revision", testNamespace, dda, dda.Spec)
	c := buildFakeClient(s, existingCR)

	cr, err := GetControllerRevision(context.TODO(), c, testNamespace, "test-revision")

	require.NoError(t, err)
	assert.Equal(t, "test-revision", cr.Name)
}

func TestGetControllerRevision_NotFound(t *testing.T) {
	s := testScheme()
	c := buildFakeClient(s)

	_, err := GetControllerRevision(context.TODO(), c, testNamespace, "nonexistent")

	require.Error(t, err)
}

// ============================================================================
// RestoreSpecFromRevision tests
// ============================================================================

func TestRestoreSpecFromRevision_Success(t *testing.T) {
	s := testScheme()
	originalSpec := specWithAPM(true)
	modifiedSpec := specWithNPM(true)

	dda := newTestDDA(modifiedSpec)                                                          // DDA currently has NPM
	baselineRevision := makeControllerRevision("baseline", testNamespace, dda, originalSpec) // Baseline has APM
	c := buildFakeClient(s, dda, baselineRevision)

	err := RestoreSpecFromRevision(context.TODO(), c, dda, "baseline")
	require.NoError(t, err)

	// Verify the DDA spec was restored to the baseline
	restored := &v2alpha1.DatadogAgent{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: testDDAName, Namespace: testNamespace}, restored)
	require.NoError(t, err)

	assert.NotNil(t, restored.Spec.Features.APM, "APM should be restored from baseline")
	assert.True(t, apiutils.BoolValue(restored.Spec.Features.APM.Enabled), "APM should be enabled")
}

func TestRestoreSpecFromRevision_NotFound(t *testing.T) {
	s := testScheme()
	dda := newTestDDA(specWithAPM(true))
	c := buildFakeClient(s, dda)

	err := RestoreSpecFromRevision(context.TODO(), c, dda, "nonexistent-revision")

	require.Error(t, err)
}

// ============================================================================
// ListOwnedRevisions tests
// ============================================================================

func TestListOwnedRevisions_FiltersUnowned(t *testing.T) {
	s := testScheme()
	dda := newTestDDA(specWithAPM(true))

	// Owned by this DDA
	ownedCR := makeControllerRevision("owned-rev", testNamespace, dda, dda.Spec)
	// Not owned by anyone
	unownedCR := makeControllerRevision("unowned-rev", testNamespace, nil, dda.Spec)
	c := buildFakeClient(s, dda, ownedCR, unownedCR)

	revisions, err := ListOwnedRevisions(context.TODO(), c, dda)

	require.NoError(t, err)
	assert.Len(t, revisions, 1, "should only return owned revisions")
	assert.Equal(t, "owned-rev", revisions[0].Name)
}

// ============================================================================
// GarbageCollectRevisions tests
// ============================================================================

func TestGC_KeepsCurrent(t *testing.T) {
	s := testScheme()
	dda := newTestDDA(specWithAPM(true))
	currentCR := makeControllerRevision("current-rev", testNamespace, dda, dda.Spec)
	c := buildFakeClient(s, dda, currentCR)

	keep := map[string]bool{"current-rev": true}
	err := GarbageCollectRevisions(context.TODO(), c, dda, keep)

	require.NoError(t, err)

	cr := &appsv1.ControllerRevision{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: "current-rev", Namespace: testNamespace}, cr)
	require.NoError(t, err, "current revision should not be deleted")
}

func TestGC_KeepsPrevious(t *testing.T) {
	s := testScheme()
	dda := newTestDDA(specWithAPM(true))
	previousCR := makeControllerRevision("previous-rev", testNamespace, dda, dda.Spec)
	c := buildFakeClient(s, dda, previousCR)

	keep := map[string]bool{"previous-rev": true}
	err := GarbageCollectRevisions(context.TODO(), c, dda, keep)

	require.NoError(t, err)

	cr := &appsv1.ControllerRevision{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: "previous-rev", Namespace: testNamespace}, cr)
	require.NoError(t, err, "previous revision should not be deleted")
}

func TestGC_KeepsBaseline(t *testing.T) {
	s := testScheme()
	dda := newTestDDA(specWithAPM(true))
	baselineCR := makeControllerRevision("baseline-rev", testNamespace, dda, dda.Spec)
	orphanCR := makeControllerRevision("orphan-rev", testNamespace, dda, dda.Spec)
	c := buildFakeClient(s, dda, baselineCR, orphanCR)

	keep := map[string]bool{"baseline-rev": true}
	err := GarbageCollectRevisions(context.TODO(), c, dda, keep)

	require.NoError(t, err)

	// Baseline should survive
	cr := &appsv1.ControllerRevision{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: "baseline-rev", Namespace: testNamespace}, cr)
	require.NoError(t, err, "baseline revision should not be deleted")

	// Orphan should be deleted
	err = c.Get(context.TODO(), types.NamespacedName{Name: "orphan-rev", Namespace: testNamespace}, cr)
	require.Error(t, err, "orphan revision should be deleted")
}

func TestGC_DeletesOrphans(t *testing.T) {
	s := testScheme()
	dda := newTestDDA(specWithAPM(true))
	orphan1 := makeControllerRevision("orphan-1", testNamespace, dda, dda.Spec)
	orphan2 := makeControllerRevision("orphan-2", testNamespace, dda, dda.Spec)
	keepCR := makeControllerRevision("keep-me", testNamespace, dda, dda.Spec)
	c := buildFakeClient(s, dda, orphan1, orphan2, keepCR)

	keep := map[string]bool{"keep-me": true}
	err := GarbageCollectRevisions(context.TODO(), c, dda, keep)

	require.NoError(t, err)

	cr := &appsv1.ControllerRevision{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: "keep-me", Namespace: testNamespace}, cr)
	require.NoError(t, err, "kept revision should exist")

	err = c.Get(context.TODO(), types.NamespacedName{Name: "orphan-1", Namespace: testNamespace}, cr)
	require.Error(t, err, "orphan-1 should be deleted")

	err = c.Get(context.TODO(), types.NamespacedName{Name: "orphan-2", Namespace: testNamespace}, cr)
	require.Error(t, err, "orphan-2 should be deleted")
}

func TestGC_NoRevisions(t *testing.T) {
	s := testScheme()
	dda := newTestDDA(specWithAPM(true))
	c := buildFakeClient(s, dda)

	keep := map[string]bool{"anything": true}
	err := GarbageCollectRevisions(context.TODO(), c, dda, keep)

	require.NoError(t, err)
}

func TestGC_AllKept(t *testing.T) {
	s := testScheme()
	dda := newTestDDA(specWithAPM(true))
	cr1 := makeControllerRevision("rev-1", testNamespace, dda, dda.Spec)
	cr2 := makeControllerRevision("rev-2", testNamespace, dda, dda.Spec)
	c := buildFakeClient(s, dda, cr1, cr2)

	keep := map[string]bool{"rev-1": true, "rev-2": true}
	err := GarbageCollectRevisions(context.TODO(), c, dda, keep)

	require.NoError(t, err)

	// Both should still exist
	cr := &appsv1.ControllerRevision{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: "rev-1", Namespace: testNamespace}, cr)
	require.NoError(t, err)
	err = c.Get(context.TODO(), types.NamespacedName{Name: "rev-2", Namespace: testNamespace}, cr)
	require.NoError(t, err)
}

// ============================================================================
// BuildKeepSet tests
// ============================================================================

func TestBuildKeepSet_NoExperiment(t *testing.T) {
	status := &v2alpha1.DatadogAgentStatus{
		CurrentRevision:  "current-rev",
		PreviousRevision: "previous-rev",
	}

	keep := BuildKeepSet(status)

	assert.True(t, keep["current-rev"])
	assert.True(t, keep["previous-rev"])
	assert.Len(t, keep, 2)
}

func TestBuildKeepSet_WithExperiment(t *testing.T) {
	status := &v2alpha1.DatadogAgentStatus{
		CurrentRevision:  "current-rev",
		PreviousRevision: "previous-rev",
		Experiment: &v2alpha1.ExperimentStatus{
			Phase:            v2alpha1.ExperimentPhaseRunning,
			BaselineRevision: "baseline-rev",
		},
	}

	keep := BuildKeepSet(status)

	assert.True(t, keep["current-rev"])
	assert.True(t, keep["previous-rev"])
	assert.True(t, keep["baseline-rev"])
	assert.Len(t, keep, 3)
}

func TestBuildKeepSet_EmptyStringsExcluded(t *testing.T) {
	status := &v2alpha1.DatadogAgentStatus{
		CurrentRevision:  "current-rev",
		PreviousRevision: "", // empty
		Experiment: &v2alpha1.ExperimentStatus{
			BaselineRevision: "", // empty
		},
	}

	keep := BuildKeepSet(status)

	assert.True(t, keep["current-rev"])
	assert.False(t, keep[""])
	assert.Len(t, keep, 1)
}

// ============================================================================
// checkTimeout tests
// ============================================================================

func TestCheckTimeout_NotExpired(t *testing.T) {
	now := time.Now()
	exp := &v2alpha1.ExperimentStatus{
		Phase:     v2alpha1.ExperimentPhaseRunning,
		StartedAt: timePtr(now.Add(-10 * time.Minute)),
	}

	result := CheckTimeout(exp, now, 30*time.Minute)
	assert.False(t, result)
}

func TestCheckTimeout_Expired(t *testing.T) {
	now := time.Now()
	exp := &v2alpha1.ExperimentStatus{
		Phase:     v2alpha1.ExperimentPhaseRunning,
		StartedAt: timePtr(now.Add(-31 * time.Minute)),
	}

	result := CheckTimeout(exp, now, 30*time.Minute)
	assert.True(t, result)
}

func TestCheckTimeout_NilStartedAt(t *testing.T) {
	exp := &v2alpha1.ExperimentStatus{
		Phase: v2alpha1.ExperimentPhaseRunning,
	}

	result := CheckTimeout(exp, time.Now(), 30*time.Minute)
	assert.False(t, result)
}

// ============================================================================
// checkConflict tests
// ============================================================================

func TestCheckConflict_NoConflict(t *testing.T) {
	s := testScheme()
	spec := specWithAPM(true)
	dda := newTestDDA(spec)

	hash, _ := ComputeSpecHash(&spec)
	revName := RevisionName(testDDAName, hash)
	cr := makeControllerRevision(revName, testNamespace, dda, spec)

	dda.Status.CurrentRevision = revName
	c := buildFakeClient(s, dda, cr)

	conflict, err := CheckConflict(context.TODO(), c, dda)
	require.NoError(t, err)
	assert.False(t, conflict)
}

func TestCheckConflict_Detected(t *testing.T) {
	s := testScheme()
	originalSpec := specWithAPM(true)
	modifiedSpec := specWithNPM(true)

	// DDA has modified spec, but currentRevision points to original
	dda := newTestDDA(modifiedSpec)
	hash, _ := ComputeSpecHash(&originalSpec)
	revName := RevisionName(testDDAName, hash)
	cr := makeControllerRevision(revName, testNamespace, dda, originalSpec)

	dda.Status.CurrentRevision = revName
	c := buildFakeClient(s, dda, cr)

	conflict, err := CheckConflict(context.TODO(), c, dda)
	require.NoError(t, err)
	assert.True(t, conflict)
}

func TestCheckConflict_NoCurrentRevision(t *testing.T) {
	s := testScheme()
	dda := newTestDDA(specWithAPM(true))
	// No currentRevision set
	c := buildFakeClient(s, dda)

	conflict, err := CheckConflict(context.TODO(), c, dda)
	require.NoError(t, err)
	assert.False(t, conflict, "no conflict when no currentRevision is set")
}

// ============================================================================
// HandleExperimentLifecycle tests — Phase transitions
// ============================================================================

func TestLifecycle_NoExperiment_CreatesRevision(t *testing.T) {
	s := testScheme()
	spec := specWithAPM(true)
	dda := newTestDDA(spec)
	c := buildFakeClient(s, dda)

	shouldReturn, _, err := HandleExperimentLifecycle(context.TODO(), c, dda, s, time.Now(), DefaultExperimentTimeout)

	require.NoError(t, err)
	assert.False(t, shouldReturn, "should not return early when no experiment")
	assert.NotEmpty(t, dda.Status.CurrentRevision, "currentRevision should be set")
}

func TestLifecycle_NoExperiment_SpecUnchanged(t *testing.T) {
	s := testScheme()
	spec := specWithAPM(true)
	hash, _ := ComputeSpecHash(&spec)
	revName := RevisionName(testDDAName, hash)

	dda := newTestDDAWithStatus(spec, v2alpha1.DatadogAgentStatus{
		CurrentRevision: revName,
	})

	cr := makeControllerRevision(revName, testNamespace, dda, spec)
	c := buildFakeClient(s, dda, cr)

	shouldReturn, _, err := HandleExperimentLifecycle(context.TODO(), c, dda, s, time.Now(), DefaultExperimentTimeout)

	require.NoError(t, err)
	assert.False(t, shouldReturn)
	assert.Equal(t, revName, dda.Status.CurrentRevision, "revision should not change")
}

func TestLifecycle_Running_SetsStartedAt(t *testing.T) {
	s := testScheme()
	spec := specWithAPM(true)
	hash, _ := ComputeSpecHash(&spec)
	revName := RevisionName(testDDAName, hash)

	dda := newTestDDAWithStatus(spec, v2alpha1.DatadogAgentStatus{
		CurrentRevision: revName,
		Experiment: &v2alpha1.ExperimentStatus{
			Phase: v2alpha1.ExperimentPhaseRunning,
			// StartedAt is nil — should be set by the lifecycle handler
		},
	})

	cr := makeControllerRevision(revName, testNamespace, dda, spec)
	c := buildFakeClient(s, dda, cr)

	now := time.Now()
	shouldReturn, _, err := HandleExperimentLifecycle(context.TODO(), c, dda, s, now, DefaultExperimentTimeout)

	require.NoError(t, err)
	assert.False(t, shouldReturn)
	assert.NotNil(t, dda.Status.Experiment.StartedAt, "startedAt should be set")
}

func TestLifecycle_Running_NoConflictNoTimeout(t *testing.T) {
	s := testScheme()
	spec := specWithAPM(true)
	hash, _ := ComputeSpecHash(&spec)
	revName := RevisionName(testDDAName, hash)

	now := time.Now()
	dda := newTestDDAWithStatus(spec, v2alpha1.DatadogAgentStatus{
		CurrentRevision: revName,
		Experiment: &v2alpha1.ExperimentStatus{
			Phase:            v2alpha1.ExperimentPhaseRunning,
			StartedAt:        timePtr(now.Add(-5 * time.Minute)), // well within timeout
			BaselineRevision: "baseline-rev",
		},
	})

	cr := makeControllerRevision(revName, testNamespace, dda, spec)
	baselineCR := makeControllerRevision("baseline-rev", testNamespace, dda, specWithNPM(true))
	c := buildFakeClient(s, dda, cr, baselineCR)

	shouldReturn, _, err := HandleExperimentLifecycle(context.TODO(), c, dda, s, now, DefaultExperimentTimeout)

	require.NoError(t, err)
	assert.False(t, shouldReturn, "should continue reconciliation normally")
	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, dda.Status.Experiment.Phase)
}

func TestLifecycle_Rollback_RestoresBaseline(t *testing.T) {
	s := testScheme()
	experimentSpec := specWithNPM(true)
	baselineSpec := specWithAPM(true)

	hash, _ := ComputeSpecHash(&experimentSpec)
	revName := RevisionName(testDDAName, hash)

	dda := newTestDDAWithStatus(experimentSpec, v2alpha1.DatadogAgentStatus{
		CurrentRevision: revName,
		Experiment: &v2alpha1.ExperimentStatus{
			Phase:            v2alpha1.ExperimentPhaseRollback,
			BaselineRevision: "baseline-rev",
		},
	})

	cr := makeControllerRevision(revName, testNamespace, dda, experimentSpec)
	baselineCR := makeControllerRevision("baseline-rev", testNamespace, dda, baselineSpec)
	c := buildFakeClient(s, dda, cr, baselineCR)

	shouldReturn, _, err := HandleExperimentLifecycle(context.TODO(), c, dda, s, time.Now(), DefaultExperimentTimeout)

	require.NoError(t, err)
	assert.True(t, shouldReturn, "should return early to trigger re-reconcile after spec restore")

	// Phase stays as rollback (persisted to API, observable) — cleared on next reconcile
	require.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseRollback, dda.Status.Experiment.Phase)
	assert.Nil(t, dda.Status.Experiment.StartedAt, "startedAt should be cleared")
	assert.Empty(t, dda.Status.Experiment.BaselineRevision, "baselineRevision cleared to signal restore done")

	// Verify spec was restored
	restored := &v2alpha1.DatadogAgent{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: testDDAName, Namespace: testNamespace}, restored)
	require.NoError(t, err)
	assert.NotNil(t, restored.Spec.Features.APM, "APM should be restored from baseline")
}

// TestLifecycle_Rollback_SecondReconcile_ClearsExperiment verifies that after
// the rollback phase is persisted, the next reconcile clears the experiment.
func TestLifecycle_Rollback_SecondReconcile_ClearsExperiment(t *testing.T) {
	s := testScheme()
	restoredSpec := specWithAPM(true)
	hash, _ := ComputeSpecHash(&restoredSpec)
	revName := RevisionName(testDDAName, hash)

	// Simulate post-rollback state: spec is restored, phase=rollback persisted,
	// baselineRevision cleared by handleRestore
	dda := newTestDDAWithStatus(restoredSpec, v2alpha1.DatadogAgentStatus{
		CurrentRevision: revName,
		Experiment: &v2alpha1.ExperimentStatus{
			Phase: v2alpha1.ExperimentPhaseRollback,
			// BaselineRevision is empty — signals restore already happened
		},
	})

	cr := makeControllerRevision(revName, testNamespace, dda, restoredSpec)
	c := buildFakeClient(s, dda, cr)

	shouldReturn, _, err := HandleExperimentLifecycle(context.TODO(), c, dda, s, time.Now(), DefaultExperimentTimeout)

	require.NoError(t, err)
	assert.False(t, shouldReturn, "should not return early — just clearing state")
	assert.Nil(t, dda.Status.Experiment, "experiment should be cleared on second reconcile")
}

func TestLifecycle_Rollback_MissingBaseline(t *testing.T) {
	s := testScheme()
	spec := specWithNPM(true)
	hash, _ := ComputeSpecHash(&spec)
	revName := RevisionName(testDDAName, hash)

	dda := newTestDDAWithStatus(spec, v2alpha1.DatadogAgentStatus{
		CurrentRevision: revName,
		Experiment: &v2alpha1.ExperimentStatus{
			Phase:            v2alpha1.ExperimentPhaseRollback,
			BaselineRevision: "nonexistent-baseline",
		},
	})

	cr := makeControllerRevision(revName, testNamespace, dda, spec)
	c := buildFakeClient(s, dda, cr)

	_, _, err := HandleExperimentLifecycle(context.TODO(), c, dda, s, time.Now(), DefaultExperimentTimeout)

	require.Error(t, err, "should error when baseline revision is missing")
}

func TestLifecycle_Promoted_ClearsExperiment(t *testing.T) {
	s := testScheme()
	spec := specWithAPM(true)
	hash, _ := ComputeSpecHash(&spec)
	revName := RevisionName(testDDAName, hash)

	dda := newTestDDAWithStatus(spec, v2alpha1.DatadogAgentStatus{
		CurrentRevision: revName,
		Experiment: &v2alpha1.ExperimentStatus{
			Phase:            v2alpha1.ExperimentPhasePromoted,
			BaselineRevision: "baseline-rev",
		},
	})

	cr := makeControllerRevision(revName, testNamespace, dda, spec)
	baselineCR := makeControllerRevision("baseline-rev", testNamespace, dda, specWithNPM(true))
	c := buildFakeClient(s, dda, cr, baselineCR)

	shouldReturn, _, err := HandleExperimentLifecycle(context.TODO(), c, dda, s, time.Now(), DefaultExperimentTimeout)

	require.NoError(t, err)
	assert.False(t, shouldReturn)
	// Experiment should be cleared
	assert.Nil(t, dda.Status.Experiment, "experiment should be cleared after promotion")
}

func TestLifecycle_Aborted_NoAction(t *testing.T) {
	s := testScheme()
	spec := specWithNPM(true) // User's edit
	hash, _ := ComputeSpecHash(&spec)
	revName := RevisionName(testDDAName, hash)

	dda := newTestDDAWithStatus(spec, v2alpha1.DatadogAgentStatus{
		CurrentRevision: revName,
		Experiment: &v2alpha1.ExperimentStatus{
			Phase:            v2alpha1.ExperimentPhaseAborted,
			BaselineRevision: "baseline-rev",
		},
	})

	cr := makeControllerRevision(revName, testNamespace, dda, spec)
	baselineCR := makeControllerRevision("baseline-rev", testNamespace, dda, specWithAPM(true))
	c := buildFakeClient(s, dda, cr, baselineCR)

	shouldReturn, _, err := HandleExperimentLifecycle(context.TODO(), c, dda, s, time.Now(), DefaultExperimentTimeout)

	require.NoError(t, err)
	assert.False(t, shouldReturn, "should not restore spec on abort — user's edit wins")
	// Spec should remain NPM (user's edit), not APM (baseline)
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, dda.Status.Experiment.Phase)
}

// ============================================================================
// Timeout lifecycle tests
// ============================================================================

func TestLifecycle_Timeout_RestoresBaseline(t *testing.T) {
	s := testScheme()
	experimentSpec := specWithNPM(true)
	baselineSpec := specWithAPM(true)

	hash, _ := ComputeSpecHash(&experimentSpec)
	revName := RevisionName(testDDAName, hash)

	now := time.Now()
	dda := newTestDDAWithStatus(experimentSpec, v2alpha1.DatadogAgentStatus{
		CurrentRevision: revName,
		Experiment: &v2alpha1.ExperimentStatus{
			Phase:            v2alpha1.ExperimentPhaseRunning,
			StartedAt:        timePtr(now.Add(-31 * time.Minute)), // past timeout
			BaselineRevision: "baseline-rev",
		},
	})

	cr := makeControllerRevision(revName, testNamespace, dda, experimentSpec)
	baselineCR := makeControllerRevision("baseline-rev", testNamespace, dda, baselineSpec)
	c := buildFakeClient(s, dda, cr, baselineCR)

	shouldReturn, _, err := HandleExperimentLifecycle(context.TODO(), c, dda, s, now, DefaultExperimentTimeout)

	require.NoError(t, err)
	assert.True(t, shouldReturn, "should return early after timeout rollback")

	// Phase set to timeout (persisted to API, observable by FA) — cleared on next reconcile
	require.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseTimeout, dda.Status.Experiment.Phase)

	// Verify spec was restored
	restored := &v2alpha1.DatadogAgent{}
	err = c.Get(context.TODO(), types.NamespacedName{Name: testDDAName, Namespace: testNamespace}, restored)
	require.NoError(t, err)
	assert.NotNil(t, restored.Spec.Features.APM, "APM should be restored from baseline")
}

func TestLifecycle_Timeout_AfterRestart(t *testing.T) {
	s := testScheme()
	experimentSpec := specWithNPM(true)
	baselineSpec := specWithAPM(true)

	hash, _ := ComputeSpecHash(&experimentSpec)
	revName := RevisionName(testDDAName, hash)

	now := time.Now()
	// Simulate: operator was down and now startedAt is way in the past
	dda := newTestDDAWithStatus(experimentSpec, v2alpha1.DatadogAgentStatus{
		CurrentRevision: revName,
		Experiment: &v2alpha1.ExperimentStatus{
			Phase:            v2alpha1.ExperimentPhaseRunning,
			StartedAt:        timePtr(now.Add(-2 * time.Hour)), // way past timeout
			BaselineRevision: "baseline-rev",
		},
	})

	cr := makeControllerRevision(revName, testNamespace, dda, experimentSpec)
	baselineCR := makeControllerRevision("baseline-rev", testNamespace, dda, baselineSpec)
	c := buildFakeClient(s, dda, cr, baselineCR)

	shouldReturn, _, err := HandleExperimentLifecycle(context.TODO(), c, dda, s, now, DefaultExperimentTimeout)

	require.NoError(t, err)
	assert.True(t, shouldReturn, "should immediately roll back after restart if timeout passed")
	require.NotNil(t, dda.Status.Experiment)
	assert.Equal(t, v2alpha1.ExperimentPhaseTimeout, dda.Status.Experiment.Phase)
}

// ============================================================================
// Conflict detection lifecycle tests
// ============================================================================

func TestLifecycle_Conflict_Aborts(t *testing.T) {
	s := testScheme()
	// DDA has been externally modified to NPM
	modifiedSpec := specWithNPM(true)
	// But currentRevision points to the experiment spec (APM)
	experimentSpec := specWithAPM(true)
	expHash, _ := ComputeSpecHash(&experimentSpec)
	expRevName := RevisionName(testDDAName, expHash)

	now := time.Now()
	dda := newTestDDAWithStatus(modifiedSpec, v2alpha1.DatadogAgentStatus{
		CurrentRevision: expRevName,
		Experiment: &v2alpha1.ExperimentStatus{
			Phase:            v2alpha1.ExperimentPhaseRunning,
			StartedAt:        timePtr(now.Add(-5 * time.Minute)),
			BaselineRevision: "baseline-rev",
		},
	})

	expCR := makeControllerRevision(expRevName, testNamespace, dda, experimentSpec)
	baselineCR := makeControllerRevision("baseline-rev", testNamespace, dda, specWithAPM(false))
	c := buildFakeClient(s, dda, expCR, baselineCR)

	shouldReturn, _, err := HandleExperimentLifecycle(context.TODO(), c, dda, s, now, DefaultExperimentTimeout)

	require.NoError(t, err)
	// On conflict, we abort but still need to create a revision for the new spec
	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, dda.Status.Experiment.Phase)
	assert.Nil(t, dda.Status.Experiment.StartedAt, "startedAt should be cleared on abort")
	// shouldReturn depends on implementation — conflict detection doesn't restore spec
	_ = shouldReturn
}

func TestLifecycle_Conflict_PreservesBaseline(t *testing.T) {
	s := testScheme()
	modifiedSpec := specWithNPM(true)
	experimentSpec := specWithAPM(true)
	expHash, _ := ComputeSpecHash(&experimentSpec)
	expRevName := RevisionName(testDDAName, expHash)

	now := time.Now()
	dda := newTestDDAWithStatus(modifiedSpec, v2alpha1.DatadogAgentStatus{
		CurrentRevision: expRevName,
		Experiment: &v2alpha1.ExperimentStatus{
			Phase:            v2alpha1.ExperimentPhaseRunning,
			StartedAt:        timePtr(now.Add(-5 * time.Minute)),
			BaselineRevision: "baseline-rev",
		},
	})

	expCR := makeControllerRevision(expRevName, testNamespace, dda, experimentSpec)
	baselineCR := makeControllerRevision("baseline-rev", testNamespace, dda, specWithAPM(false))
	c := buildFakeClient(s, dda, expCR, baselineCR)

	_, _, err := HandleExperimentLifecycle(context.TODO(), c, dda, s, now, DefaultExperimentTimeout)

	require.NoError(t, err)
	assert.Equal(t, "baseline-rev", dda.Status.Experiment.BaselineRevision,
		"baselineRevision should be preserved after abort for FA acknowledgment")
}

// ============================================================================
// Revision pointer tracking tests
// ============================================================================

func TestRevisionPointers_FirstReconcile(t *testing.T) {
	s := testScheme()
	spec := specWithAPM(true)
	dda := newTestDDA(spec) // No status set yet
	c := buildFakeClient(s, dda)

	_, _, err := HandleExperimentLifecycle(context.TODO(), c, dda, s, time.Now(), DefaultExperimentTimeout)

	require.NoError(t, err)
	assert.NotEmpty(t, dda.Status.CurrentRevision, "currentRevision should be set on first reconcile")
	assert.Empty(t, dda.Status.PreviousRevision, "previousRevision should be empty on first reconcile")
}

func TestRevisionPointers_SpecChange(t *testing.T) {
	s := testScheme()
	oldSpec := specWithAPM(true)
	newSpec := specWithNPM(true)

	oldHash, _ := ComputeSpecHash(&oldSpec)
	oldRevName := RevisionName(testDDAName, oldHash)
	oldCR := makeControllerRevision(oldRevName, testNamespace, nil, oldSpec)

	// DDA currently has the new spec, but currentRevision points to old
	dda := newTestDDAWithStatus(newSpec, v2alpha1.DatadogAgentStatus{
		CurrentRevision: oldRevName,
	})
	// Set UID and ownerRef for the old CR
	oldCR.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: "datadoghq.com/v2alpha1",
			Kind:       "DatadogAgent",
			Name:       dda.Name,
			UID:        dda.UID,
			Controller: apiutils.NewBoolPointer(true),
		},
	}
	c := buildFakeClient(s, dda, oldCR)

	_, _, err := HandleExperimentLifecycle(context.TODO(), c, dda, s, time.Now(), DefaultExperimentTimeout)

	require.NoError(t, err)
	assert.Equal(t, oldRevName, dda.Status.PreviousRevision, "previousRevision should be the old currentRevision")
	assert.NotEqual(t, oldRevName, dda.Status.CurrentRevision, "currentRevision should be updated to new hash")
}

// TestLifecycle_FirstReconcile_FAPayloadMatches tests the happy path:
// RC sets ExpectedSpecHash, patches spec to B. First reconcile sees spec=B,
// hash matches ExpectedSpecHash → experiment proceeds normally.
func TestLifecycle_FirstReconcile_FAPayloadMatches(t *testing.T) {
	s := testScheme()
	experimentSpec := specWithNPM(true)
	expHash, _ := ComputeSpecHash(&experimentSpec)

	baselineSpec := specWithAPM(false)
	baselineHash, _ := ComputeSpecHash(&baselineSpec)
	baselineRevName := RevisionName(testDDAName, baselineHash)

	now := time.Now()
	dda := newTestDDAWithStatus(experimentSpec, v2alpha1.DatadogAgentStatus{
		CurrentRevision: baselineRevName,
		Experiment: &v2alpha1.ExperimentStatus{
			Phase:            v2alpha1.ExperimentPhaseRunning,
			BaselineRevision: baselineRevName,
			ExpectedSpecHash: expHash, // RC set this
		},
	})

	baselineCR := makeControllerRevision(baselineRevName, testNamespace, dda, baselineSpec)
	c := buildFakeClient(s, dda, baselineCR)

	_, _, err := HandleExperimentLifecycle(context.TODO(), c, dda, s, now, DefaultExperimentTimeout)
	require.NoError(t, err)

	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, dda.Status.Experiment.Phase,
		"experiment should continue — spec matches FA payload")
	assert.Empty(t, dda.Status.Experiment.ExpectedSpecHash,
		"ExpectedSpecHash should be cleared after successful validation")
}

// TestLifecycle_StatusBeforeSpec_HashSurvives tests the race where the RC
// status update (setting ExpectedSpecHash) triggers a reconcile before the
// spec patch arrives. The spec is still the baseline, so specChanged=false.
// ExpectedSpecHash must NOT be cleared — it must survive until the spec
// patch arrives on a subsequent reconcile.
func TestLifecycle_StatusBeforeSpec_HashSurvives(t *testing.T) {
	s := testScheme()
	// Spec is still the baseline (RC hasn't patched spec yet)
	baselineSpec := specWithAPM(false)
	baselineHash, _ := ComputeSpecHash(&baselineSpec)
	baselineRevName := RevisionName(testDDAName, baselineHash)

	// But status already has ExpectedSpecHash (RC updated status first)
	experimentSpec := specWithNPM(true)
	expHash, _ := ComputeSpecHash(&experimentSpec)

	now := time.Now()
	dda := newTestDDAWithStatus(baselineSpec, v2alpha1.DatadogAgentStatus{
		CurrentRevision: baselineRevName,
		Experiment: &v2alpha1.ExperimentStatus{
			Phase:            v2alpha1.ExperimentPhaseRunning,
			BaselineRevision: baselineRevName,
			ExpectedSpecHash: expHash,
		},
	})

	baselineCR := makeControllerRevision(baselineRevName, testNamespace, dda, baselineSpec)
	c := buildFakeClient(s, dda, baselineCR)

	// First reconcile: spec unchanged (still baseline), status has ExpectedSpecHash
	_, _, err := HandleExperimentLifecycle(context.TODO(), c, dda, s, now, DefaultExperimentTimeout)
	require.NoError(t, err)

	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, dda.Status.Experiment.Phase,
		"experiment should still be running — spec hasn't changed yet")
	assert.Equal(t, expHash, dda.Status.Experiment.ExpectedSpecHash,
		"ExpectedSpecHash must survive when spec hasn't changed")

	// Second reconcile: spec patch arrives (NPM enabled)
	dda.Spec = experimentSpec
	_, _, err = HandleExperimentLifecycle(context.TODO(), c, dda, s, now, DefaultExperimentTimeout)
	require.NoError(t, err)

	assert.Equal(t, v2alpha1.ExperimentPhaseRunning, dda.Status.Experiment.Phase,
		"experiment should continue — spec matches ExpectedSpecHash")
	assert.Empty(t, dda.Status.Experiment.ExpectedSpecHash,
		"ExpectedSpecHash should be cleared after successful validation")
}

// TestLifecycle_FirstReconcile_UserEditBeforeReconcile tests the edge case:
// RC patches spec to B, but user edits to C before the first reconcile.
// ExpectedSpecHash != hash(C) → abort immediately.
func TestLifecycle_FirstReconcile_UserEditBeforeReconcile(t *testing.T) {
	s := testScheme()
	// FA intended spec B (NPM)
	experimentSpec := specWithNPM(true)
	expHash, _ := ComputeSpecHash(&experimentSpec)

	// But user changed spec to C (APM enabled)
	userSpec := specWithAPM(true)

	baselineSpec := specWithAPM(false)
	baselineHash, _ := ComputeSpecHash(&baselineSpec)
	baselineRevName := RevisionName(testDDAName, baselineHash)

	now := time.Now()
	dda := newTestDDAWithStatus(userSpec, v2alpha1.DatadogAgentStatus{
		CurrentRevision: baselineRevName,
		Experiment: &v2alpha1.ExperimentStatus{
			Phase:            v2alpha1.ExperimentPhaseRunning,
			BaselineRevision: baselineRevName,
			ExpectedSpecHash: expHash, // Hash of FA's intended spec (NPM)
		},
	})

	baselineCR := makeControllerRevision(baselineRevName, testNamespace, dda, baselineSpec)
	c := buildFakeClient(s, dda, baselineCR)

	_, _, err := HandleExperimentLifecycle(context.TODO(), c, dda, s, now, DefaultExperimentTimeout)
	require.NoError(t, err)

	assert.Equal(t, v2alpha1.ExperimentPhaseAborted, dda.Status.Experiment.Phase,
		"experiment should abort — user edited spec before first reconcile")
}

func TestRevisionPointers_NoChange(t *testing.T) {
	s := testScheme()
	spec := specWithAPM(true)
	hash, _ := ComputeSpecHash(&spec)
	revName := RevisionName(testDDAName, hash)

	dda := newTestDDAWithStatus(spec, v2alpha1.DatadogAgentStatus{
		CurrentRevision:  revName,
		PreviousRevision: "old-rev",
	})

	cr := makeControllerRevision(revName, testNamespace, dda, spec)
	c := buildFakeClient(s, dda, cr)

	_, _, err := HandleExperimentLifecycle(context.TODO(), c, dda, s, time.Now(), DefaultExperimentTimeout)

	require.NoError(t, err)
	assert.Equal(t, revName, dda.Status.CurrentRevision, "currentRevision should not change")
	assert.Equal(t, "old-rev", dda.Status.PreviousRevision, "previousRevision should not change")
}
