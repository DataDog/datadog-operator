// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package canary

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/DataDog/extendeddaemonset/controllers/extendeddaemonsetreplicaset/conditions"
	"github.com/DataDog/extendeddaemonset/pkg/plugin/common"
)

const (
	cmdFail = true
)

var failExample = `
    # fail a canary deployment
    kubectl eds canary fail foo
`

// failOptions provides information required to manage ExtendedDaemonSet.
type failOptions struct {
	configFlags *genericclioptions.ConfigFlags
	args        []string

	client client.Client

	genericclioptions.IOStreams

	userNamespace             string
	userExtendedDaemonSetName string
	failStatus                bool
}

// newfailOptions provides an instance of GetOptions with default values.
func newfailOptions(streams genericclioptions.IOStreams, failStatus bool) *failOptions {
	return &failOptions{
		configFlags: genericclioptions.NewConfigFlags(false),

		IOStreams: streams,

		failStatus: failStatus,
	}
}

// newCmdFail provides a cobra command wrapping failOptions.
func newCmdFail(streams genericclioptions.IOStreams) *cobra.Command {
	o := newfailOptions(streams, cmdFail)

	cmd := &cobra.Command{
		Use:          "fail [ExtendedDaemonSet name]",
		Short:        "fail canary deployment",
		Example:      failExample,
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
func (o *failOptions) complete(cmd *cobra.Command, args []string) error {
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
func (o *failOptions) validate() error {
	if len(o.args) < 1 {
		return fmt.Errorf("the extendeddaemonset name is required")
	}

	return nil
}

// run use to run the command.
func (o *failOptions) run() error {
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

	// Get Canary ERS
	canaryERSName := eds.Status.Canary.ReplicaSet
	canaryERS := &v1alpha1.ExtendedDaemonSetReplicaSet{}
	err = o.client.Get(context.TODO(), client.ObjectKey{Namespace: o.userNamespace, Name: canaryERSName}, canaryERS)
	if err != nil && errors.IsNotFound(err) {
		return fmt.Errorf("ERS %s/%s not found", o.userNamespace, canaryERSName)
	} else if err != nil {
		return fmt.Errorf("unable to get ERS, err: %w", err)
	}

	newCanaryERS := canaryERS.DeepCopy()

	newCanaryERS.Status.Conditions = append(
		newCanaryERS.Status.Conditions,
		conditions.NewExtendedDaemonSetReplicaSetCondition(
			v1alpha1.ConditionTypeCanaryFailed,
			conditions.BoolToCondition(true),
			metav1.Now(),
			"Manually failed",
			"",
			true),
	)
	if err = o.client.Status().Update(context.TODO(), newCanaryERS); err != nil {
		return fmt.Errorf("unable to update ERS status, err: %w", err)
	}

	fmt.Fprintf(o.Out, "ExtendedDaemonSetReplicaSet '%s/%s' canary deployment set to failed\n", o.userNamespace, o.userExtendedDaemonSetName)

	return nil
}
