package clusterinfo

import (
	"context"
	"fmt"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	astypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
)

// fakeASG implements AutoscalingDescriber, returning a static instance->ASG
// map and recording the inputs of every call so tests can assert batching.
type fakeASG struct {
	instances map[string]string
	calls     []*autoscaling.DescribeAutoScalingInstancesInput
	err       error
}

func (f *fakeASG) DescribeAutoScalingInstances(_ context.Context, in *autoscaling.DescribeAutoScalingInstancesInput, _ ...func(*autoscaling.Options)) (*autoscaling.DescribeAutoScalingInstancesOutput, error) {
	f.calls = append(f.calls, in)
	if f.err != nil {
		return nil, f.err
	}
	out := &autoscaling.DescribeAutoScalingInstancesOutput{}
	for _, id := range in.InstanceIds {
		if asgName, ok := f.instances[id]; ok {
			out.AutoScalingInstances = append(out.AutoScalingInstances, astypes.AutoScalingInstanceDetails{
				InstanceId:           awssdk.String(id),
				AutoScalingGroupName: awssdk.String(asgName),
			})
		}
	}
	return out, nil
}

func node(name string, providerID string, labels map[string]string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		Spec:       corev1.NodeSpec{ProviderID: providerID},
	}
}

func pod(name, namespace, nodeName string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels},
		Spec:       corev1.PodSpec{NodeName: nodeName},
	}
}

func deploymentWith(namespace, name string, labels map[string]string, image string) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name, Labels: labels},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "c", Image: image}}},
			},
		},
	}
}

func deploymentWithReplicas(namespace, name string, replicas int32, image string) *appsv1.Deployment {
	d := deploymentWith(namespace, name, nil, image)
	d.Spec.Replicas = &replicas
	return d
}

func TestClassify_EmptyCluster(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	asg := &fakeASG{}

	info, err := Classify(t.Context(), clientset, asg, "test-cluster")

	require.NoError(t, err)
	assert.Equal(t, APIVersion, info.APIVersion)
	assert.Equal(t, "test-cluster", info.ClusterName)
	assert.False(t, info.GeneratedAt.IsZero())
	assert.Empty(t, info.NodeManagement)
	assert.False(t, info.ClusterAutoscaler.Present)
	assert.Empty(t, asg.calls, "no candidates should mean no AWS API calls")
}

func TestClassify_AllBucketsByLabel(t *testing.T) {
	objs := []runtime.Object{
		// fargate via label; profile name comes from the hosted Pod's label
		// (EKS stamps `eks.amazonaws.com/fargate-profile` on the Pod, not the
		// Node).
		node("fargate-by-label", "aws:///us-east-1a/fargate-abc", map[string]string{
			"eks.amazonaws.com/compute-type": "fargate",
		}),
		pod("workload", "default", "fargate-by-label", map[string]string{
			"eks.amazonaws.com/fargate-profile": "fp-default",
		}),
		// fargate via name fallback (no compute-type label, no Pod yet
		// scheduled) — exercises the empty-key fallback path.
		node("fargate-ip-10-0-0-1.eu-west-3.compute.internal", "", nil),
		// karpenter via primary label
		node("kp-primary", "aws:///us-east-1a/i-0aaa", map[string]string{
			"karpenter.sh/nodepool": "default-np",
		}),
		// karpenter via fallback (only the EC2NodeClass label)
		node("kp-fallback", "aws:///us-east-1a/i-0bbb", map[string]string{
			"karpenter.k8s.aws/ec2nodeclass": "default-nc",
		}),
		// karpenter v0.x legacy label
		node("kp-legacy", "aws:///us-east-1a/i-0ddd", map[string]string{
			"karpenter.sh/provisioner-name": "legacy-provisioner",
		}),
		// EKS managed node group
		node("mng", "aws:///us-east-1a/i-0ccc", map[string]string{
			"eks.amazonaws.com/nodegroup": "workers",
		}),
		// non-AWS providerID -> unknown
		node("gke", "gce://project/zone/instance", nil),
		// empty providerID -> unknown
		node("orphan", "", nil),
	}
	clientset := fake.NewSimpleClientset(objs...)
	asg := &fakeASG{}

	info, err := Classify(t.Context(), clientset, asg, "c")
	require.NoError(t, err)

	assert.Equal(t, []string{"fargate-by-label"},
		info.NodeManagement[NodeManagerFargate]["fp-default"])
	assert.Equal(t, []string{"fargate-ip-10-0-0-1.eu-west-3.compute.internal"},
		info.NodeManagement[NodeManagerFargate][""])
	assert.Equal(t, []string{"kp-primary"},
		info.NodeManagement[NodeManagerKarpenter]["default-np"])
	assert.Equal(t, []string{"kp-fallback"},
		info.NodeManagement[NodeManagerKarpenter]["default-nc"])
	assert.Equal(t, []string{"kp-legacy"},
		info.NodeManagement[NodeManagerKarpenter]["legacy-provisioner"])
	assert.Equal(t, []string{"mng"},
		info.NodeManagement[NodeManagerEKSManagedNodeGroup]["workers"])
	assert.Equal(t, []string{"gke"},
		info.NodeManagement[NodeManagerUnknown]["gce://project/zone/instance"])
	assert.Equal(t, []string{"orphan"},
		info.NodeManagement[NodeManagerUnknown][""])
	assert.Empty(t, asg.calls, "label-only nodes must not trigger AWS calls")
}

func TestClassify_FargateMultipleProfiles(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		node("fargate-a", "", map[string]string{"eks.amazonaws.com/compute-type": "fargate"}),
		node("fargate-b", "", map[string]string{"eks.amazonaws.com/compute-type": "fargate"}),
		pod("workload-a", "team-a", "fargate-a", map[string]string{
			"eks.amazonaws.com/fargate-profile": "fp-team-a",
		}),
		pod("workload-b", "team-b", "fargate-b", map[string]string{
			"eks.amazonaws.com/fargate-profile": "fp-team-b",
		}),
	)

	info, err := Classify(t.Context(), clientset, &fakeASG{}, "c")
	require.NoError(t, err)

	assert.Equal(t, []string{"fargate-a"},
		info.NodeManagement[NodeManagerFargate]["fp-team-a"])
	assert.Equal(t, []string{"fargate-b"},
		info.NodeManagement[NodeManagerFargate]["fp-team-b"])
}

// TestClassify_FargatePendingPodSkipped guards the empty-NodeName branch of
// fargateProfilesByNode: a Pod that carries the Fargate label but isn't yet
// scheduled onto a Node must not populate the index (and must not panic).
func TestClassify_FargatePendingPodSkipped(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		node("fargate-a", "", map[string]string{"eks.amazonaws.com/compute-type": "fargate"}),
		// Scheduled pod -> profile resolves normally.
		pod("workload-a", "team-a", "fargate-a", map[string]string{
			"eks.amazonaws.com/fargate-profile": "fp-team-a",
		}),
		// Pending pod (no NodeName yet) -> must be ignored.
		pod("workload-pending", "team-a", "", map[string]string{
			"eks.amazonaws.com/fargate-profile": "fp-team-a",
		}),
	)

	info, err := Classify(t.Context(), clientset, &fakeASG{}, "c")
	require.NoError(t, err)

	assert.Equal(t, []string{"fargate-a"},
		info.NodeManagement[NodeManagerFargate]["fp-team-a"])
}

// TestClassify_FargatePodListErrorPropagates guards the error path of
// fargateProfilesByNode: a failing Pod listing must surface to the caller
// of Classify wrapped with a recognisable message.
func TestClassify_FargatePodListErrorPropagates(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	clientset.PrependReactor("list", "pods", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewServiceUnavailable("test failure")
	})

	_, err := Classify(t.Context(), clientset, &fakeASG{}, "c")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to list Fargate pods")
}

func TestClassify_ASGAndStandalone(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		node("worker-1", "aws:///us-east-1a/i-1111", nil),
		node("worker-2", "aws:///us-east-1a/i-2222", nil),
		node("solo", "aws:///us-east-1a/i-3333", nil),
	)
	asg := &fakeASG{
		instances: map[string]string{
			"i-1111": "legacy-asg",
			"i-2222": "legacy-asg",
			// i-3333 is intentionally absent -> standalone
		},
	}

	info, err := Classify(t.Context(), clientset, asg, "c")
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"worker-1", "worker-2"},
		info.NodeManagement[NodeManagerASG]["legacy-asg"])
	assert.Equal(t, []string{"solo"},
		info.NodeManagement[NodeManagerStandalone][""])
}

func TestClassify_ASGBatching(t *testing.T) {
	const total = 75 // 50 + 25 -> exactly 2 batches

	objs := make([]runtime.Object, 0, total)
	asg := &fakeASG{instances: map[string]string{}}
	for i := range total {
		id := fmt.Sprintf("i-%016x", i)
		objs = append(objs, node(fmt.Sprintf("n-%d", i), "aws:///us-east-1a/"+id, nil))
		asg.instances[id] = "asg-1"
	}
	clientset := fake.NewSimpleClientset(objs...)

	info, err := Classify(t.Context(), clientset, asg, "c")
	require.NoError(t, err)

	require.Len(t, asg.calls, 2)
	assert.Len(t, asg.calls[0].InstanceIds, describeASGInstancesMaxIDs)
	assert.Len(t, asg.calls[1].InstanceIds, total-describeASGInstancesMaxIDs)
	assert.Len(t, info.NodeManagement[NodeManagerASG]["asg-1"], total)
}

func TestClassify_ASGAPIError(t *testing.T) {
	clientset := fake.NewSimpleClientset(node("n", "aws:///us-east-1a/i-1111", nil))
	asg := &fakeASG{err: fmt.Errorf("AccessDenied")}

	_, err := Classify(t.Context(), clientset, asg, "c")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to describe autoscaling instances")
}

func TestClassify_ClusterAutoscalerDetection(t *testing.T) {
	for _, tc := range []struct {
		name   string
		deploy *appsv1.Deployment
		want   ClusterAutoscaler
	}{
		{
			name:   "by name",
			deploy: deploymentWith("kube-system", "cluster-autoscaler", nil, "registry.k8s.io/some-image:v1"),
			want:   ClusterAutoscaler{Present: true, Namespace: "kube-system", Name: "cluster-autoscaler"},
		},
		{
			name: "by app.kubernetes.io/name",
			deploy: deploymentWith("autoscaler", "ca-renamed",
				map[string]string{"app.kubernetes.io/name": "cluster-autoscaler"},
				"registry.k8s.io/foo:v1"),
			want: ClusterAutoscaler{Present: true, Namespace: "autoscaler", Name: "ca-renamed"},
		},
		{
			name: "by k8s-app",
			deploy: deploymentWith("autoscaler", "ca-renamed",
				map[string]string{"k8s-app": "cluster-autoscaler"},
				"registry.k8s.io/foo:v1"),
			want: ClusterAutoscaler{Present: true, Namespace: "autoscaler", Name: "ca-renamed"},
		},
		{
			name:   "by image substring",
			deploy: deploymentWith("custom", "scaler", nil, "registry.k8s.io/autoscaling/cluster-autoscaler:v1.30.0"),
			want:   ClusterAutoscaler{Present: true, Namespace: "custom", Name: "scaler", Version: "v1.30.0"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset(tc.deploy)
			info, err := Classify(t.Context(), clientset, &fakeASG{}, "c")
			require.NoError(t, err)
			assert.Equal(t, tc.want, info.ClusterAutoscaler)
		})
	}
}

func TestClassify_ClusterAutoscalerVersion(t *testing.T) {
	for _, tc := range []struct {
		name   string
		deploy *appsv1.Deployment
		want   string
	}{
		{
			name:   "from image tag",
			deploy: deploymentWith("kube-system", "cluster-autoscaler", nil, "registry.k8s.io/autoscaling/cluster-autoscaler:v1.30.0"),
			want:   "v1.30.0",
		},
		{
			name:   "image tag wins over label",
			deploy: deploymentWith("kube-system", "cluster-autoscaler", map[string]string{"app.kubernetes.io/version": "v9.9.9"}, "registry.k8s.io/autoscaling/cluster-autoscaler:v1.30.0"),
			want:   "v1.30.0",
		},
		{
			name:   "tag with digest suffix",
			deploy: deploymentWith("kube-system", "cluster-autoscaler", nil, "registry.k8s.io/autoscaling/cluster-autoscaler:v1.30.0@sha256:abcdef"),
			want:   "v1.30.0",
		},
		{
			name:   "registry with port and tag",
			deploy: deploymentWith("kube-system", "cluster-autoscaler", nil, "localhost:5000/cluster-autoscaler:v1.31.0"),
			want:   "v1.31.0",
		},
		{
			name:   "fallback to deployment label when image is digest only",
			deploy: deploymentWith("kube-system", "cluster-autoscaler", map[string]string{"app.kubernetes.io/version": "v1.32.0"}, "registry.k8s.io/autoscaling/cluster-autoscaler@sha256:abcdef"),
			want:   "v1.32.0",
		},
		{
			name:   "no tag, no label",
			deploy: deploymentWith("kube-system", "cluster-autoscaler", nil, "registry.k8s.io/autoscaling/cluster-autoscaler@sha256:abcdef"),
			want:   "",
		},
		{
			name:   "malformed image falls back to label",
			deploy: deploymentWith("kube-system", "cluster-autoscaler", map[string]string{"app.kubernetes.io/version": "v1.99.0"}, "cluster-autoscaler-:::"),
			want:   "v1.99.0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset(tc.deploy)
			info, err := Classify(t.Context(), clientset, &fakeASG{}, "c")
			require.NoError(t, err)
			assert.Equal(t, tc.want, info.ClusterAutoscaler.Version)
		})
	}
}

func TestClassify_NoClusterAutoscaler(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		// Karpenter must not be detected as cluster-autoscaler.
		deploymentWith("dd-karpenter", "karpenter", nil, "public.ecr.aws/karpenter/karpenter:v1.9.0"),
	)
	info, err := Classify(t.Context(), clientset, &fakeASG{}, "c")
	require.NoError(t, err)
	assert.False(t, info.ClusterAutoscaler.Present)
	assert.Empty(t, info.ClusterAutoscaler.Namespace)
	assert.Empty(t, info.ClusterAutoscaler.Name)
}

func TestClassify_ClusterAutoscalerScaledToZero(t *testing.T) {
	// A user following the Karpenter migration guide may have already
	// scaled the cluster-autoscaler Deployment to 0. We want Present: false
	// so the migration tooling doesn't repeatedly nag the user about it.
	clientset := fake.NewSimpleClientset(
		deploymentWithReplicas("kube-system", "cluster-autoscaler", 0, "registry.k8s.io/autoscaling/cluster-autoscaler:v1.30.0"),
	)
	info, err := Classify(t.Context(), clientset, &fakeASG{}, "c")
	require.NoError(t, err)
	assert.False(t, info.ClusterAutoscaler.Present)
}
