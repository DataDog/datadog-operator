// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package v1alpha1

import (
	"fmt"
	"time"

	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// default values
const (
	DefaultLogLevel                       string = "INFO"
	defaultAgentImage                     string = "datadog/agent:latest"
	defaultCollectEvents                  bool   = false
	defaultLeaderElection                 bool   = false
	defaultDockerSocketPath               string = "/var/run/docker.sock"
	defaultDogstatsdOriginDetection       bool   = false
	defaultUseDogStatsDSocketVolume       bool   = false
	defaultDogstatsdSocketPath            string = "/var/run/datadog/"
	defaultApmEnabled                     bool   = false
	defaultLogEnabled                     bool   = false
	defaultLogsConfigContainerCollectAll  bool   = false
	defaultLogsContainerCollectUsingFiles bool   = true
	defaultContainerLogsPath              string = "/var/lib/docker/containers"
	defaultPodLogsPath                    string = "/var/log/pods"
	defaultLogsTempStoragePath            string = "/var/lib/datadog-agent/logs"
	defaultLogsOpenFilesLimit             int32  = 100
	defaultProcessEnabled                 bool   = false
	// `false` defaults to live container, agent activated but no process collection
	defaultProcessCollectionEnabled                      bool   = false
	defaultOrchestratorExplorerEnabled                   bool   = true
	defaultOrchestratorExplorerContainerScrubbingEnabled bool   = true
	defaultMetricsProviderPort                           int32  = 8443
	defaultClusterChecksEnabled                          bool   = false
	DefaultKubeStateMetricsCoreConf                      string = "kube-state-metrics-core-config"
	defaultKubeStateMetricsCoreEnabled                   bool   = false
	defaultClusterAgentReplicas                          int32  = 1
	defaultAgentCanaryReplicas                           int32  = 1
	defaultClusterChecksRunnerReplicas                   int32  = 2
	defaultClusterAgentImage                             string = "datadog/cluster-agent:latest"
	defaultRollingUpdateMaxUnavailable                          = "10%"
	defaultUpdateStrategy                                       = appsv1.RollingUpdateDaemonSetStrategyType
	defaultRollingUpdateMaxPodSchedulerFailure                  = "10%"
	defaultRollingUpdateMaxParallelPodCreation           int32  = 250
	defaultRollingUpdateSlowStartIntervalDuration               = 1 * time.Minute
	defaultRollingUpdateSlowStartAdditiveIncrease               = "5"
	defaultAgentCanaryDuratrion                                 = 10 * time.Minute
	defaultReconcileFrequency                                   = 10 * time.Second
	defaultRbacCreate                                           = true
	defaultMutateUnlabelled                                     = false
	DefaultAdmissionServiceName                                 = "datadog-admission-controller"
)

var defaultImagePullPolicy = corev1.PullIfNotPresent

// IsDefaultedDatadogAgent used to check if an DatadogAgent was already defaulted
// returns true if yes, else false
func IsDefaultedDatadogAgent(ad *DatadogAgent) bool {
	if ad.Spec.Agent != nil {
		if ad.Spec.Agent.UseExtendedDaemonset == nil {
			return false
		}
		if !IsDefaultedImageConfig(&ad.Spec.Agent.Image) {
			return false
		}

		if !IsDefaultedDatadogAgentSpecAgentConfig(&ad.Spec.Agent.Config) {
			return false
		}

		if !IsDefaultedDatadogAgentSpecRbacConfig(&ad.Spec.Agent.Rbac) {
			return false
		}

		if !IsDefaultedDatadogAgentSpecDatadogAgentStrategy(ad.Spec.Agent.DeploymentStrategy) {
			return false
		}

		if !IsDefaultedDatadogAgentSpecApm(&ad.Spec.Agent.Apm) {
			return false
		}

		if !IsDefaultedDatadogAgentSpecLog(&ad.Spec.Agent.Log) {
			return false
		}

		if !IsDefaultedDatadogAgentSpecProcess(&ad.Spec.Agent.Process) {
			return false
		}

		if !IsDefaultedNetworkPolicy(&ad.Spec.Agent.NetworkPolicy) {
			return false
		}
	}

	if ad.Spec.Features != nil {
		if !IsDefaultedOrchestratorExplorer(ad.Spec.Features.OrchestratorExplorer) {
			return false
		}
		if !IsDefaultedKubeStateMetricsCore(ad.Spec.Features.KubeStateMetricsCore) {
			return false
		}
	}

	if ad.Spec.ClusterAgent != nil {
		if !IsDefaultedImageConfig(&ad.Spec.ClusterAgent.Image) {
			return false
		}

		if !IsDefaultedDatadogAgentSpecClusterAgentConfig(&ad.Spec.ClusterAgent.Config) {
			return false
		}

		if !IsDefaultedDatadogAgentSpecRbacConfig(&ad.Spec.ClusterAgent.Rbac) {
			return false
		}

		if !IsDefaultedNetworkPolicy(&ad.Spec.ClusterAgent.NetworkPolicy) {
			return false
		}

		if ad.Spec.ClusterAgent.Replicas == nil {
			return false
		}
	}

	if ad.Spec.ClusterChecksRunner != nil {
		if !IsDefaultedImageConfig(&ad.Spec.ClusterChecksRunner.Image) {
			return false
		}

		if !IsDefaultedDatadogAgentSpecClusterChecksRunnerConfig(&ad.Spec.ClusterChecksRunner.Config) {
			return false
		}

		if !IsDefaultedNetworkPolicy(&ad.Spec.ClusterChecksRunner.NetworkPolicy) {
			return false
		}

		if ad.Spec.ClusterChecksRunner.Replicas == nil {
			return false
		}
	}

	return true
}

// IsDefaultedImageConfig used to check if a ImageConfig was already defaulted
// returns true if yes, else false
func IsDefaultedImageConfig(imageConfig *ImageConfig) bool {
	if imageConfig == nil {
		return false
	}

	if imageConfig.Name == "" {
		return false
	}

	if imageConfig.PullPolicy == nil {
		return false
	}

	if imageConfig.PullSecrets == nil {
		return false
	}

	return true
}

// IsDefaultedOrchestratorExplorer used to check if the orchestratorExplorer feature was already defaulted
// returns true if yes, else false
func IsDefaultedOrchestratorExplorer(orc *OrchestratorExplorerConfig) bool {
	if orc == nil {
		return false
	}

	if orc.Scrubbing == nil {
		return false
	}

	if orc.Scrubbing.Containers == nil {
		return false
	}

	if orc.Enabled == nil {
		return false
	}

	return true
}

// IsDefaultedKubeStateMetricsCore check if the Kubernetes State Metrics Core has the minimal config
func IsDefaultedKubeStateMetricsCore(ksmCore *KubeStateMetricsCore) bool {
	if ksmCore == nil {
		return false
	}
	if ksmCore.Enabled == nil {
		return false
	}
	return true
}

// IsDefaultedDatadogAgentSpecAgentConfig used to check if a NodeAgentConfig was already defaulted
// returns true if yes, else false
func IsDefaultedDatadogAgentSpecAgentConfig(config *NodeAgentConfig) bool {
	if config == nil {
		return false
	}

	if config.LogLevel == nil {
		return false
	}

	if config.CollectEvents == nil {
		return false
	}

	if config.LeaderElection == nil {
		return false
	}

	if config.Resources == nil {
		return false
	}

	if config.CriSocket == nil {
		return false
	}

	if !IsDefaultedDogstatsConfig(config.Dogstatsd) {
		return false
	}

	return true
}

// IsDefaultedDogstatsConfig used to check if the dogstatsd configuration is defaulted
func IsDefaultedDogstatsConfig(dsd *DogstatsdConfig) bool {
	if dsd == nil {
		return false
	}

	if dsd.DogstatsdOriginDetection == nil {
		return false
	}

	if dsd.UseDogStatsDSocketVolume == nil {
		return false
	}

	if dsd.HostSocketPath == nil {
		return false
	}

	return true
}

// IsDefaultedDatadogAgentSpecRbacConfig used to check if a RbacConfig is defaulted
// returns true if yes, else false
func IsDefaultedDatadogAgentSpecRbacConfig(rbac *RbacConfig) bool {
	if rbac == nil {
		return false
	}

	if rbac.Create == nil {
		return false
	}

	return true
}

// IsDefaultedDatadogAgentSpecDatadogAgentStrategy used to check if a
// DaemonSetDeploymentStrategy was already defaulted
// returns true if yes, else false
func IsDefaultedDatadogAgentSpecDatadogAgentStrategy(strategy *DaemonSetDeploymentStrategy) bool {
	if strategy == nil {
		return false
	}

	if strategy.UpdateStrategyType == nil {
		return false
	}

	if strategy.RollingUpdate.MaxUnavailable == nil {
		return false
	}

	if strategy.RollingUpdate.MaxPodSchedulerFailure == nil {
		return false
	}

	if strategy.RollingUpdate.MaxParallelPodCreation == nil {
		return false
	}

	if strategy.RollingUpdate.SlowStartIntervalDuration == nil {
		return false
	}

	if strategy.RollingUpdate.SlowStartAdditiveIncrease == nil {
		return false
	}

	if strategy.Canary == nil {
		return false
	}

	if strategy.Canary.Replicas == nil {
		return false
	}

	if strategy.Canary.Duration == nil {
		return false
	}

	return true
}

// IsDefaultedDatadogAgentSpecApm used to check if an APMSpec was already defaulted
// returns true if yes, else false
func IsDefaultedDatadogAgentSpecApm(apm *APMSpec) bool {
	if apm == nil {
		return false
	}

	if apm.Enabled == nil {
		return false
	}

	return true
}

// IsDefaultedDatadogAgentSpecLog used to check if an LogSpec was already defaulted
// returns true if yes, else false
func IsDefaultedDatadogAgentSpecLog(log *LogSpec) bool {
	if log == nil {
		return false
	}

	if log.Enabled == nil {
		return false
	}

	if log.LogsConfigContainerCollectAll == nil {
		return false
	}

	if log.ContainerCollectUsingFiles == nil {
		return false
	}

	if log.ContainerLogsPath == nil {
		return false
	}

	if log.PodLogsPath == nil {
		return false
	}

	if log.TempStoragePath == nil {
		return false
	}

	if log.OpenFilesLimit == nil {
		return false
	}

	return true
}

// IsDefaultedDatadogAgentSpecProcess used to check if an ProcessSpec was already defaulted
// returns true if yes, else false
func IsDefaultedDatadogAgentSpecProcess(process *ProcessSpec) bool {
	if process == nil {
		return false
	}

	if process.Enabled == nil {
		return false
	}

	if process.ProcessCollectionEnabled == nil {
		return false
	}

	return true
}

// IsDefaultedNetworkPolicy used to check if a NetworkPolicySpec was already
// defaulted. Returns true if yes, or false otherwise
func IsDefaultedNetworkPolicy(policy *NetworkPolicySpec) bool {
	if policy == nil {
		return false
	}

	if policy.Create == nil {
		return false
	}

	return true
}

// IsDefaultedDatadogAgentSpecClusterAgentConfig used to check if
// a ClusterAgentConfig was already defaulted
// returns true if yes, else false
func IsDefaultedDatadogAgentSpecClusterAgentConfig(config *ClusterAgentConfig) bool {
	return config != nil
}

// IsDefaultedDatadogAgentSpecClusterChecksRunnerConfig used to check if
// a ClusterChecksRunnerConfig was already defaulted
// returns true if yes, else false
func IsDefaultedDatadogAgentSpecClusterChecksRunnerConfig(config *ClusterChecksRunnerConfig) bool {
	return config != nil
}

// DefaultDatadogAgent used to default an DatadogAgent
// return the defaulted DatadogAgent
func DefaultDatadogAgent(ad *DatadogAgent) *DatadogAgent {
	defaultedAD := ad.DeepCopy()
	if defaultedAD.Spec.Agent != nil {
		defaultedAD.Spec.Agent = DefaultDatadogAgentSpecAgent(defaultedAD.Spec.Agent)
	}

	if defaultedAD.Spec.ClusterAgent != nil {
		defaultedAD.Spec.ClusterAgent = DefaultDatadogAgentSpecClusterAgent(defaultedAD.Spec.ClusterAgent)
		if BoolValue(defaultedAD.Spec.ClusterAgent.Config.ClusterChecksEnabled) && ad.Spec.ClusterChecksRunner == nil {
			defaultedAD.Spec.ClusterChecksRunner = &DatadogAgentSpecClusterChecksRunnerSpec{}
		}
	}

	// Initialize the features if necessary
	// TODO defaulting values has to be consistent across all fields of the DatadogAgent
	DefaultFeatures(defaultedAD.Spec.Features)

	if defaultedAD.Spec.ClusterChecksRunner != nil {
		defaultedAD.Spec.ClusterChecksRunner = DefaultDatadogAgentSpecClusterChecksRunner(defaultedAD.Spec.ClusterChecksRunner)
	}

	return defaultedAD
}

// DefaultDatadogAgentSpecAgent used to default an DatadogAgentSpecAgentSpec
// return the defaulted DatadogAgentSpecAgentSpec
func DefaultDatadogAgentSpecAgent(agent *DatadogAgentSpecAgentSpec) *DatadogAgentSpecAgentSpec {
	if agent.UseExtendedDaemonset == nil {
		agent.UseExtendedDaemonset = NewBoolPointer(false)
	}
	DefaultDatadogAgentSpecAgentImage(&agent.Image)
	DefaultDatadogAgentSpecAgentConfig(&agent.Config)
	DefaultDatadogAgentSpecRbacConfig(&agent.Rbac)
	agent.DeploymentStrategy = DefaultDatadogAgentSpecDatadogAgentStrategy(agent.DeploymentStrategy)
	DefaultDatadogAgentSpecAgentApm(&agent.Apm)
	DefaultDatadogAgentSpecAgentLog(&agent.Log)
	DefaultDatadogAgentSpecAgentProcess(&agent.Process)
	DefaultNetworkPolicy(&agent.NetworkPolicy)
	return agent
}

// DefaultDatadogAgentSpecAgentImage used to default a ImageConfig
// return the defaulted ImageConfig
func DefaultDatadogAgentSpecAgentImage(image *ImageConfig) *ImageConfig {
	if image == nil {
		image = &ImageConfig{}
	}

	if image.Name == "" {
		image.Name = defaultAgentImage
	}

	if image.PullPolicy == nil {
		image.PullPolicy = &defaultImagePullPolicy
	}

	if image.PullSecrets == nil {
		image.PullSecrets = &[]corev1.LocalObjectReference{}
	}

	return image
}

// DefaultDatadogAgentSpecAgentConfig used to default a NodeAgentConfig
// return the defaulted NodeAgentConfig
func DefaultDatadogAgentSpecAgentConfig(config *NodeAgentConfig) *NodeAgentConfig {
	if config == nil {
		config = &NodeAgentConfig{}
	}

	if config.LogLevel == nil {
		config.LogLevel = NewStringPointer(DefaultLogLevel)
	}

	if config.CollectEvents == nil {
		config.CollectEvents = NewBoolPointer(defaultCollectEvents)
	}

	if config.LeaderElection == nil {
		config.LeaderElection = NewBoolPointer(defaultLeaderElection)
	}

	if config.Resources == nil {
		config.Resources = &corev1.ResourceRequirements{}
	}

	if config.CriSocket == nil {
		config.CriSocket = &CRISocketConfig{
			DockerSocketPath: NewStringPointer(defaultDockerSocketPath),
		}
	}

	if config.CriSocket.DockerSocketPath == nil {
		config.CriSocket.DockerSocketPath = NewStringPointer(defaultDockerSocketPath)
	}

	DefaultConfigDogstatsd(config)

	if config.PodLabelsAsTags == nil {
		config.PodLabelsAsTags = map[string]string{}
	}

	if config.PodAnnotationsAsTags == nil {
		config.PodAnnotationsAsTags = map[string]string{}
	}

	if config.Tags == nil {
		config.Tags = []string{}
	}

	return config
}

// DefaultConfigDogstatsd used to default Dogstatsd config in NodeAgentConfig
func DefaultConfigDogstatsd(config *NodeAgentConfig) {
	if config.Dogstatsd == nil {
		config.Dogstatsd = &DogstatsdConfig{}
	}

	if config.Dogstatsd.DogstatsdOriginDetection == nil {
		config.Dogstatsd.DogstatsdOriginDetection = NewBoolPointer(defaultDogstatsdOriginDetection)
	}

	if config.Dogstatsd.UseDogStatsDSocketVolume == nil {
		config.Dogstatsd.UseDogStatsDSocketVolume = NewBoolPointer(defaultUseDogStatsDSocketVolume)
	}

	if config.Dogstatsd.HostSocketPath == nil {
		path := defaultDogstatsdSocketPath
		config.Dogstatsd.HostSocketPath = &path
	}
}

// DefaultDatadogAgentSpecRbacConfig used to default a RbacConfig
// return the defaulted RbacConfig
func DefaultDatadogAgentSpecRbacConfig(rbac *RbacConfig) *RbacConfig {
	if rbac == nil {
		rbac = &RbacConfig{}
	}

	if rbac.Create == nil {
		rbac.Create = NewBoolPointer(defaultRbacCreate)
	}

	return rbac
}

// DefaultDatadogAgentSpecDatadogAgentStrategy used to default a DaemonSetDeploymentStrategy
// return the defaulted DaemonSetDeploymentStrategy
func DefaultDatadogAgentSpecDatadogAgentStrategy(strategy *DaemonSetDeploymentStrategy) *DaemonSetDeploymentStrategy {
	if strategy == nil {
		strategy = &DaemonSetDeploymentStrategy{}
	}

	if strategy.UpdateStrategyType == nil {
		updateStrategy := defaultUpdateStrategy
		strategy.UpdateStrategyType = &updateStrategy
	}

	if strategy.RollingUpdate.MaxUnavailable == nil {
		strategy.RollingUpdate.MaxUnavailable = &intstr.IntOrString{
			Type:   intstr.String,
			StrVal: defaultRollingUpdateMaxUnavailable,
		}
	}

	if strategy.RollingUpdate.MaxPodSchedulerFailure == nil {
		strategy.RollingUpdate.MaxPodSchedulerFailure = &intstr.IntOrString{
			Type:   intstr.String,
			StrVal: defaultRollingUpdateMaxPodSchedulerFailure,
		}
	}

	if strategy.RollingUpdate.MaxParallelPodCreation == nil {
		strategy.RollingUpdate.MaxParallelPodCreation = NewInt32Pointer(defaultRollingUpdateMaxParallelPodCreation)
	}

	if strategy.RollingUpdate.SlowStartIntervalDuration == nil {
		strategy.RollingUpdate.SlowStartIntervalDuration = &metav1.Duration{
			Duration: defaultRollingUpdateSlowStartIntervalDuration,
		}
	}

	if strategy.RollingUpdate.SlowStartAdditiveIncrease == nil {
		strategy.RollingUpdate.SlowStartAdditiveIncrease = &intstr.IntOrString{
			Type:   intstr.String,
			StrVal: defaultRollingUpdateSlowStartAdditiveIncrease,
		}
	}

	if strategy.Canary == nil {
		strategy.Canary = &edsdatadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanary{}
	}

	if strategy.Canary.Replicas == nil {
		strategy.Canary.Replicas = &intstr.IntOrString{
			IntVal: defaultAgentCanaryReplicas,
		}
	}

	if strategy.Canary.Duration == nil {
		strategy.Canary.Duration = &metav1.Duration{
			Duration: defaultAgentCanaryDuratrion,
		}
	}

	if strategy.ReconcileFrequency == nil {
		strategy.ReconcileFrequency = &metav1.Duration{
			Duration: defaultReconcileFrequency,
		}
	}

	return strategy
}

// DefaultDatadogAgentSpecAgentApm used to default an APMSpec
// return the defaulted APMSpec
func DefaultDatadogAgentSpecAgentApm(apm *APMSpec) *APMSpec {
	if apm == nil {
		apm = &APMSpec{}
	}

	if apm.Enabled == nil {
		apm.Enabled = NewBoolPointer(defaultApmEnabled)
	}

	return apm
}

// DefaultDatadogAgentSpecAgentLog used to default an LogSpec
// return the defaulted LogSpec
func DefaultDatadogAgentSpecAgentLog(log *LogSpec) *LogSpec {
	if log == nil {
		log = &LogSpec{}
	}

	if log.Enabled == nil {
		log.Enabled = NewBoolPointer(defaultLogEnabled)
	}

	if log.LogsConfigContainerCollectAll == nil {
		log.LogsConfigContainerCollectAll = NewBoolPointer(defaultLogsConfigContainerCollectAll)
	}

	if log.ContainerCollectUsingFiles == nil {
		log.ContainerCollectUsingFiles = NewBoolPointer(defaultLogsContainerCollectUsingFiles)
	}

	if log.ContainerLogsPath == nil {
		log.ContainerLogsPath = NewStringPointer(defaultContainerLogsPath)
	}

	if log.PodLogsPath == nil {
		log.PodLogsPath = NewStringPointer(defaultPodLogsPath)
	}

	if log.TempStoragePath == nil {
		log.TempStoragePath = NewStringPointer(defaultLogsTempStoragePath)
	}

	if log.OpenFilesLimit == nil {
		log.OpenFilesLimit = NewInt32Pointer(defaultLogsOpenFilesLimit)
	}

	return log
}

// DefaultDatadogAgentSpecAgentProcess used to default an ProcessSpec
// return the defaulted ProcessSpec
func DefaultDatadogAgentSpecAgentProcess(process *ProcessSpec) *ProcessSpec {
	if process == nil {
		process = &ProcessSpec{}
	}

	if process.Enabled == nil {
		process.Enabled = NewBoolPointer(defaultProcessEnabled)
	}

	if process.ProcessCollectionEnabled == nil {
		process.ProcessCollectionEnabled = NewBoolPointer(defaultProcessCollectionEnabled)
	}

	return process
}

// DefaultFeatures used to initialized the Features' default values if necessary
func DefaultFeatures(ft *DatadogFeatures) *DatadogFeatures {
	if ft == nil {
		return &DatadogFeatures{}
	}
	ft.OrchestratorExplorer = DefaultDatadogFeatureOrchestratorExplorer(ft.OrchestratorExplorer)
	ft.KubeStateMetricsCore = DefaultDatadogFeatureKubeStateMetricsCore(ft.KubeStateMetricsCore)
	return ft
}

// DefaultDatadogFeatureOrchestratorExplorer used to default an OrchestratorExplorerConfig
// return the defaulted OrchestratorExplorerConfig
func DefaultDatadogFeatureOrchestratorExplorer(explorerConfig *OrchestratorExplorerConfig) *OrchestratorExplorerConfig {
	if explorerConfig == nil {
		explorerConfig = &OrchestratorExplorerConfig{}
	}

	if explorerConfig.Enabled == nil {
		explorerConfig.Enabled = NewBoolPointer(defaultOrchestratorExplorerEnabled)
	}

	if explorerConfig.Scrubbing == nil || explorerConfig.Scrubbing.Containers == nil {
		explorerConfig.Scrubbing = &Scrubbing{Containers: NewBoolPointer(defaultOrchestratorExplorerContainerScrubbingEnabled)}
	}

	return explorerConfig
}

// DefaultDatadogFeatureKubeStateMetricsCore used to default the Kubernetes State Metrics core check
// Disabled by default with no overridden configuration.
func DefaultDatadogFeatureKubeStateMetricsCore(ksmCore *KubeStateMetricsCore) *KubeStateMetricsCore {
	if ksmCore == nil {
		ksmCore = &KubeStateMetricsCore{}
	}

	if ksmCore.Enabled == nil {
		ksmCore.Enabled = NewBoolPointer(defaultKubeStateMetricsCoreEnabled)
	}

	return ksmCore
}

// DefaultDatadogAgentSpecClusterAgent used to default an DatadogAgentSpecClusterAgentSpec
// return the defaulted DatadogAgentSpecClusterAgentSpec
func DefaultDatadogAgentSpecClusterAgent(clusterAgent *DatadogAgentSpecClusterAgentSpec) *DatadogAgentSpecClusterAgentSpec {
	DefaultDatadogAgentSpecClusterAgentImage(&clusterAgent.Image)
	DefaultDatadogAgentSpecClusterAgentConfig(&clusterAgent.Config)
	DefaultDatadogAgentSpecRbacConfig(&clusterAgent.Rbac)
	DefaultNetworkPolicy(&clusterAgent.NetworkPolicy)
	if clusterAgent.Replicas == nil {
		clusterAgent.Replicas = NewInt32Pointer(defaultClusterAgentReplicas)
	}
	return clusterAgent
}

// DefaultDatadogAgentSpecClusterAgentConfig used to default an ClusterAgentConfig
// return the defaulted ClusterAgentConfig
func DefaultDatadogAgentSpecClusterAgentConfig(config *ClusterAgentConfig) *ClusterAgentConfig {
	if config == nil {
		config = &ClusterAgentConfig{}
	}

	if config.ExternalMetrics != nil {
		if config.ExternalMetrics.Port == nil {
			config.ExternalMetrics.Port = NewInt32Pointer(defaultMetricsProviderPort)
		}
	}

	if config.ClusterChecksEnabled == nil {
		config.ClusterChecksEnabled = NewBoolPointer(defaultClusterChecksEnabled)
	}

	if config.CollectEvents == nil {
		config.CollectEvents = NewBoolPointer(defaultCollectEvents)
	}

	if config.AdmissionController != nil {
		if config.AdmissionController.MutateUnlabelled == nil {
			NewBoolPointer(defaultMutateUnlabelled)
		}
		if config.AdmissionController.ServiceName == nil {
			NewStringPointer(DefaultAdmissionServiceName)
		}
	}

	return config
}

// GetKubeStateMetricsConfName get the name of the Configmap for the KSM Core check.
func GetKubeStateMetricsConfName(dcaConf *DatadogAgent) string {
	// `configData` and `configMap` can't be set together.
	// Return the default if the conf is not overridden or if it is just overridden with the ConfigData.
	if dcaConf.Spec.Features.KubeStateMetricsCore.Conf != nil && dcaConf.Spec.Features.KubeStateMetricsCore.Conf.ConfigMap != nil {
		return dcaConf.Spec.Features.KubeStateMetricsCore.Conf.ConfigMap.Name
	}
	return fmt.Sprintf("%s-%s", dcaConf.Name, DefaultKubeStateMetricsCoreConf)
}

// DefaultDatadogAgentSpecClusterAgentImage used to default ImageConfig for the Datadog Cluster Agent
// return the defaulted ImageConfig
func DefaultDatadogAgentSpecClusterAgentImage(image *ImageConfig) *ImageConfig {
	if image == nil {
		image = &ImageConfig{}
	}

	if image.Name == "" {
		image.Name = defaultClusterAgentImage
	}

	if image.PullPolicy == nil {
		image.PullPolicy = &defaultImagePullPolicy
	}

	if image.PullSecrets == nil {
		image.PullSecrets = &[]corev1.LocalObjectReference{}
	}

	return image
}

// DefaultDatadogAgentSpecClusterChecksRunner used to default an DatadogAgentSpecClusterChecksRunnerSpec
// return the defaulted DatadogAgentSpecClusterChecksRunnerSpec
func DefaultDatadogAgentSpecClusterChecksRunner(clusterChecksRunner *DatadogAgentSpecClusterChecksRunnerSpec) *DatadogAgentSpecClusterChecksRunnerSpec {
	DefaultDatadogAgentSpecClusterChecksRunnerImage(&clusterChecksRunner.Image)
	DefaultDatadogAgentSpecClusterChecksRunnerConfig(&clusterChecksRunner.Config)
	DefaultDatadogAgentSpecRbacConfig(&clusterChecksRunner.Rbac)
	DefaultNetworkPolicy(&clusterChecksRunner.NetworkPolicy)
	if clusterChecksRunner.Replicas == nil {
		clusterChecksRunner.Replicas = NewInt32Pointer(defaultClusterChecksRunnerReplicas)
	}
	return clusterChecksRunner
}

// DefaultDatadogAgentSpecClusterChecksRunnerConfig used to default an ClusterChecksRunnerConfig
// return the defaulted ClusterChecksRunnerConfig
func DefaultDatadogAgentSpecClusterChecksRunnerConfig(config *ClusterChecksRunnerConfig) *ClusterChecksRunnerConfig {
	if config == nil {
		config = &ClusterChecksRunnerConfig{}
	}

	return config
}

// DefaultDatadogAgentSpecClusterChecksRunnerImage used to default ImageConfig for the Datadog Cluster Agent
// return the defaulted ImageConfig
func DefaultDatadogAgentSpecClusterChecksRunnerImage(image *ImageConfig) *ImageConfig {
	if image == nil {
		image = &ImageConfig{}
	}

	if image.Name == "" {
		image.Name = defaultAgentImage
	}

	if image.PullPolicy == nil {
		image.PullPolicy = &defaultImagePullPolicy
	}

	if image.PullSecrets == nil {
		image.PullSecrets = &[]corev1.LocalObjectReference{}
	}

	return image
}

// DefaultNetworkPolicy is used to default NetworkPolicy. Returns the defaulted
// ImageConfig
func DefaultNetworkPolicy(policy *NetworkPolicySpec) *NetworkPolicySpec {
	if policy == nil {
		policy = &NetworkPolicySpec{}
	}

	if policy.Create == nil {
		policy.Create = NewBoolPointer(false)
	}

	return policy
}
