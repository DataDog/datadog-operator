package guess

import (
	"context"
	"fmt"
	"log"
	"slices"

	"github.com/samber/lo"
	rbacv1 "k8s.io/api/rbac/v1"
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

// karpenterAPIGroup is the API group the upstream v1 Karpenter chart's
// controller ClusterRole references. Unlike the `app.kubernetes.io/name`
// label this is not affected by the chart's `nameOverride`, raw kubectl
// apply renames, or ArgoCD label rewrites.
const karpenterAPIGroup = "karpenter.sh"

// karpenterControllerResources are resources the upstream v1 Karpenter
// chart's clusterrole-core.yaml spells out explicitly. We require at least
// one of them to be named in addition to the api group, because a wildcard
// rule on `karpenter.sh/*` does not identify the controller — the Datadog
// Operator's own ClusterRole carries such a wildcard to manage Karpenter
// custom resources without being a Karpenter controller itself.
var karpenterControllerResources = []string{"nodepools", "nodeclaims"}

// clusterRoleListChunkSize bounds the size of a single List response so we
// don't pull thousands of ClusterRoles into memory at once on dense clusters.
// Matches the chunk size used by GetNodesProperties.
const clusterRoleListChunkSize = 100

// IsForeignKarpenterInstalled reports whether a Karpenter installation that
// was not produced by this plugin is running on the cluster. Detection lists
// every ClusterRole and looks for rules that name both the `karpenter.sh`
// API group AND at least one Karpenter controller resource (`nodepools` or
// `nodeclaims`) — the structural fingerprint of a Karpenter controller's
// ClusterRole, regardless of `nameOverride` or other metadata
// customizations. ClusterRoles bearing our InstalledByLabel sentinel are
// skipped. ClusterRoles are deleted by `helm uninstall`, unlike the CRDs in
// `crds/`, so a leftover-only state returns false.
//
// The list is paginated with an early exit on the first foreign match: dense
// clusters with thousands of ClusterRoles do not need to be fully materialised
// in memory just to answer "is there at least one foreign Karpenter".
func IsForeignKarpenterInstalled(ctx context.Context, clientset kubernetes.Interface) (bool, error) {
	var cont string
	for {
		crs, err := clientset.RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{
			Limit:    clusterRoleListChunkSize,
			Continue: cont,
		})
		if err != nil {
			return false, fmt.Errorf("failed to list ClusterRoles: %w", err)
		}

		for _, cr := range crs.Items {
			if !hasKarpenterControllerRule(cr.Rules) {
				continue
			}
			if cr.Labels[InstalledByLabel] == InstalledByValue {
				continue
			}
			log.Printf("Detected foreign Karpenter ClusterRole")
			return true, nil
		}

		cont = crs.Continue
		if cont == "" {
			return false, nil
		}
	}
}

// hasKarpenterControllerRule reports whether any rule explicitly grants
// permissions on a Karpenter controller resource (`nodepools` or
// `nodeclaims`) under the `karpenter.sh` API group. A wildcard rule on
// `karpenter.sh/*` (e.g. the Datadog Operator's own role for managing
// Karpenter custom resources) does not match: the upstream v1 Karpenter
// chart spells out specific resource names on its controller ClusterRole.
func hasKarpenterControllerRule(rules []rbacv1.PolicyRule) bool {
	return lo.ContainsBy(rules, func(rule rbacv1.PolicyRule) bool {
		if !slices.Contains(rule.APIGroups, karpenterAPIGroup) {
			return false
		}
		return lo.SomeBy(karpenterControllerResources, func(resource string) bool {
			return slices.Contains(rule.Resources, resource)
		})
	})
}
