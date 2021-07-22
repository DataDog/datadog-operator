// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"path"
	"testing"

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
				DogstatsdOriginDetection: NewBoolPointer(false), // defaultDogstatsdOriginDetection
				UnixDomainSocket: &DSDUnixDomainSocketSpec{
					Enabled:      NewBoolPointer(false),
					HostFilepath: &defaultPath,
				},
			},
			internal: NodeAgentConfig{
				Dogstatsd: &DogstatsdConfig{
					DogstatsdOriginDetection: NewBoolPointer(false),
					UnixDomainSocket: &DSDUnixDomainSocketSpec{
						Enabled:      NewBoolPointer(false),
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
						Enabled:      NewBoolPointer(false),
						HostFilepath: &defaultPath,
					},
				},
			},
			override: &DogstatsdConfig{
				DogstatsdOriginDetection: NewBoolPointer(false),
			},
			internal: NodeAgentConfig{
				Dogstatsd: &DogstatsdConfig{
					DogstatsdOriginDetection: NewBoolPointer(false),
					UnixDomainSocket: &DSDUnixDomainSocketSpec{
						Enabled:      NewBoolPointer(false),
						HostFilepath: &defaultPath,
					},
				},
			},
		},
		{
			name: "dogtatsd missing defaulting: UseDogStatsDSocketVolume",
			dsd: NodeAgentConfig{
				Dogstatsd: &DogstatsdConfig{
					DogstatsdOriginDetection: NewBoolPointer(false),
				},
			},
			override: &DogstatsdConfig{
				UnixDomainSocket: &DSDUnixDomainSocketSpec{
					Enabled:      NewBoolPointer(false),
					HostFilepath: &defaultPath,
				},
			},
			internal: NodeAgentConfig{
				Dogstatsd: &DogstatsdConfig{
					DogstatsdOriginDetection: NewBoolPointer(false),
					UnixDomainSocket: &DSDUnixDomainSocketSpec{
						Enabled:      NewBoolPointer(false),
						HostFilepath: &defaultPath,
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultConfigDogstatsd(&tt.dsd)
			assert.True(t, IsEqualStruct(got, tt.override), "TestDefaultFeatures override \ndiff = %s", cmp.Diff(got, tt.override))
			assert.True(t, IsEqualStruct(tt.dsd, tt.internal), "TestDefaultFeatures internal \ndiff = %s", cmp.Diff(tt.dsd, tt.internal))
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
					Enabled: NewBoolPointer(false),
				},
				KubeStateMetricsCore: &KubeStateMetricsCore{Enabled: NewBoolPointer(false), ClusterCheck: NewBoolPointer(false)},
				PrometheusScrape: &PrometheusScrapeConfig{
					Enabled: NewBoolPointer(false),
				},
				LogCollection: &LogCollectionConfig{
					Enabled: NewBoolPointer(false),
				},
				NetworkMonitoring: &NetworkMonitoringConfig{Enabled: NewBoolPointer(false)},
			},
			internalDefaulted: DatadogFeatures{
				OrchestratorExplorer: &OrchestratorExplorerConfig{
					Enabled: NewBoolPointer(false),
				},
				KubeStateMetricsCore: &KubeStateMetricsCore{Enabled: NewBoolPointer(false), ClusterCheck: NewBoolPointer(false)},
				PrometheusScrape: &PrometheusScrapeConfig{
					Enabled: NewBoolPointer(false),
				},
				LogCollection: &LogCollectionConfig{
					Enabled: NewBoolPointer(false),
				},
				NetworkMonitoring: &NetworkMonitoringConfig{Enabled: NewBoolPointer(false)},
			},
		},
		{
			name: "sparse config",
			ft: DatadogFeatures{
				OrchestratorExplorer: &OrchestratorExplorerConfig{
					Enabled: NewBoolPointer(false),
				},
				LogCollection: &LogCollectionConfig{
					Enabled: NewBoolPointer(true),
				},
			},
			overrideExpected: &DatadogFeatures{
				OrchestratorExplorer: &OrchestratorExplorerConfig{
					Enabled: NewBoolPointer(false),
				},
				KubeStateMetricsCore: &KubeStateMetricsCore{Enabled: NewBoolPointer(false), ClusterCheck: NewBoolPointer(false)},
				LogCollection: &LogCollectionConfig{
					Enabled:                       NewBoolPointer(true),
					LogsConfigContainerCollectAll: NewBoolPointer(false),
					ContainerCollectUsingFiles:    NewBoolPointer(true),
					ContainerLogsPath:             NewStringPointer("/var/lib/docker/containers"),
					PodLogsPath:                   NewStringPointer("/var/log/pods"),
					ContainerSymlinksPath:         NewStringPointer("/var/log/containers"),
					TempStoragePath:               NewStringPointer("/var/lib/datadog-agent/logs"),
					OpenFilesLimit:                NewInt32Pointer(100),
				},
				PrometheusScrape: &PrometheusScrapeConfig{
					Enabled: NewBoolPointer(false),
				},
				NetworkMonitoring: &NetworkMonitoringConfig{Enabled: NewBoolPointer(false)},
			},
			internalDefaulted: DatadogFeatures{
				OrchestratorExplorer: &OrchestratorExplorerConfig{
					Enabled: NewBoolPointer(false),
				},
				KubeStateMetricsCore: &KubeStateMetricsCore{Enabled: NewBoolPointer(false), ClusterCheck: NewBoolPointer(false)},
				LogCollection: &LogCollectionConfig{
					Enabled:                       NewBoolPointer(true),
					LogsConfigContainerCollectAll: NewBoolPointer(false),
					ContainerCollectUsingFiles:    NewBoolPointer(true),
					ContainerLogsPath:             NewStringPointer("/var/lib/docker/containers"),
					PodLogsPath:                   NewStringPointer("/var/log/pods"),
					ContainerSymlinksPath:         NewStringPointer("/var/log/containers"),
					TempStoragePath:               NewStringPointer("/var/lib/datadog-agent/logs"),
					OpenFilesLimit:                NewInt32Pointer(100),
				},
				PrometheusScrape: &PrometheusScrapeConfig{
					Enabled: NewBoolPointer(false),
				},
				NetworkMonitoring: &NetworkMonitoringConfig{Enabled: NewBoolPointer(false)},
			},
		},
		{
			name: "some config",
			ft: DatadogFeatures{
				OrchestratorExplorer: &OrchestratorExplorerConfig{
					Scrubbing: &Scrubbing{Containers: NewBoolPointer(false)},
				},
				KubeStateMetricsCore: &KubeStateMetricsCore{Enabled: NewBoolPointer(true), ClusterCheck: NewBoolPointer(true)},
				LogCollection: &LogCollectionConfig{
					LogsConfigContainerCollectAll: NewBoolPointer(false),
					ContainerLogsPath:             NewStringPointer("/var/lib/docker/containers"),
					OpenFilesLimit:                NewInt32Pointer(200),
				},
				PrometheusScrape: &PrometheusScrapeConfig{
					ServiceEndpoints: NewBoolPointer(true),
				},
				NetworkMonitoring: &NetworkMonitoringConfig{Enabled: NewBoolPointer(true)},
			},
			overrideExpected: &DatadogFeatures{
				OrchestratorExplorer: &OrchestratorExplorerConfig{
					Enabled: NewBoolPointer(true), // defaultOrchestratorExplorerEnabled
				},
				KubeStateMetricsCore: &KubeStateMetricsCore{Enabled: NewBoolPointer(true), ClusterCheck: NewBoolPointer(true)},
				LogCollection: &LogCollectionConfig{
					Enabled: NewBoolPointer(false), // defaultLogEnabled
				},
				PrometheusScrape: &PrometheusScrapeConfig{
					Enabled: NewBoolPointer(false), // defaultPrometheusScrapeEnabled
				},
				NetworkMonitoring: &NetworkMonitoringConfig{Enabled: NewBoolPointer(true)},
			},
			internalDefaulted: DatadogFeatures{
				OrchestratorExplorer: &OrchestratorExplorerConfig{
					Enabled:   NewBoolPointer(true), // defaultOrchestratorExplorerEnabled
					Scrubbing: &Scrubbing{Containers: NewBoolPointer(false)},
				},
				KubeStateMetricsCore: &KubeStateMetricsCore{Enabled: NewBoolPointer(true), ClusterCheck: NewBoolPointer(true)},
				LogCollection: &LogCollectionConfig{
					Enabled:                       NewBoolPointer(false),
					LogsConfigContainerCollectAll: NewBoolPointer(false),
					ContainerLogsPath:             NewStringPointer("/var/lib/docker/containers"),
					OpenFilesLimit:                NewInt32Pointer(200),
				},
				PrometheusScrape: &PrometheusScrapeConfig{
					Enabled:          NewBoolPointer(false),
					ServiceEndpoints: NewBoolPointer(true),
				},
				NetworkMonitoring: &NetworkMonitoringConfig{Enabled: NewBoolPointer(true)},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda := &DatadogAgent{}
			dda.Spec.Features = tt.ft
			got := DefaultFeatures(dda)
			assert.True(t, IsEqualStruct(got, tt.overrideExpected), "TestDefaultFeatures override \ndiff = %s", cmp.Diff(got, tt.overrideExpected))
			assert.True(t, IsEqualStruct(dda.Spec.Features, tt.internalDefaulted), "TestDefaultFeatures internal \ndiff = %s", cmp.Diff(dda.Spec.Features, tt.internalDefaulted))
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
			name: "empty field",
			dca:  DatadogAgentSpecClusterAgentSpec{},
			overrideExpected: &DatadogAgentSpecClusterAgentSpec{
				Enabled: NewBoolPointer(false),
			},
			internalDefaulted: DatadogAgentSpecClusterAgentSpec{
				Enabled: NewBoolPointer(false),
			},
		},
		{
			name: "some config",
			dca: DatadogAgentSpecClusterAgentSpec{
				Config: &ClusterAgentConfig{
					AdmissionController: &AdmissionControllerConfig{
						Enabled: NewBoolPointer(true),
					},
				},
			},
			overrideExpected: &DatadogAgentSpecClusterAgentSpec{
				Enabled: NewBoolPointer(true),
				Image: &ImageConfig{
					Name:       defaultClusterAgentImageName,
					Tag:        defaultClusterAgentImageTag,
					PullPolicy: &defaultImagePullPolicy,
				},
				Config: &ClusterAgentConfig{
					ExternalMetrics: &ExternalMetricsConfig{
						Enabled: NewBoolPointer(false),
					},
					AdmissionController: &AdmissionControllerConfig{
						MutateUnlabelled: NewBoolPointer(false),
						ServiceName:      NewStringPointer("datadog-admission-controller"),
					},
					ClusterChecksEnabled: NewBoolPointer(false),
					LogLevel:             NewStringPointer(defaultLogLevel),
					CollectEvents:        NewBoolPointer(false),
					HealthPort:           NewInt32Pointer(5555),
				},
				Rbac:          &RbacConfig{Create: NewBoolPointer(true)},
				Replicas:      nil,
				NetworkPolicy: &NetworkPolicySpec{Create: NewBoolPointer(false)},
			},
			internalDefaulted: DatadogAgentSpecClusterAgentSpec{
				Enabled: NewBoolPointer(true),
				Image: &ImageConfig{
					Name:        defaultClusterAgentImageName,
					Tag:         defaultClusterAgentImageTag,
					PullPolicy:  &defaultImagePullPolicy,
					PullSecrets: &[]corev1.LocalObjectReference{},
				},
				Config: &ClusterAgentConfig{
					ExternalMetrics: &ExternalMetricsConfig{
						Enabled: NewBoolPointer(false),
					},
					AdmissionController: &AdmissionControllerConfig{
						Enabled:          NewBoolPointer(true),
						MutateUnlabelled: NewBoolPointer(false),
						ServiceName:      NewStringPointer("datadog-admission-controller"),
					},
					Resources:            &corev1.ResourceRequirements{Limits: corev1.ResourceList{}, Requests: corev1.ResourceList{}},
					LogLevel:             NewStringPointer(defaultLogLevel),
					ClusterChecksEnabled: NewBoolPointer(false),
					CollectEvents:        NewBoolPointer(false),
					HealthPort:           NewInt32Pointer(5555),
				},
				Rbac:          &RbacConfig{Create: NewBoolPointer(true)},
				Replicas:      nil,
				NetworkPolicy: &NetworkPolicySpec{Create: NewBoolPointer(false)},
			},
		},
		{
			name: "almost full config",
			dca: DatadogAgentSpecClusterAgentSpec{
				Enabled: NewBoolPointer(true),
				Image: &ImageConfig{
					Name:       "foo",
					PullPolicy: (*corev1.PullPolicy)(NewStringPointer("Always")),
				},
				Config: &ClusterAgentConfig{
					ExternalMetrics: &ExternalMetricsConfig{
						WpaController:     true,
						UseDatadogMetrics: true,
					},
					LogLevel: NewStringPointer("DEBUG"),
					AdmissionController: &AdmissionControllerConfig{
						Enabled:          NewBoolPointer(true),
						MutateUnlabelled: NewBoolPointer(false),
						ServiceName:      NewStringPointer("foo"),
					},
					ClusterChecksEnabled: NewBoolPointer(true),
					CollectEvents:        NewBoolPointer(false),
					HealthPort:           NewInt32Pointer(5555),
				},
				Rbac:          &RbacConfig{Create: NewBoolPointer(false)},
				Replicas:      NewInt32Pointer(2),
				NetworkPolicy: &NetworkPolicySpec{Create: NewBoolPointer(true)},
			},
			overrideExpected: &DatadogAgentSpecClusterAgentSpec{
				Enabled: NewBoolPointer(true),
				Image: &ImageConfig{
					Tag: defaultClusterAgentImageTag,
				},
				Config: &ClusterAgentConfig{
					ExternalMetrics: &ExternalMetricsConfig{
						Enabled: NewBoolPointer(true),
						Port:    NewInt32Pointer(8443),
					},
				},
			},
			internalDefaulted: DatadogAgentSpecClusterAgentSpec{
				Enabled: NewBoolPointer(true),
				Image: &ImageConfig{
					Name:        "foo",
					Tag:         defaultClusterAgentImageTag,
					PullPolicy:  (*corev1.PullPolicy)(NewStringPointer("Always")),
					PullSecrets: &[]corev1.LocalObjectReference{},
				},
				Config: &ClusterAgentConfig{
					ExternalMetrics: &ExternalMetricsConfig{
						Enabled:           NewBoolPointer(true),
						WpaController:     true,
						UseDatadogMetrics: true,
						Port:              NewInt32Pointer(8443),
					},
					AdmissionController: &AdmissionControllerConfig{
						Enabled:          NewBoolPointer(true),
						MutateUnlabelled: NewBoolPointer(false),
						ServiceName:      NewStringPointer("foo"),
					},
					Resources:            &corev1.ResourceRequirements{Limits: corev1.ResourceList{}, Requests: corev1.ResourceList{}},
					ClusterChecksEnabled: NewBoolPointer(true),
					CollectEvents:        NewBoolPointer(false),
					LogLevel:             NewStringPointer("DEBUG"),
					HealthPort:           NewInt32Pointer(5555),
				},
				Rbac:          &RbacConfig{Create: NewBoolPointer(false)},
				Replicas:      NewInt32Pointer(2),
				NetworkPolicy: &NetworkPolicySpec{Create: NewBoolPointer(true)},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultDatadogAgentSpecClusterAgent(&tt.dca)
			assert.True(t, IsEqualStruct(got, tt.overrideExpected), "TestDefaultDatadogAgentSpecClusterAgent override \ndiff = %s", cmp.Diff(got, tt.overrideExpected))
			assert.True(t, IsEqualStruct(tt.dca, tt.internalDefaulted), "TestDefaultDatadogAgentSpecClusterAgent internal \ndiff = %s", cmp.Diff(tt.dca, tt.internalDefaulted))
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
			name:  "empty field",
			agent: DatadogAgentSpecAgentSpec{},
			overrideExpected: &DatadogAgentSpecAgentSpec{
				Enabled: NewBoolPointer(false),
			},
			internalDefaulted: DatadogAgentSpecAgentSpec{
				Enabled: NewBoolPointer(false),
			},
		},
		{
			name: "sparse config",
			agent: DatadogAgentSpecAgentSpec{
				Config: &NodeAgentConfig{},
			},
			overrideExpected: &DatadogAgentSpecAgentSpec{
				Enabled:              NewBoolPointer(true),
				UseExtendedDaemonset: NewBoolPointer(false),
				Image: &ImageConfig{
					Name:       defaultAgentImageName,
					Tag:        defaultAgentImageTag,
					PullPolicy: &defaultImagePullPolicy,
				},
				Config: &NodeAgentConfig{
					LogLevel:       NewStringPointer(defaultLogLevel),
					CollectEvents:  NewBoolPointer(false),
					LeaderElection: NewBoolPointer(false),
					LivenessProbe:  GetDefaultLivenessProbe(),
					ReadinessProbe: GetDefaultReadinessProbe(),
					HealthPort:     NewInt32Pointer(5555),
					// CriSocket unset as we use latest
					Dogstatsd: &DogstatsdConfig{
						DogstatsdOriginDetection: NewBoolPointer(false),
						UnixDomainSocket: &DSDUnixDomainSocketSpec{
							Enabled:      NewBoolPointer(false),
							HostFilepath: NewStringPointer(path.Join(defaultHostDogstatsdSocketPath, defaultHostDogstatsdSocketName)),
						},
					},
				},
				DeploymentStrategy: &DaemonSetDeploymentStrategy{
					UpdateStrategyType: (*appsv1.DaemonSetUpdateStrategyType)(NewStringPointer("RollingUpdate")),
					RollingUpdate: DaemonSetRollingUpdateSpec{
						MaxUnavailable:            &intstr.IntOrString{Type: intstr.String, StrVal: defaultRollingUpdateMaxUnavailable},
						MaxPodSchedulerFailure:    &intstr.IntOrString{Type: intstr.String, StrVal: defaultRollingUpdateMaxPodSchedulerFailure},
						MaxParallelPodCreation:    NewInt32Pointer(defaultRollingUpdateMaxParallelPodCreation),
						SlowStartIntervalDuration: &metav1.Duration{Duration: defaultRollingUpdateSlowStartIntervalDuration},
						SlowStartAdditiveIncrease: &intstr.IntOrString{Type: intstr.String, StrVal: defaultRollingUpdateSlowStartAdditiveIncrease},
					},
					Canary:             edsdatadoghqv1alpha1.DefaultExtendedDaemonSetSpecStrategyCanary(testCanary),
					ReconcileFrequency: &metav1.Duration{Duration: defaultReconcileFrequency},
				},
				Rbac:        &RbacConfig{Create: NewBoolPointer(true)},
				Apm:         &APMSpec{Enabled: NewBoolPointer(false)},
				Process:     &ProcessSpec{Enabled: NewBoolPointer(false)},
				SystemProbe: &SystemProbeSpec{Enabled: NewBoolPointer(false)},
				Security: &SecuritySpec{
					Compliance: ComplianceSpec{Enabled: NewBoolPointer(false)},
					Runtime:    RuntimeSecuritySpec{Enabled: NewBoolPointer(false), SyscallMonitor: &SyscallMonitorSpec{Enabled: NewBoolPointer(false)}},
				},
				NetworkPolicy: &NetworkPolicySpec{Create: NewBoolPointer(false)},
			},
			internalDefaulted: DatadogAgentSpecAgentSpec{
				Enabled:              NewBoolPointer(true),
				UseExtendedDaemonset: NewBoolPointer(false),
				Image: &ImageConfig{
					Name:        defaultAgentImageName,
					Tag:         defaultAgentImageTag,
					PullPolicy:  &defaultImagePullPolicy,
					PullSecrets: &[]corev1.LocalObjectReference{},
				},
				Config: &NodeAgentConfig{
					LogLevel:             NewStringPointer(defaultLogLevel),
					PodLabelsAsTags:      map[string]string{},
					PodAnnotationsAsTags: map[string]string{},
					Tags:                 []string{},
					CollectEvents:        NewBoolPointer(false),
					LeaderElection:       NewBoolPointer(false),
					LivenessProbe:        GetDefaultLivenessProbe(),
					Resources:            &corev1.ResourceRequirements{Limits: corev1.ResourceList{}, Requests: corev1.ResourceList{}},
					ReadinessProbe:       GetDefaultReadinessProbe(),
					HealthPort:           NewInt32Pointer(5555),
					Dogstatsd: &DogstatsdConfig{
						DogstatsdOriginDetection: NewBoolPointer(false),
						UnixDomainSocket: &DSDUnixDomainSocketSpec{
							Enabled:      NewBoolPointer(false),
							HostFilepath: NewStringPointer(path.Join(defaultHostDogstatsdSocketPath, defaultHostDogstatsdSocketName)),
						},
					},
				},
				DeploymentStrategy: &DaemonSetDeploymentStrategy{
					UpdateStrategyType: (*appsv1.DaemonSetUpdateStrategyType)(NewStringPointer("RollingUpdate")),
					RollingUpdate: DaemonSetRollingUpdateSpec{
						MaxUnavailable:            &intstr.IntOrString{Type: intstr.String, StrVal: defaultRollingUpdateMaxUnavailable},
						MaxPodSchedulerFailure:    &intstr.IntOrString{Type: intstr.String, StrVal: defaultRollingUpdateMaxPodSchedulerFailure},
						MaxParallelPodCreation:    NewInt32Pointer(defaultRollingUpdateMaxParallelPodCreation),
						SlowStartIntervalDuration: &metav1.Duration{Duration: defaultRollingUpdateSlowStartIntervalDuration},
						SlowStartAdditiveIncrease: &intstr.IntOrString{Type: intstr.String, StrVal: defaultRollingUpdateSlowStartAdditiveIncrease},
					},
					Canary:             edsdatadoghqv1alpha1.DefaultExtendedDaemonSetSpecStrategyCanary(testCanary),
					ReconcileFrequency: &metav1.Duration{Duration: defaultReconcileFrequency},
				},
				Rbac:        &RbacConfig{Create: NewBoolPointer(true)},
				Apm:         &APMSpec{Enabled: NewBoolPointer(false)},
				Process:     &ProcessSpec{Enabled: NewBoolPointer(false)},
				SystemProbe: &SystemProbeSpec{Enabled: NewBoolPointer(false)},
				Security: &SecuritySpec{
					Compliance: ComplianceSpec{Enabled: NewBoolPointer(false)},
					Runtime:    RuntimeSecuritySpec{Enabled: NewBoolPointer(false), SyscallMonitor: &SyscallMonitorSpec{Enabled: NewBoolPointer(false)}},
				},
				NetworkPolicy: &NetworkPolicySpec{Create: NewBoolPointer(false)},
			},
		},
		{
			name: "some config",
			agent: DatadogAgentSpecAgentSpec{
				Config: &NodeAgentConfig{
					DDUrl:          NewStringPointer("www.datadog.com"),
					LeaderElection: NewBoolPointer(true),
					Dogstatsd: &DogstatsdConfig{
						DogstatsdOriginDetection: NewBoolPointer(false),
						UnixDomainSocket:         &DSDUnixDomainSocketSpec{Enabled: NewBoolPointer(true)},
					},
				},
				Image: &ImageConfig{
					Name: "gcr.io/datadog/agent:6.26.0",
				},
				DeploymentStrategy: &DaemonSetDeploymentStrategy{
					Canary: edsdatadoghqv1alpha1.DefaultExtendedDaemonSetSpecStrategyCanary(testCanary),
				},
				Apm: &APMSpec{
					HostPort: NewInt32Pointer(1664),
				},
				Process: &ProcessSpec{
					Enabled: NewBoolPointer(true),
				},
				SystemProbe: &SystemProbeSpec{
					Enabled:         NewBoolPointer(true),
					BPFDebugEnabled: NewBoolPointer(true),
				},
			},
			overrideExpected: &DatadogAgentSpecAgentSpec{
				Enabled:              NewBoolPointer(true),
				UseExtendedDaemonset: NewBoolPointer(false),
				Image: &ImageConfig{
					Tag:        defaultAgentImageTag, // TODO fix this in the patch cycle
					PullPolicy: &defaultImagePullPolicy,
				},
				Config: &NodeAgentConfig{
					LogLevel:       NewStringPointer(defaultLogLevel),
					CollectEvents:  NewBoolPointer(false),
					LivenessProbe:  GetDefaultLivenessProbe(),
					ReadinessProbe: GetDefaultReadinessProbe(),
					HealthPort:     NewInt32Pointer(5555),
					// CRI Socket specified as we use an older image
					CriSocket: &CRISocketConfig{
						DockerSocketPath: NewStringPointer(defaultDockerSocketPath),
					},
					Dogstatsd: &DogstatsdConfig{
						UnixDomainSocket: &DSDUnixDomainSocketSpec{HostFilepath: NewStringPointer("/var/run/datadog/statsd.sock")},
					},
				},
				DeploymentStrategy: &DaemonSetDeploymentStrategy{
					UpdateStrategyType: (*appsv1.DaemonSetUpdateStrategyType)(NewStringPointer("RollingUpdate")),
					RollingUpdate: DaemonSetRollingUpdateSpec{
						MaxUnavailable:            &intstr.IntOrString{Type: intstr.String, StrVal: defaultRollingUpdateMaxUnavailable},
						MaxPodSchedulerFailure:    &intstr.IntOrString{Type: intstr.String, StrVal: defaultRollingUpdateMaxPodSchedulerFailure},
						MaxParallelPodCreation:    NewInt32Pointer(defaultRollingUpdateMaxParallelPodCreation),
						SlowStartIntervalDuration: &metav1.Duration{Duration: defaultRollingUpdateSlowStartIntervalDuration},
						SlowStartAdditiveIncrease: &intstr.IntOrString{Type: intstr.String, StrVal: defaultRollingUpdateSlowStartAdditiveIncrease},
					},
					ReconcileFrequency: &metav1.Duration{Duration: defaultReconcileFrequency},
				},
				Rbac:    &RbacConfig{Create: NewBoolPointer(true)},
				Apm:     &APMSpec{Enabled: NewBoolPointer(false)},
				Process: &ProcessSpec{ProcessCollectionEnabled: NewBoolPointer(false)},
				SystemProbe: &SystemProbeSpec{
					SecCompRootPath:      "/var/lib/kubelet/seccomp",
					SecCompProfileName:   "localhost/system-probe",
					AppArmorProfileName:  "unconfined",
					ConntrackEnabled:     NewBoolPointer(false),
					EnableTCPQueueLength: NewBoolPointer(false),
					EnableOOMKill:        NewBoolPointer(false),
					CollectDNSStats:      NewBoolPointer(false),
				},
				Security: &SecuritySpec{
					Compliance: ComplianceSpec{Enabled: NewBoolPointer(false)},
					Runtime:    RuntimeSecuritySpec{Enabled: NewBoolPointer(false), SyscallMonitor: &SyscallMonitorSpec{Enabled: NewBoolPointer(false)}},
				},
				NetworkPolicy: &NetworkPolicySpec{Create: NewBoolPointer(false)},
			},
			internalDefaulted: DatadogAgentSpecAgentSpec{
				Enabled:              NewBoolPointer(true),
				UseExtendedDaemonset: NewBoolPointer(false),
				Image: &ImageConfig{
					Name:        "gcr.io/datadog/agent:6.26.0",
					Tag:         defaultAgentImageTag,
					PullPolicy:  &defaultImagePullPolicy,
					PullSecrets: &[]corev1.LocalObjectReference{},
				},
				Config: &NodeAgentConfig{
					DDUrl:                NewStringPointer("www.datadog.com"),
					LeaderElection:       NewBoolPointer(true),
					LogLevel:             NewStringPointer(defaultLogLevel),
					PodLabelsAsTags:      map[string]string{},
					PodAnnotationsAsTags: map[string]string{},
					Tags:                 []string{},
					Resources:            &corev1.ResourceRequirements{Limits: corev1.ResourceList{}, Requests: corev1.ResourceList{}},
					CollectEvents:        NewBoolPointer(false),
					LivenessProbe:        GetDefaultLivenessProbe(),
					ReadinessProbe:       GetDefaultReadinessProbe(),
					HealthPort:           NewInt32Pointer(5555),
					CriSocket: &CRISocketConfig{
						DockerSocketPath: NewStringPointer(defaultDockerSocketPath),
					},
					Dogstatsd: &DogstatsdConfig{
						DogstatsdOriginDetection: NewBoolPointer(false),
						UnixDomainSocket: &DSDUnixDomainSocketSpec{
							Enabled:      NewBoolPointer(true),
							HostFilepath: NewStringPointer("/var/run/datadog/statsd.sock"),
						},
					},
				},
				Rbac: &RbacConfig{Create: NewBoolPointer(true)},
				DeploymentStrategy: &DaemonSetDeploymentStrategy{
					UpdateStrategyType: (*appsv1.DaemonSetUpdateStrategyType)(NewStringPointer("RollingUpdate")),
					RollingUpdate: DaemonSetRollingUpdateSpec{
						MaxUnavailable:            &intstr.IntOrString{Type: intstr.String, StrVal: defaultRollingUpdateMaxUnavailable},
						MaxPodSchedulerFailure:    &intstr.IntOrString{Type: intstr.String, StrVal: defaultRollingUpdateMaxPodSchedulerFailure},
						MaxParallelPodCreation:    NewInt32Pointer(defaultRollingUpdateMaxParallelPodCreation),
						SlowStartIntervalDuration: &metav1.Duration{Duration: defaultRollingUpdateSlowStartIntervalDuration},
						SlowStartAdditiveIncrease: &intstr.IntOrString{Type: intstr.String, StrVal: defaultRollingUpdateSlowStartAdditiveIncrease},
					},
					Canary:             edsdatadoghqv1alpha1.DefaultExtendedDaemonSetSpecStrategyCanary(testCanary),
					ReconcileFrequency: &metav1.Duration{Duration: defaultReconcileFrequency},
				},
				Apm: &APMSpec{
					Enabled:  NewBoolPointer(false),
					HostPort: NewInt32Pointer(1664),
				},
				Process: &ProcessSpec{
					Enabled:                  NewBoolPointer(true),
					ProcessCollectionEnabled: NewBoolPointer(false),
				},
				SystemProbe: &SystemProbeSpec{
					Enabled:              NewBoolPointer(true),
					BPFDebugEnabled:      NewBoolPointer(true),
					SecCompRootPath:      "/var/lib/kubelet/seccomp",
					SecCompProfileName:   "localhost/system-probe",
					AppArmorProfileName:  "unconfined",
					ConntrackEnabled:     NewBoolPointer(false),
					EnableTCPQueueLength: NewBoolPointer(false),
					EnableOOMKill:        NewBoolPointer(false),
					CollectDNSStats:      NewBoolPointer(false),
				},
				Security: &SecuritySpec{
					Compliance: ComplianceSpec{Enabled: NewBoolPointer(false)},
					Runtime:    RuntimeSecuritySpec{Enabled: NewBoolPointer(false), SyscallMonitor: &SyscallMonitorSpec{Enabled: NewBoolPointer(false)}},
				},
				NetworkPolicy: &NetworkPolicySpec{Create: NewBoolPointer(false)},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultDatadogAgentSpecAgent(&tt.agent)
			assert.True(t, IsEqualStruct(got, tt.overrideExpected), "TestDefaultDatadogAgentSpecAgent override \ndiff = %s", cmp.Diff(got, tt.overrideExpected))
			assert.True(t, IsEqualStruct(tt.agent, tt.internalDefaulted), "TestDefaultDatadogAgentSpecAgent internal \ndiff = %s", cmp.Diff(tt.agent, tt.internalDefaulted))
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
				Enabled: NewBoolPointer(false),
			},
			internalDefaulted: DatadogAgentSpecClusterChecksRunnerSpec{
				Enabled: NewBoolPointer(false),
			},
		},
		{
			name: "sparse conf",
			clc: DatadogAgentSpecClusterChecksRunnerSpec{
				Config: &ClusterChecksRunnerConfig{},
				Image: &ImageConfig{
					Name: "gcr.io/datadog/agent:latest",
					Tag:  defaultAgentImageTag,
				},
			},
			overrideExpected: &DatadogAgentSpecClusterChecksRunnerSpec{
				Enabled: NewBoolPointer(true),
				Image: &ImageConfig{
					PullPolicy: &defaultImagePullPolicy,
				},
				Config: &ClusterChecksRunnerConfig{
					LogLevel:       NewStringPointer(defaultLogLevel),
					LivenessProbe:  GetDefaultLivenessProbe(),
					ReadinessProbe: GetDefaultReadinessProbe(),
					HealthPort:     NewInt32Pointer(5555),
				},
				Rbac:          &RbacConfig{Create: NewBoolPointer(true)},
				Replicas:      nil,
				NetworkPolicy: &NetworkPolicySpec{Create: NewBoolPointer(false)},
			},
			internalDefaulted: DatadogAgentSpecClusterChecksRunnerSpec{
				Enabled: NewBoolPointer(true),
				Image: &ImageConfig{
					Name:        "gcr.io/datadog/agent:latest",
					Tag:         defaultAgentImageTag,
					PullPolicy:  &defaultImagePullPolicy,
					PullSecrets: &[]corev1.LocalObjectReference{},
				},
				Config: &ClusterChecksRunnerConfig{
					LogLevel:       NewStringPointer(defaultLogLevel),
					LivenessProbe:  GetDefaultLivenessProbe(),
					ReadinessProbe: GetDefaultReadinessProbe(),
					HealthPort:     NewInt32Pointer(5555),
					Resources:      &corev1.ResourceRequirements{Limits: corev1.ResourceList{}, Requests: corev1.ResourceList{}},
				},
				Rbac:          &RbacConfig{Create: NewBoolPointer(true)},
				Replicas:      nil,
				NetworkPolicy: &NetworkPolicySpec{Create: NewBoolPointer(false)},
			},
		},
		{
			name: "some conf",
			clc: DatadogAgentSpecClusterChecksRunnerSpec{
				Config: &ClusterChecksRunnerConfig{
					LogLevel:   NewStringPointer("DEBUG"),
					HealthPort: NewInt32Pointer(1664),
				},
				Image: &ImageConfig{
					Name: "agent",
					Tag:  defaultAgentImageTag,
				},
			},
			overrideExpected: &DatadogAgentSpecClusterChecksRunnerSpec{
				Enabled: NewBoolPointer(true),
				Image: &ImageConfig{
					PullPolicy: &defaultImagePullPolicy,
				},
				Config: &ClusterChecksRunnerConfig{
					LivenessProbe:  GetDefaultLivenessProbe(),
					ReadinessProbe: GetDefaultReadinessProbe(),
				},
				Rbac:          &RbacConfig{Create: NewBoolPointer(true)},
				Replicas:      nil,
				NetworkPolicy: &NetworkPolicySpec{Create: NewBoolPointer(false)},
			},
			internalDefaulted: DatadogAgentSpecClusterChecksRunnerSpec{
				Enabled: NewBoolPointer(true),
				Image: &ImageConfig{
					Name:        "agent",
					Tag:         defaultAgentImageTag,
					PullPolicy:  &defaultImagePullPolicy,
					PullSecrets: &[]corev1.LocalObjectReference{},
				},
				Config: &ClusterChecksRunnerConfig{
					LogLevel:       NewStringPointer("DEBUG"),
					LivenessProbe:  GetDefaultLivenessProbe(),
					ReadinessProbe: GetDefaultReadinessProbe(),
					HealthPort:     NewInt32Pointer(1664),
					Resources:      &corev1.ResourceRequirements{Limits: corev1.ResourceList{}, Requests: corev1.ResourceList{}},
				},
				Rbac:          &RbacConfig{Create: NewBoolPointer(true)},
				Replicas:      nil,
				NetworkPolicy: &NetworkPolicySpec{Create: NewBoolPointer(false)},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultDatadogAgentSpecClusterChecksRunner(&tt.clc)
			assert.True(t, IsEqualStruct(got, tt.overrideExpected), "TestDefaultDatadogAgentSpecClusterChecksRunner override \ndiff = %s", cmp.Diff(got, tt.overrideExpected))
			assert.True(t, IsEqualStruct(tt.clc, tt.internalDefaulted), "TestDefaultDatadogAgentSpecClusterChecksRunner internal \ndiff = %s", cmp.Diff(tt.clc, tt.internalDefaulted))
		})
	}
}

func TestDefaultDatadogFeatureOrchestratorExplorer(t *testing.T) {
	tests := []struct {
		name        string
		orc         *DatadogFeatures
		orcOverride *OrchestratorExplorerConfig
		internal    *OrchestratorExplorerConfig
	}{
		{
			name: "empty",
			orc:  &DatadogFeatures{},
			orcOverride: &OrchestratorExplorerConfig{
				Enabled: NewBoolPointer(false),
			},
			internal: &OrchestratorExplorerConfig{
				Enabled: NewBoolPointer(false),
			},
		},
		{
			name: "enabled orchestrator explorer, no scrubbing specified",
			orc: &DatadogFeatures{
				OrchestratorExplorer: &OrchestratorExplorerConfig{
					Enabled: NewBoolPointer(true),
				},
			},
			orcOverride: &OrchestratorExplorerConfig{
				Enabled: NewBoolPointer(true),
				Scrubbing: &Scrubbing{
					Containers: NewBoolPointer(true),
				},
			},
			internal: &OrchestratorExplorerConfig{
				Enabled: NewBoolPointer(true),
				Scrubbing: &Scrubbing{
					Containers: NewBoolPointer(true),
				},
			},
		},
		{
			name: "disabled orchestrator",
			orc: &DatadogFeatures{
				OrchestratorExplorer: &OrchestratorExplorerConfig{
					Enabled: NewBoolPointer(false),
				},
			},
			orcOverride: &OrchestratorExplorerConfig{
				Enabled: NewBoolPointer(false),
			},
			internal: &OrchestratorExplorerConfig{
				Enabled: NewBoolPointer(false),
			},
		},
		{
			name: "enabled orchestrator, filled scrubbing",
			orc: &DatadogFeatures{
				OrchestratorExplorer: &OrchestratorExplorerConfig{
					Enabled: NewBoolPointer(true),
					Scrubbing: &Scrubbing{
						Containers: NewBoolPointer(true),
					},
				},
			},
			orcOverride: &OrchestratorExplorerConfig{
				Enabled: NewBoolPointer(true),
			},
			internal: &OrchestratorExplorerConfig{
				Enabled: NewBoolPointer(true),
				Scrubbing: &Scrubbing{
					Containers: NewBoolPointer(true),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DefaultDatadogFeatureOrchestratorExplorer(tt.orc)
			assert.True(t, IsEqualStruct(got, tt.orcOverride), "TestDefaultDatadogFeatureOrchestratorExplorer override \ndiff = %s", cmp.Diff(got, tt.orcOverride))
			assert.True(t, IsEqualStruct(tt.orc.OrchestratorExplorer, tt.internal), "TestDefaultDatadogFeatureOrchestratorExplorer internal \ndiff = %s", cmp.Diff(tt.orc.OrchestratorExplorer, tt.internal))
		})
	}
}
