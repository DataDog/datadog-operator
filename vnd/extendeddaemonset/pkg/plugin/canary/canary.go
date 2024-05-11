package canary

import (
	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// NewCmdCanary provides a cobra command to control canary deployments.
func NewCmdCanary(streams genericclioptions.IOStreams) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "canary [subcommand] [flags]",
		Short: "control ExtendedDaemonSet canary deployment",
	}

	cmd.AddCommand(newCmdValidate(streams))
	cmd.AddCommand(newCmdPause(streams))
	cmd.AddCommand(newCmdUnpause(streams))
	cmd.AddCommand(newCmdFail(streams))
	cmd.AddCommand(newCmdPods(streams))

	return cmd
}
