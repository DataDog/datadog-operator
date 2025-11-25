// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package patchcrd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/pkg/plugin/common"
)

// Options provides information required to manage the patch function.
type Options struct {
	genericclioptions.IOStreams
	common.Options
	crdName        string
	storedVersions []string
}

// NewOptions provides an instance of Options with default values.
func NewOptions(streams genericclioptions.IOStreams) *Options {
	opts := &Options{
		IOStreams: streams,
	}

	opts.SetConfigFlags()

	return opts
}

// NewCmdPatch provides a cobra command wrapping Options.
func NewCmdPatch(streams genericclioptions.IOStreams) *cobra.Command {
	o := NewOptions(streams)

	cmd := &cobra.Command{
		Use:          "patch-crd <crd-name>",
		Short:        "patch the CRD status to update storedVersions list",
		Example:      "./check-operator patch-crd datadogagents.datadoghq.com",
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Complete(c, args); err != nil {
				return err
			}
			if err := o.Validate(); err != nil {
				return err
			}

			return o.Run()
		},
	}

	cmd.Flags().StringArrayVar(&o.storedVersions, "storedVersions", []string{}, "used to define the status.storedVersion field in the CRD")

	o.ConfigFlags.AddFlags(cmd.Flags())

	return cmd
}

// Complete sets all information required for processing the command.
func (o *Options) Complete(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		o.crdName = args[0]
	}

	return o.Init(cmd)
}

// Validate ensures that all required arguments and flag values are provided.
func (o *Options) Validate() error {
	if o.crdName == "" {
		return errors.New("the CRD name is required")
	}

	if len(o.storedVersions) == 0 {
		return errors.New("at least one storedVersion needs to be provided")
	}

	return nil
}

// Run is used to run the command.
func (o *Options) Run() error {
	o.printOutf("Start checking patched CRD status")
	crd, err := o.APIExtClient.ApiextensionsV1().CustomResourceDefinitions().Get(context.Background(), o.crdName, v1.GetOptions{})
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return fmt.Errorf("unable to patch %s CRD, err: %w", o.crdName, err)
		}
		return fmt.Errorf("unknown error during CRD get, err: %w", err)
	}

	o.printOutf("Current storedVersions value: %s,", strings.Join(crd.Status.StoredVersions, ","))
	crd.Status.StoredVersions = o.storedVersions

	crd, err = o.APIExtClient.ApiextensionsV1().CustomResourceDefinitions().UpdateStatus(context.Background(), crd, v1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("unable to update CRD, retry later, err: %w", err)
	}
	o.printOutf("Sucessful CRD update, new storedVersions values: %s", strings.Join(crd.Status.StoredVersions, ","))
	return nil

}

func (o *Options) printOutf(format string, a ...any) {
	args := []any{time.Now().UTC().Format("2006-01-02T15:04:05.999Z"), o.crdName}
	args = append(args, a...)
	_, _ = fmt.Fprintf(o.Out, "[%s] CRD '%s': "+format+"\n", args...)
}
