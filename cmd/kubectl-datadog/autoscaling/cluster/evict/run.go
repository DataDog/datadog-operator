package evict

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clients"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clusterinfo"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/display"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/karpenter"
)

type RunOptions struct {
	ClusterName        string
	KarpenterNamespace string // override; auto-detected when empty
	All                bool
	Targets            []Target // parsed --target=<type>/<name>
	SkipCA             bool
	EnsurePDBs         bool
	DryRun             bool
	Yes                bool
	EvictionTimeout    time.Duration
	NodeTimeout        time.Duration
	Debug              bool
}

// Run is the orchestrator entry point. Idempotent: a second invocation on a
// cluster already migrated returns success without changes. Crash-safe: a
// killed run leaves no permanently corrupting state — re-running cleans up.
func Run(ctx context.Context, streams genericclioptions.IOStreams, configFlags *genericclioptions.ConfigFlags, clientset *kubernetes.Clientset, opts RunOptions) error {
	log.SetOutput(streams.ErrOut)
	ctrl.SetLogger(zap.New(zap.UseDevMode(false), zap.WriteTo(streams.ErrOut)))

	cli, err := clients.Build(ctx, configFlags, clientset)
	if err != nil {
		return fmt.Errorf("failed to build clients: %w", err)
	}
	if err = clients.ValidateAWSAccountConsistency(ctx, cli, opts.ClusterName, configFlags); err != nil {
		return err
	}
	k, err := karpenter.FindInstallation(ctx, clientset)
	if err != nil {
		return fmt.Errorf("failed to check for an existing Karpenter installation: %w", err)
	}
	if k == nil {
		return errors.New("Karpenter is not installed on this cluster; run `kubectl datadog autoscaling cluster install` first")
	}
	namespace := opts.KarpenterNamespace
	if namespace == "" {
		namespace = k.Namespace
	}

	display.PrintBox(streams.Out, "Evicting legacy nodes from cluster "+opts.ClusterName+".")

	info, err := classify(ctx, clientset, cli, opts.ClusterName)
	if err != nil {
		return fmt.Errorf("failed to classify cluster nodes: %w", err)
	}
	if info.Autoscaling.EKSAutoMode.Enabled {
		return errors.New("EKS auto-mode is enabled on this cluster; eviction is not applicable (auto-mode manages its own node lifecycle)")
	}
	// Skip the ConfigMap write on dry-run: a preview must not mutate the
	// cluster (and must not require write RBAC on the Karpenter namespace).
	if opts.DryRun {
		log.Printf("[dry-run] would persist cluster-info to ConfigMap %s/%s", namespace, clusterinfo.ConfigMapName)
	} else if err = clusterinfo.Persist(ctx, cli.K8sClient, namespace, info); err != nil {
		log.Printf("Warning: failed to persist updated cluster-info ConfigMap: %v", err)
	}

	// Before any destructive work, verify there is at least one Datadog-managed
	// NodePool to receive the evicted pods. Without this guard, scaling legacy
	// capacity to 0 would leave the cluster with no working capacity (e.g. when
	// `install` was run with --create-karpenter-resources=none, or the user
	// deleted the Datadog NodePools out of band).
	if !hasDatadogManagedNodePool(info) {
		return errors.New("no Datadog-managed Karpenter NodePool found in the cluster snapshot; legacy nodes would have no destination to migrate to. Run `kubectl datadog autoscaling cluster install` (or `update`) to create the Datadog NodePool first")
	}

	targets, err := BuildPlan(info, opts.All, opts.Targets)
	if err != nil {
		return err
	}
	if len(targets) == 0 {
		// A previous run may have been interrupted (e.g. an EKS managed node
		// group timed out, leaving its temporary PDBs in place for a later
		// rerun). By the time that rerun finds nothing left to evict, those
		// leaked PDBs would otherwise persist indefinitely with
		// maxUnavailable: 1 and throttle future rollouts/disruptions, so
		// reclaim anything left behind before the no-op exit.
		if opts.EnsurePDBs {
			if err := cleanupTempPDBs(ctx, cli.K8sClient, opts.DryRun); err != nil {
				log.Printf("Warning: failed to cleanup leftover temporary PDBs: %v", err)
			}
		}
		display.PrintBox(streams.Out, "Nothing to evict — the cluster is already on Datadog-managed Karpenter NodePools.")
		return nil
	}

	warnKarpenterWeightConflicts(ctx, streams, cli.K8sClient)
	printPlan(streams, info, targets, !opts.SkipCA, opts.EnsurePDBs)

	if !opts.Yes && !opts.DryRun {
		if !promptConfirmation(streams) {
			return nil
		}
	}

	// Step 1: cluster-autoscaler scale-down — first, so it does not undo our
	// work by provisioning new legacy nodes while we drain.
	if !opts.SkipCA {
		if err := scaleDownClusterAutoscaler(ctx, clientset, info.Autoscaling.ClusterAutoscaler, opts.DryRun); err != nil {
			return fmt.Errorf("failed to scale down cluster-autoscaler: %w", err)
		}
	}

	// Step 2: ensure temporary PDBs. On error, attempt cleanup of whatever
	// landed before the failure — the temp PDBs carry our labels so the
	// label-based cleanup picks them up. On a SIGINT mid-flight the temp
	// PDBs may leak; a subsequent run cleans them up via the same label.
	if opts.EnsurePDBs {
		if err := ensureTempPDBs(ctx, clientset, cli.K8sClient, targets, opts.DryRun); err != nil {
			if cleanupErr := cleanupTempPDBs(ctx, cli.K8sClient, opts.DryRun); cleanupErr != nil {
				log.Printf("Warning: failed to cleanup partial temporary PDBs: %v", cleanupErr)
			}
			return fmt.Errorf("failed to ensure temporary PDBs: %w", err)
		}
	}

	drainOpts := nodeDrainOptions{
		DryRun:          opts.DryRun,
		EvictionTimeout: opts.EvictionTimeout,
		NodeTimeout:     opts.NodeTimeout,
		PollInterval:    2 * time.Second,
	}
	evictor := func(c context.Context, t Target, d nodeDrainOptions) error {
		return evictTarget(c, clientset, cli, opts.ClusterName, t, d)
	}
	result := evictAllTargets(ctx, targets, drainOpts, evictor)
	errs := result.Errors

	// Step 4: cleanup temporary PDBs explicitly here — keeping it inline
	// (rather than deferred) means the "Deleted temporary PDB …" log lines
	// appear BEFORE the final success/failure summary, so the user does not
	// experience an apparent hang after seeing the "✅" box.
	//
	// When at least one EKS managed node group did not finish draining
	// within the timeout, we skip the cleanup: EKS may still be evicting
	// pods on those nodes, and dropping the temp PDBs now would leave
	// cross-type workloads unprotected mid-drain. The label-based cleanup
	// on a subsequent run picks the PDBs up once EKS converges.
	switch {
	case !opts.EnsurePDBs:
	case result.EKSDrainIncomplete:
		log.Printf("Note: at least one EKS managed node group did not finish draining within --node-timeout; temporary PDBs left in place so EKS can finish safely. Re-run the command once `aws eks describe-nodegroup` reports the group at 0 nodes — the next run will clean them up.")
	default:
		if err := cleanupTempPDBs(ctx, cli.K8sClient, opts.DryRun); err != nil {
			log.Printf("Warning: failed to cleanup temporary PDBs: %v", err)
		}
	}

	// Re-classify and persist to reflect the final state. Skipped on dry-run
	// for the same reason as the pre-eviction Persist above.
	if opts.DryRun {
		log.Printf("[dry-run] would re-classify and persist post-eviction cluster-info to ConfigMap %s/%s", namespace, clusterinfo.ConfigMapName)
	} else if final, classifyErr := classify(ctx, clientset, cli, opts.ClusterName); classifyErr == nil {
		if err := clusterinfo.Persist(ctx, cli.K8sClient, namespace, final); err != nil {
			log.Printf("Warning: failed to persist post-eviction cluster-info: %v", err)
		}
	} else {
		log.Printf("Warning: failed to re-classify post-eviction: %v", classifyErr)
	}

	if len(errs) > 0 {
		return fmt.Errorf("eviction completed with %d error(s):\n%w", len(errs), errors.Join(errs...))
	}
	display.PrintBox(streams.Out, "✅ Legacy nodes drained from cluster "+opts.ClusterName+".")
	return nil
}

// classify wraps clusterinfo.Classify with the call-site boilerplate shared by
// the pre- and post-eviction snapshots.
func classify(ctx context.Context, clientset *kubernetes.Clientset, cli *clients.Clients, clusterName string) (*clusterinfo.ClusterInfo, error) {
	return clusterinfo.Classify(ctx, clusterinfo.ClassifyInput{
		K8sClient:   clientset,
		CtrlClient:  cli.K8sClient,
		Autoscaling: cli.Autoscaling,
		EKS:         cli.EKS,
		Discovery:   clientset.Discovery(),
		ClusterName: clusterName,
	})
}

// evictResult is the structured outcome of evictAllTargets. Errors is the
// aggregated per-target error list (empty when everything succeeded).
// EKSDrainIncomplete is true when at least one EKS managed node group target
// returned an error WRAPPING errEKSDrainIncomplete — the EKS drain may still
// be in progress, in which case the caller must NOT cleanup the temporary
// PDBs (EKS would then over-disrupt the still-running workloads).
type evictResult struct {
	Errors             []error
	EKSDrainIncomplete bool
}

// targetEvictor is the closure injected into evictAllTargets by Run. Pulling
// the dispatch out as an interface-like function makes the orchestrator
// unit-testable without faking the entire clients.Clients struct.
type targetEvictor func(ctx context.Context, t Target, drainOpts nodeDrainOptions) error

// evictAllTargets processes targets sequentially, in the order BuildPlan
// produced. Errors are accumulated; a failing target does not abort the
// others — every issue surfaces at the end so a partial migration is fully
// visible. Sequential execution keeps logs linear, bounds the eviction
// pressure on the apiserver, and matches the synchronous per-target shape
// (every evictor — including EKS MNG — already blocks until its drain
// completes).
func evictAllTargets(ctx context.Context, targets []Target, drainOpts nodeDrainOptions, evictor targetEvictor) evictResult {
	var (
		errs               []error
		eksDrainIncomplete bool
	)
	for _, t := range targets {
		err := evictor(ctx, t, drainOpts)
		if err == nil {
			continue
		}
		errs = append(errs, fmt.Errorf("%s/%s: %w", t.Manager, t.Entity, err))
		// Only treat the drain as "still in progress" when EKS accepted the
		// scaling change but observation of the drain failed or timed out.
		// A failed UpdateNodegroupConfig means EKS never started draining,
		// so cleanup is safe.
		if errors.Is(err, errEKSDrainIncomplete) {
			eksDrainIncomplete = true
		}
	}
	return evictResult{Errors: errs, EKSDrainIncomplete: eksDrainIncomplete}
}

// evictTarget dispatches a single Target to the manager-specific evictor.
// Wrapped into a closure inside Run so evictAllTargets can be exercised with
// a fake evictor in tests.
func evictTarget(ctx context.Context, clientset kubernetes.Interface, cli *clients.Clients, clusterName string, t Target, drainOpts nodeDrainOptions) error {
	switch t.Manager {
	case clusterinfo.NodeManagerASG:
		return evictASG(ctx, clientset, cli.Autoscaling, t.Entity, t.Nodes, drainOpts)
	case clusterinfo.NodeManagerEKSManagedNodeGroup:
		return evictEKSManagedNodeGroup(ctx, cli.EKS, clientset, clusterName, t.Entity, drainOpts)
	case clusterinfo.NodeManagerKarpenter:
		return evictKarpenterUserNodePool(ctx, clientset, t.Entity, t.Nodes, drainOpts)
	case clusterinfo.NodeManagerStandalone:
		return evictStandalone(ctx, clientset, cli.EC2, t.Nodes, drainOpts)
	default:
		return fmt.Errorf("unsupported manager %q", t.Manager)
	}
}
