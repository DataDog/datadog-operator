// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"path"
	"strings"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/pkg/defaulting"
	"github.com/DataDog/datadog-operator/pkg/utils"

	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// default values
const (
	defaultLogLevel                         string = "INFO"
	defaultAgentImageName                   string = "agent"
	defaultClusterAgentImageName            string = "cluster-agent"
	defaultCollectEvents                    bool   = false
	defaultLeaderElection                   bool   = false
	defaultDockerSocketPath                 string = "/var/run/docker.sock"
	defaultDogstatsdOriginDetection         bool   = false
	defaultUseDogStatsDSocketVolume         bool   = false
	defaultHostDogstatsdSocketName          string = "statsd.sock"
	defaultHostDogstatsdSocketPath          string = "/var/run/datadog"
	defaultAgentEnabled                     bool   = true
	defaultClusterAgentEnabled              bool   = true
	defaultClusterChecksRunnerEnabled       bool   = false
	defaultApmEnabled                       bool   = false
	defaultApmHostPort                      int32  = 8126
	defaultExternalMetricsEnabled           bool   = false
	defaultSystemProbeEnabled               bool   = false
	defaultSystemProbeOOMKillEnabled        bool   = false
	defaultSystemProbeTCPQueueLengthEnabled bool   = false
	defaultSystemProbeConntrackEnabled      bool   = false
	defaultSystemProbeCollectDNSStats       bool   = false
	defaultSystemProbeBPFDebugEnabled       bool   = false
	defaultSystemProbeSecCompRootPath       string = "/var/lib/kubelet/seccomp"
	defaultAppArmorProfileName              string = "unconfined"
	DefaultSeccompProfileName               string = "localhost/system-probe"
	defaultSecurityRuntimeEnabled           bool   = false
	defaultSecurityComplianceEnabled        bool   = false
	defaultSecuritySyscallMonitorEnabled    bool   = false
	defaultHostApmSocketName                string = "apm.sock"
	defaultHostApmSocketPath                string = "/var/run/datadog"
	defaultLogEnabled                       bool   = false
	defaultLogsConfigContainerCollectAll    bool   = false
	defaultLogsContainerCollectUsingFiles   bool   = true
	defaultContainerLogsPath                string = "/var/lib/docker/containers"
	defaultPodLogsPath                      string = "/var/log/pods"
	defaultContainerSymlinksPath            string = "/var/log/containers"
	defaultLogsTempStoragePath              string = "/var/lib/datadog-agent/logs"
	defaultProcessEnabled                   bool   = false
	// `false` defaults to live container, agent activated but no process collection
	defaultProcessCollectionEnabled                      bool   = false
	defaultOrchestratorExplorerEnabled                   bool   = true
	defaultOrchestratorExplorerContainerScrubbingEnabled bool   = true
	DefaultOrchestratorExplorerConf                      string = "orchestrator-explorer-config"
	defaultMetricsProviderPort                           int32  = 8443
	defaultClusterChecksEnabled                          bool   = false
	defaultKubeStateMetricsCoreEnabled                   bool   = false
	defaultPrometheusScrapeEnabled                       bool   = false
	defaultPrometheusScrapeServiceEndpoints              bool   = false
	defaultRbacCreate                                           = true
	defaultMutateUnlabelled                                     = false
	DefaultAdmissionServiceName                                 = "datadog-admission-controller"
	defaultAdmissionControllerEnabled                           = false
)

var defaultImagePullPolicy = corev1.PullIfNotPresent

// DefaultDatadogAgent defaults the DatadogAgent
func DefaultDatadogAgent(dda *DatadogAgent) *DatadogAgentStatus {
	// instOverrideStatus contains all the defaults from the runtime.
	// It is published in the status of the DatadogAgent
	dso := &DatadogAgentStatus{
		DefaultOverride: &DatadogAgentSpec{},
	}

	// Creds
	defaultCredentials(dda, dso)

	// Override spec given featureset
	FeatureOverride(&dda.Spec, dso.DefaultOverride)

	// Features
	// default features because it might have an impact on the other defaulting
	dso.DefaultOverride.Features = *DefaultFeatures(dda)

	// Cluster Agent
	dso.DefaultOverride.ClusterAgent = *DefaultDatadogAgentSpecClusterAgent(&dda.Spec.ClusterAgent)

	// Agent
	dso.DefaultOverride.Agent = *DefaultDatadogAgentSpecAgent(&dda.Spec.Agent)

	// CLC
	dso.DefaultOverride.ClusterChecksRunner = *DefaultDatadogAgentSpecClusterChecksRunner(&dda.Spec.ClusterChecksRunner)

	return dso
}

func defaultCredentials(dda *DatadogAgent, dso *DatadogAgentStatus) {
	if dda.Spec.Credentials == nil {
		dda.Spec.Credentials = &AgentCredentials{}
	}

	if dda.Spec.Credentials.UseSecretBackend == nil {
		dda.Spec.Credentials.UseSecretBackend = apiutils.NewBoolPointer(false)
		if dso.DefaultOverride == nil {
			dso.DefaultOverride = &DatadogAgentSpec{}
		}
		dso.DefaultOverride.Credentials = &AgentCredentials{
			UseSecretBackend: dda.Spec.Credentials.UseSecretBackend,
		}
	}

	defaultClusterAgentToken(dda, dso)
}

func defaultClusterAgentToken(dda *DatadogAgent, dso *DatadogAgentStatus) {
	// Token provided in the spec. No need to generate one.
	if dda.Spec.Credentials.Token != "" {
		return
	}

	if dso.DefaultOverride == nil {
		dso.DefaultOverride = &DatadogAgentSpec{}
	}

	if dso.DefaultOverride.Credentials == nil {
		dso.DefaultOverride.Credentials = &AgentCredentials{}
	}

	defaultedToken := DefaultedClusterAgentToken(&dda.Status)

	if defaultedToken != "" {
		dso.DefaultOverride.Credentials.Token = defaultedToken
	} else {
		// For backwards-compatibility, if the token is already in the status
		// use it.
		if dso.ClusterAgent != nil && dso.ClusterAgent.GeneratedToken != "" {
			dso.DefaultOverride.Credentials.Token = dso.ClusterAgent.GeneratedToken
		} else {
			dso.DefaultOverride.Credentials.Token = apiutils.GenerateRandomString(32)
		}
	}
}

// FeatureOverride defaults the feature section of the DatadogAgent
// TODO surface in the status when Overrides are not possible. Security agent requires the System Probe
func FeatureOverride(dda *DatadogAgentSpec, dso *DatadogAgentSpec) {
	if dda.Features.NetworkMonitoring != nil && apiutils.BoolValue(dda.Features.NetworkMonitoring.Enabled) {
		// If the Network Monitoring Feature is enabled, enable the System Probe.
		if !apiutils.BoolValue(dda.Agent.Enabled) {
			if dda.Agent.SystemProbe == nil {
				dda.Agent.SystemProbe = DefaultDatadogAgentSpecAgentSystemProbe(&dda.Agent)
			}
			dda.Agent.SystemProbe.Enabled = apiutils.NewBoolPointer(true)
			dso.Agent.SystemProbe = DefaultDatadogAgentSpecAgentSystemProbe(&dda.Agent)
			dso.Agent.SystemProbe.Enabled = apiutils.NewBoolPointer(true)
		}
	}
	if dda.Features.NetworkMonitoring != nil && apiutils.BoolValue(dda.Features.NetworkMonitoring.Enabled) ||
		dda.Features.OrchestratorExplorer != nil && apiutils.BoolValue(dda.Features.OrchestratorExplorer.Enabled) {
		// If the Network Monitoring or the Orchestrator Explorer Feature is enabled, enable the Process Agent.
		if !apiutils.BoolValue(dda.Agent.Enabled) {
			if dda.Agent.Process == nil {
				dda.Agent.Process = DefaultDatadogAgentSpecAgentProcess(&dda.Agent)
			}
			dda.Agent.Process.Enabled = apiutils.NewBoolPointer(true)
			dso.Agent.Process = DefaultDatadogAgentSpecAgentProcess(&dda.Agent)
			dso.Agent.Process.Enabled = apiutils.NewBoolPointer(true)
		}
	}
}

// DefaultDatadogAgentSpecAgent used to default an DatadogAgentSpecAgentSpec
// return the defaulted DatadogAgentSpecAgentSpec
func DefaultDatadogAgentSpecAgent(agent *DatadogAgentSpecAgentSpec) *DatadogAgentSpecAgentSpec {
	// If the Agent is not specified in the spec, disable it.
	if apiutils.IsEqualStruct(*agent, DatadogAgentSpecAgentSpec{}) {
		agent.Enabled = apiutils.NewBoolPointer(defaultAgentEnabled)

		if !apiutils.BoolValue(agent.Enabled) {
			return agent
		}
	}

	agentOverride := &DatadogAgentSpecAgentSpec{}
	if agent.Enabled == nil {
		agent.Enabled = apiutils.NewBoolPointer(defaultAgentEnabled)
		agentOverride.Enabled = agent.Enabled
	}

	if !apiutils.BoolValue(agent.Enabled) {
		return agentOverride
	}

	if agent.UseExtendedDaemonset == nil {
		agent.UseExtendedDaemonset = apiutils.NewBoolPointer(false)
		agentOverride.UseExtendedDaemonset = agent.UseExtendedDaemonset
	}

	if img := DefaultDatadogAgentSpecAgentImage(agent, defaultAgentImageName, defaulting.AgentLatestVersion); !apiutils.IsEqualStruct(*img, commonv1.AgentImageConfig{}) {
		agentOverride.Image = img
	}

	if cfg := DefaultDatadogAgentSpecAgentConfig(agent); !apiutils.IsEqualStruct(*cfg, NodeAgentConfig{}) {
		agentOverride.Config = cfg
	}

	if rbac := DefaultDatadogAgentSpecRbacConfig(agent); !apiutils.IsEqualStruct(*rbac, RbacConfig{}) {
		agentOverride.Rbac = rbac
	}

	deployStrat := DefaultDatadogAgentSpecDatadogAgentStrategy(agent)
	if !apiutils.IsEqualStruct(*deployStrat, DaemonSetDeploymentStrategy{}) {
		agentOverride.DeploymentStrategy = deployStrat
	}

	if apm := DefaultDatadogAgentSpecAgentApm(agent); !apiutils.IsEqualStruct(*apm, APMSpec{}) {
		agentOverride.Apm = apm
	}

	if sysProb := DefaultDatadogAgentSpecAgentSystemProbe(agent); !apiutils.IsEqualStruct(*sysProb, SystemProbeSpec{}) {
		agentOverride.SystemProbe = sysProb
	}

	if sec := DefaultDatadogAgentSpecAgentSecurity(agent); !apiutils.IsEqualStruct(*sec, SecuritySpec{}) {
		agentOverride.Security = sec
	}

	if proc := DefaultDatadogAgentSpecAgentProcess(agent); !apiutils.IsEqualStruct(*proc, ProcessSpec{}) {
		agentOverride.Process = proc
	}

	if net := DefaultAgentNetworkPolicy(agent); !apiutils.IsEqualStruct(*net, NetworkPolicySpec{}) {
		agentOverride.NetworkPolicy = net
	}

	return agentOverride
}

// DefaultDatadogAgentSpecAgentImage used to default a ImageConfig for the Agent, Cluster Agent and the Cluster Check Runner.
// Returns the defaulted ImageConfig.
func DefaultDatadogAgentSpecAgentImage(agent *DatadogAgentSpecAgentSpec, name, tag string) *commonv1.AgentImageConfig {
	imgOverride := &commonv1.AgentImageConfig{}
	if agent.Image == nil {
		agent.Image = &commonv1.AgentImageConfig{}
	}

	if agent.Image.Name == "" {
		agent.Image.Name = name
		imgOverride.Name = agent.Image.Name
	}

	// Only default Tag if not already present in the image.Name
	if !defaulting.IsImageNameContainsTag(agent.Image.Name) && agent.Image.Tag == "" {
		agent.Image.Tag = tag
		imgOverride.Tag = agent.Image.Tag
	}

	if agent.Image.PullPolicy == nil {
		agent.Image.PullPolicy = &defaultImagePullPolicy
		imgOverride.PullPolicy = agent.Image.PullPolicy
	}

	if agent.Image.PullSecrets == nil {
		agent.Image.PullSecrets = &[]corev1.LocalObjectReference{}
	}

	return imgOverride
}

// GetDefaultLivenessProbe creates a all defaulted LivenessProbe
func GetDefaultLivenessProbe() *corev1.Probe {
	livenessProbe := &corev1.Probe{
		InitialDelaySeconds: apicommon.DefaultLivenessProbeInitialDelaySeconds,
		PeriodSeconds:       apicommon.DefaultLivenessProbePeriodSeconds,
		TimeoutSeconds:      apicommon.DefaultLivenessProbeTimeoutSeconds,
		SuccessThreshold:    apicommon.DefaultLivenessProbeSuccessThreshold,
		FailureThreshold:    apicommon.DefaultLivenessProbeFailureThreshold,
	}
	livenessProbe.HTTPGet = &corev1.HTTPGetAction{
		Path: apicommon.DefaultLivenessProbeHTTPPath,
		Port: intstr.IntOrString{
			IntVal: apicommon.DefaultAgentHealthPort,
		},
	}
	return livenessProbe
}

// GetDefaultReadinessProbe creates a all defaulted ReadynessProbe
func GetDefaultReadinessProbe() *corev1.Probe {
	readinessProbe := &corev1.Probe{
		InitialDelaySeconds: apicommon.DefaultReadinessProbeInitialDelaySeconds,
		PeriodSeconds:       apicommon.DefaultReadinessProbePeriodSeconds,
		TimeoutSeconds:      apicommon.DefaultReadinessProbeTimeoutSeconds,
		SuccessThreshold:    apicommon.DefaultReadinessProbeSuccessThreshold,
		FailureThreshold:    apicommon.DefaultReadinessProbeFailureThreshold,
	}
	readinessProbe.HTTPGet = &corev1.HTTPGetAction{
		Path: apicommon.DefaultReadinessProbeHTTPPath,
		Port: intstr.IntOrString{
			IntVal: apicommon.DefaultAgentHealthPort,
		},
	}
	return readinessProbe
}

// DefaultDatadogAgentSpecAgentConfig used to default a NodeAgentConfig
// return the defaulted NodeAgentConfig
func DefaultDatadogAgentSpecAgentConfig(agent *DatadogAgentSpecAgentSpec) *NodeAgentConfig {
	configOverride := &NodeAgentConfig{}

	if agent.Config == nil {
		agent.Config = &NodeAgentConfig{}
	}

	if agent.Config.LogLevel == nil {
		agent.Config.LogLevel = apiutils.NewStringPointer(defaultLogLevel)
		configOverride.LogLevel = agent.Config.LogLevel
	}

	if agent.Config.CollectEvents == nil {
		agent.Config.CollectEvents = apiutils.NewBoolPointer(defaultCollectEvents)
		configOverride.CollectEvents = agent.Config.CollectEvents
	}

	if agent.Config.LeaderElection == nil {
		agent.Config.LeaderElection = apiutils.NewBoolPointer(defaultLeaderElection)
		configOverride.LeaderElection = agent.Config.LeaderElection
	}

	// Don't default Docker/CRI paths with Agent >= 7.27.0
	// Let Env AD do the work for us
	// Image is defaulted prior to this function.
	agentTag := strings.TrimSuffix(utils.GetTagFromImageName(agent.Image.Name), "-jmx")
	// Check against image tag + "-0"; otherwise prelease versions are not compared.
	// (See https://github.com/Masterminds/semver#working-with-prerelease-versions)
	if !(agentTag == "latest" || utils.IsAboveMinVersion(agentTag, "7.27.0-0") || utils.IsAboveMinVersion(agentTag, "6.27.0-0")) {
		if socketOverride := DefaultContainerSocket(agent.Config); !apiutils.IsEqualStruct(socketOverride, CRISocketConfig{}) {
			configOverride.CriSocket = socketOverride
		}
	}

	if dsdOverride := DefaultConfigDogstatsd(agent.Config); !apiutils.IsEqualStruct(dsdOverride, DogstatsdConfig{}) {
		configOverride.Dogstatsd = dsdOverride
	}

	if agent.Config.Resources == nil {
		agent.Config.Resources = &corev1.ResourceRequirements{}
	}

	if agent.Config.Tags == nil {
		agent.Config.Tags = []string{}
	}

	if agent.Config.LivenessProbe == nil {
		// TODO make liveness probe's fields more configurable
		agent.Config.LivenessProbe = GetDefaultLivenessProbe()
		configOverride.LivenessProbe = agent.Config.LivenessProbe
	}

	if agent.Config.ReadinessProbe == nil {
		// TODO make readiness probe's fields more configurable
		agent.Config.ReadinessProbe = GetDefaultReadinessProbe()
		configOverride.ReadinessProbe = agent.Config.ReadinessProbe
	}

	if agent.Config.HealthPort == nil {
		agent.Config.HealthPort = apiutils.NewInt32Pointer(apicommon.DefaultAgentHealthPort)
		configOverride.HealthPort = agent.Config.HealthPort
	}

	return configOverride
}

// DefaultContainerSocket defaults the socket configuration for the Datadog Agent
func DefaultContainerSocket(config *NodeAgentConfig) *CRISocketConfig {
	if config.CriSocket == nil {
		config.CriSocket = &CRISocketConfig{
			DockerSocketPath: apiutils.NewStringPointer(defaultDockerSocketPath),
		}
		return config.CriSocket
	}
	socketOverride := &CRISocketConfig{}
	if config.CriSocket.DockerSocketPath == nil {
		config.CriSocket.DockerSocketPath = apiutils.NewStringPointer(defaultDockerSocketPath)
		socketOverride.DockerSocketPath = config.CriSocket.DockerSocketPath
	}
	return socketOverride
}

// DefaultConfigDogstatsd used to default Dogstatsd config in NodeAgentConfig
func DefaultConfigDogstatsd(config *NodeAgentConfig) *DogstatsdConfig {
	dsdOverride := &DogstatsdConfig{}
	if config.Dogstatsd == nil {
		config.Dogstatsd = &DogstatsdConfig{}
	}

	if config.Dogstatsd.DogstatsdOriginDetection == nil {
		config.Dogstatsd.DogstatsdOriginDetection = apiutils.NewBoolPointer(defaultDogstatsdOriginDetection)
		dsdOverride.DogstatsdOriginDetection = config.Dogstatsd.DogstatsdOriginDetection
	}

	if uds := DefaultConfigDogstatsdUDS(config.Dogstatsd); !apiutils.IsEqualStruct(uds, DSDUnixDomainSocketSpec{}) {
		dsdOverride.UnixDomainSocket = uds
	}

	return dsdOverride
}

// DefaultConfigDogstatsdUDS used to default DSDUnixDomainSocketSpec
// return the defaulted DSDUnixDomainSocketSpec
func DefaultConfigDogstatsdUDS(dsd *DogstatsdConfig) *DSDUnixDomainSocketSpec {
	if dsd.UnixDomainSocket == nil {
		dsd.UnixDomainSocket = &DSDUnixDomainSocketSpec{}
	}

	udsOverride := &DSDUnixDomainSocketSpec{}
	if dsd.UnixDomainSocket.Enabled == nil {
		dsd.UnixDomainSocket.Enabled = apiutils.NewBoolPointer(defaultUseDogStatsDSocketVolume)
		udsOverride.Enabled = dsd.UnixDomainSocket.Enabled
	}

	if dsd.UnixDomainSocket.HostFilepath == nil {
		socketPath := path.Join(defaultHostDogstatsdSocketPath, defaultHostDogstatsdSocketName)
		dsd.UnixDomainSocket.HostFilepath = &socketPath
		udsOverride.HostFilepath = dsd.UnixDomainSocket.HostFilepath
	}

	return udsOverride
}

// DefaultRbacConfig defaults the RBAC section of the DatadogAgent
func DefaultRbacConfig(rbac *RbacConfig) *RbacConfig {
	rbacOverride := &RbacConfig{}
	if rbac == nil {
		rbac = &RbacConfig{}
	}

	if rbac.Create == nil {
		rbac.Create = apiutils.NewBoolPointer(defaultRbacCreate)
		rbacOverride.Create = rbac.Create
	}

	return rbacOverride
}

// DefaultDatadogClusterChecksRunnerSpecRbacConfig used to default a RbacConfig of the Cluster Check Runner
func DefaultDatadogClusterChecksRunnerSpecRbacConfig(clc *DatadogAgentSpecClusterChecksRunnerSpec) *RbacConfig {
	if clc.Rbac == nil {
		// prevent passing an empty reference
		clc.Rbac = &RbacConfig{}
	}
	return DefaultRbacConfig(clc.Rbac)
}

// DefaultDatadogClusterAgentSpecRbacConfig used to default a RbacConfig of the Cluster Agent
func DefaultDatadogClusterAgentSpecRbacConfig(dca *DatadogAgentSpecClusterAgentSpec) *RbacConfig {
	if dca.Rbac == nil {
		// prevent passing an empty reference
		dca.Rbac = &RbacConfig{}
	}
	return DefaultRbacConfig(dca.Rbac)
}

// DefaultDatadogAgentSpecRbacConfig used to default a RbacConfig
// return the defaulted RbacConfig
func DefaultDatadogAgentSpecRbacConfig(agent *DatadogAgentSpecAgentSpec) *RbacConfig {
	if agent.Rbac == nil {
		// prevent passing an empty reference
		agent.Rbac = &RbacConfig{}
	}
	return DefaultRbacConfig(agent.Rbac)
}

// DefaultDatadogAgentSpecDatadogAgentStrategy used to default a DaemonSetDeploymentStrategy
// return the defaulted DaemonSetDeploymentStrategy
func DefaultDatadogAgentSpecDatadogAgentStrategy(agent *DatadogAgentSpecAgentSpec) *DaemonSetDeploymentStrategy {
	strategyOverride := &DaemonSetDeploymentStrategy{}
	if agent.DeploymentStrategy == nil {
		agent.DeploymentStrategy = &DaemonSetDeploymentStrategy{}
	}

	if agent.DeploymentStrategy.UpdateStrategyType == nil {
		updateStrategy := apicommon.DefaultUpdateStrategy
		agent.DeploymentStrategy.UpdateStrategyType = &updateStrategy
		strategyOverride.UpdateStrategyType = agent.DeploymentStrategy.UpdateStrategyType
	}

	if agent.DeploymentStrategy.RollingUpdate.MaxUnavailable == nil {
		agent.DeploymentStrategy.RollingUpdate.MaxUnavailable = &intstr.IntOrString{
			Type:   intstr.String,
			StrVal: apicommon.DefaultRollingUpdateMaxUnavailable,
		}
		strategyOverride.RollingUpdate.MaxUnavailable = agent.DeploymentStrategy.RollingUpdate.MaxUnavailable
	}

	if agent.DeploymentStrategy.RollingUpdate.MaxPodSchedulerFailure == nil {
		agent.DeploymentStrategy.RollingUpdate.MaxPodSchedulerFailure = &intstr.IntOrString{
			Type:   intstr.String,
			StrVal: apicommon.DefaultRollingUpdateMaxPodSchedulerFailure,
		}
		strategyOverride.RollingUpdate.MaxPodSchedulerFailure = agent.DeploymentStrategy.RollingUpdate.MaxPodSchedulerFailure
	}

	if agent.DeploymentStrategy.RollingUpdate.MaxParallelPodCreation == nil {
		agent.DeploymentStrategy.RollingUpdate.MaxParallelPodCreation = apiutils.NewInt32Pointer(apicommon.DefaultRollingUpdateMaxParallelPodCreation)
		strategyOverride.RollingUpdate.MaxParallelPodCreation = agent.DeploymentStrategy.RollingUpdate.MaxParallelPodCreation
	}

	if agent.DeploymentStrategy.RollingUpdate.SlowStartIntervalDuration == nil {
		agent.DeploymentStrategy.RollingUpdate.SlowStartIntervalDuration = &metav1.Duration{
			Duration: apicommon.DefaultRollingUpdateSlowStartIntervalDuration,
		}
		strategyOverride.RollingUpdate.SlowStartIntervalDuration = agent.DeploymentStrategy.RollingUpdate.SlowStartIntervalDuration
	}

	if agent.DeploymentStrategy.RollingUpdate.SlowStartAdditiveIncrease == nil {
		agent.DeploymentStrategy.RollingUpdate.SlowStartAdditiveIncrease = &intstr.IntOrString{
			Type:   intstr.String,
			StrVal: apicommon.DefaultRollingUpdateSlowStartAdditiveIncrease,
		}
		strategyOverride.RollingUpdate.SlowStartAdditiveIncrease = agent.DeploymentStrategy.RollingUpdate.SlowStartAdditiveIncrease
	}

	if agent.DeploymentStrategy.Canary == nil {
		agent.DeploymentStrategy.Canary = edsdatadoghqv1alpha1.DefaultExtendedDaemonSetSpecStrategyCanary(
			&edsdatadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanary{},
			edsdatadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanaryValidationModeAuto,
		)
		strategyOverride.Canary = agent.DeploymentStrategy.Canary
	}

	if agent.DeploymentStrategy.ReconcileFrequency == nil {
		agent.DeploymentStrategy.ReconcileFrequency = &metav1.Duration{
			Duration: apicommon.DefaultReconcileFrequency,
		}
		strategyOverride.ReconcileFrequency = agent.DeploymentStrategy.ReconcileFrequency
	}

	return strategyOverride
}

// DefaultDatadogAgentSpecAgentApm used to default an APMSpec
// return the defaulted APMSpec
func DefaultDatadogAgentSpecAgentApm(agent *DatadogAgentSpecAgentSpec) *APMSpec {
	if agent.Apm == nil {
		agent.Apm = &APMSpec{Enabled: apiutils.NewBoolPointer(defaultApmEnabled)}
		return agent.Apm
	}

	apmOverride := &APMSpec{}
	if agent.Apm.Enabled == nil {
		agent.Apm.Enabled = apiutils.NewBoolPointer(defaultApmEnabled)
		apmOverride.Enabled = agent.Apm.Enabled
	}

	if !apiutils.BoolValue(agent.Apm.Enabled) {
		return apmOverride
	}

	if agent.Apm.HostPort == nil {
		agent.Apm.HostPort = apiutils.NewInt32Pointer(defaultApmHostPort)
		apmOverride.HostPort = agent.Apm.HostPort
	}

	if agent.Apm.LivenessProbe == nil {
		agent.Apm.LivenessProbe = getDefaultAPMAgentLivenessProbe()
		apmOverride.LivenessProbe = agent.Apm.LivenessProbe
	}

	if udsOverride := DefaultDatadogAgentSpecAgentApmUDS(agent.Apm); !apiutils.IsEqualStruct(udsOverride, APMUnixDomainSocketSpec{}) {
		apmOverride.UnixDomainSocket = udsOverride
	}

	return apmOverride
}

func getDefaultAPMAgentLivenessProbe() *corev1.Probe {
	livenessProbe := &corev1.Probe{
		InitialDelaySeconds: apicommon.DefaultLivenessProbeInitialDelaySeconds,
		PeriodSeconds:       apicommon.DefaultLivenessProbePeriodSeconds,
		TimeoutSeconds:      apicommon.DefaultLivenessProbeTimeoutSeconds,
	}
	livenessProbe.TCPSocket = &corev1.TCPSocketAction{
		Port: intstr.IntOrString{
			IntVal: defaultApmHostPort,
		},
	}
	return livenessProbe
}

// DefaultDatadogAgentSpecAgentApmUDS used to default APMUnixDomainSocketSpec
// rreturn the defaulted APMUnixDomainSocketSpec
func DefaultDatadogAgentSpecAgentApmUDS(apm *APMSpec) *APMUnixDomainSocketSpec {
	if apm.UnixDomainSocket == nil {
		apm.UnixDomainSocket = &APMUnixDomainSocketSpec{Enabled: apiutils.NewBoolPointer(false)}
		return apm.UnixDomainSocket
	}

	udsOverride := &APMUnixDomainSocketSpec{}
	if apm.UnixDomainSocket.Enabled == nil {
		apm.UnixDomainSocket.Enabled = apiutils.NewBoolPointer(false)
		udsOverride.Enabled = apm.UnixDomainSocket.Enabled
	}

	if !apiutils.BoolValue(apm.UnixDomainSocket.Enabled) {
		return udsOverride
	}

	if apm.UnixDomainSocket.HostFilepath == nil {
		socketPath := path.Join(defaultHostApmSocketPath, defaultHostApmSocketName)
		apm.UnixDomainSocket.HostFilepath = &socketPath
		udsOverride.HostFilepath = apm.UnixDomainSocket.HostFilepath
	}

	return udsOverride
}

// DefaultDatadogAgentSpecAgentSystemProbe defaults the System Probe
// This method can be re-run as part of the FeatureOverride
func DefaultDatadogAgentSpecAgentSystemProbe(agent *DatadogAgentSpecAgentSpec) *SystemProbeSpec {
	sysOverride := &SystemProbeSpec{}

	if agent.SystemProbe == nil {
		agent.SystemProbe = &SystemProbeSpec{Enabled: apiutils.NewBoolPointer(defaultSystemProbeEnabled)}
		sysOverride = agent.SystemProbe
	}

	if agent.Security != nil && apiutils.BoolValue(agent.Security.Runtime.Enabled) {
		agent.SystemProbe.Enabled = agent.Security.Runtime.Enabled
		sysOverride = agent.SystemProbe
	}

	if agent.SystemProbe.Enabled == nil {
		agent.SystemProbe.Enabled = apiutils.NewBoolPointer(defaultSystemProbeEnabled)
		sysOverride.Enabled = agent.SystemProbe.Enabled
	}

	if !apiutils.BoolValue(agent.SystemProbe.Enabled) {
		return sysOverride
	}

	if agent.SystemProbe.EnableOOMKill == nil {
		agent.SystemProbe.EnableOOMKill = apiutils.NewBoolPointer(defaultSystemProbeOOMKillEnabled)
		sysOverride.EnableOOMKill = agent.SystemProbe.EnableOOMKill
	}

	if agent.SystemProbe.EnableTCPQueueLength == nil {
		agent.SystemProbe.EnableTCPQueueLength = apiutils.NewBoolPointer(defaultSystemProbeTCPQueueLengthEnabled)
		sysOverride.EnableTCPQueueLength = agent.SystemProbe.EnableTCPQueueLength
	}

	if agent.SystemProbe.BPFDebugEnabled == nil {
		agent.SystemProbe.BPFDebugEnabled = apiutils.NewBoolPointer(defaultSystemProbeBPFDebugEnabled)
		sysOverride.BPFDebugEnabled = agent.SystemProbe.BPFDebugEnabled
	}

	if agent.SystemProbe.CollectDNSStats == nil {
		agent.SystemProbe.CollectDNSStats = apiutils.NewBoolPointer(defaultSystemProbeCollectDNSStats)
		sysOverride.CollectDNSStats = agent.SystemProbe.CollectDNSStats
	}

	if agent.SystemProbe.ConntrackEnabled == nil {
		agent.SystemProbe.ConntrackEnabled = apiutils.NewBoolPointer(defaultSystemProbeConntrackEnabled)
		sysOverride.ConntrackEnabled = agent.SystemProbe.ConntrackEnabled
	}

	if agent.SystemProbe.SecCompRootPath == "" {
		agent.SystemProbe.SecCompRootPath = defaultSystemProbeSecCompRootPath
		sysOverride.SecCompRootPath = agent.SystemProbe.SecCompRootPath
	}

	if agent.SystemProbe.AppArmorProfileName == "" {
		agent.SystemProbe.AppArmorProfileName = defaultAppArmorProfileName
		sysOverride.AppArmorProfileName = agent.SystemProbe.AppArmorProfileName
	}

	if agent.SystemProbe.SecCompProfileName == "" {
		agent.SystemProbe.SecCompProfileName = DefaultSeccompProfileName
		sysOverride.SecCompProfileName = agent.SystemProbe.SecCompProfileName
	}
	return sysOverride
}

// DefaultDatadogAgentSpecAgentSecurity defaults the Security Agent in the DatadogAgentSpec
func DefaultDatadogAgentSpecAgentSecurity(agent *DatadogAgentSpecAgentSpec) *SecuritySpec {
	secOverride := &SecuritySpec{}

	if agent.Security == nil {
		agent.Security = &SecuritySpec{}
	}

	if agent.Security.Compliance.Enabled == nil {
		agent.Security.Compliance.Enabled = apiutils.NewBoolPointer(defaultSecurityComplianceEnabled)
		secOverride.Compliance.Enabled = agent.Security.Compliance.Enabled
	}

	if agent.Security.Runtime.Enabled == nil {
		agent.Security.Runtime.Enabled = apiutils.NewBoolPointer(defaultSecurityRuntimeEnabled)
		secOverride.Runtime.Enabled = agent.Security.Runtime.Enabled
	}

	if agent.Security.Runtime.SyscallMonitor == nil {
		agent.Security.Runtime.SyscallMonitor = &SyscallMonitorSpec{}
		secOverride.Runtime.SyscallMonitor = agent.Security.Runtime.SyscallMonitor
	}

	if agent.Security.Runtime.SyscallMonitor.Enabled == nil {
		agent.Security.Runtime.SyscallMonitor.Enabled = apiutils.NewBoolPointer(defaultSecuritySyscallMonitorEnabled)
		secOverride.Runtime.SyscallMonitor.Enabled = agent.Security.Runtime.SyscallMonitor.Enabled
	}

	return secOverride
}

// DefaultDatadogFeatureLogCollection used to default an LogCollectionConfig
// return the defaulted LogCollectionConfig
func DefaultDatadogFeatureLogCollection(ft *DatadogFeatures) *LogCollectionConfig {
	if ft.LogCollection == nil {
		ft.LogCollection = &LogCollectionConfig{Enabled: apiutils.NewBoolPointer(defaultLogEnabled)}
		return ft.LogCollection
	}

	if ft.LogCollection.Enabled == nil {
		ft.LogCollection.Enabled = apiutils.NewBoolPointer(defaultLogEnabled)
	}

	logOverride := &LogCollectionConfig{Enabled: ft.LogCollection.Enabled}

	if !apiutils.BoolValue(ft.LogCollection.Enabled) {
		return logOverride
	}

	if ft.LogCollection.LogsConfigContainerCollectAll == nil {
		ft.LogCollection.LogsConfigContainerCollectAll = apiutils.NewBoolPointer(defaultLogsConfigContainerCollectAll)
		logOverride.LogsConfigContainerCollectAll = ft.LogCollection.LogsConfigContainerCollectAll
	}

	if ft.LogCollection.ContainerCollectUsingFiles == nil {
		ft.LogCollection.ContainerCollectUsingFiles = apiutils.NewBoolPointer(defaultLogsContainerCollectUsingFiles)
		logOverride.ContainerCollectUsingFiles = ft.LogCollection.ContainerCollectUsingFiles
	}

	if ft.LogCollection.ContainerLogsPath == nil {
		ft.LogCollection.ContainerLogsPath = apiutils.NewStringPointer(defaultContainerLogsPath)
		logOverride.ContainerLogsPath = ft.LogCollection.ContainerLogsPath
	}

	if ft.LogCollection.PodLogsPath == nil {
		ft.LogCollection.PodLogsPath = apiutils.NewStringPointer(defaultPodLogsPath)
		logOverride.PodLogsPath = ft.LogCollection.PodLogsPath
	}

	if ft.LogCollection.ContainerSymlinksPath == nil {
		ft.LogCollection.ContainerSymlinksPath = apiutils.NewStringPointer(defaultContainerSymlinksPath)
		logOverride.ContainerSymlinksPath = ft.LogCollection.ContainerSymlinksPath
	}

	if ft.LogCollection.TempStoragePath == nil {
		ft.LogCollection.TempStoragePath = apiutils.NewStringPointer(defaultLogsTempStoragePath)
		logOverride.TempStoragePath = ft.LogCollection.TempStoragePath
	}

	return logOverride
}

// DefaultDatadogAgentSpecAgentProcess used to default an ProcessSpec
// return the defaulted ProcessSpec
func DefaultDatadogAgentSpecAgentProcess(agent *DatadogAgentSpecAgentSpec) *ProcessSpec {
	if agent.Process == nil {
		agent.Process = &ProcessSpec{
			Enabled:                  apiutils.NewBoolPointer(defaultProcessEnabled),
			ProcessCollectionEnabled: apiutils.NewBoolPointer(defaultProcessCollectionEnabled),
		}
		return agent.Process
	}

	processOverride := &ProcessSpec{}

	if agent.Process.Enabled == nil {
		agent.Process.Enabled = apiutils.NewBoolPointer(defaultProcessEnabled)
		processOverride.Enabled = agent.Process.Enabled
	}

	if !apiutils.BoolValue(agent.Process.Enabled) {
		return processOverride
	}

	if agent.Process.ProcessCollectionEnabled == nil {
		agent.Process.ProcessCollectionEnabled = apiutils.NewBoolPointer(defaultProcessCollectionEnabled)
		processOverride.ProcessCollectionEnabled = agent.Process.ProcessCollectionEnabled
	}

	return processOverride
}

func clusterChecksRunnerEnabled(dda *DatadogAgent) bool {
	if dda.Spec.ClusterChecksRunner.Enabled != nil {
		return *dda.Spec.ClusterChecksRunner.Enabled
	}

	return false
}

// DefaultFeatures used to initialized the Features' default values if necessary
func DefaultFeatures(dda *DatadogAgent) *DatadogFeatures {
	ft := &dda.Spec.Features
	featureOverride := &DatadogFeatures{}

	clusterCheckEnabled := clusterChecksRunnerEnabled(dda)

	if orch := DefaultDatadogFeatureOrchestratorExplorer(ft, clusterCheckEnabled); !apiutils.IsEqualStruct(*orch, OrchestratorExplorerConfig{}) {
		featureOverride.OrchestratorExplorer = orch
	}

	if ksm := DefaultDatadogFeatureKubeStateMetricsCore(ft, clusterCheckEnabled); !apiutils.IsEqualStruct(*ksm, KubeStateMetricsCore{}) {
		featureOverride.KubeStateMetricsCore = ksm
	}

	if promScrape := DefaultDatadogFeaturePrometheusScrape(ft); !apiutils.IsEqualStruct(*promScrape, PrometheusScrapeConfig{}) {
		featureOverride.PrometheusScrape = promScrape
	}

	if logColl := DefaultDatadogFeatureLogCollection(ft); !apiutils.IsEqualStruct(*logColl, LogCollectionConfig{}) {
		featureOverride.LogCollection = logColl
	}

	if net := DefaultDatadogFeatureNetworkMonitoring(ft); !apiutils.IsEqualStruct(*net, NetworkMonitoringConfig{}) {
		featureOverride.NetworkMonitoring = net
	}

	return featureOverride
}

// DefaultDatadogFeatureOrchestratorExplorer used to default an OrchestratorExplorerConfig
// return the defaulted OrchestratorExplorerConfig
func DefaultDatadogFeatureOrchestratorExplorer(ft *DatadogFeatures, withClusterChecksRunner bool) *OrchestratorExplorerConfig {
	if ft.OrchestratorExplorer == nil {
		ft.OrchestratorExplorer = &OrchestratorExplorerConfig{}
	}

	return defaultEnabledDatadogFeatureOrchestratorExplorer(ft.OrchestratorExplorer, withClusterChecksRunner)
}

func defaultEnabledDatadogFeatureOrchestratorExplorer(config *OrchestratorExplorerConfig, withClusterChecksRunner bool) *OrchestratorExplorerConfig {
	explorerConfigOverride := &OrchestratorExplorerConfig{}

	if config.Enabled == nil {
		config.Enabled = apiutils.NewBoolPointer(defaultOrchestratorExplorerEnabled)
		explorerConfigOverride.Enabled = config.Enabled
	}
	if apiutils.BoolValue(config.Enabled) {
		if config.ClusterCheck == nil {
			config.ClusterCheck = apiutils.NewBoolPointer(withClusterChecksRunner)
			explorerConfigOverride.ClusterCheck = config.ClusterCheck
		}

		if config.Scrubbing == nil {
			config.Scrubbing = &Scrubbing{}
			explorerConfigOverride.Scrubbing = config.Scrubbing
		}

		if config.Scrubbing.Containers == nil {
			config.Scrubbing.Containers = apiutils.NewBoolPointer(defaultOrchestratorExplorerContainerScrubbingEnabled)
			explorerConfigOverride.Scrubbing.Containers = config.Scrubbing.Containers
		}
	} else {
		explorerConfigOverride.Enabled = apiutils.NewBoolPointer(false)
	}
	return explorerConfigOverride
}

// DefaultDatadogFeatureKubeStateMetricsCore used to default the Kubernetes State Metrics core check
// Disabled by default with no overridden configuration.
func DefaultDatadogFeatureKubeStateMetricsCore(ft *DatadogFeatures, withClusterChecksRunner bool) *KubeStateMetricsCore {
	if ft.KubeStateMetricsCore == nil {
		ft.KubeStateMetricsCore = &KubeStateMetricsCore{
			Enabled:      apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled),
			ClusterCheck: apiutils.NewBoolPointer(withClusterChecksRunner),
		}
		return ft.KubeStateMetricsCore
	}

	if ft.KubeStateMetricsCore.Enabled == nil {
		ft.KubeStateMetricsCore.Enabled = apiutils.NewBoolPointer(defaultKubeStateMetricsCoreEnabled)
	}

	if ft.KubeStateMetricsCore.ClusterCheck == nil {
		ft.KubeStateMetricsCore.ClusterCheck = apiutils.NewBoolPointer(withClusterChecksRunner)
	}

	ksmCoreOverride := &KubeStateMetricsCore{Enabled: ft.KubeStateMetricsCore.Enabled, ClusterCheck: ft.KubeStateMetricsCore.ClusterCheck}
	return ksmCoreOverride
}

// DefaultDatadogFeaturePrometheusScrape used to default the Prometheus Scrape config
func DefaultDatadogFeaturePrometheusScrape(ft *DatadogFeatures) *PrometheusScrapeConfig {
	if ft.PrometheusScrape == nil {
		ft.PrometheusScrape = &PrometheusScrapeConfig{Enabled: apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled)}
	}

	if ft.PrometheusScrape.Enabled == nil {
		ft.PrometheusScrape.Enabled = apiutils.NewBoolPointer(defaultPrometheusScrapeEnabled)
	}

	promOverride := &PrometheusScrapeConfig{Enabled: ft.PrometheusScrape.Enabled}

	if !apiutils.BoolValue(ft.PrometheusScrape.Enabled) {
		return promOverride
	}

	if ft.PrometheusScrape.ServiceEndpoints == nil {
		ft.PrometheusScrape.ServiceEndpoints = apiutils.NewBoolPointer(defaultPrometheusScrapeServiceEndpoints)
		promOverride.ServiceEndpoints = ft.PrometheusScrape.ServiceEndpoints
	}

	return promOverride
}

// DefaultDatadogFeatureNetworkMonitoring used to default the NetworkMonitoring config
func DefaultDatadogFeatureNetworkMonitoring(ft *DatadogFeatures) *NetworkMonitoringConfig {
	if ft.NetworkMonitoring == nil {
		ft.NetworkMonitoring = &NetworkMonitoringConfig{Enabled: apiutils.NewBoolPointer(false)}

		if !apiutils.BoolValue(ft.NetworkMonitoring.Enabled) {
			return ft.NetworkMonitoring
		}
	}

	if ft.NetworkMonitoring.Enabled == nil {
		ft.NetworkMonitoring.Enabled = apiutils.NewBoolPointer(false)
	}

	netOverride := &NetworkMonitoringConfig{Enabled: ft.NetworkMonitoring.Enabled}

	return netOverride
}

// DefaultDatadogAgentSpecClusterAgent used to default an DatadogAgentSpecClusterAgentSpec
// Mutate the internal DatadogAgentSpecClusterAgent throughout the method
// return the defaulted DatadogAgentSpecClusterAgentSpec to update the status
func DefaultDatadogAgentSpecClusterAgent(clusterAgent *DatadogAgentSpecClusterAgentSpec) *DatadogAgentSpecClusterAgentSpec {
	if apiutils.IsEqualStruct(*clusterAgent, DatadogAgentSpecClusterAgentSpec{}) {
		clusterAgent.Enabled = apiutils.NewBoolPointer(defaultClusterAgentEnabled)

		if !apiutils.BoolValue(clusterAgent.Enabled) {
			return clusterAgent
		}
	}

	clusterAgentOverride := &DatadogAgentSpecClusterAgentSpec{}

	if clusterAgent.Enabled == nil {
		// Cluster Agent is enabled by default unless undeclared then it is set to false.
		clusterAgent.Enabled = apiutils.NewBoolPointer(defaultClusterAgentEnabled)
		clusterAgentOverride.Enabled = clusterAgent.Enabled
	}

	if !apiutils.BoolValue(clusterAgent.Enabled) {
		return clusterAgentOverride
	}

	if clusterAgent.Image == nil {
		clusterAgent.Image = &commonv1.AgentImageConfig{}
	}
	if img := DefaultDatadogClusterAgentImage(clusterAgent, defaultClusterAgentImageName, defaulting.ClusterAgentLatestVersion); !apiutils.IsEqualStruct(*img, commonv1.AgentImageConfig{}) {
		clusterAgentOverride.Image = img
	}

	if cfg := DefaultDatadogAgentSpecClusterAgentConfig(clusterAgent); !apiutils.IsEqualStruct(cfg, ClusterAgentConfig{}) {
		clusterAgentOverride.Config = cfg
	}

	if rbac := DefaultDatadogClusterAgentSpecRbacConfig(clusterAgent); !apiutils.IsEqualStruct(rbac, RbacConfig{}) {
		clusterAgentOverride.Rbac = rbac
	}

	if net := DefaultClusterAgentNetworkPolicy(clusterAgent); !apiutils.IsEqualStruct(net, NetworkPolicySpec{}) {
		clusterAgentOverride.NetworkPolicy = net
	}

	return clusterAgentOverride
}

// DefaultDatadogAgentSpecClusterAgentConfig used to default an ClusterAgentConfig
// return the defaulted ClusterAgentConfig
func DefaultDatadogAgentSpecClusterAgentConfig(dca *DatadogAgentSpecClusterAgentSpec) *ClusterAgentConfig {
	configOverride := &ClusterAgentConfig{}

	if dca.Config == nil {
		dca.Config = &ClusterAgentConfig{}
	}

	if dca.Config.LogLevel == nil {
		dca.Config.LogLevel = apiutils.NewStringPointer(defaultLogLevel)
		configOverride.LogLevel = dca.Config.LogLevel
	}

	if extMetricsOverride := DefaultExternalMetrics(dca.Config); !apiutils.IsEqualStruct(extMetricsOverride, ExternalMetricsConfig{}) {
		configOverride.ExternalMetrics = extMetricsOverride
	}

	if dca.Config.ClusterChecksEnabled == nil {
		dca.Config.ClusterChecksEnabled = apiutils.NewBoolPointer(defaultClusterChecksEnabled)
		configOverride.ClusterChecksEnabled = dca.Config.ClusterChecksEnabled
	}

	if dca.Config.CollectEvents == nil {
		dca.Config.CollectEvents = apiutils.NewBoolPointer(defaultCollectEvents)
		configOverride.CollectEvents = dca.Config.CollectEvents
	}
	if admCtrlOverride := DefaultAdmissionController(dca.Config); !apiutils.IsEqualStruct(admCtrlOverride, AdmissionControllerConfig{}) {
		configOverride.AdmissionController = admCtrlOverride
	}

	if dca.Config.Resources == nil {
		dca.Config.Resources = &corev1.ResourceRequirements{}
	}

	if dca.Config.HealthPort == nil {
		dca.Config.HealthPort = apiutils.NewInt32Pointer(apicommon.DefaultAgentHealthPort)
		configOverride.HealthPort = dca.Config.HealthPort
	}

	return configOverride
}

// DefaultExternalMetrics defaults the External Metrics Server's config in the Cluster Agent's config
func DefaultExternalMetrics(conf *ClusterAgentConfig) *ExternalMetricsConfig {
	if conf.ExternalMetrics == nil {
		conf.ExternalMetrics = &ExternalMetricsConfig{Enabled: apiutils.NewBoolPointer(defaultExternalMetricsEnabled)}

		if !apiutils.BoolValue(conf.ExternalMetrics.Enabled) {
			return conf.ExternalMetrics
		}
	}

	extMetricsOverride := &ExternalMetricsConfig{}
	if conf.ExternalMetrics.Enabled == nil {
		// default to `true` because in that case we know that other parameters are
		// present in the `conf.ExternalMetrics` struct.
		conf.ExternalMetrics.Enabled = apiutils.NewBoolPointer(true)
		extMetricsOverride.Enabled = conf.ExternalMetrics.Enabled
	}

	if conf.ExternalMetrics.Port == nil && apiutils.BoolValue(conf.ExternalMetrics.Enabled) {
		conf.ExternalMetrics.Port = apiutils.NewInt32Pointer(defaultMetricsProviderPort)
		extMetricsOverride.Port = conf.ExternalMetrics.Port
	}
	return extMetricsOverride
}

// DefaultAdmissionController defaults the Admission Controller's config in the Cluster Agent's config
func DefaultAdmissionController(conf *ClusterAgentConfig) *AdmissionControllerConfig {
	if conf.AdmissionController == nil {
		conf.AdmissionController = &AdmissionControllerConfig{Enabled: apiutils.NewBoolPointer(defaultAdmissionControllerEnabled)}

		if !apiutils.BoolValue(conf.AdmissionController.Enabled) {
			return conf.AdmissionController
		}
	}
	admCtrlOverride := &AdmissionControllerConfig{}

	if conf.AdmissionController.Enabled == nil {
		conf.AdmissionController.Enabled = apiutils.NewBoolPointer(defaultAdmissionControllerEnabled)
		admCtrlOverride.Enabled = conf.AdmissionController.Enabled
	}

	if conf.AdmissionController.MutateUnlabelled == nil {
		conf.AdmissionController.MutateUnlabelled = apiutils.NewBoolPointer(defaultMutateUnlabelled)
		admCtrlOverride.MutateUnlabelled = conf.AdmissionController.MutateUnlabelled
	}

	if conf.AdmissionController.ServiceName == nil {
		conf.AdmissionController.ServiceName = apiutils.NewStringPointer(DefaultAdmissionServiceName)
		admCtrlOverride.ServiceName = conf.AdmissionController.ServiceName
	}
	return admCtrlOverride
}

// DefaultDatadogClusterAgentImage used to default a ImageConfig for the Agent, Cluster Agent and the Cluster Check Runner.
// Returns the defaulted ImageConfig.
func DefaultDatadogClusterAgentImage(dca *DatadogAgentSpecClusterAgentSpec, name, tag string) *commonv1.AgentImageConfig {
	imgOverride := &commonv1.AgentImageConfig{}
	if dca.Image == nil {
		dca.Image = &commonv1.AgentImageConfig{}
	}

	if dca.Image.Name == "" {
		dca.Image.Name = name
		imgOverride.Name = dca.Image.Name
	}

	// Only default Tag if not already present in the image.Name
	if !defaulting.IsImageNameContainsTag(dca.Image.Name) && dca.Image.Tag == "" {
		dca.Image.Tag = tag
		imgOverride.Tag = dca.Image.Tag
	}

	if dca.Image.PullPolicy == nil {
		dca.Image.PullPolicy = &defaultImagePullPolicy
		imgOverride.PullPolicy = dca.Image.PullPolicy
	}

	if dca.Image.PullSecrets == nil {
		dca.Image.PullSecrets = &[]corev1.LocalObjectReference{}
	}

	return imgOverride
}

// DefaultDatadogAgentSpecClusterChecksRunner used to default an DatadogAgentSpecClusterChecksRunnerSpec
// return the defaulted DatadogAgentSpecClusterChecksRunnerSpec
func DefaultDatadogAgentSpecClusterChecksRunner(clusterChecksRunner *DatadogAgentSpecClusterChecksRunnerSpec) *DatadogAgentSpecClusterChecksRunnerSpec {
	if apiutils.IsEqualStruct(clusterChecksRunner, DatadogAgentSpecClusterChecksRunnerSpec{}) {
		clusterChecksRunner.Enabled = apiutils.NewBoolPointer(defaultClusterChecksRunnerEnabled)

		if !apiutils.BoolValue(clusterChecksRunner.Enabled) {
			return clusterChecksRunner
		}
	}

	clcOverride := &DatadogAgentSpecClusterChecksRunnerSpec{}
	if clusterChecksRunner.Enabled == nil {
		// Default to `true` because we are in the case it means other parameters
		// are present in the struct.
		clusterChecksRunner.Enabled = apiutils.NewBoolPointer(true)
		clcOverride.Enabled = clusterChecksRunner.Enabled
	}

	if img := DefaultDatadogAgentSpecClusterChecksRunnerImage(clusterChecksRunner, defaultAgentImageName, defaulting.AgentLatestVersion); !apiutils.IsEqualStruct(img, commonv1.AgentImageConfig{}) {
		clcOverride.Image = img
	}

	if cfg := DefaultDatadogAgentSpecClusterChecksRunnerConfig(clusterChecksRunner); !apiutils.IsEqualStruct(cfg, ClusterChecksRunnerConfig{}) {
		clcOverride.Config = cfg
	}

	if rbac := DefaultDatadogClusterChecksRunnerSpecRbacConfig(clusterChecksRunner); !apiutils.IsEqualStruct(rbac, RbacConfig{}) {
		clcOverride.Rbac = rbac
	}

	if net := DefaultClusterChecksRunnerNetworkPolicy(clusterChecksRunner); !apiutils.IsEqualStruct(net, NetworkPolicySpec{}) {
		clcOverride.NetworkPolicy = net
	}

	return clcOverride
}

// DefaultDatadogAgentSpecClusterChecksRunnerImage used to default a ImageConfig for the Agent, Cluster Agent and the Cluster Check Runner.
// Returns the defaulted ImageConfig.
func DefaultDatadogAgentSpecClusterChecksRunnerImage(clc *DatadogAgentSpecClusterChecksRunnerSpec, name, tag string) *commonv1.AgentImageConfig {
	imgOverride := &commonv1.AgentImageConfig{}
	if clc.Image == nil {
		clc.Image = &commonv1.AgentImageConfig{}
	}

	if clc.Image.Name == "" {
		clc.Image.Name = name
		imgOverride.Name = clc.Image.Name
	}

	// Only default Tag if not already present in the image.Name
	if !defaulting.IsImageNameContainsTag(clc.Image.Name) && clc.Image.Tag == "" {
		clc.Image.Tag = tag
		imgOverride.Tag = clc.Image.Tag
	}

	if clc.Image.PullPolicy == nil {
		clc.Image.PullPolicy = &defaultImagePullPolicy
		imgOverride.PullPolicy = clc.Image.PullPolicy
	}

	if clc.Image.PullSecrets == nil {
		clc.Image.PullSecrets = &[]corev1.LocalObjectReference{}
	}

	return imgOverride
}

// DefaultDatadogAgentSpecClusterChecksRunnerConfig used to default an ClusterChecksRunnerConfig
// return the defaulted ClusterChecksRunnerConfig
func DefaultDatadogAgentSpecClusterChecksRunnerConfig(clc *DatadogAgentSpecClusterChecksRunnerSpec) *ClusterChecksRunnerConfig {
	configOverride := &ClusterChecksRunnerConfig{}

	if clc.Config == nil {
		clc.Config = &ClusterChecksRunnerConfig{}
	}

	if clc.Config.LogLevel == nil {
		clc.Config.LogLevel = apiutils.NewStringPointer(defaultLogLevel)
		configOverride.LogLevel = clc.Config.LogLevel
	}

	if clc.Config.LivenessProbe == nil {
		// TODO make liveness probe's fields more configurable
		clc.Config.LivenessProbe = GetDefaultLivenessProbe()
		configOverride.LivenessProbe = clc.Config.LivenessProbe
	}

	if clc.Config.ReadinessProbe == nil {
		// TODO make readiness probe's fields more configurable
		clc.Config.ReadinessProbe = GetDefaultReadinessProbe()
		configOverride.ReadinessProbe = clc.Config.ReadinessProbe
	}
	if clc.Config.HealthPort == nil {
		clc.Config.HealthPort = apiutils.NewInt32Pointer(apicommon.DefaultAgentHealthPort)
		configOverride.HealthPort = clc.Config.HealthPort
	}

	if clc.Config.Resources == nil {
		clc.Config.Resources = &corev1.ResourceRequirements{}
	}
	return configOverride
}

// DefaultNetworkPolicy is used to default NetworkPolicy. Returns the defaulted
// NetworkPolicySpec
func DefaultNetworkPolicy(policy *NetworkPolicySpec) *NetworkPolicySpec {
	policyOverride := &NetworkPolicySpec{}
	if policy == nil {
		policy = &NetworkPolicySpec{}
	}

	if policy.Create == nil {
		policy.Create = apiutils.NewBoolPointer(false)
		policyOverride.Create = policy.Create
	}

	if apiutils.BoolValue(policy.Create) {
		if policy.Flavor == "" {
			policy.Flavor = NetworkPolicyFlavorKubernetes
			policyOverride.Flavor = policy.Flavor
		}

		if policy.Flavor == NetworkPolicyFlavorCilium && policy.DNSSelectorEndpoints == nil {
			policy.DNSSelectorEndpoints = []metav1.LabelSelector{
				{
					MatchLabels: map[string]string{
						"k8s:io.kubernetes.pod.namespace": "kube-system",
						"k8s:k8s-app":                     "kube-dns",
					},
				},
			}
			policyOverride.DNSSelectorEndpoints = policy.DNSSelectorEndpoints
		}
	}

	return policyOverride
}

// DefaultAgentNetworkPolicy defaults the Network Policy for the Datadog Agent
func DefaultAgentNetworkPolicy(agent *DatadogAgentSpecAgentSpec) *NetworkPolicySpec {
	if agent.NetworkPolicy == nil {
		agent.NetworkPolicy = &NetworkPolicySpec{}
	}
	return DefaultNetworkPolicy(agent.NetworkPolicy)
}

// DefaultClusterAgentNetworkPolicy defaults the Network Policy for the Datadog Cluster Agent
func DefaultClusterAgentNetworkPolicy(dca *DatadogAgentSpecClusterAgentSpec) *NetworkPolicySpec {
	if dca.NetworkPolicy == nil {
		dca.NetworkPolicy = &NetworkPolicySpec{}
	}
	return DefaultNetworkPolicy(dca.NetworkPolicy)
}

// DefaultClusterChecksRunnerNetworkPolicy defaults the Network Policy for the Cluster Check Runner
func DefaultClusterChecksRunnerNetworkPolicy(clc *DatadogAgentSpecClusterChecksRunnerSpec) *NetworkPolicySpec {
	if clc.NetworkPolicy == nil {
		clc.NetworkPolicy = &NetworkPolicySpec{}
	}
	return DefaultNetworkPolicy(clc.NetworkPolicy)
}

// DefaultedClusterAgentToken returns the autogenerated token used for the
// communication between the agents and the DCA. If the token has not been
// autogenerated, this function returns an empty string.
func DefaultedClusterAgentToken(ddaStatus *DatadogAgentStatus) string {
	tokenHasBeenDefaulted := ddaStatus.DefaultOverride != nil &&
		ddaStatus.DefaultOverride.Credentials != nil &&
		ddaStatus.DefaultOverride.Credentials.Token != ""

	if !tokenHasBeenDefaulted {
		return ""
	}

	return ddaStatus.DefaultOverride.Credentials.Token
}
