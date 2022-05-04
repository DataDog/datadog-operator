// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	apiutils "github.com/DataDog/datadog-operator/apis/utils"
)

const (
	valueFalse = false
	valueTrue  = true
)

func Test_defaultCredentials(t *testing.T) {
	notEmptyString := "notEmpty"

	clusterAgentEnabled := make(map[ComponentName]*DatadogAgentComponentOverride)
	clusterAgentEnabled[ClusterAgentComponentName] = &DatadogAgentComponentOverride{
		Disabled: apiutils.NewBoolPointer(valueFalse),
	}

	clusterAgentDisabled := make(map[ComponentName]*DatadogAgentComponentOverride)
	clusterAgentDisabled[ClusterAgentComponentName] = &DatadogAgentComponentOverride{
		Disabled: apiutils.NewBoolPointer(valueTrue),
	}

	tests := []struct {
		name           string
		ddaSpec        *DatadogAgentSpec
		wantEmptyToken bool
	}{
		{
			name: "test cluster agent is enabled (by default) but no token set",
			ddaSpec: &DatadogAgentSpec{
				Global: &GlobalConfig{
					Credentials: &DatadogCredentials{},
				},
			},
			wantEmptyToken: false,
		},
		{
			name: "test cluster agent is enabled (explicitly) but no token set",
			ddaSpec: &DatadogAgentSpec{
				Global: &GlobalConfig{
					Credentials: &DatadogCredentials{},
				},
				Override: clusterAgentEnabled,
			},
			wantEmptyToken: false,
		},
		{
			name: "test cluster agent is enabled (explicitly), and token is set",
			ddaSpec: &DatadogAgentSpec{
				Global: &GlobalConfig{
					Credentials: &DatadogCredentials{
						Token: apiutils.NewStringPointer(notEmptyString),
					},
				},
				Override: clusterAgentEnabled,
			},
			wantEmptyToken: false,
		},
		{
			name: "test cluster agent is disabled",
			ddaSpec: &DatadogAgentSpec{
				Global: &GlobalConfig{
					Credentials: &DatadogCredentials{},
				},
				Override: clusterAgentDisabled,
			},
			wantEmptyToken: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defaultCredentials(tt.ddaSpec)
			if tt.wantEmptyToken {
				assert.Empty(t, tt.ddaSpec.Global.Credentials.Token)
			} else {
				assert.NotEmpty(t, tt.ddaSpec.Global.Credentials.Token)
			}
		})
	}
}

func Test_defaultFeatures(t *testing.T) {
	// falsePointer := apiutils.NewBoolPointer(valueFalse)
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
						// These are not set because the default is Enabled = false
						// ContainerCollectUsingFiles: apiutils.NewBoolPointer(defaultLogContainerCollectUsingFiles),
						// ContainerLogsPath:          apiutils.NewStringPointer(defaultLogContainerLogsPath),
						// PodLogsPath:                apiutils.NewStringPointer(defaultLogPodLogsPath),
						// ContainerSymlinksPath:      apiutils.NewStringPointer(defaultLogContainerSymlinksPath),
						// TempStoragePath:            apiutils.NewStringPointer(defaultLogTempStoragePath),
					},
					LiveProcessCollection: &LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					OOMKill: &OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					APM: &APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAPMEnabled),
						// These are not set because the default is Enabled = false
						// HostPortConfig: &HostPortConfig{
						// 	Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						// 	Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
						// },
						// UnixDomainSocketConfig: &UnixDomainSocketConfig{
						// 	Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
						// 	Path:    apiutils.NewStringPointer(defaultAPMSocketPath),
						// },
					},
					CSPM: &CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCSPMEnabled),
					},
					CWS: &CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCWSEnabled),
					},
					NPM: &NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultNPMEnabled),
						// These are not set because the default is Enabled = false
						// EnableConntrack: apiutils.NewBoolPointer(defaultNPMEnableConntrack),
						// CollectDNSStats: apiutils.NewBoolPointer(defaultNPMCollectDNSStats),
					},
					USM: &USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
					},
					EventCollection: &EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &OrchestratorExplorerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						Conf: &CustomConfig{
							ConfigData: apiutils.NewStringPointer(DefaultOrchestratorExplorerConf),
						},
					},
					KubeStateMetricsCore: &KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
						// This is not set because the default is Enabled = false
						// Conf: &CustomConfig{
						// 	ConfigData: apiutils.NewStringPointer(defaultKubeStateMetricsCoreConf),
						// },
					},
					AdmissionController: &AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
					},
					ExternalMetricsServer: &ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultExternalMetricsServerEnabled),
						// These are not set because the default is Enabled = false
						// UseDatadogMetrics: apiutils.NewBoolPointer(defaultDatadogMetricsEnabled),
						// Port:              apiutils.NewInt32Pointer(defaultMetricsProviderPort),
					},
					ClusterChecks: &ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewBoolPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewBoolPointer(defaultUseClusterChecksRunners),
					},
					PrometheusScrape: &PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					DatadogMonitor: &DatadogMonitorFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultDatadogMonitorEnabled),
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
					LiveProcessCollection: &LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					OOMKill: &OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					APM: &APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAPMEnabled),
						// These are not set because the default is Enabled = false
						// HostPortConfig: &HostPortConfig{
						// 	Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						// 	Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
						// },
						// UnixDomainSocketConfig: &UnixDomainSocketConfig{
						// 	Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
						// 	Path:    apiutils.NewStringPointer(defaultAPMSocketPath),
						// },
					},
					CSPM: &CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCSPMEnabled),
					},
					CWS: &CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCWSEnabled),
					},
					NPM: &NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultNPMEnabled),
						// These are not set because the default is Enabled = false
						// EnableConntrack: apiutils.NewBoolPointer(defaultNPMEnableConntrack),
						// CollectDNSStats: apiutils.NewBoolPointer(defaultNPMCollectDNSStats),
					},
					USM: &USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
					},
					EventCollection: &EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &OrchestratorExplorerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						Conf: &CustomConfig{
							ConfigData: apiutils.NewStringPointer(DefaultOrchestratorExplorerConf),
						},
					},
					KubeStateMetricsCore: &KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
						// This is not set because the default is Enabled = false
						// Conf: &CustomConfig{
						// 	ConfigData: apiutils.NewStringPointer(defaultKubeStateMetricsCoreConf),
						// },
					},
					AdmissionController: &AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
					},
					ExternalMetricsServer: &ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultExternalMetricsServerEnabled),
						// These are not set because the default is Enabled = false
						// UseDatadogMetrics: apiutils.NewBoolPointer(defaultDatadogMetricsEnabled),
						// Port:              apiutils.NewInt32Pointer(defaultMetricsProviderPort),
					},
					ClusterChecks: &ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewBoolPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewBoolPointer(defaultUseClusterChecksRunners),
					},
					PrometheusScrape: &PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					DatadogMonitor: &DatadogMonitorFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultDatadogMonitorEnabled),
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
						// These are not set because the default is Enabled = false
						// ContainerCollectUsingFiles: apiutils.NewBoolPointer(defaultLogContainerCollectUsingFiles),
						// ContainerLogsPath:          apiutils.NewStringPointer(defaultLogContainerLogsPath),
						// PodLogsPath:                apiutils.NewStringPointer(defaultLogPodLogsPath),
						// ContainerSymlinksPath:      apiutils.NewStringPointer(defaultLogContainerSymlinksPath),
						// TempStoragePath:            apiutils.NewStringPointer(defaultLogTempStoragePath),
					},
					LiveProcessCollection: &LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					OOMKill: &OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
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
					CSPM: &CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCSPMEnabled),
					},
					CWS: &CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCWSEnabled),
					},
					NPM: &NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultNPMEnabled),
						// These are not set because the default is Enabled = false
						// EnableConntrack: apiutils.NewBoolPointer(defaultNPMEnableConntrack),
						// CollectDNSStats: apiutils.NewBoolPointer(defaultNPMCollectDNSStats),
					},
					USM: &USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
					},
					EventCollection: &EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &OrchestratorExplorerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						Conf: &CustomConfig{
							ConfigData: apiutils.NewStringPointer(DefaultOrchestratorExplorerConf),
						},
					},
					KubeStateMetricsCore: &KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
						// This is not set because the default is Enabled = false
						// Conf: &CustomConfig{
						// 	ConfigData: apiutils.NewStringPointer(defaultKubeStateMetricsCoreConf),
						// },
					},
					AdmissionController: &AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
					},
					ExternalMetricsServer: &ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultExternalMetricsServerEnabled),
						// These are not set because the default is Enabled = false
						// UseDatadogMetrics: apiutils.NewBoolPointer(defaultDatadogMetricsEnabled),
						// Port:              apiutils.NewInt32Pointer(defaultMetricsProviderPort),
					},
					ClusterChecks: &ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewBoolPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewBoolPointer(defaultUseClusterChecksRunners),
					},
					PrometheusScrape: &PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					DatadogMonitor: &DatadogMonitorFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultDatadogMonitorEnabled),
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
						// These are not set because the default is Enabled = false
						// ContainerCollectUsingFiles: apiutils.NewBoolPointer(defaultLogContainerCollectUsingFiles),
						// ContainerLogsPath:          apiutils.NewStringPointer(defaultLogContainerLogsPath),
						// PodLogsPath:                apiutils.NewStringPointer(defaultLogPodLogsPath),
						// ContainerSymlinksPath:      apiutils.NewStringPointer(defaultLogContainerSymlinksPath),
						// TempStoragePath:            apiutils.NewStringPointer(defaultLogTempStoragePath),
					},
					LiveProcessCollection: &LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					OOMKill: &OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					APM: &APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAPMEnabled),
						// These are not set because the default is Enabled = false
						// HostPortConfig: &HostPortConfig{
						// 	Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						// 	Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
						// },
						// UnixDomainSocketConfig: &UnixDomainSocketConfig{
						// 	Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
						// 	Path:    apiutils.NewStringPointer(defaultAPMSocketPath),
						// },
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
					EventCollection: &EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &OrchestratorExplorerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						Conf: &CustomConfig{
							ConfigData: apiutils.NewStringPointer(DefaultOrchestratorExplorerConf),
						},
					},
					KubeStateMetricsCore: &KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
						// This is not set because the default is Enabled = false
						// Conf: &CustomConfig{
						// 	ConfigData: apiutils.NewStringPointer(defaultKubeStateMetricsCoreConf),
						// },
					},
					AdmissionController: &AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
					},
					ExternalMetricsServer: &ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultExternalMetricsServerEnabled),
						// These are not set because the default is Enabled = false
						// UseDatadogMetrics: apiutils.NewBoolPointer(defaultDatadogMetricsEnabled),
						// Port:              apiutils.NewInt32Pointer(defaultMetricsProviderPort),
					},
					ClusterChecks: &ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewBoolPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewBoolPointer(defaultUseClusterChecksRunners),
					},
					PrometheusScrape: &PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					DatadogMonitor: &DatadogMonitorFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultDatadogMonitorEnabled),
					},
				},
			},
		},
		{
			name: "KubeStateMetricsCore is enabled",
			ddaSpec: &DatadogAgentSpec{
				Features: &DatadogFeatures{
					KubeStateMetricsCore: &KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueTrue),
					},
				},
			},
			want: &DatadogAgentSpec{
				Features: &DatadogFeatures{
					LogCollection: &LogCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLogCollectionEnabled),
						// These are not set because the default is Enabled = false
						// ContainerCollectUsingFiles: apiutils.NewBoolPointer(defaultLogContainerCollectUsingFiles),
						// ContainerLogsPath:          apiutils.NewStringPointer(defaultLogContainerLogsPath),
						// PodLogsPath:                apiutils.NewStringPointer(defaultLogPodLogsPath),
						// ContainerSymlinksPath:      apiutils.NewStringPointer(defaultLogContainerSymlinksPath),
						// TempStoragePath:            apiutils.NewStringPointer(defaultLogTempStoragePath),
					},
					LiveProcessCollection: &LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					OOMKill: &OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					APM: &APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAPMEnabled),
						// These are not set because the default is Enabled = false
						// HostPortConfig: &HostPortConfig{
						// 	Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						// 	Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
						// },
						// UnixDomainSocketConfig: &UnixDomainSocketConfig{
						// 	Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
						// 	Path:    apiutils.NewStringPointer(defaultAPMSocketPath),
						// },
					},
					CSPM: &CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCSPMEnabled),
					},
					CWS: &CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCWSEnabled),
					},
					NPM: &NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultNPMEnabled),
						// These are not set because the default is Enabled = false
						// EnableConntrack: apiutils.NewBoolPointer(defaultNPMEnableConntrack),
						// CollectDNSStats: apiutils.NewBoolPointer(defaultNPMCollectDNSStats),
					},
					USM: &USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
					},
					EventCollection: &EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &OrchestratorExplorerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						Conf: &CustomConfig{
							ConfigData: apiutils.NewStringPointer(DefaultOrchestratorExplorerConf),
						},
					},
					KubeStateMetricsCore: &KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(valueTrue),
						Conf: &CustomConfig{
							ConfigData: apiutils.NewStringPointer(defaultKubeStateMetricsCoreConf),
						},
					},
					AdmissionController: &AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
					},
					ExternalMetricsServer: &ExternalMetricsServerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultExternalMetricsServerEnabled),
						// These are not set because the default is Enabled = false
						// UseDatadogMetrics: apiutils.NewBoolPointer(defaultDatadogMetricsEnabled),
						// Port:              apiutils.NewInt32Pointer(defaultMetricsProviderPort),
					},
					ClusterChecks: &ClusterChecksFeatureConfig{
						Enabled:                 apiutils.NewBoolPointer(defaultClusterChecksEnabled),
						UseClusterChecksRunners: apiutils.NewBoolPointer(defaultUseClusterChecksRunners),
					},
					PrometheusScrape: &PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					DatadogMonitor: &DatadogMonitorFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultDatadogMonitorEnabled),
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
						// These are not set because the default is Enabled = false
						// ContainerCollectUsingFiles: apiutils.NewBoolPointer(defaultLogContainerCollectUsingFiles),
						// ContainerLogsPath:          apiutils.NewStringPointer(defaultLogContainerLogsPath),
						// PodLogsPath:                apiutils.NewStringPointer(defaultLogPodLogsPath),
						// ContainerSymlinksPath:      apiutils.NewStringPointer(defaultLogContainerSymlinksPath),
						// TempStoragePath:            apiutils.NewStringPointer(defaultLogTempStoragePath),
					},
					LiveProcessCollection: &LiveProcessCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveProcessCollectionEnabled),
					},
					LiveContainerCollection: &LiveContainerCollectionFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultLiveContainerCollectionEnabled),
					},
					OOMKill: &OOMKillFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOOMKillEnabled),
					},
					TCPQueueLength: &TCPQueueLengthFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultTCPQueueLengthEnabled),
					},
					APM: &APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAPMEnabled),
						// These are not set because the default is Enabled = false
						// HostPortConfig: &HostPortConfig{
						// 	Enabled: apiutils.NewBoolPointer(defaultAPMHostPortEnabled),
						// 	Port:    apiutils.NewInt32Pointer(defaultAPMHostPort),
						// },
						// UnixDomainSocketConfig: &UnixDomainSocketConfig{
						// 	Enabled: apiutils.NewBoolPointer(defaultAPMSocketEnabled),
						// 	Path:    apiutils.NewStringPointer(defaultAPMSocketPath),
						// },
					},
					CSPM: &CSPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCSPMEnabled),
					},
					CWS: &CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultCWSEnabled),
					},
					NPM: &NPMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultNPMEnabled),
						// These are not set because the default is Enabled = false
						// EnableConntrack: apiutils.NewBoolPointer(defaultNPMEnableConntrack),
						// CollectDNSStats: apiutils.NewBoolPointer(defaultNPMCollectDNSStats),
					},
					USM: &USMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultUSMEnabled),
					},
					EventCollection: &EventCollectionFeatureConfig{
						CollectKubernetesEvents: apiutils.NewBoolPointer(defaultCollectKubernetesEvents),
					},
					OrchestratorExplorer: &OrchestratorExplorerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled),
						Conf: &CustomConfig{
							ConfigData: apiutils.NewStringPointer(DefaultOrchestratorExplorerConf),
						},
					},
					KubeStateMetricsCore: &KubeStateMetricsCoreFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
						// This is not set because the default is Enabled = false
						// Conf: &CustomConfig{
						// 	ConfigData: apiutils.NewStringPointer(defaultKubeStateMetricsCoreConf),
						// },
					},
					AdmissionController: &AdmissionControllerFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerEnabled),
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
					PrometheusScrape: &PrometheusScrapeFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled),
					},
					DatadogMonitor: &DatadogMonitorFeatureConfig{
						Enabled: apiutils.NewBoolPointer(defaultDatadogMonitorEnabled),
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defaultFeaturesConfig(tt.ddaSpec)

			if !reflect.DeepEqual(*tt.ddaSpec.Features.LogCollection, *tt.want.Features.LogCollection) {
				t.Errorf("defaultFeatures() LogCollection = %v, want %v", *tt.ddaSpec.Features.LogCollection, *tt.want.Features.LogCollection)
			}

			if !reflect.DeepEqual(*tt.ddaSpec.Features.LiveProcessCollection, *tt.want.Features.LiveProcessCollection) {
				t.Errorf("defaultFeatures() LiveProcessCollection = %v, want %v", *tt.ddaSpec.Features.LiveProcessCollection, *tt.want.Features.LiveProcessCollection)
			}

			if !reflect.DeepEqual(*tt.ddaSpec.Features.LiveContainerCollection, *tt.want.Features.LiveContainerCollection) {
				t.Errorf("defaultFeatures() LiveContainerCollection = %v, want %v", *tt.ddaSpec.Features.LiveContainerCollection, *tt.want.Features.LiveContainerCollection)
			}

			if !reflect.DeepEqual(*tt.ddaSpec.Features.OOMKill, *tt.want.Features.OOMKill) {
				t.Errorf("defaultFeatures() OOMKill = %v, want %v", *tt.ddaSpec.Features.OOMKill, *tt.want.Features.OOMKill)
			}

			if !reflect.DeepEqual(*tt.ddaSpec.Features.TCPQueueLength, *tt.want.Features.TCPQueueLength) {
				t.Errorf("defaultFeatures() TCPQueueLength = %v, want %v", *tt.ddaSpec.Features.TCPQueueLength, *tt.want.Features.TCPQueueLength)
			}

			if !reflect.DeepEqual(*tt.ddaSpec.Features.APM, *tt.want.Features.APM) {
				t.Errorf("defaultFeatures() APM = %v, want %v", *tt.ddaSpec.Features.APM, *tt.want.Features.APM)
			}

			if !reflect.DeepEqual(*tt.ddaSpec.Features.CSPM, *tt.want.Features.CSPM) {
				t.Errorf("defaultFeatures() CSPM = %v, want %v", *tt.ddaSpec.Features.CSPM, *tt.want.Features.CSPM)
			}

			if !reflect.DeepEqual(*tt.ddaSpec.Features.CWS, *tt.want.Features.CWS) {
				t.Errorf("defaultFeatures() CWS = %v, want %v", *tt.ddaSpec.Features.CWS, *tt.want.Features.CWS)
			}

			if !reflect.DeepEqual(*tt.ddaSpec.Features.NPM, *tt.want.Features.NPM) {
				t.Errorf("defaultFeatures() NPM = %v, want %v", *tt.ddaSpec.Features.NPM, *tt.want.Features.NPM)
			}

			if !reflect.DeepEqual(*tt.ddaSpec.Features.USM, *tt.want.Features.USM) {
				t.Errorf("defaultFeatures() USM = %v, want %v", *tt.ddaSpec.Features.USM, *tt.want.Features.USM)
			}

			if !reflect.DeepEqual(*tt.ddaSpec.Features.EventCollection, *tt.want.Features.EventCollection) {
				t.Errorf("defaultFeatures() EventCollection = %v, want %v", *tt.ddaSpec.Features.EventCollection, *tt.want.Features.EventCollection)
			}

			if !reflect.DeepEqual(*tt.ddaSpec.Features.OrchestratorExplorer, *tt.want.Features.OrchestratorExplorer) {
				t.Errorf("defaultFeatures() OrchestratorExplorer = %v, want %v", *tt.ddaSpec.Features.OrchestratorExplorer, *tt.want.Features.OrchestratorExplorer)
			}

			if !reflect.DeepEqual(*tt.ddaSpec.Features.KubeStateMetricsCore, *tt.want.Features.KubeStateMetricsCore) {
				t.Errorf("defaultFeatures() KubeStateMetricsCore = %v, want %v", *tt.ddaSpec.Features.KubeStateMetricsCore, *tt.want.Features.KubeStateMetricsCore)
			}

			if !reflect.DeepEqual(*tt.ddaSpec.Features.AdmissionController, *tt.want.Features.AdmissionController) {
				t.Errorf("defaultFeatures() AdmissionController = %v, want %v", *tt.ddaSpec.Features.AdmissionController, *tt.want.Features.AdmissionController)
			}

			if !reflect.DeepEqual(*tt.ddaSpec.Features.ExternalMetricsServer, *tt.want.Features.ExternalMetricsServer) {
				t.Errorf("defaultFeatures() ExternalMetricsServer = %v, want %v", *tt.ddaSpec.Features.ExternalMetricsServer, *tt.want.Features.ExternalMetricsServer)
			}

			if !reflect.DeepEqual(*tt.ddaSpec.Features.ClusterChecks, *tt.want.Features.ClusterChecks) {
				t.Errorf("defaultFeatures() ClusterChecks = %v, want %v", *tt.ddaSpec.Features.ClusterChecks, *tt.want.Features.ClusterChecks)
			}

			if !reflect.DeepEqual(*tt.ddaSpec.Features.PrometheusScrape, *tt.want.Features.PrometheusScrape) {
				t.Errorf("defaultFeatures() PrometheusScrape = %v, want %v", *tt.ddaSpec.Features.PrometheusScrape, *tt.want.Features.PrometheusScrape)
			}

			if !reflect.DeepEqual(*tt.ddaSpec.Features.DatadogMonitor, *tt.want.Features.DatadogMonitor) {
				t.Errorf("defaultFeatures() DatadogMonitor = %v, want %v", *tt.ddaSpec.Features.DatadogMonitor, *tt.want.Features.DatadogMonitor)
			}
		})
	}
}
