// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package canary

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/DataDog/extendeddaemonset/pkg/plugin/common"
)

const (
	cmdPause   = true
	cmdUnpause = false
)

var pauseExample = `
	# %[1]s a canary deployment
	kubectl eds %[1]s foo
`

// pauseOptions provides information required to manage ExtendedDaemonSet.
type pauseOptions struct {
	configFlags *genericclioptions.ConfigFlags
	args        []string

	client client.Client

	genericclioptions.IOStreams

	userNamespace             string
	userExtendedDaemonSetName string
	pauseStatus               bool
}

// newPauseOptions provides an instance of GetOptions with default values.
func newPauseOptions(streams genericclioptions.IOStreams, pauseStatus bool) *pauseOptions {
	return &pauseOptions{
		configFlags: genericclioptions.NewConfigFlags(false),

		IOStreams: streams,

		pauseStatus: pauseStatus,
	}
}

// newCmdPause provides a cobra command wrapping pauseOptions.
func newCmdPause(streams genericclioptions.IOStreams) *cobra.Command {
	o := newPauseOptions(streams, cmdPause)

	cmd := &cobra.Command{
		Use:          "pause [ExtendedDaemonSet name]",
		Short:        "pause canary deployment",
		Example:      fmt.Sprintf(pauseExample, "pause"),
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

	o.configFlags.AddFlags(cmd.Flags())

	return cmd
}

// newCmdUnpause provides a cobra command wrapping pauseOptions.
func newCmdUnpause(streams genericclioptions.IOStreams) *cobra.Command {
	o := newPauseOptions(streams, cmdUnpause)

	cmd := &cobra.Command{
		Use:          "unpause [ExtendedDaemonSet name]",
		Short:        "unpause canary deployment",
		Example:      fmt.Sprintf(pauseExample, "unpause"),
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

	o.configFlags.AddFlags(cmd.Flags())

	return cmd
}

// complete sets all information required for processing the command.
func (o *pauseOptions) complete(cmd *cobra.Command, args []string) error {
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
func (o *pauseOptions) validate() error {
	if len(o.args) < 1 {
		return fmt.Errorf("the extendeddaemonset name is required")
	}

	return nil
}

// run use to run the command.
func (o *pauseOptions) run() error {
	eds := &v1alpha1.ExtendedDaemonSet{}
	err := o.client.Get(context.TODO(), client.ObjectKey{Namespace: o.userNamespace, Name: o.userExtendedDaemonSetName}, eds)
	if err != nil && errors.IsNotFound(err) {
		return fmt.Errorf("ExtendedDaemonSet %s/%s not found", o.userNamespace, o.userExtendedDaemonSetName)
	} else if err != nil {
		return fmt.Errorf("unable to get ExtendedDaemonSet, err: %w", err)
	}

	if eds.Spec.Strategy.Canary == nil {
		return fmt.Errorf("the ExtendedDaemonset does not have a canary strategy")
	}
	if eds.Status.Canary == nil {
		return fmt.Errorf("the ExtendedDaemonset does not have an active canary deployment")
	}

	newEds := eds.DeepCopy()

	// TODO: update pause action to be less dependent on annotations as in the fail action
	if newEds.Annotations == nil {
		newEds.Annotations = make(map[string]string)
	} else if isPaused, ok := newEds.Annotations[v1alpha1.ExtendedDaemonSetCanaryPausedAnnotationKey]; ok {
		if o.pauseStatus && isPaused == v1alpha1.ValueStringTrue {
			return fmt.Errorf("canary deployment already paused")
		} else if !o.pauseStatus && isPaused == v1alpha1.ValueStringFalse {
			return fmt.Errorf("canary deployment is not paused; cannot unpause")
		}
	}
	// Set appropriate annotation depending on whether cmd is to pause or unpause
	if o.pauseStatus {
		newEds.Annotations[v1alpha1.ExtendedDaemonSetCanaryPausedAnnotationKey] = v1alpha1.ValueStringTrue
		// Set to false in case it was previously true
		newEds.Annotations[v1alpha1.ExtendedDaemonSetCanaryUnpausedAnnotationKey] = v1alpha1.ValueStringFalse
	} else {
		newEds.Annotations[v1alpha1.ExtendedDaemonSetCanaryPausedAnnotationKey] = v1alpha1.ValueStringFalse
		newEds.Annotations[v1alpha1.ExtendedDaemonSetCanaryUnpausedAnnotationKey] = v1alpha1.ValueStringTrue
	}

	patch := client.MergeFrom(eds)
	if err = o.client.Patch(context.TODO(), newEds, patch); err != nil {
		return fmt.Errorf("unable to %s ExtendedDaemonset deployment, err: %w", strconv.FormatBool(o.pauseStatus), err)
	}

	fmt.Fprintf(o.Out, "ExtendedDaemonset '%s/%s' deployment paused set to %t\n", o.userNamespace, o.userExtendedDaemonSetName, o.pauseStatus)

	return nil
}
