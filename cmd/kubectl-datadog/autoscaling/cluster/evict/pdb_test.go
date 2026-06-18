package evict

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newCtrlScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	sch := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(sch))
	return sch
}

func TestTempPDBName(t *testing.T) {
	for _, tc := range []struct {
		kind   string
		name   string
		expect string
	}{
		{"Deployment", "my-app", "deployment-my-app-evict-legacy"},
		{"StatefulSet", "db", "statefulset-db-evict-legacy"},
		// Long name that must be truncated to fit 63 chars
		{"Deployment", strings.Repeat("a", 80), "deployment-" + strings.Repeat("a", 63-len("deployment-")-len(pdbNameSuffix)) + pdbNameSuffix},
	} {
		got := tempPDBName(tc.kind, tc.name)
		assert.LessOrEqual(t, len(got), 63, "tempPDBName output exceeds 63 chars: %s", got)
		assert.Equal(t, tc.expect, got)
	}
}

func TestIsTemporaryPDB(t *testing.T) {
	for _, tc := range []struct {
		name   string
		labels map[string]string
		want   bool
	}{
		{
			name: "both labels present",
			labels: map[string]string{
				pdbManagedByLabelKey: pdbManagedByLabelValue,
				pdbTempLabelKey:      pdbTempLabelValue,
			},
			want: true,
		},
		{name: "no labels", labels: map[string]string{"app": "x"}, want: false},
		{
			name:   "only managed-by label, must require BOTH",
			labels: map[string]string{pdbManagedByLabelKey: pdbManagedByLabelValue},
			want:   false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pdb := &policyv1.PodDisruptionBudget{ObjectMeta: metav1.ObjectMeta{Labels: tc.labels}}
			assert.Equal(t, tc.want, isTemporaryPDB(pdb))
		})
	}
}

func TestHasUserPDB(t *testing.T) {
	matchingSelector := &metav1.LabelSelector{MatchLabels: map[string]string{"app": "x"}}
	otherSelector := &metav1.LabelSelector{MatchLabels: map[string]string{"app": "y"}}
	userPDB := policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "user", Namespace: "default"},
		Spec:       policyv1.PodDisruptionBudgetSpec{Selector: matchingSelector},
	}
	tempPDB := policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "temp", Namespace: "default", Labels: map[string]string{
			pdbManagedByLabelKey: pdbManagedByLabelValue,
			pdbTempLabelKey:      pdbTempLabelValue,
		}},
		Spec: policyv1.PodDisruptionBudgetSpec{Selector: matchingSelector},
	}

	for _, tc := range []struct {
		name               string
		existing           []policyv1.PodDisruptionBudget
		controllerSelector *metav1.LabelSelector
		want               bool
	}{
		{name: "matching user PDB", existing: []policyv1.PodDisruptionBudget{userPDB}, controllerSelector: matchingSelector, want: true},
		{name: "temp PDB ignored", existing: []policyv1.PodDisruptionBudget{tempPDB}, controllerSelector: matchingSelector, want: false},
		{name: "nil controller selector", existing: []policyv1.PodDisruptionBudget{userPDB}, controllerSelector: nil, want: false},
		{name: "different selector", existing: []policyv1.PodDisruptionBudget{userPDB}, controllerSelector: otherSelector, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, hasUserPDB(tc.existing, tc.controllerSelector))
		})
	}
}

func TestCleanupTempPDBs(t *testing.T) {
	tempPDB := func() *policyv1.PodDisruptionBudget {
		return &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name: "temp", Namespace: "default",
				Labels: map[string]string{
					pdbManagedByLabelKey: pdbManagedByLabelValue,
					pdbTempLabelKey:      pdbTempLabelValue,
				},
			},
		}
	}
	userPDB := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "user", Namespace: "default"},
	}
	otherManaged := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name: "other", Namespace: "kube-system",
			Labels: map[string]string{pdbManagedByLabelKey: "some-other-tool"},
		},
	}

	for _, tc := range []struct {
		name string
		// existing seeds the controller-runtime fake client.
		existing []ctrlclient.Object
		dryRun   bool
		// wantRemaining is the expected `<namespace>/<name>` set after cleanup.
		wantRemaining []string
	}{
		{
			// Only PDBs carrying BOTH temp labels are removed; user PDBs
			// and PDBs from other tools stay put.
			name:          "deletes only fully-labelled temp PDBs",
			existing:      []ctrlclient.Object{tempPDB(), userPDB, otherManaged},
			wantRemaining: []string{"default/user", "kube-system/other"},
		},
		{
			name:          "no temp PDBs is a no-op",
			existing:      []ctrlclient.Object{userPDB},
			wantRemaining: []string{"default/user"},
		},
		{
			name:          "dry-run does not delete",
			existing:      []ctrlclient.Object{tempPDB()},
			dryRun:        true,
			wantRemaining: []string{"default/temp"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cli := ctrlfake.NewClientBuilder().WithScheme(newCtrlScheme(t)).WithObjects(tc.existing...).Build()

			require.NoError(t, cleanupTempPDBs(t.Context(), cli, tc.dryRun))

			list := &policyv1.PodDisruptionBudgetList{}
			require.NoError(t, cli.List(t.Context(), list))
			names := make([]string, 0, len(list.Items))
			for _, p := range list.Items {
				names = append(names, p.Namespace+"/"+p.Name)
			}
			assert.ElementsMatch(t, tc.wantRemaining, names)
		})
	}
}

func TestCreateTempPDB(t *testing.T) {
	appController := controllerInfo{
		controllerKey: controllerKey{Namespace: "default", Kind: "Deployment", Name: "app"},
		Selector:      &metav1.LabelSelector{MatchLabels: map[string]string{"app": "x"}},
	}
	existingTempPDB := func() *policyv1.PodDisruptionBudget {
		return &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name: tempPDBName("Deployment", "app"), Namespace: "default",
				Labels: map[string]string{
					pdbManagedByLabelKey: pdbManagedByLabelValue,
					pdbTempLabelKey:      pdbTempLabelValue,
				},
			},
			Spec: policyv1.PodDisruptionBudgetSpec{
				MaxUnavailable: ptr.To(intstr.FromInt(1)),
				Selector:       &metav1.LabelSelector{MatchLabels: map[string]string{"app": "x"}},
			},
		}
	}
	userPDBAtSameName := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name: tempPDBName("Deployment", "app"), Namespace: "default",
			// no temp-pdb labels: looks like a user PDB that happens to share
			// our naming.
		},
	}

	// ownership describes the expected ownership of the surviving PDB.
	type ownership int
	const (
		expectNoPDB   ownership = iota // 0 PDBs in cluster
		expectOurs                     // 1 PDB, with our temp labels + MaxUnavailable=1
		expectUserPDB                  // 1 PDB, NOT carrying our labels
		expectAny                      // 1 PDB, ownership irrelevant (idempotent case)
	)

	for _, tc := range []struct {
		name string
		// existing seeds the controller-runtime fake client.
		existing []ctrlclient.Object
		dryRun   bool
		// expect describes the expected post-create cluster state.
		expect ownership
		// runCleanupAfter, when true, runs cleanupTempPDBs after createTempPDB
		// and asserts the cluster ends up empty — exercises the crash-recovery
		// loop where a previous run left a temp PDB behind.
		runCleanupAfter bool
	}{
		{
			name:   "creates PDB when missing",
			expect: expectOurs,
		},
		{
			// Crash-recovery scenario: a previous run created a temp PDB
			// and was killed. A fresh ensure must NOT duplicate it, and
			// cleanup must still remove it.
			name:            "idempotent on pre-existing temp PDB",
			existing:        []ctrlclient.Object{existingTempPDB()},
			expect:          expectAny,
			runCleanupAfter: true,
		},
		{
			// User-owned PDB at the same name (no temp labels) must be
			// left untouched.
			name:     "name collision with user PDB leaves it untouched",
			existing: []ctrlclient.Object{userPDBAtSameName},
			expect:   expectUserPDB,
		},
		{
			name:   "dry-run does not create",
			dryRun: true,
			expect: expectNoPDB,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cli := ctrlfake.NewClientBuilder().WithScheme(newCtrlScheme(t)).WithObjects(tc.existing...).Build()

			require.NoError(t, createTempPDB(t.Context(), cli, appController, tc.dryRun))

			list := &policyv1.PodDisruptionBudgetList{}
			require.NoError(t, cli.List(t.Context(), list))
			switch tc.expect {
			case expectNoPDB:
				assert.Empty(t, list.Items)
			case expectOurs:
				require.Len(t, list.Items, 1)
				pdb := list.Items[0]
				assert.Equal(t, pdbManagedByLabelValue, pdb.Labels[pdbManagedByLabelKey])
				assert.Equal(t, pdbTempLabelValue, pdb.Labels[pdbTempLabelKey])
				require.NotNil(t, pdb.Spec.MaxUnavailable)
				assert.Equal(t, int32(1), pdb.Spec.MaxUnavailable.IntVal)
			case expectUserPDB:
				require.Len(t, list.Items, 1)
				assert.NotEqual(t, pdbManagedByLabelValue, list.Items[0].Labels[pdbManagedByLabelKey])
			case expectAny:
				require.Len(t, list.Items, 1)
			}

			if tc.runCleanupAfter {
				require.NoError(t, cleanupTempPDBs(t.Context(), cli, false))
				require.NoError(t, cli.List(t.Context(), list))
				assert.Empty(t, list.Items, "cleanup must remove pre-existing temp PDB even with no state from ensure")
			}
		})
	}
}

// TestDiscoverControllers_FiltersByNodeSet locks in the cluster-wide-list +
// client-side filter shape of discoverControllers: pods on target nodes
// resolve to their top-level controllers, pods elsewhere are ignored, and
// duplicate controllers (multiple pods of one Deployment) collapse to a
// single entry.
func TestDiscoverControllers_FiltersByNodeSet(t *testing.T) {
	mkDeployment := func(name, ns, appLabel string) *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": appLabel}},
			},
		}
	}
	mkReplicaSet := func(name, ns, owningDeployment string) *appsv1.ReplicaSet {
		return &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: name, Namespace: ns,
				OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: owningDeployment, Controller: ptr.To(true)}},
			},
		}
	}
	// mkBareReplicaSet is a ReplicaSet with no owning Deployment — its pods
	// resolve to the ReplicaSet itself as the top-level controller.
	mkBareReplicaSet := func(name, ns, appLabel string) *appsv1.ReplicaSet {
		return &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: appsv1.ReplicaSetSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": appLabel}},
			},
		}
	}
	mkStatefulSet := func(name, ns, appLabel string) *appsv1.StatefulSet {
		return &appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
			Spec: appsv1.StatefulSetSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": appLabel}},
			},
		}
	}
	mkPod := func(name, ns, nodeName, ownerKind, ownerName string) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: name, Namespace: ns,
				OwnerReferences: []metav1.OwnerReference{{Kind: ownerKind, Name: ownerName, Controller: ptr.To(true)}},
			},
			Spec: corev1.PodSpec{NodeName: nodeName},
		}
	}

	client := fake.NewClientset(
		mkDeployment("target-app", "default", "target"),
		mkReplicaSet("target-app-abc", "default", "target-app"),
		// two pods of the same Deployment, both on the target node — must dedup.
		mkPod("target-pod-1", "default", "target-node", "ReplicaSet", "target-app-abc"),
		mkPod("target-pod-2", "default", "target-node", "ReplicaSet", "target-app-abc"),
		// StatefulSet pod on the target node — resolves to the StatefulSet.
		mkStatefulSet("target-sts", "default", "target-sts"),
		mkPod("target-sts-0", "default", "target-node", "StatefulSet", "target-sts"),
		// bare ReplicaSet (no owning Deployment) on the target node — resolves
		// to the ReplicaSet itself.
		mkBareReplicaSet("target-bare-rs", "default", "target-bare"),
		mkPod("target-bare-rs-xyz", "default", "target-node", "ReplicaSet", "target-bare-rs"),
		// DaemonSet-owned pod on the target node — the default switch arm
		// returns no controller (DaemonSets get no temporary PDB).
		mkPod("target-ds-pod", "default", "target-node", "DaemonSet", "target-ds"),
		// orphan pod (no controller owner) on the target node — ignored.
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "target-orphan", Namespace: "default"},
			Spec:       corev1.PodSpec{NodeName: "target-node"},
		},
		// pod on a non-target node, must be ignored.
		mkDeployment("off-target-app", "default", "off"),
		mkReplicaSet("off-target-app-def", "default", "off-target-app"),
		mkPod("off-target-pod", "default", "other-node", "ReplicaSet", "off-target-app-def"),
	)

	controllers, err := discoverControllers(t.Context(), client, map[string]struct{}{"target-node": {}})
	require.NoError(t, err)
	got := make(map[string]string, len(controllers)) // name -> kind
	for _, c := range controllers {
		got[c.Name] = c.Kind
	}
	assert.Equal(t, map[string]string{
		"target-app":     "Deployment",
		"target-sts":     "StatefulSet",
		"target-bare-rs": "ReplicaSet",
	}, got)
}
