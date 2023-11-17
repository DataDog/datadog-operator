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
		{
			name: "test registry defaulting based on site - EU",
			ddaSpec: &DatadogAgentSpec{
				Global: &GlobalConfig{
					Site: apiutils.NewStringPointer(defaultEuropeSite),
				},
			},
			want: &DatadogAgentSpec{
				Global: &GlobalConfig{
					Site:     apiutils.NewStringPointer(defaultEuropeSite),
					Registry: apiutils.NewStringPointer(apicommon.DefaultEuropeImageRegistry),
					LogLevel: apiutils.NewStringPointer(defaultLogLevel),
				},
			},
		},
		{
			name: "test registry defaulting based on site - Asia",
			ddaSpec: &DatadogAgentSpec{
				Global: &GlobalConfig{
					Site: apiutils.NewStringPointer(defaultAsiaSite),
				},
			},
			want: &DatadogAgentSpec{
				Global: &GlobalConfig{
					Site:     apiutils.NewStringPointer(defaultAsiaSite),
					Registry: apiutils.NewStringPointer(apicommon.DefaultAsiaImageRegistry),
					LogLevel: apiutils.NewStringPointer(defaultLogLevel),
				},
			},
		},
		{
			name: "test registry defaulting based on site - Gov",
			ddaSpec: &DatadogAgentSpec{
				Global: &GlobalConfig{
					Site: apiutils.NewStringPointer(defaultGovSite),
				},
			},
			want: &DatadogAgentSpec{
				Global: &GlobalConfig{
					Site:     apiutils.NewStringPointer(defaultGovSite),
					Registry: apiutils.NewStringPointer(apicommon.DefaultGovImageRegistry),
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
					ProcessDiscovery: &ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					Dogstatsd: &DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdHostSocketPath),
						},
					},
					APM: &APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAPMEnabled),
						HostPortConfig: &HostPortConfig{
							Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
							Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultAPMSocketHostPath),
						},
					},
					OTLP: &OTLPFeatureConfig{Receiver: OTLPReceiverConfig{Protocols: OTLPProtocolsConfig{
						GRPC: &OTLPGRPCConfig{
							Enabled:  apiutils.NewBoolPointer(defaultOTLPGRPCEnabled),
							Endpoint: apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &OTLPHTTPConfig{
							Enabled:  apiutils.NewBoolPointer(defaultOTLPHTTPEnabled),
							Endpoint: apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
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
					AdmissionController: &AdmissionControllerFeatureConfig{
						Enabled:          apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
						MutateUnlabelled: apiutils.NewBoolPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewStringPointer(defaultAdmissionServiceName),
					},
					RemoteConfiguration: &RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
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
					ProcessDiscovery: &ProcessDiscoveryFeatureConfig{
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
					OTLP: &OTLPFeatureConfig{Receiver: OTLPReceiverConfig{Protocols: OTLPProtocolsConfig{
						GRPC: &OTLPGRPCConfig{Enabled: apiutils.NewBoolPointer(valueFalse)},
						HTTP: &OTLPHTTPConfig{Enabled: apiutils.NewBoolPointer(valueFalse)},
					}}},
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
					RemoteConfiguration: &RemoteConfigurationFeatureConfig{
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
					ProcessDiscovery: &ProcessDiscoveryFeatureConfig{
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
							Path:    apiutils.NewStringPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &OTLPFeatureConfig{Receiver: OTLPReceiverConfig{Protocols: OTLPProtocolsConfig{
						GRPC: &OTLPGRPCConfig{
							Enabled:  apiutils.NewBoolPointer(defaultOTLPGRPCEnabled),
							Endpoint: apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &OTLPHTTPConfig{
							Enabled:  apiutils.NewBoolPointer(defaultOTLPHTTPEnabled),
							Endpoint: apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
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
					RemoteConfiguration: &RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
				},
			},
		},
		{
			name: "liveProcess is enabled",
			ddaSpec: &DatadogAgentSpec{
				Features: &DatadogFeatures{
					LiveProcessCollection: &LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueTrue),
					},
				},
			},
			want: &DatadogAgentSpec{
				Features: &DatadogFeatures{
					LiveProcessCollection: &LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueTrue),
					},
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &ProcessDiscoveryFeatureConfig{
						// The agent will automatically disable process discovery collection in this case
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					Dogstatsd: &DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &OTLPFeatureConfig{Receiver: OTLPReceiverConfig{Protocols: OTLPProtocolsConfig{
						GRPC: &OTLPGRPCConfig{
							Enabled:  apiutils.NewBoolPointer(defaultOTLPGRPCEnabled),
							Endpoint: apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &OTLPHTTPConfig{
							Enabled:  apiutils.NewBoolPointer(defaultOTLPHTTPEnabled),
							Endpoint: apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
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
					AdmissionController: &AdmissionControllerFeatureConfig{
						Enabled:          apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
						MutateUnlabelled: apiutils.NewBoolPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewStringPointer(defaultAdmissionServiceName),
					},
					APM: &APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAPMEnabled),
						HostPortConfig: &HostPortConfig{
							Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
							Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultAPMSocketHostPath),
						},
					},
					RemoteConfiguration: &RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
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
					ProcessDiscovery: &ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					Dogstatsd: &DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &OTLPFeatureConfig{Receiver: OTLPReceiverConfig{Protocols: OTLPProtocolsConfig{
						GRPC: &OTLPGRPCConfig{
							Enabled:  apiutils.NewBoolPointer(defaultOTLPGRPCEnabled),
							Endpoint: apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &OTLPHTTPConfig{
							Enabled:  apiutils.NewBoolPointer(defaultOTLPHTTPEnabled),
							Endpoint: apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
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
					AdmissionController: &AdmissionControllerFeatureConfig{
						Enabled:          apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
						MutateUnlabelled: apiutils.NewBoolPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewStringPointer(defaultAdmissionServiceName),
					},
					APM: &APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAPMEnabled),
						HostPortConfig: &HostPortConfig{
							Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
							Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultAPMSocketHostPath),
						},
					},
					RemoteConfiguration: &RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
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
					ProcessDiscovery: &ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					APM: &APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueTrue),
						HostPortConfig: &HostPortConfig{
							Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
							Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
						},
						UnixDomainSocketConfig: &UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultAPMSocketHostPath),
						},
					},
					Dogstatsd: &DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &OTLPFeatureConfig{Receiver: OTLPReceiverConfig{Protocols: OTLPProtocolsConfig{
						GRPC: &OTLPGRPCConfig{
							Enabled:  apiutils.NewBoolPointer(defaultOTLPGRPCEnabled),
							Endpoint: apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &OTLPHTTPConfig{
							Enabled:  apiutils.NewBoolPointer(defaultOTLPHTTPEnabled),
							Endpoint: apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
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
					AdmissionController: &AdmissionControllerFeatureConfig{
						Enabled:          apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
						MutateUnlabelled: apiutils.NewBoolPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewStringPointer(defaultAdmissionServiceName),
					},
					RemoteConfiguration: &RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
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
					ProcessDiscovery: &ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
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
							Path:    apiutils.NewStringPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &OTLPFeatureConfig{Receiver: OTLPReceiverConfig{Protocols: OTLPProtocolsConfig{
						GRPC: &OTLPGRPCConfig{
							Enabled:  apiutils.NewBoolPointer(defaultOTLPGRPCEnabled),
							Endpoint: apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &OTLPHTTPConfig{
							Enabled:  apiutils.NewBoolPointer(defaultOTLPHTTPEnabled),
							Endpoint: apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
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
					APM: &APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAPMEnabled),
						HostPortConfig: &HostPortConfig{
							Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
							Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultAPMSocketHostPath),
						},
					},
					AdmissionController: &AdmissionControllerFeatureConfig{
						Enabled:          apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
						MutateUnlabelled: apiutils.NewBoolPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewStringPointer(defaultAdmissionServiceName),
					},
					RemoteConfiguration: &RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
				},
			},
		},
		{
			name: "OTLP is enabled",
			ddaSpec: &DatadogAgentSpec{
				Features: &DatadogFeatures{
					OTLP: &OTLPFeatureConfig{Receiver: OTLPReceiverConfig{Protocols: OTLPProtocolsConfig{
						GRPC: &OTLPGRPCConfig{
							Enabled:  apiutils.NewBoolPointer(true),
							Endpoint: apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &OTLPHTTPConfig{
							Enabled:  apiutils.NewBoolPointer(true),
							Endpoint: apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
				},
			},
			want: &DatadogAgentSpec{
				Features: &DatadogFeatures{
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					Dogstatsd: &DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &OTLPFeatureConfig{Receiver: OTLPReceiverConfig{Protocols: OTLPProtocolsConfig{
						GRPC: &OTLPGRPCConfig{
							Enabled:  apiutils.NewBoolPointer(true),
							Endpoint: apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &OTLPHTTPConfig{
							Enabled:  apiutils.NewBoolPointer(true),
							Endpoint: apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
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
					AdmissionController: &AdmissionControllerFeatureConfig{
						Enabled:          apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
						MutateUnlabelled: apiutils.NewBoolPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewStringPointer(defaultAdmissionServiceName),
					},
					APM: &APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAPMEnabled),
						HostPortConfig: &HostPortConfig{
							Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
							Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultAPMSocketHostPath),
						},
					},
					RemoteConfiguration: &RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
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
					ProcessDiscovery: &ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					Dogstatsd: &DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &OTLPFeatureConfig{Receiver: OTLPReceiverConfig{Protocols: OTLPProtocolsConfig{
						GRPC: &OTLPGRPCConfig{
							Enabled:  apiutils.NewBoolPointer(defaultOTLPGRPCEnabled),
							Endpoint: apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &OTLPHTTPConfig{
							Enabled:  apiutils.NewBoolPointer(defaultOTLPHTTPEnabled),
							Endpoint: apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
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
					APM: &APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAPMEnabled),
						HostPortConfig: &HostPortConfig{
							Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
							Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultAPMSocketHostPath),
						},
					},
					ExternalMetricsServer: &ExternalMetricsServerFeatureConfig{
						Enabled:            apiutils.NewBoolPointer(valueTrue),
						RegisterAPIService: apiutils.NewBoolPointer(defaultRegisterAPIService),
						UseDatadogMetrics:  apiutils.NewBoolPointer(defaultDatadogMetricsEnabled),
						Port:               apiutils.NewInt32Pointer(defaultMetricsProviderPort),
					},
					ClusterChecks: &ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewBoolPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewBoolPointer(defaultUseClusterChecksRunners),
					},
					AdmissionController: &AdmissionControllerFeatureConfig{
						Enabled:          apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
						MutateUnlabelled: apiutils.NewBoolPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewStringPointer(defaultAdmissionServiceName),
					},
					RemoteConfiguration: &RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
				},
			},
		},
		{
			name: "ClusterChecks feature with a field set, but \"enabled\" field not set",
			ddaSpec: &DatadogAgentSpec{
				Features: &DatadogFeatures{
					ClusterChecks: &ClusterChecksFeatureConfig{
						UseClusterChecksRunners: apiutils.NewBoolPointer(false),
					},
				},
			},
			want: &DatadogAgentSpec{
				Features: &DatadogFeatures{
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					Dogstatsd: &DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &OTLPFeatureConfig{Receiver: OTLPReceiverConfig{Protocols: OTLPProtocolsConfig{
						GRPC: &OTLPGRPCConfig{
							Enabled:  apiutils.NewBoolPointer(defaultOTLPGRPCEnabled),
							Endpoint: apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &OTLPHTTPConfig{
							Enabled:  apiutils.NewBoolPointer(defaultOTLPHTTPEnabled),
							Endpoint: apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
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
						UseClusterChecksRunners: apiutils.NewBoolPointer(false),
					},
					AdmissionController: &AdmissionControllerFeatureConfig{
						Enabled:          apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
						MutateUnlabelled: apiutils.NewBoolPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewStringPointer(defaultAdmissionServiceName),
					},
					APM: &APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAPMEnabled),
						HostPortConfig: &HostPortConfig{
							Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
							Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultAPMSocketHostPath),
						},
					},
					RemoteConfiguration: &RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
				},
			},
		},
		{
			name: "Admission controller enabled unset, other fields set",
			ddaSpec: &DatadogAgentSpec{
				Features: &DatadogFeatures{
					AdmissionController: &AdmissionControllerFeatureConfig{
						MutateUnlabelled:       apiutils.NewBoolPointer(true),
						AgentCommunicationMode: apiutils.NewStringPointer("socket"),
					},
				},
			},
			want: &DatadogAgentSpec{
				Features: &DatadogFeatures{
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					APM: &APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAPMEnabled),
						HostPortConfig: &HostPortConfig{
							Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
							Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultAPMSocketHostPath),
						},
					},
					ProcessDiscovery: &ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					Dogstatsd: &DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &OTLPFeatureConfig{Receiver: OTLPReceiverConfig{Protocols: OTLPProtocolsConfig{
						GRPC: &OTLPGRPCConfig{
							Enabled:  apiutils.NewBoolPointer(defaultOTLPGRPCEnabled),
							Endpoint: apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &OTLPHTTPConfig{
							Enabled:  apiutils.NewBoolPointer(defaultOTLPHTTPEnabled),
							Endpoint: apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
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
						UseClusterChecksRunners: apiutils.NewBoolPointer(false),
					},
					AdmissionController: &AdmissionControllerFeatureConfig{
						Enabled:                apiutils.NewBoolPointer(true),
						MutateUnlabelled:       apiutils.NewBoolPointer(true),
						ServiceName:            apiutils.NewStringPointer(defaultAdmissionServiceName),
						AgentCommunicationMode: apiutils.NewStringPointer("socket"),
					},
					RemoteConfiguration: &RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
				},
			},
		},
		{
			name: "Orchestrator explorer enabled unset, other fields set",
			ddaSpec: &DatadogAgentSpec{
				Features: &DatadogFeatures{
					OrchestratorExplorer: &OrchestratorExplorerFeatureConfig{
						CustomResources: []string{"datadoghq.com/v1alpha1/datadogmetrics"},
					},
				},
			},
			want: &DatadogAgentSpec{
				Features: &DatadogFeatures{
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					Dogstatsd: &DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &OTLPFeatureConfig{Receiver: OTLPReceiverConfig{Protocols: OTLPProtocolsConfig{
						GRPC: &OTLPGRPCConfig{
							Enabled:  apiutils.NewBoolPointer(defaultOTLPGRPCEnabled),
							Endpoint: apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &OTLPHTTPConfig{
							Enabled:  apiutils.NewBoolPointer(defaultOTLPHTTPEnabled),
							Endpoint: apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
					EventCollection: &EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					APM: &APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAPMEnabled),
						HostPortConfig: &HostPortConfig{
							Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
							Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultAPMSocketHostPath),
						},
					},
					OrchestratorExplorer: &OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
						CustomResources: []string{"datadoghq.com/v1alpha1/datadogmetrics"},
					},
					KubeStateMetricsCore: &KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewBoolPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewBoolPointer(false),
					},
					AdmissionController: &AdmissionControllerFeatureConfig{
						Enabled:          apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
						MutateUnlabelled: apiutils.NewBoolPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewStringPointer(defaultAdmissionServiceName),
					},
					RemoteConfiguration: &RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
				},
			},
		},
		{
			// This test sets same defaults as the one with `Features: nil`; and leaves other configs as empty structs.
			name: "all feature configs are empty structs, configures defaults where applicable, leaves others empty",
			ddaSpec: &DatadogAgentSpec{
				Features: &DatadogFeatures{
					LogCollection:           &LogCollectionFeatureConfig{},
					LiveProcessCollection:   &LiveProcessCollectionFeatureConfig{},
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{},
					ProcessDiscovery:        &ProcessDiscoveryFeatureConfig{},
					OOMKill:                 &OOMKillFeatureConfig{},
					TCPQueueLength:          &TCPQueueLengthFeatureConfig{},
					APM:                     &APMFeatureConfig{},
					CSPM:                    &CSPMFeatureConfig{},
					CWS:                     &CWSFeatureConfig{},
					NPM:                     &NPMFeatureConfig{},
					USM:                     &USMFeatureConfig{},
					OTLP:                    &OTLPFeatureConfig{},
					EventCollection:         &EventCollectionFeatureConfig{},
					OrchestratorExplorer:    &OrchestratorExplorerFeatureConfig{},
					KubeStateMetricsCore:    &KubeStateMetricsCoreFeatureConfig{},
					AdmissionController:     &AdmissionControllerFeatureConfig{},
					ExternalMetricsServer:   &ExternalMetricsServerFeatureConfig{},
					ClusterChecks:           &ClusterChecksFeatureConfig{},
					PrometheusScrape:        &PrometheusScrapeFeatureConfig{},
					RemoteConfiguration:     &RemoteConfigurationFeatureConfig{},
				},
			},
			want: &DatadogAgentSpec{
				Features: &DatadogFeatures{
					LogCollection:         &LogCollectionFeatureConfig{},
					LiveProcessCollection: &LiveProcessCollectionFeatureConfig{},
					ProcessDiscovery: &ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill:        &OOMKillFeatureConfig{},
					TCPQueueLength: &TCPQueueLengthFeatureConfig{},
					APM: &APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAPMEnabled),
						HostPortConfig: &HostPortConfig{
							Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
							Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultAPMSocketHostPath),
						},
					},
					CSPM:                  &CSPMFeatureConfig{},
					CWS:                   &CWSFeatureConfig{},
					NPM:                   &NPMFeatureConfig{},
					USM:                   &USMFeatureConfig{},
					ExternalMetricsServer: &ExternalMetricsServerFeatureConfig{},
					PrometheusScrape:      &PrometheusScrapeFeatureConfig{},

					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					Dogstatsd: &DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &OTLPFeatureConfig{Receiver: OTLPReceiverConfig{Protocols: OTLPProtocolsConfig{
						GRPC: &OTLPGRPCConfig{
							Enabled:  apiutils.NewBoolPointer(defaultOTLPGRPCEnabled),
							Endpoint: apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &OTLPHTTPConfig{
							Enabled:  apiutils.NewBoolPointer(defaultOTLPHTTPEnabled),
							Endpoint: apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
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
					AdmissionController: &AdmissionControllerFeatureConfig{
						Enabled:          apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
						MutateUnlabelled: apiutils.NewBoolPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewStringPointer(defaultAdmissionServiceName),
					},
					RemoteConfiguration: &RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
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
