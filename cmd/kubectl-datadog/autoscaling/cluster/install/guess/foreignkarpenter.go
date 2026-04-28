package guess

import (
	"context"
	"fmt"
	"log"
	"slices"
	"strings"

	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
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
// controller deployment unconditionally sets (its value is the chart's
// fullname, used by the controller to locate its own Service). Distinctive
// enough that no other workload sets it, and robust to image-registry
// rewrites — Docker Hardened Images, Chainguard, ECR pull-through caches
// all swap the image but keep this env intact.
const karpenterServiceEnvName = "KARPENTER_SERVICE"

// karpenterControllerImageRepoSuffix is the trailing two path components of
// the upstream chart's `controller.image.repository`. Used as a secondary
// signal so chart forks that drop KARPENTER_SERVICE are still caught when
// they keep the canonical `karpenter/controller` path. Match is
// component-aware (not a substring) so `team/karpenter/controllers` or
// `someone/karpenter/controller-something` do not false-positive.
const karpenterControllerImageRepoSuffix = "karpenter/controller"

// deploymentListChunkSize bounds the size of a single List response so we
// don't pull thousands of Deployments into memory at once on dense clusters.
// Matches the chunk size used by GetNodesProperties.
const deploymentListChunkSize = 100

// ForeignKarpenter is the location of a Karpenter controller Deployment that
// was not produced by this plugin.
type ForeignKarpenter struct {
	Namespace string
	Name      string
}

// FindForeignKarpenterInstallation returns the location of a Karpenter
// controller Deployment running on the cluster that was not produced by
// this plugin, or nil when none is found.
//
// Detection scans every Deployment for a container that either sets the
// chart-emitted `KARPENTER_SERVICE` env var or runs an image whose
// repository ends with `karpenter/controller`. Looking at the running
// controller is more robust than RBAC-based detection — monitoring or
// management roles legitimately hold permissions on the `karpenter.sh` API
// group without running a controller, and a Deployment that matches either
// container signal is the only one that distinguishes "Karpenter is
// actually running" from "something has read access to its CRs".
//
// Deployments bearing our InstalledByLabel sentinel are skipped. The list
// is paginated with an early exit on the first foreign match: dense
// clusters with thousands of Deployments do not need to be fully
// materialised in memory just to answer "is there at least one foreign
// Karpenter Deployment".
func FindForeignKarpenterInstallation(ctx context.Context, clientset kubernetes.Interface) (*ForeignKarpenter, error) {
	var cont string
	for {
		deps, err := clientset.AppsV1().Deployments(metav1.NamespaceAll).List(ctx, metav1.ListOptions{
			Limit:    deploymentListChunkSize,
			Continue: cont,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list Deployments: %w", err)
		}

		for _, dep := range deps.Items {
			if !hasKarpenterControllerContainer(dep.Spec.Template.Spec.Containers) {
				continue
			}
			if dep.Labels[InstalledByLabel] == InstalledByValue {
				continue
			}
			log.Printf("Detected foreign Karpenter Deployment %s/%s", dep.Namespace, dep.Name)
			return &ForeignKarpenter{Namespace: dep.Namespace, Name: dep.Name}, nil
		}

		cont = deps.Continue
		if cont == "" {
			return nil, nil
		}
	}
}

// hasKarpenterControllerContainer reports whether any container in the pod
// spec is the Karpenter controller — primary signal is the
// chart-unconditional KARPENTER_SERVICE env var; secondary is the canonical
// `karpenter/controller` image repository tail.
func hasKarpenterControllerContainer(containers []corev1.Container) bool {
	return lo.ContainsBy(containers, isKarpenterControllerContainer)
}

func isKarpenterControllerContainer(c corev1.Container) bool {
	if lo.ContainsBy(c.Env, func(e corev1.EnvVar) bool { return e.Name == karpenterServiceEnvName }) {
		return true
	}
	return imageRepoEndsWith(c.Image, karpenterControllerImageRepoSuffix)
}

// imageRepoEndsWith reports whether `image`'s repository path (with tag and
// digest stripped) ends with the slash-separated path components in `suffix`.
// Used to avoid false positives from `team/karpenter/controllers` or
// `someone/karpenter/controller-something`.
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
