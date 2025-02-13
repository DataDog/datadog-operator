// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/pkg/plugin/common"
)

var serviceExample = `
  # validate the autodiscovery annotations for a service named foo
  %[1]s service foo
`

// options provides information required by validate service command.
type options struct {
	genericclioptions.IOStreams
	common.Options
	args        []string
	serviceName string
}

// newOptions provides an instance of options with default values.
func newOptions(streams genericclioptions.IOStreams) *options {
	o := &options{
		IOStreams: streams,
	}
	o.SetConfigFlags()

	return o
}

// New provides a cobra command wrapping options for service sub command.
func New(streams genericclioptions.IOStreams) *cobra.Command {
	o := newOptions(streams)
	cmd := &cobra.Command{
		Use:          "service [service name] [flags]",
		Short:        "Validate the autodiscovery annotations for a service",
		Example:      fmt.Sprintf(serviceExample, "kubectl datadog agent"),
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
		o.serviceName = args[0]
	}

	return o.Init(cmd)
}

// validate ensures that all required arguments and flag values are provided.
func (o *options) validate() error {
	if o.serviceName == "" {
		return errors.New("service name argument is missing")
	}

	argsCount := len(o.args)
	if argsCount > 1 {
		return fmt.Errorf("one argument is allowed, got %d", argsCount)
	}

	return nil
}

// run runs the service command.
func (o *options) run(cmd *cobra.Command) error {
	svc, err := o.Clientset.CoreV1().Services(o.UserNamespace).Get(context.TODO(), o.serviceName, metav1.GetOptions{})
	if err != nil {
		return err
	}

	annotations := svc.GetAnnotations()
	svcID := fmt.Sprintf("%s%s", common.ADPrefix, "service")
	epID := fmt.Sprintf("%s%s", common.ADPrefix, "endpoints")
	svcErrors, svcAnnotated := common.ValidateAnnotationsContent(annotations, svcID)
	epErrors, epAnnotated := common.ValidateAnnotationsContent(annotations, epID)
	errors := append(svcErrors, epErrors...)

	if len(errors) > 0 {
		cmd.Println(len(errors), "error(s) detected:")
		for _, err := range errors {
			cmd.Println("\t", err)
		}

		return nil
	}

	if (svcAnnotated || epAnnotated) && len(errors) == 0 {
		cmd.Println(fmt.Sprintf("Annotations for service %s are valid", o.serviceName))
	} else {
		cmd.Println(fmt.Sprintf("Service %s is not annotated", o.serviceName))
	}

	return nil
}
