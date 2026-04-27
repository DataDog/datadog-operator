package guess

import (
	"context"
	"fmt"
	"log"

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

// Standard chart-emitted label we use to recognize a Karpenter install,
// regardless of whether it was deployed via Helm, raw kubectl apply or ArgoCD.
const (
	karpenterChartNameLabel      = "app.kubernetes.io/name"
	karpenterChartNameValue      = "karpenter"
	karpenterClusterRoleSelector = karpenterChartNameLabel + "=" + karpenterChartNameValue
)

// IsForeignKarpenterInstalled reports whether a Karpenter installation that
// was not produced by this plugin is running on the cluster. Detection scans
// ClusterRoles labeled `app.kubernetes.io/name=karpenter` for the absence of
// our InstalledByLabel sentinel. ClusterRoles are deleted by `helm uninstall`,
// unlike the CRDs in `crds/`, so a leftover-only state returns false.
func IsForeignKarpenterInstalled(ctx context.Context, clientset kubernetes.Interface) (bool, error) {
	crs, err := clientset.RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{
		LabelSelector: karpenterClusterRoleSelector,
	})
	if err != nil {
		return false, fmt.Errorf("failed to list Karpenter ClusterRoles: %w", err)
	}

	for _, cr := range crs.Items {
		if cr.Labels[InstalledByLabel] == InstalledByValue {
			continue
		}
		log.Printf("Detected foreign Karpenter ClusterRole %q (instance=%q)", cr.Name, cr.Labels["app.kubernetes.io/instance"])
		return true, nil
	}

	return false, nil
}
