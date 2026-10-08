package evict

import (
	"context"
	"errors"
	"fmt"
	"log"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

func evictKarpenterUserNodePool(ctx context.Context, clientset kubernetes.Interface, ctrlClient client.Client, nodePoolName string, nodes []string, drainOpts nodeDrainOptions) error {
	cordoned, errs := cordonNodes(ctx, clientset, nodes, drainOpts.DryRun)

	// Map each node's providerID to its Karpenter NodeClaim so the claim can be
	// deleted once the node is drained. Deleting the NodeClaim is Karpenter's
	// native "decommission this node" action — it terminates the underlying
	// instance and removes the Node — and does NOT touch the user's NodePool
	// spec, so it stays within this command's scope. Without it, an empty
	// cordoned node lingers until Karpenter consolidation reaps it, which never
	// happens if the user disabled consolidation on their NodePool.
	//
	// Claims are indexed by providerID rather than filtered by NodePool label:
	// a target "entity" is only the NodePool name when the node carried the
	// karpenter.sh/nodepool label; classification also falls back to the
	// EC2NodeClass or (legacy) provisioner name, which would not match the
	// label. providerID ties a claim to the node we actually drained regardless.
	claimByProviderID, err := nodeClaimsByProviderID(ctx, ctrlClient)
	if err != nil {
		// Non-fatal: keep draining, and fall back to Karpenter consolidation for
		// the actual node removal. The error is surfaced so the operator knows
		// the claims may not have been deleted.
		errs = append(errs, fmt.Errorf("list NodeClaims for NodePool %s: %w", nodePoolName, err))
	}

	for _, node := range cordoned {
		if err := drainNode(ctx, clientset, node.Name, drainOpts); err != nil {
			errs = append(errs, fmt.Errorf("drain node %s: %w", node.Name, err))
			continue // do NOT delete the NodeClaim: workloads are still on it
		}
		if claimByProviderID == nil {
			continue // NodeClaims unavailable (listing failed, or the CRD is absent); any error was already recorded
		}
		nc, ok := claimByProviderID[node.Spec.ProviderID]
		if !ok {
			log.Printf("Warning: no Karpenter NodeClaim found for node %s (providerID %q) in NodePool %s; Karpenter will reap it once empty", node.Name, node.Spec.ProviderID, nodePoolName)
			continue
		}
		if err := deleteNodeClaim(ctx, ctrlClient, nc, drainOpts.DryRun); err != nil {
			errs = append(errs, fmt.Errorf("delete NodeClaim %s for node %s: %w", nc.Name, node.Name, err))
		}
	}
	return errors.Join(errs...)
}

// nodeClaimsByProviderID lists every Karpenter NodeClaim and indexes them by
// their status providerID — the stable key that ties a NodeClaim to its
// Kubernetes Node (node.spec.providerID). Claims without a providerID yet
// (still provisioning) are omitted; they carry no drained node. A missing
// NodeClaim CRD (e.g. legacy Karpenter v0.x with no NodeClaims) yields a nil
// map and no error, so the caller falls back to consolidation silently.
func nodeClaimsByProviderID(ctx context.Context, ctrlClient client.Client) (map[string]*karpv1.NodeClaim, error) {
	list := &karpv1.NodeClaimList{}
	if err := ctrlClient.List(ctx, list); err != nil {
		if meta.IsNoMatchError(err) {
			return nil, nil
		}
		return nil, err
	}
	byID := make(map[string]*karpv1.NodeClaim, len(list.Items))
	for i := range list.Items {
		nc := &list.Items[i]
		if nc.Status.ProviderID != "" {
			byID[nc.Status.ProviderID] = nc
		}
	}
	return byID, nil
}

// deleteNodeClaim deletes a single (already drained) NodeClaim. A claim that is
// already gone — Karpenter reaped it between the list and now — is treated as
// success.
func deleteNodeClaim(ctx context.Context, ctrlClient client.Client, nc *karpv1.NodeClaim, dryRun bool) error {
	if dryRun {
		log.Printf("[dry-run] would delete Karpenter NodeClaim %s", nc.Name)
		return nil
	}
	if err := ctrlClient.Delete(ctx, nc); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	log.Printf("Deleted Karpenter NodeClaim %s; Karpenter will terminate the underlying instance.", nc.Name)
	return nil
}
