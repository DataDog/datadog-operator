package guess

import (
	"context"
	"errors"
	"fmt"
	"log"
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

// errStopScan is a sentinel returned by visit callbacks to stop the
// Deployment scan once the caller has found what it needs. Stripped at the
// scan boundary so it never leaks to callers.
var errStopScan = errors.New("stop scan")

// KarpenterInstallation describes a Karpenter controller Deployment found on
// the cluster. The label fields capture the kubectl-datadog sentinel labels
// when present so callers can tell our installation apart from a third-party
// one.
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

// ForeignKarpenter is the location of a Karpenter controller Deployment
// that conflicts with the install we're about to perform — either a
// third-party install or a previous kubectl-datadog install in a different
// namespace, both of which would race the new controller on the
// cluster-scoped Karpenter CRDs.
type ForeignKarpenter struct {
	Namespace string
	Name      string
}

// FindAnyKarpenterInstallation returns the first Karpenter controller
// Deployment running on the cluster, or nil. Detection signals are the same
// as FindForeignKarpenterInstallation (KARPENTER_SERVICE env var, or
// `karpenter/controller` image suffix); the returned struct exposes the
// kubectl-datadog sentinel labels so callers can decide whether the
// installation is theirs.
//
// Used by `update` to require a kubectl-datadog Karpenter exists before
// touching CFN stacks or the Helm release.
func FindAnyKarpenterInstallation(ctx context.Context, clientset kubernetes.Interface) (*KarpenterInstallation, error) {
	var found *KarpenterInstallation
	err := scanKarpenterInstallations(ctx, clientset, func(k *KarpenterInstallation) error {
		found = k
		return errStopScan
	})
	return found, err
}

// FindForeignKarpenterInstallation returns the location of a Karpenter
// controller Deployment running on the cluster that this `install` run
// would conflict with, or nil when none is found. `targetNamespace` is the
// namespace the install would deploy into; only a kubectl-datadog
// Deployment in that namespace is treated as ours and skipped — an
// existing kubectl-datadog Deployment in a *different* namespace would
// race the new one on the cluster-scoped Karpenter CRDs, so we surface
// it too.
//
// The scan short-circuits on the first foreign match: dense clusters with
// thousands of Deployments do not need to be fully materialised in memory
// just to answer "is there at least one foreign Karpenter Deployment".
func FindForeignKarpenterInstallation(ctx context.Context, clientset kubernetes.Interface, targetNamespace string) (*ForeignKarpenter, error) {
	var found *ForeignKarpenter
	err := scanKarpenterInstallations(ctx, clientset, func(k *KarpenterInstallation) error {
		if k.IsOwn() && k.Namespace == targetNamespace {
			return nil
		}
		log.Printf("Detected foreign Karpenter Deployment %s/%s", k.Namespace, k.Name)
		found = &ForeignKarpenter{Namespace: k.Namespace, Name: k.Name}
		return errStopScan
	})
	return found, err
}

// scanKarpenterInstallations walks every Deployment cluster-wide via
// pager.ListPager and invokes visit on each one whose pod spec matches the
// Karpenter controller fingerprint. Returning errStopScan from visit halts
// the walk; any other error propagates. Looking at the running controller is
// more robust than RBAC-based detection — monitoring or management roles
// legitimately hold permissions on the `karpenter.sh` API group without
// running a controller, and a Deployment that matches either container
// signal is the only one that distinguishes "Karpenter is actually running"
// from "something has read access to its CRs".
func scanKarpenterInstallations(ctx context.Context, clientset kubernetes.Interface, visit func(*KarpenterInstallation) error) error {
	p := pager.New(func(ctx context.Context, opts metav1.ListOptions) (runtime.Object, error) {
		return clientset.AppsV1().Deployments(metav1.NamespaceAll).List(ctx, opts)
	})
	err := p.EachListItem(ctx, metav1.ListOptions{}, func(obj runtime.Object) error {
		dep := obj.(*appsv1.Deployment)
		if !hasKarpenterControllerContainer(dep.Spec.Template.Spec.Containers) {
			return nil
		}
		return visit(&KarpenterInstallation{
			Namespace:        dep.Namespace,
			Name:             dep.Name,
			InstalledBy:      dep.Labels[InstalledByLabel],
			InstallerVersion: dep.Labels[InstallerVersionLabel],
		})
	})
	if err != nil && !errors.Is(err, errStopScan) {
		return fmt.Errorf("failed to list Deployments: %w", err)
	}
	return nil
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
