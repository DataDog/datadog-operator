// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"fmt"
	"strconv"

	edsv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	componentdca "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusteragent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/defaulting"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewDefaultAgentDaemonset return a new default agent DaemonSet
func NewDefaultAgentDaemonset(dda metav1.Object, edsOptions *ExtendedDaemonsetOptions, agentComponent feature.RequiredComponent) *appsv1.DaemonSet {
	daemonset := NewDaemonset(dda, edsOptions, v2alpha1.DefaultAgentResourceSuffix, GetAgentName(dda), common.GetAgentVersion(dda), nil)
	podTemplate := NewDefaultAgentPodTemplateSpec(dda, agentComponent, daemonset.GetLabels())
	daemonset.Spec.Template = *podTemplate
	return daemonset
}

// NewDefaultAgentExtendedDaemonset return a new default agent DaemonSet
func NewDefaultAgentExtendedDaemonset(dda metav1.Object, edsOptions *ExtendedDaemonsetOptions, agentComponent feature.RequiredComponent) *edsv1alpha1.ExtendedDaemonSet {
	edsDaemonset := NewExtendedDaemonset(dda, edsOptions, v2alpha1.DefaultAgentResourceSuffix, GetAgentName(dda), common.GetAgentVersion(dda), nil)
	edsDaemonset.Spec.Template = *NewDefaultAgentPodTemplateSpec(dda, agentComponent, edsDaemonset.GetLabels())
	return edsDaemonset
}

// NewDefaultAgentPodTemplateSpec returns a defaulted node agent PodTemplateSpec with a single multi-process container or multiple single-process containers
func NewDefaultAgentPodTemplateSpec(dda metav1.Object, agentComponent feature.RequiredComponent, labels map[string]string) *corev1.PodTemplateSpec {
	requiredContainers := agentComponent.Containers

	var agentContainers []corev1.Container
	if agentComponent.SingleContainerStrategyEnabled() {
		agentContainers = agentSingleContainer(dda)
	} else {
		agentContainers = agentOptimizedContainers(dda, requiredContainers)
	}

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
			InitContainers:     initContainers(dda, requiredContainers),
			Containers:         agentContainers,
			Volumes:            volumesForAgent(dda, requiredContainers),
		},
	}
}

// DefaultCapabilitiesForSystemProbe returns the default Security Context
// Capabilities for the System Probe container
func DefaultCapabilitiesForSystemProbe() []corev1.Capability {
	return []corev1.Capability{
		"SYS_ADMIN",
		"SYS_RESOURCE",
		"SYS_PTRACE",
		"NET_ADMIN",
		"NET_BROADCAST",
		"NET_RAW",
		"IPC_LOCK",
		"CHOWN",
		"DAC_READ_SEARCH",
	}
}

// GetAgentName return the Agent name based on the DatadogAgent info
func GetAgentName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), v2alpha1.DefaultAgentResourceSuffix)
}

// GetAgentRoleName returns the name of the role for the Agent
func GetAgentRoleName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), v2alpha1.DefaultAgentResourceSuffix)
}

func getDefaultServiceAccountName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), v2alpha1.DefaultAgentResourceSuffix)
}

func agentImage() string {
	return fmt.Sprintf("%s/%s:%s", v2alpha1.DefaultImageRegistry, v2alpha1.DefaultAgentImageName, defaulting.AgentLatestVersion)
}

func otelAgentImage() string {
	// todo(mackjmr): make this dynamic once we have otel agent image which releases with regular agent.
	return fmt.Sprintf("%s:%s", defaulting.AgentDevImageName, defaulting.OTelAgentNightlyTag)

}

func initContainers(dda metav1.Object, requiredContainers []apicommon.AgentContainerName) []corev1.Container {
	initContainers := []corev1.Container{
		initVolumeContainer(),
		initConfigContainer(dda),
	}
	for _, containerName := range requiredContainers {
		if containerName == apicommon.SystemProbeContainerName {
			initContainers = append(initContainers, initSeccompSetupContainer())
		}
	}

	return initContainers
}

func agentSingleContainer(dda metav1.Object) []corev1.Container {
	agentSingleContainer := corev1.Container{
		Name:           string(apicommon.UnprivilegedSingleAgentContainerName),
		Image:          agentImage(),
		Env:            envVarsForCoreAgent(dda),
		VolumeMounts:   volumeMountsForCoreAgent(),
		LivenessProbe:  v2alpha1.GetDefaultLivenessProbe(),
		ReadinessProbe: v2alpha1.GetDefaultReadinessProbe(),
		StartupProbe:   v2alpha1.GetDefaultStartupProbe(),
	}

	containers := []corev1.Container{
		agentSingleContainer,
	}

	return containers
}

func agentOptimizedContainers(dda metav1.Object, requiredContainers []apicommon.AgentContainerName) []corev1.Container {
	containers := []corev1.Container{coreAgentContainer(dda)}

	for _, containerName := range requiredContainers {
		switch containerName {
		case apicommon.CoreAgentContainerName:
			// Nothing to do. It's always required.
		case apicommon.TraceAgentContainerName:
			containers = append(containers, traceAgentContainer(dda))
		case apicommon.ProcessAgentContainerName:
			containers = append(containers, processAgentContainer(dda))
		case apicommon.SecurityAgentContainerName:
			containers = append(containers, securityAgentContainer(dda))
		case apicommon.SystemProbeContainerName:
			containers = append(containers, systemProbeContainer(dda))
		case apicommon.OtelAgent:
			containers = append(containers, otelAgentContainer(dda))
		case apicommon.AgentDataPlaneContainerName:
			containers = append(containers, agentDataPlaneContainer(dda))
		}
	}

	return containers
}

func coreAgentContainer(dda metav1.Object) corev1.Container {
	return corev1.Container{
		Name:           string(apicommon.CoreAgentContainerName),
		Image:          agentImage(),
		Command:        []string{"agent", "run"},
		Env:            envVarsForCoreAgent(dda),
		VolumeMounts:   volumeMountsForCoreAgent(),
		LivenessProbe:  v2alpha1.GetDefaultLivenessProbe(),
		ReadinessProbe: v2alpha1.GetDefaultReadinessProbe(),
		StartupProbe:   v2alpha1.GetDefaultStartupProbe(),
	}
}

func traceAgentContainer(dda metav1.Object) corev1.Container {
	return corev1.Container{
		Name:  string(apicommon.TraceAgentContainerName),
		Image: agentImage(),
		Command: []string{
			"trace-agent",
			fmt.Sprintf("--config=%s", v2alpha1.AgentCustomConfigVolumePath),
		},
		Env:           envVarsForTraceAgent(dda),
		VolumeMounts:  volumeMountsForTraceAgent(),
		LivenessProbe: v2alpha1.GetDefaultTraceAgentProbe(),
	}
}

func processAgentContainer(dda metav1.Object) corev1.Container {
	return corev1.Container{
		Name:  string(apicommon.ProcessAgentContainerName),
		Image: agentImage(),
		Command: []string{
			"process-agent", fmt.Sprintf("--config=%s", v2alpha1.AgentCustomConfigVolumePath),
			fmt.Sprintf("--sysprobe-config=%s", v2alpha1.SystemProbeConfigVolumePath),
		},
		Env:          commonEnvVars(dda),
		VolumeMounts: volumeMountsForProcessAgent(),
	}
}

func otelAgentContainer(_ metav1.Object) corev1.Container {
	return corev1.Container{
		Name:  string(apicommon.OtelAgent),
		Image: otelAgentImage(),
		Command: []string{
			"otel-agent",
			"--config=" + v2alpha1.OtelCustomConfigVolumePath,
			"--core-config=" + v2alpha1.AgentCustomConfigVolumePath,
			"--sync-delay=10s",
		},
		Env:          []corev1.EnvVar{},
		VolumeMounts: volumeMountsForOtelAgent(),
		// todo(mackjmr): remove once support for annotations is removed.
		// the otel-agent feature adds these ports if none are supplied by
		// the user.
		Ports: []corev1.ContainerPort{
			{
				Name:          "otel-grpc",
				ContainerPort: 4317,
				HostPort:      4317,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "otel-http",
				ContainerPort: 4318,
				HostPort:      4318,
				Protocol:      corev1.ProtocolTCP,
			},
		},
	}
}

func securityAgentContainer(dda metav1.Object) corev1.Container {
	return corev1.Container{
		Name:  string(apicommon.SecurityAgentContainerName),
		Image: agentImage(),
		Command: []string{
			"security-agent",
			"start", fmt.Sprintf("-c=%s", v2alpha1.AgentCustomConfigVolumePath),
		},
		Env:          envVarsForSecurityAgent(dda),
		VolumeMounts: volumeMountsForSecurityAgent(),
	}
}

func systemProbeContainer(dda metav1.Object) corev1.Container {
	return corev1.Container{
		Name:  string(apicommon.SystemProbeContainerName),
		Image: agentImage(),
		Command: []string{
			"system-probe",
			fmt.Sprintf("--config=%s", v2alpha1.SystemProbeConfigVolumePath),
		},
		Env:          commonEnvVars(dda),
		VolumeMounts: volumeMountsForSystemProbe(),
		SecurityContext: &corev1.SecurityContext{
			SeccompProfile: &corev1.SeccompProfile{
				Type:             corev1.SeccompProfileTypeLocalhost,
				LocalhostProfile: apiutils.NewStringPointer(v2alpha1.SystemProbeSeccompProfileName),
			},
		},
	}
}

func agentDataPlaneContainer(dda metav1.Object) corev1.Container {
	return corev1.Container{
		Name:  string(apicommon.AgentDataPlaneContainerName),
		Image: agentImage(),
		Command: []string{
			"agent-data-plane",
			"run",
			fmt.Sprintf("--config=%s", v2alpha1.AgentCustomConfigVolumePath),
		},
		Env:            commonEnvVars(dda),
		VolumeMounts:   volumeMountsForAgentDataPlane(),
		LivenessProbe:  v2alpha1.GetDefaultAgentDataPlaneLivenessProbe(),
		ReadinessProbe: v2alpha1.GetDefaultAgentDataPlaneReadinessProbe(),
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
				Name:      v2alpha1.ConfigVolumeName,
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
		VolumeMounts: volumeMountsForInitConfig(),
		Env:          envVarsForCoreAgent(dda),
	}
}

func initSeccompSetupContainer() corev1.Container {
	return corev1.Container{
		Name:  "seccomp-setup",
		Image: agentImage(),
		Command: []string{
			"cp",
			fmt.Sprintf("%s/%s", v2alpha1.SeccompSecurityVolumePath, v2alpha1.SystemProbeSeccompKey),
			fmt.Sprintf("%s/%s", v2alpha1.SeccompRootVolumePath, v2alpha1.SystemProbeSeccompProfileName),
		},
		VolumeMounts: volumeMountsForSeccompSetup(),
	}
}

func commonEnvVars(dda metav1.Object) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  v2alpha1.KubernetesEnvVar,
			Value: "yes",
		},
		{
			Name:  v2alpha1.DDClusterAgentEnabled,
			Value: strconv.FormatBool(true),
		},
		{
			Name:  v2alpha1.DDClusterAgentKubeServiceName,
			Value: componentdca.GetClusterAgentServiceName(dda),
		},
		{
			Name:  v2alpha1.DDClusterAgentTokenName,
			Value: v2alpha1.GetDefaultDCATokenSecretName(dda),
		},
		{
			Name: v2alpha1.DDKubeletHost,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: v2alpha1.FieldPathStatusHostIP,
				},
			},
		},
	}
}

func envVarsForCoreAgent(dda metav1.Object) []corev1.EnvVar {
	envs := []corev1.EnvVar{
		{
			Name:  v2alpha1.DDHealthPort,
			Value: strconv.Itoa(int(v2alpha1.DefaultAgentHealthPort)),
		},
		{
			Name:  v2alpha1.DDLeaderElection,
			Value: "true",
		},
		{
			// we want to default it in 7.49.0
			// but in 7.50.0 it will be already defaulted in the agent process.
			Name:  v2alpha1.DDContainerImageEnabled,
			Value: "true",
		},
	}

	return append(envs, commonEnvVars(dda)...)
}

func envVarsForTraceAgent(dda metav1.Object) []corev1.EnvVar {
	envs := []corev1.EnvVar{
		{
			Name:  v2alpha1.DDAPMInstrumentationInstallId,
			Value: utils.GetDatadogAgentResourceUID(dda),
		},
		{
			Name:  v2alpha1.DDAPMInstrumentationInstallTime,
			Value: utils.GetDatadogAgentResourceCreationTime(dda),
		},
		{
			Name:  v2alpha1.DDAPMInstrumentationInstallType,
			Value: common.DefaultAgentInstallType,
		},
	}

	return append(envs, commonEnvVars(dda)...)
}

func envVarsForSecurityAgent(dda metav1.Object) []corev1.EnvVar {
	envs := []corev1.EnvVar{
		{
			Name:  "HOST_ROOT",
			Value: v2alpha1.HostRootMountPath,
		},
	}

	return append(envs, commonEnvVars(dda)...)
}

func volumeMountsForInitConfig() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		common.GetVolumeMountForLogs(),
		common.GetVolumeMountForChecksd(),
		common.GetVolumeMountForAuth(false),
		common.GetVolumeMountForConfd(),
		common.GetVolumeMountForConfig(),
		common.GetVolumeMountForProc(),
		common.GetVolumeMountForRuntimeSocket(true),
	}
}

func volumesForAgent(dda metav1.Object, requiredContainers []apicommon.AgentContainerName) []corev1.Volume {
	volumes := []corev1.Volume{
		common.GetVolumeForLogs(),
		common.GetVolumeForAuth(),
		common.GetVolumeInstallInfo(dda),
		common.GetVolumeForChecksd(),
		common.GetVolumeForConfd(),
		common.GetVolumeForConfig(),
		common.GetVolumeForProc(),
		common.GetVolumeForCgroups(),
		common.GetVolumeForDogstatsd(),
		common.GetVolumeForRuntimeSocket(),
	}

	for _, containerName := range requiredContainers {
		if containerName == apicommon.SystemProbeContainerName {
			sysProbeVolumes := []corev1.Volume{
				common.GetVolumeForSecurity(dda),
				common.GetVolumeForSeccomp(),
			}
			volumes = append(volumes, sysProbeVolumes...)
		}
	}

	return volumes
}

func volumeMountsForCoreAgent() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		common.GetVolumeMountForLogs(),
		common.GetVolumeMountForAuth(false),
		common.GetVolumeMountForInstallInfo(),
		common.GetVolumeMountForConfig(),
		common.GetVolumeMountForProc(),
		common.GetVolumeMountForCgroups(),
		common.GetVolumeMountForDogstatsdSocket(false),
		common.GetVolumeMountForRuntimeSocket(true),
	}
}

func volumeMountsForTraceAgent() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		common.GetVolumeMountForLogs(),
		common.GetVolumeMountForProc(),
		common.GetVolumeMountForCgroups(),
		common.GetVolumeMountForAuth(true),
		common.GetVolumeMountForConfig(),
		common.GetVolumeMountForDogstatsdSocket(false),
		common.GetVolumeMountForRuntimeSocket(true),
	}
}

func volumeMountsForProcessAgent() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		common.GetVolumeMountForLogs(),
		common.GetVolumeMountForAuth(true),
		common.GetVolumeMountForConfig(),
		common.GetVolumeMountForDogstatsdSocket(false),
		common.GetVolumeMountForRuntimeSocket(true),
		common.GetVolumeMountForProc(),
	}
}

func volumeMountsForSecurityAgent() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		common.GetVolumeMountForLogs(),
		common.GetVolumeMountForAuth(true),
		common.GetVolumeMountForConfig(),
		common.GetVolumeMountForDogstatsdSocket(false),
		common.GetVolumeMountForRuntimeSocket(true),
	}
}

func volumeMountsForSystemProbe() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		common.GetVolumeMountForLogs(),
		common.GetVolumeMountForAuth(true),
		common.GetVolumeMountForConfig(),
	}
}

func volumeMountsForSeccompSetup() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		common.GetVolumeMountForSecurity(),
		common.GetVolumeMountForSeccomp(),
	}
}

func volumeMountsForOtelAgent() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		common.GetVolumeMountForLogs(),
		common.GetVolumeMountForConfig(),
		common.GetVolumeMountForAuth(true),
	}
}

func volumeMountsForAgentDataPlane() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		common.GetVolumeMountForLogs(),
		common.GetVolumeMountForAuth(true),
		common.GetVolumeMountForConfig(),
		common.GetVolumeMountForDogstatsdSocket(false),
		common.GetVolumeMountForRuntimeSocket(true),
		common.GetVolumeMountForProc(),
		common.GetVolumeMountForCgroups(),
	}
}

func GetDefaultMetadata(owner metav1.Object, componentKind, componentName, version string, selector *metav1.LabelSelector) (map[string]string, map[string]string, *metav1.LabelSelector) {
	labels := common.GetDefaultLabels(owner, componentKind, componentName, version)
	annotations := object.GetDefaultAnnotations(owner)

	if selector != nil {
		for key, val := range selector.MatchLabels {
			labels[key] = val
		}
	} else {
		selector = &metav1.LabelSelector{
			MatchLabels: map[string]string{
				apicommon.AgentDeploymentNameLabelKey:      owner.GetName(),
				apicommon.AgentDeploymentComponentLabelKey: componentKind,
			},
		}
	}
	return labels, annotations, selector
}
