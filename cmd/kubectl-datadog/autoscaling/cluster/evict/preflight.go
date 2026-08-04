package evict

import (
	"context"
	"fmt"
	"log"

	"github.com/fatih/color"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

// ddNodePoolCreatedLabel is the label every Datadog autoscaling product (this
// CLI and the cluster agent) puts on the Karpenter NodePools it creates. Used
// here to identify "Datadog-side" NodePools when comparing weights against
// user-side ones.
const ddNodePoolCreatedLabel = "autoscaling.datadoghq.com/created"

// warnKarpenterWeightConflicts compares the spec.weight of every user-managed
// Karpenter NodePool in the cluster against the max spec.weight among
// Datadog-managed NodePools. When a user NodePool has a weight >= the Datadog
// max, freshly evicted pods may be re-scheduled onto a new node from a user
// NodePool, defeating the migration.
//
// The check spans all NodePools, not just the eviction targets: Karpenter
// arbitrates provisioning across every NodePool by weight, so a high-weight
// user NodePool can capture evicted pods even when it is not itself a target
// (e.g. the run targets an ASG, or the user NodePool currently has no nodes
// and is therefore absent from the eviction plan).
//
// This is a non-blocking pre-flight warning: it flags a situation that may
// surprise the operator after the fact but does not prevent eviction. Pure
// best-effort — failures here are logged but do not abort the run.
func warnKarpenterWeightConflicts(ctx context.Context, streams genericclioptions.IOStreams, ctrlClient client.Client) {
	list := &karpv1.NodePoolList{}
	if err := ctrlClient.List(ctx, list); err != nil {
		if meta.IsNoMatchError(err) {
			return
		}
		log.Printf("Warning: failed to list NodePools for pre-flight weight check: %v", err)
		return
	}

	ddMaxWeight := int32(-1)
	for i := range list.Items {
		np := &list.Items[i]
		if np.Labels[ddNodePoolCreatedLabel] != "true" {
			continue
		}
		if w := lo.FromPtr(np.Spec.Weight); w > ddMaxWeight {
			ddMaxWeight = w
		}
	}

	if ddMaxWeight < 0 {
		return
	}

	for i := range list.Items {
		np := &list.Items[i]
		if np.Labels[ddNodePoolCreatedLabel] == "true" {
			continue
		}
		if w := lo.FromPtr(np.Spec.Weight); w >= ddMaxWeight {
			fmt.Fprintln(streams.ErrOut, color.YellowString(
				"⚠ user NodePool %q has spec.weight=%d ≥ Datadog NodePool max weight=%d; evicted pods may be re-scheduled onto a freshly provisioned node from a user NodePool.",
				np.Name, w, ddMaxWeight,
			))
		}
	}
}
