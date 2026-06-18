package evict

import (
	"context"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func runPreflightWarnings(ctx context.Context, streams genericclioptions.IOStreams, ctrlClient client.Client, targets []Target) {
	panic("TODO: runPreflightWarnings — implemented in PR https://github.com/DataDog/datadog-operator/pull/3163")
}
