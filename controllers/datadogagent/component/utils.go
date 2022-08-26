// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package component

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/version"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object"
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

// GetVolumeForRmCorechecks return the volume that contains the agent confd config files
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

// GetVolumeMountForRmCorechecks return the VolumeMount that contains the agent confd config files
func GetVolumeMountForRmCorechecks() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      "remove-corechecks",
		MountPath: fmt.Sprintf("%s/%s", apicommon.ConfigVolumePath, "conf.d"),
	}
}

// GetVolumeMountForAuth returns the VolumeMount that contains the authentication information
func GetVolumeMountForAuth() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      apicommon.AuthVolumeName,
		MountPath: apicommon.AuthVolumePath,
		ReadOnly:  true,
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

// GetClusterChecksRunnerName return the Cluster-Checks-Runner name based on the DatadogAgent name
func GetClusterChecksRunnerName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), apicommon.DefaultClusterChecksRunnerResourceSuffix)
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
