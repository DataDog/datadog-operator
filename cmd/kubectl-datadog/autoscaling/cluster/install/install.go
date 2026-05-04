// Package install provides functionality to install and configure Karpenter
// autoscaling on EKS clusters, including CloudFormation stack creation,
// Helm chart deployment, and resource configuration.
package install

import (
	"errors"
	"fmt"
	"slices"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/pkg/plugin/common"
)

var (
	clusterName              string
	karpenterNamespace       string
	karpenterVersion         string
	installMode              = InstallModeFargate
	fargateSubnets           []string
	createKarpenterResources = CreateKarpenterResourcesAll
	inferenceMethod          = InferenceMethodNodeGroups
	debug                    bool
	installExample           = `
  # install autoscaling
  %[1]s install
`
)

type options struct {
	genericclioptions.IOStreams
	common.Options
	args []string
}

func newOptions(streams genericclioptions.IOStreams) *options {
	o := &options{
		IOStreams: streams,
	}
	o.SetConfigFlags()
	return o
}

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

			return o.run(c)
		},
	}

	cmd.Flags().StringVar(&clusterName, "cluster-name", "", "Name of the EKS cluster")
	cmd.Flags().StringVar(&karpenterNamespace, "karpenter-namespace", "dd-karpenter", "Name of the Kubernetes namespace to deploy Karpenter into")
	cmd.Flags().StringVar(&karpenterVersion, "karpenter-version", "", "Version of Karpenter to install (default to latest)")
	cmd.Flags().Var(&installMode, "install-mode", "How to run the Karpenter controller: fargate (on dedicated Fargate nodes, default) or existing-nodes (on existing cluster nodes)")
	cmd.Flags().StringSliceVar(&fargateSubnets, "fargate-subnets", nil, "Override auto-discovery of private subnets for the Fargate profile (comma-separated subnet IDs). Only used when --install-mode=fargate.")
	cmd.Flags().Var(&createKarpenterResources, "create-karpenter-resources", "Which Karpenter resources to create: none, ec2nodeclass, all (default: all)")
	cmd.Flags().Var(&inferenceMethod, "inference-method", "Method to infer EC2NodeClass and NodePool properties: nodes, nodegroups")
	cmd.Flags().BoolVar(&debug, "debug", false, "Enable debug logs")

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

	if !slices.Contains([]InstallMode{InstallModeFargate, InstallModeExistingNodes}, installMode) {
		return errors.New("install-mode must be one of fargate or existing-nodes")
	}

	if len(fargateSubnets) > 0 && installMode != InstallModeFargate {
		return errors.New("--fargate-subnets can only be used with --install-mode=fargate")
	}

	if !slices.Contains([]CreateKarpenterResources{CreateKarpenterResourcesNone, CreateKarpenterResourcesEC2NodeClass, CreateKarpenterResourcesAll}, createKarpenterResources) {
		return errors.New("create-karpenter-resources must be one of none, ec2nodeclass or all")
	}

	if !slices.Contains([]InferenceMethod{InferenceMethodNodes, InferenceMethodNodeGroups}, inferenceMethod) {
		return errors.New("inference-method must be one of nodes or nodegroups")
	}

	return nil
}
