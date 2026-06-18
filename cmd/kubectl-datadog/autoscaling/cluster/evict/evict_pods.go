package evict

import (
	"context"
	"time"

	"k8s.io/client-go/kubernetes"
)

// nodeDrainOptions captures the per-call tunables for evicting a node's pods.
type nodeDrainOptions struct {
	DryRun          bool
	EvictionTimeout time.Duration // per pod, bound for retries on 429
	NodeTimeout     time.Duration // total wait for the node to become empty
	PollInterval    time.Duration // interval between empty-checks; default 2s
}

// drainNode evicts every evictable pod from the node and waits for the node to
// become empty. Pods owned by a DaemonSet, mirror pods, terminating pods and
// completed Job pods are skipped — the kubelet handles their cleanup when the
// underlying instance disappears.
//
// Pods that cannot be evicted (PDB-blocked beyond EvictionTimeout, etc.) are
// logged as warnings; drainNode then continues with the remaining pods rather
// than aborting the whole run.
func drainNode(ctx context.Context, clientset kubernetes.Interface, nodeName string, opts nodeDrainOptions) error {
	panic("TODO: drainNode — implemented in PR #8")
}
