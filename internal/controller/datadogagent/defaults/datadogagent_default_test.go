// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaults

import (
	"testing"

	"k8s.io/utils/ptr"

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
					Site:     ptr.To(defaultSite),
					Registry: ptr.To(images.DefaultImageRegistry),
					LogLevel: ptr.To(defaultLogLevel),
				},
			},
		},
		{
			name: "test registry defaulting based on site - EU",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					Site: ptr.To(defaultEuropeSite),
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					Site:     ptr.To(defaultEuropeSite),
					Registry: ptr.To(images.DefaultEuropeImageRegistry),
					LogLevel: ptr.To(defaultLogLevel),
				},
			},
		},
		{
			name: "test registry defaulting based on site - Asia",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					Site: ptr.To(defaultAsiaSite),
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					Site:     ptr.To(defaultAsiaSite),
					Registry: ptr.To(images.DefaultAsiaImageRegistry),
					LogLevel: ptr.To(defaultLogLevel),
				},
			},
		},
		{
			name: "test registry defaulting based on site - Azure",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					Site: ptr.To(defaultAzureSite),
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					Site:     ptr.To(defaultAzureSite),
					Registry: ptr.To(images.DefaultAzureImageRegistry),
					LogLevel: ptr.To(defaultLogLevel),
				},
			},
		},
		{
			name: "test registry defaulting based on site - Gov",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					Site: ptr.To(defaultGovSite),
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					Site:     ptr.To(defaultGovSite),
					Registry: ptr.To(images.DefaultGovImageRegistry),
					LogLevel: ptr.To(defaultLogLevel),
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
					UseFIPSAgent: ptr.To(defaultUseFIPSAgent),
					Site:         ptr.To(defaultSite),
					Registry:     ptr.To(images.DefaultImageRegistry),
					LogLevel:     ptr.To(defaultLogLevel),
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
						Enabled: ptr.To(defaultFIPSProxyEnabled),
					},
					Site:     ptr.To(defaultSite),
					Registry: ptr.To(images.DefaultImageRegistry),
					LogLevel: ptr.To(defaultLogLevel),
				},
			},
		},
		{
			name: "test FIPS defaulting - enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					FIPS: &v2alpha1.FIPSConfig{
						Enabled: ptr.To(true),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					FIPS: &v2alpha1.FIPSConfig{
						Enabled: ptr.To(true),
						Image: &v2alpha1.AgentImageConfig{
							Name: defaultFIPSImageName,
							Tag:  defaultFIPSImageTag,
						},
						LocalAddress: ptr.To(defaultFIPSLocalAddress),
						Port:         ptr.To(defaultFIPSPort),
						PortRange:    ptr.To(defaultFIPSPortRange),
						UseHTTPS:     ptr.To(defaultFIPSUseHTTPS),
					},
					Site:     ptr.To(defaultSite),
					Registry: ptr.To(images.DefaultImageRegistry),
					LogLevel: ptr.To(defaultLogLevel),
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
						Enabled: ptr.To(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: ptr.To(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: ptr.To(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: ptr.To(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: ptr.To(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: ptr.To(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: ptr.To(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: ptr.To(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: ptr.To(defaultGPUMonitoringEnabled),
					},
					DataPlane: &v2alpha1.DataPlaneFeatureConfig{
						Dogstatsd: &v2alpha1.DataPlaneDogstatsdConfig{Enabled: ptr.To(defaultDataPlaneDogstatsdEnabled)},
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: ptr.To(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    ptr.To(defaultAPMHostPort),
							Enabled: ptr.To(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: ptr.To(defaultAPMSocketEnabled),
							Path:    ptr.To(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           ptr.To(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
						ErrorTrackingStandalone: &v2alpha1.ErrorTrackingStandalone{
							Enabled: ptr.To(defaultAPMErrorTrackingStandalone),
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: ptr.To(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: ptr.To(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: ptr.To(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: ptr.To(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: ptr.To(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: ptr.To(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: ptr.To(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: ptr.To(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: ptr.To(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: ptr.To(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: ptr.To(defaultDogstatsdSocketEnabled),
							Path:    ptr.To(defaultDogstatsdHostSocketPath),
						},
						NonLocalTraffic: ptr.To(defaultDogstatsdNonLocalTraffic),
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        ptr.To(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       ptr.To(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        ptr.To(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       ptr.To(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: ptr.To(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: ptr.To(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         ptr.To(defaultOrchestratorExplorerEnabled),
						ScrubContainers: ptr.To(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: ptr.To(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: ptr.To(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 ptr.To(defaultClusterChecksEnabled),
						UseClusterChecksRunners: ptr.To(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: ptr.To(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: ptr.To(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: ptr.To(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: ptr.To(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      ptr.To(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: ptr.To(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: ptr.To(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: ptr.To(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: ptr.To(defaultHelmCheckEnabled),
					},
					ControlPlaneMonitoring: &v2alpha1.ControlPlaneMonitoringFeatureConfig{
						Enabled: ptr.To(defaultControlPlaneMonitoringEnabled),
					},
				},
			},
		},
		{
			name: "all features are disabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: ptr.To(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: ptr.To(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: ptr.To(defaultGPUMonitoringEnabled),
					},
					DataPlane: &v2alpha1.DataPlaneFeatureConfig{
						Dogstatsd: &v2alpha1.DataPlaneDogstatsdConfig{Enabled: ptr.To(defaultDataPlaneDogstatsdEnabled)},
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: ptr.To(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: ptr.To(valueFalse),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: ptr.To(valueFalse),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: ptr.To(valueFalse),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{Enabled: ptr.To(valueFalse)},
						HTTP: &v2alpha1.OTLPHTTPConfig{Enabled: ptr.To(valueFalse)},
					}}},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: ptr.To(valueFalse),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled:    ptr.To(valueFalse),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{Enabled: ptr.To(valueFalse)},
						Mutation:   &v2alpha1.AdmissionControllerMutationConfig{Enabled: ptr.To(valueFalse)},
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					ControlPlaneMonitoring: &v2alpha1.ControlPlaneMonitoringFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: ptr.To(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: ptr.To(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: ptr.To(defaultGPUMonitoringEnabled),
					},
					DataPlane: &v2alpha1.DataPlaneFeatureConfig{
						Dogstatsd: &v2alpha1.DataPlaneDogstatsdConfig{Enabled: ptr.To(defaultDataPlaneDogstatsdEnabled)},
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: ptr.To(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: ptr.To(valueFalse),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: ptr.To(valueFalse),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: ptr.To(valueFalse),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: ptr.To(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: ptr.To(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: ptr.To(defaultDogstatsdSocketEnabled),
							Path:    ptr.To(defaultDogstatsdHostSocketPath),
						},
						NonLocalTraffic: ptr.To(defaultDogstatsdNonLocalTraffic),
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        ptr.To(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       ptr.To(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        ptr.To(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       ptr.To(defaultOTLPHTTPEndpoint),
						},
					}}},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: ptr.To(valueFalse),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: ptr.To(valueFalse),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: ptr.To(valueFalse),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: ptr.To(valueFalse),
						},
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: ptr.To(valueFalse),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: ptr.To(valueFalse),
						},
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
					ControlPlaneMonitoring: &v2alpha1.ControlPlaneMonitoringFeatureConfig{
						Enabled: ptr.To(valueFalse),
					},
				},
			},
		},
		{
			name: "liveProcess is enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: ptr.To(valueTrue),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: ptr.To(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: ptr.To(valueTrue),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: ptr.To(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: ptr.To(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: ptr.To(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: ptr.To(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: ptr.To(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: ptr.To(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: ptr.To(defaultGPUMonitoringEnabled),
					},
					DataPlane: &v2alpha1.DataPlaneFeatureConfig{
						Dogstatsd: &v2alpha1.DataPlaneDogstatsdConfig{Enabled: ptr.To(defaultDataPlaneDogstatsdEnabled)},
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: ptr.To(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    ptr.To(defaultAPMHostPort),
							Enabled: ptr.To(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: ptr.To(defaultAPMSocketEnabled),
							Path:    ptr.To(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           ptr.To(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
						ErrorTrackingStandalone: &v2alpha1.ErrorTrackingStandalone{
							Enabled: ptr.To(defaultAPMErrorTrackingStandalone),
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: ptr.To(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: ptr.To(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: ptr.To(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: ptr.To(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: ptr.To(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: ptr.To(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: ptr.To(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: ptr.To(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: ptr.To(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: ptr.To(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: ptr.To(defaultDogstatsdSocketEnabled),
							Path:    ptr.To(defaultDogstatsdHostSocketPath),
						},
						NonLocalTraffic: ptr.To(defaultDogstatsdNonLocalTraffic),
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        ptr.To(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       ptr.To(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        ptr.To(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       ptr.To(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: ptr.To(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: ptr.To(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         ptr.To(defaultOrchestratorExplorerEnabled),
						ScrubContainers: ptr.To(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: ptr.To(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: ptr.To(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 ptr.To(defaultClusterChecksEnabled),
						UseClusterChecksRunners: ptr.To(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: ptr.To(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: ptr.To(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: ptr.To(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: ptr.To(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      ptr.To(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: ptr.To(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: ptr.To(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: ptr.To(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: ptr.To(defaultHelmCheckEnabled),
					},
					ControlPlaneMonitoring: &v2alpha1.ControlPlaneMonitoringFeatureConfig{
						Enabled: ptr.To(defaultControlPlaneMonitoringEnabled),
					},
				},
			},
		},
		{
			name: "logCollection is enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: ptr.To(valueTrue),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled:                    ptr.To(valueTrue),
						ContainerCollectUsingFiles: ptr.To(defaultLogContainerCollectUsingFiles),
						ContainerLogsPath:          ptr.To(defaultLogContainerLogsPath),
						PodLogsPath:                ptr.To(defaultLogPodLogsPath),
						ContainerSymlinksPath:      ptr.To(defaultLogContainerSymlinksPath),
						TempStoragePath:            ptr.To(common.DefaultLogTempStoragePath),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: ptr.To(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: ptr.To(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: ptr.To(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: ptr.To(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: ptr.To(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: ptr.To(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: ptr.To(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: ptr.To(defaultGPUMonitoringEnabled),
					},
					DataPlane: &v2alpha1.DataPlaneFeatureConfig{
						Dogstatsd: &v2alpha1.DataPlaneDogstatsdConfig{Enabled: ptr.To(defaultDataPlaneDogstatsdEnabled)},
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: ptr.To(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    ptr.To(defaultAPMHostPort),
							Enabled: ptr.To(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: ptr.To(defaultAPMSocketEnabled),
							Path:    ptr.To(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           ptr.To(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
						ErrorTrackingStandalone: &v2alpha1.ErrorTrackingStandalone{
							Enabled: ptr.To(defaultAPMErrorTrackingStandalone),
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: ptr.To(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: ptr.To(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: ptr.To(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: ptr.To(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: ptr.To(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: ptr.To(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: ptr.To(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: ptr.To(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: ptr.To(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: ptr.To(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: ptr.To(defaultDogstatsdSocketEnabled),
							Path:    ptr.To(defaultDogstatsdHostSocketPath),
						},
						NonLocalTraffic: ptr.To(defaultDogstatsdNonLocalTraffic),
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        ptr.To(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       ptr.To(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        ptr.To(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       ptr.To(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: ptr.To(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: ptr.To(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         ptr.To(defaultOrchestratorExplorerEnabled),
						ScrubContainers: ptr.To(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: ptr.To(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: ptr.To(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 ptr.To(defaultClusterChecksEnabled),
						UseClusterChecksRunners: ptr.To(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: ptr.To(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: ptr.To(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: ptr.To(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: ptr.To(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      ptr.To(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: ptr.To(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: ptr.To(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: ptr.To(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: ptr.To(defaultHelmCheckEnabled),
					},
					ControlPlaneMonitoring: &v2alpha1.ControlPlaneMonitoringFeatureConfig{
						Enabled: ptr.To(defaultControlPlaneMonitoringEnabled),
					},
				},
			},
		},
		{
			name: "APM is enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: ptr.To(valueTrue),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: ptr.To(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: ptr.To(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: ptr.To(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: ptr.To(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: ptr.To(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: ptr.To(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: ptr.To(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: ptr.To(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: ptr.To(defaultGPUMonitoringEnabled),
					},
					DataPlane: &v2alpha1.DataPlaneFeatureConfig{
						Dogstatsd: &v2alpha1.DataPlaneDogstatsdConfig{Enabled: ptr.To(defaultDataPlaneDogstatsdEnabled)},
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: ptr.To(valueTrue),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    ptr.To(defaultAPMHostPort),
							Enabled: ptr.To(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: ptr.To(defaultAPMSocketEnabled),
							Path:    ptr.To(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           ptr.To(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
						ErrorTrackingStandalone: &v2alpha1.ErrorTrackingStandalone{
							Enabled: ptr.To(defaultAPMErrorTrackingStandalone),
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: ptr.To(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: ptr.To(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: ptr.To(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: ptr.To(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: ptr.To(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: ptr.To(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: ptr.To(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: ptr.To(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: ptr.To(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: ptr.To(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: ptr.To(defaultDogstatsdSocketEnabled),
							Path:    ptr.To(defaultDogstatsdHostSocketPath),
						},
						NonLocalTraffic: ptr.To(defaultDogstatsdNonLocalTraffic),
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        ptr.To(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       ptr.To(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        ptr.To(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       ptr.To(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: ptr.To(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: ptr.To(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         ptr.To(defaultOrchestratorExplorerEnabled),
						ScrubContainers: ptr.To(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: ptr.To(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: ptr.To(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 ptr.To(defaultClusterChecksEnabled),
						UseClusterChecksRunners: ptr.To(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: ptr.To(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: ptr.To(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: ptr.To(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: ptr.To(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      ptr.To(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: ptr.To(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: ptr.To(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: ptr.To(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: ptr.To(defaultHelmCheckEnabled),
					},
					ControlPlaneMonitoring: &v2alpha1.ControlPlaneMonitoringFeatureConfig{
						Enabled: ptr.To(defaultControlPlaneMonitoringEnabled),
					},
				},
			},
		},
		{
			name: "NPM is enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: ptr.To(valueTrue),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: ptr.To(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: ptr.To(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: ptr.To(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: ptr.To(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: ptr.To(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: ptr.To(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: ptr.To(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: ptr.To(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: ptr.To(defaultGPUMonitoringEnabled),
					},
					DataPlane: &v2alpha1.DataPlaneFeatureConfig{
						Dogstatsd: &v2alpha1.DataPlaneDogstatsdConfig{Enabled: ptr.To(defaultDataPlaneDogstatsdEnabled)},
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: ptr.To(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    ptr.To(defaultAPMHostPort),
							Enabled: ptr.To(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: ptr.To(defaultAPMSocketEnabled),
							Path:    ptr.To(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           ptr.To(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
						ErrorTrackingStandalone: &v2alpha1.ErrorTrackingStandalone{
							Enabled: ptr.To(defaultAPMErrorTrackingStandalone),
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: ptr.To(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: ptr.To(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: ptr.To(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: ptr.To(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: ptr.To(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: ptr.To(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled:         ptr.To(valueTrue),
						EnableConntrack: ptr.To(defaultNPMEnableConntrack),
						CollectDNSStats: ptr.To(defaultNPMCollectDNSStats),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: ptr.To(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: ptr.To(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: ptr.To(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: ptr.To(defaultDogstatsdSocketEnabled),
							Path:    ptr.To(defaultDogstatsdHostSocketPath),
						},
						NonLocalTraffic: ptr.To(defaultDogstatsdNonLocalTraffic),
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        ptr.To(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       ptr.To(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        ptr.To(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       ptr.To(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: ptr.To(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: ptr.To(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         ptr.To(defaultOrchestratorExplorerEnabled),
						ScrubContainers: ptr.To(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: ptr.To(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: ptr.To(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 ptr.To(defaultClusterChecksEnabled),
						UseClusterChecksRunners: ptr.To(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: ptr.To(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: ptr.To(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: ptr.To(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: ptr.To(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      ptr.To(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: ptr.To(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: ptr.To(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: ptr.To(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: ptr.To(defaultHelmCheckEnabled),
					},
					ControlPlaneMonitoring: &v2alpha1.ControlPlaneMonitoringFeatureConfig{
						Enabled: ptr.To(defaultControlPlaneMonitoringEnabled),
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
							Enabled:  ptr.To(true),
							Endpoint: ptr.To(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:  ptr.To(true),
							Endpoint: ptr.To(defaultOTLPHTTPEndpoint),
						},
					}}},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: ptr.To(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: ptr.To(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: ptr.To(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: ptr.To(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: ptr.To(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: ptr.To(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: ptr.To(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: ptr.To(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: ptr.To(defaultGPUMonitoringEnabled),
					},
					DataPlane: &v2alpha1.DataPlaneFeatureConfig{
						Dogstatsd: &v2alpha1.DataPlaneDogstatsdConfig{Enabled: ptr.To(defaultDataPlaneDogstatsdEnabled)},
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: ptr.To(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    ptr.To(defaultAPMHostPort),
							Enabled: ptr.To(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: ptr.To(defaultAPMSocketEnabled),
							Path:    ptr.To(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           ptr.To(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
						ErrorTrackingStandalone: &v2alpha1.ErrorTrackingStandalone{
							Enabled: ptr.To(defaultAPMErrorTrackingStandalone),
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: ptr.To(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: ptr.To(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: ptr.To(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: ptr.To(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: ptr.To(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: ptr.To(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: ptr.To(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: ptr.To(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: ptr.To(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: ptr.To(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: ptr.To(defaultDogstatsdSocketEnabled),
							Path:    ptr.To(defaultDogstatsdHostSocketPath),
						},
						NonLocalTraffic: ptr.To(defaultDogstatsdNonLocalTraffic),
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        ptr.To(valueTrue),
							HostPortConfig: &v2alpha1.HostPortConfig{Enabled: ptr.To(defaultOTLPGRPCHostPortEnabled)},
							Endpoint:       ptr.To(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        ptr.To(valueTrue),
							HostPortConfig: &v2alpha1.HostPortConfig{Enabled: ptr.To(defaultOTLPGRPCHostPortEnabled)},
							Endpoint:       ptr.To(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: ptr.To(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: ptr.To(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         ptr.To(defaultOrchestratorExplorerEnabled),
						ScrubContainers: ptr.To(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: ptr.To(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: ptr.To(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 ptr.To(defaultClusterChecksEnabled),
						UseClusterChecksRunners: ptr.To(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: ptr.To(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: ptr.To(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: ptr.To(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: ptr.To(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      ptr.To(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: ptr.To(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: ptr.To(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: ptr.To(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: ptr.To(defaultHelmCheckEnabled),
					},
					ControlPlaneMonitoring: &v2alpha1.ControlPlaneMonitoringFeatureConfig{
						Enabled: ptr.To(defaultControlPlaneMonitoringEnabled),
					},
				},
			},
		},
		{
			name: "ExternalMetricsServer is enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: ptr.To(valueTrue),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: ptr.To(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: ptr.To(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: ptr.To(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: ptr.To(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: ptr.To(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: ptr.To(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: ptr.To(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: ptr.To(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: ptr.To(defaultGPUMonitoringEnabled),
					},
					DataPlane: &v2alpha1.DataPlaneFeatureConfig{
						Dogstatsd: &v2alpha1.DataPlaneDogstatsdConfig{Enabled: ptr.To(defaultDataPlaneDogstatsdEnabled)},
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: ptr.To(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    ptr.To(defaultAPMHostPort),
							Enabled: ptr.To(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: ptr.To(defaultAPMSocketEnabled),
							Path:    ptr.To(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           ptr.To(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
						ErrorTrackingStandalone: &v2alpha1.ErrorTrackingStandalone{
							Enabled: ptr.To(defaultAPMErrorTrackingStandalone),
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: ptr.To(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: ptr.To(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: ptr.To(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: ptr.To(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: ptr.To(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: ptr.To(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: ptr.To(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: ptr.To(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: ptr.To(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: ptr.To(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: ptr.To(defaultDogstatsdSocketEnabled),
							Path:    ptr.To(defaultDogstatsdHostSocketPath),
						},
						NonLocalTraffic: ptr.To(defaultDogstatsdNonLocalTraffic),
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        ptr.To(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       ptr.To(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        ptr.To(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       ptr.To(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: ptr.To(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: ptr.To(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         ptr.To(defaultOrchestratorExplorerEnabled),
						ScrubContainers: ptr.To(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled:            ptr.To(valueTrue),
						RegisterAPIService: ptr.To(defaultRegisterAPIService),
						UseDatadogMetrics:  ptr.To(defaultDatadogMetricsEnabled),
						Port:               ptr.To(defaultMetricsProviderPort),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: ptr.To(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 ptr.To(defaultClusterChecksEnabled),
						UseClusterChecksRunners: ptr.To(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: ptr.To(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: ptr.To(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: ptr.To(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: ptr.To(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      ptr.To(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: ptr.To(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: ptr.To(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: ptr.To(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: ptr.To(defaultHelmCheckEnabled),
					},
					ControlPlaneMonitoring: &v2alpha1.ControlPlaneMonitoringFeatureConfig{
						Enabled: ptr.To(defaultControlPlaneMonitoringEnabled),
					},
				},
			},
		},
		{
			name: "ClusterChecks feature with a field set, but \"enabled\" field not set",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						UseClusterChecksRunners: ptr.To(defaultUseClusterChecksRunners),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: ptr.To(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: ptr.To(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: ptr.To(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: ptr.To(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: ptr.To(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: ptr.To(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: ptr.To(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: ptr.To(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: ptr.To(defaultGPUMonitoringEnabled),
					},
					DataPlane: &v2alpha1.DataPlaneFeatureConfig{
						Dogstatsd: &v2alpha1.DataPlaneDogstatsdConfig{Enabled: ptr.To(defaultDataPlaneDogstatsdEnabled)},
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: ptr.To(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    ptr.To(defaultAPMHostPort),
							Enabled: ptr.To(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: ptr.To(defaultAPMSocketEnabled),
							Path:    ptr.To(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           ptr.To(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
						ErrorTrackingStandalone: &v2alpha1.ErrorTrackingStandalone{
							Enabled: ptr.To(defaultAPMErrorTrackingStandalone),
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: ptr.To(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: ptr.To(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: ptr.To(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: ptr.To(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: ptr.To(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: ptr.To(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: ptr.To(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: ptr.To(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: ptr.To(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: ptr.To(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: ptr.To(defaultDogstatsdSocketEnabled),
							Path:    ptr.To(defaultDogstatsdHostSocketPath),
						},
						NonLocalTraffic: ptr.To(defaultDogstatsdNonLocalTraffic),
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        ptr.To(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       ptr.To(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        ptr.To(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       ptr.To(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: ptr.To(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: ptr.To(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         ptr.To(defaultOrchestratorExplorerEnabled),
						ScrubContainers: ptr.To(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: ptr.To(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: ptr.To(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 ptr.To(defaultClusterChecksEnabled),
						UseClusterChecksRunners: ptr.To(valueFalse),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: ptr.To(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: ptr.To(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: ptr.To(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: ptr.To(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      ptr.To(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: ptr.To(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: ptr.To(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: ptr.To(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: ptr.To(defaultHelmCheckEnabled),
					},
					ControlPlaneMonitoring: &v2alpha1.ControlPlaneMonitoringFeatureConfig{
						Enabled: ptr.To(defaultControlPlaneMonitoringEnabled),
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
							Enabled: ptr.To(true),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: ptr.To(true),
						},
						MutateUnlabelled:       ptr.To(true),
						AgentCommunicationMode: ptr.To("socket"),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: ptr.To(true),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: ptr.To(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: ptr.To(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: ptr.To(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: ptr.To(defaultLiveContainerCollectionEnabled),
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: ptr.To(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    ptr.To(defaultAPMHostPort),
							Enabled: ptr.To(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: ptr.To(defaultAPMSocketEnabled),
							Path:    ptr.To(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           ptr.To(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
						ErrorTrackingStandalone: &v2alpha1.ErrorTrackingStandalone{
							Enabled: ptr.To(defaultAPMErrorTrackingStandalone),
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: ptr.To(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: ptr.To(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: ptr.To(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: ptr.To(defaultAdmissionASMIASTEnabled),
						},
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: ptr.To(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: ptr.To(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: ptr.To(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: ptr.To(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: ptr.To(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: ptr.To(defaultGPUMonitoringEnabled),
					},
					DataPlane: &v2alpha1.DataPlaneFeatureConfig{
						Dogstatsd: &v2alpha1.DataPlaneDogstatsdConfig{Enabled: ptr.To(defaultDataPlaneDogstatsdEnabled)},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: ptr.To(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: ptr.To(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: ptr.To(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: ptr.To(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: ptr.To(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: ptr.To(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: ptr.To(defaultDogstatsdSocketEnabled),
							Path:    ptr.To(defaultDogstatsdHostSocketPath),
						},
						NonLocalTraffic: ptr.To(defaultDogstatsdNonLocalTraffic),
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        ptr.To(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       ptr.To(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        ptr.To(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       ptr.To(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: ptr.To(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: ptr.To(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         ptr.To(defaultOrchestratorExplorerEnabled),
						ScrubContainers: ptr.To(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: ptr.To(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: ptr.To(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 ptr.To(defaultClusterChecksEnabled),
						UseClusterChecksRunners: ptr.To(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: ptr.To(valueTrue),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: ptr.To(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: ptr.To(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled:       ptr.To(valueTrue),
						ServiceName:            ptr.To(defaultAdmissionServiceName),
						AgentCommunicationMode: ptr.To("socket"),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: ptr.To(valueTrue),
							Mode:    ptr.To(DefaultAdmissionControllerCWSInstrumentationMode),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: ptr.To(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: ptr.To(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: ptr.To(defaultHelmCheckEnabled),
					},
					ControlPlaneMonitoring: &v2alpha1.ControlPlaneMonitoringFeatureConfig{
						Enabled: ptr.To(defaultControlPlaneMonitoringEnabled),
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
						Enabled: ptr.To(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: ptr.To(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: ptr.To(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: ptr.To(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: ptr.To(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: ptr.To(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: ptr.To(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: ptr.To(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: ptr.To(defaultGPUMonitoringEnabled),
					},
					DataPlane: &v2alpha1.DataPlaneFeatureConfig{
						Dogstatsd: &v2alpha1.DataPlaneDogstatsdConfig{Enabled: ptr.To(defaultDataPlaneDogstatsdEnabled)},
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: ptr.To(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    ptr.To(defaultAPMHostPort),
							Enabled: ptr.To(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: ptr.To(defaultAPMSocketEnabled),
							Path:    ptr.To(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           ptr.To(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
						ErrorTrackingStandalone: &v2alpha1.ErrorTrackingStandalone{
							Enabled: ptr.To(defaultAPMErrorTrackingStandalone),
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: ptr.To(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: ptr.To(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: ptr.To(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: ptr.To(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: ptr.To(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: ptr.To(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: ptr.To(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: ptr.To(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: ptr.To(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: ptr.To(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: ptr.To(defaultDogstatsdSocketEnabled),
							Path:    ptr.To(defaultDogstatsdHostSocketPath),
						},
						NonLocalTraffic: ptr.To(defaultDogstatsdNonLocalTraffic),
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        ptr.To(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       ptr.To(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        ptr.To(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       ptr.To(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: ptr.To(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: ptr.To(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         ptr.To(defaultOrchestratorExplorerEnabled),
						ScrubContainers: ptr.To(defaultOrchestratorExplorerScrubContainers),
						CustomResources: []string{"datadoghq.com/v1alpha1/datadogmetrics"},
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: ptr.To(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: ptr.To(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 ptr.To(defaultClusterChecksEnabled),
						UseClusterChecksRunners: ptr.To(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: ptr.To(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: ptr.To(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: ptr.To(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: ptr.To(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      ptr.To(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: ptr.To(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: ptr.To(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: ptr.To(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: ptr.To(defaultHelmCheckEnabled),
					},
					ControlPlaneMonitoring: &v2alpha1.ControlPlaneMonitoringFeatureConfig{
						Enabled: ptr.To(defaultControlPlaneMonitoringEnabled),
					},
				},
			},
		},
		{
			name: "OTel Collector is enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: ptr.To(valueTrue),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: ptr.To(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: ptr.To(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: ptr.To(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: ptr.To(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: ptr.To(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: ptr.To(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: ptr.To(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: ptr.To(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: ptr.To(defaultGPUMonitoringEnabled),
					},
					DataPlane: &v2alpha1.DataPlaneFeatureConfig{
						Dogstatsd: &v2alpha1.DataPlaneDogstatsdConfig{Enabled: ptr.To(defaultDataPlaneDogstatsdEnabled)},
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: ptr.To(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    ptr.To(defaultAPMHostPort),
							Enabled: ptr.To(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: ptr.To(defaultAPMSocketEnabled),
							Path:    ptr.To(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           ptr.To(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
						ErrorTrackingStandalone: &v2alpha1.ErrorTrackingStandalone{
							Enabled: ptr.To(defaultAPMErrorTrackingStandalone),
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: ptr.To(valueTrue),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: ptr.To(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: ptr.To(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: ptr.To(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: ptr.To(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: ptr.To(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: ptr.To(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: ptr.To(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: ptr.To(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: ptr.To(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: ptr.To(defaultDogstatsdSocketEnabled),
							Path:    ptr.To(defaultDogstatsdHostSocketPath),
						},
						NonLocalTraffic: ptr.To(defaultDogstatsdNonLocalTraffic),
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        ptr.To(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       ptr.To(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        ptr.To(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       ptr.To(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: ptr.To(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: ptr.To(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         ptr.To(defaultOrchestratorExplorerEnabled),
						ScrubContainers: ptr.To(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: ptr.To(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: ptr.To(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 ptr.To(defaultClusterChecksEnabled),
						UseClusterChecksRunners: ptr.To(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: ptr.To(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: ptr.To(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: ptr.To(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: ptr.To(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      ptr.To(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: ptr.To(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: ptr.To(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: ptr.To(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: ptr.To(defaultHelmCheckEnabled),
					},
					ControlPlaneMonitoring: &v2alpha1.ControlPlaneMonitoringFeatureConfig{
						Enabled: ptr.To(defaultControlPlaneMonitoringEnabled),
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
					ControlPlaneMonitoring:  &v2alpha1.ControlPlaneMonitoringFeatureConfig{},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: ptr.To(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: ptr.To(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: ptr.To(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: ptr.To(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: ptr.To(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: ptr.To(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: ptr.To(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: ptr.To(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: ptr.To(defaultGPUMonitoringEnabled),
					},
					DataPlane: &v2alpha1.DataPlaneFeatureConfig{
						Dogstatsd: &v2alpha1.DataPlaneDogstatsdConfig{Enabled: ptr.To(defaultDataPlaneDogstatsdEnabled)},
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: ptr.To(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    ptr.To(defaultAPMHostPort),
							Enabled: ptr.To(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: ptr.To(defaultAPMSocketEnabled),
							Path:    ptr.To(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           ptr.To(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
						ErrorTrackingStandalone: &v2alpha1.ErrorTrackingStandalone{
							Enabled: ptr.To(defaultAPMErrorTrackingStandalone),
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: ptr.To(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: ptr.To(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: ptr.To(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: ptr.To(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: ptr.To(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: ptr.To(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: ptr.To(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: ptr.To(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: ptr.To(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: ptr.To(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: ptr.To(defaultDogstatsdSocketEnabled),
							Path:    ptr.To(defaultDogstatsdHostSocketPath),
						},
						NonLocalTraffic: ptr.To(defaultDogstatsdNonLocalTraffic),
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        ptr.To(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       ptr.To(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        ptr.To(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       ptr.To(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: ptr.To(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: ptr.To(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         ptr.To(defaultOrchestratorExplorerEnabled),
						ScrubContainers: ptr.To(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: ptr.To(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: ptr.To(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 ptr.To(defaultClusterChecksEnabled),
						UseClusterChecksRunners: ptr.To(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: ptr.To(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: ptr.To(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: ptr.To(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: ptr.To(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      ptr.To(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: ptr.To(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: ptr.To(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: ptr.To(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: ptr.To(defaultHelmCheckEnabled),
					},
					ControlPlaneMonitoring: &v2alpha1.ControlPlaneMonitoringFeatureConfig{
						Enabled: ptr.To(defaultControlPlaneMonitoringEnabled),
					},
				},
			},
		},
		{
			name: "CSPM and CWS are enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: ptr.To(valueTrue),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: ptr.To(valueTrue),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: ptr.To(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: ptr.To(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: ptr.To(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: ptr.To(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: ptr.To(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: ptr.To(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: ptr.To(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: ptr.To(defaultServiceDiscoveryEnabled),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: ptr.To(defaultGPUMonitoringEnabled),
					},
					DataPlane: &v2alpha1.DataPlaneFeatureConfig{
						Dogstatsd: &v2alpha1.DataPlaneDogstatsdConfig{Enabled: ptr.To(defaultDataPlaneDogstatsdEnabled)},
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: ptr.To(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    ptr.To(defaultAPMHostPort),
							Enabled: ptr.To(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: ptr.To(defaultAPMSocketEnabled),
							Path:    ptr.To(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           ptr.To(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
						ErrorTrackingStandalone: &v2alpha1.ErrorTrackingStandalone{
							Enabled: ptr.To(defaultAPMErrorTrackingStandalone),
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: ptr.To(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: ptr.To(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: ptr.To(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: ptr.To(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: ptr.To(valueTrue),
						HostBenchmarks: &v2alpha1.CSPMHostBenchmarksConfig{
							Enabled: ptr.To(defaultCSPMHostBenchmarksEnabled),
						},
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled:                   ptr.To(valueTrue),
						SyscallMonitorEnabled:     ptr.To(defaultCWSSyscallMonitorEnabled),
						DirectSendFromSystemProbe: ptr.To(defaultCWSDirectSendFromSystemProbe),
						Network: &v2alpha1.CWSNetworkConfig{
							Enabled: ptr.To(defaultCWSNetworkEnabled),
						},
						SecurityProfiles: &v2alpha1.CWSSecurityProfilesConfig{
							Enabled: ptr.To(defaultCWSSecurityProfilesEnabled),
						},
						Enforcement: &v2alpha1.CWSEnforcementConfig{
							Enabled: ptr.To(defaultCWSEnforcementEnabled),
						},
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: ptr.To(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: ptr.To(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: ptr.To(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: ptr.To(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: ptr.To(defaultDogstatsdSocketEnabled),
							Path:    ptr.To(defaultDogstatsdHostSocketPath),
						},
						NonLocalTraffic: ptr.To(defaultDogstatsdNonLocalTraffic),
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        ptr.To(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       ptr.To(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        ptr.To(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       ptr.To(defaultOTLPHTTPEndpoint),
						},
					},
					},
					},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: ptr.To(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: ptr.To(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         ptr.To(defaultOrchestratorExplorerEnabled),
						ScrubContainers: ptr.To(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: ptr.To(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: ptr.To(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 ptr.To(defaultClusterChecksEnabled),
						UseClusterChecksRunners: ptr.To(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: ptr.To(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: ptr.To(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: ptr.To(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: ptr.To(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      ptr.To(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: ptr.To(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: ptr.To(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: ptr.To(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: ptr.To(defaultHelmCheckEnabled),
					},
					ControlPlaneMonitoring: &v2alpha1.ControlPlaneMonitoringFeatureConfig{
						Enabled: ptr.To(defaultControlPlaneMonitoringEnabled),
					},
				},
			},
		},
		{
			name: "Service Discovery is enabled",
			ddaSpec: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: ptr.To(valueTrue),
					},
				},
			},
			want: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					LogCollection: &v2alpha1.LogCollectionFeatureConfig{
						Enabled: ptr.To(defaultLogCollectionEnabled),
					},
					LiveProcessCollection: &v2alpha1.LiveProcessCollectionFeatureConfig{
						Enabled: ptr.To(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &v2alpha1.LiveContainerCollectionFeatureConfig{
						Enabled: ptr.To(defaultLiveContainerCollectionEnabled),
					},
					ProcessDiscovery: &v2alpha1.ProcessDiscoveryFeatureConfig{
						Enabled: ptr.To(defaultProcessDiscoveryEnabled),
					},
					OOMKill: &v2alpha1.OOMKillFeatureConfig{
						Enabled: ptr.To(defaultOOMKillEnabled),
					},
					TCPQueueLength: &v2alpha1.TCPQueueLengthFeatureConfig{
						Enabled: ptr.To(defaultTCPQueueLengthEnabled),
					},
					EBPFCheck: &v2alpha1.EBPFCheckFeatureConfig{
						Enabled: ptr.To(defaultEBPFCheckEnabled),
					},
					ServiceDiscovery: &v2alpha1.ServiceDiscoveryFeatureConfig{
						Enabled: ptr.To(valueTrue),
					},
					GPU: &v2alpha1.GPUFeatureConfig{
						Enabled: ptr.To(defaultGPUMonitoringEnabled),
					},
					DataPlane: &v2alpha1.DataPlaneFeatureConfig{
						Dogstatsd: &v2alpha1.DataPlaneDogstatsdConfig{Enabled: ptr.To(defaultDataPlaneDogstatsdEnabled)},
					},
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: ptr.To(defaultAPMEnabled),
						HostPortConfig: &v2alpha1.HostPortConfig{
							Port:    ptr.To(defaultAPMHostPort),
							Enabled: ptr.To(defaultAPMHostPortEnabled),
						},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: ptr.To(defaultAPMSocketEnabled),
							Path:    ptr.To(defaultAPMSocketHostPath),
						},
						SingleStepInstrumentation: &v2alpha1.SingleStepInstrumentation{
							Enabled:           ptr.To(defaultAPMSingleStepInstrEnabled),
							LanguageDetection: &v2alpha1.LanguageDetectionConfig{Enabled: ptr.To(defaultLanguageDetectionEnabled)},
							Injector:          &v2alpha1.InjectorConfig{},
						},
						ErrorTrackingStandalone: &v2alpha1.ErrorTrackingStandalone{
							Enabled: ptr.To(defaultAPMErrorTrackingStandalone),
						},
					},
					OtelCollector: &v2alpha1.OtelCollectorFeatureConfig{
						Enabled: ptr.To(defaultOtelCollectorEnabled),
					},
					ASM: &v2alpha1.ASMFeatureConfig{
						Threats: &v2alpha1.ASMThreatsConfig{
							Enabled: ptr.To(defaultAdmissionASMThreatsEnabled),
						},
						SCA: &v2alpha1.ASMSCAConfig{
							Enabled: ptr.To(defaultAdmissionASMSCAEnabled),
						},
						IAST: &v2alpha1.ASMIASTConfig{
							Enabled: ptr.To(defaultAdmissionASMIASTEnabled),
						},
					},
					CSPM: &v2alpha1.CSPMFeatureConfig{
						Enabled: ptr.To(defaultCSPMEnabled),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: ptr.To(defaultCWSEnabled),
					},
					NPM: &v2alpha1.NPMFeatureConfig{
						Enabled: ptr.To(defaultNPMEnabled),
					},
					USM: &v2alpha1.USMFeatureConfig{
						Enabled: ptr.To(defaultUSMEnabled),
					},
					Dogstatsd: &v2alpha1.DogstatsdFeatureConfig{
						OriginDetectionEnabled: ptr.To(defaultDogstatsdOriginDetectionEnabled),
						HostPortConfig:         &v2alpha1.HostPortConfig{Enabled: ptr.To(defaultDogstatsdHostPortEnabled)},
						UnixDomainSocketConfig: &v2alpha1.UnixDomainSocketConfig{
							Enabled: ptr.To(defaultDogstatsdSocketEnabled),
							Path:    ptr.To(defaultDogstatsdHostSocketPath),
						},
						NonLocalTraffic: ptr.To(defaultDogstatsdNonLocalTraffic),
					},
					OTLP: &v2alpha1.OTLPFeatureConfig{Receiver: v2alpha1.OTLPReceiverConfig{Protocols: v2alpha1.OTLPProtocolsConfig{
						GRPC: &v2alpha1.OTLPGRPCConfig{
							Enabled:        ptr.To(defaultOTLPGRPCEnabled),
							HostPortConfig: nil,
							Endpoint:       ptr.To(defaultOTLPGRPCEndpoint),
						},
						HTTP: &v2alpha1.OTLPHTTPConfig{
							Enabled:        ptr.To(defaultOTLPHTTPEnabled),
							HostPortConfig: nil,
							Endpoint:       ptr.To(defaultOTLPHTTPEndpoint),
						},
					}}},
					RemoteConfiguration: &v2alpha1.RemoteConfigurationFeatureConfig{
						Enabled: ptr.To(defaultRemoteConfigurationEnabled),
					},
					EventCollection: &v2alpha1.EventCollectionFeatureConfig{
						CollectKubernetesEvents: ptr.To(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &v2alpha1.OrchestratorExplorerFeatureConfig{
						Enabled:         ptr.To(defaultOrchestratorExplorerEnabled),
						ScrubContainers: ptr.To(defaultOrchestratorExplorerScrubContainers),
					},
					ExternalMetricsServer: &v2alpha1.ExternalMetricsServerFeatureConfig{
						Enabled: ptr.To(defaultExternalMetricsServerEnabled),
					},
					KubeStateMetricsCore: &v2alpha1.KubeStateMetricsCoreFeatureConfig{
						Enabled: ptr.To(defaultKubeStateMetricsCoreEnabled),
					},
					ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
						Enabled:                 ptr.To(defaultClusterChecksEnabled),
						UseClusterChecksRunners: ptr.To(defaultUseClusterChecksRunners),
					},
					AdmissionController: &v2alpha1.AdmissionControllerFeatureConfig{
						Enabled: ptr.To(defaultAdmissionControllerEnabled),
						Validation: &v2alpha1.AdmissionControllerValidationConfig{
							Enabled: ptr.To(defaultAdmissionControllerValidationEnabled),
						},
						Mutation: &v2alpha1.AdmissionControllerMutationConfig{
							Enabled: ptr.To(defaultAdmissionControllerMutationEnabled),
						},
						MutateUnlabelled: ptr.To(defaultAdmissionControllerMutateUnlabelled),
						ServiceName:      ptr.To(defaultAdmissionServiceName),
						CWSInstrumentation: &v2alpha1.CWSInstrumentationConfig{
							Enabled: ptr.To(DefaultAdmissionControllerCWSInstrumentationEnabled),
						},
						KubernetesAdmissionEvents: &v2alpha1.KubernetesAdmissionEventsConfig{
							Enabled: ptr.To(defaultAdmissionControllerKubernetesAdmissionEventsEnabled),
						},
					},
					PrometheusScrape: &v2alpha1.PrometheusScrapeFeatureConfig{
						Enabled: ptr.To(defaultPrometheusScrapeEnabled),
					},
					HelmCheck: &v2alpha1.HelmCheckFeatureConfig{
						Enabled: ptr.To(defaultHelmCheckEnabled),
					},
					ControlPlaneMonitoring: &v2alpha1.ControlPlaneMonitoringFeatureConfig{
						Enabled: ptr.To(defaultControlPlaneMonitoringEnabled),
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
