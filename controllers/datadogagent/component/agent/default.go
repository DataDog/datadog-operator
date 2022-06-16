// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"fmt"
	"strconv"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component"
	componentdca "github.com/DataDog/datadog-operator/controllers/datadogagent/component/clusteragent"
	"github.com/DataDog/datadog-operator/pkg/defaulting"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
	edsv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	rbacv1 "k8s.io/api/rbac/v1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewDefaultAgentDaemonset return a new default agent DaemonSet
func NewDefaultAgentDaemonset(dda metav1.Object, requiredContainers []common.AgentContainerName) *appsv1.DaemonSet {
	daemonset := component.NewDaemonset(dda, apicommon.DefaultAgentResourceSuffix, component.GetAgentName(dda), component.GetAgentVersion(dda), nil)
	podTemplate := NewDefaultAgentPodTemplateSpec(dda, requiredContainers, daemonset.GetLabels())

	daemonset.Spec.Template = *podTemplate
	return daemonset
}

// NewDefaultAgentExtendedDaemonset return a new default agent DaemonSet
func NewDefaultAgentExtendedDaemonset(dda metav1.Object, requiredContainers []common.AgentContainerName) *edsv1alpha1.ExtendedDaemonSet {
	edsDaemonset := component.NewExtendedDaemonset(dda, apicommon.DefaultAgentResourceSuffix, component.GetAgentName(dda), component.GetAgentVersion(dda), nil)
	edsDaemonset.Spec.Template = *NewDefaultAgentPodTemplateSpec(dda, requiredContainers, edsDaemonset.GetLabels())
	return edsDaemonset
}

// NewDefaultAgentPodTemplateSpec return a default node agent for the cluster-agent deployment
func NewDefaultAgentPodTemplateSpec(dda metav1.Object, requiredContainers []common.AgentContainerName, labels map[string]string) *corev1.PodTemplateSpec {
	return &corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      labels,
			Annotations: make(map[string]string),
		},
		Spec: corev1.PodSpec{
			// Force root user for when the agent Dockerfile will be updated to use a non-root user by default
			SecurityContext: &corev1.PodSecurityContext{
				RunAsUser: apiutils.NewInt64Pointer(0),
			},
			ServiceAccountName: getDefaultServiceAccountName(dda),
			InitContainers: []corev1.Container{
				initVolumeContainer(),
				initConfigContainer(dda),
			},
			Containers: agentContainers(dda, requiredContainers),
			Volumes:    volumesForAgent(dda),
		},
	}
}

func getDefaultServiceAccountName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), apicommon.DefaultAgentResourceSuffix)
}

func agentImage() string {
	return fmt.Sprintf("%s/%s:%s", apicommon.DefaultImageRegistry, apicommon.DefaultAgentImageName, defaulting.AgentLatestVersion)
}

func agentContainers(dda metav1.Object, requiredContainers []common.AgentContainerName) []corev1.Container {
	containers := []corev1.Container{coreAgentContainer(dda)}

	for _, containerName := range requiredContainers {
		switch containerName {
		case common.CoreAgentContainerName:
			// Nothing to do. It's always required.
		case common.TraceAgentContainerName:
			containers = append(containers, traceAgentContainer(dda))
		case common.ProcessAgentContainerName:
			containers = append(containers, processAgentContainer(dda))
		case common.SecurityAgentContainerName:
			containers = append(containers, securityAgentContainer(dda))
		case common.SystemProbeContainerName:
			containers = append(containers, systemProbeContainer(dda))
		}
	}

	return containers
}

func coreAgentContainer(dda metav1.Object) corev1.Container {
	return corev1.Container{
		Name:           string(common.CoreAgentContainerName),
		Image:          agentImage(),
		Command:        []string{"agent", "run"},
		Env:            envVarsForCoreAgent(dda),
		VolumeMounts:   volumeMountsForCoreAgent(),
		LivenessProbe:  apicommon.GetDefaultLivenessProbe(),
		ReadinessProbe: apicommon.GetDefaultReadinessProbe(),
	}
}

func traceAgentContainer(dda metav1.Object) corev1.Container {
	return corev1.Container{
		Name:  string(common.TraceAgentContainerName),
		Image: agentImage(),
		Command: []string{
			"trace-agent",
			fmt.Sprintf("--config=%s", apicommon.AgentCustomConfigVolumePath),
		},
		Env:            commonEnvVars(dda),
		VolumeMounts:   volumeMountsForTraceAgent(),
		LivenessProbe:  apicommon.GetDefaultLivenessProbe(),
		ReadinessProbe: apicommon.GetDefaultReadinessProbe(),
	}
}

func processAgentContainer(dda metav1.Object) corev1.Container {
	return corev1.Container{
		Name:  string(common.ProcessAgentContainerName),
		Image: agentImage(),
		Command: []string{
			"process-agent", fmt.Sprintf("--config=%s", apicommon.AgentCustomConfigVolumePath),
			fmt.Sprintf("--sysprobe-config=%s", apicommon.SystemProbeConfigVolumePath),
		},
		Env:            commonEnvVars(dda),
		VolumeMounts:   volumeMountsForProcessAgent(),
		LivenessProbe:  apicommon.GetDefaultLivenessProbe(),
		ReadinessProbe: apicommon.GetDefaultReadinessProbe(),
	}
}

func securityAgentContainer(dda metav1.Object) corev1.Container {
	return corev1.Container{
		Name:  string(common.SecurityAgentContainerName),
		Image: agentImage(),
		Command: []string{
			"security-agent",
			"start", fmt.Sprintf("-c=%s", apicommon.AgentCustomConfigVolumePath),
		},
		Env:            envVarsForSecurityAgent(dda),
		VolumeMounts:   volumeMountsForSecurityAgent(),
		LivenessProbe:  apicommon.GetDefaultLivenessProbe(),
		ReadinessProbe: apicommon.GetDefaultReadinessProbe(),
	}
}

func systemProbeContainer(dda metav1.Object) corev1.Container {
	return corev1.Container{
		Name:  string(common.SystemProbeContainerName),
		Image: agentImage(),
		Command: []string{
			"system-probe",
			fmt.Sprintf("--config=%s", apicommon.SystemProbeConfigVolumePath),
		},
		Env:            commonEnvVars(dda),
		VolumeMounts:   volumeMountsForSystemProbe(),
		LivenessProbe:  apicommon.GetDefaultLivenessProbe(),
		ReadinessProbe: apicommon.GetDefaultReadinessProbe(),
	}
}

func initVolumeContainer() corev1.Container {
	return corev1.Container{
		Name:    "init-volume",
		Image:   agentImage(),
		Command: []string{"bash", "-c"},
		Args:    []string{"cp -vnr /etc/datadog-agent /opt"},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      apicommon.ConfigVolumeName,
				MountPath: "/opt/datadog-agent",
			},
		},
	}
}

func initConfigContainer(dda metav1.Object) corev1.Container {
	return corev1.Container{
		Name:    "init-config",
		Image:   agentImage(),
		Command: []string{"bash", "-c"},
		Args: []string{
			"for script in $(find /etc/cont-init.d/ -type f -name '*.sh' | sort) ; do bash $script ; done",
		},
		VolumeMounts: volumeMountsForCoreAgent(),
		Env:          envVarsForCoreAgent(dda),
	}
}

func commonEnvVars(dda metav1.Object) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  apicommon.KubernetesEnvVar,
			Value: "yes",
		},
		{
			Name:  apicommon.DDClusterAgentEnabled,
			Value: strconv.FormatBool(true),
		},
		{
			Name:  apicommon.DDClusterAgentKubeServiceName,
			Value: componentdca.GetClusterAgentServiceName(dda),
		},
		{
			Name:  apicommon.DDClusterAgentTokenName,
			Value: v2alpha1.GetDefaultDCATokenSecretName(dda),
		},
		{
			Name: apicommon.DDKubeletHost,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: apicommon.FieldPathStatusHostIP,
				},
			},
		},
	}
}

func envVarsForCoreAgent(dda metav1.Object) []corev1.EnvVar {
	envs := []corev1.EnvVar{
		{
			Name:  apicommon.DDHealthPort,
			Value: strconv.Itoa(int(apicommon.DefaultAgentHealthPort)),
		},
		{
			Name:  apicommon.DDLeaderElection,
			Value: "true",
		},
	}

	return append(envs, commonEnvVars(dda)...)
}

func envVarsForSecurityAgent(dda metav1.Object) []corev1.EnvVar {
	envs := []corev1.EnvVar{
		{
			Name:  "HOST_ROOT",
			Value: apicommon.HostRootMountPath,
		},
	}

	return append(envs, commonEnvVars(dda)...)
}

func volumesForAgent(dda metav1.Object) []corev1.Volume {
	return []corev1.Volume{
		component.GetVolumeForLogs(),
		component.GetVolumeForAuth(),
		component.GetVolumeInstallInfo(dda),
		component.GetVolumeForConfd(),
		component.GetVolumeForConfig(),
		component.GetVolumeForProc(),
		component.GetVolumeForCgroups(),
		component.GetVolumeForDogstatsd(),
	}
}

func volumeMountsForCoreAgent() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		component.GetVolumeMountForLogs(),
		component.GetVolumeMountForAuth(),
		component.GetVolumeMountForInstallInfo(),
		component.GetVolumeMountForConfd(),
		component.GetVolumeMountForConfig(),
		component.GetVolumeMountForProc(),
		component.GetVolumeMountForCgroups(),
		component.GetVolumeMountForDogstatsdSocket(false),
	}
}

func volumeMountsForTraceAgent() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		component.GetVolumeMountForLogs(),
		component.GetVolumeMountForAuth(),
		component.GetVolumeMountForConfig(),
		component.GetVolumeMountForDogstatsdSocket(true),
	}
}

func volumeMountsForProcessAgent() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		component.GetVolumeMountForLogs(),
		component.GetVolumeMountForAuth(),
		component.GetVolumeMountForConfig(),
		component.GetVolumeMountForDogstatsdSocket(true),
	}
}

func volumeMountsForSecurityAgent() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		component.GetVolumeMountForLogs(),
		component.GetVolumeMountForAuth(),
		component.GetVolumeMountForConfig(),
		component.GetVolumeMountForDogstatsdSocket(true),
	}
}

func volumeMountsForSystemProbe() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		component.GetVolumeMountForLogs(),
		component.GetVolumeMountForAuth(),
	}
}

// GetDefaultAgentClusterRolePolicyRules returns the default policy rules for
// the Agent
func GetDefaultAgentClusterRolePolicyRules() []rbacv1.PolicyRule {
	// TODO (operator-ga):: this only adds the policy for the Kubelet. Check if we need others.
	return []rbacv1.PolicyRule{GetKubeletPolicyRule()}
}

// GetKubeletPolicyRule returns the policy rule for Kubelet
func GetKubeletPolicyRule() rbacv1.PolicyRule {
	return rbacv1.PolicyRule{
		APIGroups: []string{rbac.CoreAPIGroup},
		Resources: []string{
			rbac.NodeMetricsResource,
			rbac.NodeSpecResource,
			rbac.NodeProxyResource,
			rbac.NodeStats,
		},
		Verbs: []string{rbac.GetVerb},
	}
}

// GetAgentRbacResourcesName return the Cluster-Agent RBAC resource name
func GetAgentRbacResourcesName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), apicommon.DefaultAgentResourceSuffix)
}
