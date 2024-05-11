// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package freeze

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
	frozen   wantedStatus = "frozen"
	unfrozen wantedStatus = "unfrozen"
)

var freezeExample = `
	# %[1]s a rollout
	kubectl eds %[1]s foo
`

// freezeOptions provides information required to manage ExtendedDaemonSet.
type freezeOptions struct {
	configFlags *genericclioptions.ConfigFlags
	args        []string

	client client.Client

	genericclioptions.IOStreams

	userNamespace             string
	userExtendedDaemonSetName string
	want                      wantedStatus
}

// newfreezeOptions provides an instance of freezeOptions with default values.
func newfreezeOptions(streams genericclioptions.IOStreams, want wantedStatus) *freezeOptions {
	return &freezeOptions{
		configFlags: genericclioptions.NewConfigFlags(false),

		IOStreams: streams,

		want: want,
	}
}

// NewCmdFreeze provides a cobra command wrapping freezeOptions.
func NewCmdFreeze(streams genericclioptions.IOStreams) *cobra.Command {
	o := newfreezeOptions(streams, frozen)

	cmd := &cobra.Command{
		Use:          "freeze-rollout [ExtendedDaemonSet name]",
		Short:        "freeze a rollout. This blocks the creation of new pods even on new nodes",
		Example:      fmt.Sprintf(freezeExample, "freeze-rollout"),
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

// NewCmdUnfreeze provides a cobra command wrapping freezeOptions.
func NewCmdUnfreeze(streams genericclioptions.IOStreams) *cobra.Command {
	o := newfreezeOptions(streams, unfrozen)

	cmd := &cobra.Command{
		Use:          "unfreeze-rollout [ExtendedDaemonSet name]",
		Short:        "unfreeze-rollout a frozen rollout.",
		Example:      fmt.Sprintf(freezeExample, "unfreeze-rollout"),
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
func (o *freezeOptions) complete(cmd *cobra.Command, args []string) error {
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
func (o *freezeOptions) validate() error {
	if len(o.args) < 1 {
		return fmt.Errorf("the extendeddaemonset name is required")
	}

	return nil
}

// run used to run the command.
func (o *freezeOptions) run() error {
	eds := &v1alpha1.ExtendedDaemonSet{}
	err := o.client.Get(context.TODO(), client.ObjectKey{Namespace: o.userNamespace, Name: o.userExtendedDaemonSetName}, eds)
	if err != nil && errors.IsNotFound(err) {
		return fmt.Errorf("ExtendedDaemonSet %s/%s not found", o.userNamespace, o.userExtendedDaemonSetName)
	} else if err != nil {
		return fmt.Errorf("unable to get ExtendedDaemonSet, err: %w", err)
	}

	if eds.Status.Canary != nil {
		return fmt.Errorf("cannot freeze rollout: the ExtendedDaemonset has an active canary deployment. You should either fail or validate the canary first")
	}

	newEds := eds.DeepCopy()

	if newEds.Annotations == nil {
		newEds.Annotations = make(map[string]string)
	}

	isFrozen, found := newEds.Annotations[v1alpha1.ExtendedDaemonSetRolloutFrozenAnnotationKey]
	if o.want == frozen && isFrozen == v1alpha1.ValueStringTrue {
		// One case where freezing is impossible:
		// - EDS is already frozen
		return fmt.Errorf("rollout already frozen")
	}
	if o.want == unfrozen && (isFrozen == v1alpha1.ValueStringFalse || !found) {
		// Two cases where unfreezing is impossible:
		// - EDS is already unfrozen
		// - EDS was never frozen (freeze annotation not found)
		return fmt.Errorf("rollout is not frozen; cannot unfreeze")
	}

	// Set appropriate annotation depending on whether cmd is to freeze or unfreeze
	switch o.want {
	case frozen:
		newEds.Annotations[v1alpha1.ExtendedDaemonSetRolloutFrozenAnnotationKey] = v1alpha1.ValueStringTrue
	case unfrozen:
		newEds.Annotations[v1alpha1.ExtendedDaemonSetRolloutFrozenAnnotationKey] = v1alpha1.ValueStringFalse
	}

	patch := client.MergeFrom(eds)
	if err = o.client.Patch(context.TODO(), newEds, patch); err != nil {
		return fmt.Errorf("unable to %s ExtendedDaemonset, err: %w", o.want, err)
	}

	fmt.Fprintf(o.Out, "ExtendedDaemonset '%s/%s' rollout is now %s\n", o.userNamespace, o.userExtendedDaemonSetName, o.want)

	return nil
}
