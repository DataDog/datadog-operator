// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutils

// This file contains several functions to instantiate v2alpha1.DatadogAgent
// with different features enabled.
//
// For now, the configuration of the features is pretty basic. In most cases it
// just sets "Enabled" to true. If at some point, that's not good enough,
// evaluate whether adding more complex configs here for the integration tests
// makes sense or if those should be better tested in unit tests.

import (
	"time"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	controllerruntime "sigs.k8s.io/controller-runtime"
)

// NewDatadogAgentWithoutFeatures returns an agent without any features enabled
func NewDatadogAgentWithoutFeatures(namespace string, name string) v2alpha1.DatadogAgent {
	return newDatadogAgentWithFeatures(namespace, name, nil)
}

// NewDatadogAgentWithCSPM returns an agent with CSPM enabled
func NewDatadogAgentWithCSPM(namespace string, name string) v2alpha1.DatadogAgent {
	return newDatadogAgentWithFeatures(
		namespace,
		name,
		&v2alpha1.DatadogFeatures{
			CSPM: &v2alpha1.CSPMFeatureConfig{
				Enabled: apiutils.NewBoolPointer(true),
				CheckInterval: &metav1.Duration{
					Duration: 1 * time.Second,
				},
			},
		},
	)
}

// NewDatadogAgentWithDogstatsd returns an agent with Dogstatsd enabled
func NewDatadogAgentWithDogstatsd(namespace string, name string) v2alpha1.DatadogAgent {
	return newDatadogAgentWithFeatures(
		namespace,
		name,
		&v2alpha1.DatadogFeatures{
			Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
				HostPortConfig: &v2alpha1.HostPortConfig{
					Enabled: apiutils.NewBoolPointer(true),
					Port:    apiutils.NewInt32Pointer(1234),
				},
			},
		},
	)
}

// NewDatadogAgentWithEventCollection returns an agent with event collection enabled
func NewDatadogAgentWithEventCollection(namespace string, name string) v2alpha1.DatadogAgent {
	return newDatadogAgentWithFeatures(
		namespace,
		name,
		&v2alpha1.DatadogFeatures{
			EventCollection: &v2alpha1.EventCollectionFeatureConfig{
				CollectKubernetesEvents: apiutils.NewBoolPointer(true),
			},
		},
	)
}

// NewDatadogAgentWithKSM returns an agent with KSM enabled
func NewDatadogAgentWithKSM(namespace string, name string) v2alpha1.DatadogAgent {
	return newDatadogAgentWithFeatures(
		namespace,
		name,
		&v2alpha1.DatadogFeatures{
			KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
				Enabled: apiutils.NewBoolPointer(true),
			},
		},
	)
}

// NewDatadogAgentWithLogCollection returns an agent with log collection enabled
func NewDatadogAgentWithLogCollection(namespace string, name string) v2alpha1.DatadogAgent {
	return newDatadogAgentWithFeatures(
		namespace,
		name,
		&v2alpha1.DatadogFeatures{
			LogCollection: &v2alpha1.LogCollectionFeatureConfig{
				Enabled:             apiutils.NewBoolPointer(true),
				ContainerCollectAll: apiutils.NewBoolPointer(true),
			},
		},
	)
}

// NewDatadogAgentWithNPM returns an agent with NPM enabled
func NewDatadogAgentWithNPM(namespace string, name string) v2alpha1.DatadogAgent {
	return newDatadogAgentWithFeatures(
		namespace,
		name,
		&v2alpha1.DatadogFeatures{
			NPM: &v2alpha1.NPMFeatureConfig{
				Enabled: apiutils.NewBoolPointer(true),
			},
		},
	)
}

// NewDatadogAgentWithOOMKill returns an agent with OOM kill enabled
func NewDatadogAgentWithOOMKill(namespace string, name string) v2alpha1.DatadogAgent {
	return newDatadogAgentWithFeatures(
		namespace,
		name,
		&v2alpha1.DatadogFeatures{
			OOMKill: &v2alpha1.OOMKillFeatureConfig{
				Enabled: apiutils.NewBoolPointer(true),
			},
		},
	)
}

// NewDatadogAgentWithPrometheusScrape returns an agent with Prometheus scraping enabled
func NewDatadogAgentWithPrometheusScrape(namespace string, name string) v2alpha1.DatadogAgent {
	return newDatadogAgentWithFeatures(
		namespace,
		name,
		&v2alpha1.DatadogFeatures{
			PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
				Enabled: apiutils.NewBoolPointer(true),
			},
		},
	)
}

// NewDatadogAgentWithTCPQueueLength returns an agent with TCP queue length enabled
func NewDatadogAgentWithTCPQueueLength(namespace string, name string) v2alpha1.DatadogAgent {
	return newDatadogAgentWithFeatures(
		namespace,
		name,
		&v2alpha1.DatadogFeatures{
			TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
				Enabled: apiutils.NewBoolPointer(true),
			},
		},
	)
}

// NewDatadogAgentWithUSM returns an agent with USM enabled
func NewDatadogAgentWithUSM(namespace string, name string) v2alpha1.DatadogAgent {
	return newDatadogAgentWithFeatures(
		namespace,
		name,
		&v2alpha1.DatadogFeatures{
			USM: &v2alpha1.USMFeatureConfig{
				Enabled: apiutils.NewBoolPointer(true),
			},
		},
	)
}

func newDatadogAgentWithFeatures(namespace string, name string, features *v2alpha1.DatadogFeatures) v2alpha1.DatadogAgent {
	apiKey := "my-api-key"
	appKey := "my-app-key"

	return v2alpha1.DatadogAgent{
		ObjectMeta: controllerruntime.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: v2alpha1.DatadogAgentSpec{
			Global: &v2alpha1.GlobalConfig{
				Credentials: &v2alpha1.DatadogCredentials{
					APIKey: &apiKey,
					AppKey: &appKey,
				},
			},
			Features: features,
		},
	}
}
