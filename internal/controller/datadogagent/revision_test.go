// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"testing"

	"k8s.io/utils/ptr"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

func newRevisionTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, appsv1.AddToScheme(s))
	require.NoError(t, v2alpha1.AddToScheme(s))
	return s
}

func newRevisionTestOwner(name, namespace string) *v2alpha1.DatadogAgent {
	return &v2alpha1.DatadogAgent{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "datadoghq.com/v2alpha1",
			Kind:       "DatadogAgent",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			UID:        types.UID("test-uid-1234"),
			Generation: 3,
		},
	}
}

func newRevisionTestReconciler(t *testing.T) (*Reconciler, client.Client) {
	t.Helper()
	scheme := newRevisionTestScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	return &Reconciler{client: c, scheme: scheme}, c
}

func mustListRevisions(t *testing.T, r *Reconciler, instance *v2alpha1.DatadogAgent) []appsv1.ControllerRevision {
	t.Helper()
	revisions, err := r.listRevisions(context.Background(), instance)
	require.NoError(t, err)
	return revisions
}

func fetchRevisionByName(t *testing.T, c client.Client, namespace, name string) *appsv1.ControllerRevision {
	t.Helper()
	rev := &appsv1.ControllerRevision{}
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: name}, rev))
	return rev
}

func TestEnsureRevision_CreatesOnFirstCall(t *testing.T) {
	r, c := newRevisionTestReconciler(t)
	instance := newRevisionTestOwner("test-dda", "default")

	name, err := r.ensureRevision(context.Background(), instance, mustListRevisions(t, r, instance), false)
	require.NoError(t, err)
	assert.NotEmpty(t, name)

	rev := fetchRevisionByName(t, c, "default", name)
	assert.NotEmpty(t, rev.Data.Raw)
	assert.Equal(t, int64(1), rev.Revision)
	assert.Len(t, rev.OwnerReferences, 1)
	assert.Equal(t, "test-dda", rev.OwnerReferences[0].Name)
}

func TestEnsureRevision_Idempotent(t *testing.T) {
	r, c := newRevisionTestReconciler(t)
	instance := newRevisionTestOwner("test-dda", "default")
	instance.Annotations = map[string]string{"foo": "bar"}

	name1, err := r.ensureRevision(context.Background(), instance, mustListRevisions(t, r, instance), false)
	require.NoError(t, err)
	name2, err := r.ensureRevision(context.Background(), instance, mustListRevisions(t, r, instance), false)
	require.NoError(t, err)

	assert.Equal(t, name1, name2)

	revList := &appsv1.ControllerRevisionList{}
	require.NoError(t, c.List(context.Background(), revList))
	assert.Len(t, revList.Items, 1)
}

func TestEnsureRevision_DifferentSpecsDifferentNames(t *testing.T) {
	r, _ := newRevisionTestReconciler(t)

	instanceA := newRevisionTestOwner("test-dda", "default")
	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}

	name1, err := r.ensureRevision(context.Background(), instanceA, mustListRevisions(t, r, instanceA), false)
	require.NoError(t, err)
	name2, err := r.ensureRevision(context.Background(), instanceB, mustListRevisions(t, r, instanceB), false)
	require.NoError(t, err)

	assert.NotEqual(t, name1, name2)
}

func TestEnsureRevision_DifferentAnnotationsDifferentNames(t *testing.T) {
	r, _ := newRevisionTestReconciler(t)

	instanceA := newRevisionTestOwner("test-dda", "default")
	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Annotations = map[string]string{"feature.datadoghq.com/beta": "true"}

	name1, err := r.ensureRevision(context.Background(), instanceA, mustListRevisions(t, r, instanceA), false)
	require.NoError(t, err)
	name2, err := r.ensureRevision(context.Background(), instanceB, mustListRevisions(t, r, instanceB), false)
	require.NoError(t, err)

	assert.NotEqual(t, name1, name2)
}

// TestEnsureRevision_NonDatadogAnnotationsIgnored verifies that annotations
// outside the .datadoghq.com/ domain are not included in the snapshot, so
// they don't produce distinct revisions.
func TestEnsureRevision_NonDatadogAnnotationsIgnored(t *testing.T) {
	r, _ := newRevisionTestReconciler(t)

	instanceA := newRevisionTestOwner("test-dda", "default")
	instanceB := newRevisionTestOwner("test-dda", "default")
	// kubectl and other tooling annotations should be invisible to the snapshot.
	instanceB.Annotations = map[string]string{
		"kubectl.kubernetes.io/last-applied-configuration": `{"apiVersion":"datadoghq.com/v2alpha1"}`,
		"some-other-tool/annotation":                       "value",
	}

	name1, err := r.ensureRevision(context.Background(), instanceA, mustListRevisions(t, r, instanceA), false)
	require.NoError(t, err)
	name2, err := r.ensureRevision(context.Background(), instanceB, mustListRevisions(t, r, instanceB), false)
	require.NoError(t, err)

	assert.Equal(t, name1, name2, "non-datadoghq annotations should not affect the revision snapshot")
}

func TestGCOldRevisions_KeepsCurrentAndPrevious(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	instances := []*v2alpha1.DatadogAgent{
		newRevisionTestOwner("test-dda", "default"),
		func() *v2alpha1.DatadogAgent {
			i := newRevisionTestOwner("test-dda", "default")
			i.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
			return i
		}(),
		func() *v2alpha1.DatadogAgent {
			i := newRevisionTestOwner("test-dda", "default")
			i.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{Site: ptr.To("datadoghq.eu")}}
			return i
		}(),
	}
	names := make([]string, len(instances))
	for i, inst := range instances {
		name, err := r.ensureRevision(context.Background(), inst, mustListRevisions(t, r, inst), false)
		require.NoError(t, err)
		names[i] = name
	}

	err := r.gcOldRevisions(context.Background(), instances[2], names[2], mustListRevisions(t, r, instances[2]))
	require.NoError(t, err)

	revList := &appsv1.ControllerRevisionList{}
	require.NoError(t, c.List(context.Background(), revList))
	assert.Len(t, revList.Items, 2)

	remaining := map[string]bool{}
	for _, rev := range revList.Items {
		remaining[rev.Name] = true
	}
	assert.True(t, remaining[names[2]], "current should be kept")
	assert.True(t, remaining[names[1]], "previous should be kept")
	assert.False(t, remaining[names[0]], "old revision should be deleted")
}

func TestEnsureRevision_RevertBumpsRevision(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	instanceA := newRevisionTestOwner("test-dda", "default")
	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}

	name1, err := r.ensureRevision(context.Background(), instanceA, mustListRevisions(t, r, instanceA), false)
	require.NoError(t, err)
	_, err = r.ensureRevision(context.Background(), instanceB, mustListRevisions(t, r, instanceB), false)
	require.NoError(t, err)

	// Revert to spec A — should reuse name1 but bump its Revision.
	name3, err := r.ensureRevision(context.Background(), instanceA, mustListRevisions(t, r, instanceA), false)
	require.NoError(t, err)
	assert.Equal(t, name1, name3, "revert should reuse same CR name")

	rev := fetchRevisionByName(t, c, "default", name1)
	assert.Equal(t, int64(3), rev.Revision, "revision should be bumped to max+1")
}

func TestGCOldRevisions_KeepsTwoRevisions(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	instanceA := newRevisionTestOwner("test-dda", "default")
	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}

	_, err := r.ensureRevision(context.Background(), instanceA, mustListRevisions(t, r, instanceA), false)
	require.NoError(t, err)
	name2, err := r.ensureRevision(context.Background(), instanceB, mustListRevisions(t, r, instanceB), false)
	require.NoError(t, err)

	err = r.gcOldRevisions(context.Background(), instanceB, name2, mustListRevisions(t, r, instanceB))
	require.NoError(t, err)

	revList := &appsv1.ControllerRevisionList{}
	require.NoError(t, c.List(context.Background(), revList))
	assert.Len(t, revList.Items, 2)
}

func TestEnsureRevision_RevisionNumbersMonotonic(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	instances := []*v2alpha1.DatadogAgent{
		newRevisionTestOwner("test-dda", "default"),
		func() *v2alpha1.DatadogAgent {
			i := newRevisionTestOwner("test-dda", "default")
			i.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}
			return i
		}(),
		func() *v2alpha1.DatadogAgent {
			i := newRevisionTestOwner("test-dda", "default")
			i.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{Site: ptr.To("datadoghq.eu")}}
			return i
		}(),
	}

	names := make([]string, len(instances))
	for i, inst := range instances {
		name, err := r.ensureRevision(context.Background(), inst, mustListRevisions(t, r, inst), false)
		require.NoError(t, err)
		names[i] = name
	}

	for i, name := range names {
		rev := fetchRevisionByName(t, c, "default", name)
		assert.Equal(t, int64(i+1), rev.Revision, "revision %d should be %d", i, i+1)
	}
}

func TestGCOldRevisions_NoPreviousWhenOnlyCurrent(t *testing.T) {
	r, c := newRevisionTestReconciler(t)
	instance := newRevisionTestOwner("test-dda", "default")

	revName, err := r.ensureRevision(context.Background(), instance, mustListRevisions(t, r, instance), false)
	require.NoError(t, err)

	err = r.gcOldRevisions(context.Background(), instance, revName, mustListRevisions(t, r, instance))
	require.NoError(t, err)

	revList := &appsv1.ControllerRevisionList{}
	require.NoError(t, c.List(context.Background(), revList))
	assert.Len(t, revList.Items, 1, "current should not be deleted")
}

func TestGCOldRevisions_DeletesMultipleOld(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Create 5 distinct revisions.
	sites := []string{"us", "eu", "ap1", "ap2", "gov"}
	names := make([]string, len(sites))
	for i, site := range sites {
		inst := newRevisionTestOwner("test-dda", "default")
		inst.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{Site: ptr.To(site)}}
		name, err := r.ensureRevision(context.Background(), inst, mustListRevisions(t, r, inst), false)
		require.NoError(t, err)
		names[i] = name
	}

	current := newRevisionTestOwner("test-dda", "default")
	err := r.gcOldRevisions(context.Background(), current, names[4], mustListRevisions(t, r, current))
	require.NoError(t, err)

	revList := &appsv1.ControllerRevisionList{}
	require.NoError(t, c.List(context.Background(), revList))
	assert.Len(t, revList.Items, 2, "only current and previous should remain")

	remaining := map[string]bool{}
	for _, rev := range revList.Items {
		remaining[rev.Name] = true
	}
	assert.True(t, remaining[names[4]], "current should be kept")
	assert.True(t, remaining[names[3]], "previous should be kept")
	for _, old := range names[:3] {
		assert.False(t, remaining[old], "old revision %s should be deleted", old)
	}
}

func TestManageRevision_CreatesRevision(t *testing.T) {
	r, _ := newRevisionTestReconciler(t)

	instanceA := newRevisionTestOwner("test-dda", "default")
	err := r.manageRevision(context.Background(), instanceA, mustListRevisions(t, r, instanceA), nil)
	require.NoError(t, err)

	revList := mustListRevisions(t, r, instanceA)
	assert.Len(t, revList, 1, "one revision after first call")

	// Change spec and call again — should now have current and previous.
	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}

	err = r.manageRevision(context.Background(), instanceB, mustListRevisions(t, r, instanceB), nil)
	require.NoError(t, err)

	revList = mustListRevisions(t, r, instanceB)
	assert.Len(t, revList, 2, "current and previous kept after spec change")
}

func TestManageRevision_Idempotent(t *testing.T) {
	r, _ := newRevisionTestReconciler(t)
	instance := newRevisionTestOwner("test-dda", "default")

	err := r.manageRevision(context.Background(), instance, mustListRevisions(t, r, instance), nil)
	require.NoError(t, err)
	err = r.manageRevision(context.Background(), instance, mustListRevisions(t, r, instance), nil)
	require.NoError(t, err)

	revList := mustListRevisions(t, r, instance)
	assert.Len(t, revList, 1, "idempotent calls should not create duplicate revisions")
}

// TestManageRevision_CurrentDeletedIsRecreated verifies that if the current
// ControllerRevision is manually deleted, the next manageRevision call
// re-creates it at the next revision number.
func TestManageRevision_CurrentDeletedIsRecreated(t *testing.T) {
	r, c := newRevisionTestReconciler(t)
	instance := newRevisionTestOwner("test-dda", "default")

	err := r.manageRevision(context.Background(), instance, mustListRevisions(t, r, instance), nil)
	require.NoError(t, err)

	revList := mustListRevisions(t, r, instance)
	require.Len(t, revList, 1)
	deletedName := revList[0].Name

	// Simulate manual deletion.
	require.NoError(t, c.Delete(context.Background(), &revList[0]))

	// Next reconcile should re-create the revision.
	err = r.manageRevision(context.Background(), instance, mustListRevisions(t, r, instance), nil)
	require.NoError(t, err)

	revList = mustListRevisions(t, r, instance)
	require.Len(t, revList, 1)

	// Content is identical so the name (hash-based) should be the same.
	assert.Equal(t, deletedName, revList[0].Name, "re-created revision should have the same name")
	assert.Equal(t, int64(1), revList[0].Revision)
}

// TestManageRevision_PreviousDeletedContinuesNormally verifies that if the
// previous ControllerRevision is manually deleted, the next manageRevision
// call succeeds and the current revision is preserved.
func TestManageRevision_PreviousDeletedContinuesNormally(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	instanceA := newRevisionTestOwner("test-dda", "default")
	instanceB := newRevisionTestOwner("test-dda", "default")
	instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}

	err := r.manageRevision(context.Background(), instanceA, mustListRevisions(t, r, instanceA), nil)
	require.NoError(t, err)
	err = r.manageRevision(context.Background(), instanceB, mustListRevisions(t, r, instanceB), nil)
	require.NoError(t, err)

	revList := mustListRevisions(t, r, instanceB)
	require.Len(t, revList, 2)

	// Identify and delete the previous (lower Revision number).
	var prev appsv1.ControllerRevision
	for _, rev := range revList {
		if prev.Name == "" || rev.Revision < prev.Revision {
			prev = rev
		}
	}
	require.NoError(t, c.Delete(context.Background(), &prev))

	// Next reconcile on the same spec should succeed and keep only the current.
	err = r.manageRevision(context.Background(), instanceB, mustListRevisions(t, r, instanceB), nil)
	require.NoError(t, err)

	revList = mustListRevisions(t, r, instanceB)
	assert.Len(t, revList, 1, "only current should remain after previous was deleted")
	assert.Equal(t, int64(2), revList[0].Revision)
}

// TestGCOldRevisions_DeletesPreviousAfterRejectedExperiment verifies that when
// the persisted experiment phase is a rejected terminal phase (Rollback, Timeout,
// or Aborted), gcOldRevisions deletes the stale experiment revision instead of
// keeping it as "previous". This prevents an immediate timeout if the same spec
// is re-applied as a new experiment.
func TestGCOldRevisions_DeletesPreviousAfterRejectedExperiment(t *testing.T) {
	rejectedPhases := []v2alpha1.ExperimentPhase{
		v2alpha1.ExperimentPhaseRollback,
		v2alpha1.ExperimentPhaseTimeout,
		v2alpha1.ExperimentPhaseAborted,
	}

	for _, phase := range rejectedPhases {
		t.Run(string(phase), func(t *testing.T) {
			r, c := newRevisionTestReconciler(t)

			instanceA := newRevisionTestOwner("test-dda", "default")
			instanceB := newRevisionTestOwner("test-dda", "default")
			instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}

			nameA, err := r.ensureRevision(context.Background(), instanceA, mustListRevisions(t, r, instanceA), false)
			require.NoError(t, err)
			_, err = r.ensureRevision(context.Background(), instanceB, mustListRevisions(t, r, instanceB), false)
			require.NoError(t, err)

			// Simulate the persisted status having a rejected terminal phase.
			// current is instanceA (spec restored after rollback).
			instanceA.Status.Experiment = &v2alpha1.ExperimentStatus{Phase: phase}

			err = r.gcOldRevisions(context.Background(), instanceA, nameA, mustListRevisions(t, r, instanceA))
			require.NoError(t, err)

			revList := &appsv1.ControllerRevisionList{}
			require.NoError(t, c.List(context.Background(), revList))
			assert.Len(t, revList.Items, 1, "experiment revision should be deleted after rejected phase")
			assert.Equal(t, nameA, revList.Items[0].Name, "only the current (pre-experiment) revision should remain")
		})
	}
}

// TestGCOldRevisions_KeepsPreviousForNonRejectedPhases verifies that for
// non-rejected experiment phases (Running, Promoted, or nil), the normal
// "keep current + previous" behavior is preserved.
func TestGCOldRevisions_KeepsPreviousForNonRejectedPhases(t *testing.T) {
	nonRejectedPhases := []v2alpha1.ExperimentPhase{
		v2alpha1.ExperimentPhaseRunning,
		v2alpha1.ExperimentPhasePromoted,
		"", // nil experiment
	}

	for _, phase := range nonRejectedPhases {
		t.Run(string(phase)+"_or_nil", func(t *testing.T) {
			r, c := newRevisionTestReconciler(t)

			instanceA := newRevisionTestOwner("test-dda", "default")
			instanceB := newRevisionTestOwner("test-dda", "default")
			instanceB.Spec = v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{}}

			_, err := r.ensureRevision(context.Background(), instanceA, mustListRevisions(t, r, instanceA), false)
			require.NoError(t, err)
			nameB, err := r.ensureRevision(context.Background(), instanceB, mustListRevisions(t, r, instanceB), false)
			require.NoError(t, err)

			if phase != "" {
				instanceB.Status.Experiment = &v2alpha1.ExperimentStatus{Phase: phase}
			}

			err = r.gcOldRevisions(context.Background(), instanceB, nameB, mustListRevisions(t, r, instanceB))
			require.NoError(t, err)

			revList := &appsv1.ControllerRevisionList{}
			require.NoError(t, c.List(context.Background(), revList))
			assert.Len(t, revList.Items, 2, "current and previous should both be kept for non-rejected phase")
		})
	}
}

// TestListRevisions_ExcludesForeignOwner verifies that a ControllerRevision
// sharing the same name label but owned by a different DDA UID (e.g. a
// deleted-and-recreated DDA) is not returned by listRevisions.
func TestListRevisions_ExcludesForeignOwner(t *testing.T) {
	r, c := newRevisionTestReconciler(t)

	// Current instance with UID "new-uid".
	current := newRevisionTestOwner("test-dda", "default")
	current.UID = "new-uid"

	// A revision left over from a previous DDA with the same name but UID "old-uid".
	isController := true
	foreign := &appsv1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dda-oldrev",
			Namespace: "default",
			Labels:    map[string]string{"agent.datadoghq.com/datadogagent": "test-dda"},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "datadoghq.com/v2alpha1",
					Kind:       "DatadogAgent",
					Name:       "test-dda",
					UID:        "old-uid",
					Controller: &isController,
				},
			},
		},
		Revision: 1,
	}
	require.NoError(t, c.Create(context.Background(), foreign))

	revList, err := r.listRevisions(context.Background(), current)
	require.NoError(t, err)
	assert.Empty(t, revList, "foreign revision should be excluded by UID filter")
}
