// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// Package pods contains the "kubectl eds pod" command logic.
package pods

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/extendeddaemonset/pkg/plugin/common"
)

var (
	podsExample = `
	# list the not ready pods managed by the EDS foo
	%[1]s pods --select not-ready foo

	# list the canary pods managed by the EDS foo
	%[1]s pods --select canary foo
`
	selectOpt string
)

const (
	canary   = "canary"
	notReady = "not-ready"
)

// podsOptions provides information required to manage ExtendedDaemonSet.
type podsOptions struct {
	client client.Client
	genericclioptions.IOStreams
	configFlags               *genericclioptions.ConfigFlags
	args                      []string
	userNamespace             string
	userExtendedDaemonSetName string
}

// newpodsOptions provides an instance of podsOptions with default values.
func newPodsOptions(streams genericclioptions.IOStreams) *podsOptions {
	return &podsOptions{
		configFlags: genericclioptions.NewConfigFlags(false),
		IOStreams:   streams,
	}
}

// NewCmdPods provides a cobra command wrapping podsOptions.
func NewCmdPods(streams genericclioptions.IOStreams) *cobra.Command {
	o := newPodsOptions(streams)

	cmd := &cobra.Command{
		Use:          "pods [flags] [ExtendedDaemonSet name]",
		Short:        "print the list pods managed by the EDS",
		Example:      fmt.Sprintf(podsExample, "kubectl eds"),
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

	cmd.Flags().StringVarP(&selectOpt, "select", "", "", "Select the pods to show (can be either canary or not-ready)")
	o.configFlags.AddFlags(cmd.Flags())

	return cmd
}

// complete sets all information required for processing the command.
func (o *podsOptions) complete(cmd *cobra.Command, args []string) error {
	o.args = args
	var err error

	clientConfig := o.configFlags.ToRawKubeConfigLoader()
	// Create the Client for Read/Write operations.
	o.client, err = common.NewClient(clientConfig)
	if err != nil {
		return fmt.Errorf("unable to instantiate client, err: %w", err)
	}

	o.userNamespace, _, err = clientConfig.Namespace()
	if err != nil {
		return err
	}

	ns, err2 := cmd.Flags().GetString("namespace")
	if err2 != nil {
		return err
	}
	if ns != "" {
		o.userNamespace = ns
	}

	if len(args) > 0 {
		o.userExtendedDaemonSetName = args[0]
	}

	return nil
}

// validate ensures that all required arguments and flag values are provided.
func (o *podsOptions) validate() error {
	if len(o.args) < 1 {
		return fmt.Errorf("the extendeddaemonset name is required")
	}

	if selectOpt == "" {
		return fmt.Errorf("missing --select flag")
	}

	if selectOpt != canary && selectOpt != notReady {
		return invalidSelectErr(selectOpt)
	}

	return nil
}

// run runs the command.
func (o *podsOptions) run() error {
	switch selectOpt {
	case canary:
		return common.PrintCanaryPods(o.client, o.userNamespace, o.userExtendedDaemonSetName, o.Out)
	case notReady:
		return common.PrintNotReadyPods(o.client, o.userNamespace, o.userExtendedDaemonSetName, o.Out)
	default:
		return invalidSelectErr(selectOpt)
	}
}

func invalidSelectErr(opt string) error {
	return fmt.Errorf("invalid select flag value '%s': the supported select values are: 'canary' 'not-ready'", opt)
}
