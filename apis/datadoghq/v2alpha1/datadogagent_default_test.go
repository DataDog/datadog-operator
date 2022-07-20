// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	assert "github.com/stretchr/testify/require"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
)

const (
	valueFalse = false
	valueTrue  = true
)

func Test_defaultGlobal(t *testing.T) {
	tests := []struct {
		name    string
		ddaSpec *DatadogAgentSpec
		want    *DatadogAgentSpec
	}{
		{
			name: "global is nil",
			ddaSpec: &DatadogAgentSpec{
				Global: nil,
			},
			want: &DatadogAgentSpec{
				Global: &GlobalConfig{
					Site:     apiutils.NewStringPointer(defaultSite),
					Registry: apiutils.NewStringPointer(apicommon.DefaultImageRegistry),
					LogLevel: apiutils.NewStringPointer(defaultLogLevel),
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defaultGlobalConfig(tt.ddaSpec)

			if *tt.ddaSpec.Global.Site != *tt.want.Global.Site {
				t.Errorf("defaultGlobalConfig() Site = %v, want %v", *tt.ddaSpec.Global.Site, *tt.want.Global.Site)
			}
			if *tt.ddaSpec.Global.Registry != *tt.want.Global.Registry {
				t.Errorf("defaultGlobalConfig() Registry = %v, want %v", *tt.ddaSpec.Global.Registry, *tt.want.Global.Registry)
			}
			if *tt.ddaSpec.Global.LogLevel != *tt.want.Global.LogLevel {
				t.Errorf("defaultGlobalConfig() LogLevel = %v, want %v", *tt.ddaSpec.Global.LogLevel, *tt.want.Global.LogLevel)
			}
		})
	}
}

func Test_defaultFeatures(t *testing.T) {
	tests := []struct {
		name    string
		ddaSpec *DatadogAgentSpec
		want    *DatadogAgentSpec
	}{
		{
			name: "all features are nil",
			ddaSpec: &DatadogAgentSpec{
				Features: nil,
			},
			want: &DatadogAgentSpec{
				Features: &DatadogFeatures{
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					Dogstatsd: &DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdSocketPath),
						},
					},
					EventCollection: &EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
					},
					KubeStateMetricsCore: &KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewBoolPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewBoolPointer(defaultUseClusterChecksRunners),
					},
				},
			},
		},
		{
			name: "all features are disabled",
			ddaSpec: &DatadogAgentSpec{
				Features: &DatadogFeatures{
					LogCollection: &LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					LiveProcessCollection: &LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					OOMKill: &OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					TCPQueueLength: &TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					APM: &APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					CSPM: &CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					CWS: &CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					NPM: &NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					USM: &USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					EventCollection: &EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(valueFalse),
					},
					OrchestratorExplorer: &OrchestratorExplorerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					KubeStateMetricsCore: &KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					AdmissionController: &AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					ExternalMetricsServer: &ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					ClusterChecks: &ClusterChecksFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					PrometheusScrape: &PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					DatadogMonitor: &DatadogMonitorFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
				},
			},
			want: &DatadogAgentSpec{
				Features: &DatadogFeatures{
					LogCollection: &LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					LiveProcessCollection: &LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					OOMKill: &OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					TCPQueueLength: &TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					APM: &APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					CSPM: &CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					CWS: &CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					NPM: &NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					USM: &USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					Dogstatsd: &DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdSocketPath),
						},
					},
					EventCollection: &EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(valueFalse),
					},
					OrchestratorExplorer: &OrchestratorExplorerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					KubeStateMetricsCore: &KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					AdmissionController: &AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					ExternalMetricsServer: &ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					ClusterChecks: &ClusterChecksFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					PrometheusScrape: &PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					DatadogMonitor: &DatadogMonitorFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
				},
			},
		},
		{
			name: "logCollection is enabled",
			ddaSpec: &DatadogAgentSpec{
				Features: &DatadogFeatures{
					LogCollection: &LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueTrue),
					},
				},
			},
			want: &DatadogAgentSpec{
				Features: &DatadogFeatures{
					LogCollection: &LogCollectionFeatureConfig{
						Enabled:                    apiutils.NewBoolPointer(valueTrue),
						ContainerCollectUsingFiles: apiutils.NewBoolPointer(defaultLogContainerCollectUsingFiles),
						ContainerLogsPath:          apiutils.NewStringPointer(defaultLogContainerLogsPath),
						PodLogsPath:                apiutils.NewStringPointer(defaultLogPodLogsPath),
						ContainerSymlinksPath:      apiutils.NewStringPointer(defaultLogContainerSymlinksPath),
						TempStoragePath:            apiutils.NewStringPointer(defaultLogTempStoragePath),
					},
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					Dogstatsd: &DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdSocketPath),
						},
					},
					EventCollection: &EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
					},
					KubeStateMetricsCore: &KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewBoolPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewBoolPointer(defaultUseClusterChecksRunners),
					},
				},
			},
		},
		{
			name: "APM is enabled",
			ddaSpec: &DatadogAgentSpec{
				Features: &DatadogFeatures{
					APM: &APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueTrue),
					},
				},
			},
			want: &DatadogAgentSpec{
				Features: &DatadogFeatures{
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					APM: &APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueTrue),
						HostPortConfig: &HostPortConfig{
							Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
							Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
						},
						UnixDomainSocketConfig: &UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultAPMSocketPath),
						},
					},
					Dogstatsd: &DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdSocketPath),
						},
					},
					EventCollection: &EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
					},
					KubeStateMetricsCore: &KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewBoolPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewBoolPointer(defaultUseClusterChecksRunners),
					},
				},
			},
		},
		{
			name: "NPM is enabled",
			ddaSpec: &DatadogAgentSpec{
				Features: &DatadogFeatures{
					NPM: &NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueTrue),
					},
				},
			},
			want: &DatadogAgentSpec{
				Features: &DatadogFeatures{
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					NPM: &NPMFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(valueTrue),
						EnableConntrack: apiutils.NewBoolPointer(defaultNPMEnableConntrack),
						CollectDNSStats: apiutils.NewBoolPointer(defaultNPMCollectDNSStats),
					},
					Dogstatsd: &DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdSocketPath),
						},
					},
					EventCollection: &EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
					},
					KubeStateMetricsCore: &KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewBoolPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewBoolPointer(defaultUseClusterChecksRunners),
					},
				},
			},
		},
		{
			name: "ExternalMetricsServer is enabled",
			ddaSpec: &DatadogAgentSpec{
				Features: &DatadogFeatures{
					ExternalMetricsServer: &ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueTrue),
					},
				},
			},
			want: &DatadogAgentSpec{
				Features: &DatadogFeatures{
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					Dogstatsd: &DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdSocketPath),
						},
					},
					EventCollection: &EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
					},
					KubeStateMetricsCore: &KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ExternalMetricsServer: &ExternalMetricsServerFeatureConfig{
						Enabled:           apiutils.NewBoolPointer(valueTrue),
						UseDatadogMetrics: apiutils.NewBoolPointer(defaultDatadogMetricsEnabled),
						Port:              apiutils.NewInt32Pointer(defaultMetricsProviderPort),
					},
					ClusterChecks: &ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewBoolPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewBoolPointer(defaultUseClusterChecksRunners),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defaultFeaturesConfig(tt.ddaSpec)

			assert.True(t, apiutils.IsEqualStruct(tt.ddaSpec.Features, tt.want.Features), "defaultFeatures() \ndiff = %s", cmp.Diff(tt.ddaSpec.Features, tt.want.Features))
		})
	}
}
