// Package root contains root plugin command logic.
package root

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/extendeddaemonset/cmd/check-eds/upgrade"
)

// Options provides information required to manage Root.
type Options struct {
	configFlags *genericclioptions.ConfigFlags
	genericclioptions.IOStreams
}

// NewRootOptions provides an instance of Options with default values.
func NewRootOptions(streams genericclioptions.IOStreams) *Options {
	return &Options{
		configFlags: genericclioptions.NewConfigFlags(false),

		IOStreams: streams,
	}
}

// NewCmdRoot provides a cobra command wrapping Options.
func NewCmdRoot(streams genericclioptions.IOStreams) *cobra.Command {
	o := NewRootOptions(streams)

	cmd := &cobra.Command{
		Use: "check-eds [subcommand] [flags]",
	}

	o.configFlags.AddFlags(cmd.Flags())

	cmd.AddCommand(upgrade.NewCmdUpgrade(streams))

	return cmd
}

// Complete sets all information required for processing the command.
func (o *Options) Complete(cmd *cobra.Command, args []string) error {
	return nil
}

// Root ensures that all required arguments and flag values are provided.
func (o *Options) Root() error {
	return nil
}

// Run use to run the command.
func (o *Options) Run() error {
	return nil
}
