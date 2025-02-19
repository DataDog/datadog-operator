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
	"github.com/DataDog/datadog-operator/pkg/defaulting"

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
					Site:     apiutils.NewPointer(defaultSite),
					Registry: apiutils.NewPointer(defaulting.DefaultImageRegistry),
					LogLevel: apiutils.NewPointer(defaultLogLevel),
				},
			},
		},
		{
			name: "test registry defaulting based on site - EU",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					Site: apiutils.NewPointer(defaultEuropeSite),
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					Site:     apiutils.NewPointer(defaultEuropeSite),
					Registry: apiutils.NewPointer(defaulting.DefaultEuropeImageRegistry),
					LogLevel: apiutils.NewPointer(defaultLogLevel),
				},
			},
		},
		{
			name: "test registry defaulting based on site - Asia",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					Site: apiutils.NewPointer(defaultAsiaSite),
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					Site:     apiutils.NewPointer(defaultAsiaSite),
					Registry: apiutils.NewPointer(defaulting.DefaultAsiaImageRegistry),
					LogLevel: apiutils.NewPointer(defaultLogLevel),
				},
			},
		},
		{
			name: "test registry defaulting based on site - Azure",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					Site: apiutils.NewPointer(defaultAzureSite),
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					Site:     apiutils.NewPointer(defaultAzureSite),
					Registry: apiutils.NewPointer(defaulting.DefaultAzureImageRegistry),
					LogLevel: apiutils.NewPointer(defaultLogLevel),
				},
			},
		},
		{
			name: "test registry defaulting based on site - Gov",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					Site: apiutils.NewPointer(defaultGovSite),
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					Site:     apiutils.NewPointer(defaultGovSite),
					Registry: apiutils.NewPointer(defaulting.DefaultGovImageRegistry),
					LogLevel: apiutils.NewPointer(defaultLogLevel),
				},
			},
		},
		{
			name: "test FIPS defaulting - disabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					FIPS: &v2alpha1.FIPSConfig{
						Enabled: apiutils.NewPointer(defaultFIPSEnabled),
					},
					Site:     apiutils.NewPointer(defaultSite),
					Registry: apiutils.NewPointer(defaulting.DefaultImageRegistry),
					LogLevel: apiutils.NewPointer(defaultLogLevel),
				},
			},
		},
		{
			name: "test FIPS defaulting - enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					FIPS: &v2alpha1.FIPSConfig{
						Enabled: apiutils.NewPointer(true),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					FIPS: &v2alpha1.FIPSConfig{
						Enabled: apiutils.NewPointer(true),
						Image: &v2alpha1.AgentImageConfig{
							Name: defaultFIPSImageName,
							Tag:  defaultFIPSImageTag,
						},
						LocalAddress: apiutils.NewPointer(defaultFIPSLocalAddress),
						Port:         apiutils.NewPointer[int32](defaultFIPSPort),
						PortRange:    apiutils.NewPointer[int32](defaultFIPSPortRange),
						UseHTTPS:     apiutils.NewPointer(defaultFIPSUseHTTPS),
					},
					Site:     apiutils.NewPointer(defaultSite),
					Registry: apiutils.NewPointer(defaulting.DefaultImageRegistry),
					LogLevel: apiutils.NewPointer(defaultLogLevel),
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
						Enabled: apiutils.NewPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewPointer(defaultGPUMonitoringEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    apiutils.NewPointer[int32](defaultAPMHostPort),
							Enabled: apiutils.NewPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewPointer(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           apiutils.NewPointer(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: apiutils.NewPointer(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewPointer(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewPointer(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: apiutils.NewPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        apiutils.NewPointer(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        apiutils.NewPointer(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewPointer(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewPointer(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewPointer(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: apiutils.NewPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewPointer(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewPointer(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewPointer(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewPointer(defaultHelmCheckEnabled),
					},
				},
			},
		},
		{
			name: "all features are disabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewPointer(defaultGPUMonitoringEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewPointer(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewPointer(valueFalse),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewPointer(valueFalse),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewPointer(valueFalse),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{Enabled: apiutils.NewPointer(valueFalse)},
						HTTP: &v2alpha1.OTLPHTTPConfig{Enabled: apiutils.NewPointer(valueFalse)},
					}}},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewPointer(valueFalse),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled:    apiutils.NewPointer(valueFalse),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{Enabled: apiutils.NewPointer(valueFalse)},
						Mutation:   &v2alpha1.AdmissionControllerMutationConfig{Enabled: apiutils.NewPointer(valueFalse)},
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewPointer(defaultGPUMonitoringEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewPointer(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewPointer(valueFalse),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewPointer(valueFalse),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewPointer(valueFalse),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: apiutils.NewPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        apiutils.NewPointer(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        apiutils.NewPointer(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewPointer(valueFalse),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewPointer(valueFalse),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewPointer(valueFalse),
						},
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewPointer(valueFalse),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewPointer(valueFalse),
						},
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewPointer(valueFalse),
					},
				},
			},
		},
		{
			name: "liveProcess is enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(valueTrue),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(valueTrue),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewPointer(defaultGPUMonitoringEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    apiutils.NewPointer[int32](defaultAPMHostPort),
							Enabled: apiutils.NewPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewPointer(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           apiutils.NewPointer(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: apiutils.NewPointer(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewPointer(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewPointer(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: apiutils.NewPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        apiutils.NewPointer(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        apiutils.NewPointer(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewPointer(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewPointer(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewPointer(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: apiutils.NewPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewPointer(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewPointer(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewPointer(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewPointer(defaultHelmCheckEnabled),
					},
				},
			},
		},
		{
			name: "logCollection is enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(valueTrue),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled:                    apiutils.NewPointer(valueTrue),
						ContainerCollectUsingFiles: apiutils.NewPointer(defaultLogContainerCollectUsingFiles),
						ContainerLogsPath:          apiutils.NewPointer(defaultLogContainerLogsPath),
						PodLogsPath:                apiutils.NewPointer(defaultLogPodLogsPath),
						ContainerSymlinksPath:      apiutils.NewPointer(defaultLogContainerSymlinksPath),
						TempStoragePath:            apiutils.NewPointer(common.DefaultLogTempStoragePath),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewPointer(defaultGPUMonitoringEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    apiutils.NewPointer[int32](defaultAPMHostPort),
							Enabled: apiutils.NewPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewPointer(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           apiutils.NewPointer(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: apiutils.NewPointer(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewPointer(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewPointer(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: apiutils.NewPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        apiutils.NewPointer(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        apiutils.NewPointer(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewPointer(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewPointer(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewPointer(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: apiutils.NewPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewPointer(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewPointer(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewPointer(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewPointer(defaultHelmCheckEnabled),
					},
				},
			},
		},
		{
			name: "APM is enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewPointer(valueTrue),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewPointer(defaultGPUMonitoringEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewPointer(valueTrue),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    apiutils.NewPointer[int32](defaultAPMHostPort),
							Enabled: apiutils.NewPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewPointer(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           apiutils.NewPointer(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: apiutils.NewPointer(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewPointer(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewPointer(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: apiutils.NewPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        apiutils.NewPointer(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        apiutils.NewPointer(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewPointer(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewPointer(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewPointer(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: apiutils.NewPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewPointer(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewPointer(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewPointer(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewPointer(defaultHelmCheckEnabled),
					},
				},
			},
		},
		{
			name: "NPM is enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewPointer(valueTrue),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewPointer(defaultGPUMonitoringEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    apiutils.NewPointer[int32](defaultAPMHostPort),
							Enabled: apiutils.NewPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewPointer(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           apiutils.NewPointer(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: apiutils.NewPointer(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewPointer(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewPointer(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled:         apiutils.NewPointer(valueTrue),
						EnableConntrack: apiutils.NewPointer(defaultNPMEnableConntrack),
						CollectDNSStats: apiutils.NewPointer(defaultNPMCollectDNSStats),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: apiutils.NewPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        apiutils.NewPointer(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        apiutils.NewPointer(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewPointer(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewPointer(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewPointer(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: apiutils.NewPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewPointer(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewPointer(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewPointer(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewPointer(defaultHelmCheckEnabled),
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
							Enabled:  apiutils.NewPointer(true),
							Endpoint: apiutils.NewPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:  apiutils.NewPointer(true),
							Endpoint: apiutils.NewPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewPointer(defaultGPUMonitoringEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    apiutils.NewPointer[int32](defaultAPMHostPort),
							Enabled: apiutils.NewPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewPointer(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           apiutils.NewPointer(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: apiutils.NewPointer(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewPointer(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewPointer(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: apiutils.NewPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        apiutils.NewPointer(valueTrue),
							HostPortConfig: &v2alpha1.HostPortConfig{Enabled: apiutils.NewPointer(defaultOTLPGRPCHostPortEnabled)},
							Endpoint:       apiutils.NewPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        apiutils.NewPointer(valueTrue),
							HostPortConfig: &v2alpha1.HostPortConfig{Enabled: apiutils.NewPointer(defaultOTLPGRPCHostPortEnabled)},
							Endpoint:       apiutils.NewPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewPointer(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewPointer(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewPointer(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: apiutils.NewPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewPointer(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewPointer(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewPointer(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewPointer(defaultHelmCheckEnabled),
					},
				},
			},
		},
		{
			name: "ExternalMetricsServer is enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewPointer(valueTrue),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewPointer(defaultGPUMonitoringEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    apiutils.NewPointer[int32](defaultAPMHostPort),
							Enabled: apiutils.NewPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewPointer(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           apiutils.NewPointer(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: apiutils.NewPointer(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewPointer(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewPointer(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: apiutils.NewPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        apiutils.NewPointer(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        apiutils.NewPointer(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled:            apiutils.NewPointer(valueTrue),
						RegisterAPIService: apiutils.NewPointer(defaultRegisterAPIService),
						UseDatadogMetrics:  apiutils.NewPointer(defaultDatadogMetricsEnabled),
						Port:               apiutils.NewPointer[int32](defaultMetricsProviderPort),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewPointer(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewPointer(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: apiutils.NewPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewPointer(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewPointer(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewPointer(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewPointer(defaultHelmCheckEnabled),
					},
				},
			},
		},
		{
			name: "ClusterChecks feature with a field set, but \"enabled\" field not set",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						UseClusterChecksRunners: apiutils.NewPointer(defaultUseClusterChecksRunners),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewPointer(defaultGPUMonitoringEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    apiutils.NewPointer[int32](defaultAPMHostPort),
							Enabled: apiutils.NewPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewPointer(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           apiutils.NewPointer(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: apiutils.NewPointer(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewPointer(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewPointer(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: apiutils.NewPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        apiutils.NewPointer(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        apiutils.NewPointer(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewPointer(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewPointer(valueFalse),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewPointer(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: apiutils.NewPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewPointer(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewPointer(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewPointer(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewPointer(defaultHelmCheckEnabled),
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
							Enabled: apiutils.NewPointer(true),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewPointer(true),
						},
						MutateUnlabelled:       apiutils.NewPointer(true),
						AgentCommunicationMode: apiutils.NewPointer("socket"),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewPointer(true),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLiveContainerCollectionEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    apiutils.NewPointer[int32](defaultAPMHostPort),
							Enabled: apiutils.NewPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewPointer(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           apiutils.NewPointer(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: apiutils.NewPointer(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewPointer(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMIASTEnabled),
						},
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewPointer(defaultGPUMonitoringEnabled),
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewPointer(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: apiutils.NewPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        apiutils.NewPointer(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        apiutils.NewPointer(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewPointer(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewPointer(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewPointer(valueTrue),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled:       apiutils.NewPointer(valueTrue),
						ServiceName:            apiutils.NewPointer(defaultAdmissionServiceName),
						AgentCommunicationMode: apiutils.NewPointer("socket"),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewPointer(valueTrue),
							Mode:    apiutils.NewPointer(DefaultAdmissionControllerCWSInstrumentationMode),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewPointer(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewPointer(defaultHelmCheckEnabled),
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
						Enabled: apiutils.NewPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewPointer(defaultGPUMonitoringEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    apiutils.NewPointer[int32](defaultAPMHostPort),
							Enabled: apiutils.NewPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewPointer(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           apiutils.NewPointer(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: apiutils.NewPointer(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewPointer(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewPointer(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: apiutils.NewPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        apiutils.NewPointer(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        apiutils.NewPointer(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewPointer(defaultOrchestratorExplorerScrubContainers),
						CustomResources: []string{"datadoghq.com/v1alpha1/datadogmetrics"},
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewPointer(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewPointer(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewPointer(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: apiutils.NewPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewPointer(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewPointer(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewPointer(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewPointer(defaultHelmCheckEnabled),
					},
				},
			},
		},
		{
			name: "OTel Collector is enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewPointer(valueTrue),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewPointer(defaultGPUMonitoringEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    apiutils.NewPointer[int32](defaultAPMHostPort),
							Enabled: apiutils.NewPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewPointer(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           apiutils.NewPointer(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: apiutils.NewPointer(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewPointer(valueTrue),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewPointer(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: apiutils.NewPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        apiutils.NewPointer(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        apiutils.NewPointer(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewPointer(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewPointer(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewPointer(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: apiutils.NewPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewPointer(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewPointer(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewPointer(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewPointer(defaultHelmCheckEnabled),
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
						Enabled: apiutils.NewPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewPointer(defaultGPUMonitoringEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    apiutils.NewPointer[int32](defaultAPMHostPort),
							Enabled: apiutils.NewPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewPointer(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           apiutils.NewPointer(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: apiutils.NewPointer(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewPointer(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewPointer(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: apiutils.NewPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        apiutils.NewPointer(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        apiutils.NewPointer(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewPointer(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewPointer(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewPointer(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewPointer(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: apiutils.NewPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewPointer(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewPointer(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewPointer(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewPointer(defaultHelmCheckEnabled),
					},
				},
			},
		},
		{
			name: "CSPM and CWS are enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewPointer(valueTrue),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewPointer(valueTrue),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewPointer(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: apiutils.NewPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewPointer(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: apiutils.NewPointer(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: apiutils.NewPointer(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: apiutils.NewPointer(defaultGPUMonitoringEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    apiutils.NewPointer[int32](defaultAPMHostPort),
							Enabled: apiutils.NewPointer(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewPointer(defaultAPMSocketEnabled),
							Path:    apiutils.NewPointer(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           apiutils.NewPointer(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: apiutils.NewPointer(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: apiutils.NewPointer(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: apiutils.NewPointer(valueTrue),
						HostBenchmarks: &v2alpha1.CSPMHostBenchmarksConfig{
							Enabled: apiutils.NewPointer(defaultCSPMHostBenchmarksEnabled),
						},
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled:               apiutils.NewPointer(valueTrue),
						SyscallMonitorEnabled: apiutils.NewPointer(defaultCWSSyscallMonitorEnabled),
						Network: &v2alpha1.CWSNetworkConfig{
							Enabled: apiutils.NewPointer(defaultCWSNetworkEnabled),
						},
						SecurityProfiles: &v2alpha1.CWSSecurityProfilesConfig{
							Enabled: apiutils.NewPointer(defaultCWSSecurityProfilesEnabled),
						},
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: apiutils.NewPointer(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: apiutils.NewPointer(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: apiutils.NewPointer(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: apiutils.NewPointer(defaultDogstatsdSocketEnabled),
							Path:    apiutils.NewPointer(defaultDogstatsdHostSocketPath),
						},
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        apiutils.NewPointer(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewPointer(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        apiutils.NewPointer(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       apiutils.NewPointer(defaultOTLPHTTPEndpoint),
						},
					},
					},
					},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: apiutils.NewPointer(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         apiutils.NewPointer(defaultOrchestratorExplorerEnabled),
						ScrubContainers: apiutils.NewPointer(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewPointer(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewPointer(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewPointer(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewPointer(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: apiutils.NewPointer(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      apiutils.NewPointer(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: apiutils.NewPointer(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: apiutils.NewPointer(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewPointer(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: apiutils.NewPointer(defaultHelmCheckEnabled),
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
