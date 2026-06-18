package evict

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/samber/lo"

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

func BuildPlan(info *clusterinfo.ClusterInfo, all bool, specs []Target) ([]Target, error) {
	if info == nil {
		return nil, errors.New("cluster info is nil")
	}
	if all {
		return buildAllPlan(info), nil
	}
	return buildTargetedPlan(info, specs)
}

func buildAllPlan(info *clusterinfo.ClusterInfo) []Target {
	var targets []Target
	for _, mgr := range []clusterinfo.NodeManager{
		clusterinfo.NodeManagerKarpenter,
		clusterinfo.NodeManagerEKSManagedNodeGroup,
		clusterinfo.NodeManagerASG,
		clusterinfo.NodeManagerStandalone,
	} {
		bucket, ok := info.NodeManagement[mgr]
		if !ok {
			continue
		}
		targets = append(targets, lo.FilterMap(slices.Sorted(maps.Keys(bucket)), func(name string, _ int) (Target, bool) {
			entry := bucket[name]
			if entry.ManagedByDatadog {
				return Target{}, false
			}
			return Target{
				Manager: mgr,
				Entity:  name,
				Nodes:   entry.Nodes,
			}, true
		})...)
	}
	return targets
}

func buildTargetedPlan(info *clusterinfo.ClusterInfo, specs []Target) ([]Target, error) {
	if len(specs) == 0 {
		return nil, errors.New("at least one --target must be provided, or --all")
	}
	var (
		targets []Target
		errs    []error
	)
	for _, t := range specs {
		bucket, ok := info.NodeManagement[t.Manager]
		if !ok {
			errs = append(errs, fmt.Errorf("--target=%s/%s: no %s entities found in the cluster snapshot", t.Manager, t.Entity, t.Manager))
			continue
		}
		entry, ok := bucket[t.Entity]
		if !ok {
			errs = append(errs, fmt.Errorf("--target=%s/%s: entity not found in the cluster snapshot", t.Manager, t.Entity))
			continue
		}
		if entry.ManagedByDatadog {
			errs = append(errs, fmt.Errorf("--target=%s/%s: this entity is managed by Datadog and cannot be evicted", t.Manager, t.Entity))
			continue
		}
		targets = append(targets, Target{
			Manager: t.Manager,
			Entity:  t.Entity,
			Nodes:   entry.Nodes,
		})
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return targets, nil
}

func hasDatadogManagedNodePool(info *clusterinfo.ClusterInfo) bool {
	for _, entry := range info.NodeManagement[clusterinfo.NodeManagerKarpenter] {
		if entry.ManagedByDatadog {
			return true
		}
	}
	return false
}
