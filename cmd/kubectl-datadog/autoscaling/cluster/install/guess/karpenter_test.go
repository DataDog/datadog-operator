package guess

import (
	"testing"

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

// karpenterControllerImage is the upstream chart's default controller image,
// constructed exactly as the `karpenter.controller.image` helper renders it.
const karpenterControllerImage = "public.ecr.aws/karpenter/controller:1.12.0"

// TestKarpenterControllerFingerprintContract pins the two signals we match
// on. Subsequent tests build fake Deployments via these constants, so a
// typo would silently let them pass while real Karpenter installs stop
// matching — this assertion locks the contract against the chart's
// deployment.yaml.
func TestKarpenterControllerFingerprintContract(t *testing.T) {
	assert.Equal(t, "KARPENTER_SERVICE", karpenterServiceEnvName)
	assert.Equal(t, "karpenter/controller", karpenterControllerImageRepoSuffix)
}

func TestImageRepoEndsWith(t *testing.T) {
	for _, tc := range []struct {
		image    string
		suffix   string
		expected bool
	}{
		{"public.ecr.aws/karpenter/controller:1.12.0", "karpenter/controller", true},
		{"public.ecr.aws/karpenter/controller@sha256:abc", "karpenter/controller", true},
		{"public.ecr.aws/karpenter/controller:1.12.0@sha256:abc", "karpenter/controller", true},
		{"012345678901.dkr.ecr.us-west-2.amazonaws.com/karpenter/controller:1.10.0", "karpenter/controller", true},
		{"registry.local:5000/karpenter/controller:1.12.0", "karpenter/controller", true},
		{"registry.local:5000/karpenter/controller@sha256:abc", "karpenter/controller", true},
		{"registry.local:5000/karpenter/controller:1.12.0@sha256:abc", "karpenter/controller", true},
		{"karpenter/controller", "karpenter/controller", true},
		{"team/karpenter/controllers:v1", "karpenter/controller", false},
		{"team/karpenter/controller-something:v1", "karpenter/controller", false},
		{"registry.local:5000/team/karpenter/controllers:v1", "karpenter/controller", false},
		{"public.ecr.aws/karpenter/karpenter:1.0", "karpenter/controller", false},
		{"cgr.dev/chainguard/karpenter:1.0", "karpenter/controller", false},
		{"controller", "karpenter/controller", false},
	} {
		t.Run(tc.image, func(t *testing.T) {
			assert.Equal(t, tc.expected, imageRepoEndsWith(tc.image, tc.suffix))
		})
	}
}

func TestFindKarpenterInstallation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		objects  []runtime.Object
		expected *KarpenterInstallation
	}{
		{
			name:     "no Deployments on the cluster",
			objects:  nil,
			expected: nil,
		},
		{
			name: "no Karpenter Deployment among unrelated ones",
			objects: []runtime.Object{
				deployment("kube-system", "coredns", nil, "registry.k8s.io/coredns/coredns:v1.10.1"),
				deployment("default", "nginx", nil, "nginx:1.25"),
			},
			expected: nil,
		},
		{
			name: "kubectl-datadog installation surfaces with sentinel labels",
			objects: []runtime.Object{
				deployment("dd-karpenter", "karpenter",
					map[string]string{
						InstalledByLabel:      InstalledByValue,
						InstallerVersionLabel: "v1.2.3",
					},
					karpenterControllerImage,
				),
			},
			expected: &KarpenterInstallation{
				Namespace:        "dd-karpenter",
				Name:             "karpenter",
				InstalledBy:      InstalledByValue,
				InstallerVersion: "v1.2.3",
			},
		},
		{
			name: "third-party installation surfaces with empty sentinel fields",
			objects: []runtime.Object{
				deployment("karpenter", "karpenter", nil, karpenterControllerImage),
			},
			expected: &KarpenterInstallation{
				Namespace: "karpenter",
				Name:      "karpenter",
			},
		},
		{
			name: "foreign Karpenter installed via a private mirror still matches",
			// Users behind a private registry rewrite the image but keep the
			// `karpenter/controller` path so the marker still hits.
			objects: []runtime.Object{
				deployment("kube-system", "their-karpenter", nil,
					"012345678901.dkr.ecr.us-west-2.amazonaws.com/karpenter/controller:1.10.0",
				),
			},
			expected: &KarpenterInstallation{
				Namespace: "kube-system",
				Name:      "their-karpenter",
			},
		},
		{
			name: "foreign Karpenter installed with custom nameOverride",
			// nameOverride only renames the Deployment object itself; the
			// controller still pulls public.ecr.aws/karpenter/controller.
			objects: []runtime.Object{
				deployment("autoscaling", "my-karpenter", nil, karpenterControllerImage),
			},
			expected: &KarpenterInstallation{
				Namespace: "autoscaling",
				Name:      "my-karpenter",
			},
		},
		{
			name: "Datadog Cluster Agent Deployment with karpenter.sh RBAC is not the controller",
			// The DD chart's cluster-agent grants karpenter.sh permissions to
			// inspect Karpenter resources, but its Deployment runs the
			// cluster-agent image and does not set KARPENTER_SERVICE. It
			// must not trigger the guard.
			objects: []runtime.Object{
				deployment("datadog-agent", "datadog-cluster-agent", nil,
					"gcr.io/datadoghq/cluster-agent:7.78.1",
				),
			},
			expected: nil,
		},
		{
			name: "hardened image without canonical path is matched via KARPENTER_SERVICE env",
			// Docker Hardened Images / Chainguard ship Karpenter under
			// repositories like `cgr.dev/chainguard/karpenter` whose path
			// does not end in `karpenter/controller`. The chart still sets
			// the KARPENTER_SERVICE env on the controller container; that
			// env name is the more robust signal.
			objects: []runtime.Object{
				deploymentWithEnv("kube-system", "their-karpenter", nil,
					"cgr.dev/chainguard/karpenter:1.0",
					[]corev1.EnvVar{{Name: "KARPENTER_SERVICE", Value: "their-karpenter"}},
				),
			},
			expected: &KarpenterInstallation{
				Namespace: "kube-system",
				Name:      "their-karpenter",
			},
		},
		{
			name: "image with `controllers` plural does not false-positive",
			// Defensive: a workload named `team/karpenter/controllers`
			// (note plural) must not match the canonical-suffix check.
			objects: []runtime.Object{
				deployment("team-ns", "controllers", nil,
					"team/karpenter/controllers:v1",
				),
			},
			expected: nil,
		},
		{
			name: "foreign sentinel value carries through to InstalledBy",
			objects: []runtime.Object{
				deployment("karpenter", "karpenter",
					map[string]string{InstalledByLabel: "someone-else"},
					karpenterControllerImage,
				),
			},
			expected: &KarpenterInstallation{
				Namespace:   "karpenter",
				Name:        "karpenter",
				InstalledBy: "someone-else",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			clientset := fake.NewSimpleClientset(tc.objects...)

			result, err := FindKarpenterInstallation(t.Context(), clientset)

			require.NoError(t, err)
			assert.Equal(t, tc.expected, result)
		})
	}

	t.Run("API list error propagates", func(t *testing.T) {
		clientset := fake.NewSimpleClientset()
		clientset.PrependReactor("list", "deployments", func(_ k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, apierrors.NewServiceUnavailable("test failure")
		})

		_, err := FindKarpenterInstallation(t.Context(), clientset)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to list Deployments")
	})

	t.Run("pagination forwards Continue tokens across pages", func(t *testing.T) {
		// Three pages with the Karpenter installation on the last one,
		// exercising cross-page Continue token forwarding.
		pages := []*appsv1.DeploymentList{
			{
				ListMeta: metav1.ListMeta{Continue: "page2"},
				Items:    nil,
			},
			{
				ListMeta: metav1.ListMeta{Continue: "page3"},
				Items: []appsv1.Deployment{
					*deployment("default", "nginx", nil, "nginx:1.25"),
				},
			},
			{
				Items: []appsv1.Deployment{
					*deployment("dd-karpenter", "karpenter",
						map[string]string{InstalledByLabel: InstalledByValue},
						karpenterControllerImage,
					),
				},
			},
		}

		clientset := fake.NewSimpleClientset()
		var calls []string
		clientset.PrependReactor("list", "deployments", func(action k8stesting.Action) (bool, runtime.Object, error) {
			opts := action.(k8stesting.ListActionImpl).GetListOptions()
			calls = append(calls, opts.Continue)
			assert.NotZero(t, opts.Limit, "Limit must be set so the API server can chunk")
			require.Less(t, len(calls)-1, len(pages),
				"reactor would over-fetch beyond the synthetic pages")
			return true, pages[len(calls)-1], nil
		})

		result, err := FindKarpenterInstallation(t.Context(), clientset)

		require.NoError(t, err)
		assert.Equal(t, &KarpenterInstallation{
			Namespace:   "dd-karpenter",
			Name:        "karpenter",
			InstalledBy: InstalledByValue,
		}, result)
		assert.Equal(t, []string{"", "page2", "page3"}, calls,
			"each call must forward the previous page's Continue token")
	})
}

func TestKarpenterInstallationIsOwn(t *testing.T) {
	for _, tc := range []struct {
		name     string
		k        *KarpenterInstallation
		expected bool
	}{
		{"nil receiver is not ours", nil, false},
		{"empty InstalledBy is not ours", &KarpenterInstallation{}, false},
		{"foreign InstalledBy is not ours", &KarpenterInstallation{InstalledBy: "someone-else"}, false},
		{"matching sentinel is ours", &KarpenterInstallation{InstalledBy: InstalledByValue}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, tc.k.IsOwn())
		})
	}
}

func deployment(namespace, name string, labels map[string]string, image string) *appsv1.Deployment {
	return deploymentWithEnv(namespace, name, labels, image, nil)
}

func deploymentWithEnv(namespace, name string, labels map[string]string, image string, env []corev1.EnvVar) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      name,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "controller", Image: image, Env: env},
					},
				},
			},
		},
	}
}
