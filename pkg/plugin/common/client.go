// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package common

import (
	"fmt"

	"github.com/DataDog/datadog-operator/pkg/apis"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// NewClient returns a new controller-runtime client instance
func NewClient(clientConfig clientcmd.ClientConfig) (client.Client, error) {
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("unable to get rest client config: %v", err)
	}

	// Create the mapper provider
	mapper, err := apiutil.NewDiscoveryRESTMapper(restConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to to instantiate mapper: %v", err)
	}

	// Register DatadogAgent scheme
	if err = apis.AddToScheme(scheme.Scheme); err != nil {
		return nil, fmt.Errorf("unable register DatadogAgent apis: %v", err)
	}

	// Create the Client for Read/Write operations.
	var newClient client.Client
	newClient, err = client.New(restConfig, client.Options{Scheme: scheme.Scheme, Mapper: mapper})
	if err != nil {
		return nil, fmt.Errorf("unable to instantiate client: %v", err)
	}

	return newClient, nil
}

// NewClientset returns a new client-go instance
func NewClientset(clientConfig clientcmd.ClientConfig) (*kubernetes.Clientset, error) {
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("unable to get rest client config: %v", err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to instantiate client: %v", err)
	}

	return clientset, nil
}
