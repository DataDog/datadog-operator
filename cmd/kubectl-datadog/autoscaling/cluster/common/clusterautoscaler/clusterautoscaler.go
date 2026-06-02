// Package clusterautoscaler detects the legacy kubernetes/autoscaler
// cluster-autoscaler controller running on the cluster, mirroring the
// API shape of the karpenter package.
package clusterautoscaler

import (
	"context"
	"slices"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	commonk8s "github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/k8s"
)

// Installation describes a cluster-autoscaler Deployment found on the
// cluster. Version is extracted from the controller image tag with a
// fallback to `app.kubernetes.io/version` labels.
//
// Unlike karpenter.Installation there is no IsOwn / InstalledBy:
// kubectl-datadog never installs cluster-autoscaler, only detects it.
type Installation struct {
	Namespace string
	Name      string
	Version   string
}

// FindInstallation returns the first cluster-autoscaler Deployment running
// on the cluster, or nil if none. A Deployment scaled to zero replicas is
// treated as absent — the Karpenter migration guide instructs users to
// scale CA to zero before adopting Karpenter, and we want `Present: false`
// in the snapshot in that state.
//
// Detection matches by Deployment name (`cluster-autoscaler`), the
// well-known `app.kubernetes.io/name` / `k8s-app` labels, or a container
// image referencing `cluster-autoscaler`.
func FindInstallation(ctx context.Context, clientset kubernetes.Interface) (*Installation, error) {
	dep, err := commonk8s.FindFirstDeployment(ctx, clientset, matchesDeployment)
	if err != nil || dep == nil {
		return nil, err
	}
	return &Installation{
		Namespace: dep.Namespace,
		Name:      dep.Name,
		Version:   commonk8s.ExtractDeploymentVersion(dep, matchesContainer),
	}, nil
}

func matchesDeployment(d *appsv1.Deployment) bool {
	if d.Spec.Replicas != nil && *d.Spec.Replicas == 0 {
		return false
	}
	if d.Name == "cluster-autoscaler" ||
		d.Labels["app.kubernetes.io/name"] == "cluster-autoscaler" ||
		d.Labels["k8s-app"] == "cluster-autoscaler" {
		return true
	}
	return slices.ContainsFunc(d.Spec.Template.Spec.Containers, matchesContainer)
}

func matchesContainer(c corev1.Container) bool {
	return strings.Contains(c.Image, "cluster-autoscaler")
}
