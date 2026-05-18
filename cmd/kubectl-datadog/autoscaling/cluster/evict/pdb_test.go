package evict

import (
	"context"
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
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clusterinfo"
)

func newCtrlScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	sch := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(sch))
	return sch
}

func TestUniqueNodes_ExcludesEKSMNG(t *testing.T) {
	// EKS MNG targets are excluded on purpose: EKS handles their drain
	// asynchronously, so we don't create temp PDBs for pods on those nodes.
	targets := []Target{
		{Manager: clusterinfo.NodeManagerASG, Nodes: []string{"asg-1", "asg-2"}},
		{Manager: clusterinfo.NodeManagerEKSManagedNodeGroup, Nodes: []string{"mng-1"}},
		{Manager: clusterinfo.NodeManagerKarpenter, Nodes: []string{"kp-1"}},
		{Manager: clusterinfo.NodeManagerStandalone, Nodes: []string{"standalone-1"}},
	}
	got := uniqueNodes(targets)
	assert.Contains(t, got, "asg-1")
	assert.Contains(t, got, "asg-2")
	assert.Contains(t, got, "kp-1")
	assert.Contains(t, got, "standalone-1")
	assert.NotContains(t, got, "mng-1", "EKS MNG nodes must be excluded")
	assert.Len(t, got, 4)
}

func TestUniqueNodes_Dedup(t *testing.T) {
	targets := []Target{
		{Manager: clusterinfo.NodeManagerASG, Nodes: []string{"shared", "asg-only"}},
		{Manager: clusterinfo.NodeManagerStandalone, Nodes: []string{"shared", "standalone-only"}},
	}
	got := uniqueNodes(targets)
	assert.Len(t, got, 3)
	assert.Contains(t, got, "shared")
}

func TestTempPDBName(t *testing.T) {
	tests := []struct {
		kind   string
		name   string
		expect string
	}{
		{"Deployment", "my-app", "deployment-my-app-evict-legacy"},
		{"StatefulSet", "db", "statefulset-db-evict-legacy"},
		// Long name that must be truncated to fit 63 chars
		{"Deployment", strings.Repeat("a", 80), "deployment-" + strings.Repeat("a", 63-len("deployment-")-len(pdbNameSuffix)) + pdbNameSuffix},
	}
	for _, tc := range tests {
		got := tempPDBName(tc.kind, tc.name)
		assert.LessOrEqual(t, len(got), 63, "tempPDBName output exceeds 63 chars: %s", got)
		assert.Equal(t, tc.expect, got)
	}
}

func TestIsTemporaryPDB(t *testing.T) {
	temp := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
			pdbManagedByLabelKey: pdbManagedByLabelValue,
			pdbTempLabelKey:      pdbTempLabelValue,
		}},
	}
	user := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "x"}},
	}
	onlyManagedBy := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
			pdbManagedByLabelKey: pdbManagedByLabelValue,
		}},
	}
	assert.True(t, isTemporaryPDB(temp))
	assert.False(t, isTemporaryPDB(user))
	assert.False(t, isTemporaryPDB(onlyManagedBy), "must require BOTH labels")
}

func TestHasUserPDB(t *testing.T) {
	selector := &metav1.LabelSelector{MatchLabels: map[string]string{"app": "x"}}
	matching := policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "user", Namespace: "default"},
		Spec:       policyv1.PodDisruptionBudgetSpec{Selector: selector},
	}
	tempWithSameSelector := policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "temp", Namespace: "default", Labels: map[string]string{
			pdbManagedByLabelKey: pdbManagedByLabelValue,
			pdbTempLabelKey:      pdbTempLabelValue,
		}},
		Spec: policyv1.PodDisruptionBudgetSpec{Selector: selector},
	}

	t.Run("matching user PDB", func(t *testing.T) {
		assert.True(t, hasUserPDB([]policyv1.PodDisruptionBudget{matching}, selector))
	})
	t.Run("temp PDB is ignored", func(t *testing.T) {
		assert.False(t, hasUserPDB([]policyv1.PodDisruptionBudget{tempWithSameSelector}, selector),
			"temp PDB must not count as a user PDB")
	})
	t.Run("nil controller selector", func(t *testing.T) {
		assert.False(t, hasUserPDB([]policyv1.PodDisruptionBudget{matching}, nil))
	})
	t.Run("different selector", func(t *testing.T) {
		other := &metav1.LabelSelector{MatchLabels: map[string]string{"app": "y"}}
		assert.False(t, hasUserPDB([]policyv1.PodDisruptionBudget{matching}, other))
	})
}

func TestCleanupTempPDBs_DeletesOnlyLabelled(t *testing.T) {
	temp := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name: "temp", Namespace: "default",
			Labels: map[string]string{
				pdbManagedByLabelKey: pdbManagedByLabelValue,
				pdbTempLabelKey:      pdbTempLabelValue,
			},
		},
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

	cli := ctrlfake.NewClientBuilder().WithScheme(newCtrlScheme(t)).WithObjects(temp, userPDB, otherManaged).Build()

	require.NoError(t, cleanupTempPDBs(context.Background(), cli, false))

	list := &policyv1.PodDisruptionBudgetList{}
	require.NoError(t, cli.List(context.Background(), list))
	names := make([]string, 0, len(list.Items))
	for _, p := range list.Items {
		names = append(names, p.Namespace+"/"+p.Name)
	}
	assert.ElementsMatch(t, []string{"default/user", "kube-system/other"}, names)
}

func TestCleanupTempPDBs_NoTempPDBs_NoOp(t *testing.T) {
	user := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{Name: "user", Namespace: "default"},
	}
	cli := ctrlfake.NewClientBuilder().WithScheme(newCtrlScheme(t)).WithObjects(user).Build()
	require.NoError(t, cleanupTempPDBs(context.Background(), cli, false))
	list := &policyv1.PodDisruptionBudgetList{}
	require.NoError(t, cli.List(context.Background(), list))
	assert.Len(t, list.Items, 1)
}

func TestCleanupTempPDBs_DryRun_DoesNotDelete(t *testing.T) {
	temp := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name: "temp", Namespace: "default",
			Labels: map[string]string{
				pdbManagedByLabelKey: pdbManagedByLabelValue,
				pdbTempLabelKey:      pdbTempLabelValue,
			},
		},
	}
	cli := ctrlfake.NewClientBuilder().WithScheme(newCtrlScheme(t)).WithObjects(temp).Build()
	require.NoError(t, cleanupTempPDBs(context.Background(), cli, true))
	list := &policyv1.PodDisruptionBudgetList{}
	require.NoError(t, cli.List(context.Background(), list))
	assert.Len(t, list.Items, 1, "dry-run must not delete")
}

// TestCreateTempPDB_CrashRecovery is the killer test: it simulates the
// scenario where a previous run created a temp PDB and was then killed before
// cleanup. A fresh ensure/cleanup pair must NOT recreate it, and must still
// remove it at cleanup time.
func TestCreateTempPDB_Idempotent_PreExistingTempPDB(t *testing.T) {
	existing := &policyv1.PodDisruptionBudget{
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
	cli := ctrlfake.NewClientBuilder().WithScheme(newCtrlScheme(t)).WithObjects(existing).Build()

	c := controllerInfo{
		Namespace: "default", Kind: "Deployment", Name: "app",
		Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "x"}},
	}
	require.NoError(t, createTempPDB(context.Background(), cli, c, false))

	list := &policyv1.PodDisruptionBudgetList{}
	require.NoError(t, cli.List(context.Background(), list))
	require.Len(t, list.Items, 1, "createTempPDB must not duplicate when ours already exists")

	// And cleanup wipes it — the full crash-recovery loop.
	require.NoError(t, cleanupTempPDBs(context.Background(), cli, false))
	require.NoError(t, cli.List(context.Background(), list))
	assert.Empty(t, list.Items, "cleanup must remove pre-existing temp PDB even with no state from ensure")
}

func TestCreateTempPDB_CreatesWhenMissing(t *testing.T) {
	cli := ctrlfake.NewClientBuilder().WithScheme(newCtrlScheme(t)).Build()
	c := controllerInfo{
		Namespace: "default", Kind: "Deployment", Name: "app",
		Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "x"}},
	}
	require.NoError(t, createTempPDB(context.Background(), cli, c, false))
	list := &policyv1.PodDisruptionBudgetList{}
	require.NoError(t, cli.List(context.Background(), list))
	require.Len(t, list.Items, 1)
	pdb := list.Items[0]
	assert.Equal(t, pdbManagedByLabelValue, pdb.Labels[pdbManagedByLabelKey])
	assert.Equal(t, pdbTempLabelValue, pdb.Labels[pdbTempLabelKey])
	require.NotNil(t, pdb.Spec.MaxUnavailable)
	assert.Equal(t, int32(1), pdb.Spec.MaxUnavailable.IntVal)
}

func TestCreateTempPDB_NameCollisionWithUserPDB(t *testing.T) {
	collision := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name: tempPDBName("Deployment", "app"), Namespace: "default",
			// no temp-pdb labels
		},
	}
	cli := ctrlfake.NewClientBuilder().WithScheme(newCtrlScheme(t)).WithObjects(collision).Build()
	c := controllerInfo{
		Namespace: "default", Kind: "Deployment", Name: "app",
		Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "x"}},
	}
	require.NoError(t, createTempPDB(context.Background(), cli, c, false))
	list := &policyv1.PodDisruptionBudgetList{}
	require.NoError(t, cli.List(context.Background(), list))
	require.Len(t, list.Items, 1, "user PDB at same name must be left untouched, no new one created")
	// The single remaining PDB must be the user's, not labelled by us.
	assert.NotEqual(t, pdbManagedByLabelValue, list.Items[0].Labels[pdbManagedByLabelKey])
}

func TestCreateTempPDB_DryRun(t *testing.T) {
	cli := ctrlfake.NewClientBuilder().WithScheme(newCtrlScheme(t)).Build()
	c := controllerInfo{
		Namespace: "default", Kind: "Deployment", Name: "app",
		Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "x"}},
	}
	require.NoError(t, createTempPDB(context.Background(), cli, c, true))
	list := &policyv1.PodDisruptionBudgetList{}
	require.NoError(t, cli.List(context.Background(), list))
	assert.Empty(t, list.Items, "dry-run must not create PDB")
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
	mkPod := func(name, ns, nodeName, owningRS string) *corev1.Pod {
		return &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: name, Namespace: ns,
				OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: owningRS, Controller: ptr.To(true)}},
			},
			Spec: corev1.PodSpec{NodeName: nodeName},
		}
	}

	client := fake.NewClientset(
		mkDeployment("target-app", "default", "target"),
		mkReplicaSet("target-app-abc", "default", "target-app"),
		// two pods of the same Deployment, both on the target node — must dedup.
		mkPod("target-pod-1", "default", "target-node", "target-app-abc"),
		mkPod("target-pod-2", "default", "target-node", "target-app-abc"),
		// pod on a non-target node, must be ignored.
		mkDeployment("off-target-app", "default", "off"),
		mkReplicaSet("off-target-app-def", "default", "off-target-app"),
		mkPod("off-target-pod", "default", "other-node", "off-target-app-def"),
	)

	controllers, err := discoverControllers(context.Background(), client, map[string]struct{}{"target-node": {}})
	require.NoError(t, err)
	require.Len(t, controllers, 1)
	assert.Equal(t, "target-app", controllers[0].Name)
	assert.Equal(t, "Deployment", controllers[0].Kind)
}
