package evict

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clusterinfo"
)

// deploymentsGVR is the GroupVersionResource used to talk to the fake
// clientset's object tracker for Deployment objects.
var deploymentsGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}

// scaleReactor wires GetScale/UpdateScale on a fake clientset so the test
// observes the same read-modify-write surface as production. The fake doesn't
// natively support the Scale subresource, so we synthesize Scale objects from
// the underlying Deployment. The reactor talks to the underlying tracker
// directly (rather than via client.AppsV1()) because the fake holds an
// internal mutex while a reactor runs — calling back through the typed client
// would deadlock.
func scaleReactor(t *testing.T, client *fake.Clientset, conflictFirstUpdate bool) *int {
	t.Helper()
	updateCalls := 0
	tracker := client.Tracker()
	client.PrependReactor("get", "deployments", func(action clienttesting.Action) (bool, runtime.Object, error) {
		ga, ok := action.(clienttesting.GetAction)
		if !ok || ga.GetSubresource() != "scale" {
			return false, nil, nil
		}
		obj, err := tracker.Get(deploymentsGVR, ga.GetNamespace(), ga.GetName())
		if err != nil {
			return true, nil, err
		}
		dep := obj.(*appsv1.Deployment)
		var replicas int32
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		return true, &autoscalingv1.Scale{
			ObjectMeta: metav1.ObjectMeta{Name: dep.Name, Namespace: dep.Namespace, ResourceVersion: dep.ResourceVersion},
			Spec:       autoscalingv1.ScaleSpec{Replicas: replicas},
		}, nil
	})
	client.PrependReactor("update", "deployments", func(action clienttesting.Action) (bool, runtime.Object, error) {
		ua, ok := action.(clienttesting.UpdateAction)
		if !ok || ua.GetSubresource() != "scale" {
			return false, nil, nil
		}
		scale := ua.GetObject().(*autoscalingv1.Scale)
		updateCalls++
		if conflictFirstUpdate && updateCalls == 1 {
			return true, nil, apierrors.NewConflict(
				schema.GroupResource{Group: "apps", Resource: "deployments"},
				scale.Name,
				errors.New("forced conflict"),
			)
		}
		obj, err := tracker.Get(deploymentsGVR, ua.GetNamespace(), scale.Name)
		if err != nil {
			return true, nil, err
		}
		dep := obj.(*appsv1.Deployment).DeepCopy()
		dep.Spec.Replicas = ptr.To(scale.Spec.Replicas)
		if err := tracker.Update(deploymentsGVR, dep, ua.GetNamespace()); err != nil {
			return true, nil, err
		}
		return true, scale, nil
	})
	return &updateCalls
}

func TestScaleDownClusterAutoscaler_NotPresent(t *testing.T) {
	client := fake.NewClientset()
	require.NoError(t, scaleDownClusterAutoscaler(context.Background(), client, clusterinfo.ClusterAutoscaler{Present: false}, false))
}

func TestScaleDownClusterAutoscaler_AlreadyZero_NoUpdate(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-autoscaler", Namespace: "kube-system"},
		Spec:       appsv1.DeploymentSpec{Replicas: ptr.To(int32(0))},
	}
	client := fake.NewClientset(dep)
	calls := scaleReactor(t, client, false)

	require.NoError(t, scaleDownClusterAutoscaler(context.Background(), client, clusterinfo.ClusterAutoscaler{
		Present: true, Namespace: "kube-system", Name: "cluster-autoscaler",
	}, false))
	assert.Zero(t, *calls, "Update should not be called when replicas already 0")
}

func TestScaleDownClusterAutoscaler_ScalesToZero(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-autoscaler", Namespace: "kube-system"},
		Spec:       appsv1.DeploymentSpec{Replicas: ptr.To(int32(3))},
	}
	client := fake.NewClientset(dep)
	scaleReactor(t, client, false)

	require.NoError(t, scaleDownClusterAutoscaler(context.Background(), client, clusterinfo.ClusterAutoscaler{
		Present: true, Namespace: "kube-system", Name: "cluster-autoscaler",
	}, false))

	got, err := client.AppsV1().Deployments("kube-system").Get(context.Background(), "cluster-autoscaler", metav1.GetOptions{})
	require.NoError(t, err)
	require.NotNil(t, got.Spec.Replicas)
	assert.EqualValues(t, 0, *got.Spec.Replicas)
}

func TestScaleDownClusterAutoscaler_RetriesOnConflict(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-autoscaler", Namespace: "kube-system"},
		Spec:       appsv1.DeploymentSpec{Replicas: ptr.To(int32(2))},
	}
	client := fake.NewClientset(dep)
	calls := scaleReactor(t, client, true)

	require.NoError(t, scaleDownClusterAutoscaler(context.Background(), client, clusterinfo.ClusterAutoscaler{
		Present: true, Namespace: "kube-system", Name: "cluster-autoscaler",
	}, false))

	assert.GreaterOrEqual(t, *calls, 2, "UpdateScale should be retried after a Conflict")

	got, err := client.AppsV1().Deployments("kube-system").Get(context.Background(), "cluster-autoscaler", metav1.GetOptions{})
	require.NoError(t, err)
	require.NotNil(t, got.Spec.Replicas)
	assert.EqualValues(t, 0, *got.Spec.Replicas)
}

func TestScaleDownClusterAutoscaler_DryRun(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "cluster-autoscaler", Namespace: "kube-system"},
		Spec:       appsv1.DeploymentSpec{Replicas: ptr.To(int32(3))},
	}
	client := fake.NewClientset(dep)
	calls := scaleReactor(t, client, false)

	require.NoError(t, scaleDownClusterAutoscaler(context.Background(), client, clusterinfo.ClusterAutoscaler{
		Present: true, Namespace: "kube-system", Name: "cluster-autoscaler",
	}, true))

	assert.Zero(t, *calls, "dry-run must not call UpdateScale")
	got, err := client.AppsV1().Deployments("kube-system").Get(context.Background(), "cluster-autoscaler", metav1.GetOptions{})
	require.NoError(t, err)
	assert.EqualValues(t, 3, *got.Spec.Replicas)
}
