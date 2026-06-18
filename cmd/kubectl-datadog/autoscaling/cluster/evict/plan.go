// Package evict implements the `kubectl datadog autoscaling cluster
// evict-legacy-nodes` subcommand. It drains workloads from nodes that are not
// managed by the Datadog Karpenter NodePools (cluster-autoscaler ASGs, EKS
// managed node groups, user-created Karpenter NodePools, standalone EC2
// instances) and scales those node groups to zero so that future capacity is
// provisioned exclusively by the NodePools created by
// `kubectl datadog autoscaling cluster install`.
package evict

import (
	"errors"
	"fmt"
	"strings"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clusterinfo"
)

// Target identifies a single node-management entity to evict. The Entity field
// is the bucket name within `ClusterInfo.NodeManagement[Manager]` — an ASG
// name, an EKS managed node group name, a Karpenter NodePool name, or the
// empty string for the standalone bucket.
type Target struct {
	Manager clusterinfo.NodeManager
	Entity  string
	Nodes   []string
}

// ParseTargetSpec parses a CLI `--target` value into a Target with an empty
// Nodes slice (Nodes are filled in by BuildPlan from the cluster snapshot).
//
// Format: `<manager>/<entity>`, with the entity omitted for the standalone
// manager. Fargate and unknown are rejected here — their migration is out of
// scope of this command.
func ParseTargetSpec(raw string) (Target, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Target{}, errors.New("--target value cannot be empty")
	}
	mgrStr, entity, hasSlash := strings.Cut(raw, "/")
	mgr := clusterinfo.NodeManager(mgrStr)
	switch mgr {
	case clusterinfo.NodeManagerASG,
		clusterinfo.NodeManagerEKSManagedNodeGroup,
		clusterinfo.NodeManagerKarpenter:
		if !hasSlash || entity == "" {
			return Target{}, fmt.Errorf("--target=%s requires a name: %s/<name>", mgrStr, mgrStr)
		}
		return Target{Manager: mgr, Entity: entity}, nil
	case clusterinfo.NodeManagerStandalone:
		if hasSlash && entity != "" {
			return Target{}, fmt.Errorf("--target=standalone does not take a name (got %q)", entity)
		}
		return Target{Manager: mgr, Entity: ""}, nil
	case clusterinfo.NodeManagerFargate:
		return Target{}, errors.New("--target=fargate is not supported: manage Fargate profiles directly via AWS")
	case clusterinfo.NodeManagerUnknown:
		return Target{}, errors.New("--target=unknown is not supported: nodes with unknown providers cannot be migrated automatically")
	default:
		return Target{}, fmt.Errorf("--target=%s: unknown manager type (supported: asg, eksManagedNodeGroup, karpenter, standalone)", mgrStr)
	}
}

// BuildPlan turns user intent (--all or one or more --target flags) into the
// list of Targets the orchestrator will evict. Entries flagged ManagedByDatadog
// are filtered out (protected by definition); fargate and unknown managers are
// skipped in --all mode (their lifecycle is handled elsewhere or unknown).
//
// In targeted mode, user input errors are collected and returned together so
// the user can fix all mistakes in a single iteration.
func BuildPlan(info *clusterinfo.ClusterInfo, all bool, specs []Target) ([]Target, error) {
	panic("TODO: BuildPlan — implemented in PR #2")
}

// hasDatadogManagedNodePool reports whether the snapshot lists at least one
// Karpenter NodePool flagged as Datadog-managed. The orchestrator uses this as
// a pre-flight guard: if no destination NodePool exists, scaling legacy
// capacity to zero would leave the cluster with no working capacity.
func hasDatadogManagedNodePool(info *clusterinfo.ClusterInfo) bool {
	panic("TODO: hasDatadogManagedNodePool — implemented in PR #2")
}
