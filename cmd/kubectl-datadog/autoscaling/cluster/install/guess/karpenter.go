package guess

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/pager"
)

// Datadog-namespaced labels written via the Karpenter chart's additionalLabels.
// We avoid overriding standard app.kubernetes.io/* keys: the chart's
// _helpers.tpl emits them before additionalLabels, producing duplicate YAML
// keys whose deduplication at the API server is non-deterministic.
const (
	InstalledByLabel      = "autoscaling.datadoghq.com/installed-by"
	InstalledByValue      = "kubectl-datadog"
	InstallerVersionLabel = "autoscaling.datadoghq.com/installer-version"
)

// karpenterServiceEnvName is the env var name the upstream Karpenter chart's
// controller deployment unconditionally sets. Distinctive enough that no
// other workload sets it, and robust to image-registry rewrites — Docker
// Hardened Images, Chainguard, ECR pull-through caches all swap the image
// but keep this env intact.
const karpenterServiceEnvName = "KARPENTER_SERVICE"

// karpenterControllerImageRepoSuffix is the trailing two path components of
// the upstream chart's `controller.image.repository`. Secondary signal so
// chart forks that drop KARPENTER_SERVICE are still caught when they keep
// the canonical `karpenter/controller` path. Match is component-aware (not
// a substring) so `team/karpenter/controllers` or `someone/karpenter/controller-something`
// do not false-positive.
const karpenterControllerImageRepoSuffix = "karpenter/controller"

// KarpenterInstallation describes a Karpenter controller Deployment found on
// the cluster. The label fields capture the kubectl-datadog sentinel labels
// when present so callers can tell our installation apart from a third-party
// one via IsOwn.
type KarpenterInstallation struct {
	Namespace        string
	Name             string
	InstalledBy      string
	InstallerVersion string
}

// IsOwn reports whether the installation was created by kubectl-datadog.
func (k *KarpenterInstallation) IsOwn() bool {
	return k != nil && k.InstalledBy == InstalledByValue
}

// FindKarpenterInstallation returns the first Karpenter controller Deployment
// running on the cluster, or nil. We assume at most one Karpenter installed
// per cluster — kubectl-datadog's install/update guards prevent multiple
// controllers coexisting.
//
// Detection looks at the running controller (KARPENTER_SERVICE env var or
// `karpenter/controller` image suffix) rather than RBAC, since monitoring
// agents legitimately hold permissions on the karpenter.sh API group without
// running a controller.
func FindKarpenterInstallation(ctx context.Context, clientset kubernetes.Interface) (*KarpenterInstallation, error) {
	stop := errors.New("first match")

	var found *KarpenterInstallation
	p := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
		return clientset.AppsV1().Deployments(metav1.NamespaceAll).List(ctx, opts)
	})
	err := p.EachListItem(ctx, metav1.ListOptions{}, func(obj runtime.Object) error {
		dep := obj.(*appsv1.Deployment)
		if !hasKarpenterControllerContainer(dep.Spec.Template.Spec.Containers) {
			return nil
		}
		found = &KarpenterInstallation{
			Namespace:        dep.Namespace,
			Name:             dep.Name,
			InstalledBy:      dep.Labels[InstalledByLabel],
			InstallerVersion: dep.Labels[InstallerVersionLabel],
		}
		return stop
	})
	if err != nil && !errors.Is(err, stop) {
		return nil, fmt.Errorf("failed to list Deployments: %w", err)
	}
	return found, nil
}

// hasKarpenterControllerContainer reports whether any container in the pod
// spec is the Karpenter controller — primary signal is the
// chart-unconditional KARPENTER_SERVICE env var; secondary is the canonical
// `karpenter/controller` image repository tail.
func hasKarpenterControllerContainer(containers []corev1.Container) bool {
	return slices.ContainsFunc(containers, func(c corev1.Container) bool {
		return slices.ContainsFunc(c.Env, func(e corev1.EnvVar) bool { return e.Name == karpenterServiceEnvName }) ||
			imageRepoEndsWith(c.Image, karpenterControllerImageRepoSuffix)
	})
}

// imageRepoEndsWith reports whether `image`'s repository path (with tag and
// digest stripped) ends with the slash-separated path components in `suffix`.
//
// Stripping order matters because of registries with ports
// (`registry.local:5000/...`): digest comes off first (everything after `@`),
// then a tag is recognised only when the last `:` lies after the last `/` —
// otherwise the registry's port colon would be mistaken for a tag separator.
func imageRepoEndsWith(image, suffix string) bool {
	if i := strings.Index(image, "@"); i >= 0 {
		image = image[:i]
	}
	lastSlash := strings.LastIndex(image, "/")
	if lastColon := strings.LastIndex(image, ":"); lastColon > lastSlash {
		image = image[:lastColon]
	}
	suffixParts := strings.Split(suffix, "/")
	imageParts := strings.Split(image, "/")
	if len(imageParts) < len(suffixParts) {
		return false
	}
	return slices.Equal(imageParts[len(imageParts)-len(suffixParts):], suffixParts)
}
