package clusterinfo

import (
	"context"
	"errors"
	"fmt"
	"testing"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/autoscaling"
	astypes "github.com/aws/aws-sdk-go-v2/service/autoscaling/types"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	fakediscovery "k8s.io/client-go/discovery/fake"
	"k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlfake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/karpenter"
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

// fakeEKS implements EKSDescriber, returning a static profileName->tags map.
// Names absent from the map yield ResourceNotFoundException so tests can
// also exercise the failure path.
type fakeEKS struct {
	profiles map[string]map[string]string
	err      error
}

func (f *fakeEKS) DescribeFargateProfile(_ context.Context, in *eks.DescribeFargateProfileInput, _ ...func(*eks.Options)) (*eks.DescribeFargateProfileOutput, error) {
	if f.err != nil {
		return nil, f.err
	}
	tags, ok := f.profiles[awssdk.ToString(in.FargateProfileName)]
	if !ok {
		return nil, &ekstypes.ResourceNotFoundException{Message: awssdk.String("not found")}
	}
	return &eks.DescribeFargateProfileOutput{
		FargateProfile: &ekstypes.FargateProfile{
			FargateProfileName: in.FargateProfileName,
			Tags:               tags,
		},
	}, nil
}

func node(name string, providerID string, labels map[string]string) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels},
		Spec:       corev1.NodeSpec{ProviderID: providerID},
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

// nodePool builds a synthetic Karpenter NodePool unstructured for the
// controller-runtime fake client. We use the dynamic-typed shape here for
// the same reason classify.go does — keeps Karpenter API types out of the
// kubectl-datadog binary.
func nodePool(name string, labels map[string]string) *unstructured.Unstructured {
	u := &unstructured.Unstructured{}
	u.SetGroupVersionKind(schema.GroupVersionKind{Group: "karpenter.sh", Version: "v1", Kind: "NodePool"})
	u.SetName(name)
	u.SetLabels(labels)
	return u
}

// classifyOpts builds a minimal ClassifyInput for the existing tests that
// only exercised node classification + cluster-autoscaler detection. New
// fields default to fakes that report nothing.
func classifyOpts(clientset *fake.Clientset, asg AutoscalingDescriber) ClassifyInput {
	return ClassifyInput{
		K8sClient:   clientset,
		CtrlClient:  newCtrlFake(),
		Autoscaling: asg,
		EKS:         &fakeEKS{},
		Discovery:   newDiscoveryFake(false),
		ClusterName: "test-cluster",
	}
}

// newCtrlFake returns a controller-runtime fake client preloaded with the
// Karpenter NodePool list type so List() does not return NoMatchError. Tests
// that need to seed NodePools build their own client via WithObjects.
func newCtrlFake(npObjs ...ctrlclient.Object) ctrlclient.Client {
	scheme := runtime.NewScheme()
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "karpenter.sh", Version: "v1", Kind: "NodePoolList"}, &unstructured.UnstructuredList{})
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "karpenter.sh", Version: "v1", Kind: "NodePool"}, &unstructured.Unstructured{})
	return ctrlfake.NewClientBuilder().WithScheme(scheme).WithObjects(npObjs...).Build()
}

func newDiscoveryFake(autoMode bool) *fakediscovery.FakeDiscovery {
	resources := []*metav1.APIResourceList{}
	if autoMode {
		resources = append(resources, &metav1.APIResourceList{
			GroupVersion: "eks.amazonaws.com/v1",
			APIResources: []metav1.APIResource{{Name: "nodeclasses", Kind: "NodeClass"}},
		})
	}
	return &fakediscovery.FakeDiscovery{Fake: &k8stesting.Fake{Resources: resources}}
}

func TestClassify_EmptyCluster(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	asg := &fakeASG{}

	info, err := Classify(t.Context(), classifyOpts(clientset, asg))

	require.NoError(t, err)
	assert.Equal(t, APIVersion, info.APIVersion)
	assert.Equal(t, "test-cluster", info.ClusterName)
	assert.False(t, info.GeneratedAt.IsZero())
	assert.Empty(t, info.NodeManagement)
	assert.False(t, info.Autoscaling.ClusterAutoscaler.Present)
	assert.False(t, info.Autoscaling.Karpenter.Present)
	assert.False(t, info.Autoscaling.EKSAutoMode.Enabled)
	assert.Empty(t, asg.calls, "no candidates should mean no AWS API calls")
}

func TestClassify_AllBucketsByLabel(t *testing.T) {
	objs := []runtime.Object{
		// fargate via label
		node("fargate-by-label", "aws:///us-east-1a/fargate-abc", map[string]string{
			"eks.amazonaws.com/compute-type":    "fargate",
			"eks.amazonaws.com/fargate-profile": "fp-default",
		}),
		// fargate via name fallback (no compute-type label)
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

	info, err := Classify(t.Context(), classifyOpts(clientset, asg))
	require.NoError(t, err)

	assert.Equal(t, []string{"fargate-by-label"},
		info.NodeManagement[NodeManagerFargate]["fp-default"].Nodes)
	assert.Equal(t, []string{"fargate-ip-10-0-0-1.eu-west-3.compute.internal"},
		info.NodeManagement[NodeManagerFargate][""].Nodes)
	assert.Equal(t, []string{"kp-primary"},
		info.NodeManagement[NodeManagerKarpenter]["default-np"].Nodes)
	assert.Equal(t, []string{"kp-fallback"},
		info.NodeManagement[NodeManagerKarpenter]["default-nc"].Nodes)
	assert.Equal(t, []string{"kp-legacy"},
		info.NodeManagement[NodeManagerKarpenter]["legacy-provisioner"].Nodes)
	assert.Equal(t, []string{"mng"},
		info.NodeManagement[NodeManagerEKSManagedNodeGroup]["workers"].Nodes)
	assert.Equal(t, []string{"gke"},
		info.NodeManagement[NodeManagerUnknown]["gce://project/zone/instance"].Nodes)
	assert.Equal(t, []string{"orphan"},
		info.NodeManagement[NodeManagerUnknown][""].Nodes)
	assert.Empty(t, asg.calls, "label-only nodes must not trigger AWS calls")
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

	info, err := Classify(t.Context(), classifyOpts(clientset, asg))
	require.NoError(t, err)

	assert.ElementsMatch(t, []string{"worker-1", "worker-2"},
		info.NodeManagement[NodeManagerASG]["legacy-asg"].Nodes)
	assert.Equal(t, []string{"solo"},
		info.NodeManagement[NodeManagerStandalone][""].Nodes)
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

	info, err := Classify(t.Context(), classifyOpts(clientset, asg))
	require.NoError(t, err)

	require.Len(t, asg.calls, 2)
	assert.Len(t, asg.calls[0].InstanceIds, describeASGInstancesMaxIDs)
	assert.Len(t, asg.calls[1].InstanceIds, total-describeASGInstancesMaxIDs)
	assert.Len(t, info.NodeManagement[NodeManagerASG]["asg-1"].Nodes, total)
}

func TestClassify_ASGAPIError(t *testing.T) {
	clientset := fake.NewSimpleClientset(node("n", "aws:///us-east-1a/i-1111", nil))
	asg := &fakeASG{err: fmt.Errorf("AccessDenied")}

	_, err := Classify(t.Context(), classifyOpts(clientset, asg))
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
			info, err := Classify(t.Context(), classifyOpts(clientset, &fakeASG{}))
			require.NoError(t, err)
			assert.Equal(t, tc.want, info.Autoscaling.ClusterAutoscaler)
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
			info, err := Classify(t.Context(), classifyOpts(clientset, &fakeASG{}))
			require.NoError(t, err)
			assert.Equal(t, tc.want, info.Autoscaling.ClusterAutoscaler.Version)
		})
	}
}

func TestClassify_NoClusterAutoscaler(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		// Karpenter must not be detected as cluster-autoscaler.
		deploymentWith("dd-karpenter", "karpenter", nil, "public.ecr.aws/karpenter/karpenter:v1.9.0"),
	)
	info, err := Classify(t.Context(), classifyOpts(clientset, &fakeASG{}))
	require.NoError(t, err)
	assert.False(t, info.Autoscaling.ClusterAutoscaler.Present)
	assert.Empty(t, info.Autoscaling.ClusterAutoscaler.Namespace)
	assert.Empty(t, info.Autoscaling.ClusterAutoscaler.Name)
}

func TestClassify_ClusterAutoscalerScaledToZero(t *testing.T) {
	// A user following the Karpenter migration guide may have already
	// scaled the cluster-autoscaler Deployment to 0. We want Present: false
	// so the migration tooling doesn't repeatedly nag the user about it.
	clientset := fake.NewSimpleClientset(
		deploymentWithReplicas("kube-system", "cluster-autoscaler", 0, "registry.k8s.io/autoscaling/cluster-autoscaler:v1.30.0"),
	)
	info, err := Classify(t.Context(), classifyOpts(clientset, &fakeASG{}))
	require.NoError(t, err)
	assert.False(t, info.Autoscaling.ClusterAutoscaler.Present)
}

// karpenterControllerImage is the upstream chart's default — pinned in the
// karpenter package's own contract test, so a typo here would silently
// regress detection.
const karpenterControllerImage = "public.ecr.aws/karpenter/controller:v1.9.0"

func TestClassify_KarpenterDetection(t *testing.T) {
	for _, tc := range []struct {
		name   string
		deploy *appsv1.Deployment
		want   Karpenter
	}{
		{
			name: "kubectl-datadog installation surfaces with sentinel labels and version",
			deploy: deploymentWith("dd-karpenter", "karpenter",
				map[string]string{
					karpenter.InstalledByLabel:      karpenter.InstalledByValue,
					karpenter.InstallerVersionLabel: "v0.7.0",
				},
				karpenterControllerImage,
			),
			want: Karpenter{
				Present:          true,
				Namespace:        "dd-karpenter",
				Name:             "karpenter",
				Version:          "v1.9.0",
				ManagedByDatadog: true,
				InstallerVersion: "v0.7.0",
			},
		},
		{
			name:   "third-party installation: present but not managed",
			deploy: deploymentWith("karpenter", "karpenter", nil, karpenterControllerImage),
			want: Karpenter{
				Present:   true,
				Namespace: "karpenter",
				Name:      "karpenter",
				Version:   "v1.9.0",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset(tc.deploy)
			info, err := Classify(t.Context(), classifyOpts(clientset, &fakeASG{}))
			require.NoError(t, err)
			assert.Equal(t, tc.want, info.Autoscaling.Karpenter)
		})
	}
}

func TestClassify_EKSAutoMode(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	in := classifyOpts(clientset, &fakeASG{})
	in.Discovery = newDiscoveryFake(true)

	info, err := Classify(t.Context(), in)
	require.NoError(t, err)
	assert.True(t, info.Autoscaling.EKSAutoMode.Enabled)
}

func TestClassify_KarpenterNodePoolOwnership(t *testing.T) {
	// Four Karpenter NodePools exercising the ownership pass:
	//   - kdd: kubectl-datadog (both sentinel labels), already has a node.
	//   - clusteragent: cluster agent (only the broader `created` label),
	//     already has a node.
	//   - empty-kdd: kubectl-datadog NodePool with NO node yet (typical
	//     immediately after install). Must surface in the bucket so the
	//     migration tool can see it.
	//   - foreign: third-party NodePool, no Datadog label.
	nodes := []runtime.Object{
		node("kp-kdd", "aws:///us-east-1a/i-0aaa", map[string]string{"karpenter.sh/nodepool": "kdd"}),
		node("kp-clusteragent", "aws:///us-east-1a/i-0bbb", map[string]string{"karpenter.sh/nodepool": "clusteragent"}),
		node("kp-foreign", "aws:///us-east-1a/i-0ccc", map[string]string{"karpenter.sh/nodepool": "foreign"}),
	}
	clientset := fake.NewSimpleClientset(nodes...)

	in := classifyOpts(clientset, &fakeASG{})
	in.CtrlClient = newCtrlFake(
		nodePool("kdd", map[string]string{
			"app.kubernetes.io/managed-by":      "kubectl-datadog",
			"autoscaling.datadoghq.com/created": "true",
		}),
		nodePool("clusteragent", map[string]string{
			"autoscaling.datadoghq.com/created": "true",
		}),
		nodePool("empty-kdd", map[string]string{
			"app.kubernetes.io/managed-by":      "kubectl-datadog",
			"autoscaling.datadoghq.com/created": "true",
		}),
		nodePool("foreign", nil),
	)

	info, err := Classify(t.Context(), in)
	require.NoError(t, err)

	assert.True(t, info.NodeManagement[NodeManagerKarpenter]["kdd"].ManagedByDatadog,
		"NodePools with both labels must be flagged as Datadog-managed")
	assert.True(t, info.NodeManagement[NodeManagerKarpenter]["clusteragent"].ManagedByDatadog,
		"NodePools with only the broader 'created' label (cluster agent path) must also be flagged")

	emptyKdd, ok := info.NodeManagement[NodeManagerKarpenter]["empty-kdd"]
	require.True(t, ok, "Datadog-managed NodePools without nodes yet must still appear in the bucket")
	assert.True(t, emptyKdd.ManagedByDatadog)
	assert.Empty(t, emptyKdd.Nodes, "no node should have landed on the empty NodePool yet")

	assert.False(t, info.NodeManagement[NodeManagerKarpenter]["foreign"].ManagedByDatadog,
		"foreign NodePools must remain unflagged")
}

func TestClassify_KarpenterNodePoolOwnership_NoCRD(t *testing.T) {
	// When the karpenter.sh CRDs are not installed, the controller-runtime
	// fake client returns an apimeta.NoMatchError. The classifier must
	// tolerate it and leave entries unflagged rather than failing the
	// whole snapshot.
	clientset := fake.NewSimpleClientset(
		node("kp", "aws:///us-east-1a/i-0aaa", map[string]string{"karpenter.sh/nodepool": "default"}),
	)
	in := classifyOpts(clientset, &fakeASG{})
	// A fresh scheme without the NodePoolList registration triggers
	// NoMatchError on List.
	in.CtrlClient = ctrlfake.NewClientBuilder().WithScheme(runtime.NewScheme()).Build()

	info, err := Classify(t.Context(), in)
	require.NoError(t, err)
	assert.False(t, info.NodeManagement[NodeManagerKarpenter]["default"].ManagedByDatadog)
}

func TestClassify_FargateProfileOwnership(t *testing.T) {
	clientset := fake.NewSimpleClientset(
		node("fargate-1", "", map[string]string{
			"eks.amazonaws.com/compute-type":    "fargate",
			"eks.amazonaws.com/fargate-profile": "dd-karpenter-test",
		}),
		node("fargate-2", "", map[string]string{
			"eks.amazonaws.com/compute-type":    "fargate",
			"eks.amazonaws.com/fargate-profile": "third-party",
		}),
	)
	in := classifyOpts(clientset, &fakeASG{})
	in.EKS = &fakeEKS{
		profiles: map[string]map[string]string{
			"dd-karpenter-test": {"managed-by": "kubectl-datadog", "version": "v0.7.0"},
			"third-party":       {"managed-by": "someone-else"},
		},
	}

	info, err := Classify(t.Context(), in)
	require.NoError(t, err)

	assert.True(t, info.NodeManagement[NodeManagerFargate]["dd-karpenter-test"].ManagedByDatadog)
	assert.False(t, info.NodeManagement[NodeManagerFargate]["third-party"].ManagedByDatadog)
}

func TestClassify_FargateProfileOwnership_DescribeError(t *testing.T) {
	// Transient EKS API errors must not fail the snapshot. The profile
	// stays unflagged and a warning is logged.
	clientset := fake.NewSimpleClientset(
		node("fargate-1", "", map[string]string{
			"eks.amazonaws.com/compute-type":    "fargate",
			"eks.amazonaws.com/fargate-profile": "dd-karpenter-test",
		}),
	)
	in := classifyOpts(clientset, &fakeASG{})
	in.EKS = &fakeEKS{err: errors.New("AccessDenied")}

	info, err := Classify(t.Context(), in)
	require.NoError(t, err, "best-effort: API errors must not fail Classify")
	assert.False(t, info.NodeManagement[NodeManagerFargate]["dd-karpenter-test"].ManagedByDatadog)
}

func TestClassify_KarpenterDeploymentListError(t *testing.T) {
	// Surface unexpected errors from the controller Deployment lookup,
	// not silently lose them — the helm caller (steps.go) already logs
	// them as warnings.
	clientset := fake.NewSimpleClientset()
	clientset.PrependReactor("list", "deployments", func(_ k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewServiceUnavailable("test failure")
	})

	_, err := Classify(t.Context(), classifyOpts(clientset, &fakeASG{}))
	require.Error(t, err, "deployment list errors must propagate from Classify")
}
