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
					LogCollection: &LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &EBPFCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultEBPFCheckEnabled),
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
					CSPM: &CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCSPMEnabled),
					},
					CWS: &CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCWSEnabled),
					},
					NPM: &NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultNPMEnabled),
					},
					USM: &USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
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
					RemoteConfiguration: &RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultExternalMetricsServerEnabled),
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
					PrometheusScrape: &PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					HighAvailability: &HighAvailabilityFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultHighAvailabilityEnabled),
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
					EBPFCheck: &EBPFCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultEBPFCheckEnabled),
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
					HighAvailability: &HighAvailabilityFeatureConfig{
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
					EBPFCheck: &EBPFCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultEBPFCheckEnabled),
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
					HighAvailability: &HighAvailabilityFeatureConfig{
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
					LogCollection: &LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueTrue),
					},
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &EBPFCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultEBPFCheckEnabled),
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
					CSPM: &CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCSPMEnabled),
					},
					CWS: &CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCWSEnabled),
					},
					NPM: &NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultNPMEnabled),
					},
					USM: &USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
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
					RemoteConfiguration: &RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultExternalMetricsServerEnabled),
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
					PrometheusScrape: &PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					HighAvailability: &HighAvailabilityFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultHighAvailabilityEnabled),
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
					LiveProcessCollection: &LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &EBPFCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultEBPFCheckEnabled),
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
					CSPM: &CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCSPMEnabled),
					},
					CWS: &CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCWSEnabled),
					},
					NPM: &NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultNPMEnabled),
					},
					USM: &USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
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
					RemoteConfiguration: &RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultExternalMetricsServerEnabled),
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
					PrometheusScrape: &PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					HighAvailability: &HighAvailabilityFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultHighAvailabilityEnabled),
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
					LogCollection: &LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &EBPFCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultEBPFCheckEnabled),
					},
					APM: &APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueTrue),
						HostPortConfig: &HostPortConfig{
							Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
							Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultAPMSocketHostPath),
						},
					},
					CSPM: &CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCSPMEnabled),
					},
					CWS: &CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCWSEnabled),
					},
					NPM: &NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultNPMEnabled),
					},
					USM: &USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
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
					RemoteConfiguration: &RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultExternalMetricsServerEnabled),
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
					PrometheusScrape: &PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					HighAvailability: &HighAvailabilityFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultHighAvailabilityEnabled),
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
					LogCollection: &LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &EBPFCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultEBPFCheckEnabled),
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
					CSPM: &CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCSPMEnabled),
					},
					CWS: &CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCWSEnabled),
					},
					NPM: &NPMFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(valueTrue),
						EnableConntrack: apiutils.NewBoolPointer(defaultNPMEnableConntrack),
						CollectDNSStats: apiutils.NewBoolPointer(defaultNPMCollectDNSStats),
					},
					USM: &USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
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
					RemoteConfiguration: &RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultExternalMetricsServerEnabled),
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
					PrometheusScrape: &PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					HighAvailability: &HighAvailabilityFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultHighAvailabilityEnabled),
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
					LogCollection: &LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &EBPFCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultEBPFCheckEnabled),
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
					CSPM: &CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCSPMEnabled),
					},
					CWS: &CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCWSEnabled),
					},
					NPM: &NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultNPMEnabled),
					},
					USM: &USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
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
							Enabled:  apiutils.NewBoolPointer(valueTrue),
							Endpoint: apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &OTLPHTTPConfig{
							Enabled:  apiutils.NewBoolPointer(valueTrue),
							Endpoint: apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultExternalMetricsServerEnabled),
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
					PrometheusScrape: &PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					HighAvailability: &HighAvailabilityFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultHighAvailabilityEnabled),
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
					LogCollection: &LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &EBPFCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultEBPFCheckEnabled),
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
					CSPM: &CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCSPMEnabled),
					},
					CWS: &CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCWSEnabled),
					},
					NPM: &NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultNPMEnabled),
					},
					USM: &USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
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
					RemoteConfiguration: &RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &ExternalMetricsServerFeatureConfig{
						Enabled:            apiutils.NewBoolPointer(valueTrue),
						RegisterAPIService: apiutils.NewBoolPointer(defaultRegisterAPIService),
						UseDatadogMetrics:  apiutils.NewBoolPointer(defaultDatadogMetricsEnabled),
						Port:               apiutils.NewInt32Pointer(defaultMetricsProviderPort),
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
					PrometheusScrape: &PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					HighAvailability: &HighAvailabilityFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultHighAvailabilityEnabled),
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
					LogCollection: &LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &EBPFCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultEBPFCheckEnabled),
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
					CSPM: &CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCSPMEnabled),
					},
					CWS: &CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCWSEnabled),
					},
					NPM: &NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultNPMEnabled),
					},
					USM: &USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
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
					RemoteConfiguration: &RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewBoolPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewBoolPointer(valueFalse),
					},
					AdmissionController: &AdmissionControllerFeatureConfig{
						Enabled:          apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
						MutateUnlabelled: apiutils.NewBoolPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewStringPointer(defaultAdmissionServiceName),
					},
					PrometheusScrape: &PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					HighAvailability: &HighAvailabilityFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultHighAvailabilityEnabled),
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
					LogCollection: &LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &EBPFCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultEBPFCheckEnabled),
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
					CSPM: &CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCSPMEnabled),
					},
					CWS: &CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCWSEnabled),
					},
					NPM: &NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultNPMEnabled),
					},
					USM: &USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
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
					RemoteConfiguration: &RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewBoolPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewBoolPointer(defaultUseClusterChecksRunners),
					},
					AdmissionController: &AdmissionControllerFeatureConfig{
						Enabled:                apiutils.NewBoolPointer(valueTrue),
						MutateUnlabelled:       apiutils.NewBoolPointer(valueTrue),
						ServiceName:            apiutils.NewStringPointer(defaultAdmissionServiceName),
						AgentCommunicationMode: apiutils.NewStringPointer("socket"),
					},
					PrometheusScrape: &PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					HighAvailability: &HighAvailabilityFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultHighAvailabilityEnabled),
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
					LogCollection: &LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &EBPFCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultEBPFCheckEnabled),
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
					CSPM: &CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCSPMEnabled),
					},
					CWS: &CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCWSEnabled),
					},
					NPM: &NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultNPMEnabled),
					},
					USM: &USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
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
					RemoteConfiguration: &RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
						CustomResources: []string{"datadoghq.com/v1alpha1/datadogmetrics"},
					},
					ExternalMetricsServer: &ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultExternalMetricsServerEnabled),
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
					PrometheusScrape: &PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					HighAvailability: &HighAvailabilityFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultHighAvailabilityEnabled),
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
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{},
					LiveProcessCollection:   &LiveProcessCollectionFeatureConfig{},
					ProcessDiscovery:        &ProcessDiscoveryFeatureConfig{},
					OOMKill:                 &OOMKillFeatureConfig{},
					TCPQueueLength:          &TCPQueueLengthFeatureConfig{},
					EBPFCheck:               &EBPFCheckFeatureConfig{},
					APM:                     &APMFeatureConfig{},
					CSPM:                    &CSPMFeatureConfig{},
					CWS:                     &CWSFeatureConfig{},
					NPM:                     &NPMFeatureConfig{},
					USM:                     &USMFeatureConfig{},
					OTLP:                    &OTLPFeatureConfig{},
					RemoteConfiguration:     &RemoteConfigurationFeatureConfig{},
					EventCollection:         &EventCollectionFeatureConfig{},
					OrchestratorExplorer:    &OrchestratorExplorerFeatureConfig{},
					KubeStateMetricsCore:    &KubeStateMetricsCoreFeatureConfig{},
					AdmissionController:     &AdmissionControllerFeatureConfig{},
					ExternalMetricsServer:   &ExternalMetricsServerFeatureConfig{},
					ClusterChecks:           &ClusterChecksFeatureConfig{},
					PrometheusScrape:        &PrometheusScrapeFeatureConfig{},
					HighAvailability:        &HighAvailabilityFeatureConfig{},
				},
			},
			want: &DatadogAgentSpec{
				Features: &DatadogFeatures{
					LogCollection: &LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &EBPFCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultEBPFCheckEnabled),
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
					CSPM: &CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCSPMEnabled),
					},
					CWS: &CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCWSEnabled),
					},
					NPM: &NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultNPMEnabled),
					},
					USM: &USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
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
					RemoteConfiguration: &RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultExternalMetricsServerEnabled),
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
					PrometheusScrape: &PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					HighAvailability: &HighAvailabilityFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultHighAvailabilityEnabled),
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
