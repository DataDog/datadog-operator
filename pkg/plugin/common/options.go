// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package common

import (
	"fmt"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Options encapsulates the common fields of command options
type Options struct {
	ConfigFlags   *genericclioptions.ConfigFlags
	Client        client.Client
	Clientset     *kubernetes.Clientset
	UserNamespace string
}

// Init initialize the common config of command options
func (o *Options) Init(cmd *cobra.Command) error {
	clientConfig := o.GetClientConfig()

	client, err := NewClient(clientConfig)
	if err != nil {
		return fmt.Errorf("unable to instantiate client: %w", err)
	}
	o.SetClient(client)

	clientset, err := NewClientset(clientConfig)
	if err != nil {
		return fmt.Errorf("unable to instantiate clientset: %w", err)
	}
	o.SetClientset(clientset)

	nsConfig, _, err := clientConfig.Namespace()
	if err != nil {
		return err
	}

	nsFlag, err := cmd.Flags().GetString("namespace")
	if err != nil {
		return err
	}

	if nsFlag != "" {
		o.SetNamespace(nsFlag)
	} else {
		o.SetNamespace(nsConfig)
	}

	return nil
}

// SetNamespace configures the namespace
func (o *Options) SetNamespace(ns string) {
	o.UserNamespace = ns
}

// SetClient configures the client
func (o *Options) SetClient(client client.Client) {
	o.Client = client
}

// SetClientset configures the clientset
func (o *Options) SetClientset(clientset *kubernetes.Clientset) {
	o.Clientset = clientset
}

// GetClientConfig returns the client config
func (o *Options) GetClientConfig() clientcmd.ClientConfig {
	return o.ConfigFlags.ToRawKubeConfigLoader()
}

// SetConfigFlags configures the config flags
func (o *Options) SetConfigFlags() {
	o.ConfigFlags = genericclioptions.NewConfigFlags(false)
}
