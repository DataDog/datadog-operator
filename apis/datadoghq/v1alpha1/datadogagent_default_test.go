// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"path"
	"testing"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/pkg/defaulting"
	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"

	"github.com/google/go-cmp/cmp"
	assert "github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestDefaultConfigDogstatsd(t *testing.T) {
	defaultPath := path.Join(defaultHostDogstatsdSocketPath, defaultHostDogstatsdSocketName)
	tests := []struct {
		name     string
		dsd      NodeAgentConfig
		override *DogstatsdConfig
		internal NodeAgentConfig
	}{
		{
			name: "empty conf",
			dsd:  NodeAgentConfig{},
			override: &DogstatsdConfig{
				DogstatsdOriginDetection: apiutils.NewBoolPointer(false), // defaultDogstatsdOriginDetection
				UnixDomainSocket: &DSDUnixDomainSocketSpec{
					Enabled:      apiutils.NewBoolPointer(false),
					HostFilepath: &defaultPath,
				},
			},
			internal: NodeAgentConfig{
				Dogstatsd: &DogstatsdConfig{
					DogstatsdOriginDetection: apiutils.NewBoolPointer(false),
					UnixDomainSocket: &DSDUnixDomainSocketSpec{
						Enabled:      apiutils.NewBoolPointer(false),
						HostFilepath: &defaultPath,
					},
				},
			},
		},
		{
			name: "dogtatsd missing defaulting: DogstatsdOriginDetection",
			dsd: NodeAgentConfig{
				Dogstatsd: &DogstatsdConfig{
					UnixDomainSocket: &DSDUnixDomainSocketSpec{
						Enabled:      apiutils.NewBoolPointer(false),
						HostFilepath: &defaultPath,
					},
				},
			},
			override: &DogstatsdConfig{
				DogstatsdOriginDetection: apiutils.NewBoolPointer(false),
			},
			internal: NodeAgentConfig{
				Dogstatsd: &DogstatsdConfig{
					DogstatsdOriginDetection: apiutils.NewBoolPointer(false),
					UnixDomainSocket: &DSDUnixDomainSocketSpec{
						Enabled:      apiutils.NewBoolPointer(false),
						HostFilepath: &defaultPath,
					},
				},
			},
		},
		{
			name: "dogtatsd missing defaulting: UseDogStatsDSocketVolume",
			dsd: NodeAgentConfig{
				Dogstatsd: &DogstatsdConfig{
					DogstatsdOriginDetection: apiutils.NewBoolPointer(false),
				},
			},
			override: &DogstatsdConfig{
				UnixDomainSocket: &DSDUnixDomainSocketSpec{
					Enabled:      apiutils.NewBoolPointer(false),
					HostFilepath: &defaultPath,
				},
			},
			internal: NodeAgentConfig{
				Dogstatsd: &DogstatsdConfig{
					DogstatsdOriginDetection: apiutils.NewBoolPointer(false),
					UnixDomainSocket: &DSDUnixDomainSocketSpec{
						Enabled:      apiutils.NewBoolPointer(false),
						HostFilepath: &defaultPath,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultConfigDogstatsd(&tt.dsd)
			assert.True(t, apiutils.IsEqualStruct(got, tt.override), "TestDefaultFeatures override \ndiff = %s", cmp.Diff(got, tt.override))
			assert.True(t, apiutils.IsEqualStruct(tt.dsd, tt.internal), "TestDefaultFeatures internal \ndiff = %s", cmp.Diff(tt.dsd, tt.internal))
		})
	}
}

func TestDefaultFeatures(t *testing.T) {
	tests := []struct {
		name              string
		ft                DatadogFeatures
		overrideExpected  *DatadogFeatures
		internalDefaulted DatadogFeatures
	}{
		{
			name: "empty fields",
			ft:   DatadogFeatures{},
			overrideExpected: &DatadogFeatures{
				OrchestratorExplorer: &OrchestratorExplorerConfig{
					Enabled:      apiutils.NewBoolPointer(true),
					ClusterCheck: apiutils.NewBoolPointer(false),
					Scrubbing:    &Scrubbing{Containers: apiutils.NewBoolPointer(true)},
				},
				KubeStateMetricsCore: &KubeStateMetricsCore{Enabled: apiutils.NewBoolPointer(false), ClusterCheck: apiutils.NewBoolPointer(false)},
				PrometheusScrape: &PrometheusScrapeConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
				LogCollection: &LogCollectionConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
				NetworkMonitoring: &NetworkMonitoringConfig{Enabled: apiutils.NewBoolPointer(false)},
			},
			internalDefaulted: DatadogFeatures{
				OrchestratorExplorer: &OrchestratorExplorerConfig{
					Enabled:      apiutils.NewBoolPointer(true),
					ClusterCheck: apiutils.NewBoolPointer(false),
					Scrubbing:    &Scrubbing{Containers: apiutils.NewBoolPointer(true)},
				},
				KubeStateMetricsCore: &KubeStateMetricsCore{Enabled: apiutils.NewBoolPointer(false), ClusterCheck: apiutils.NewBoolPointer(false)},
				PrometheusScrape: &PrometheusScrapeConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
				LogCollection: &LogCollectionConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
				NetworkMonitoring: &NetworkMonitoringConfig{Enabled: apiutils.NewBoolPointer(false)},
			},
		},
		{
			name: "sparse config",
			ft: DatadogFeatures{
				OrchestratorExplorer: &OrchestratorExplorerConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
				LogCollection: &LogCollectionConfig{
					Enabled: apiutils.NewBoolPointer(true),
				},
			},
			overrideExpected: &DatadogFeatures{
				OrchestratorExplorer: &OrchestratorExplorerConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
				KubeStateMetricsCore: &KubeStateMetricsCore{Enabled: apiutils.NewBoolPointer(false), ClusterCheck: apiutils.NewBoolPointer(false)},
				LogCollection: &LogCollectionConfig{
					Enabled:                       apiutils.NewBoolPointer(true),
					LogsConfigContainerCollectAll: apiutils.NewBoolPointer(false),
					ContainerCollectUsingFiles:    apiutils.NewBoolPointer(true),
					ContainerLogsPath:             apiutils.NewStringPointer("/var/lib/docker/containers"),
					PodLogsPath:                   apiutils.NewStringPointer("/var/log/pods"),
					ContainerSymlinksPath:         apiutils.NewStringPointer("/var/log/containers"),
					TempStoragePath:               apiutils.NewStringPointer("/var/lib/datadog-agent/logs"),
				},
				PrometheusScrape: &PrometheusScrapeConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
				NetworkMonitoring: &NetworkMonitoringConfig{Enabled: apiutils.NewBoolPointer(false)},
			},
			internalDefaulted: DatadogFeatures{
				OrchestratorExplorer: &OrchestratorExplorerConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
				KubeStateMetricsCore: &KubeStateMetricsCore{Enabled: apiutils.NewBoolPointer(false), ClusterCheck: apiutils.NewBoolPointer(false)},
				LogCollection: &LogCollectionConfig{
					Enabled:                       apiutils.NewBoolPointer(true),
					LogsConfigContainerCollectAll: apiutils.NewBoolPointer(false),
					ContainerCollectUsingFiles:    apiutils.NewBoolPointer(true),
					ContainerLogsPath:             apiutils.NewStringPointer("/var/lib/docker/containers"),
					PodLogsPath:                   apiutils.NewStringPointer("/var/log/pods"),
					ContainerSymlinksPath:         apiutils.NewStringPointer("/var/log/containers"),
					TempStoragePath:               apiutils.NewStringPointer("/var/lib/datadog-agent/logs"),
				},
				PrometheusScrape: &PrometheusScrapeConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
				NetworkMonitoring: &NetworkMonitoringConfig{Enabled: apiutils.NewBoolPointer(false)},
			},
		},
		{
			name: "some config",
			ft: DatadogFeatures{
				OrchestratorExplorer: &OrchestratorExplorerConfig{
					Scrubbing: &Scrubbing{Containers: apiutils.NewBoolPointer(false)},
				},
				KubeStateMetricsCore: &KubeStateMetricsCore{Enabled: apiutils.NewBoolPointer(true), ClusterCheck: apiutils.NewBoolPointer(true)},
				LogCollection: &LogCollectionConfig{
					LogsConfigContainerCollectAll: apiutils.NewBoolPointer(false),
					ContainerLogsPath:             apiutils.NewStringPointer("/var/lib/docker/containers"),
				},
				PrometheusScrape: &PrometheusScrapeConfig{
					ServiceEndpoints: apiutils.NewBoolPointer(true),
				},
				NetworkMonitoring: &NetworkMonitoringConfig{Enabled: apiutils.NewBoolPointer(true)},
			},
			overrideExpected: &DatadogFeatures{
				OrchestratorExplorer: &OrchestratorExplorerConfig{
					Enabled:      apiutils.NewBoolPointer(true), // defaultOrchestratorExplorerEnabled
					ClusterCheck: apiutils.NewBoolPointer(false),
				},
				KubeStateMetricsCore: &KubeStateMetricsCore{
					Enabled:      apiutils.NewBoolPointer(true),
					ClusterCheck: apiutils.NewBoolPointer(true),
				},
				LogCollection: &LogCollectionConfig{
					Enabled: apiutils.NewBoolPointer(false), // defaultLogEnabled
				},
				PrometheusScrape: &PrometheusScrapeConfig{
					Enabled: apiutils.NewBoolPointer(false), // defaultPrometheusScrapeEnabled
				},
				NetworkMonitoring: &NetworkMonitoringConfig{Enabled: apiutils.NewBoolPointer(true)},
			},
			internalDefaulted: DatadogFeatures{
				OrchestratorExplorer: &OrchestratorExplorerConfig{
					Enabled:      apiutils.NewBoolPointer(true), // defaultOrchestratorExplorerEnabled
					ClusterCheck: apiutils.NewBoolPointer(false),
					Scrubbing:    &Scrubbing{Containers: apiutils.NewBoolPointer(false)},
				},
				KubeStateMetricsCore: &KubeStateMetricsCore{Enabled: apiutils.NewBoolPointer(true), ClusterCheck: apiutils.NewBoolPointer(true)},
				LogCollection: &LogCollectionConfig{
					Enabled:                       apiutils.NewBoolPointer(false),
					LogsConfigContainerCollectAll: apiutils.NewBoolPointer(false),
					ContainerLogsPath:             apiutils.NewStringPointer("/var/lib/docker/containers"),
				},
				PrometheusScrape: &PrometheusScrapeConfig{
					Enabled:          apiutils.NewBoolPointer(false),
					ServiceEndpoints: apiutils.NewBoolPointer(true),
				},
				NetworkMonitoring: &NetworkMonitoringConfig{Enabled: apiutils.NewBoolPointer(true)},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda := &DatadogAgent{}
			dda.Spec.Features = tt.ft
			got := DefaultFeatures(dda)
			assert.True(t, apiutils.IsEqualStruct(got, tt.overrideExpected), "TestDefaultFeatures override \ndiff = %s", cmp.Diff(got, tt.overrideExpected))
			assert.True(t, apiutils.IsEqualStruct(dda.Spec.Features, tt.internalDefaulted), "TestDefaultFeatures internal \ndiff = %s", cmp.Diff(dda.Spec.Features, tt.internalDefaulted))
		})
	}
}

func TestDefaultDatadogAgentSpecClusterAgent(t *testing.T) {
	tests := []struct {
		name              string
		dca               DatadogAgentSpecClusterAgentSpec
		overrideExpected  *DatadogAgentSpecClusterAgentSpec
		internalDefaulted DatadogAgentSpecClusterAgentSpec
	}{
		{
			name: "disable field",
			dca: DatadogAgentSpecClusterAgentSpec{
				Enabled: apiutils.NewBoolPointer(false),
			},
			overrideExpected: &DatadogAgentSpecClusterAgentSpec{},
			internalDefaulted: DatadogAgentSpecClusterAgentSpec{
				Enabled: apiutils.NewBoolPointer(false),
			},
		},
		{
			name: "some config",
			dca: DatadogAgentSpecClusterAgentSpec{
				Config: &ClusterAgentConfig{
					AdmissionController: &AdmissionControllerConfig{
						Enabled: apiutils.NewBoolPointer(true),
					},
				},
			},
			overrideExpected: &DatadogAgentSpecClusterAgentSpec{
				Enabled: apiutils.NewBoolPointer(true),
				Image: &commonv1.AgentImageConfig{
					Name:       defaultClusterAgentImageName,
					Tag:        defaulting.ClusterAgentLatestVersion,
					PullPolicy: &defaultImagePullPolicy,
				},
				Config: &ClusterAgentConfig{
					ExternalMetrics: &ExternalMetricsConfig{
						Enabled: apiutils.NewBoolPointer(false),
					},
					AdmissionController: &AdmissionControllerConfig{
						MutateUnlabelled: apiutils.NewBoolPointer(false),
						ServiceName:      apiutils.NewStringPointer("datadog-admission-controller"),
					},
					ClusterChecksEnabled: apiutils.NewBoolPointer(false),
					LogLevel:             apiutils.NewStringPointer(defaultLogLevel),
					CollectEvents:        apiutils.NewBoolPointer(false),
					HealthPort:           apiutils.NewInt32Pointer(5555),
				},
				Rbac:          &RbacConfig{Create: apiutils.NewBoolPointer(true)},
				Replicas:      nil,
				NetworkPolicy: &NetworkPolicySpec{Create: apiutils.NewBoolPointer(false)},
			},
			internalDefaulted: DatadogAgentSpecClusterAgentSpec{
				Enabled: apiutils.NewBoolPointer(true),
				Image: &commonv1.AgentImageConfig{
					Name:        defaultClusterAgentImageName,
					Tag:         defaulting.ClusterAgentLatestVersion,
					PullPolicy:  &defaultImagePullPolicy,
					PullSecrets: &[]corev1.LocalObjectReference{},
				},
				Config: &ClusterAgentConfig{
					ExternalMetrics: &ExternalMetricsConfig{
						Enabled: apiutils.NewBoolPointer(false),
					},
					AdmissionController: &AdmissionControllerConfig{
						Enabled:          apiutils.NewBoolPointer(true),
						MutateUnlabelled: apiutils.NewBoolPointer(false),
						ServiceName:      apiutils.NewStringPointer("datadog-admission-controller"),
					},
					Resources:            &corev1.ResourceRequirements{Limits: corev1.ResourceList{}, Requests: corev1.ResourceList{}},
					LogLevel:             apiutils.NewStringPointer(defaultLogLevel),
					ClusterChecksEnabled: apiutils.NewBoolPointer(false),
					CollectEvents:        apiutils.NewBoolPointer(false),
					HealthPort:           apiutils.NewInt32Pointer(5555),
				},
				Rbac:          &RbacConfig{Create: apiutils.NewBoolPointer(true)},
				Replicas:      nil,
				NetworkPolicy: &NetworkPolicySpec{Create: apiutils.NewBoolPointer(false)},
			},
		},
		{
			name: "almost full config",
			dca: DatadogAgentSpecClusterAgentSpec{
				Enabled: apiutils.NewBoolPointer(true),
				Image: &commonv1.AgentImageConfig{
					Name:       "foo",
					PullPolicy: (*corev1.PullPolicy)(apiutils.NewStringPointer("Always")),
				},
				Config: &ClusterAgentConfig{
					ExternalMetrics: &ExternalMetricsConfig{
						Enabled:           apiutils.NewBoolPointer(true),
						WpaController:     true,
						UseDatadogMetrics: true,
					},
					LogLevel: apiutils.NewStringPointer("DEBUG"),
					AdmissionController: &AdmissionControllerConfig{
						Enabled:          apiutils.NewBoolPointer(true),
						MutateUnlabelled: apiutils.NewBoolPointer(false),
						ServiceName:      apiutils.NewStringPointer("foo"),
					},
					ClusterChecksEnabled: apiutils.NewBoolPointer(true),
					CollectEvents:        apiutils.NewBoolPointer(false),
					HealthPort:           apiutils.NewInt32Pointer(5555),
				},
				Rbac:          &RbacConfig{Create: apiutils.NewBoolPointer(false)},
				Replicas:      apiutils.NewInt32Pointer(2),
				NetworkPolicy: &NetworkPolicySpec{Create: apiutils.NewBoolPointer(true)},
			},
			overrideExpected: &DatadogAgentSpecClusterAgentSpec{
				Image: &commonv1.AgentImageConfig{
					Tag: defaulting.ClusterAgentLatestVersion,
				},
				Config: &ClusterAgentConfig{
					ExternalMetrics: &ExternalMetricsConfig{
						Port: apiutils.NewInt32Pointer(8443),
					},
				},
				NetworkPolicy: &NetworkPolicySpec{Flavor: NetworkPolicyFlavorKubernetes},
			},
			internalDefaulted: DatadogAgentSpecClusterAgentSpec{
				Enabled: apiutils.NewBoolPointer(true),
				Image: &commonv1.AgentImageConfig{
					Name:        "foo",
					Tag:         defaulting.ClusterAgentLatestVersion,
					PullPolicy:  (*corev1.PullPolicy)(apiutils.NewStringPointer("Always")),
					PullSecrets: &[]corev1.LocalObjectReference{},
				},
				Config: &ClusterAgentConfig{
					ExternalMetrics: &ExternalMetricsConfig{
						Enabled:           apiutils.NewBoolPointer(true),
						WpaController:     true,
						UseDatadogMetrics: true,
						Port:              apiutils.NewInt32Pointer(8443),
					},
					AdmissionController: &AdmissionControllerConfig{
						Enabled:          apiutils.NewBoolPointer(true),
						MutateUnlabelled: apiutils.NewBoolPointer(false),
						ServiceName:      apiutils.NewStringPointer("foo"),
					},
					Resources:            &corev1.ResourceRequirements{Limits: corev1.ResourceList{}, Requests: corev1.ResourceList{}},
					ClusterChecksEnabled: apiutils.NewBoolPointer(true),
					CollectEvents:        apiutils.NewBoolPointer(false),
					LogLevel:             apiutils.NewStringPointer("DEBUG"),
					HealthPort:           apiutils.NewInt32Pointer(5555),
				},
				Rbac:     &RbacConfig{Create: apiutils.NewBoolPointer(false)},
				Replicas: apiutils.NewInt32Pointer(2),
				NetworkPolicy: &NetworkPolicySpec{
					Create: apiutils.NewBoolPointer(true),
					Flavor: NetworkPolicyFlavorKubernetes,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultDatadogAgentSpecClusterAgent(&tt.dca)
			assert.True(t, apiutils.IsEqualStruct(got, tt.overrideExpected), "TestDefaultDatadogAgentSpecClusterAgent override \ndiff = %s", cmp.Diff(got, tt.overrideExpected))
			assert.True(t, apiutils.IsEqualStruct(tt.dca, tt.internalDefaulted), "TestDefaultDatadogAgentSpecClusterAgent internal \ndiff = %s", cmp.Diff(tt.dca, tt.internalDefaulted))
		})
	}
}

func TestDefaultDatadogAgentSpecAgent(t *testing.T) {
	testCanary := &edsdatadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanary{}
	tests := []struct {
		name              string
		agent             DatadogAgentSpecAgentSpec
		overrideExpected  *DatadogAgentSpecAgentSpec
		internalDefaulted DatadogAgentSpecAgentSpec
	}{
		{
			name: "agent disabled field",
			agent: DatadogAgentSpecAgentSpec{
				Enabled: apiutils.NewBoolPointer(false),
			},
			overrideExpected: &DatadogAgentSpecAgentSpec{},
			internalDefaulted: DatadogAgentSpecAgentSpec{
				Enabled: apiutils.NewBoolPointer(false),
			},
		},
		{
			name: "agent spec sparse config",
			agent: DatadogAgentSpecAgentSpec{
				Config: &NodeAgentConfig{},
			},
			overrideExpected: &DatadogAgentSpecAgentSpec{
				Enabled:              apiutils.NewBoolPointer(true),
				UseExtendedDaemonset: apiutils.NewBoolPointer(false),
				Image: &commonv1.AgentImageConfig{
					Name:       defaultAgentImageName,
					Tag:        defaulting.AgentLatestVersion,
					PullPolicy: &defaultImagePullPolicy,
				},
				Config: &NodeAgentConfig{
					LogLevel:       apiutils.NewStringPointer(defaultLogLevel),
					CollectEvents:  apiutils.NewBoolPointer(false),
					LeaderElection: apiutils.NewBoolPointer(false),
					LivenessProbe:  GetDefaultLivenessProbe(),
					ReadinessProbe: GetDefaultReadinessProbe(),
					HealthPort:     apiutils.NewInt32Pointer(5555),
					// CriSocket unset as we use latest
					Dogstatsd: &DogstatsdConfig{
						DogstatsdOriginDetection: apiutils.NewBoolPointer(false),
						UnixDomainSocket: &DSDUnixDomainSocketSpec{
							Enabled:      apiutils.NewBoolPointer(false),
							HostFilepath: apiutils.NewStringPointer(path.Join(defaultHostDogstatsdSocketPath, defaultHostDogstatsdSocketName)),
						},
					},
				},
				DeploymentStrategy: &DaemonSetDeploymentStrategy{
					UpdateStrategyType: (*appsv1.DaemonSetUpdateStrategyType)(apiutils.NewStringPointer("RollingUpdate")),
					RollingUpdate: DaemonSetRollingUpdateSpec{
						MaxUnavailable:            &intstr.IntOrString{Type: intstr.String, StrVal: apicommon.DefaultRollingUpdateMaxUnavailable},
						MaxPodSchedulerFailure:    &intstr.IntOrString{Type: intstr.String, StrVal: apicommon.DefaultRollingUpdateMaxPodSchedulerFailure},
						MaxParallelPodCreation:    apiutils.NewInt32Pointer(apicommon.DefaultRollingUpdateMaxParallelPodCreation),
						SlowStartIntervalDuration: &metav1.Duration{Duration: apicommon.DefaultRollingUpdateSlowStartIntervalDuration},
						SlowStartAdditiveIncrease: &intstr.IntOrString{Type: intstr.String, StrVal: apicommon.DefaultRollingUpdateSlowStartAdditiveIncrease},
					},
					Canary:             edsdatadoghqv1alpha1.DefaultExtendedDaemonSetSpecStrategyCanary(testCanary, edsdatadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanaryValidationModeAuto),
					ReconcileFrequency: &metav1.Duration{Duration: apicommon.DefaultReconcileFrequency},
				},
				Rbac:        &RbacConfig{Create: apiutils.NewBoolPointer(true)},
				Apm:         &APMSpec{Enabled: apiutils.NewBoolPointer(false)},
				Process:     &ProcessSpec{Enabled: apiutils.NewBoolPointer(false), ProcessCollectionEnabled: apiutils.NewBoolPointer(false)},
				SystemProbe: &SystemProbeSpec{Enabled: apiutils.NewBoolPointer(false)},
				Security: &SecuritySpec{
					Compliance: ComplianceSpec{Enabled: apiutils.NewBoolPointer(false)},
					Runtime:    RuntimeSecuritySpec{Enabled: apiutils.NewBoolPointer(false), SyscallMonitor: &SyscallMonitorSpec{Enabled: apiutils.NewBoolPointer(false)}},
				},
				NetworkPolicy: &NetworkPolicySpec{Create: apiutils.NewBoolPointer(false)},
			},
			internalDefaulted: DatadogAgentSpecAgentSpec{
				Enabled:              apiutils.NewBoolPointer(true),
				UseExtendedDaemonset: apiutils.NewBoolPointer(false),
				Image: &commonv1.AgentImageConfig{
					Name:        defaultAgentImageName,
					Tag:         defaulting.AgentLatestVersion,
					PullPolicy:  &defaultImagePullPolicy,
					PullSecrets: &[]corev1.LocalObjectReference{},
				},
				Config: &NodeAgentConfig{
					LogLevel:             apiutils.NewStringPointer(defaultLogLevel),
					PodLabelsAsTags:      map[string]string{},
					PodAnnotationsAsTags: map[string]string{},
					Tags:                 []string{},
					CollectEvents:        apiutils.NewBoolPointer(false),
					LeaderElection:       apiutils.NewBoolPointer(false),
					LivenessProbe:        GetDefaultLivenessProbe(),
					Resources:            &corev1.ResourceRequirements{Limits: corev1.ResourceList{}, Requests: corev1.ResourceList{}},
					ReadinessProbe:       GetDefaultReadinessProbe(),
					HealthPort:           apiutils.NewInt32Pointer(5555),
					Dogstatsd: &DogstatsdConfig{
						DogstatsdOriginDetection: apiutils.NewBoolPointer(false),
						UnixDomainSocket: &DSDUnixDomainSocketSpec{
							Enabled:      apiutils.NewBoolPointer(false),
							HostFilepath: apiutils.NewStringPointer(path.Join(defaultHostDogstatsdSocketPath, defaultHostDogstatsdSocketName)),
						},
					},
				},
				DeploymentStrategy: &DaemonSetDeploymentStrategy{
					UpdateStrategyType: (*appsv1.DaemonSetUpdateStrategyType)(apiutils.NewStringPointer("RollingUpdate")),
					RollingUpdate: DaemonSetRollingUpdateSpec{
						MaxUnavailable:            &intstr.IntOrString{Type: intstr.String, StrVal: apicommon.DefaultRollingUpdateMaxUnavailable},
						MaxPodSchedulerFailure:    &intstr.IntOrString{Type: intstr.String, StrVal: apicommon.DefaultRollingUpdateMaxPodSchedulerFailure},
						MaxParallelPodCreation:    apiutils.NewInt32Pointer(apicommon.DefaultRollingUpdateMaxParallelPodCreation),
						SlowStartIntervalDuration: &metav1.Duration{Duration: apicommon.DefaultRollingUpdateSlowStartIntervalDuration},
						SlowStartAdditiveIncrease: &intstr.IntOrString{Type: intstr.String, StrVal: apicommon.DefaultRollingUpdateSlowStartAdditiveIncrease},
					},
					Canary:             edsdatadoghqv1alpha1.DefaultExtendedDaemonSetSpecStrategyCanary(testCanary, edsdatadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanaryValidationModeAuto),
					ReconcileFrequency: &metav1.Duration{Duration: apicommon.DefaultReconcileFrequency},
				},
				Rbac:        &RbacConfig{Create: apiutils.NewBoolPointer(true)},
				Apm:         &APMSpec{Enabled: apiutils.NewBoolPointer(false)},
				Process:     &ProcessSpec{Enabled: apiutils.NewBoolPointer(false), ProcessCollectionEnabled: apiutils.NewBoolPointer(false)},
				SystemProbe: &SystemProbeSpec{Enabled: apiutils.NewBoolPointer(false)},
				Security: &SecuritySpec{
					Compliance: ComplianceSpec{Enabled: apiutils.NewBoolPointer(false)},
					Runtime:    RuntimeSecuritySpec{Enabled: apiutils.NewBoolPointer(false), SyscallMonitor: &SyscallMonitorSpec{Enabled: apiutils.NewBoolPointer(false)}},
				},
				NetworkPolicy: &NetworkPolicySpec{Create: apiutils.NewBoolPointer(false)},
			},
		},
		{
			name: "some config",
			agent: DatadogAgentSpecAgentSpec{
				Config: &NodeAgentConfig{
					DDUrl:          apiutils.NewStringPointer("www.datadog.com"),
					LeaderElection: apiutils.NewBoolPointer(true),
					Dogstatsd: &DogstatsdConfig{
						DogstatsdOriginDetection: apiutils.NewBoolPointer(false),
						UnixDomainSocket:         &DSDUnixDomainSocketSpec{Enabled: apiutils.NewBoolPointer(true)},
					},
				},
				Image: &commonv1.AgentImageConfig{
					Name: "gcr.io/datadog/agent:6.26.0",
				},
				DeploymentStrategy: &DaemonSetDeploymentStrategy{
					Canary: edsdatadoghqv1alpha1.DefaultExtendedDaemonSetSpecStrategyCanary(testCanary, edsdatadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanaryValidationModeAuto),
				},
				Apm: &APMSpec{
					HostPort: apiutils.NewInt32Pointer(1664),
				},
				Process: &ProcessSpec{
					Enabled: apiutils.NewBoolPointer(true),
				},
				SystemProbe: &SystemProbeSpec{
					Enabled:         apiutils.NewBoolPointer(true),
					BPFDebugEnabled: apiutils.NewBoolPointer(true),
				},
			},
			overrideExpected: &DatadogAgentSpecAgentSpec{
				Enabled:              apiutils.NewBoolPointer(true),
				UseExtendedDaemonset: apiutils.NewBoolPointer(false),
				Image: &commonv1.AgentImageConfig{
					PullPolicy: &defaultImagePullPolicy,
				},
				Config: &NodeAgentConfig{
					LogLevel:       apiutils.NewStringPointer(defaultLogLevel),
					CollectEvents:  apiutils.NewBoolPointer(false),
					LivenessProbe:  GetDefaultLivenessProbe(),
					ReadinessProbe: GetDefaultReadinessProbe(),
					HealthPort:     apiutils.NewInt32Pointer(5555),
					// CRI Socket specified as we use an older image
					CriSocket: &CRISocketConfig{
						DockerSocketPath: apiutils.NewStringPointer(defaultDockerSocketPath),
					},
					Dogstatsd: &DogstatsdConfig{
						UnixDomainSocket: &DSDUnixDomainSocketSpec{HostFilepath: apiutils.NewStringPointer("/var/run/datadog/statsd.sock")},
					},
				},
				DeploymentStrategy: &DaemonSetDeploymentStrategy{
					UpdateStrategyType: (*appsv1.DaemonSetUpdateStrategyType)(apiutils.NewStringPointer("RollingUpdate")),
					RollingUpdate: DaemonSetRollingUpdateSpec{
						MaxUnavailable:            &intstr.IntOrString{Type: intstr.String, StrVal: apicommon.DefaultRollingUpdateMaxUnavailable},
						MaxPodSchedulerFailure:    &intstr.IntOrString{Type: intstr.String, StrVal: apicommon.DefaultRollingUpdateMaxPodSchedulerFailure},
						MaxParallelPodCreation:    apiutils.NewInt32Pointer(apicommon.DefaultRollingUpdateMaxParallelPodCreation),
						SlowStartIntervalDuration: &metav1.Duration{Duration: apicommon.DefaultRollingUpdateSlowStartIntervalDuration},
						SlowStartAdditiveIncrease: &intstr.IntOrString{Type: intstr.String, StrVal: apicommon.DefaultRollingUpdateSlowStartAdditiveIncrease},
					},
					ReconcileFrequency: &metav1.Duration{Duration: apicommon.DefaultReconcileFrequency},
				},
				Rbac:    &RbacConfig{Create: apiutils.NewBoolPointer(true)},
				Apm:     &APMSpec{Enabled: apiutils.NewBoolPointer(false)},
				Process: &ProcessSpec{ProcessCollectionEnabled: apiutils.NewBoolPointer(false)},
				SystemProbe: &SystemProbeSpec{
					SecCompRootPath:      "/var/lib/kubelet/seccomp",
					SecCompProfileName:   "localhost/system-probe",
					AppArmorProfileName:  "unconfined",
					ConntrackEnabled:     apiutils.NewBoolPointer(false),
					EnableTCPQueueLength: apiutils.NewBoolPointer(false),
					EnableOOMKill:        apiutils.NewBoolPointer(false),
					CollectDNSStats:      apiutils.NewBoolPointer(false),
				},
				Security: &SecuritySpec{
					Compliance: ComplianceSpec{Enabled: apiutils.NewBoolPointer(false)},
					Runtime:    RuntimeSecuritySpec{Enabled: apiutils.NewBoolPointer(false), SyscallMonitor: &SyscallMonitorSpec{Enabled: apiutils.NewBoolPointer(false)}},
				},
				NetworkPolicy: &NetworkPolicySpec{Create: apiutils.NewBoolPointer(false)},
			},
			internalDefaulted: DatadogAgentSpecAgentSpec{
				Enabled:              apiutils.NewBoolPointer(true),
				UseExtendedDaemonset: apiutils.NewBoolPointer(false),
				Image: &commonv1.AgentImageConfig{
					Name:        "gcr.io/datadog/agent:6.26.0",
					PullPolicy:  &defaultImagePullPolicy,
					PullSecrets: &[]corev1.LocalObjectReference{},
				},
				Config: &NodeAgentConfig{
					DDUrl:                apiutils.NewStringPointer("www.datadog.com"),
					LeaderElection:       apiutils.NewBoolPointer(true),
					LogLevel:             apiutils.NewStringPointer(defaultLogLevel),
					PodLabelsAsTags:      map[string]string{},
					PodAnnotationsAsTags: map[string]string{},
					Tags:                 []string{},
					Resources:            &corev1.ResourceRequirements{Limits: corev1.ResourceList{}, Requests: corev1.ResourceList{}},
					CollectEvents:        apiutils.NewBoolPointer(false),
					LivenessProbe:        GetDefaultLivenessProbe(),
					ReadinessProbe:       GetDefaultReadinessProbe(),
					HealthPort:           apiutils.NewInt32Pointer(5555),
					CriSocket: &CRISocketConfig{
						DockerSocketPath: apiutils.NewStringPointer(defaultDockerSocketPath),
					},
					Dogstatsd: &DogstatsdConfig{
						DogstatsdOriginDetection: apiutils.NewBoolPointer(false),
						UnixDomainSocket: &DSDUnixDomainSocketSpec{
							Enabled:      apiutils.NewBoolPointer(true),
							HostFilepath: apiutils.NewStringPointer("/var/run/datadog/statsd.sock"),
						},
					},
				},
				Rbac: &RbacConfig{Create: apiutils.NewBoolPointer(true)},
				DeploymentStrategy: &DaemonSetDeploymentStrategy{
					UpdateStrategyType: (*appsv1.DaemonSetUpdateStrategyType)(apiutils.NewStringPointer("RollingUpdate")),
					RollingUpdate: DaemonSetRollingUpdateSpec{
						MaxUnavailable:            &intstr.IntOrString{Type: intstr.String, StrVal: apicommon.DefaultRollingUpdateMaxUnavailable},
						MaxPodSchedulerFailure:    &intstr.IntOrString{Type: intstr.String, StrVal: apicommon.DefaultRollingUpdateMaxPodSchedulerFailure},
						MaxParallelPodCreation:    apiutils.NewInt32Pointer(apicommon.DefaultRollingUpdateMaxParallelPodCreation),
						SlowStartIntervalDuration: &metav1.Duration{Duration: apicommon.DefaultRollingUpdateSlowStartIntervalDuration},
						SlowStartAdditiveIncrease: &intstr.IntOrString{Type: intstr.String, StrVal: apicommon.DefaultRollingUpdateSlowStartAdditiveIncrease},
					},
					Canary:             edsdatadoghqv1alpha1.DefaultExtendedDaemonSetSpecStrategyCanary(testCanary, edsdatadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanaryValidationModeAuto),
					ReconcileFrequency: &metav1.Duration{Duration: apicommon.DefaultReconcileFrequency},
				},
				Apm: &APMSpec{
					Enabled:  apiutils.NewBoolPointer(false),
					HostPort: apiutils.NewInt32Pointer(1664),
				},
				Process: &ProcessSpec{
					Enabled:                  apiutils.NewBoolPointer(true),
					ProcessCollectionEnabled: apiutils.NewBoolPointer(false),
				},
				SystemProbe: &SystemProbeSpec{
					Enabled:              apiutils.NewBoolPointer(true),
					BPFDebugEnabled:      apiutils.NewBoolPointer(true),
					SecCompRootPath:      "/var/lib/kubelet/seccomp",
					SecCompProfileName:   "localhost/system-probe",
					AppArmorProfileName:  "unconfined",
					ConntrackEnabled:     apiutils.NewBoolPointer(false),
					EnableTCPQueueLength: apiutils.NewBoolPointer(false),
					EnableOOMKill:        apiutils.NewBoolPointer(false),
					CollectDNSStats:      apiutils.NewBoolPointer(false),
				},
				Security: &SecuritySpec{
					Compliance: ComplianceSpec{Enabled: apiutils.NewBoolPointer(false)},
					Runtime:    RuntimeSecuritySpec{Enabled: apiutils.NewBoolPointer(false), SyscallMonitor: &SyscallMonitorSpec{Enabled: apiutils.NewBoolPointer(false)}},
				},
				NetworkPolicy: &NetworkPolicySpec{Create: apiutils.NewBoolPointer(false)},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultDatadogAgentSpecAgent(&tt.agent)
			assert.True(t, apiutils.IsEqualStruct(got, tt.overrideExpected), "TestDefaultDatadogAgentSpecAgent override \ndiff = %s", cmp.Diff(got, tt.overrideExpected))
			assert.True(t, apiutils.IsEqualStruct(tt.agent, tt.internalDefaulted), "TestDefaultDatadogAgentSpecAgent internal \ndiff = %s", cmp.Diff(tt.agent, tt.internalDefaulted))
		})
	}
}

func TestDefaultDatadogAgentSpecClusterChecksRunner(t *testing.T) {
	tests := []struct {
		name              string
		clc               DatadogAgentSpecClusterChecksRunnerSpec
		overrideExpected  *DatadogAgentSpecClusterChecksRunnerSpec
		internalDefaulted DatadogAgentSpecClusterChecksRunnerSpec
	}{
		{
			name: "empty conf",
			clc:  DatadogAgentSpecClusterChecksRunnerSpec{},
			overrideExpected: &DatadogAgentSpecClusterChecksRunnerSpec{
				Enabled: apiutils.NewBoolPointer(false),
			},
			internalDefaulted: DatadogAgentSpecClusterChecksRunnerSpec{
				Enabled: apiutils.NewBoolPointer(false),
			},
		},
		{
			name: "sparse conf",
			clc: DatadogAgentSpecClusterChecksRunnerSpec{
				Enabled: apiutils.NewBoolPointer(true),
				Config:  &ClusterChecksRunnerConfig{},
				Image: &commonv1.AgentImageConfig{
					Name: "gcr.io/datadog/agent:latest",
					Tag:  defaulting.AgentLatestVersion,
				},
			},
			overrideExpected: &DatadogAgentSpecClusterChecksRunnerSpec{
				Image: &commonv1.AgentImageConfig{
					PullPolicy: &defaultImagePullPolicy,
				},
				Config: &ClusterChecksRunnerConfig{
					LogLevel:       apiutils.NewStringPointer(defaultLogLevel),
					LivenessProbe:  GetDefaultLivenessProbe(),
					ReadinessProbe: GetDefaultReadinessProbe(),
					HealthPort:     apiutils.NewInt32Pointer(5555),
				},
				Rbac:          &RbacConfig{Create: apiutils.NewBoolPointer(true)},
				Replicas:      nil,
				NetworkPolicy: &NetworkPolicySpec{Create: apiutils.NewBoolPointer(false)},
			},
			internalDefaulted: DatadogAgentSpecClusterChecksRunnerSpec{
				Enabled: apiutils.NewBoolPointer(true),
				Image: &commonv1.AgentImageConfig{
					Name:        "gcr.io/datadog/agent:latest",
					Tag:         defaulting.AgentLatestVersion,
					PullPolicy:  &defaultImagePullPolicy,
					PullSecrets: &[]corev1.LocalObjectReference{},
				},
				Config: &ClusterChecksRunnerConfig{
					LogLevel:       apiutils.NewStringPointer(defaultLogLevel),
					LivenessProbe:  GetDefaultLivenessProbe(),
					ReadinessProbe: GetDefaultReadinessProbe(),
					HealthPort:     apiutils.NewInt32Pointer(5555),
					Resources:      &corev1.ResourceRequirements{Limits: corev1.ResourceList{}, Requests: corev1.ResourceList{}},
				},
				Rbac:          &RbacConfig{Create: apiutils.NewBoolPointer(true)},
				Replicas:      nil,
				NetworkPolicy: &NetworkPolicySpec{Create: apiutils.NewBoolPointer(false)},
			},
		},
		{
			name: "some conf",
			clc: DatadogAgentSpecClusterChecksRunnerSpec{
				Enabled: apiutils.NewBoolPointer(true),
				Config: &ClusterChecksRunnerConfig{
					LogLevel:   apiutils.NewStringPointer("DEBUG"),
					HealthPort: apiutils.NewInt32Pointer(1664),
				},
				Image: &commonv1.AgentImageConfig{
					Name: "agent",
					Tag:  defaulting.AgentLatestVersion,
				},
			},
			overrideExpected: &DatadogAgentSpecClusterChecksRunnerSpec{
				Image: &commonv1.AgentImageConfig{
					PullPolicy: &defaultImagePullPolicy,
				},
				Config: &ClusterChecksRunnerConfig{
					LivenessProbe:  GetDefaultLivenessProbe(),
					ReadinessProbe: GetDefaultReadinessProbe(),
				},
				Rbac:          &RbacConfig{Create: apiutils.NewBoolPointer(true)},
				Replicas:      nil,
				NetworkPolicy: &NetworkPolicySpec{Create: apiutils.NewBoolPointer(false)},
			},
			internalDefaulted: DatadogAgentSpecClusterChecksRunnerSpec{
				Enabled: apiutils.NewBoolPointer(true),
				Image: &commonv1.AgentImageConfig{
					Name:        "agent",
					Tag:         defaulting.AgentLatestVersion,
					PullPolicy:  &defaultImagePullPolicy,
					PullSecrets: &[]corev1.LocalObjectReference{},
				},
				Config: &ClusterChecksRunnerConfig{
					LogLevel:       apiutils.NewStringPointer("DEBUG"),
					LivenessProbe:  GetDefaultLivenessProbe(),
					ReadinessProbe: GetDefaultReadinessProbe(),
					HealthPort:     apiutils.NewInt32Pointer(1664),
					Resources:      &corev1.ResourceRequirements{Limits: corev1.ResourceList{}, Requests: corev1.ResourceList{}},
				},
				Rbac:          &RbacConfig{Create: apiutils.NewBoolPointer(true)},
				Replicas:      nil,
				NetworkPolicy: &NetworkPolicySpec{Create: apiutils.NewBoolPointer(false)},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultDatadogAgentSpecClusterChecksRunner(&tt.clc)
			assert.True(t, apiutils.IsEqualStruct(got, tt.overrideExpected), "TestDefaultDatadogAgentSpecClusterChecksRunner override \ndiff = %s", cmp.Diff(got, tt.overrideExpected))
			assert.True(t, apiutils.IsEqualStruct(tt.clc, tt.internalDefaulted), "TestDefaultDatadogAgentSpecClusterChecksRunner internal \ndiff = %s", cmp.Diff(tt.clc, tt.internalDefaulted))
		})
	}
}

func TestDefaultDatadogFeatureOrchestratorExplorer(t *testing.T) {
	tests := []struct {
		name         string
		orc          *DatadogFeatures
		clustercheck bool
		orcOverride  *OrchestratorExplorerConfig
		internal     *OrchestratorExplorerConfig
	}{
		{
			name: "empty",
			orc:  &DatadogFeatures{},
			orcOverride: &OrchestratorExplorerConfig{
				Enabled:      apiutils.NewBoolPointer(true),
				ClusterCheck: apiutils.NewBoolPointer(false),
				Scrubbing: &Scrubbing{
					Containers: apiutils.NewBoolPointer(true),
				},
			},
			internal: &OrchestratorExplorerConfig{
				Enabled:      apiutils.NewBoolPointer(true),
				ClusterCheck: apiutils.NewBoolPointer(false),
				Scrubbing: &Scrubbing{
					Containers: apiutils.NewBoolPointer(true),
				},
			},
		},
		{
			name: "enabled orchestrator explorer, no scrubbing specified",
			orc: &DatadogFeatures{
				OrchestratorExplorer: &OrchestratorExplorerConfig{
					Enabled: apiutils.NewBoolPointer(true),
				},
			},
			orcOverride: &OrchestratorExplorerConfig{
				ClusterCheck: apiutils.NewBoolPointer(false),
				Scrubbing: &Scrubbing{
					Containers: apiutils.NewBoolPointer(true),
				},
			},
			internal: &OrchestratorExplorerConfig{
				Enabled:      apiutils.NewBoolPointer(true),
				ClusterCheck: apiutils.NewBoolPointer(false),
				Scrubbing: &Scrubbing{
					Containers: apiutils.NewBoolPointer(true),
				},
			},
		},
		{
			name: "disabled orchestrator",
			orc: &DatadogFeatures{
				OrchestratorExplorer: &OrchestratorExplorerConfig{
					Enabled: apiutils.NewBoolPointer(false),
				},
			},
			orcOverride: &OrchestratorExplorerConfig{
				Enabled: apiutils.NewBoolPointer(false),
			},
			internal: &OrchestratorExplorerConfig{
				Enabled: apiutils.NewBoolPointer(false),
			},
		},
		{
			name: "enabled orchestrator, filled scrubbing",
			orc: &DatadogFeatures{
				OrchestratorExplorer: &OrchestratorExplorerConfig{
					Enabled: apiutils.NewBoolPointer(true),
					Scrubbing: &Scrubbing{
						Containers: apiutils.NewBoolPointer(true),
					},
				},
			},
			orcOverride: &OrchestratorExplorerConfig{
				Enabled:      nil,
				ClusterCheck: apiutils.NewBoolPointer(false),
			},
			internal: &OrchestratorExplorerConfig{
				Enabled:      apiutils.NewBoolPointer(true),
				ClusterCheck: apiutils.NewBoolPointer(false),
				Scrubbing: &Scrubbing{
					Containers: apiutils.NewBoolPointer(true),
				},
			},
		},
		{
			name: "enabled orchestrator, enabled clustercheck",
			orc: &DatadogFeatures{
				OrchestratorExplorer: &OrchestratorExplorerConfig{
					Enabled:      apiutils.NewBoolPointer(true),
					ClusterCheck: apiutils.NewBoolPointer(true),
				},
			},
			clustercheck: true,
			orcOverride: &OrchestratorExplorerConfig{
				Enabled:      nil,
				ClusterCheck: nil,
				Scrubbing: &Scrubbing{
					Containers: apiutils.NewBoolPointer(true),
				},
			},
			internal: &OrchestratorExplorerConfig{
				Enabled:      apiutils.NewBoolPointer(true),
				ClusterCheck: apiutils.NewBoolPointer(true),
				Scrubbing: &Scrubbing{
					Containers: apiutils.NewBoolPointer(true),
				},
			},
		},
		{
			name: "enabled orchestrator, checksrunner enabled, clustercheck disabled",
			orc: &DatadogFeatures{
				OrchestratorExplorer: &OrchestratorExplorerConfig{
					Enabled:      apiutils.NewBoolPointer(true),
					ClusterCheck: apiutils.NewBoolPointer(false),
				},
			},
			clustercheck: true,
			orcOverride: &OrchestratorExplorerConfig{
				Enabled: nil,
				Scrubbing: &Scrubbing{
					Containers: apiutils.NewBoolPointer(true),
				},
			},
			internal: &OrchestratorExplorerConfig{
				Enabled:      apiutils.NewBoolPointer(true),
				ClusterCheck: apiutils.NewBoolPointer(false),
				Scrubbing: &Scrubbing{
					Containers: apiutils.NewBoolPointer(true),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultDatadogFeatureOrchestratorExplorer(tt.orc, tt.clustercheck)
			assert.True(t, apiutils.IsEqualStruct(got, tt.orcOverride), "TestDefaultDatadogFeatureOrchestratorExplorer override \ndiff = %s", cmp.Diff(got, tt.orcOverride))
			assert.True(t, apiutils.IsEqualStruct(tt.orc.OrchestratorExplorer, tt.internal), "TestDefaultDatadogFeatureOrchestratorExplorer internal \ndiff = %s", cmp.Diff(tt.orc.OrchestratorExplorer, tt.internal))
		})
	}
}

func TestDefaultDatadogAgentSpecAgentApm(t *testing.T) {
	tests := []struct {
		input *DatadogAgentSpecAgentSpec
		name  string
		want  *APMSpec
	}{
		{
			name: "APM not set",
			input: &DatadogAgentSpecAgentSpec{
				Enabled: apiutils.NewBoolPointer(true),
			},
			want: &APMSpec{
				Enabled: apiutils.NewBoolPointer(false),
			},
		},
		{
			name: "APM not enabled",
			input: &DatadogAgentSpecAgentSpec{
				Apm: &APMSpec{
					Enabled: apiutils.NewBoolPointer(false),
				},
			},
			want: &APMSpec{},
		},
		{
			name: "APM enabled",
			input: &DatadogAgentSpecAgentSpec{
				Apm: &APMSpec{
					Enabled: apiutils.NewBoolPointer(true),
				},
			},
			want: &APMSpec{
				HostPort:         apiutils.NewInt32Pointer(8126),
				UnixDomainSocket: &APMUnixDomainSocketSpec{Enabled: apiutils.NewBoolPointer(false)},
				LivenessProbe:    getDefaultAPMAgentLivenessProbe(),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultDatadogAgentSpecAgentApm(tt.input)
			assert.True(t, apiutils.IsEqualStruct(got, tt.want), "TestDefaultDatadogAgentSpecAgentApm defaulting \ndiff = %s", cmp.Diff(got, tt.want))
		})
	}
}

func Test_defaultCredentials(t *testing.T) {
	tests := []struct {
		name                      string
		tokenInSpec               string
		defaultedToken            string
		expectsDefaultedToken     bool
		expectsSameDefaultedToken bool
	}{
		{
			name:                  "token in spec",
			tokenInSpec:           "a_token",
			defaultedToken:        "",
			expectsDefaultedToken: false,
		},
		{
			name:                      "no token in spec and not defaulted",
			tokenInSpec:               "",
			defaultedToken:            "",
			expectsDefaultedToken:     true,
			expectsSameDefaultedToken: false,
		},
		{
			name:                      "no token in spec, but already defaulted",
			tokenInSpec:               "",
			defaultedToken:            "a_defaulted_token",
			expectsDefaultedToken:     true,
			expectsSameDefaultedToken: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda := DatadogAgent{
				Spec: DatadogAgentSpec{
					Credentials: &AgentCredentials{
						Token: tt.tokenInSpec,
					},
				},
				Status: DatadogAgentStatus{
					DefaultOverride: &DatadogAgentSpec{
						Credentials: &AgentCredentials{
							Token: tt.defaultedToken,
						},
					},
				},
			}

			ddaStatus := DatadogAgentStatus{}

			defaultCredentials(&dda, &ddaStatus)

			if tt.expectsDefaultedToken {
				assert.NotEmpty(t, ddaStatus.DefaultOverride.Credentials.Token)
			} else if ddaStatus.DefaultOverride != nil && ddaStatus.DefaultOverride.Credentials != nil {
				assert.Empty(t, ddaStatus.DefaultOverride.Credentials.Token)
			}

			if tt.expectsSameDefaultedToken {
				assert.Equal(t, tt.defaultedToken, ddaStatus.DefaultOverride.Credentials.Token)
			}
		})
	}
}

func TestDefaultedClusterAgentToken(t *testing.T) {
	tests := []struct {
		name          string
		ddaStatus     *DatadogAgentStatus
		expectedToken string
	}{
		{
			name: "status without default overrides",
			ddaStatus: &DatadogAgentStatus{
				DefaultOverride: nil,
			},
			expectedToken: "",
		},
		{
			name: "status with overrides but no overridden credentials",
			ddaStatus: &DatadogAgentStatus{
				DefaultOverride: &DatadogAgentSpec{
					Credentials: nil,
				},
			},
			expectedToken: "",
		},
		{
			name: "status with defaulted token",
			ddaStatus: &DatadogAgentStatus{
				DefaultOverride: &DatadogAgentSpec{
					Credentials: &AgentCredentials{
						Token: "some_token",
					},
				},
			},
			expectedToken: "some_token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectedToken, DefaultedClusterAgentToken(tt.ddaStatus))
		})
	}
}
