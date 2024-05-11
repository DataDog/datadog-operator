// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pause

import (
	"context"
	"fmt"

	"github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/DataDog/extendeddaemonset/pkg/plugin/common"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type wantedStatus string

const (
	paused   wantedStatus = "paused"
	unpaused wantedStatus = "unpaused"
)

var pauseExample = `
	# %[1]s an active rolling update
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
	want                      wantedStatus
}

// newPauseOptions provides an instance of GetOptions with default values.
func newPauseOptions(streams genericclioptions.IOStreams, want wantedStatus) *pauseOptions {
	return &pauseOptions{
		configFlags: genericclioptions.NewConfigFlags(false),

		IOStreams: streams,

		want: want,
	}
}

// NewCmdPause provides a cobra command wrapping pauseOptions.
func NewCmdPause(streams genericclioptions.IOStreams) *cobra.Command {
	o := newPauseOptions(streams, paused)

	cmd := &cobra.Command{
		Use:          "pause-rolling-update [ExtendedDaemonSet name]",
		Short:        "pause a rolling update. This will not block the creation of new pods if new nodes appear or if it's the deployment of the first replicaset",
		Example:      fmt.Sprintf(pauseExample, "pause-rolling-update"),
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

// NewCmdUnpause provides a cobra command wrapping pauseOptions.
func NewCmdUnpause(streams genericclioptions.IOStreams) *cobra.Command {
	o := newPauseOptions(streams, unpaused)

	cmd := &cobra.Command{
		Use:          "unpause-rolling-update [ExtendedDaemonSet name]",
		Short:        "unpause a paused rolling update.",
		Example:      fmt.Sprintf(pauseExample, "unpause-rolling-update"),
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

// run used to run the command.
func (o *pauseOptions) run() error {
	eds := &v1alpha1.ExtendedDaemonSet{}
	err := o.client.Get(context.TODO(), client.ObjectKey{Namespace: o.userNamespace, Name: o.userExtendedDaemonSetName}, eds)
	if err != nil && errors.IsNotFound(err) {
		return fmt.Errorf("ExtendedDaemonSet %s/%s not found", o.userNamespace, o.userExtendedDaemonSetName)
	} else if err != nil {
		return fmt.Errorf("unable to get ExtendedDaemonSet, err: %w", err)
	}

	if eds.Status.Canary != nil {
		return fmt.Errorf("cannot pause rolling update: the ExtendedDaemonset has an active canary deployment. You can use the canary pause command instead")
	}

	newEds := eds.DeepCopy()

	if newEds.Annotations == nil {
		newEds.Annotations = make(map[string]string)
	}

	isPaused, found := newEds.Annotations[v1alpha1.ExtendedDaemonSetRollingUpdatePausedAnnotationKey]
	if o.want == paused && isPaused == v1alpha1.ValueStringTrue {
		// One case where pausing is impossible:
		// - EDS is already paused
		return fmt.Errorf("rolling update already paused")
	}
	if o.want == unpaused && (isPaused == v1alpha1.ValueStringFalse || !found) {
		// Two cases where unpausing is impossible:
		// - EDS is already unpaused
		// - EDS was never paused (pause annotation not found)
		return fmt.Errorf("rolling update is not paused; cannot unpause")
	}

	// Set appropriate annotation depending on whether cmd is to pause or unpause
	switch o.want {
	case paused:
		newEds.Annotations[v1alpha1.ExtendedDaemonSetRollingUpdatePausedAnnotationKey] = v1alpha1.ValueStringTrue
	case unpaused:
		newEds.Annotations[v1alpha1.ExtendedDaemonSetRollingUpdatePausedAnnotationKey] = v1alpha1.ValueStringFalse
	}

	patch := client.MergeFrom(eds)
	if err = o.client.Patch(context.TODO(), newEds, patch); err != nil {
		return fmt.Errorf("unable to %s ExtendedDaemonset, err: %w", o.want, err)
	}

	fmt.Fprintf(o.Out, "ExtendedDaemonset '%s/%s' rolling update is now %s\n", o.userNamespace, o.userExtendedDaemonSetName, o.want)

	return nil
}
