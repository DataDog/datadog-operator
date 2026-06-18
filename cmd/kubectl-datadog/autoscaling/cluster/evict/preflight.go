package evict

import (
	"context"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// runPreflightWarnings prints non-blocking warnings for situations that don't
// prevent eviction but may surprise the operator after the fact. Pure
// best-effort — failures here are logged but do not abort the run.
func runPreflightWarnings(ctx context.Context, streams genericclioptions.IOStreams, ctrlClient client.Client, targets []Target) {
	panic("TODO: runPreflightWarnings — implemented in PR #4")
}
