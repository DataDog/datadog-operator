package evict

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clusterinfo"
)

func newKarpenterScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	sch := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(sch))
	gv := schema.GroupVersion{Group: "karpenter.sh", Version: "v1"}
	sch.AddKnownTypes(gv, &karpv1.NodePool{}, &karpv1.NodePoolList{})
	metav1.AddToGroupVersion(sch, gv)
	return sch
}

func mkNodePool(name string, weight *int32, datadogManaged bool) *karpv1.NodePool {
	labels := map[string]string{}
	if datadogManaged {
		labels[ddNodePoolCreatedLabel] = "true"
	}
	return &karpv1.NodePool{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		Spec:       karpv1.NodePoolSpec{Weight: weight},
	}
}

func TestWarnKarpenterWeightConflicts_NoKarpenterTargets(t *testing.T) {
	cli := ctrlfake.NewClientBuilder().WithScheme(newKarpenterScheme(t)).Build()
	streams, _, _, errBuf := genericclioptions.NewTestIOStreams()
	targets := []Target{
		{Manager: clusterinfo.NodeManagerASG, Entity: "asg-1"},
		{Manager: clusterinfo.NodeManagerStandalone, Entity: ""},
	}
	warnKarpenterWeightConflicts(context.Background(), streams, cli, targets)
	assert.Empty(t, errBuf.String(), "no karpenter targets ⇒ no warning")
}

func TestWarnKarpenterWeightConflicts_NoConflict(t *testing.T) {
	// Datadog NodePool has higher weight than the user one being evicted.
	cli := ctrlfake.NewClientBuilder().
		WithScheme(newKarpenterScheme(t)).
		WithObjects(
			mkNodePool("dd-np", ptr.To(int32(100)), true),
			mkNodePool("user-np", ptr.To(int32(50)), false),
		).
		Build()
	streams, _, _, errBuf := genericclioptions.NewTestIOStreams()
	targets := []Target{{Manager: clusterinfo.NodeManagerKarpenter, Entity: "user-np"}}
	warnKarpenterWeightConflicts(context.Background(), streams, cli, targets)
	assert.NotContains(t, errBuf.String(), "user-np", "no warning when user weight < Datadog weight")
}

func TestWarnKarpenterWeightConflicts_WeightConflictWarns(t *testing.T) {
	// User NodePool has equal weight ⇒ should warn (>= check).
	cli := ctrlfake.NewClientBuilder().
		WithScheme(newKarpenterScheme(t)).
		WithObjects(
			mkNodePool("dd-np", ptr.To(int32(10)), true),
			mkNodePool("user-np", ptr.To(int32(50)), false),
		).
		Build()
	streams, _, _, errBuf := genericclioptions.NewTestIOStreams()
	targets := []Target{{Manager: clusterinfo.NodeManagerKarpenter, Entity: "user-np"}}
	warnKarpenterWeightConflicts(context.Background(), streams, cli, targets)
	out := errBuf.String()
	assert.Contains(t, out, "user-np")
	assert.Contains(t, out, "weight=50")
}

func TestWarnKarpenterWeightConflicts_NilUserWeightUsesZero(t *testing.T) {
	// User NodePool without spec.weight defaults to 0; Datadog at 0 too →
	// equal-weight conflict still warns.
	cli := ctrlfake.NewClientBuilder().
		WithScheme(newKarpenterScheme(t)).
		WithObjects(
			mkNodePool("dd-np", nil, true),
			mkNodePool("user-np", nil, false),
		).
		Build()
	streams, _, _, errBuf := genericclioptions.NewTestIOStreams()
	targets := []Target{{Manager: clusterinfo.NodeManagerKarpenter, Entity: "user-np"}}
	warnKarpenterWeightConflicts(context.Background(), streams, cli, targets)
	assert.Contains(t, errBuf.String(), "user-np")
}

func TestWarnKarpenterWeightConflicts_TargetNotInClusterIsIgnored(t *testing.T) {
	// The target Karpenter entity name doesn't match any NodePool — no panic.
	cli := ctrlfake.NewClientBuilder().
		WithScheme(newKarpenterScheme(t)).
		WithObjects(mkNodePool("dd-np", ptr.To(int32(100)), true)).
		Build()
	streams, _, _, errBuf := genericclioptions.NewTestIOStreams()
	targets := []Target{{Manager: clusterinfo.NodeManagerKarpenter, Entity: "ghost-np"}}
	warnKarpenterWeightConflicts(context.Background(), streams, cli, targets)
	assert.Empty(t, errBuf.String(), "unknown user NodePool name ⇒ no warning")
}

// Ensure runPreflightWarnings is exercised (it just calls
// warnKarpenterWeightConflicts today, but the wrapper has its own coverage
// regardless of what is added later).
func newPreflightTestIO() (genericclioptions.IOStreams, *bytes.Buffer) {
	streams, _, _, errBuf := genericclioptions.NewTestIOStreams()
	return streams, errBuf
}

func TestRunPreflightWarnings_NoOp(t *testing.T) {
	cli := ctrlfake.NewClientBuilder().WithScheme(newKarpenterScheme(t)).Build()
	streams, errBuf := newPreflightTestIO()
	runPreflightWarnings(context.Background(), streams, cli, nil)
	assert.Empty(t, errBuf.String())
}
