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

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	controllerruntime "sigs.k8s.io/controller-runtime"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

// NewDatadogAgentWithoutFeatures returns an agent without any features enabled
func NewDatadogAgentWithoutFeatures(namespace string, name string) v2alpha1.DatadogAgent {
	return newDatadogAgentWithFeatures(namespace, name, nil)
}

// NewDatadogAgentWithAdmissionController returns an agent with APM enabled
func NewDatadogAgentWithAdmissionController(namespace string, name string) v2alpha1.DatadogAgent {
	return newDatadogAgentWithFeatures(
		namespace,
		name,
		&v2alpha1.DatadogFeatures{
			AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
				Enabled:          ptr.To(true),
				MutateUnlabelled: ptr.To(true),
			},
		},
	)
}

// NewDatadogAgentWithCWSInstrumentation returns an agent with CWS Instrumentation enabled
func NewDatadogAgentWithCWSInstrumentation(namespace string, name string) v2alpha1.DatadogAgent {
	return newDatadogAgentWithFeatures(
		namespace,
		name,
		&v2alpha1.DatadogFeatures{
			AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
				Enabled:          ptr.To(true),
				MutateUnlabelled: ptr.To(true),
				CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
					Enabled: ptr.To(true),
				},
			},
		},
	)
}

// NewDatadogAgentWithAPM returns an agent with APM enabled
func NewDatadogAgentWithAPM(namespace string, name string) v2alpha1.DatadogAgent {
	return newDatadogAgentWithFeatures(
		namespace,
		name,
		&v2alpha1.DatadogFeatures{
			APM: &v2alpha1.APMFeatureConfig{
				Enabled: ptr.To(true),
				HostPortConfig: &v2alpha1.HostPortConfig{
					Enabled: ptr.To(true),
				},
			},
		},
	)
}

// NewDatadogAgentWithClusterChecks returns an agent with cluster checks enabled
func NewDatadogAgentWithClusterChecks(namespace string, name string) v2alpha1.DatadogAgent {
	return newDatadogAgentWithFeatures(
		namespace,
		name,
		&v2alpha1.DatadogFeatures{
			ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
				Enabled:                 ptr.To(true),
				UseClusterChecksRunners: ptr.To(true),
			},
		},
	)
}

// NewDatadogAgentWithCSPM returns an agent with CSPM enabled
func NewDatadogAgentWithCSPM(namespace string, name string) v2alpha1.DatadogAgent {
	return newDatadogAgentWithFeatures(
		namespace,
		name,
		&v2alpha1.DatadogFeatures{
			CSPM: &v2alpha1.CSPMFeatureConfig{
				Enabled: ptr.To(true),
				CheckInterval: &metav1.Duration{
					Duration: 1 * time.Second,
				},
			},
		},
	)
}

// NewDatadogAgentWithCWS returns an agent with CWS enabled
func NewDatadogAgentWithCWS(namespace string, name string) v2alpha1.DatadogAgent {
	return newDatadogAgentWithFeatures(
		namespace,
		name,
		&v2alpha1.DatadogFeatures{
			CWS: &v2alpha1.CWSFeatureConfig{
				Enabled:               ptr.To(true),
				SyscallMonitorEnabled: ptr.To(true),
				SecurityProfiles: &v2alpha1.CWSSecurityProfilesConfig{
					Enabled: ptr.To(true),
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
					Enabled: ptr.To(true),
					Port:    ptr.To[int32](1234),
				},
			},
		},
	)
}

// NewDatadogAgentWithEBPFCheck returns an agent with eBPF Check enabled
func NewDatadogAgentWithEBPFCheck(namespace string, name string) v2alpha1.DatadogAgent {
	return newDatadogAgentWithFeatures(
		namespace,
		name,
		&v2alpha1.DatadogFeatures{
			EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
				Enabled: ptr.To(true),
			},
		},
	)
}

// NewDatadogAgentWithServiceDiscovery returns an agent with Service Discovery enabled
func NewDatadogAgentWithServiceDiscovery(namespace, name string) v2alpha1.DatadogAgent {
	return newDatadogAgentWithFeatures(
		namespace,
		name,
		&v2alpha1.DatadogFeatures{
			ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
				Enabled: ptr.To(true),
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
				CollectKubernetesEvents: ptr.To(true),
			},
		},
	)
}

// NewDatadogAgentWithExternalMetrics returns an agent with event collection enabled
func NewDatadogAgentWithExternalMetrics(namespace string, name string) v2alpha1.DatadogAgent {
	return newDatadogAgentWithFeatures(
		namespace,
		name,
		&v2alpha1.DatadogFeatures{
			ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
				Enabled:           ptr.To(true),
				WPAController:     ptr.To(true),
				UseDatadogMetrics: ptr.To(true),
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
				Enabled: ptr.To(true),
			},
		},
	)
}

// NewDatadogAgentWithLiveContainerCollection returns an agent with live container collection enabled
func NewDatadogAgentWithLiveContainerCollection(namespace string, name string) v2alpha1.DatadogAgent {
	return newDatadogAgentWithFeatures(
		namespace,
		name,
		&v2alpha1.DatadogFeatures{
			LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
				Enabled: ptr.To(true),
			},
		},
	)
}

// NewDatadogAgentWithLiveProcessCollection returns an agent with LiveProcess collection enabled
func NewDatadogAgentWithLiveProcessCollection(namespace string, name string) v2alpha1.DatadogAgent {
	return newDatadogAgentWithFeatures(
		namespace,
		name,
		&v2alpha1.DatadogFeatures{
			LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
				Enabled: ptr.To(true),
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
				Enabled:             ptr.To(true),
				ContainerCollectAll: ptr.To(true),
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
				Enabled: ptr.To(true),
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
				Enabled: ptr.To(true),
			},
		},
	)
}

// NewDatadogAgentWithOrchestratorExplorer returns an agent with the
// orchestrator explorer enabled
func NewDatadogAgentWithOrchestratorExplorer(namespace string, name string) v2alpha1.DatadogAgent {
	return newDatadogAgentWithFeatures(
		namespace,
		name,
		&v2alpha1.DatadogFeatures{
			OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
				Enabled: ptr.To(true),
			},
		},
	)
}

// NewDatadogAgentWithOTLP returns an agent with OTLP enabled
func NewDatadogAgentWithOTLP(namespace string, name string) v2alpha1.DatadogAgent {
	return newDatadogAgentWithFeatures(
		namespace,
		name,
		&v2alpha1.DatadogFeatures{
			OTLP: &v2alpha1.OTLPFeatureConfig{
				Receiver: v2alpha1.OTLPReceiverConfig{
					Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled: ptr.To(true),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled: ptr.To(true),
						},
					},
				},
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
				Enabled: ptr.To(true),
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
				Enabled: ptr.To(true),
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
				Enabled: ptr.To(true),
			},
		},
	)
}

// NewDatadogAgentWithGPUMonitoring returns an agent with GPU monitoring enabled
func NewDatadogAgentWithGPUMonitoring(namespace string, name string) v2alpha1.DatadogAgent {
	return newDatadogAgentWithFeatures(
		namespace,
		name,
		&v2alpha1.DatadogFeatures{
			GPU: &v2alpha1.GPUFeatureConfig{
				Enabled: ptr.To(true),
			},
		},
	)
}

// NewDatadogAgentWithGlobalConfigSettings returns an agent with some global
// settings set
func NewDatadogAgentWithGlobalConfigSettings(namespace string, name string) v2alpha1.DatadogAgent {
	agent := newDatadogAgentWithFeatures(namespace, name, nil)

	// This config is invalid (incorrect URLs, paths, etc), but it's good enough
	// to verify that the operator does not crash when parsing it and using it
	// to configure some agent dependencies.
	agent.Spec.Global = &v2alpha1.GlobalConfig{
		ClusterAgentToken: ptr.To("my-cluster-agent-token"),
		ClusterName:       ptr.To("my-cluster"),
		Site:              ptr.To("some-dd-site"),
		Credentials: &v2alpha1.DatadogCredentials{
			APIKey: ptr.To("my-api-key"),
			AppKey: ptr.To("my-app-key"),
		},
		Endpoint: &v2alpha1.Endpoint{
			URL: ptr.To("some-url"),
			Credentials: &v2alpha1.DatadogCredentials{
				APIKey: ptr.To("my-api-key"),
				AppKey: ptr.To("my-app-key"),
			},
		},
		Registry: ptr.To("my-custom-registry"),
		LogLevel: ptr.To("INFO"),
		Tags:     []string{"tagA:valA", "tagB:valB"},
		Env: []v1.EnvVar{
			{
				Name:  "some-envA",
				Value: "some-valA",
			},
			{
				Name:  "some-envB",
				Value: "some-valB",
			},
		},
		PodLabelsAsTags:            map[string]string{"some-label": "some-tag"},
		PodAnnotationsAsTags:       map[string]string{"some-annotation": "some-tag"},
		NodeLabelsAsTags:           map[string]string{"some-label": "some-tag"},
		NamespaceLabelsAsTags:      map[string]string{"some-label": "some-tag"},
		NamespaceAnnotationsAsTags: map[string]string{"some-annotation": "some-tag"},
		KubernetesResourcesLabelsAsTags: map[string]map[string]string{
			"some-group.some-resource": {"some-label": "some-tag"},
		},
		KubernetesResourcesAnnotationsAsTags: map[string]map[string]string{
			"some-group.some-resource": {"some-annotation": "some-tag"},
		},
		NetworkPolicy: &v2alpha1.NetworkPolicyConfig{
			Create: ptr.To(true),
			Flavor: v2alpha1.NetworkPolicyFlavorKubernetes,
		},
		LocalService: &v2alpha1.LocalService{
			NameOverride:            ptr.To("my-local-service"),
			ForceEnableLocalService: ptr.To(true),
		},
		Kubelet: &v2alpha1.KubeletConfig{
			Host: &v1.EnvVarSource{
				FieldRef: &v1.ObjectFieldSelector{
					FieldPath: common.FieldPathSpecNodeName,
				},
			},
			TLSVerify:  ptr.To(true),
			HostCAPath: "some/path",
		},
		DockerSocketPath: ptr.To("/some/path"),
		CriSocketPath:    ptr.To("/another/path"),
	}

	return agent
}

// NewDatadogAgentWithOverrides returns an agent with overrides set
func NewDatadogAgentWithOverrides(namespace string, name string) v2alpha1.DatadogAgent {
	agent := newDatadogAgentWithFeatures(namespace, name, nil)

	// This config is invalid (non-existing images, etc.), but it's good enough
	// to verify that the operator does not crash when parsing it.

	agent.Spec.Override = make(map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride)

	agent.Spec.Override[v2alpha1.NodeAgentComponentName] = &v2alpha1.DatadogAgentComponentOverride{
		Name:               nil, // Don't override because these tests assume that it's always the default
		Replicas:           nil, // Does not apply for the node agent
		CreateRbac:         ptr.To(true),
		ServiceAccountName: ptr.To("an-overridden-sa"),
		Image: &v2alpha1.AgentImageConfig{
			Name:       "an-overridden-image-name",
			Tag:        "7",
			JMXEnabled: true,
		},
		Env: []v1.EnvVar{
			{
				Name:  "some-env",
				Value: "some-val",
			},
		},
		CustomConfigurations: nil, // This option requires creating a configmap. Set to nil here to simplify the test
		ExtraConfd:           nil, // Also requires creating a configmap
		ExtraChecksd:         nil, // Also requires creating a configmap
		Containers: map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
			apicommon.CoreAgentContainerName: {
				Name:     ptr.To("my-container-name"),
				LogLevel: ptr.To("debug"),
				Env: []v1.EnvVar{
					{
						Name:  "DD_LOG_LEVEL",
						Value: "debug",
					},
				},
				VolumeMounts: nil, // This option requires creating a configmap. Set to nil here to simplify the test
				Resources: &v1.ResourceRequirements{
					Limits: map[v1.ResourceName]resource.Quantity{
						v1.ResourceCPU: *resource.NewQuantity(2, resource.DecimalSI),
					},
					Requests: map[v1.ResourceName]resource.Quantity{
						v1.ResourceCPU: *resource.NewQuantity(1, resource.DecimalSI),
					},
				},
				Command:    []string{"test-agent", "start"},
				Args:       []string{"arg1", "val1"},
				HealthPort: ptr.To[int32](1234),
				ReadinessProbe: &v1.Probe{
					ProbeHandler: v1.ProbeHandler{
						HTTPGet: &v1.HTTPGetAction{
							Path: constants.DefaultLivenessProbeHTTPPath,
							Port: intstr.IntOrString{
								IntVal: constants.DefaultAgentHealthPort,
							},
						},
					},
					InitialDelaySeconds: 10,
					TimeoutSeconds:      5,
					PeriodSeconds:       30,
					SuccessThreshold:    1,
					FailureThreshold:    5,
				},
				LivenessProbe: &v1.Probe{
					ProbeHandler: v1.ProbeHandler{
						HTTPGet: &v1.HTTPGetAction{
							Path: constants.DefaultLivenessProbeHTTPPath,
							Port: intstr.IntOrString{
								IntVal: constants.DefaultAgentHealthPort,
							},
						},
					},
					InitialDelaySeconds: 10,
					TimeoutSeconds:      5,
					PeriodSeconds:       30,
					SuccessThreshold:    1,
					FailureThreshold:    5,
				},
				StartupProbe: &v1.Probe{
					ProbeHandler: v1.ProbeHandler{
						HTTPGet: &v1.HTTPGetAction{
							Path: constants.DefaultLivenessProbeHTTPPath,
							Port: intstr.IntOrString{
								IntVal: constants.DefaultAgentHealthPort,
							},
						},
					},
					InitialDelaySeconds: 15,
					TimeoutSeconds:      5,
					PeriodSeconds:       15,
					SuccessThreshold:    1,
					FailureThreshold:    6,
				},
				SecurityContext: &v1.SecurityContext{
					RunAsUser: ptr.To[int64](12345),
				},
				SeccompConfig: &v2alpha1.SeccompConfig{
					CustomRootPath: ptr.To("/some/path"),
					CustomProfile: &v2alpha1.CustomConfig{
						ConfigMap: &v2alpha1.ConfigMapConfig{
							Name: "custom-seccomp-cm",
						},
					},
				},
				AppArmorProfileName: ptr.To("runtime/default"),
			},
		},
		Volumes: []v1.Volume{
			{
				Name: "added-volume",
				VolumeSource: v1.VolumeSource{
					EmptyDir: &v1.EmptyDirVolumeSource{},
				},
			},
		},
		SecurityContext: &v1.PodSecurityContext{
			RunAsUser: ptr.To[int64](1234),
		},
		PriorityClassName: ptr.To("a-priority-class"),
		Affinity: &v1.Affinity{
			PodAntiAffinity: &v1.PodAntiAffinity{
				PreferredDuringSchedulingIgnoredDuringExecution: []v1.WeightedPodAffinityTerm{
					{
						Weight: 50,
						PodAffinityTerm: v1.PodAffinityTerm{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"some-label": "some-value",
								},
							},
							TopologyKey: "kubernetes.io/hostname",
						},
					},
				},
			},
		},
		NodeSelector: map[string]string{
			"key1": "val1",
		},
		Tolerations: []v1.Toleration{
			{
				Key:      "key1",
				Operator: "Exists",
				Effect:   "NoSchedule",
			},
		},
		Annotations: map[string]string{
			"an-annotation": "123",
		},
		Labels: map[string]string{
			"some-label": "456",
		},
		HostNetwork: ptr.To(false),
		HostPID:     ptr.To(true),
		Disabled:    ptr.To(false),
	}

	return agent
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
