// Package install provides the cobra command that installs Karpenter on an
// EKS cluster. The command is a thin wrapper around the convergence logic in
// the apply package — install binds CLI flags, validates them, and delegates
// the actual deployment work to apply.Run.
package install

import (
	"context"
	"errors"
	"fmt"
	"os/signal"
	"slices"
	"syscall"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/apply"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/clients"
	"github.com/DataDog/datadog-operator/pkg/plugin/common"
)

var installExample = `
  # install autoscaling
  %[1]s install
`

type options struct {
	genericclioptions.IOStreams
	common.Options
	args []string

	clusterName              string
	karpenterNamespace       string
	karpenterVersion         string
	installMode              installMode
	fargateSubnets           []string
	createKarpenterResources apply.CreateKarpenterResources
	inferenceMethod          apply.InferenceMethod
	debug                    bool
}

func newOptions(streams genericclioptions.IOStreams) *options {
	o := &options{
		IOStreams:                streams,
		installMode:              installMode(apply.InstallModeFargate),
		createKarpenterResources: apply.CreateKarpenterResourcesAll,
		inferenceMethod:          apply.InferenceMethodNodeGroups,
	}
	o.SetConfigFlags()
	return o
}

// New returns the cobra command for `kubectl datadog autoscaling cluster install`.
func New(streams genericclioptions.IOStreams) *cobra.Command {
	o := newOptions(streams)
	cmd := &cobra.Command{
		Use:          "install",
		Short:        "Install autoscaling on an EKS cluster",
		Example:      fmt.Sprintf(installExample, "kubectl datadog autoscaling cluster"),
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
	cmd.Flags().StringVar(&o.karpenterNamespace, "karpenter-namespace", "dd-karpenter", "Name of the Kubernetes namespace to deploy Karpenter into")
	cmd.Flags().StringVar(&o.karpenterVersion, "karpenter-version", "", "Version of Karpenter to install (default to latest)")
	cmd.Flags().Var(&o.installMode, "install-mode", "How to run the Karpenter controller: fargate (on dedicated Fargate nodes, default) or existing-nodes (on existing cluster nodes)")
	cmd.Flags().StringSliceVar(&o.fargateSubnets, "fargate-subnets", nil, "Override auto-discovery of private subnets for the Fargate profile (comma-separated subnet IDs). Only used when --install-mode=fargate.")
	cmd.Flags().Var(&o.createKarpenterResources, "create-karpenter-resources", "Which Karpenter resources to create: none, ec2nodeclass, all (default: all)")
	cmd.Flags().Var(&o.inferenceMethod, "inference-method", "Method to infer EC2NodeClass and NodePool properties: nodes, nodegroups")
	cmd.Flags().BoolVar(&o.debug, "debug", false, "Enable debug logs")

	o.ConfigFlags.AddFlags(cmd.Flags())

	return cmd
}

// complete sets all information required for processing the command.
func (o *options) complete(cmd *cobra.Command, args []string) error {
	o.args = args
	return o.Init(cmd)
}

// validate ensures that all required arguments and flag values are provided.
func (o *options) validate() error {
	if len(o.args) > 0 {
		return errors.New("no arguments are allowed")
	}

	mode := apply.InstallMode(o.installMode)
	if !slices.Contains([]apply.InstallMode{apply.InstallModeFargate, apply.InstallModeExistingNodes}, mode) {
		return errors.New("install-mode must be one of fargate or existing-nodes")
	}

	if len(o.fargateSubnets) > 0 && mode != apply.InstallModeFargate {
		return errors.New("--fargate-subnets can only be used with --install-mode=fargate")
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

	return apply.Run(ctx, o.IOStreams, o.ConfigFlags, o.Clientset, apply.RunOptions{
		ClusterName:              clusterName,
		KarpenterNamespace:       o.karpenterNamespace,
		KarpenterVersion:         o.karpenterVersion,
		InstallMode:              apply.InstallMode(o.installMode),
		FargateSubnets:           o.fargateSubnets,
		CreateKarpenterResources: o.createKarpenterResources,
		InferenceMethod:          o.inferenceMethod,
		Debug:                    o.debug,
		ActionLabel:              "Installing",
	})
}
