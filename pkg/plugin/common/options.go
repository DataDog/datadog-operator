// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"fmt"

	"github.com/spf13/cobra"
	apiextensionclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Options encapsulates the common fields of command options
type Options struct {
	ConfigFlags     *genericclioptions.ConfigFlags
	Client          client.Client
	Clientset       *kubernetes.Clientset
	APIExtClient    *apiextensionclient.Clientset
	DiscoveryClient discovery.DiscoveryInterface

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

	restConfig, err := o.ConfigFlags.ToRESTConfig()
	if err != nil {
		return fmt.Errorf("unable to create restConfig for APIExtensionClient, err:%w", err)
	}
	apiextClient, err := apiextensionclient.NewForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("unable to create APIExtensionClient, err:%w", err)
	}
	o.SetApiExtensionClient(apiextClient)

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(restConfig)
	if err != nil {
		return fmt.Errorf("unable to create DiscoveryClient, err:%w", err)
	}
	o.SetDiscoveryClient(discoveryClient)

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

// SetApiExtensionClient configures the APIExtClient
func (o *Options) SetApiExtensionClient(client *apiextensionclient.Clientset) {
	o.APIExtClient = client
}

// SetDiscoveryClient configures the DiscoveryClient
func (o *Options) SetDiscoveryClient(client discovery.DiscoveryInterface) {
	o.DiscoveryClient = client
}

// GetClientConfig returns the client config
func (o *Options) GetClientConfig() clientcmd.ClientConfig {
	return o.ConfigFlags.ToRawKubeConfigLoader()
}

// SetConfigFlags configures the config flags
func (o *Options) SetConfigFlags() {
	o.ConfigFlags = genericclioptions.NewConfigFlags(false)
}
