package evict

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/fatih/color"
	"github.com/samber/lo"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clusterinfo"
)

// printPlan writes a human-readable description of the upcoming actions —
// cluster-autoscaler scale-down, PDB creation, per-target evictions — to
// streams.Out. Used both at confirmation time and in --dry-run mode.
func printPlan(streams genericclioptions.IOStreams, info *clusterinfo.ClusterInfo, targets []Target, scaleCA, ensurePDBs bool) {
	fmt.Fprintln(streams.Out, "\nThe following actions will be performed:")
	if scaleCA && info != nil && info.Autoscaling.ClusterAutoscaler.Present {
		fmt.Fprintf(streams.Out, "  • Scale cluster-autoscaler Deployment %s/%s to 0 replicas\n",
			info.Autoscaling.ClusterAutoscaler.Namespace, info.Autoscaling.ClusterAutoscaler.Name)
	}
	if ensurePDBs {
		fmt.Fprintln(streams.Out, "  • Create temporary PodDisruptionBudgets (maxUnavailable: 1) for workloads without one")
	}

	grouped := lo.GroupBy(targets, func(t Target) clusterinfo.NodeManager { return t.Manager })
	// Sort the manager keys for stable, deterministic output.
	for _, mgr := range slices.Sorted(maps.Keys(grouped)) {
		ts := grouped[mgr]
		fmt.Fprintf(streams.Out, "  • Evict %s entities:\n", mgr)
		for _, t := range ts {
			label := t.Entity
			if label == "" {
				label = "(all)"
			}
			fmt.Fprintf(streams.Out, "      - %s (%d node(s))\n", label, len(t.Nodes))
		}
	}
	if ensurePDBs {
		fmt.Fprintln(streams.Out, "  • Remove the temporary PodDisruptionBudgets created above")
	}
	fmt.Fprintln(streams.Out, color.YellowString("\n⚠ This will drain workloads and terminate the underlying instances of non-Datadog node groups."))
	fmt.Fprintln(streams.Out, color.YellowString("  Verify the Datadog Karpenter NodePool has enough capacity headroom for the migrated pods."))
}

// promptConfirmation reads a y/N answer from streams.In after the plan has
// been printed. Returns true when the user confirmed (typed `y` or `yes`).
// Read errors or any other input mean "declined" — unattended runs should
// pass --yes to skip the prompt entirely.
func promptConfirmation(streams genericclioptions.IOStreams) bool {
	fmt.Fprint(streams.Out, "\nContinue? (y/N): ")
	var response string
	// Fscanln may return "unexpected newline" when the user just presses
	// Enter; that's treated as decline below.
	_, _ = fmt.Fscanln(streams.In, &response)
	response = strings.ToLower(strings.TrimSpace(response))
	confirmed := response == "y" || response == "yes"
	if !confirmed {
		fmt.Fprintln(streams.Out, "Eviction cancelled.")
	}
	return confirmed
}
