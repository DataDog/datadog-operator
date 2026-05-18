package evict

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clusterinfo"
)

func TestPrintPlan_AllSections(t *testing.T) {
	streams, _, outBuf, _ := genericclioptions.NewTestIOStreams()
	info := &clusterinfo.ClusterInfo{
		Autoscaling: clusterinfo.Autoscaling{
			ClusterAutoscaler: clusterinfo.ClusterAutoscaler{Present: true, Namespace: "kube-system", Name: "cluster-autoscaler"},
		},
	}
	targets := []Target{
		{Manager: clusterinfo.NodeManagerASG, Entity: "legacy-asg", Nodes: []string{"ip-1", "ip-2"}},
		{Manager: clusterinfo.NodeManagerKarpenter, Entity: "user-np", Nodes: []string{"ip-3"}},
		{Manager: clusterinfo.NodeManagerStandalone, Entity: "", Nodes: []string{"ip-4"}},
	}
	printPlan(streams, info, targets, true, true)
	out := outBuf.String()

	// CA scale-down line present when scaleCA=true and CA detected.
	assert.Contains(t, out, "cluster-autoscaler Deployment kube-system/cluster-autoscaler")
	// PDB ensure + cleanup lines present when ensurePDBs=true.
	assert.Contains(t, out, "Create temporary PodDisruptionBudgets")
	assert.Contains(t, out, "Remove the temporary PodDisruptionBudgets")
	// Each manager section listed.
	assert.Contains(t, out, "Evict asg entities")
	assert.Contains(t, out, "legacy-asg (2 node(s))")
	assert.Contains(t, out, "Evict karpenter entities")
	assert.Contains(t, out, "user-np (1 node(s))")
	assert.Contains(t, out, "Evict standalone entities")
	// Standalone entry's empty name renders as "(all)".
	assert.Contains(t, out, "(all) (1 node(s))")
}

func TestPrintPlan_SkipsCAWhenScaleDisabled(t *testing.T) {
	streams, _, outBuf, _ := genericclioptions.NewTestIOStreams()
	info := &clusterinfo.ClusterInfo{
		Autoscaling: clusterinfo.Autoscaling{
			ClusterAutoscaler: clusterinfo.ClusterAutoscaler{Present: true, Namespace: "kube-system", Name: "cluster-autoscaler"},
		},
	}
	targets := []Target{{Manager: clusterinfo.NodeManagerASG, Entity: "x"}}
	printPlan(streams, info, targets, false, false)
	out := outBuf.String()
	assert.NotContains(t, out, "Scale cluster-autoscaler", "scaleCA=false hides CA bullet")
	assert.NotContains(t, out, "PodDisruptionBudgets", "ensurePDBs=false hides PDB bullets")
}

func TestPrintPlan_SkipsCAWhenAbsent(t *testing.T) {
	streams, _, outBuf, _ := genericclioptions.NewTestIOStreams()
	info := &clusterinfo.ClusterInfo{
		Autoscaling: clusterinfo.Autoscaling{
			ClusterAutoscaler: clusterinfo.ClusterAutoscaler{Present: false},
		},
	}
	printPlan(streams, info, nil, true, false)
	out := outBuf.String()
	assert.NotContains(t, out, "Scale cluster-autoscaler", "CA not Present ⇒ no bullet")
}

func TestPrintPlan_NilInfoTolerated(t *testing.T) {
	streams, _, outBuf, _ := genericclioptions.NewTestIOStreams()
	printPlan(streams, nil, nil, true, true)
	out := outBuf.String()
	// Nil info ⇒ no CA bullet rendered, but PDB bullets and the warning footer still appear.
	assert.NotContains(t, out, "Scale cluster-autoscaler")
	assert.Contains(t, out, "Create temporary PodDisruptionBudgets")
}

func TestPromptConfirmation_AcceptsYes(t *testing.T) {
	for _, in := range []string{"y", "Y", "yes", "YES", "Yes\n"} {
		streams, stdin, _, _ := genericclioptions.NewTestIOStreams()
		stdin.WriteString(in + "\n")
		assert.True(t, promptConfirmation(streams), "input %q should confirm", in)
	}
}

func TestPromptConfirmation_RejectsAnythingElse(t *testing.T) {
	for _, in := range []string{"n", "N", "no", "", "blah", "1"} {
		streams, stdin, outBuf, _ := genericclioptions.NewTestIOStreams()
		stdin.WriteString(in + "\n")
		assert.False(t, promptConfirmation(streams), "input %q must NOT confirm", in)
		// Cancellation message printed to Out.
		assert.True(t, strings.Contains(outBuf.String(), "cancelled") || strings.Contains(outBuf.String(), "Cancelled"))
	}
}
