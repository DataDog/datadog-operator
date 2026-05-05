package karpenter

import (
	"context"
	"slices"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"

	commonk8s "github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/k8s"
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

// Installation describes a Karpenter controller Deployment found on the
// cluster. Version is the controller app version (extracted from the image
// tag with a fallback to `app.kubernetes.io/version` labels). The
// InstalledBy/InstallerVersion fields capture the kubectl-datadog sentinel
// labels when present so callers can tell our installation apart from a
// third-party one via IsOwn.
type Installation struct {
	Namespace        string
	Name             string
	Version          string
	InstalledBy      string
	InstallerVersion string
}

// IsOwn reports whether the installation was created by kubectl-datadog.
func (k *Installation) IsOwn() bool {
	return k != nil && k.InstalledBy == InstalledByValue
}

// FindInstallation returns the first Karpenter controller Deployment running
// on the cluster, or nil if none. We assume at most one Karpenter installed
// per cluster — kubectl-datadog's install/update guards prevent multiple
// controllers coexisting.
//
// Detection looks at the running controller (KARPENTER_SERVICE env var or
// `karpenter/controller` image suffix) rather than RBAC, since monitoring
// agents legitimately hold permissions on the karpenter.sh API group without
// running a controller.
func FindInstallation(ctx context.Context, clientset kubernetes.Interface) (*Installation, error) {
	dep, err := commonk8s.FindFirstDeployment(ctx, clientset, matchesController)
	if err != nil || dep == nil {
		return nil, err
	}
	return &Installation{
		Namespace:        dep.Namespace,
		Name:             dep.Name,
		Version:          commonk8s.ExtractDeploymentVersion(*dep, isControllerContainer),
		InstalledBy:      dep.Labels[InstalledByLabel],
		InstallerVersion: dep.Labels[InstallerVersionLabel],
	}, nil
}

// matchesController reports whether the Deployment runs the Karpenter
// controller. The primary signal is the KARPENTER_SERVICE env var on a
// container; the secondary is the canonical `karpenter/controller` image
// repository tail. See karpenterServiceEnvName and
// karpenterControllerImageRepoSuffix for the rationale.
func matchesController(d appsv1.Deployment) bool {
	return slices.ContainsFunc(d.Spec.Template.Spec.Containers, isControllerContainer)
}

func isControllerContainer(c corev1.Container) bool {
	return slices.ContainsFunc(c.Env, func(e corev1.EnvVar) bool { return e.Name == karpenterServiceEnvName }) ||
		imageRepoPathHasSuffix(c.Image, karpenterControllerImageRepoSuffix)
}

// imageRepoPathHasSuffix reports whether `image`'s repository path (registry
// stripped, tag and digest discarded) ends with the slash-separated path
// components in `suffix`. Match is component-aware so
// `team/karpenter/controllers` (plural) does not satisfy a
// `karpenter/controller` suffix.
func imageRepoPathHasSuffix(image, suffix string) bool {
	// Strip a trailing `@digest` so we tolerate combined `tag@digest` forms
	// (which pkg/name parses as a Digest, not a Tag) and malformed digest
	// strings (which would otherwise make pkg/name reject the whole image).
	if i := strings.Index(image, "@"); i >= 0 {
		image = image[:i]
	}
	ref, err := name.ParseReference(image, name.WeakValidation)
	if err != nil {
		return false
	}
	suffixParts := strings.Split(suffix, "/")
	imageParts := strings.Split(ref.Context().RepositoryStr(), "/")
	if len(imageParts) < len(suffixParts) {
		return false
	}
	return slices.Equal(imageParts[len(imageParts)-len(suffixParts):], suffixParts)
}
