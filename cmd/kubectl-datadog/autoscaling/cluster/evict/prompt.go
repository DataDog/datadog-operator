package evict

import (
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clusterinfo"
)

// printPlan writes a human-readable description of the upcoming actions —
// cluster-autoscaler scale-down, PDB creation, per-target evictions — to
// streams.Out. Used both at confirmation time and in --dry-run mode.
func printPlan(streams genericclioptions.IOStreams, info *clusterinfo.ClusterInfo, targets []Target, scaleCA, ensurePDBs bool) {
	panic("TODO: printPlan — implemented in PR #3")
}

// promptConfirmation reads a y/N answer from streams.In after the plan has
// been printed. Returns true when the user confirmed (typed `y` or `yes`).
// Read errors or any other input mean "declined" — unattended runs should
// pass --yes to skip the prompt entirely.
func promptConfirmation(streams genericclioptions.IOStreams) bool {
	panic("TODO: promptConfirmation — implemented in PR #3")
}
