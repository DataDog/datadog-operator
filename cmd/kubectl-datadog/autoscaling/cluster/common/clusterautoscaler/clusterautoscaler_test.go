package clusterautoscaler

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func deployment(namespace, name string, labels map[string]string, image string) *appsv1.Deployment {
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
	d := deployment(namespace, name, nil, image)
	d.Spec.Replicas = &replicas
	return d
}

func TestFindInstallation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		deploy  *appsv1.Deployment
		want    *Installation
	}{
		{
			name:   "no deployment",
			deploy: nil,
			want:   nil,
		},
		{
			name:   "match by name",
			deploy: deployment("kube-system", "cluster-autoscaler", nil, "registry.k8s.io/some-image:v1"),
			want:   &Installation{Namespace: "kube-system", Name: "cluster-autoscaler"},
		},
		{
			name: "match by app.kubernetes.io/name label",
			deploy: deployment("autoscaler", "ca-renamed",
				map[string]string{"app.kubernetes.io/name": "cluster-autoscaler"},
				"registry.k8s.io/foo:v1"),
			want: &Installation{Namespace: "autoscaler", Name: "ca-renamed"},
		},
		{
			name: "match by k8s-app label",
			deploy: deployment("autoscaler", "ca-renamed",
				map[string]string{"k8s-app": "cluster-autoscaler"},
				"registry.k8s.io/foo:v1"),
			want: &Installation{Namespace: "autoscaler", Name: "ca-renamed"},
		},
		{
			name:   "match by image substring (also extracts version)",
			deploy: deployment("custom", "scaler", nil, "registry.k8s.io/autoscaling/cluster-autoscaler:v1.30.0"),
			want:   &Installation{Namespace: "custom", Name: "scaler", Version: "v1.30.0"},
		},
		{
			name:   "scaled to zero is treated as absent",
			deploy: deploymentWithReplicas("kube-system", "cluster-autoscaler", 0, "registry.k8s.io/autoscaling/cluster-autoscaler:v1.30.0"),
			want:   nil,
		},
		{
			name:   "Karpenter Deployment is not the cluster-autoscaler",
			deploy: deployment("dd-karpenter", "karpenter", nil, "public.ecr.aws/karpenter/karpenter:v1.9.0"),
			want:   nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cli := fake.NewSimpleClientset()
			if tc.deploy != nil {
				cli = fake.NewSimpleClientset(tc.deploy)
			}

			got, err := FindInstallation(t.Context(), cli)

			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestFindInstallation_Version(t *testing.T) {
	for _, tc := range []struct {
		name   string
		deploy *appsv1.Deployment
		want   string
	}{
		{
			name:   "from image tag",
			deploy: deployment("kube-system", "cluster-autoscaler", nil, "registry.k8s.io/autoscaling/cluster-autoscaler:v1.30.0"),
			want:   "v1.30.0",
		},
		{
			name:   "image tag wins over label",
			deploy: deployment("kube-system", "cluster-autoscaler", map[string]string{"app.kubernetes.io/version": "v9.9.9"}, "registry.k8s.io/autoscaling/cluster-autoscaler:v1.30.0"),
			want:   "v1.30.0",
		},
		{
			name:   "tag with digest suffix",
			deploy: deployment("kube-system", "cluster-autoscaler", nil, "registry.k8s.io/autoscaling/cluster-autoscaler:v1.30.0@sha256:abcdef"),
			want:   "v1.30.0",
		},
		{
			name:   "registry with port and tag",
			deploy: deployment("kube-system", "cluster-autoscaler", nil, "localhost:5000/cluster-autoscaler:v1.31.0"),
			want:   "v1.31.0",
		},
		{
			name:   "fallback to deployment label when image is digest only",
			deploy: deployment("kube-system", "cluster-autoscaler", map[string]string{"app.kubernetes.io/version": "v1.32.0"}, "registry.k8s.io/autoscaling/cluster-autoscaler@sha256:abcdef"),
			want:   "v1.32.0",
		},
		{
			name:   "no tag, no label",
			deploy: deployment("kube-system", "cluster-autoscaler", nil, "registry.k8s.io/autoscaling/cluster-autoscaler@sha256:abcdef"),
			want:   "",
		},
		{
			name:   "malformed image falls back to label",
			deploy: deployment("kube-system", "cluster-autoscaler", map[string]string{"app.kubernetes.io/version": "v1.99.0"}, "cluster-autoscaler-:::"),
			want:   "v1.99.0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cli := fake.NewSimpleClientset(tc.deploy)

			got, err := FindInstallation(t.Context(), cli)

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tc.want, got.Version)
		})
	}
}
