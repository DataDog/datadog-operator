// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package leader

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-operator/pkg/plugin/common"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var leaderExample = `
  # get the pod name of the datadog cluster agent leader
  %[1]s leader
`

// options provides information required by clusteragent leader command.
type options struct {
	genericclioptions.IOStreams
	common.Options
	args []string
}

type leaderResponse struct {
	HolderIdentity string `json:"holderIdentity"`
}

// newOptions provides an instance of options with default values.
func newOptions(streams genericclioptions.IOStreams) *options {
	o := &options{
		IOStreams: streams,
	}
	o.SetConfigFlags()

	return o
}

// New provides a cobra command wrapping options for "leader" sub command.
func New(streams genericclioptions.IOStreams) *cobra.Command {
	o := newOptions(streams)
	cmd := &cobra.Command{
		Use:          "leader",
		Short:        "Get Datadog Cluster Agent leader",
		Example:      fmt.Sprintf(leaderExample, "kubectl datadog clusteragent"),
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

	return o.Init(cmd)
}

// validate ensures that all required arguments and flag values are provided.
func (o *options) validate() error {
	return nil
}

// run runs the leader command.
func (o *options) run(cmd *cobra.Command) error {
	// FIXME: Support multiple leader election config maps.
	cmName := "datadog-leader-election"

	// Get the config map holding the leader identity.
	cm := &corev1.ConfigMap{}
	err := o.Client.Get(context.TODO(), client.ObjectKey{Namespace: o.UserNamespace, Name: cmName}, cm)
	if err != nil && apierrors.IsNotFound(err) {
		return fmt.Errorf("config map %s/%s not found", o.UserNamespace, cmName)
	} else if err != nil {
		return fmt.Errorf("unable to get leader election config map: %w", err)
	}

	// Get leader from annotations.
	annotations := cm.GetAnnotations()
	leaderInfo, found := annotations["control-plane.alpha.kubernetes.io/leader"]
	if !found {
		return fmt.Errorf("couldn't find leader annotation on %s config map", cmName)
	}
	leader := leaderResponse{}
	if err := json.Unmarshal([]byte(leaderInfo), &leader); err != nil {
		return fmt.Errorf("couldn't unmarshal leader annotation: %w", err)
	}
	cmd.Println("The Pod name of the Cluster Agent is:", leader.HolderIdentity)

	return nil
}
