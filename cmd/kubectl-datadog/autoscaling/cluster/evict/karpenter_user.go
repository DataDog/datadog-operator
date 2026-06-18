package evict

import (
	"context"

	"k8s.io/client-go/kubernetes"
)

// evictKarpenterUserNodePool cordons and drains the nodes managed by a user-
// created Karpenter NodePool. Once each node is cordoned and empty, Karpenter
// itself observes the state and terminates the underlying NodeClaim, and the
// pods re-scheduled by their controllers will be placed by Karpenter on the
// best-matching NodePool (ideally one managed by Datadog, if its spec.weight
// is high enough).
//
// The user NodePool spec is NOT modified — this command intentionally leaves
// user-managed NodePool configuration intact. If a user NodePool has a
// spec.weight greater than or equal to the Datadog NodePool weight, evicted
// pods may land on a freshly provisioned node from the SAME user NodePool.
// That case is surfaced by a pre-flight warning in the orchestrator; this
// function makes no attempt to re-balance the weights.
func evictKarpenterUserNodePool(ctx context.Context, clientset kubernetes.Interface, nodePoolName string, nodes []string, drainOpts nodeDrainOptions) error {
	panic("TODO: evictKarpenterUserNodePool — implemented in PR #12")
}
