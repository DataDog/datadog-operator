package guess

import (
	"context"
	"fmt"
	"log"
	"slices"

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

// karpenterAPIGroup is the API group every Karpenter controller's ClusterRole
// must reference: the chart's clusterrole.yaml and clusterrole-core.yaml
// templates hard-code rules on `karpenter.sh` for nodepools/nodeclaims, which
// is the structural fingerprint of a Karpenter install. Unlike the
// `app.kubernetes.io/name` label this is not affected by the chart's
// `nameOverride`, raw kubectl apply renames, or ArgoCD label rewrites.
const karpenterAPIGroup = "karpenter.sh"

// IsForeignKarpenterInstalled reports whether a Karpenter installation that
// was not produced by this plugin is running on the cluster. Detection lists
// every ClusterRole and looks for rules on the `karpenter.sh` API group,
// which the chart hard-codes for nodepools/nodeclaims regardless of
// `nameOverride` or other metadata customizations. ClusterRoles bearing our
// InstalledByLabel sentinel are skipped. ClusterRoles are deleted by `helm
// uninstall`, unlike the CRDs in `crds/`, so a leftover-only state returns
// false.
func IsForeignKarpenterInstalled(ctx context.Context, clientset kubernetes.Interface) (bool, error) {
	crs, err := clientset.RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("failed to list ClusterRoles: %w", err)
	}

	for _, cr := range crs.Items {
		if !hasKarpenterAPIGroupRule(cr.Rules) {
			continue
		}
		if cr.Labels[InstalledByLabel] == InstalledByValue {
			continue
		}
		log.Printf("Detected foreign Karpenter ClusterRole %q (instance=%q)", cr.Name, cr.Labels["app.kubernetes.io/instance"])
		return true, nil
	}

	return false, nil
}

// hasKarpenterAPIGroupRule reports whether any rule grants permissions on the
// karpenter.sh API group. We don't constrain on resource names: any rule
// touching the group is enough to identify a Karpenter ClusterRole, and a
// looser check stays robust against upstream resource additions.
func hasKarpenterAPIGroupRule(rules []rbacv1.PolicyRule) bool {
	for _, rule := range rules {
		if slices.Contains(rule.APIGroups, karpenterAPIGroup) {
			return true
		}
	}
	return false
}
