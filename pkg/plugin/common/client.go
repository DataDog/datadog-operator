// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"fmt"

	"github.com/DataDog/datadog-operator/api/crds/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/crds/datadoghq/v2alpha1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
)

// NewClient returns a new controller-runtime client instance
func NewClient(clientConfig clientcmd.ClientConfig) (client.Client, error) {
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("unable to get rest client config: %w", err)
	}

	httpClient, err := rest.HTTPClientFor(restConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to create http client from rest config: %w", err)
	}

	// Create the mapper provider
	mapper, err := apiutil.NewDynamicRESTMapper(restConfig, httpClient)
	if err != nil {
		return nil, fmt.Errorf("unable to instantiate mapper: %w", err)
	}

	// Register DatadogAgent scheme
	if err = v1alpha1.AddToScheme(scheme.Scheme); err != nil {
		return nil, fmt.Errorf("unable register DatadogAgent apis: %w", err)
	}

	if err = v2alpha1.AddToScheme(scheme.Scheme); err != nil {
		return nil, fmt.Errorf("unable register DatadogAgent apis: %w", err)
	}

	// Create the Client for Read/Write operations.
	var newClient client.Client
	newClient, err = client.New(restConfig, client.Options{Scheme: scheme.Scheme, Mapper: mapper})
	if err != nil {
		return nil, fmt.Errorf("unable to instantiate client: %w", err)
	}

	return newClient, nil
}

// NewClientset returns a new client-go instance
func NewClientset(clientConfig clientcmd.ClientConfig) (*kubernetes.Clientset, error) {
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("unable to get rest client config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to instantiate client: %w", err)
	}

	return clientset, nil
}
