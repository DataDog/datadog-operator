package evict

import (
	"context"
	"fmt"
	"log"

	"github.com/fatih/color"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clusterinfo"
)

// ddNodePoolCreatedLabel is the label every Datadog autoscaling product (this
// CLI and the cluster agent) puts on the Karpenter NodePools it creates. Used
// here to identify "Datadog-side" NodePools when comparing weights against
// user-side ones.
const ddNodePoolCreatedLabel = "autoscaling.datadoghq.com/created"

// runPreflightWarnings prints non-blocking warnings for situations that don't
// prevent eviction but may surprise the operator after the fact. Pure
// best-effort — failures here are logged but do not abort the run.
func runPreflightWarnings(ctx context.Context, streams genericclioptions.IOStreams, ctrlClient client.Client, targets []Target) {
	warnKarpenterWeightConflicts(ctx, streams, ctrlClient, targets)
}

// warnKarpenterWeightConflicts compares the spec.weight of each user-managed
// Karpenter NodePool we are about to drain against the max spec.weight among
// Datadog-managed NodePools. When a user NodePool has a weight >= the Datadog
// max, freshly evicted pods may be re-scheduled onto a new node from the
// same user NodePool, defeating the migration.
func warnKarpenterWeightConflicts(ctx context.Context, streams genericclioptions.IOStreams, ctrlClient client.Client, targets []Target) {
	var userNPNames []string
	for _, t := range targets {
		if t.Manager == clusterinfo.NodeManagerKarpenter {
			userNPNames = append(userNPNames, t.Entity)
		}
	}
	if len(userNPNames) == 0 {
		return
	}

	list := &karpv1.NodePoolList{}
	if err := ctrlClient.List(ctx, list); err != nil {
		if meta.IsNoMatchError(err) {
			return
		}
		log.Printf("Warning: failed to list NodePools for pre-flight weight check: %v", err)
		return
	}

	var ddMaxWeight int32
	weightByName := make(map[string]int32, len(list.Items))
	for i := range list.Items {
		np := &list.Items[i]
		var w int32
		if np.Spec.Weight != nil {
			w = *np.Spec.Weight
		}
		weightByName[np.Name] = w
		if np.Labels[ddNodePoolCreatedLabel] == "true" && w > ddMaxWeight {
			ddMaxWeight = w
		}
	}

	for _, name := range userNPNames {
		w, ok := weightByName[name]
		if !ok {
			continue
		}
		if w >= ddMaxWeight {
			fmt.Fprintln(streams.ErrOut, color.YellowString(
				"⚠ NodePool %q has spec.weight=%d ≥ Datadog NodePool max weight=%d; evicted pods may be re-scheduled onto a freshly provisioned node from the same user NodePool. Consider raising the Datadog NodePool weight before retrying.",
				name, w, ddMaxWeight,
			))
		}
	}
}
