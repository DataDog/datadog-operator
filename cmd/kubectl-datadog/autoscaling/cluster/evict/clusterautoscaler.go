package evict

import (
	"context"

	"k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clusterinfo"
)

// scaleDownClusterAutoscaler patches the cluster-autoscaler Deployment's
// /scale subresource to 0 replicas. Using the subresource keeps the change
// minimal — we never touch the rest of the Deployment spec, so the diff is
// invisible to a Helm/Argo controller that watches the image, env, etc.
//
// Wrapped in RetryOnConflict to survive races against any controller that
// updates the Deployment between our Get and our Update.
//
// Idempotent: a Deployment already at 0 replicas returns nil without an
// Update call. Best-effort against GitOps controllers — if Helm/Argo reverts
// the change, this command does not loop; the operator is expected to pause
// the GitOps reconciliation for cluster-autoscaler before running.
func scaleDownClusterAutoscaler(ctx context.Context, clientset kubernetes.Interface, ca clusterinfo.ClusterAutoscaler, dryRun bool) error {
	panic("TODO: scaleDownClusterAutoscaler — implemented in PR #5")
}
