// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package profile

import (
	"context"
	"errors"
	"fmt"

	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/plugin/common"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

var nodeExample = `
  # profile the datadogagentprofile applied to node foo
  %[1]s profile foo
`

// options provides information required by Datadog profile command
type options struct {
	genericclioptions.IOStreams
	common.Options
	args     []string
	nodeName string
}

// newOptions provides an instance of getOptions with default values
func newOptions(streams genericclioptions.IOStreams) *options {
	o := &options{
		IOStreams: streams,
	}
	o.SetConfigFlags()
	return o
}

// New provides a cobra command wrapping options for "profile" sub command
func New(streams genericclioptions.IOStreams) *cobra.Command {
	o := newOptions(streams)
	cmd := &cobra.Command{
		Use:          "profile [node name] [flags]",
		Short:        "Find the datadogagentprofile applied to a given node",
		Example:      fmt.Sprintf(nodeExample, "kubectl datadog dap find"),
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

	o.ConfigFlags.AddFlags(cmd.Flags())

	return cmd
}

// complete sets all information required for processing the command
func (o *options) complete(cmd *cobra.Command, args []string) error {
	o.args = args
	if len(args) > 0 {
		o.nodeName = args[0]
	}
	return o.Init(cmd)
}

// validate ensures that all required arguments and flag values are provided
func (o *options) validate() error {
	if o.nodeName == "" {
		return errors.New("node name argument is missing")
	}
	argsCount := len(o.args)
	if argsCount > 1 {
		return fmt.Errorf("one argument is allowed, got %d", argsCount)
	}
	return nil
}

// run runs the profile command
func (o *options) run(cmd *cobra.Command) error {
	node, err := o.Clientset.CoreV1().Nodes().Get(context.TODO(), o.nodeName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	profileName := getProfileNameFromLabels(node.Labels)
	cmd.Println(profileName)
	return nil
}

// getProfileNameFromLabels returns the name of the datadogagentprofile applied to a given node
func getProfileNameFromLabels(labels map[string]string) string {
	if profileName, ok := labels[agentprofile.ProfileLabelKey]; ok {
		return profileName
	}
	return "None"
}
