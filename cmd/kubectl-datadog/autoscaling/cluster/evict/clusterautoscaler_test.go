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

func TestScaleDownClusterAutoscaler(t *testing.T) {
	caDep := func(replicas int32) *appsv1.Deployment {
		return &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "cluster-autoscaler", Namespace: "kube-system"},
			Spec:       appsv1.DeploymentSpec{Replicas: ptr.To(replicas)},
		}
	}
	caPresent := clusterinfo.ClusterAutoscaler{Present: true, Namespace: "kube-system", Name: "cluster-autoscaler"}

	for _, tc := range []struct {
		name string
		// dep, when non-nil, is pre-loaded into the fake clientset.
		dep *appsv1.Deployment
		// ca is what Run would have passed in from clusterinfo.Classify.
		ca clusterinfo.ClusterAutoscaler
		// conflictFirstUpdate forces the scaleReactor to return Conflict on
		// the first UpdateScale call so RetryOnConflict has to refetch.
		conflictFirstUpdate bool
		dryRun              bool
		// wantUpdateCalls is the minimum number of UpdateScale invocations
		// the test expects (>= because retries may happen).
		wantUpdateCalls int
		// wantReplicas, when non-nil, is the final value of the Deployment's
		// Spec.Replicas after the call.
		wantReplicas *int32
	}{
		{
			name: "CA absent is a no-op",
			ca:   clusterinfo.ClusterAutoscaler{Present: false},
		},
		{
			name:            "replicas already 0 skips Update",
			dep:             caDep(0),
			ca:              caPresent,
			wantUpdateCalls: 0,
			wantReplicas:    ptr.To(int32(0)),
		},
		{
			name:            "scales from 3 to 0",
			dep:             caDep(3),
			ca:              caPresent,
			wantUpdateCalls: 1,
			wantReplicas:    ptr.To(int32(0)),
		},
		{
			name:                "retries on Conflict",
			dep:                 caDep(2),
			ca:                  caPresent,
			conflictFirstUpdate: true,
			wantUpdateCalls:     2,
			wantReplicas:        ptr.To(int32(0)),
		},
		{
			name:            "dry-run touches nothing",
			dep:             caDep(3),
			ca:              caPresent,
			dryRun:          true,
			wantUpdateCalls: 0,
			wantReplicas:    ptr.To(int32(3)),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var objs []runtime.Object
			if tc.dep != nil {
				objs = append(objs, tc.dep)
			}
			client := fake.NewClientset(objs...)
			calls := scaleReactor(t, client, tc.conflictFirstUpdate)

			require.NoError(t, scaleDownClusterAutoscaler(context.Background(), client, tc.ca, tc.dryRun))

			assert.GreaterOrEqual(t, *calls, tc.wantUpdateCalls, "minimum UpdateScale calls")
			if tc.wantReplicas == nil {
				return
			}
			got, err := client.AppsV1().Deployments(tc.ca.Namespace).Get(context.Background(), tc.ca.Name, metav1.GetOptions{})
			require.NoError(t, err)
			require.NotNil(t, got.Spec.Replicas)
			assert.Equal(t, *tc.wantReplicas, *got.Spec.Replicas)
		})
	}
}
