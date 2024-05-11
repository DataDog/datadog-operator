package main

import (
	"os"

	"github.com/spf13/pflag"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/extendeddaemonset/cmd/check-eds/root"
)

// edscheck is use to validate a extendeddaemonset deployment
// main usecase it to validate a helm deployment

func main() {
	flags := pflag.NewFlagSet("eds-checks", pflag.ExitOnError)
	pflag.CommandLine = flags

	root := root.NewCmdRoot(genericclioptions.IOStreams{In: os.Stdin, Out: os.Stdout, ErrOut: os.Stderr})
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
