package evict

import (
	"context"

	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ensureTempPDBs(ctx context.Context, clientset kubernetes.Interface, ctrlClient client.Client, targets []Target, dryRun bool) error {
	panic("TODO: ensureTempPDBs — implemented in PR https://github.com/DataDog/datadog-operator/pull/3178")
}

func cleanupTempPDBs(ctx context.Context, ctrlClient client.Client, dryRun bool) error {
	panic("TODO: cleanupTempPDBs — implemented in PR https://github.com/DataDog/datadog-operator/pull/3178")
}
