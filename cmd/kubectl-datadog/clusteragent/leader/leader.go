// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package leader

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	coordv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/pkg/plugin/common"
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

	DDAName string
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
		Use:          "leader <DatadogAgent resource name>",
		Short:        "Get Datadog Cluster Agent leader",
		Example:      fmt.Sprintf(leaderExample, "kubectl datadog clusteragent datadog-agent"),
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

	cmd.Flags().StringVarP(&o.DDAName, "dda-name", "", "", "The DatadogAgent resource name to get the leader from")
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
	if o.DDAName == "" {
		if len(o.args) != 0 {
			o.DDAName = o.args[0]
		} else {
			return fmt.Errorf("DatadogAgent resource name is required")
		}
	}
	return nil
}

// run runs the leader command.
func (o *options) run(cmd *cobra.Command) error {
	leaderObjName := fmt.Sprintf("%s-leader-election", o.DDAName)
	objKey := client.ObjectKey{Namespace: o.UserNamespace, Name: leaderObjName}

	var leaderName string
	var err error
	var useLease bool

	useLease, err = isLeaseSupported(o.DiscoveryClient)
	if err != nil {
		return fmt.Errorf("unable to check if lease is suppoered %w", err)
	}
	if useLease {
		fmt.Fprintln(o.IOStreams.Out, "Using lease for leader election")
		leaderName, err = o.getLeaderFromLease(objKey)
	} else {
		fmt.Fprintln(o.IOStreams.Out, "Using lease for configmap")
		leaderName, err = o.getLeaderFromConfigMap(objKey)
	}
	if err != nil {
		return fmt.Errorf("unable to get leader from lease: %w", err)
	}

	cmd.Println("The Pod name of the Cluster Agent is:", leaderName)

	return nil
}

func (o *options) getLeaderFromLease(objKey client.ObjectKey) (string, error) {
	lease := &coordv1.Lease{}
	err := o.Client.Get(context.TODO(), objKey, lease)
	if err != nil && apierrors.IsNotFound(err) {
		return "", fmt.Errorf("lease %s/%s not found", objKey.Namespace, objKey.Name)
	} else if err != nil {
		return "", fmt.Errorf("unable to get leader election config map: %w", err)
	}

	// get the info from the lease
	if lease.Spec.HolderIdentity == nil {
		return "", fmt.Errorf("lease %s/%s does not have a holder identity", objKey.Namespace, objKey.Name)
	}

	return *lease.Spec.HolderIdentity, nil
}

func (o *options) getLeaderFromConfigMap(objKey client.ObjectKey) (string, error) {
	// Get the config map holding the leader identity.
	cm := &corev1.ConfigMap{}
	err := o.Client.Get(context.TODO(), objKey, cm)
	if err != nil && apierrors.IsNotFound(err) {
		return "", fmt.Errorf("config map %s/%s not found", objKey.Namespace, objKey.Name)
	} else if err != nil {
		return "", fmt.Errorf("unable to get leader election config map: %w", err)
	}

	// Get leader from annotations.
	annotations := cm.GetAnnotations()
	leaderInfo, found := annotations["control-plane.alpha.kubernetes.io/leader"]
	if !found {
		return "", fmt.Errorf("couldn't find leader annotation on %s/%s config map", objKey.Namespace, objKey.Name)
	}
	resp := leaderResponse{}
	if err := json.Unmarshal([]byte(leaderInfo), &resp); err != nil {
		return "", fmt.Errorf("couldn't unmarshal leader annotation: %w", err)
	}

	return resp.HolderIdentity, nil
}

func isLeaseSupported(client discovery.DiscoveryInterface) (bool, error) {
	apiGroupList, err := client.ServerGroups()
	if err != nil {
		return false, fmt.Errorf("unable to discover APIGroups, err:%w", err)
	}
	groupVersions := metav1.ExtractGroupVersions(apiGroupList)
	for _, grv := range groupVersions {
		if grv == "coordination.k8s.io/v1" || grv == "coordination.k8s.io/v1beta1" {
			return true, nil
		}
	}

	return false, nil
}
