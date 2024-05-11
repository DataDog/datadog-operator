// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package common

import (
	"fmt"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	"github.com/DataDog/extendeddaemonset/api/v1alpha1"
)

// NewClient returns new client instance.
func NewClient(clientConfig clientcmd.ClientConfig) (client.Client, error) {
	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("unable to get rest client config, err: %w", err)
	}

	// earlier version of the refactor
	// https://github.com/kubernetes-sigs/controller-runtime/pull/2122/files#diff-964e351ee2375d359c78d69e514c4edc42577219761c4475f391ed2daf715e51R427
	httpClient, err := rest.HTTPClientFor(restConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to create http client from rest config, err: %w", err)
	}

	// Create the mapper provider.
	mapper, err := apiutil.NewDiscoveryRESTMapper(restConfig, httpClient)
	if err != nil {
		return nil, fmt.Errorf("unable to instantiate mapper, err: %w", err)
	}

	if err = v1alpha1.AddToScheme(scheme.Scheme); err != nil {
		return nil, fmt.Errorf("unable register ExtendedDaemonset apis, err: %w", err)
	}
	// Create the Client for Read/Write operations.
	var newClient client.Client
	newClient, err = client.New(restConfig, client.Options{Scheme: scheme.Scheme, Mapper: mapper})
	if err != nil {
		return nil, fmt.Errorf("unable to instantiate client, err: %w", err)
	}

	return newClient, nil
}
