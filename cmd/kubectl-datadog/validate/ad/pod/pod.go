// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pod

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/pkg/plugin/common"
)

var podExample = `
  # validate the autodiscovery annotations for a pod named foo
  %[1]s pod foo
`

// options provides information required by agent validate pod command.
type options struct {
	genericclioptions.IOStreams
	common.Options
	args    []string
	podName string
}

// newOptions provides an instance of options with default values.
func newOptions(streams genericclioptions.IOStreams) *options {
	o := &options{
		IOStreams: streams,
	}
	o.SetConfigFlags()

	return o
}

// New provides a cobra command wrapping options for pod sub command.
func New(streams genericclioptions.IOStreams) *cobra.Command {
	o := newOptions(streams)
	cmd := &cobra.Command{
		Use:          "pod [pod name] [flags]",
		Short:        "Validate the autodiscovery annotations for a pod",
		Example:      fmt.Sprintf(podExample, "kubectl datadog agent"),
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

// complete sets all information required for processing the command.
func (o *options) complete(cmd *cobra.Command, args []string) error {
	o.args = args
	if len(args) > 0 {
		o.podName = args[0]
	}

	return o.Init(cmd)
}

// validate ensures that all required arguments and flag values are provided.
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

// run runs the pod command.
func (o *options) run(cmd *cobra.Command) error {
	pod, err := o.Clientset.CoreV1().Pods(o.UserNamespace).Get(context.TODO(), o.podName, metav1.GetOptions{})
	if err != nil {
		return err
	}
	validIDs := map[string]bool{}
	totalErrors := []string{}
	annotations := pod.GetAnnotations()
	if !common.IsAnnotated(annotations, common.ADPrefix) {
		cmd.Println(fmt.Sprintf("Pod %s doesn't have autodiscovery annotations", o.podName))
		return nil
	}

	for _, container := range pod.Spec.Containers {
		id := fmt.Sprintf("%s%s", common.ADPrefix, container.Name)
		validIDs[container.Name] = true
		if common.IsAnnotated(annotations, id) {
			errors, _ := common.ValidateAnnotationsContent(annotations, id)
			totalErrors = append(totalErrors, errors...)
		}
	}

	errors := common.ValidateAnnotationsMatching(annotations, validIDs)
	totalErrors = append(totalErrors, errors...)
	if len(totalErrors) > 0 {
		cmd.Println(len(totalErrors), "error(s) detected:")
		for _, err := range totalErrors {
			cmd.Println("\t", err)
		}
	} else {
		cmd.Println(fmt.Sprintf("Annotations for pod %s are valid", o.podName))
	}

	return nil
}
