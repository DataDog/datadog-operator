package evict

import "time"

// nodeDrainOptions captures the per-call tunables for evicting a node's pods.
type nodeDrainOptions struct {
	DryRun          bool
	EvictionTimeout time.Duration // per pod, bound for retries on 429
	NodeTimeout     time.Duration // total wait for the node to become empty
	PollInterval    time.Duration // interval between empty-checks; default 2s
}
