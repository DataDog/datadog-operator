// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package find

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/pkg/plugin/common"
)

var findExample = `
  # find the datadog agent pod monitoring a pod named foo
  %[1]s find foo
`

// options provides information required by Datadog find command
type options struct {
	genericclioptions.IOStreams
	common.Options
	args    []string
	podName string
}

// newOptions provides an instance of getOptions with default values
func newOptions(streams genericclioptions.IOStreams) *options {
	o := &options{
		IOStreams: streams,
	}
	o.SetConfigFlags()
	return o
}

// New provides a cobra command wrapping options for "find" sub command
func New(streams genericclioptions.IOStreams) *cobra.Command {
	o := newOptions(streams)
	cmd := &cobra.Command{
		Use:          "find [pod name] [flags]",
		Short:        "Find datadog agent pod monitoring a given pod",
		Example:      fmt.Sprintf(findExample, "kubectl datadog"),
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
		o.podName = args[0]
	}
	return o.Init(cmd)
}

// validate ensures that all required arguments and flag values are provided
func (o *options) validate() error {
	if o.podName == "" {
		return errors.New("pod name argument is missing")
	}
	argsCount := len(o.args)
	if argsCount > 1 {
		return fmt.Errorf("one argument is allowed, got %d", argsCount)
	}
	return nil
}

// run runs the find command
func (o *options) run(cmd *cobra.Command) error {
	pod, err := o.Clientset.CoreV1().Pods(o.UserNamespace).Get(context.TODO(), o.podName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	agentName, err := o.getAgentByNode(pod.Spec.NodeName)
	if err != nil {
		return err
	}
	cmd.Println(fmt.Sprintf("Agent %s is monitoring %s", agentName, o.podName))
	return nil
}

// getAgentByNode returns the pod of the datadog agent running on a given node
func (o *options) getAgentByNode(nodeName string) (string, error) {
	podList, err := o.Clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + nodeName,
		LabelSelector: common.AgentLabel,
	})
	if err != nil {
		return "", err
	}
	if len(podList.Items) == 0 {
		return "", fmt.Errorf("no agent pod found. Label selector used: %s", common.AgentLabel)
	}
	return podList.Items[0].Name, nil
}
