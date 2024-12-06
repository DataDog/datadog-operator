// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package upgrade

import (
	"context"
	"errors"
	"fmt"

	"github.com/DataDog/datadog-operator/api/crds/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/crds/utils"
	"github.com/DataDog/datadog-operator/pkg/defaulting"
	"github.com/DataDog/datadog-operator/pkg/plugin/common"

	"github.com/spf13/cobra"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	image          string
	latest         bool
	latestImage    = defaulting.GetLatestAgentImage()
	upgradeExample = `
  # upgrade the version of the datadog agent to known release (%[2]s)
  %[1]s upgrade --latest

  # upgrade the version of the datadog agent defined in DatadogAgent named foo to latest
  %[1]s upgrade foo --latest

  # upgrade the datadog agent with a custom image
  %[1]s upgrade --image <account>/<repo>:<tag>
`
)

// options provides information required by agent upgrade command
type options struct {
	genericclioptions.IOStreams
	common.Options
	args                 []string
	userDatadogAgentName string
}

// newOptions provides an instance of options with default values
func newOptions(streams genericclioptions.IOStreams) *options {
	o := &options{
		IOStreams: streams,
	}
	o.SetConfigFlags()
	return o
}

// New provides a cobra command wrapping options for "upgrade" sub command
func New(streams genericclioptions.IOStreams) *cobra.Command {
	o := newOptions(streams)
	cmd := &cobra.Command{
		Use:          "upgrade [flags]",
		Short:        "Upgrade the Datadog Agent version",
		Example:      fmt.Sprintf(upgradeExample, "kubectl datadog agent", defaulting.AgentLatestVersion),
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

	cmd.Flags().StringVarP(&image, "image", "i", "", "The image of the Datadog Agent")
	cmd.Flags().BoolVarP(&latest, "latest", "l", false, fmt.Sprintf("Upgrade to %s", defaulting.GetLatestAgentImage()))

	o.ConfigFlags.AddFlags(cmd.Flags())

	return cmd
}

// complete sets all information required for processing the command
func (o *options) complete(cmd *cobra.Command, args []string) error {
	o.args = args
	if len(args) > 0 {
		o.userDatadogAgentName = args[0]
	}
	return o.Init(cmd)
}

// validate ensures that all required arguments and flag values are provided
func (o *options) validate() error {
	return common.ValidateUpgrade(image, latest)
}

// run runs the upgrade command
func (o *options) run(cmd *cobra.Command) error {
	return o.runV2(cmd)
}

// runV2 runs the upgrade command on v2alpha1.DatadogAgent resource
func (o *options) runV2(cmd *cobra.Command) error {
	ddList := &v2alpha1.DatadogAgentList{}
	if o.userDatadogAgentName == "" {
		if err := o.Client.List(context.TODO(), ddList, &client.ListOptions{Namespace: o.UserNamespace}); err != nil {
			return fmt.Errorf("unable to list DatadogAgent: %w", err)
		}
		if len(ddList.Items) == 0 {
			return errors.New("cannot find any DatadogAgent")
		}
	} else {
		dd := &v2alpha1.DatadogAgent{}
		err := o.Client.Get(context.TODO(), client.ObjectKey{Namespace: o.UserNamespace, Name: o.userDatadogAgentName}, dd)
		if err != nil && apierrors.IsNotFound(err) {
			return fmt.Errorf("DatadogAgent %s/%s not found", o.UserNamespace, o.userDatadogAgentName)
		} else if err != nil {
			return fmt.Errorf("unable to get DatadogAgent: %w", err)
		}
		ddList.Items = append(ddList.Items, *dd)
	}
	image = getImage()
	for _, dd := range ddList.Items {
		err := o.upgradeV2(&dd, image)
		if err != nil {
			cmd.Println(fmt.Sprintf("Couldn't update %s/%s: %v", dd.GetNamespace(), dd.GetName(), err))
		} else {
			cmd.Println(fmt.Sprintf("Agent image updated successfully in %s/%s", dd.GetNamespace(), dd.GetName()))
		}
	}
	return nil
}

// upgrade updates the agent and cluster-check-runner versions in the DatadogAgent object
func (o *options) upgradeV2(dd *v2alpha1.DatadogAgent, image string) error {
	if dd.Spec.Override == nil {
		dd.Spec.Override = make(map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride)
	}

	imgName, imgTag := common.SplitImageString(image)

	newDD := dd.DeepCopy()

	if err := common.OverrideComponentImage(&newDD.Spec, v2alpha1.NodeAgentComponentName, imgName, imgTag); err != nil {
		return fmt.Errorf("unable to update %s image, err: %w", v2alpha1.NodeAgentComponentName, err)
	}

	if err := common.OverrideComponentImage(&newDD.Spec, v2alpha1.ClusterChecksRunnerComponentName, imgName, imgTag); err != nil {
		return fmt.Errorf("unable to update %s image, err: %w", v2alpha1.ClusterChecksRunnerComponentName, err)
	}

	if !apiutils.IsEqualStruct(dd.Spec, newDD.Spec) {
		return o.Client.Update(context.TODO(), newDD)
	}
	return nil
}

func getImage() string {
	if image != "" {
		return image
	}
	return latestImage
}
