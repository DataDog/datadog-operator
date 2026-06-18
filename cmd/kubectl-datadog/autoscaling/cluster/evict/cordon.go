package evict

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

// cordonNodes marks every node in the group Unschedulable up front, before any
// of them is drained — mirroring how EKS marks a managed node group
// unschedulable before draining it. A pod evicted from one node is then never
// rescheduled onto another node of the same group that is itself about to be
// drained.
//
// It returns the Node objects that are now cordoned and safe to drain, so the
// caller can read their immutable fields (e.g. providerID) without a second
// Get. A node that is already gone is skipped silently (absent from both
// return values); a node that fails to cordon is recorded in errs and left out
// of cordoned so the caller never drains a node that can still receive pods.
func cordonNodes(ctx context.Context, clientset kubernetes.Interface, nodes []string, dryRun bool) (cordoned []*corev1.Node, errs []error) {
	panic("TODO: cordonNodes — implemented in PR #8")
}

// cordonNode marks the node Unschedulable and returns the resulting Node so the
// caller can read immutable fields (e.g. providerID) without a second Get. It
// returns (nil, nil) when the node is already gone: there is nothing to
// schedule onto a deleted node, and a re-run rediscovers any surviving nodes.
// The Get + mutate + Update is wrapped in RetryOnConflict to survive races
// against kubelet, the scheduler, and any other controller that mutates Node
// objects.
func cordonNode(ctx context.Context, clientset kubernetes.Interface, name string, dryRun bool) (*corev1.Node, error) {
	panic("TODO: cordonNode — implemented in PR #8")
}
