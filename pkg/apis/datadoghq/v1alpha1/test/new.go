// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package test

import (
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	edsdatadoghqv1alpha1 "github.com/datadog/extendeddaemonset/pkg/apis/datadoghq/v1alpha1"
)

var (
	// apiVersion datadoghqv1alpha1 api version
	apiVersion = fmt.Sprintf("%s/%s", datadoghqv1alpha1.SchemeGroupVersion.Group, datadoghqv1alpha1.SchemeGroupVersion.Version)
	pullPolicy = v1.PullIfNotPresent
)

// NewDatadogAgentDeploymentOptions set of option for the DatadogAgentDeployment creation
type NewDatadogAgentDeploymentOptions struct {
	Labels                     map[string]string
	Annotations                map[string]string
	Status                     *datadoghqv1alpha1.DatadogAgentDeploymentStatus
	UseEDS                     bool
	ClusterAgentEnabled        bool
	MetricsServerEnabled       bool
	ClusterChecksRunnerEnabled bool
	NodeAgentConfig            *datadoghqv1alpha1.NodeAgentConfig
	APMEnabled                 bool
	ProcessEnabled             bool
	SystemProbeEnabled         bool
}

// NewDefaultedDatadogAgentDeployment returns an initialized and defaulted DatadogAgentDeployment for testing purpose
func NewDefaultedDatadogAgentDeployment(ns, name string, options *NewDatadogAgentDeploymentOptions) *datadoghqv1alpha1.DatadogAgentDeployment {
	ad := &datadoghqv1alpha1.DatadogAgentDeployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogAgentDeployment",
			APIVersion: apiVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
			Labels:    map[string]string{},
		},
	}
	ad.Spec = datadoghqv1alpha1.DatadogAgentDeploymentSpec{
		Credentials: datadoghqv1alpha1.AgentCredentials{Token: "token-foo"},
		Agent: &datadoghqv1alpha1.DatadogAgentDeploymentSpecAgentSpec{
			Image: datadoghqv1alpha1.ImageConfig{
				Name:       "datadog/agent:latest",
				PullPolicy: &pullPolicy,
			},
			Config:             datadoghqv1alpha1.NodeAgentConfig{},
			DeploymentStrategy: &datadoghqv1alpha1.DaemonSetDeploymentcStrategy{},
			Apm:                datadoghqv1alpha1.APMSpec{},
			Log:                datadoghqv1alpha1.LogSpec{},
			Process:            datadoghqv1alpha1.ProcessSpec{},
		},
	}
	if options != nil {
		if options.UseEDS {
			ad.Spec.Agent.UseExtendedDaemonset = &options.UseEDS
		}
		if options.Labels != nil {
			for key, value := range options.Labels {
				ad.Labels[key] = value
			}
		}
		if options.Annotations != nil {
			ad.Annotations = make(map[string]string)
			for key, value := range options.Annotations {
				ad.Annotations[key] = value
			}
		}
		if options.Status != nil {
			ad.Status = *options.Status
		}
		if options.ClusterAgentEnabled {
			ad.Spec.ClusterAgent = &datadoghqv1alpha1.DatadogAgentDeploymentSpecClusterAgentSpec{
				Config: datadoghqv1alpha1.ClusterAgentConfig{},
				Rbac: datadoghqv1alpha1.RbacConfig{
					Create: datadoghqv1alpha1.NewBoolPointer(true),
				},
			}
			if options.MetricsServerEnabled {
				ad.Spec.ClusterAgent.Config.MetricsProviderEnabled = datadoghqv1alpha1.NewBoolPointer(true)
			}
			if options.ClusterChecksRunnerEnabled {
				ad.Spec.ClusterAgent.Config.ClusterChecksRunnerEnabled = datadoghqv1alpha1.NewBoolPointer(true)
			}
		}

		if options.NodeAgentConfig != nil {
			ad.Spec.Agent.Config = *options.NodeAgentConfig
		}

		if options.APMEnabled {
			ad.Spec.Agent.Apm.Enabled = datadoghqv1alpha1.NewBoolPointer(true)
		}

		if options.ProcessEnabled {
			ad.Spec.Agent.Process.Enabled = datadoghqv1alpha1.NewBoolPointer(true)
		}

		if options.SystemProbeEnabled {
			ad.Spec.Agent.SystemProbe.Enabled = datadoghqv1alpha1.NewBoolPointer(true)
		}

	}
	return datadoghqv1alpha1.DefaultDatadogAgentDeployment(ad)
}

// NewExtendedDaemonSetOptions set of option for the ExtendedDaemonset creation
type NewExtendedDaemonSetOptions struct {
	CreationTime *time.Time
	Annotations  map[string]string
	Labels       map[string]string
	Canary       *edsdatadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanary
	Status       *edsdatadoghqv1alpha1.ExtendedDaemonSetStatus
}

// NewExtendedDaemonSet return new ExtendedDDaemonset instance for testing purpose
func NewExtendedDaemonSet(ns, name string, options *NewExtendedDaemonSetOptions) *edsdatadoghqv1alpha1.ExtendedDaemonSet {
	dd := &edsdatadoghqv1alpha1.ExtendedDaemonSet{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ExtendedDaemonSet",
			APIVersion: apiVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   ns,
			Name:        name,
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
	}
	if options != nil {
		if options.CreationTime != nil {
			dd.CreationTimestamp = metav1.NewTime(*options.CreationTime)
		}
		if options.Annotations != nil {
			for key, value := range options.Annotations {
				dd.Annotations[key] = value
			}
		}
		if options.Labels != nil {
			for key, value := range options.Labels {
				dd.Labels[key] = value
			}
		}
		if options.Canary != nil {
			dd.Spec.Strategy.Canary = options.Canary
		}
		if options.Status != nil {
			dd.Status = *options.Status
		}
	}

	return dd
}

// NewDeploymentOptions set of option for the Deployment creation
type NewDeploymentOptions struct {
	CreationTime *time.Time
	Annotations  map[string]string
	Labels       map[string]string
}

// NewClusterAgentDeployment return new Cluster Agent Deployment instance for testing purpose
func NewClusterAgentDeployment(ns, name string, options *NewDeploymentOptions) *appsv1.Deployment {
	dca := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Deployment",
			APIVersion: "apps/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   ns,
			Name:        fmt.Sprintf("%s-%s", name, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix),
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
	}
	if options != nil {
		if options.CreationTime != nil {
			dca.CreationTimestamp = metav1.NewTime(*options.CreationTime)
		}
		if options.Annotations != nil {
			for key, value := range options.Annotations {
				dca.Annotations[key] = value
			}
		}
		if options.Labels != nil {
			for key, value := range options.Labels {
				dca.Labels[key] = value
			}
		}
	}

	return dca
}

type NewSecretOptions struct {
	CreationTime *time.Time
	Annotations  map[string]string
	Labels       map[string]string
	Data         map[string][]byte
}

func NewSecret(ns, name string, opts *NewSecretOptions) *corev1.Secret {
	secret := &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Secret",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   ns,
			Name:        name,
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
		Type: corev1.SecretTypeOpaque,
	}

	if opts != nil {
		if opts.CreationTime != nil {
			secret.CreationTimestamp = metav1.NewTime(*opts.CreationTime)
		}
		if opts.Labels != nil {
			secret.Labels = opts.Labels
		}
		if opts.Annotations != nil {
			secret.Annotations = opts.Annotations
		}
		if opts.Data != nil {
			secret.Data = opts.Data
		}
	}

	return secret
}

type NewServiceOptions struct {
	CreationTime *time.Time
	Annotations  map[string]string
	Labels       map[string]string
	Spec         *corev1.ServiceSpec
}

func NewService(ns, name string, opts *NewServiceOptions) *corev1.Service {
	service := &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:   ns,
			Name:        name,
			Labels:      map[string]string{},
			Annotations: map[string]string{},
		},
	}

	if opts != nil {
		if opts.CreationTime != nil {
			service.CreationTimestamp = metav1.NewTime(*opts.CreationTime)
		}
		if opts.Labels != nil {
			service.Labels = opts.Labels
		}
		if opts.Annotations != nil {
			service.Annotations = opts.Annotations
		}
		if opts.Spec != nil {
			service.Spec = *opts.Spec
		}
	}

	return service
}
