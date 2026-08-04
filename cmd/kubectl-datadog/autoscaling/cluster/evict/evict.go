package evict

import (
	"context"
	"errors"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clients"
	"github.com/DataDog/datadog-operator/pkg/plugin/common"
)

var evictExample = `
  # evict every node group that is not Datadog-managed (cluster-autoscaler ASGs,
  # EKS managed node groups, user Karpenter NodePools, standalone EC2)
  %[1]s evict-legacy-nodes --all

  # evict a single ASG by name
  %[1]s evict-legacy-nodes --target=asg/my-legacy-asg

  # preview the actions without performing them
  %[1]s evict-legacy-nodes --all --dry-run
`

type options struct {
	genericclioptions.IOStreams
	common.Options
	args []string

	clusterName        string
	karpenterNamespace string
	all                bool
	targetSpecs        []string
	targets            []Target // populated by validate()
	skipCA             bool
	ensurePDBs         bool
	evictionTimeout    time.Duration
	nodeTimeout        time.Duration
	dryRun             bool
	yes                bool
	debug              bool
}

func newOptions(streams genericclioptions.IOStreams) *options {
	o := &options{
		IOStreams:       streams,
		ensurePDBs:      true,
		evictionTimeout: 5 * time.Minute,
		nodeTimeout:     15 * time.Minute,
	}
	o.SetConfigFlags()
	return o
}

// New returns the cobra command for `kubectl datadog autoscaling cluster evict-legacy-nodes`.
func New(streams genericclioptions.IOStreams) *cobra.Command {
	o := newOptions(streams)
	cmd := &cobra.Command{
		Use:          "evict-legacy-nodes",
		Short:        "Drain workloads from non-Datadog node groups onto Datadog-managed Karpenter NodePools",
		Example:      fmt.Sprintf(evictExample, "kubectl datadog autoscaling cluster"),
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.complete(c, args); err != nil {
				return err
			}
			if err := o.validate(); err != nil {
				return err
			}
			return o.run()
		},
	}

	cmd.Flags().StringVar(&o.clusterName, "cluster-name", "", "Name of the EKS cluster")
	cmd.Flags().StringVar(&o.karpenterNamespace, "karpenter-namespace", "", "Namespace where Karpenter is deployed (auto-detected when empty)")
	cmd.Flags().BoolVar(&o.all, "all", false, "Evict every node group that is not managed by Datadog")
	cmd.Flags().StringSliceVar(&o.targetSpecs, "target", nil, "Target a specific node group: <manager>/<name>, with <manager> one of asg, eksManagedNodeGroup, karpenter. Use `standalone` (no name) for standalone EC2 instances. Repeatable. Mutually exclusive with --all.")
	cmd.Flags().BoolVar(&o.skipCA, "skip-cluster-autoscaler", false, "Do not scale the cluster-autoscaler Deployment to 0 replicas as step 1")
	cmd.Flags().BoolVar(&o.ensurePDBs, "ensure-pdbs", true, "Create temporary PodDisruptionBudgets (maxUnavailable: 1) for workloads without one, and remove them at the end")
	cmd.Flags().DurationVar(&o.evictionTimeout, "eviction-timeout", 5*time.Minute, "Time budget per pod for the Eviction API to succeed before giving up (PDB-blocked pods)")
	cmd.Flags().DurationVar(&o.nodeTimeout, "node-timeout", 15*time.Minute, "Time budget per node for it to become empty after pods have been evicted")
	cmd.Flags().BoolVar(&o.dryRun, "dry-run", false, "Log the actions that would be taken without performing them")
	cmd.Flags().BoolVar(&o.yes, "yes", false, "Skip the confirmation prompt")
	cmd.Flags().BoolVar(&o.debug, "debug", false, "Enable debug logs")

	o.ConfigFlags.AddFlags(cmd.Flags())

	return cmd
}

func (o *options) complete(cmd *cobra.Command, args []string) error {
	o.args = args
	return o.Init(cmd)
}

func (o *options) validate() error {
	if len(o.args) > 0 {
		return errors.New("no positional arguments are allowed")
	}
	if o.all && len(o.targetSpecs) > 0 {
		return errors.New("--all and --target are mutually exclusive")
	}
	if !o.all && len(o.targetSpecs) == 0 {
		return errors.New("at least one of --all or --target must be provided")
	}
	if o.evictionTimeout <= 0 {
		return errors.New("--eviction-timeout must be positive")
	}
	if o.nodeTimeout <= 0 {
		return errors.New("--node-timeout must be positive")
	}
	var (
		parsed []Target
		errs   []error
	)
	for _, spec := range o.targetSpecs {
		if t, err := ParseTargetSpec(spec); err != nil {
			errs = append(errs, err)
		} else {
			parsed = append(parsed, t)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	o.targets = parsed
	return nil
}

func (o *options) run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	clusterName, err := clients.ResolveClusterName(o.ConfigFlags, o.clusterName)
	if err != nil {
		return err
	}

	return Run(ctx, o.IOStreams, o.ConfigFlags, o.Clientset, RunOptions{
		ClusterName:        clusterName,
		KarpenterNamespace: o.karpenterNamespace,
		All:                o.all,
		Targets:            o.targets,
		SkipCA:             o.skipCA,
		EnsurePDBs:         o.ensurePDBs,
		DryRun:             o.dryRun,
		Yes:                o.yes,
		EvictionTimeout:    o.evictionTimeout,
		NodeTimeout:        o.nodeTimeout,
		Debug:              o.debug,
	})
}
