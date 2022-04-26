// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutils

import (
	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewDatadogAgentOptions used to provide creation options to the NewDatadogAgent function
type NewDatadogAgentOptions struct {
	ExtraLabels                  map[string]string
	ExtraAnnotations             map[string]string
	AgentDisabled                bool
	ClusterAgentDisabled         bool
	OrchestratorExplorerDisabled bool
	UseEDS                       bool
	APIKey                       string
	AppKey                       string
	Token                        string
	CustomConfig                 *datadoghqv1alpha1.CustomConfigSpec
	SecuritySpec                 *datadoghqv1alpha1.SecuritySpec
	VolumeMounts                 []v1.VolumeMount
}

var pullPolicy = v1.PullIfNotPresent

// NewDatadogAgent returns new DatadogAgent instance with is config hash
func NewDatadogAgent(ns, name, image string, options *NewDatadogAgentOptions) *datadoghqv1alpha1.DatadogAgent {
	ad := &datadoghqv1alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
	}

	ad.Spec = datadoghqv1alpha1.DatadogAgentSpec{
		Credentials: &datadoghqv1alpha1.AgentCredentials{
			DatadogCredentials: datadoghqv1alpha1.DatadogCredentials{
				APIKey: "",
				AppKey: "",
			},
		},
		Agent: datadoghqv1alpha1.DatadogAgentSpecAgentSpec{
			Image: &commonv1.AgentImageConfig{},
			Config: &datadoghqv1alpha1.NodeAgentConfig{
				Resources: &v1.ResourceRequirements{
					Requests: v1.ResourceList{
						v1.ResourceCPU:    resource.MustParse("0"),
						v1.ResourceMemory: resource.MustParse("0"),
					},
				},
				CriSocket: &datadoghqv1alpha1.CRISocketConfig{
					CriSocketPath: apiutils.NewStringPointer("/var/run/containerd/containerd.sock"),
				},
				Env: []v1.EnvVar{
					{
						Name:  "DD_KUBELET_TLS_VERIFY",
						Value: "false",
					},
				},
				LeaderElection: apiutils.NewBoolPointer(true),
			},
			DeploymentStrategy: &datadoghqv1alpha1.DaemonSetDeploymentStrategy{},
			Apm:                &datadoghqv1alpha1.APMSpec{},
			Process: &datadoghqv1alpha1.ProcessSpec{
				Enabled:                  apiutils.NewBoolPointer(false),
				ProcessCollectionEnabled: apiutils.NewBoolPointer(false),
			},
		},
	}
	_ = datadoghqv1alpha1.DefaultDatadogAgent(ad)
	ad.Spec.Agent.Image = &commonv1.AgentImageConfig{
		Name:        image,
		PullPolicy:  &pullPolicy,
		PullSecrets: &[]v1.LocalObjectReference{},
	}
	ad.Spec.Agent.Rbac.Create = apiutils.NewBoolPointer(true)

	if options != nil {
		if options.APIKey != "" {
			ad.Spec.Credentials.APIKey = options.APIKey
		}

		if options.AppKey != "" {
			ad.Spec.Credentials.AppKey = options.AppKey
		}

		if options.Token != "" {
			ad.Spec.Credentials.Token = options.Token
		}

		if options.AgentDisabled {
			ad.Spec.Agent.Enabled = apiutils.NewBoolPointer(false)
		}

		if options.UseEDS && apiutils.BoolValue(ad.Spec.Agent.Enabled) {
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

		if !options.ClusterAgentDisabled {
			ad.Spec.ClusterAgent = datadoghqv1alpha1.DatadogAgentSpecClusterAgentSpec{
				Config: &datadoghqv1alpha1.ClusterAgentConfig{},
				Image: &commonv1.AgentImageConfig{
					Name:        image,
					PullPolicy:  &pullPolicy,
					PullSecrets: &[]v1.LocalObjectReference{},
				},
			}
		} else {
			ad.Spec.ClusterAgent = datadoghqv1alpha1.DatadogAgentSpecClusterAgentSpec{
				Enabled: apiutils.NewBoolPointer(false),
			}
		}

		ad.Spec.Agent.Config.VolumeMounts = options.VolumeMounts
		ad.Spec.Agent.Process.VolumeMounts = options.VolumeMounts
		ad.Spec.Agent.Apm.VolumeMounts = options.VolumeMounts

		if options.CustomConfig != nil {
			ad.Spec.Agent.CustomConfig = options.CustomConfig
		}

		if options.SecuritySpec != nil {
			ad.Spec.Agent.Security = options.SecuritySpec
		} else {
			ad.Spec.Agent.Security.VolumeMounts = options.VolumeMounts
		}

		if options.OrchestratorExplorerDisabled {
			if ad.Spec.Features.OrchestratorExplorer == nil {
				ad.Spec.Features.OrchestratorExplorer = &datadoghqv1alpha1.OrchestratorExplorerConfig{}
			}

			ad.Spec.Features.OrchestratorExplorer.Enabled = apiutils.NewBoolPointer(false)
		}
		// options can have an impact on the defaulting
		_ = datadoghqv1alpha1.DefaultDatadogAgent(ad)
	}

	return ad
}
