// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package component

import (
	"fmt"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/version"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object"
	cilium "github.com/DataDog/datadog-operator/pkg/cilium/v1"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/utils"
)

const (
	localServiceMinimumVersion        = "1.21-0"
	localServiceDefaultMinimumVersion = "1.22-0"
)

// GetVolumeForConfig return the volume that contains the agent config
func GetVolumeForConfig() corev1.Volume {
	return corev1.Volume{
		Name: apicommon.ConfigVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetVolumeForConfd return the volume that contains the agent confd config files
func GetVolumeForConfd() corev1.Volume {
	return corev1.Volume{
		Name: apicommon.ConfdVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetVolumeForChecksd return the volume that contains the agent confd config files
func GetVolumeForChecksd() corev1.Volume {
	return corev1.Volume{
		Name: apicommon.ChecksdVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetVolumeForRmCorechecks return the volume that overwrites the corecheck directory
func GetVolumeForRmCorechecks() corev1.Volume {
	return corev1.Volume{
		Name: "remove-corechecks",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetVolumeForAuth return the Volume container authentication information
func GetVolumeForAuth() corev1.Volume {
	return corev1.Volume{
		Name: apicommon.AuthVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetVolumeForLogs return the Volume that should container generated logs
func GetVolumeForLogs() corev1.Volume {
	return corev1.Volume{
		Name: apicommon.LogDatadogVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetVolumeInstallInfo return the Volume that should install-info file
func GetVolumeInstallInfo(owner metav1.Object) corev1.Volume {
	return corev1.Volume{
		Name: apicommon.InstallInfoVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: GetInstallInfoConfigMapName(owner),
				},
			},
		},
	}
}

// GetVolumeForProc returns the volume with /proc
func GetVolumeForProc() corev1.Volume {
	return corev1.Volume{
		Name: apicommon.ProcdirVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: apicommon.ProcdirHostPath,
			},
		},
	}
}

// GetVolumeForCgroups returns the volume that contains the cgroup directory
func GetVolumeForCgroups() corev1.Volume {
	return corev1.Volume{
		Name: apicommon.CgroupsVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: "/sys/fs/cgroup",
			},
		},
	}
}

// GetVolumeForDogstatsd returns the volume with the Dogstatsd socket
func GetVolumeForDogstatsd() corev1.Volume {
	return corev1.Volume{
		Name: apicommon.DogstatsdSocketVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetInstallInfoConfigMapName return the InstallInfo config map name base on the dda name
func GetInstallInfoConfigMapName(dda metav1.Object) string {
	return fmt.Sprintf("%s-install-info", dda.GetName())
}

// GetVolumeMountForConfig return the VolumeMount that contains the agent config
func GetVolumeMountForConfig() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.ConfigVolumeName,
		MountPath: apicommon.ConfigVolumePath,
	}
}

// GetVolumeMountForConfd return the VolumeMount that contains the agent confd config files
func GetVolumeMountForConfd() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.ConfdVolumeName,
		MountPath: apicommon.ConfdVolumePath,
		ReadOnly:  true,
	}
}

// GetVolumeMountForChecksd return the VolumeMount that contains the agent checksd config files
func GetVolumeMountForChecksd() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.ChecksdVolumeName,
		MountPath: apicommon.ChecksdVolumePath,
		ReadOnly:  true,
	}
}

// GetVolumeMountForRmCorechecks return the VolumeMount that overwrites the corechecks directory
func GetVolumeMountForRmCorechecks() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      "remove-corechecks",
		MountPath: fmt.Sprintf("%s/%s", apicommon.ConfigVolumePath, "conf.d"),
	}
}

// GetVolumeMountForAuth returns the VolumeMount that contains the authentication information
func GetVolumeMountForAuth(readOnly bool) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.AuthVolumeName,
		MountPath: apicommon.AuthVolumePath,
		ReadOnly:  readOnly,
	}
}

// GetVolumeMountForLogs return the VolumeMount for the container generated logs
func GetVolumeMountForLogs() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.LogDatadogVolumeName,
		MountPath: apicommon.LogDatadogVolumePath,
		ReadOnly:  false,
	}
}

// GetVolumeForTmp return the Volume use for /tmp
func GetVolumeForTmp() corev1.Volume {
	return corev1.Volume{
		Name: apicommon.TmpVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetVolumeMountForTmp return the VolumeMount for /tmp
func GetVolumeMountForTmp() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.TmpVolumeName,
		MountPath: apicommon.TmpVolumePath,
		ReadOnly:  false,
	}
}

// GetVolumeForCertificates return the Volume use to store certificates
func GetVolumeForCertificates() corev1.Volume {
	return corev1.Volume{
		Name: apicommon.CertificatesVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

// GetVolumeMountForCertificates return the VolumeMount use to store certificates
func GetVolumeMountForCertificates() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.CertificatesVolumeName,
		MountPath: apicommon.CertificatesVolumePath,
		ReadOnly:  false,
	}
}

// GetVolumeMountForInstallInfo return the VolumeMount that contains the agent install-info file
func GetVolumeMountForInstallInfo() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.InstallInfoVolumeName,
		MountPath: apicommon.InstallInfoVolumePath,
		SubPath:   apicommon.InstallInfoVolumeSubPath,
		ReadOnly:  apicommon.InstallInfoVolumeReadOnly,
	}
}

// GetVolumeMountForProc returns the VolumeMount that contains /proc
func GetVolumeMountForProc() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.ProcdirVolumeName,
		MountPath: apicommon.ProcdirMountPath,
		ReadOnly:  true,
	}
}

// GetVolumeMountForCgroups returns the VolumeMount that contains the cgroups info
func GetVolumeMountForCgroups() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.CgroupsVolumeName,
		MountPath: apicommon.CgroupsMountPath,
		ReadOnly:  true,
	}
}

// GetVolumeMountForDogstatsdSocket returns the VolumeMount with the Dogstatsd socket
func GetVolumeMountForDogstatsdSocket(readOnly bool) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.DogstatsdSocketVolumeName,
		MountPath: apicommon.DogstatsdSocketVolumePath,
		ReadOnly:  readOnly,
	}
}

// GetVolumeForRuntimeSocket returns the Volume for the runtime socket
func GetVolumeForRuntimeSocket() corev1.Volume {
	return corev1.Volume{
		Name: apicommon.CriSocketVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: apicommon.RuntimeDirVolumePath,
			},
		},
	}
}

// GetVolumeMountForRuntimeSocket returns the VolumeMount with the runtime socket
func GetVolumeMountForRuntimeSocket(readOnly bool) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.CriSocketVolumeName,
		MountPath: apicommon.HostCriSocketPathPrefix + apicommon.RuntimeDirVolumePath,
		ReadOnly:  readOnly,
	}
}

// GetVolumeMountForSecurity returns the VolumeMount for datadog-agent-security
func GetVolumeMountForSecurity() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.SeccompSecurityVolumeName,
		MountPath: apicommon.SeccompSecurityVolumePath,
	}
}

// GetVolumeForSecurity returns the Volume for datadog-agent-security
func GetVolumeForSecurity(owner metav1.Object) corev1.Volume {
	return corev1.Volume{
		Name: apicommon.SeccompSecurityVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: GetDefaultSeccompConfigMapName(owner),
				},
			},
		},
	}
}

// GetVolumeMountForSeccomp returns the VolumeMount for seccomp root
func GetVolumeMountForSeccomp() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.SeccompRootVolumeName,
		MountPath: apicommon.SeccompRootVolumePath,
	}
}

// GetVolumeForSeccomp returns the volume for seccomp root
func GetVolumeForSeccomp() corev1.Volume {
	return corev1.Volume{
		Name: apicommon.SeccompRootVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: apicommon.SeccompRootPath,
			},
		},
	}
}

// GetClusterAgentServiceName return the Cluster-Agent service name based on the DatadogAgent name
func GetClusterAgentServiceName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), apicommon.DefaultClusterAgentResourceSuffix)
}

// GetClusterAgentName return the Cluster-Agent name based on the DatadogAgent name
func GetClusterAgentName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), apicommon.DefaultClusterAgentResourceSuffix)
}

// GetClusterAgentVersion return the Cluster-Agent version based on the DatadogAgent info
func GetClusterAgentVersion(dda metav1.Object) string {
	// Todo implement this function
	return ""
}

// GetClusterAgentSCCName returns the Cluster-Agent SCC name based on the DatadogAgent name
func GetClusterAgentSCCName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), apicommon.DefaultClusterAgentResourceSuffix)
}

// GetAgentServiceName return the Agent service name based on the DatadogAgent name
func GetAgentServiceName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), apicommon.DefaultAgentResourceSuffix)
}

// GetAgentName return the Agent name based on the DatadogAgent info
func GetAgentName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), apicommon.DefaultAgentResourceSuffix)
}

// GetAgentVersion return the Agent version based on the DatadogAgent info
func GetAgentVersion(dda metav1.Object) string {
	// TODO implement this method
	return ""
}

// GetAgentSCCName returns the Agent SCC name based on the DatadogAgent name
func GetAgentSCCName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), apicommon.DefaultAgentResourceSuffix)
}

// GetClusterChecksRunnerName return the Cluster-Checks-Runner name based on the DatadogAgent name
func GetClusterChecksRunnerName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), apicommon.DefaultClusterChecksRunnerResourceSuffix)
}

// GetDefaultSeccompConfigMapName returns the default seccomp configmap name based on the DatadogAgent name
func GetDefaultSeccompConfigMapName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), apicommon.SystemProbeAgentSecurityConfigMapSuffixName)
}

// BuildEnvVarFromSource return an *corev1.EnvVar from a Env Var name and *corev1.EnvVarSource
func BuildEnvVarFromSource(name string, source *corev1.EnvVarSource) *corev1.EnvVar {
	return &corev1.EnvVar{
		Name:      name,
		ValueFrom: source,
	}
}

// BuildEnvVarFromSecret return an corev1.EnvVarSource correspond to a secret reference
func BuildEnvVarFromSecret(name, key string) *corev1.EnvVarSource {
	return &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: name,
			},
			Key: key,
		},
	}
}

// BuildKubernetesNetworkPolicy creates the base node agent kubernetes network policy
func BuildKubernetesNetworkPolicy(dda metav1.Object, componentName v2alpha1.ComponentName) (string, string, metav1.LabelSelector, []netv1.PolicyType, []netv1.NetworkPolicyIngressRule, []netv1.NetworkPolicyEgressRule) {
	policyName, podSelector := GetNetworkPolicyMetadata(dda, componentName)
	ddaNamespace := dda.GetNamespace()

	policyTypes := []netv1.PolicyType{
		netv1.PolicyTypeIngress,
		netv1.PolicyTypeEgress,
	}

	var egress []netv1.NetworkPolicyEgressRule
	var ingress []netv1.NetworkPolicyIngressRule

	switch componentName {
	case v2alpha1.NodeAgentComponentName:
		// The agents are susceptible to connect to any pod that would
		// be annotated with auto-discovery annotations.
		//
		// When a user wants to add a check on one of its pod, they
		// need to
		// * annotate its pod
		// * add an ingress policy from the agent on its own pod
		// In order to not ask end-users to inject NetworkPolicy on the
		// agent in the agent namespace, the agent must be allowed to
		// probe any pod.
		egress = []netv1.NetworkPolicyEgressRule{
			{
				Ports: append([]netv1.NetworkPolicyPort{}, ddIntakePort()),
			},
		}
		ingress = []netv1.NetworkPolicyIngressRule{}
	case v2alpha1.ClusterAgentComponentName:
		_, nodeAgentPodSelector := GetNetworkPolicyMetadata(dda, v2alpha1.NodeAgentComponentName)
		egress = []netv1.NetworkPolicyEgressRule{
			{
				Ports: append([]netv1.NetworkPolicyPort{}, ddIntakePort()),
			},
			// Egress to other cluster agents
			{
				Ports: append([]netv1.NetworkPolicyPort{}, dcaServicePort()),
				To: []netv1.NetworkPolicyPeer{
					{
						PodSelector: &podSelector,
					},
				},
			},
		}
		ingress = []netv1.NetworkPolicyIngressRule{
			// Ingress from the node agents (for the metadata provider) and other cluster agents
			{
				Ports: append([]netv1.NetworkPolicyPort{}, dcaServicePort()),
				From: []netv1.NetworkPolicyPeer{
					{
						PodSelector: &nodeAgentPodSelector,
					},
					{
						PodSelector: &podSelector,
					},
				},
			},
			// Ingress from the node agents (for the prometheus check)
			{
				Ports: []netv1.NetworkPolicyPort{
					{
						Port: &intstr.IntOrString{
							Type:   intstr.Int,
							IntVal: 5000,
						},
					},
				},
				From: []netv1.NetworkPolicyPeer{
					{
						PodSelector: &nodeAgentPodSelector,
					},
				},
			},
		}
	case v2alpha1.ClusterChecksRunnerComponentName:
		// The cluster check runners are susceptible to connect to any service
		// that would be annotated with auto-discovery annotations.
		//
		// When a user wants to add a check on one of its service, he needs to
		// * annotate its service
		// * add an ingress policy from the CLC on its own pod
		// In order to not ask end-users to inject NetworkPolicy on the agent in
		// the agent namespace, the agent must be allowed to probe any service.
		egress = []netv1.NetworkPolicyEgressRule{
			{
				Ports: append([]netv1.NetworkPolicyPort{}, ddIntakePort(), dcaServicePort()),
				To: []netv1.NetworkPolicyPeer{
					{
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app": policyName,
							},
						},
					},
				},
			},
		}
		ingress = []netv1.NetworkPolicyIngressRule{}
	}

	return policyName, ddaNamespace, podSelector, policyTypes, ingress, egress
}

// GetNetworkPolicyMetadata generates a label selector based on component
func GetNetworkPolicyMetadata(dda metav1.Object, componentName v2alpha1.ComponentName) (policyName string, podSelector metav1.LabelSelector) {
	var suffix string
	switch componentName {
	case v2alpha1.NodeAgentComponentName:
		policyName = GetAgentName(dda)
		suffix = apicommon.DefaultAgentResourceSuffix
	case v2alpha1.ClusterAgentComponentName:
		policyName = GetClusterAgentName(dda)
		suffix = apicommon.DefaultClusterAgentResourceSuffix
	case v2alpha1.ClusterChecksRunnerComponentName:
		policyName = GetClusterChecksRunnerName(dda)
		suffix = apicommon.DefaultClusterChecksRunnerResourceSuffix
	}
	podSelector = metav1.LabelSelector{
		MatchLabels: map[string]string{
			kubernetes.AppKubernetesInstanceLabelKey: suffix,
			kubernetes.AppKubernetesPartOfLabelKey:   object.NewPartOfLabelValue(dda).String(),
		},
	}
	return policyName, podSelector
}

// datadog intake and kubeapi server port
func ddIntakePort() netv1.NetworkPolicyPort {
	return netv1.NetworkPolicyPort{
		Port: &intstr.IntOrString{
			Type:   intstr.Int,
			IntVal: 443,
		},
	}
}

// cluster agent service port
func dcaServicePort() netv1.NetworkPolicyPort {
	return netv1.NetworkPolicyPort{
		Port: &intstr.IntOrString{
			Type:   intstr.Int,
			IntVal: apicommon.DefaultClusterAgentServicePort,
		},
	}
}

// BuildAgentLocalService creates a local service for the node agent
func BuildAgentLocalService(dda metav1.Object, name string) (string, string, map[string]string, []corev1.ServicePort, *corev1.ServiceInternalTrafficPolicyType) {
	if name == "" {
		name = GetAgentServiceName(dda)
	}
	serviceInternalTrafficPolicy := corev1.ServiceInternalTrafficPolicyLocal
	selector := map[string]string{
		apicommon.AgentDeploymentNameLabelKey:      dda.GetName(),
		apicommon.AgentDeploymentComponentLabelKey: apicommon.DefaultAgentResourceSuffix,
	}
	ports := []corev1.ServicePort{
		{
			Protocol:   corev1.ProtocolUDP,
			TargetPort: intstr.FromInt(apicommon.DefaultDogstatsdPort),
			Port:       apicommon.DefaultDogstatsdPort,
			Name:       apicommon.DefaultDogstatsdPortName,
		},
	}
	return name, dda.GetNamespace(), selector, ports, &serviceInternalTrafficPolicy
}

// ShouldCreateAgentLocalService returns whether the node agent local service should be created based on the Kubernetes version
func ShouldCreateAgentLocalService(versionInfo *version.Info, forceEnableLocalService bool) bool {
	if versionInfo == nil || versionInfo.GitVersion == "" {
		return false
	}
	// Service Internal Traffic Policy exists in Kube 1.21 but it is enabled by default since 1.22
	return utils.IsAboveMinVersion(versionInfo.GitVersion, localServiceDefaultMinimumVersion) ||
		(utils.IsAboveMinVersion(versionInfo.GitVersion, localServiceMinimumVersion) && forceEnableLocalService)
}

// BuildCiliumPolicy creates the base node agent, DCA, or CCR cilium network policy
func BuildCiliumPolicy(dda metav1.Object, site string, ddURL string, hostNetwork bool, dnsSelectorEndpoints []metav1.LabelSelector, componentName v2alpha1.ComponentName) (string, string, []cilium.NetworkPolicySpec) {
	policyName, podSelector := GetNetworkPolicyMetadata(dda, componentName)
	var policySpecs []cilium.NetworkPolicySpec

	switch componentName {
	case v2alpha1.NodeAgentComponentName:
		policySpecs = []cilium.NetworkPolicySpec{
			egressECSPorts(podSelector),
			egressNTP(podSelector),
			egressMetadataServerRule(podSelector),
			egressDNS(podSelector, dnsSelectorEndpoints),
			egressAgentDatadogIntake(podSelector, site, ddURL),
			egressKubelet(podSelector),
			ingressDogstatsd(podSelector),
			egressChecks(podSelector),
		}
	case v2alpha1.ClusterAgentComponentName:
		_, nodeAgentPodSelector := GetNetworkPolicyMetadata(dda, v2alpha1.NodeAgentComponentName)
		policySpecs = []cilium.NetworkPolicySpec{
			egressMetadataServerRule(podSelector),
			egressDNS(podSelector, dnsSelectorEndpoints),
			egressDCADatadogIntake(podSelector, site, ddURL),
			egressKubeAPIServer(),
			ingressAgent(podSelector, dda, hostNetwork),
			ingressDCA(podSelector, nodeAgentPodSelector),
			egressDCA(podSelector, nodeAgentPodSelector),
		}
	case v2alpha1.ClusterChecksRunnerComponentName:
		policySpecs = []cilium.NetworkPolicySpec{
			egressMetadataServerRule(podSelector),
			egressDNS(podSelector, dnsSelectorEndpoints),
			egressCCRDatadogIntake(podSelector, site, ddURL),
			egressCCRToDCA(podSelector, dda),
			egressChecks(podSelector),
		}
	}
	return policyName, dda.GetNamespace(), policySpecs
}

// cilium egress ports for ECS
func egressECSPorts(podSelector metav1.LabelSelector) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Egress to ECS agent port 51678",
		EndpointSelector: podSelector,
		Egress: []cilium.EgressRule{
			{
				ToEntities: []cilium.Entity{cilium.EntityHost},
				ToPorts: []cilium.PortRule{
					{
						Ports: []cilium.PortProtocol{
							{
								Port:     "51678",
								Protocol: cilium.ProtocolTCP,
							},
						},
					},
				},
			},
			{
				ToCIDR: []string{"169.254.0.0/16"},
				ToPorts: []cilium.PortRule{
					{
						Ports: []cilium.PortProtocol{
							{
								Port:     "51678",
								Protocol: cilium.ProtocolTCP,
							},
						},
					},
				},
			},
		},
	}
}

// cilium NTP egress
func egressNTP(podSelector metav1.LabelSelector) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Egress to ntp",
		EndpointSelector: podSelector,
		Egress: []cilium.EgressRule{
			{
				ToPorts: []cilium.PortRule{
					{
						Ports: []cilium.PortProtocol{
							{
								Port:     "123",
								Protocol: cilium.ProtocolUDP,
							},
						},
					},
				},
				ToFQDNs: []cilium.FQDNSelector{
					{
						MatchPattern: "*.datadog.pool.ntp.org",
					},
				},
			},
		},
	}
}

// cilium egress for agent intake endpoints
func egressAgentDatadogIntake(podSelector metav1.LabelSelector, site string, ddURL string) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Egress to Datadog intake",
		EndpointSelector: podSelector,
		Egress: []cilium.EgressRule{
			{
				ToFQDNs: append(defaultDDFQDNs(site, ddURL), []cilium.FQDNSelector{
					{
						MatchName: fmt.Sprintf("api.%s", site),
					},
					{
						MatchName: fmt.Sprintf("agent-intake.logs.%s", site),
					},
					{
						MatchName: fmt.Sprintf("agent-http-intake.logs.%s", site),
					},
					{
						MatchName: fmt.Sprintf("process.%s", site),
					},
					{
						MatchName: fmt.Sprintf("orchestrator.%s", site),
					},
				}...),
				ToPorts: []cilium.PortRule{
					{
						Ports: []cilium.PortProtocol{
							{
								Port:     "443",
								Protocol: cilium.ProtocolTCP,
							},
							{
								Port:     "10516",
								Protocol: cilium.ProtocolTCP,
							},
						},
					},
				},
			},
		},
	}
}

// cilium egress for DCA intake endpoints
func egressDCADatadogIntake(podSelector metav1.LabelSelector, site string, ddURL string) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Egress to Datadog intake",
		EndpointSelector: podSelector,
		Egress: []cilium.EgressRule{
			{
				ToFQDNs: append(defaultDDFQDNs(site, ddURL), cilium.FQDNSelector{MatchName: fmt.Sprintf("orchestrator.%s", site)}),
				ToPorts: []cilium.PortRule{
					{
						Ports: []cilium.PortProtocol{
							{
								Port:     "443",
								Protocol: cilium.ProtocolTCP,
							},
						},
					},
				},
			},
		},
	}
}

// cilium egress for CCR intake endpoints
func egressCCRDatadogIntake(podSelector metav1.LabelSelector, site string, ddURL string) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Egress to Datadog intake",
		EndpointSelector: podSelector,
		Egress: []cilium.EgressRule{
			{
				ToFQDNs: defaultDDFQDNs(site, ddURL),
				ToPorts: []cilium.PortRule{
					{
						Ports: []cilium.PortProtocol{
							{
								Port:     "443",
								Protocol: cilium.ProtocolTCP,
							},
						},
					},
				},
			},
		},
	}
}

// cilium egress to kubelet
func egressKubelet(podSelector metav1.LabelSelector) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Egress to kubelet",
		EndpointSelector: podSelector,
		Egress: []cilium.EgressRule{
			{
				ToEntities: []cilium.Entity{
					cilium.EntityHost,
				},
				ToPorts: []cilium.PortRule{
					{
						Ports: []cilium.PortProtocol{
							{
								Port:     "10250",
								Protocol: cilium.ProtocolTCP,
							},
						},
					},
				},
			},
		},
	}
}

// cilium ingress for dogstatsd
func ingressDogstatsd(podSelector metav1.LabelSelector) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Ingress for dogstatsd",
		EndpointSelector: podSelector,
		Ingress: []cilium.IngressRule{
			{
				FromEndpoints: []metav1.LabelSelector{
					{},
				},
				ToPorts: []cilium.PortRule{
					{
						Ports: []cilium.PortProtocol{
							{
								Port:     strconv.Itoa(apicommon.DefaultDogstatsdPort),
								Protocol: cilium.ProtocolUDP,
							},
						},
					},
				},
			},
		},
	}
}

// cilium egress to metadata server for cloud providers
func egressMetadataServerRule(podSelector metav1.LabelSelector) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Egress to metadata server",
		EndpointSelector: podSelector,
		Egress: []cilium.EgressRule{
			{
				ToCIDR: []string{"169.254.169.254/32"},
				ToPorts: []cilium.PortRule{
					{
						Ports: []cilium.PortProtocol{
							{
								Port:     "80",
								Protocol: cilium.ProtocolTCP,
							},
						},
					},
				},
			},
		},
	}
}

// cilium egress to dns endpoints
func egressDNS(podSelector metav1.LabelSelector, dnsSelectorEndpoints []metav1.LabelSelector) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Egress to DNS",
		EndpointSelector: podSelector,
		Egress: []cilium.EgressRule{
			{
				ToEndpoints: dnsSelectorEndpoints,
				ToPorts: []cilium.PortRule{
					{
						Ports: []cilium.PortProtocol{
							{
								Port:     "53",
								Protocol: cilium.ProtocolAny,
							},
						},
						Rules: &cilium.L7Rules{
							DNS: []cilium.FQDNSelector{
								{
									MatchPattern: "*",
								},
							},
						},
					},
				},
			},
		},
	}
}

// The agents are susceptible to connect to any pod that would be annotated
// with auto-discovery annotations.
//
// When a user wants to add a check on one of its pod, he needs to
// * annotate its pod
// * add an ingress policy from the agent on its own pod
//
// In order to not ask end-users to inject NetworkPolicy on the agent in the
// agent namespace, the agent must be allowed to probe any pod.
func egressChecks(podSelector metav1.LabelSelector) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Egress to anything for checks",
		EndpointSelector: podSelector,
		Egress: []cilium.EgressRule{
			{
				ToEndpoints: []metav1.LabelSelector{
					{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "k8s:io.kubernetes.pod.namespace",
								Operator: "Exists",
							},
						},
					},
				},
			},
		},
	}
}

// cilium egress to kube api server
func egressKubeAPIServer() cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description: "Egress to Kube API Server",
		Egress: []cilium.EgressRule{
			{
				// ToServices works only for endpoints
				// outside of the cluster This section
				// handles the case where the control
				// plane is outside of the cluster.
				ToServices: []cilium.Service{
					{
						K8sService: &cilium.K8sServiceNamespace{
							Namespace:   "default",
							ServiceName: "kubernetes",
						},
					},
				},
				ToEntities: []cilium.Entity{
					cilium.EntityHost,
					cilium.EntityRemoteNode,
				},
				ToPorts: []cilium.PortRule{
					{
						Ports: []cilium.PortProtocol{
							{
								Port:     "443",
								Protocol: cilium.ProtocolTCP,
							},
						},
					},
				},
			},
		},
	}
}

// cilium egress for CCR to DCA
func egressCCRToDCA(podSelector metav1.LabelSelector, dda metav1.Object) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Egress to cluster agent",
		EndpointSelector: podSelector,
		Egress: []cilium.EgressRule{
			{
				ToPorts: []cilium.PortRule{
					{
						Ports: []cilium.PortProtocol{
							{
								Port:     "5005",
								Protocol: cilium.ProtocolTCP,
							},
						},
					},
				},
				ToEndpoints: []metav1.LabelSelector{
					{
						MatchLabels: map[string]string{
							kubernetes.AppKubernetesInstanceLabelKey: apicommon.DefaultClusterAgentResourceSuffix,
							kubernetes.AppKubernetesPartOfLabelKey:   fmt.Sprintf("%s-%s", dda.GetNamespace(), dda.GetName()),
						},
					},
				},
			},
		},
	}
}

func defaultDDFQDNs(site, ddURL string) []cilium.FQDNSelector {
	selectors := []cilium.FQDNSelector{}
	if ddURL != "" {
		selectors = append(selectors, cilium.FQDNSelector{
			MatchName: strings.TrimPrefix(ddURL, "https://"),
		})
	}

	selectors = append(selectors, []cilium.FQDNSelector{
		{
			MatchPattern: fmt.Sprintf("*-app.agent.%s", site),
		},
	}...)

	return selectors
}

// cilium ingress from agent
func ingressAgent(podSelector metav1.LabelSelector, dda metav1.Object, hostNetwork bool) cilium.NetworkPolicySpec {
	ingress := cilium.IngressRule{
		ToPorts: []cilium.PortRule{
			{
				Ports: []cilium.PortProtocol{
					{
						Port:     "5000",
						Protocol: cilium.ProtocolTCP,
					},
					{
						Port:     "5005",
						Protocol: cilium.ProtocolTCP,
					},
				},
			},
		},
	}

	if hostNetwork {
		ingress.FromEntities = []cilium.Entity{
			cilium.EntityHost,
			cilium.EntityRemoteNode,
		}
	} else {
		ingress.FromEndpoints = []metav1.LabelSelector{
			{
				MatchLabels: map[string]string{
					kubernetes.AppKubernetesInstanceLabelKey: GetAgentName(dda),
					kubernetes.AppKubernetesPartOfLabelKey:   fmt.Sprintf("%s-%s", dda.GetNamespace(), dda.GetName()),
				},
			},
		}
	}

	return cilium.NetworkPolicySpec{
		Description:      "Ingress from agent",
		EndpointSelector: podSelector,
		Ingress:          []cilium.IngressRule{ingress},
	}
}

// cilium ingress from DCA
func ingressDCA(podSelector metav1.LabelSelector, nodeAgentPodSelector metav1.LabelSelector) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Ingress from cluster agent",
		EndpointSelector: podSelector,
		Ingress: []cilium.IngressRule{
			{
				FromEndpoints: []metav1.LabelSelector{
					nodeAgentPodSelector,
				},
				ToPorts: []cilium.PortRule{
					{
						Ports: []cilium.PortProtocol{
							{
								Port:     "5005",
								Protocol: cilium.ProtocolTCP,
							},
						},
					},
				},
			},
		},
	}
}

// cilium egress to DCA
func egressDCA(podSelector metav1.LabelSelector, nodeAgentPodSelector metav1.LabelSelector) cilium.NetworkPolicySpec {
	return cilium.NetworkPolicySpec{
		Description:      "Egress to cluster agent",
		EndpointSelector: podSelector,
		Egress: []cilium.EgressRule{
			{
				ToEndpoints: []metav1.LabelSelector{
					nodeAgentPodSelector,
				},
				ToPorts: []cilium.PortRule{
					{
						Ports: []cilium.PortProtocol{
							{
								Port:     "5005",
								Protocol: cilium.ProtocolTCP,
							},
						},
					},
				},
			},
		},
	}
}
