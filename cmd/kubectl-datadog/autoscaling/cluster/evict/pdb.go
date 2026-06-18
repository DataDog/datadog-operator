package evict

import (
	"context"

	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ensureTempPDBs scans the pods running on the nodes of every target and
// creates a temporary PodDisruptionBudget (maxUnavailable: 1) for each
// top-level controller (Deployment, StatefulSet, bare ReplicaSet) that does
// not already have one with a matching selector.
//
// The created PDBs carry two labels (managed-by + temporary-pdb) that the
// cleanup step uses to find and delete them, regardless of which process
// created them. ensureTempPDBs itself is idempotent: a PDB created by a
// previous (possibly crashed) run is detected by its labels and left alone.
func ensureTempPDBs(ctx context.Context, clientset kubernetes.Interface, ctrlClient client.Client, targets []Target, dryRun bool) error {
	panic("TODO: ensureTempPDBs — implemented in PR #6")
}

// cleanupTempPDBs deletes every PodDisruptionBudget cluster-wide that carries
// both temp-PDB labels. Listing by labels (not by a state struct returned from
// ensureTempPDBs) is what makes the command crash-safe: a re-run after a kill
// still finds and removes the orphans left by the previous attempt.
func cleanupTempPDBs(ctx context.Context, ctrlClient client.Client, dryRun bool) error {
	panic("TODO: cleanupTempPDBs — implemented in PR #6")
}
