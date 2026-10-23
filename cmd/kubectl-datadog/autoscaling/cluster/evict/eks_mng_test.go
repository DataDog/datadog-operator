package evict

import (
	"context"
	"errors"
	"testing"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"

	commonaws "github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/aws"
)

type stubEKS struct {
	gotInputs []*eks.UpdateNodegroupConfigInput
	err       error
}

func (s *stubEKS) UpdateNodegroupConfig(_ context.Context, in *eks.UpdateNodegroupConfigInput, _ ...func(*eks.Options)) (*eks.UpdateNodegroupConfigOutput, error) {
	s.gotInputs = append(s.gotInputs, in)
	return &eks.UpdateNodegroupConfigOutput{}, s.err
}

func mngDrainOpts() nodeDrainOptions {
	return nodeDrainOptions{
		EvictionTimeout: time.Second,
		NodeTimeout:     5 * time.Second,
		PollInterval:    10 * time.Millisecond,
	}
}

func TestEvictEKSManagedNodeGroup(t *testing.T) {
	const cluster, mng, nodeName = "my-cluster", "my-mng", "ip-1"
	newMngNode := func() *corev1.Node {
		return &corev1.Node{ObjectMeta: metav1.ObjectMeta{
			Name:   nodeName,
			Labels: map[string]string{commonaws.LabelEKSNodegroup: mng},
		}}
	}
	podOnNode := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "default", OwnerReferences: controllerOwnerRefs()},
		Spec:       corev1.PodSpec{NodeName: nodeName},
	}

	// installEvictionReactor makes the fake accept Eviction creates (echoing
	// the object) or fail them with evictErr.
	installEvictionReactor := func(client *fake.Clientset, evictErr error) {
		client.PrependReactor("create", "pods", func(a clienttesting.Action) (bool, runtime.Object, error) {
			ca, ok := a.(clienttesting.CreateAction)
			if !ok || ca.GetSubresource() != "eviction" {
				return false, nil, nil
			}
			if evictErr != nil {
				return true, nil, evictErr
			}
			return true, ca.GetObject(), nil
		})
	}
	// installPodListReactor returns the pod list for each 1-based List call, so a
	// case can drain a node across successive polls.
	installPodListReactor := func(client *fake.Clientset, lists func(call int) []corev1.Pod) {
		var n int
		client.PrependReactor("list", "pods", func(_ clienttesting.Action) (bool, runtime.Object, error) {
			n++
			return true, &corev1.PodList{Items: lists(n)}, nil
		})
	}
	// installNodeListReactor drives waitEKSNodegroupEmpty's Nodes().List (by
	// nodegroup label). It does not affect cordonNodes, which uses Get + Update.
	installNodeListReactor := func(client *fake.Clientset, items []corev1.Node) {
		client.PrependReactor("list", "nodes", func(_ clienttesting.Action) (bool, runtime.Object, error) {
			return true, &corev1.NodeList{Items: items}, nil
		})
	}
	assertScaledToZero := func(t *testing.T, in *eks.UpdateNodegroupConfigInput) {
		t.Helper()
		assert.Equal(t, cluster, awssdk.ToString(in.ClusterName))
		assert.Equal(t, mng, awssdk.ToString(in.NodegroupName))
		require.NotNil(t, in.ScalingConfig)
		assert.Equal(t, int32(0), awssdk.ToInt32(in.ScalingConfig.MinSize))
		assert.Equal(t, int32(0), awssdk.ToInt32(in.ScalingConfig.DesiredSize))
		// MaxSize is deliberately omitted so EKS preserves the existing ceiling
		// (partial ScalingConfig update).
		assert.Nil(t, in.ScalingConfig.MaxSize)
	}

	t.Run("dry-run cordons nothing and skips Update", func(t *testing.T) {
		stub := &stubEKS{}
		client := fake.NewClientset(newMngNode())
		opts := mngDrainOpts()
		opts.DryRun = true

		err := evictEKSManagedNodeGroup(t.Context(), stub, client, cluster, mng, []string{nodeName}, opts)
		require.NoError(t, err)
		assert.Empty(t, stub.gotInputs)
		got, getErr := client.CoreV1().Nodes().Get(t.Context(), nodeName, metav1.GetOptions{})
		require.NoError(t, getErr)
		assert.False(t, got.Spec.Unschedulable, "dry-run must not cordon")
	})

	t.Run("cordons, drains gracefully, then scales to zero", func(t *testing.T) {
		stub := &stubEKS{}
		client := fake.NewClientset(newMngNode())
		installEvictionReactor(client, nil)
		// The node still hosts the pod when drainNode enumerates it (call 1);
		// the eviction takes effect by the time waitForNodeEmpty polls (call 2+).
		installPodListReactor(client, func(call int) []corev1.Pod {
			if call == 1 {
				return []corev1.Pod{podOnNode}
			}
			return nil
		})
		// EKS has terminated the instance by the time we poll for nodes.
		installNodeListReactor(client, nil)

		err := evictEKSManagedNodeGroup(t.Context(), stub, client, cluster, mng, []string{nodeName}, mngDrainOpts())
		require.NoError(t, err)
		require.Len(t, stub.gotInputs, 1)
		assertScaledToZero(t, stub.gotInputs[0])
		got, getErr := client.CoreV1().Nodes().Get(t.Context(), nodeName, metav1.GetOptions{})
		require.NoError(t, getErr)
		assert.True(t, got.Spec.Unschedulable, "the node must be cordoned before draining")
	})

	t.Run("drain failure skips scale-down", func(t *testing.T) {
		stub := &stubEKS{}
		// The seeded pod is never removed, so the node never empties.
		client := fake.NewClientset(newMngNode(), &podOnNode)
		installEvictionReactor(client, apierrors.NewTooManyRequests("PDB blocked", 1))
		opts := mngDrainOpts()
		opts.EvictionTimeout = 30 * time.Millisecond
		opts.NodeTimeout = 60 * time.Millisecond

		err := evictEKSManagedNodeGroup(t.Context(), stub, client, cluster, mng, []string{nodeName}, opts)
		require.Error(t, err)
		assert.Empty(t, stub.gotInputs, "must not scale down while a node still holds workloads")
	})

	t.Run("cordon failure skips scale-down", func(t *testing.T) {
		stub := &stubEKS{}
		client := fake.NewClientset(newMngNode())
		client.PrependReactor("update", "nodes", func(_ clienttesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewInternalError(errors.New("cordon boom"))
		})

		err := evictEKSManagedNodeGroup(t.Context(), stub, client, cluster, mng, []string{nodeName}, mngDrainOpts())
		require.Error(t, err)
		assert.Empty(t, stub.gotInputs)
	})

	t.Run("UpdateNodegroupConfig failure after drain surfaces the error", func(t *testing.T) {
		stub := &stubEKS{err: errors.New("api error")}
		client := fake.NewClientset(newMngNode()) // no pods ⇒ the node drains immediately

		err := evictEKSManagedNodeGroup(t.Context(), stub, client, cluster, mng, []string{nodeName}, mngDrainOpts())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "UpdateNodegroupConfig")
		require.Len(t, stub.gotInputs, 1)
	})

	t.Run("post-scale wait timeout surfaces an error", func(t *testing.T) {
		stub := &stubEKS{}
		client := fake.NewClientset(newMngNode()) // no pods ⇒ the node drains immediately
		// The instance never terminates, so waitEKSNodegroupEmpty times out.
		installNodeListReactor(client, []corev1.Node{*newMngNode()})
		opts := mngDrainOpts()
		opts.NodeTimeout = 60 * time.Millisecond

		err := evictEKSManagedNodeGroup(t.Context(), stub, client, cluster, mng, []string{nodeName}, opts)
		require.Error(t, err)
		assert.Contains(t, err.Error(), mng)
		require.Len(t, stub.gotInputs, 1)
	})

	t.Run("post-scale node-list error surfaces an error", func(t *testing.T) {
		stub := &stubEKS{}
		client := fake.NewClientset(newMngNode()) // no pods ⇒ the node drains immediately
		// The scale-down succeeded, but the Nodes().List used to confirm the
		// group emptied errors out, so the drain cannot be confirmed complete.
		client.PrependReactor("list", "nodes", func(_ clienttesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewInternalError(errors.New("apiserver unreachable"))
		})

		err := evictEKSManagedNodeGroup(t.Context(), stub, client, cluster, mng, []string{nodeName}, mngDrainOpts())
		require.Error(t, err)
		require.Len(t, stub.gotInputs, 1)
	})
}
