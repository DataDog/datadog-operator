package evict

import (
	"context"

	"k8s.io/client-go/kubernetes"
)

func evictKarpenterUserNodePool(ctx context.Context, clientset kubernetes.Interface, nodePoolName string, nodes []string, drainOpts nodeDrainOptions) error {
	panic("TODO: evictKarpenterUserNodePool — implemented in PR https://github.com/DataDog/datadog-operator/pull/3177")
}
