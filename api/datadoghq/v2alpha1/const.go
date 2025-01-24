// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

// This file tracks constants related to the DatadogAgent CRD

const (
	// ClusterAgentReconcileConditionType ReconcileConditionType for Cluster Agent component
	ClusterAgentReconcileConditionType = "ClusterAgentReconcile"
	// AgentReconcileConditionType ReconcileConditionType for Agent component
	AgentReconcileConditionType = "AgentReconcile"
	// ClusterChecksRunnerReconcileConditionType ReconcileConditionType for Cluster Checks Runner component
	ClusterChecksRunnerReconcileConditionType = "ClusterChecksRunnerReconcile"
	// OverrideReconcileConflictConditionType ReconcileConditionType for override conflict
	OverrideReconcileConflictConditionType = "OverrideReconcileConflict"
	// DatadogAgentReconcileErrorConditionType ReconcileConditionType for DatadogAgent reconcile error
	DatadogAgentReconcileErrorConditionType = "DatadogAgentReconcileError"

	// ExtraConfdConfigMapName is the name of the ConfigMap storing Custom Confd data
	ExtraConfdConfigMapName = "%s-extra-confd"
	// ExtraChecksdConfigMapName is the name of the ConfigMap storing Custom Checksd data
	ExtraChecksdConfigMapName = "%s-extra-checksd"

	// DefaultMetricsServerResourceSuffix use as suffix for cluster-agent metrics-server resource naming
	DefaultMetricsServerResourceSuffix = "cluster-agent-metrics-server"
	// DefaultAPPKeyKey default app-key key (use in secret for instance).
	DefaultAPPKeyKey = "app_key"
	// DefaultAPIKeyKey default api-key key (use in secret for instance).
	DefaultAPIKeyKey = "api_key"
	// DefaultTokenKey default token key (use in secret for instance).
	DefaultTokenKey = "token"
	// DefaultClusterAgentReplicas default cluster-agent deployment replicas
	DefaultClusterAgentReplicas = 1
	// DefaultClusterAgentServicePort default cluster-agent service port
	DefaultClusterAgentServicePort = 5005
	// DefaultClusterChecksRunnerReplicas default cluster checks runner deployment replicas
	DefaultClusterChecksRunnerReplicas = 1
	// DefaultAdmissionControllerServicePort default admission controller service port
	DefaultAdmissionControllerServicePort = 443
	// DefaultAdmissionControllerTargetPort default admission controller pod port
	DefaultAdmissionControllerTargetPort = 8000
	// DefaultAdmissionControllerWebhookName default admission controller webhook name
	DefaultAdmissionControllerWebhookName string = "datadog-webhook"
	// DefaultDogstatsdPort default dogstatsd port
	DefaultDogstatsdPort = 8125
	// DefaultDogstatsdPortName default dogstatsd port name
	DefaultDogstatsdPortName = "dogstatsdport"
	// DefaultOTelAgentConf default otel agent ConfigMap name
	DefaultOTelAgentConf string = "otel-agent-config"
	// DefaultKubeStateMetricsCoreConf default ksm core ConfigMap name
	DefaultKubeStateMetricsCoreConf string = "kube-state-metrics-core-config"
	// DefaultOrchestratorExplorerConf default orchestrator explorer ConfigMap name
	DefaultOrchestratorExplorerConf string = "orchestrator-explorer-config"
	// DefaultKubeAPIServerConf default Kubernetes APIServer ConfigMap name
	DefaultKubeAPIServerConf string = "kube-apiserver-config"
	// DefaultSystemProbeSocketPath default System Probe socket path
	DefaultSystemProbeSocketPath string = "/var/run/sysprobe/sysprobe.sock"
	// DefaultCSPMConf default CSPM ConfigMap name
	DefaultCSPMConf string = "cspm-config"
	// DefaultCWSConf default CWS ConfigMap name
	DefaultCWSConf string = "cws-config"
	// DefaultHelmCheckConf default Helm Check ConfigMap name
	DefaultHelmCheckConf string = "helm-check-config"

	// Default Image name
	DefaultAgentImageName        string = "agent"
	DefaultClusterAgentImageName string = "cluster-agent"
	DefaultImageRegistry         string = "gcr.io/datadoghq"
	DefaultAzureImageRegistry    string = "datadoghq.azurecr.io"
	DefaultEuropeImageRegistry   string = "eu.gcr.io/datadoghq"
	DefaultAsiaImageRegistry     string = "asia.gcr.io/datadoghq"
	DefaultGovImageRegistry      string = "public.ecr.aws/datadog"

	KubeServicesAndEndpointsConfigProviders = "kube_services kube_endpoints"
	KubeServicesAndEndpointsListeners       = "kube_services kube_endpoints"
	EndpointsChecksConfigProvider           = "endpointschecks"
	ClusterAndEndpointsConfigProviders      = "clusterchecks endpointschecks"

	// DefaultGPUMonitoringRuntimeClass default runtime class for GPU pods
	DefaultGPUMonitoringRuntimeClass = "nvidia"
)

// Labels
const (
	// MD5AgentDeploymentProviderLabelKey label key is used to identify which provider is being used
	MD5AgentDeploymentProviderLabelKey = "agent.datadoghq.com/provider"
	// MD5AgentDeploymentAnnotationKey annotation key used on a Resource in order to identify which AgentDeployment have been used to generate it.
	MD5AgentDeploymentAnnotationKey = "agent.datadoghq.com/agentspechash"
	// MD5ChecksumAnnotationKey annotation key is used to identify customConfig configurations
	MD5ChecksumAnnotationKey = "checksum/%s-custom-config"
)

// Annotations
const (
	AppArmorAnnotationKey = "container.apparmor.security.beta.kubernetes.io"

	SystemProbeAppArmorAnnotationKey   = "container.apparmor.security.beta.kubernetes.io/system-probe"
	SystemProbeAppArmorAnnotationValue = "unconfined"

	AgentAppArmorAnnotationKey   = "container.apparmor.security.beta.kubernetes.io/agent"
	AgentAppArmorAnnotationValue = "unconfined"
)

// Volumes and paths
const (
	ConfdVolumeName   = "confd"
	ConfdVolumePath   = "/conf.d"
	ConfigVolumeName  = "config"
	ConfigVolumePath  = "/etc/datadog-agent"
	ChecksdVolumeName = "checksd"
	ChecksdVolumePath = "/checks.d"

	HostRootVolumeName = "hostroot"
	HostRootHostPath   = "/"
	HostRootMountPath  = "/host/root"

	GroupVolumeName = "group"
	GroupHostPath   = "/etc/group"
	GroupMountPath  = "/etc/group"

	PasswdVolumeName = "passwd"
	PasswdHostPath   = "/etc/passwd"
	PasswdMountPath  = "/etc/passwd"

	ProcdirVolumeName = "procdir"
	ProcdirHostPath   = "/proc"
	ProcdirMountPath  = "/host/proc"

	CgroupsVolumeName = "cgroups"
	CgroupsHostPath   = "/sys/fs/cgroup"
	CgroupsMountPath  = "/host/sys/fs/cgroup"

	SystemProbeOSReleaseDirVolumeName = "host-osrelease"
	SystemProbeOSReleaseDirVolumePath = "/etc/os-release"
	SystemProbeOSReleaseDirMountPath  = "/host/etc/os-release"

	SystemProbeSocketVolumeName = "sysprobe-socket-dir"
	SystemProbeSocketVolumePath = "/var/run/sysprobe"

	DebugfsVolumeName = "debugfs"
	// same path on host and container
	DebugfsPath = "/sys/kernel/debug"

	ModulesVolumeName = "modules"
	// same path on host and container
	ModulesVolumePath = "/lib/modules"

	SrcVolumeName = "src"
	// same path on host and container
	SrcVolumePath = "/usr/src"

	AgentCustomConfigVolumePath = "/etc/datadog-agent/datadog.yaml"
	SystemProbeConfigVolumePath = "/etc/datadog-agent/system-probe.yaml"
	OtelCustomConfigVolumePath  = "/etc/datadog-agent/otel-config.yaml"

	LogDatadogVolumeName      = "logdatadog"
	LogDatadogVolumePath      = "/var/log/datadog"
	DefaultLogTempStoragePath = "/var/lib/datadog-agent/logs"
	TmpVolumeName             = "tmp"
	TmpVolumePath             = "/tmp"
	CertificatesVolumeName    = "certificates"
	CertificatesVolumePath    = "/etc/datadog-agent/certificates"
	AuthVolumeName            = "datadog-agent-auth"
	AuthVolumePath            = "/etc/datadog-agent/auth"
	InstallInfoVolumeName     = "installinfo"
	InstallInfoVolumeSubPath  = "install_info"
	InstallInfoVolumePath     = "/etc/datadog-agent/install_info"
	InstallInfoVolumeReadOnly = true

	DogstatsdHostPortName      = "dogstatsdport"
	DogstatsdHostPortHostPort  = 8125
	DogstatsdSocketVolumeName  = "dsdsocket"
	DogstatsdAPMSocketHostPath = "/var/run/datadog"
	DogstatsdSocketLocalPath   = "/var/run/datadog"
	DogstatsdSocketName        = "dsd.socket"

	HostCriSocketPathPrefix = "/host"
	CriSocketVolumeName     = "runtimesocketdir"
	RuntimeDirVolumePath    = "/var/run"

	KubeletAgentCAPath  = "/var/run/host-kubelet-ca.crt"
	KubeletCAVolumeName = "kubelet-ca"

	APMSocketName = "apm.socket"

	ExternalMetricsAPIServiceName = "v1beta1.external.metrics.k8s.io"

	SeccompSecurityVolumeName                   = "datadog-agent-security"
	SeccompSecurityVolumePath                   = "/etc/config"
	SeccompRootVolumeName                       = "seccomp-root"
	SeccompRootVolumePath                       = "/host/var/lib/kubelet/seccomp"
	SeccompRootPath                             = "/var/lib/kubelet/seccomp"
	SystemProbeSeccompKey                       = "system-probe-seccomp.json"
	SystemProbeAgentSecurityConfigMapSuffixName = "system-probe-seccomp"
	SystemProbeSeccompProfileName               = "system-probe"

	AgentCustomConfigVolumeName        = "custom-datadog-yaml"
	ClusterAgentCustomConfigVolumeName = "custom-cluster-agent-yaml"

	FIPSProxyCustomConfigVolumeName = "fips-proxy-cfg"
	FIPSProxyCustomConfigFileName   = "datadog-fips-proxy.cfg"
	FIPSProxyCustomConfigMapName    = "%s-fips-config"
	FIPSProxyCustomConfigMountPath  = "/etc/datadog-fips-proxy/datadog-fips-proxy.cfg"
)

// Field paths
const (
	// FieldPathSpecNodeName used as FieldPath for selecting the NodeName
	FieldPathSpecNodeName = "spec.nodeName"

	// FieldPathStatusHostIP used as FieldPath to retrieve the host ip
	FieldPathStatusHostIP = "status.hostIP"

	// FieldPathStatusPodIP used as FieldPath to retrieve the pod ip
	FieldPathStatusPodIP = "status.podIP"

	// FieldPathMetaName used as FieldPath to retrieve the pod name
	FieldPathMetaName = "metadata.name"
)
