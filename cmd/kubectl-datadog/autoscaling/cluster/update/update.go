// Package update provides the cobra command that refreshes a previously
// installed kubectl-datadog Karpenter deployment on an EKS cluster. It
// auto-detects immutable parameters (namespace, install-mode, fargate-subnets)
// from the dd-karpenter CloudFormation stack laid down at install time and
// refuses to touch a Karpenter installation it did not create.
package update

import (
	"context"
	"errors"
	"fmt"
	"os/signal"
	"slices"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/apply"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/aws"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clients"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/display"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/guess"
	"github.com/DataDog/datadog-operator/pkg/plugin/common"
)

var updateExample = `
  # update a previously installed kubectl-datadog Karpenter deployment
  %[1]s update
`

// options bundles flag-bound state. update only exposes flags for parameters
// that can change between the original install and a subsequent update; the
// immutable parameters (namespace, install-mode, fargate-subnets) are
// auto-detected from the dd-karpenter CFN stack and never accepted as flags.
type options struct {
	genericclioptions.IOStreams
	common.Options
	args []string

	clusterName              string
	karpenterVersion         string
	createKarpenterResources apply.CreateKarpenterResources
	inferenceMethod          apply.InferenceMethod
	debug                    bool
}

func newOptions(streams genericclioptions.IOStreams) *options {
	o := &options{
		IOStreams: streams,
		// Default `none` (vs install's `all`): a flagless update preserves
		// any hand-edits to the EC2NodeClass / NodePool resources. Pass
		// --create-karpenter-resources=all to regenerate them.
		createKarpenterResources: apply.CreateKarpenterResourcesNone,
		inferenceMethod:          apply.InferenceMethodNodeGroups,
	}
	o.SetConfigFlags()
	return o
}

// New returns the cobra command for `kubectl datadog autoscaling cluster update`.
func New(streams genericclioptions.IOStreams) *cobra.Command {
	o := newOptions(streams)
	cmd := &cobra.Command{
		Use:          "update",
		Short:        "Update an existing kubectl-datadog Karpenter installation on an EKS cluster",
		Example:      fmt.Sprintf(updateExample, "kubectl datadog autoscaling cluster"),
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
	cmd.Flags().StringVar(&o.karpenterVersion, "karpenter-version", "", "Version of Karpenter to upgrade to (default to latest)")
	cmd.Flags().Var(&o.createKarpenterResources, "create-karpenter-resources", "Which Karpenter resources to (re-)create: none (default), ec2nodeclass, all")
	cmd.Flags().Var(&o.inferenceMethod, "inference-method", "Method to infer EC2NodeClass and NodePool properties when --create-karpenter-resources is set: nodes, nodegroups")
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
		return errors.New("no arguments are allowed")
	}

	if !slices.Contains([]apply.CreateKarpenterResources{apply.CreateKarpenterResourcesNone, apply.CreateKarpenterResourcesEC2NodeClass, apply.CreateKarpenterResourcesAll}, o.createKarpenterResources) {
		return errors.New("create-karpenter-resources must be one of none, ec2nodeclass or all")
	}

	if !slices.Contains([]apply.InferenceMethod{apply.InferenceMethodNodes, apply.InferenceMethodNodeGroups}, o.inferenceMethod) {
		return errors.New("inference-method must be one of nodes or nodegroups")
	}

	return nil
}

func (o *options) run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	clusterName, err := clients.ResolveClusterName(o.ConfigFlags, o.clusterName)
	if err != nil {
		return err
	}

	// Refuse early if no kubectl-datadog Karpenter is installed, or if a
	// Karpenter installed by a different tool occupies the cluster — we
	// never modify what we did not install. Run this check before AWS
	// client setup so a flaky AWS does not mask the "run install first"
	// signal the user actually needs.
	k, err := guess.FindKarpenterInstallation(ctx, o.Clientset)
	if err != nil {
		return fmt.Errorf("failed to check for an existing Karpenter installation: %w", err)
	}
	if k == nil {
		return fmt.Errorf("no Karpenter installation found on cluster %s; run 'kubectl datadog autoscaling cluster install' first", clusterName)
	}
	if !k.IsOwn() {
		display.PrintBox(o.Out,
			"A Karpenter installation managed by another tool was found on cluster "+clusterName+":",
			"Deployment "+k.Namespace+"/"+k.Name+".",
			"",
			"kubectl-datadog will not modify a Karpenter installation it did not create.",
		)
		return fmt.Errorf("refusing to update Karpenter installation %s/%s not created by kubectl-datadog", k.Namespace, k.Name)
	}

	cli, err := clients.Build(ctx, o.ConfigFlags, o.Clientset)
	if err != nil {
		return fmt.Errorf("failed to build clients: %w", err)
	}

	if err = clients.ValidateAWSAccountConsistency(ctx, cli, clusterName, o.ConfigFlags); err != nil {
		return err
	}

	ddStackName := apply.DDKarpenterStackName(clusterName)
	ddStack, err := aws.GetStack(ctx, cli.CloudFormation, ddStackName)
	if err != nil {
		return fmt.Errorf("failed to read CloudFormation stack %s: %w", ddStackName, err)
	}
	if ddStack == nil {
		return fmt.Errorf("Karpenter Deployment %s/%s exists but its CloudFormation stack %s was not found; install state is inconsistent — re-run 'install' to recover",
			k.Namespace, k.Name, ddStackName)
	}

	opts, err := o.resolveOptions(clusterName, ddStack)
	if err != nil {
		return err
	}

	// apply.Run treats a Deployment in a different namespace than what we
	// pass it as a foreign install (no-op success). For update we want to
	// surface the inconsistency loudly instead.
	if k.Namespace != opts.KarpenterNamespace {
		return fmt.Errorf("inconsistent state: Karpenter Deployment %s/%s does not match CloudFormation stack %s namespace %s — re-run 'install' to recover",
			k.Namespace, k.Name, ddStackName, opts.KarpenterNamespace)
	}

	return apply.Run(ctx, o.IOStreams, o.ConfigFlags, o.Clientset, opts)
}

// resolveOptions builds the apply.RunOptions to pass to apply.Run, using the
// dd-karpenter CFN stack as the source of truth for the immutable parameters
// (namespace, install-mode, fargate-subnets). update never accepts these as
// flags — the function reads them straight from the stack. Mutable values
// (KarpenterVersion, CreateKarpenterResources, InferenceMethod, Debug) come
// from the user-provided flags.
func (o *options) resolveOptions(clusterName string, ddStack *aws.Stack) (apply.RunOptions, error) {
	params := ddStack.ParameterMap()

	detectedNamespace := params["KarpenterNamespace"]
	if detectedNamespace == "" {
		return apply.RunOptions{}, fmt.Errorf("CloudFormation stack %s has no KarpenterNamespace parameter; install state is inconsistent",
			apply.DDKarpenterStackName(clusterName))
	}

	detectedMode := apply.DetectedInstallMode(ddStack)
	// Reject a corrupt or unknown tag here so we surface a clear error before
	// apply.Run mutates the mode-independent CFN stack and only then crashes
	// on the unsupported mode in its switch statement.
	switch detectedMode {
	case apply.InstallModeFargate, apply.InstallModeExistingNodes:
	default:
		return apply.RunOptions{}, fmt.Errorf("CloudFormation stack %s has unsupported install-mode tag %q",
			apply.DDKarpenterStackName(clusterName), detectedMode)
	}

	var detectedSubnets []string
	if detectedMode == apply.InstallModeFargate {
		if raw := params["FargateSubnets"]; raw != "" {
			detectedSubnets = strings.Split(raw, ",")
			slices.Sort(detectedSubnets)
		}
	}

	return apply.RunOptions{
		ClusterName:              clusterName,
		KarpenterNamespace:       detectedNamespace,
		KarpenterVersion:         o.karpenterVersion,
		InstallMode:              detectedMode,
		FargateSubnets:           detectedSubnets,
		CreateKarpenterResources: o.createKarpenterResources,
		InferenceMethod:          o.inferenceMethod,
		Debug:                    o.debug,
		ActionLabel:              "Updating",
	}, nil
}
