// Package cluster provides CLI commands for managing cluster-level autoscaling features,
// specifically Karpenter installation and configuration on EKS clusters.
package cluster

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/install"
)

// options provides information required by cluster command
type options struct {
	genericclioptions.IOStreams
	configFlags *genericclioptions.ConfigFlags
}

// newOptions provides an instance of options with default values
func newOptions(streams genericclioptions.IOStreams) *options {
	return &options{
		configFlags: genericclioptions.NewConfigFlags(false),
		IOStreams:   streams,
	}
}

// New provides a cobra command wrapping options for "cluster" sub command
func New(streams genericclioptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cluster [subcommand] [flags]",
		Short: "Manage cluster autoscaling",
	}

	cmd.AddCommand(install.New(streams))

	o := newOptions(streams)
	o.configFlags.AddFlags(cmd.Flags())

	return cmd
}
