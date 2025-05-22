// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaults

import (
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/images"

	"github.com/google/go-cmp/cmp"
	assert "github.com/stretchr/testify/require"
)

const (
	valueFalse = false
	valueTrue  = true
)

func Test_defaultGlobal(t *testing.T) {
	tests := []struct {
		name    string
		ddaSpec *v2alpha1.DatadogAgentSpec
		want    *v2alpha1.DatadogAgentSpec
	}{
		{
			name: "global is nil",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Global: nil,
			},
			want: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					Site:     apiutils.NewStringPointer(defaultSite),
					Registry: apiutils.NewStringPointer(images.DefaultImageRegistry),
					LogLevel: apiutils.NewStringPointer(defaultLogLevel),
				},
			},
		},
		{
			name: "test registry defaulting based on site - EU",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					Site: apiutils.NewStringPointer(defaultEuropeSite),
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					Site:     apiutils.NewStringPointer(defaultEuropeSite),
					Registry: apiutils.NewStringPointer(images.DefaultEuropeImageRegistry),
					LogLevel: apiutils.NewStringPointer(defaultLogLevel),
				},
			},
		},
		{
			name: "test registry defaulting based on site - Asia",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					Site: apiutils.NewStringPointer(defaultAsiaSite),
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					Site:     apiutils.NewStringPointer(defaultAsiaSite),
					Registry: apiutils.NewStringPointer(images.DefaultAsiaImageRegistry),
					LogLevel: apiutils.NewStringPointer(defaultLogLevel),
				},
			},
		},
		{
			name: "test registry defaulting based on site - Azure",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					Site: apiutils.NewStringPointer(defaultAzureSite),
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					Site:     apiutils.NewStringPointer(defaultAzureSite),
					Registry: apiutils.NewStringPointer(images.DefaultAzureImageRegistry),
					LogLevel: apiutils.NewStringPointer(defaultLogLevel),
				},
			},
		},
		{
			name: "test registry defaulting based on site - Gov",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					Site: apiutils.NewStringPointer(defaultGovSite),
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					Site:     apiutils.NewStringPointer(defaultGovSite),
					Registry: apiutils.NewStringPointer(images.DefaultGovImageRegistry),
					LogLevel: apiutils.NewStringPointer(defaultLogLevel),
				},
			},
		},
		{
			name: "test FIPS Agent defaulting - disabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					UseFIPSAgent: apiutils.NewBoolPointer(defaultUseFIPSAgent),
					Site:         apiutils.NewStringPointer(defaultSite),
					Registry:     apiutils.NewStringPointer(images.DefaultImageRegistry),
					LogLevel:     apiutils.NewStringPointer(defaultLogLevel),
				},
			},
		},
		{
			name: "test FIPS proxy defaulting - disabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					FIPS: &v2alpha1.FIPSConfig{
						Enabled: apiutils.NewBoolPointer(defaultFIPSProxyEnabled),
					},
					Site:     apiutils.NewStringPointer(defaultSite),
					Registry: apiutils.NewStringPointer(images.DefaultImageRegistry),
					LogLevel: apiutils.NewStringPointer(defaultLogLevel),
				},
			},
		},
		{
			name: "test FIPS defaulting - enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					FIPS: &v2alpha1.FIPSConfig{
						Enabled: apiutils.NewBoolPointer(true),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					FIPS: &v2alpha1.FIPSConfig{
						Enabled: apiutils.NewBoolPointer(true),
						Image: &v2alpha1.AgentImageConfig{
							Name: defaultFIPSImageName,
							Tag:  defaultFIPSImageTag,
						},
						LocalAddress: apiutils.NewStringPointer(defaultFIPSLocalAddress),
						Port:         apiutils.NewInt32Pointer(defaultFIPSPort),
						PortRange:    apiutils.NewInt32Pointer(defaultFIPSPortRange),
						UseHTTPS:     apiutils.NewBoolPointer(defaultFIPSUseHTTPS),
					},
					Site:     apiutils.NewStringPointer(defaultSite),
					Registry: apiutils.NewStringPointer(images.DefaultImageRegistry),
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
		ddaSpec *v2alpha1.DatadogAgentSpec
		want    *v2alpha1.DatadogAgentSpec
	}{
		{
			name: "all features are nil",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: nil,
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultGPUMonitoringEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
							Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           apiutils.NewBoolPointer(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: apiutils.NewBoolPointer(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
						ErrorTrackingStandalone: &v2alpha1.ErrorTrackingStandalone{
							Enabled: apiutils.NewBoolPointer(defaultAPMErrorTrackingStandalone),
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        apiutils.NewBoolPointer(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        apiutils.NewBoolPointer(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewBoolPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewBoolPointer(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: apiutils.NewBoolPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewStringPointer(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewBoolPointer(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultHelmCheckEnabled),
					},
				},
			},
		},
		{
			name: "all features are disabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultGPUMonitoringEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewBoolPointer(valueFalse),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewBoolPointer(valueFalse),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewBoolPointer(valueFalse),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{Enabled: apiutils.NewBoolPointer(valueFalse)},
						HTTP: &v2alpha1.OTLPHTTPConfig{Enabled: apiutils.NewBoolPointer(valueFalse)},
					}}},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(valueFalse),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled:    apiutils.NewBoolPointer(valueFalse),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{Enabled: apiutils.NewBoolPointer(valueFalse)},
						Mutation:   &v2alpha1.AdmissionControllerMutationConfig{Enabled: apiutils.NewBoolPointer(valueFalse)},
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultGPUMonitoringEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewBoolPointer(valueFalse),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewBoolPointer(valueFalse),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewBoolPointer(valueFalse),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        apiutils.NewBoolPointer(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        apiutils.NewBoolPointer(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(valueFalse),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewBoolPointer(valueFalse),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewBoolPointer(valueFalse),
						},
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewBoolPointer(valueFalse),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewBoolPointer(valueFalse),
						},
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueFalse),
					},
				},
			},
		},
		{
			name: "liveProcess is enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueTrue),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueTrue),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultGPUMonitoringEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
							Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           apiutils.NewBoolPointer(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: apiutils.NewBoolPointer(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
						ErrorTrackingStandalone: &v2alpha1.ErrorTrackingStandalone{
							Enabled: apiutils.NewBoolPointer(defaultAPMErrorTrackingStandalone),
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        apiutils.NewBoolPointer(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        apiutils.NewBoolPointer(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewBoolPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewBoolPointer(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: apiutils.NewBoolPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewStringPointer(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewBoolPointer(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultHelmCheckEnabled),
					},
				},
			},
		},
		{
			name: "logCollection is enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueTrue),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled:                    apiutils.NewBoolPointer(valueTrue),
						ContainerCollectUsingFiles: apiutils.NewBoolPointer(defaultLogContainerCollectUsingFiles),
						ContainerLogsPath:          apiutils.NewStringPointer(defaultLogContainerLogsPath),
						PodLogsPath:                apiutils.NewStringPointer(defaultLogPodLogsPath),
						ContainerSymlinksPath:      apiutils.NewStringPointer(defaultLogContainerSymlinksPath),
						TempStoragePath:            apiutils.NewStringPointer(common.DefaultLogTempStoragePath),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultGPUMonitoringEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
							Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           apiutils.NewBoolPointer(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: apiutils.NewBoolPointer(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
						ErrorTrackingStandalone: &v2alpha1.ErrorTrackingStandalone{
							Enabled: apiutils.NewBoolPointer(defaultAPMErrorTrackingStandalone),
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        apiutils.NewBoolPointer(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        apiutils.NewBoolPointer(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewBoolPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewBoolPointer(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: apiutils.NewBoolPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewStringPointer(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewBoolPointer(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultHelmCheckEnabled),
					},
				},
			},
		},
		{
			name: "APM is enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueTrue),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultGPUMonitoringEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueTrue),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
							Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           apiutils.NewBoolPointer(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: apiutils.NewBoolPointer(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
						ErrorTrackingStandalone: &v2alpha1.ErrorTrackingStandalone{
							Enabled: apiutils.NewBoolPointer(defaultAPMErrorTrackingStandalone),
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        apiutils.NewBoolPointer(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        apiutils.NewBoolPointer(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewBoolPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewBoolPointer(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: apiutils.NewBoolPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewStringPointer(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewBoolPointer(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultHelmCheckEnabled),
					},
				},
			},
		},
		{
			name: "NPM is enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueTrue),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultGPUMonitoringEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
							Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           apiutils.NewBoolPointer(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: apiutils.NewBoolPointer(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
						ErrorTrackingStandalone: &v2alpha1.ErrorTrackingStandalone{
							Enabled: apiutils.NewBoolPointer(defaultAPMErrorTrackingStandalone),
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(valueTrue),
						EnableConntrack: apiutils.NewBoolPointer(defaultNPMEnableConntrack),
						CollectDNSStats: apiutils.NewBoolPointer(defaultNPMCollectDNSStats),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        apiutils.NewBoolPointer(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        apiutils.NewBoolPointer(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewBoolPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewBoolPointer(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: apiutils.NewBoolPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewStringPointer(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewBoolPointer(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultHelmCheckEnabled),
					},
				},
			},
		},
		{
			name: "OTLP is enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:  apiutils.NewBoolPointer(true),
							Endpoint: apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:  apiutils.NewBoolPointer(true),
							Endpoint: apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultGPUMonitoringEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
							Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           apiutils.NewBoolPointer(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: apiutils.NewBoolPointer(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
						ErrorTrackingStandalone: &v2alpha1.ErrorTrackingStandalone{
							Enabled: apiutils.NewBoolPointer(defaultAPMErrorTrackingStandalone),
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        apiutils.NewBoolPointer(valueTrue),
							HostPortConfig: &v2alpha1.HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultOTLPGRPCHostPortEnabled)},
							Endpoint:       apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        apiutils.NewBoolPointer(valueTrue),
							HostPortConfig: &v2alpha1.HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultOTLPGRPCHostPortEnabled)},
							Endpoint:       apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewBoolPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewBoolPointer(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: apiutils.NewBoolPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewStringPointer(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewBoolPointer(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultHelmCheckEnabled),
					},
				},
			},
		},
		{
			name: "ExternalMetricsServer is enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueTrue),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultGPUMonitoringEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
							Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           apiutils.NewBoolPointer(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: apiutils.NewBoolPointer(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
						ErrorTrackingStandalone: &v2alpha1.ErrorTrackingStandalone{
							Enabled: apiutils.NewBoolPointer(defaultAPMErrorTrackingStandalone),
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        apiutils.NewBoolPointer(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        apiutils.NewBoolPointer(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled:            apiutils.NewBoolPointer(valueTrue),
						RegisterAPIService: apiutils.NewBoolPointer(defaultRegisterAPIService),
						UseDatadogMetrics:  apiutils.NewBoolPointer(defaultDatadogMetricsEnabled),
						Port:               apiutils.NewInt32Pointer(defaultMetricsProviderPort),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewBoolPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewBoolPointer(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: apiutils.NewBoolPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewStringPointer(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewBoolPointer(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultHelmCheckEnabled),
					},
				},
			},
		},
		{
			name: "ClusterChecks feature with a field set, but \"enabled\" field not set",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						UseClusterChecksRunners: apiutils.NewBoolPointer(defaultUseClusterChecksRunners),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultGPUMonitoringEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
							Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           apiutils.NewBoolPointer(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: apiutils.NewBoolPointer(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
						ErrorTrackingStandalone: &v2alpha1.ErrorTrackingStandalone{
							Enabled: apiutils.NewBoolPointer(defaultAPMErrorTrackingStandalone),
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        apiutils.NewBoolPointer(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        apiutils.NewBoolPointer(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewBoolPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewBoolPointer(valueFalse),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: apiutils.NewBoolPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewStringPointer(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewBoolPointer(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultHelmCheckEnabled),
					},
				},
			},
		},
		{
			name: "Admission controller enabled unset, other fields set",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewBoolPointer(true),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewBoolPointer(true),
						},
						MutateUnlabelled:       apiutils.NewBoolPointer(true),
						AgentCommunicationMode: apiutils.NewStringPointer("socket"),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewBoolPointer(true),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
							Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           apiutils.NewBoolPointer(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: apiutils.NewBoolPointer(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
						ErrorTrackingStandalone: &v2alpha1.ErrorTrackingStandalone{
							Enabled: apiutils.NewBoolPointer(defaultAPMErrorTrackingStandalone),
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMIASTEnabled),
						},
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultGPUMonitoringEnabled),
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        apiutils.NewBoolPointer(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        apiutils.NewBoolPointer(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewBoolPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewBoolPointer(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueTrue),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled:       apiutils.NewBoolPointer(valueTrue),
						ServiceName:            apiutils.NewStringPointer(defaultAdmissionServiceName),
						AgentCommunicationMode: apiutils.NewStringPointer("socket"),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewBoolPointer(valueTrue),
							Mode:    apiutils.NewStringPointer(DefaultAdmissionControllerCWSInstrumentationMode),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultHelmCheckEnabled),
					},
				},
			},
		},
		{
			name: "Orchestrator explorer enabled unset, other fields set",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						CustomResources: []string{"datadoghq.com/v1alpha1/datadogmetrics"},
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultGPUMonitoringEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
							Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           apiutils.NewBoolPointer(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: apiutils.NewBoolPointer(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
						ErrorTrackingStandalone: &v2alpha1.ErrorTrackingStandalone{
							Enabled: apiutils.NewBoolPointer(defaultAPMErrorTrackingStandalone),
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        apiutils.NewBoolPointer(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        apiutils.NewBoolPointer(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
						CustomResources: []string{"datadoghq.com/v1alpha1/datadogmetrics"},
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewBoolPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewBoolPointer(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: apiutils.NewBoolPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewStringPointer(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewBoolPointer(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultHelmCheckEnabled),
					},
				},
			},
		},
		{
			name: "OTel Collector is enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueTrue),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultGPUMonitoringEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
							Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           apiutils.NewBoolPointer(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: apiutils.NewBoolPointer(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
						ErrorTrackingStandalone: &v2alpha1.ErrorTrackingStandalone{
							Enabled: apiutils.NewBoolPointer(defaultAPMErrorTrackingStandalone),
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueTrue),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        apiutils.NewBoolPointer(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        apiutils.NewBoolPointer(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewBoolPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewBoolPointer(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: apiutils.NewBoolPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewStringPointer(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewBoolPointer(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultHelmCheckEnabled),
					},
				},
			},
		},
		{
			// This test sets same defaults as the one with `Features: nil`; and leaves other configs as empty structs.
			name: "all feature configs are empty structs, configures defaults where applicable, leaves others empty",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection:           &v2alpha1.LogCollectionFeatureConfig{},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{},
					LiveProcessCollection:   &v2alpha1.LiveProcessCollectionFeatureConfig{},
					ProcessDiscovery:        &v2alpha1.ProcessDiscoveryFeatureConfig{},
					OOMKill:                 &v2alpha1.OOMKillFeatureConfig{},
					TCPQueueLength:          &v2alpha1.TCPQueueLengthFeatureConfig{},
					EBPFCheck:               &v2alpha1.EBPFCheckFeatureConfig{},
					GPU:                     &v2alpha1.GPUFeatureConfig{},
					ServiceDiscovery:        &v2alpha1.ServiceDiscoveryFeatureConfig{},
					APM:                     &v2alpha1.APMFeatureConfig{},
					ASM:                     &v2alpha1.ASMFeatureConfig{},
					CSPM:                    &v2alpha1.CSPMFeatureConfig{},
					CWS:                     &v2alpha1.CWSFeatureConfig{},
					NPM:                     &v2alpha1.NPMFeatureConfig{},
					USM:                     &v2alpha1.USMFeatureConfig{},
					OTLP:                    &v2alpha1.OTLPFeatureConfig{},
					RemoteConfiguration:     &v2alpha1.RemoteConfigurationFeatureConfig{},
					EventCollection:         &v2alpha1.EventCollectionFeatureConfig{},
					OrchestratorExplorer:    &v2alpha1.OrchestratorExplorerFeatureConfig{},
					KubeStateMetricsCore:    &v2alpha1.KubeStateMetricsCoreFeatureConfig{},
					AdmissionController:     &v2alpha1.AdmissionControllerFeatureConfig{},
					ExternalMetricsServer:   &v2alpha1.ExternalMetricsServerFeatureConfig{},
					ClusterChecks:           &v2alpha1.ClusterChecksFeatureConfig{},
					PrometheusScrape:        &v2alpha1.PrometheusScrapeFeatureConfig{},
					HelmCheck:               &v2alpha1.HelmCheckFeatureConfig{},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultGPUMonitoringEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
							Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           apiutils.NewBoolPointer(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: apiutils.NewBoolPointer(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
						ErrorTrackingStandalone: &v2alpha1.ErrorTrackingStandalone{
							Enabled: apiutils.NewBoolPointer(defaultAPMErrorTrackingStandalone),
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        apiutils.NewBoolPointer(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        apiutils.NewBoolPointer(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewBoolPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewBoolPointer(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: apiutils.NewBoolPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewStringPointer(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewBoolPointer(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultHelmCheckEnabled),
					},
				},
			},
		},
		{
			name: "CSPM and CWS are enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueTrue),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueTrue),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultGPUMonitoringEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
							Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           apiutils.NewBoolPointer(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: apiutils.NewBoolPointer(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
						ErrorTrackingStandalone: &v2alpha1.ErrorTrackingStandalone{
							Enabled: apiutils.NewBoolPointer(defaultAPMErrorTrackingStandalone),
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueTrue),
						HostBenchmarks: &v2alpha1.CSPMHostBenchmarksConfig{
							Enabled: apiutils.NewBoolPointer(defaultCSPMHostBenchmarksEnabled),
						},
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled:               apiutils.NewBoolPointer(valueTrue),
						SyscallMonitorEnabled: apiutils.NewBoolPointer(defaultCWSSyscallMonitorEnabled),
						Network: &v2alpha1.CWSNetworkConfig{
							Enabled: apiutils.NewBoolPointer(defaultCWSNetworkEnabled),
						},
						SecurityProfiles: &v2alpha1.CWSSecurityProfilesConfig{
							Enabled: apiutils.NewBoolPointer(defaultCWSSecurityProfilesEnabled),
						},
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        apiutils.NewBoolPointer(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        apiutils.NewBoolPointer(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					},
					},
					},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewBoolPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewBoolPointer(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: apiutils.NewBoolPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewStringPointer(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewBoolPointer(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultHelmCheckEnabled),
					},
				},
			},
		},
		{
			name: "Service Discovery is enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueTrue),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueTrue),
						NetworkStats: &v2alpha1.ServiceDiscoveryNetworkStatsConfig{
							Enabled: apiutils.NewBoolPointer(defaultServiceDiscoveryNetworkStatsEnabled),
						},
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultGPUMonitoringEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
							Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           apiutils.NewBoolPointer(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: apiutils.NewBoolPointer(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
						ErrorTrackingStandalone: &v2alpha1.ErrorTrackingStandalone{
							Enabled: apiutils.NewBoolPointer(defaultAPMErrorTrackingStandalone),
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewBoolPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: apiutils.NewBoolPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewBoolPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewStringPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        apiutils.NewBoolPointer(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewStringPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        apiutils.NewBoolPointer(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewStringPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewBoolPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewBoolPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewBoolPointer(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: apiutils.NewBoolPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewStringPointer(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewBoolPointer(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultHelmCheckEnabled),
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
