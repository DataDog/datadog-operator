// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

// Integration-style tests for ControllerRevision management wired through the
// full DDA reconcile path. These complement the unit tests in revision_test.go
// which exercise ensureRevision/gcOldRevisions in isolation.
//
// Coverage goals:
//   - Feature flag gate: revisions are only created when CreateControllerRevisions=true
//   - Idempotency: repeated reconciles with the same spec do not accumulate revisions
//   - Spec change: a new revision is created; the old one is kept as previous
//   - Revert: reverting to an earlier spec reuses the same ControllerRevision name
//     (content-addressed) and bumps the Revision counter
//   - GC: only the two most recent revisions survive after multiple spec changes
//   - Annotation filtering: non-datadog annotations (e.g. kubectl management
//     annotations) do not cause revision churn
//   - UID scoping: revisions from a deleted and recreated DDA are not inherited
//
// NOTE: appsv1.ControllerRevision is registered via appsv1.AddToScheme which is
// called by k8s.io/client-go/kubernetes/scheme in its init(). TestScheme() uses
// that as its base, so no additional registration is needed here.

import (
	"context"
	"testing"
	"time"

	"k8s.io/utils/ptr"

	assert "github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	agenttestutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/testutils"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	testutils "github.com/DataDog/datadog-operator/pkg/testutils"
)

// -----------------------------------------------------------------------------
// Helpers
// -----------------------------------------------------------------------------

// newRevisionIntegrationReconciler builds a Reconciler with CreateControllerRevisions
// enabled, ready for multi-reconcile tests.
func newRevisionIntegrationReconciler(t *testing.T) (*Reconciler, client.Client) {
	t.Helper()
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := agenttestutils.TestScheme()

	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(s, corev1.EventSource{Component: "test"})

	// Load DDAI CRD — required for reconcileInstanceV3 (which is where
	// manageRevision is called) to check CRD existence before creating DDAIs.
	crd, err := getDDAICRDFromConfig(s)
	assert.NoError(t, err)

	c := fake.NewClientBuilder().
		WithScheme(s).
		WithObjects(crd).
		WithStatusSubresource(&v2alpha1.DatadogAgent{}).
		Build()

	r := &Reconciler{
		client:       c,
		scheme:       s,
		platformInfo: kubernetes.PlatformInfo{},
		recorder:     recorder,
		log:          logf.Log.WithName(t.Name()),
		forwarders:   dummyManager{},
		options: ReconcilerOptions{
			CreateControllerRevisions:   true,
			DatadogAgentInternalEnabled: true,
		},
	}
	r.initializeComponentRegistry()
	return r, c
}

// createAndReconcile creates a DDA in the fake client and runs one reconcile.
func createAndReconcile(t *testing.T, r *Reconciler, dda *v2alpha1.DatadogAgent) {
	t.Helper()
	assert.NoError(t, r.client.Create(context.TODO(), dda))
	_, err := r.Reconcile(context.TODO(), dda)
	assert.NoError(t, err)
}

// listOwnedRevisions returns all ControllerRevisions in ns owned by ddaUID.
func listOwnedRevisions(t *testing.T, c client.Client, ns string, ddaUID types.UID) []appsv1.ControllerRevision {
	t.Helper()
	all := &appsv1.ControllerRevisionList{}
	assert.NoError(t, c.List(context.TODO(), all, client.InNamespace(ns)))
	var owned []appsv1.ControllerRevision
	for _, rev := range all.Items {
		for _, ref := range rev.OwnerReferences {
			if ref.Controller != nil && *ref.Controller && ref.UID == ddaUID {
				owned = append(owned, rev)
				break
			}
		}
	}
	return owned
}

// baseDDA returns an initialized DDA (with credentials and finalizer) for use
// in tests. The UID is set explicitly to allow OwnerReference scoping assertions.
func baseDDA(ns, name string, uid types.UID) *v2alpha1.DatadogAgent {
	dda := testutils.NewDatadogAgent(ns, name, nil)
	dda.UID = uid
	return dda
}

// baseDDAWithSite returns an initialized DDA with a specific Datadog site set,
// preserving the required credentials so the reconciler does not reject it.
func baseDDAWithSite(ns, name string, uid types.UID, site string) *v2alpha1.DatadogAgent {
	dda := baseDDA(ns, name, uid)
	dda.Spec.Global.Site = ptr.To(site)
	return dda
}

// -----------------------------------------------------------------------------
// Single-reconcile test via testCase harness
// -----------------------------------------------------------------------------

// Test_ControllerRevisions_FlagDisabled verifies that no ControllerRevisions are
// created when CreateControllerRevisions is false (the default).
//
// Uses the existing testCase harness so it exercises both the DDA-only and the
// full DDA+DDAI reconcile paths automatically.
func Test_ControllerRevisions_FlagDisabled(t *testing.T) {
	const ns, name = "default", "test-dda"

	tests := []testCase{
		{
			name: "CreateControllerRevisions=false: no revisions created",
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				dda := testutils.NewDatadogAgent(ns, name, nil)
				_ = c.Create(context.TODO(), dda)
				return dda
			},
			want:    reconcile.Result{RequeueAfter: 15 * time.Second},
			wantErr: false,
			wantFunc: func(t *testing.T, c client.Client) {
				revList := &appsv1.ControllerRevisionList{}
				assert.NoError(t, c.List(context.TODO(), revList, client.InNamespace(ns)))
				assert.Empty(t, revList.Items, "expected no ControllerRevisions when flag is disabled")
			},
		},
	}

	// runTestCases uses ReconcilerOptions without CreateControllerRevisions, which
	// defaults to false — exactly what we want to assert against.
	runTestCases(t, tests, runDDAReconcilerTest)
}

// -----------------------------------------------------------------------------
// Multi-reconcile tests
// -----------------------------------------------------------------------------

// Test_ControllerRevisions_FirstReconcile verifies that the first reconcile
// creates exactly one ControllerRevision with Revision=1, owned by the DDA.
func Test_ControllerRevisions_FirstReconcile(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")

	r, c := newRevisionIntegrationReconciler(t)
	createAndReconcile(t, r, baseDDA(ns, name, uid))

	revs := listOwnedRevisions(t, c, ns, uid)
	assert.Len(t, revs, 1)
	assert.Equal(t, int64(1), revs[0].Revision)
	assert.Len(t, revs[0].OwnerReferences, 1)
	assert.Equal(t, name, revs[0].OwnerReferences[0].Name)
}

// Test_ControllerRevisions_Idempotent verifies that reconciling the same spec
// twice does not create a second ControllerRevision.
func Test_ControllerRevisions_Idempotent(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")

	r, c := newRevisionIntegrationReconciler(t)
	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	firstName := listOwnedRevisions(t, c, ns, uid)[0].Name

	// Second reconcile — same spec.
	_, err := r.Reconcile(context.TODO(), dda)
	assert.NoError(t, err)

	revs := listOwnedRevisions(t, c, ns, uid)
	assert.Len(t, revs, 1, "second reconcile must not create a new revision")
	assert.Equal(t, firstName, revs[0].Name, "revision name must be stable")
}

// Test_ControllerRevisions_SpecChange verifies that a spec change on the second
// reconcile creates a new ControllerRevision while keeping the previous one.
func Test_ControllerRevisions_SpecChange(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")

	r, c := newRevisionIntegrationReconciler(t)
	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	firstName := listOwnedRevisions(t, c, ns, uid)[0].Name

	// Re-fetch to get current resourceVersion, then change a spec field.
	assert.NoError(t, c.Get(context.TODO(), types.NamespacedName{Namespace: ns, Name: name}, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, c.Update(context.TODO(), dda))
	_, err := r.Reconcile(context.TODO(), dda)
	assert.NoError(t, err)

	revs := listOwnedRevisions(t, c, ns, uid)
	assert.Len(t, revs, 2, "expected two revisions after spec change")

	names := map[string]bool{}
	for _, rev := range revs {
		names[rev.Name] = true
	}
	assert.True(t, names[firstName], "first revision must be kept as previous")
}

// Test_ControllerRevisions_Revert verifies that reverting to an earlier spec
// reuses the same ControllerRevision name (content-addressed) and bumps the
// Revision counter to maintain monotonic ordering.
func Test_ControllerRevisions_Revert(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")

	r, c := newRevisionIntegrationReconciler(t)
	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	firstName := listOwnedRevisions(t, c, ns, uid)[0].Name

	// Change spec.
	assert.NoError(t, c.Get(context.TODO(), types.NamespacedName{Namespace: ns, Name: name}, dda))
	dda.Spec.Global.Site = ptr.To("datadoghq.eu")
	assert.NoError(t, c.Update(context.TODO(), dda))
	_, err := r.Reconcile(context.TODO(), dda)
	assert.NoError(t, err)

	// Revert: clear the site back to nil.
	assert.NoError(t, c.Get(context.TODO(), types.NamespacedName{Namespace: ns, Name: name}, dda))
	dda.Spec.Global.Site = nil
	assert.NoError(t, c.Update(context.TODO(), dda))
	_, err = r.Reconcile(context.TODO(), dda)
	assert.NoError(t, err)

	revs := listOwnedRevisions(t, c, ns, uid)

	// Find the revision with the original name.
	var revertedRev *appsv1.ControllerRevision
	for i := range revs {
		if revs[i].Name == firstName {
			revertedRev = &revs[i]
		}
	}
	assert.NotNil(t, revertedRev, "original revision must still exist after revert")
	assert.Equal(t, int64(3), revertedRev.Revision,
		"reverted revision's counter must be bumped to max+1 to preserve ordering")
}

// Test_ControllerRevisions_GC verifies that after four distinct spec changes
// only the two most recent revisions survive.
func Test_ControllerRevisions_GC(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")

	r, c := newRevisionIntegrationReconciler(t)

	sites := []string{"us1", "eu1", "ap1", "gov1"}
	var lastRevName string

	dda := baseDDAWithSite(ns, name, uid, sites[0])
	createAndReconcile(t, r, dda)

	for i, site := range sites[1:] {
		assert.NoError(t, c.Get(context.TODO(), types.NamespacedName{Namespace: ns, Name: name}, dda))
		dda.Spec.Global.Site = ptr.To(site)
		assert.NoError(t, c.Update(context.TODO(), dda))
		_, err := r.Reconcile(context.TODO(), dda)
		assert.NoError(t, err)

		if i == len(sites)-2 {
			// Last iteration: find the revision with the highest counter.
			revs := listOwnedRevisions(t, c, ns, uid)
			maxRev := int64(0)
			for _, rev := range revs {
				if rev.Revision > maxRev {
					maxRev = rev.Revision
					lastRevName = rev.Name
				}
			}
		}
	}

	revs := listOwnedRevisions(t, c, ns, uid)
	assert.Len(t, revs, 2, "GC must keep only current and previous")

	surviving := map[string]bool{}
	for _, rev := range revs {
		surviving[rev.Name] = true
	}
	assert.True(t, surviving[lastRevName], "most recent revision must survive")
}

// Test_ControllerRevisions_AnnotationFiltering verifies that non-datadog
// annotations (e.g. kubectl.kubernetes.io/last-applied-configuration) do not
// cause a new revision to be created, because they are filtered before snapshotting.
func Test_ControllerRevisions_AnnotationFiltering(t *testing.T) {
	const ns, name = "default", "test-dda"
	const uid = types.UID("uid-1")

	r, c := newRevisionIntegrationReconciler(t)
	dda := baseDDA(ns, name, uid)
	createAndReconcile(t, r, dda)

	firstName := listOwnedRevisions(t, c, ns, uid)[0].Name

	// Add a kubectl management annotation — this simulates what `kubectl apply` does.
	assert.NoError(t, c.Get(context.TODO(), types.NamespacedName{Namespace: ns, Name: name}, dda))
	dda.Annotations = map[string]string{
		"kubectl.kubernetes.io/last-applied-configuration": `{"apiVersion":"datadoghq.com/v2alpha1"}`,
	}
	assert.NoError(t, c.Update(context.TODO(), dda))
	_, err := r.Reconcile(context.TODO(), dda)
	assert.NoError(t, err)

	revs := listOwnedRevisions(t, c, ns, uid)
	assert.Len(t, revs, 1, "non-datadog annotation change must not create a new revision")
	assert.Equal(t, firstName, revs[0].Name)
}

// Test_ControllerRevisions_UIDScoping verifies that if a DDA is deleted and
// recreated with the same name (new UID), the new instance does not inherit
// revisions from the old one.
func Test_ControllerRevisions_UIDScoping(t *testing.T) {
	const ns, name = "default", "test-dda"

	r, c := newRevisionIntegrationReconciler(t)

	// First DDA lifecycle.
	ddaV1 := baseDDA(ns, name, "uid-old")
	createAndReconcile(t, r, ddaV1)
	assert.Len(t, listOwnedRevisions(t, c, ns, "uid-old"), 1)

	// Simulate deletion: re-fetch, remove the finalizer (so the fake client
	// actually removes the object), then delete. ControllerRevision orphaning
	// is not enforced by the fake client so the old revision stays, unowned.
	assert.NoError(t, r.client.Get(context.TODO(), types.NamespacedName{Namespace: ns, Name: name}, ddaV1))
	ddaV1.Finalizers = nil
	assert.NoError(t, r.client.Update(context.TODO(), ddaV1))
	assert.NoError(t, r.client.Delete(context.TODO(), ddaV1))

	// New DDA with same name but different UID and a different spec (as would
	// happen after deletion and re-creation with updated config).
	ddaV2 := baseDDAWithSite(ns, name, "uid-new", "datadoghq.eu")
	createAndReconcile(t, r, ddaV2)

	// New instance should have exactly one revision scoped to its own UID.
	assert.Len(t, listOwnedRevisions(t, c, ns, "uid-new"), 1,
		"new DDA instance must start with a fresh revision history")
	assert.Len(t, listOwnedRevisions(t, c, ns, "uid-old"), 1,
		"old revision is still present (orphaned); not mistaken for new owner's history")
}
