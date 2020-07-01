// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package utils

import (
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewDatadogAgentOptions used to provide creation options to the NewDatadogAgent function
type NewDatadogAgentOptions struct {
	ExtraLabels         map[string]string
	ExtraAnnotations    map[string]string
	ClusterAgentEnabled bool
	UseEDS              bool
	APIKey              string
}

var (
	pullPolicy = v1.PullIfNotPresent
)

// NewDatadogAgent returns new DatadogAgent instance with is config hash
func NewDatadogAgent(ns, name, image string, options *NewDatadogAgentOptions) *datadoghqv1alpha1.DatadogAgent {
	ad := &datadoghqv1alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}
	ad.Spec = datadoghqv1alpha1.DatadogAgentSpec{
		Credentials: datadoghqv1alpha1.AgentCredentials{
			APIKey: "adflkajdflkjalkcmlkdjacsf",
			AppKey: "sgfggtdhfghfghfghfgbdfdgs",
		},
		Agent: &datadoghqv1alpha1.DatadogAgentSpecAgentSpec{
			Image: datadoghqv1alpha1.ImageConfig{},
			Config: datadoghqv1alpha1.NodeAgentConfig{
				Resources: &v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("0"),
						v1.ResourceMemory: resource.MustParse("0"),
					},
				},
				CriSocket: &datadoghqv1alpha1.CRISocketConfig{
					CriSocketPath: datadoghqv1alpha1.NewStringPointer("/var/run/containerd/containerd.sock"),
				},
				Env: []v1.EnvVar{
					{
						Name:  "DD_KUBELET_TLS_VERIFY",
						Value: "false",
					},
				},
				LeaderElection: datadoghqv1alpha1.NewBoolPointer(true),
			},
			DeploymentStrategy: &datadoghqv1alpha1.DaemonSetDeploymentStrategy{},
			Apm:                datadoghqv1alpha1.APMSpec{},
			Log:                datadoghqv1alpha1.LogSpec{},
			Process:            datadoghqv1alpha1.ProcessSpec{},
		},
	}
	ad = datadoghqv1alpha1.DefaultDatadogAgent(ad)
	ad.Spec.Agent.Image = datadoghqv1alpha1.ImageConfig{
		Name:        image,
		PullPolicy:  &pullPolicy,
		PullSecrets: &[]v1.LocalObjectReference{},
	}
	ad.Spec.Agent.Rbac.Create = datadoghqv1alpha1.NewBoolPointer(true)

	if options != nil {
		if options.APIKey != "" {
			ad.Spec.Credentials.APIKey = options.APIKey
		}

		if options.UseEDS && ad.Spec.Agent != nil {
			ad.Spec.Agent.UseExtendedDaemonset = &options.UseEDS
		}

		if options.ExtraLabels != nil {
			if ad.Labels == nil {
				ad.Labels = map[string]string{}
			}
			for key, val := range options.ExtraLabels {
				ad.Labels[key] = val
			}
		}

		if options.ExtraAnnotations != nil {
			if ad.Annotations == nil {
				ad.Annotations = map[string]string{}
			}
			for key, val := range options.ExtraAnnotations {
				ad.Annotations[key] = val
			}
		}

		if options.ClusterAgentEnabled {
			ad.Spec.ClusterAgent = &datadoghqv1alpha1.DatadogAgentSpecClusterAgentSpec{
				Config: datadoghqv1alpha1.ClusterAgentConfig{},
				Image: datadoghqv1alpha1.ImageConfig{
					Name:        image,
					PullPolicy:  &pullPolicy,
					PullSecrets: &[]v1.LocalObjectReference{},
				},
			}
		}
	}

	return ad
}
