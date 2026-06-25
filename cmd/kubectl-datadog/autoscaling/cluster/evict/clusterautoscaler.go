package evict

import (
	"context"

	"k8s.io/client-go/kubernetes"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clusterinfo"
)

func scaleDownClusterAutoscaler(ctx context.Context, clientset kubernetes.Interface, ca clusterinfo.ClusterAutoscaler, dryRun bool) error {
	panic("TODO: scaleDownClusterAutoscaler — implemented in PR https://github.com/DataDog/datadog-operator/pull/3164")
}
