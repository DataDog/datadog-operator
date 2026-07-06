package evict

import (
	"context"
	"errors"
	"fmt"
	"log"

	"k8s.io/client-go/kubernetes"
)

func evictKarpenterUserNodePool(ctx context.Context, clientset kubernetes.Interface, nodePoolName string, nodes []string, drainOpts nodeDrainOptions) error {
	cordoned, errs := cordonNodes(ctx, clientset, nodes, drainOpts.DryRun)
	for _, node := range cordoned {
		if err := drainNode(ctx, clientset, node.Name, drainOpts); err != nil {
			errs = append(errs, fmt.Errorf("drain node %s: %w", node.Name, err))
		}
	}
	if !drainOpts.DryRun && len(errs) == 0 {
		log.Printf("Drained %d node(s) from user NodePool %s; Karpenter will terminate their NodeClaims once empty.", len(cordoned), nodePoolName)
	}
	return errors.Join(errs...)
}
