// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagentdeployment

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	authDelegatorName   = "%s-auth-delegator"
	datadogOperatorName = "DatadogAgentDeployment"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// newAgentPodTemplate generates a PodTemplate from a DatadogAgentDeployment spec
func newAgentPodTemplate(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgentDeployment) (*corev1.PodTemplateSpec, error) {
	// copy Agent Spec to configure Agent Pod Template
	labels := getDefaultLabels(agentdeployment, "agent", getAgentVersion(agentdeployment))
	labels[datadoghqv1alpha1.AgentDeploymentNameLabelKey] = agentdeployment.Name
	labels[datadoghqv1alpha1.AgentDeploymentComponentLabelKey] = "agent"

	annotations := getDefaultAnnotations(agentdeployment)
	if isSystemProbeEnabled(agentdeployment) {
		annotations["container.apparmor.security.beta.kubernetes.io/system-probe"] = getAppArmorProfileName(&agentdeployment.Spec.Agent.SystemProbe)
		annotations["container.seccomp.security.alpha.kubernetes.io/system-probe"] = "localhost/system-probe"
	}

	containers := []corev1.Container{}
	agentContainer, err := getAgentContainer(agentdeployment)
	if err != nil {
		return nil, err
	}
	containers = append(containers, *agentContainer)

	if isAPMEnabled(agentdeployment) {
		var container *corev1.Container
		container, err = getAPMAgentContainer(agentdeployment)
		if err != nil {
			return nil, err
		}
		containers = append(containers, *container)
	}
	if isProcessEnabled(agentdeployment) {
		var processContainers []corev1.Container

		processContainers, err = getProcessContainers(agentdeployment)
		if err != nil {
			return nil, err
		}
		containers = append(containers, processContainers...)
	}

	var initContainers []corev1.Container
	initContainers, err = getInitContainers(logger, agentdeployment)
	if err != nil {
		return nil, err
	}

	return &corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: agentdeployment.Name,
			Namespace:    agentdeployment.Namespace,
			Labels:       labels,
			Annotations:  annotations,
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: getAgentServiceAccount(agentdeployment),
			InitContainers:     initContainers,
			Containers:         containers,
			Volumes:            getVolumesForAgent(agentdeployment),
			Tolerations:        agentdeployment.Spec.Agent.Config.Tolerations,
		},
	}, nil
}

func isAPMEnabled(dad *datadoghqv1alpha1.DatadogAgentDeployment) bool {
	if dad.Spec.Agent == nil {
		return false
	}
	return datadoghqv1alpha1.BoolValue(dad.Spec.Agent.Apm.Enabled)
}

func isProcessEnabled(dad *datadoghqv1alpha1.DatadogAgentDeployment) bool {
	if dad.Spec.Agent == nil {
		return false
	}
	return datadoghqv1alpha1.BoolValue(dad.Spec.Agent.Process.Enabled)
}

func isSystemProbeEnabled(dad *datadoghqv1alpha1.DatadogAgentDeployment) bool {
	if dad.Spec.Agent == nil {
		return false
	}
	return datadoghqv1alpha1.BoolValue(dad.Spec.Agent.SystemProbe.Enabled)
}

func getAgentContainer(dad *datadoghqv1alpha1.DatadogAgentDeployment) (*corev1.Container, error) {
	agentSpec := dad.Spec.Agent
	envVars, err := getEnvVarsForAgent(dad)
	if err != nil {
		return nil, err
	}
	agentContainer := &corev1.Container{
		Name:            "agent",
		Image:           agentSpec.Image.Name,
		ImagePullPolicy: *agentSpec.Image.PullPolicy,
		Command: []string{
			"agent",
			"start",
		},
		Resources: *agentSpec.Config.Resources,
		Ports: []corev1.ContainerPort{
			{
				ContainerPort: 8125,
				Name:          "dogstatsdport",
				Protocol:      "UDP",
			},
		},
		Env:           envVars,
		VolumeMounts:  getVolumeMountsForAgent(&dad.Spec),
		LivenessProbe: getDefaultLivenessProbe(),
	}

	return agentContainer, nil
}

func getAPMAgentContainer(dad *datadoghqv1alpha1.DatadogAgentDeployment) (*corev1.Container, error) {
	agentSpec := dad.Spec.Agent
	envVars, err := getEnvVarsForAPMAgent(dad)
	if err != nil {
		return nil, err
	}
	tcpPort := corev1.ContainerPort{
		ContainerPort: datadoghqv1alpha1.DefaultAPMAgentTCPPort,
		Name:          "traceport",
		Protocol:      corev1.ProtocolTCP,
	}
	if agentSpec.Apm.HostPort != nil {
		tcpPort.HostPort = *agentSpec.Apm.HostPort
	}

	apmContainer := &corev1.Container{
		Name:            "trace-agent",
		Image:           agentSpec.Image.Name,
		ImagePullPolicy: *agentSpec.Image.PullPolicy,
		Command: []string{
			"trace-agent",
			"--config=/etc/datadog-agent/datadog.yaml",
		},

		Ports: []corev1.ContainerPort{
			tcpPort,
		},
		Env:           envVars,
		LivenessProbe: getDefaultAPMAgentLivenessProbe(),
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      datadoghqv1alpha1.ConfigVolumeName,
				MountPath: datadoghqv1alpha1.ConfigVolumePath,
			},
		},
	}
	if agentSpec.Apm.Resources != nil {
		apmContainer.Resources = *agentSpec.Apm.Resources
	}

	return apmContainer, nil
}

func getProcessContainers(dad *datadoghqv1alpha1.DatadogAgentDeployment) ([]corev1.Container, error) {
	agentSpec := dad.Spec.Agent
	envVars, err := getEnvVarsForProcessAgent(dad)
	if err != nil {
		return nil, err
	}

	containers := []corev1.Container{}

	process := corev1.Container{
		Name:            "process-agent",
		Image:           agentSpec.Image.Name,
		ImagePullPolicy: *agentSpec.Image.PullPolicy,
		Command: []string{
			"process-agent",
			"-config=/etc/datadog-agent/datadog.yaml",
		},
		Env:          envVars,
		VolumeMounts: getVolumeMountsForProcessAgent(&dad.Spec),
	}

	if agentSpec.Process.Resources != nil {
		process.Resources = *agentSpec.Process.Resources
	}
	containers = append(containers, process)
	if isSystemProbeEnabled(dad) {
		var systemProbeEnvVars []corev1.EnvVar
		systemProbeEnvVars, err = getEnvVarsForSystemProbe(dad)
		if err != nil {
			return nil, err
		}
		systemProbe := corev1.Container{
			Name:            "system-probe",
			Image:           agentSpec.Image.Name,
			ImagePullPolicy: *agentSpec.Image.PullPolicy,
			Command: []string{
				"/opt/datadog-agent/embedded/bin/system-probe",
				fmt.Sprintf("-config=%s/system-probe.yaml", datadoghqv1alpha1.SystemProbeConfigVolumePath),
			},
			SecurityContext: &corev1.SecurityContext{
				Capabilities: &corev1.Capabilities{
					Add: []corev1.Capability{"SYS_ADMIN", "SYS_RESOURCE", "SYS_PTRACE", "NET_ADMIN", "IPC_LOCK"},
				},
			},
			Env:          systemProbeEnvVars,
			VolumeMounts: getVolumeMountsForSystemProbe(&dad.Spec),
		}
		if agentSpec.SystemProbe.Resources != nil {
			systemProbe.Resources = *agentSpec.SystemProbe.Resources

		}
		containers = append(containers, systemProbe)
	}

	return containers, nil
}

func getInitContainers(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment) ([]corev1.Container, error) {
	spec := &dad.Spec
	volumeMounts := getVolumeMountsForAgent(spec)
	envVars, err := getEnvVarsForAgent(dad)
	if err != nil {
		return nil, err
	}
	containers := []corev1.Container{
		{
			Name:            "init-volume",
			Image:           spec.Agent.Image.Name,
			ImagePullPolicy: *spec.Agent.Image.PullPolicy,
			Resources:       *spec.Agent.Config.Resources,
			Command:         []string{"bash", "-c"},
			Args:            []string{"cp -r /etc/datadog-agent /opt"},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      datadoghqv1alpha1.ConfigVolumeName,
					MountPath: "/opt/datadog-agent",
				},
			},
		},
		{
			Name:            "init-config",
			Image:           spec.Agent.Image.Name,
			ImagePullPolicy: *spec.Agent.Image.PullPolicy,
			Resources:       *spec.Agent.Config.Resources,
			Command:         []string{"bash", "-c"},
			Args:            []string{"for script in $(find /etc/cont-init.d/ -type f -name '*.sh' | sort) ; do bash $script ; done"},
			Env:             envVars,
			VolumeMounts:    volumeMounts,
		},
	}
	if isSystemProbeEnabled(dad) {
		systemProbeInit := corev1.Container{
			Name:            "seccomp-setup",
			Image:           spec.Agent.Image.Name,
			ImagePullPolicy: *spec.Agent.Image.PullPolicy,
			Resources:       *spec.Agent.Config.Resources,
			Command: []string{
				"cp",
				fmt.Sprintf("%s/system-probe-seccomp.json", datadoghqv1alpha1.SystemProbeAgentSecurityVolumePath),
				fmt.Sprintf("%s/system-probe", datadoghqv1alpha1.SystemProbeSecCompRootVolumePath),
			},
			VolumeMounts: []corev1.VolumeMount{
				{
					Name:      datadoghqv1alpha1.SystemProbeAgentSecurityVolumeName,
					MountPath: datadoghqv1alpha1.SystemProbeAgentSecurityVolumePath,
				},
				{
					Name:      datadoghqv1alpha1.SystemProbeSecCompRootVolumeName,
					MountPath: datadoghqv1alpha1.SystemProbeSecCompRootVolumePath,
				},
			},
		}
		containers = append(containers, systemProbeInit)
	}

	return containers, nil
}

// getEnvVarsForAPMAgent converts APM Agent Config into container env vars
func getEnvVarsForAPMAgent(dad *datadoghqv1alpha1.DatadogAgentDeployment) ([]corev1.EnvVar, error) {
	envVars := []corev1.EnvVar{
		{
			Name:  datadoghqv1alpha1.DDAPMEnabled,
			Value: strconv.FormatBool(isAPMEnabled(dad)),
		},
	}
	commonEnvVars, err := getEnvVarsCommon(dad, true)
	if err != nil {
		return nil, err
	}
	envVars = append(envVars, commonEnvVars...)
	envVars = append(envVars, dad.Spec.Agent.Apm.Env...)
	return envVars, nil
}

// getEnvVarsForProcessAgent converts Process Agent Config into container env vars
func getEnvVarsForProcessAgent(dad *datadoghqv1alpha1.DatadogAgentDeployment) ([]corev1.EnvVar, error) {
	envVars := []corev1.EnvVar{
		{
			Name:  datadoghqv1alpha1.DDProcessAgentEnabled,
			Value: strconv.FormatBool(isProcessEnabled(dad)),
		},
		{
			Name:  datadoghqv1alpha1.DDSystemProbeAgentEnabled,
			Value: strconv.FormatBool(isSystemProbeEnabled(dad)),
		},
	}
	commonEnvVars, err := getEnvVarsCommon(dad, true)
	if err != nil {
		return nil, err
	}
	envVars = append(envVars, commonEnvVars...)
	envVars = append(envVars, dad.Spec.Agent.Process.Env...)
	return envVars, nil
}

// getEnvVarsForSystemProbe converts System Probe Config into container env vars
func getEnvVarsForSystemProbe(dad *datadoghqv1alpha1.DatadogAgentDeployment) ([]corev1.EnvVar, error) {
	envVars := []corev1.EnvVar{}
	commonEnvVars, err := getEnvVarsCommon(dad, false)
	if err != nil {
		return nil, err
	}
	envVars = append(envVars, commonEnvVars...)
	envVars = append(envVars, dad.Spec.Agent.SystemProbe.Env...)
	return envVars, nil
}

func getEnvVarsCommon(dad *datadoghqv1alpha1.DatadogAgentDeployment, needApiKey bool) ([]corev1.EnvVar, error) {

	envVars := []corev1.EnvVar{
		{
			Name:  datadoghqv1alpha1.DDLogLevel,
			Value: getLogLevel(dad),
		},
		{
			Name: datadoghqv1alpha1.DDKubeletHost,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: FieldPathStatusHostIP,
				},
			},
		},
		{
			Name: datadoghqv1alpha1.DDHostname,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: FieldPathSpecNodeName,
				},
			},
		},
		{
			Name:  datadoghqv1alpha1.KubernetesEnvvarName,
			Value: "yes",
		},
	}

	if needApiKey {
		var apiKeyEnvVar corev1.EnvVar
		if dad.Spec.Credentials.APIKeyExistingSecret != "" {
			apiKeyEnvVar = corev1.EnvVar{
				Name:      datadoghqv1alpha1.DDAPIKey,
				ValueFrom: getAPIKeyFromSecret(dad),
			}
		} else {
			apiKeyEnvVar = corev1.EnvVar{
				Name:  datadoghqv1alpha1.DDAPIKey,
				Value: dad.Spec.Credentials.APIKey,
			}
		}
		envVars = append(envVars, apiKeyEnvVar)
	}

	if len(dad.Spec.Agent.Config.Tags) > 0 {
		tags, err := json.Marshal(dad.Spec.Agent.Config.Tags)
		if err != nil {
			return nil, err
		}

		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDTags,
			Value: string(tags),
		})
	}

	return envVars, nil
}

// getEnvVarsForAgent converts Agent Config into container env vars
func getEnvVarsForAgent(dad *datadoghqv1alpha1.DatadogAgentDeployment) ([]corev1.EnvVar, error) {
	spec := dad.Spec
	// Marshal tag fields
	podLabelsAsTags, err := json.Marshal(spec.Agent.Config.PodLabelsAsTags)
	if err != nil {
		return nil, err
	}
	podAnnotationsAsTags, err := json.Marshal(spec.Agent.Config.PodAnnotationsAsTags)
	if err != nil {
		return nil, err
	}

	envVars := []corev1.EnvVar{
		{
			Name:  datadoghqv1alpha1.DDClusterName,
			Value: spec.ClusterName,
		},
		{
			Name:  datadoghqv1alpha1.DDSite,
			Value: spec.Site,
		},
		{
			Name:  datadoghqv1alpha1.DDddURL,
			Value: *spec.Agent.Config.DDUrl,
		},
		{
			Name:  datadoghqv1alpha1.DDHealthPort,
			Value: strconv.Itoa(int(datadoghqv1alpha1.DefaultAgentHealthPort)),
		},
		{
			Name:  datadoghqv1alpha1.DDPodLabelsAsTags,
			Value: string(podLabelsAsTags),
		},
		{
			Name:  datadoghqv1alpha1.DDPodAnnotationsAsTags,
			Value: string(podAnnotationsAsTags),
		},
		{
			Name:  datadoghqv1alpha1.DDCollectKubeEvents,
			Value: strconv.FormatBool(*spec.Agent.Config.CollectEvents),
		},
		{
			Name:  datadoghqv1alpha1.DDLeaderElection,
			Value: strconv.FormatBool(*spec.Agent.Config.LeaderElection),
		},
		{
			Name:  datadoghqv1alpha1.DDLogsEnabled,
			Value: strconv.FormatBool(*spec.Agent.Log.Enabled),
		},
		{
			Name:  datadoghqv1alpha1.DDLogsConfigContainerCollectAll,
			Value: strconv.FormatBool(*spec.Agent.Log.LogsConfigContainerCollectAll),
		},
		{
			Name:  datadoghqv1alpha1.DDDogstatsdOriginDetection,
			Value: strconv.FormatBool(*spec.Agent.Config.Dogstatsd.DogstatsdOriginDetection),
		},
	}
	commonEnvVars, err := getEnvVarsCommon(dad, true)
	if err != nil {
		return nil, err
	}
	envVars = append(envVars, commonEnvVars...)
	if spec.ClusterAgent != nil {
		clusterEnv := []corev1.EnvVar{
			{
				Name:  datadoghqv1alpha1.DDClusterAgentEnabled,
				Value: strconv.FormatBool(true),
			},
			{
				Name:  datadoghqv1alpha1.DDClusterAgentKubeServiceName,
				Value: getClusterAgentServiceName(dad),
			},
			{
				Name:      datadoghqv1alpha1.DDClusterAgentAuthToken,
				ValueFrom: getClusterAgentAuthToken(dad),
			},
		}
		if *spec.ClusterAgent.Config.ClusterChecksRunnerEnabled && spec.ClusterChecksRunner == nil {
			clusterEnv = append(clusterEnv, corev1.EnvVar{
				Name:  datadoghqv1alpha1.DDExtraConfigProviders,
				Value: datadoghqv1alpha1.ClusterChecksConfigProvider,
			})
		}
		envVars = append(envVars, clusterEnv...)
	}
	return append(envVars, spec.Agent.Config.Env...), nil
}

// getVolumesForAgent defines volumes for the Agent
func getVolumesForAgent(dad *datadoghqv1alpha1.DatadogAgentDeployment) []corev1.Volume {
	confdVolumeSource := corev1.VolumeSource{
		EmptyDir: &corev1.EmptyDirVolumeSource{},
	}
	if dad.Spec.Agent.Confd != nil {
		confdVolumeSource = corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: dad.Spec.Agent.Confd.ConfigMapName,
				},
			},
		}
	}
	checksdVolumeSource := corev1.VolumeSource{
		EmptyDir: &corev1.EmptyDirVolumeSource{},
	}
	if dad.Spec.Agent.Checksd != nil {
		checksdVolumeSource = corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: dad.Spec.Agent.Checksd.ConfigMapName,
				},
			},
		}
	}

	volumes := []corev1.Volume{
		{
			Name:         datadoghqv1alpha1.ConfdVolumeName,
			VolumeSource: confdVolumeSource,
		},
		{
			Name:         datadoghqv1alpha1.ChecksdVolumeName,
			VolumeSource: checksdVolumeSource,
		},
		{
			Name: datadoghqv1alpha1.ConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: datadoghqv1alpha1.ProcVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/proc",
				},
			},
		},
		{
			Name: datadoghqv1alpha1.CgroupsVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/sys/fs/cgroup",
				},
			},
		},
	}
	if dad.Spec.Agent.Config.CriSocket != nil && dad.Spec.Agent.Config.CriSocket.UseCriSocketVolume != nil && *dad.Spec.Agent.Config.CriSocket.UseCriSocketVolume {
		path := "/var/run/docker.sock"
		if dad.Spec.Agent.Config.CriSocket.CriSocketPath != nil {
			path = *dad.Spec.Agent.Config.CriSocket.CriSocketPath
		}
		criVolume := corev1.Volume{
			Name: datadoghqv1alpha1.CriSockerVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: path,
				},
			},
		}
		volumes = append(volumes, criVolume)
	}
	if datadoghqv1alpha1.BoolValue(dad.Spec.Agent.Process.Enabled) {
		passwdVolume := corev1.Volume{
			Name: datadoghqv1alpha1.PasswdVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: datadoghqv1alpha1.PasswdVolumePath,
				},
			},
		}
		volumes = append(volumes, passwdVolume)
	}
	if datadoghqv1alpha1.BoolValue(dad.Spec.Agent.SystemProbe.Enabled) {
		systemProbeVolumes := []corev1.Volume{
			{
				Name: datadoghqv1alpha1.SystemProbeAgentSecurityVolumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: getSecCompConfigMapName(dad.Name),
						},
					},
				},
			},
			{
				Name: datadoghqv1alpha1.SystemProbeConfigVolumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: getSystemProbeConfiConfigMapName(dad.Name),
						},
					},
				},
			},
			{
				Name: datadoghqv1alpha1.SystemProbeSecCompRootVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: getSecCompRootPath(&dad.Spec.Agent.SystemProbe),
					},
				},
			},
			{
				Name: datadoghqv1alpha1.SystemProbeDebugfsVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: datadoghqv1alpha1.SystemProbeDebugfsVolumePath,
					},
				},
			},
			{
				Name: datadoghqv1alpha1.SystemProbeSocketVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		}

		volumes = append(volumes, systemProbeVolumes...)
	}
	return volumes
}

func getSecCompRootPath(spec *datadoghqv1alpha1.SystemProbeSpec) string {
	if spec.SecCompRootPath != "" {
		return spec.SecCompRootPath
	}
	return datadoghqv1alpha1.DefaultSystemProbeSecCompRootPath
}

func getAppArmorProfileName(spec *datadoghqv1alpha1.SystemProbeSpec) string {
	if spec.AppArmorProfileName != "" {
		return spec.AppArmorProfileName
	}
	return datadoghqv1alpha1.DefaultAppArmorProfileName
}

// getVolumeMountsForAgent defines mounted volumes for the Agent
func getVolumeMountsForAgent(spec *datadoghqv1alpha1.DatadogAgentDeploymentSpec) []corev1.VolumeMount {
	// Default mounted volumes
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      datadoghqv1alpha1.ConfdVolumeName,
			MountPath: datadoghqv1alpha1.ConfdVolumePath,
			ReadOnly:  true,
		},
		{
			Name:      datadoghqv1alpha1.ChecksdVolumeName,
			MountPath: datadoghqv1alpha1.ChecksdVolumePath,
			ReadOnly:  true,
		},
		{
			Name:      datadoghqv1alpha1.ConfigVolumeName,
			MountPath: datadoghqv1alpha1.ConfigVolumePath,
		},
		{
			Name:      datadoghqv1alpha1.ProcVolumeName,
			MountPath: datadoghqv1alpha1.ProcVolumePath,
			ReadOnly:  datadoghqv1alpha1.ProcVolumeReadOnly,
		},
		{
			Name:      datadoghqv1alpha1.CgroupsVolumeName,
			MountPath: datadoghqv1alpha1.CgroupsVolumePath,
			ReadOnly:  datadoghqv1alpha1.CgroupsVolumeReadOnly,
		},
	}

	// Cri socket volume
	if *spec.Agent.Config.CriSocket.UseCriSocketVolume {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      datadoghqv1alpha1.CriSockerVolumeName,
			MountPath: *spec.Agent.Config.CriSocket.CriSocketPath,
			ReadOnly:  true,
		})
	}

	// Dogstatsd volume
	if *spec.Agent.Config.Dogstatsd.UseDogStatsDSocketVolume {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      datadoghqv1alpha1.DogstatsdSockerVolumeName,
			MountPath: datadoghqv1alpha1.DogstatsdSockerVolumePath,
		})
	}

	// Log volumes
	if *spec.Agent.Log.Enabled {
		volumeMounts = append(volumeMounts, []corev1.VolumeMount{
			{
				Name:      datadoghqv1alpha1.PointerVolumeName,
				MountPath: datadoghqv1alpha1.PointerVolumePath,
			},
			{
				Name:      datadoghqv1alpha1.LogPodVolumeName,
				MountPath: datadoghqv1alpha1.LogPodVolumePath,
				ReadOnly:  datadoghqv1alpha1.LogPodVolumeReadOnly,
			},
			{
				Name:      datadoghqv1alpha1.LogContainerVolumeName,
				MountPath: *spec.Agent.Log.ContainerLogsPath,
				ReadOnly:  datadoghqv1alpha1.LogContainerVolumeReadOnly,
			},
		}...)
	}
	return append(volumeMounts, spec.Agent.Config.VolumeMounts...)
}

// getVolumeMountsForAgent defines mounted volumes for the Process Agent
func getVolumeMountsForProcessAgent(spec *datadoghqv1alpha1.DatadogAgentDeploymentSpec) []corev1.VolumeMount {
	// Default mounted volumes
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      datadoghqv1alpha1.CgroupsVolumeName,
			MountPath: datadoghqv1alpha1.CgroupsVolumePath,
			ReadOnly:  true,
		},
		{
			Name:      datadoghqv1alpha1.ConfigVolumeName,
			MountPath: datadoghqv1alpha1.ConfigVolumePath,
		},
		{
			Name:      datadoghqv1alpha1.PasswdVolumeName,
			MountPath: datadoghqv1alpha1.PasswdVolumePath,
			ReadOnly:  true,
		},
		{
			Name:      datadoghqv1alpha1.ProcVolumeName,
			MountPath: datadoghqv1alpha1.ProcVolumePath,
			ReadOnly:  true,
		},
	}

	// Cri socket volume
	if *spec.Agent.Config.CriSocket.UseCriSocketVolume {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      datadoghqv1alpha1.CriSockerVolumeName,
			MountPath: *spec.Agent.Config.CriSocket.CriSocketPath,
			ReadOnly:  true,
		})
	}

	if datadoghqv1alpha1.BoolValue(spec.Agent.SystemProbe.Enabled) {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      datadoghqv1alpha1.SystemProbeSocketVolumeName,
			MountPath: datadoghqv1alpha1.SystemProbeSocketVolumePath,
			ReadOnly:  true,
		})
	}

	return volumeMounts
}

// getVolumeMountsForSystemProbe defines mounted volumes for the SystemProbe
func getVolumeMountsForSystemProbe(spec *datadoghqv1alpha1.DatadogAgentDeploymentSpec) []corev1.VolumeMount {
	// Default mounted volumes
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      datadoghqv1alpha1.SystemProbeDebugfsVolumeName,
			MountPath: datadoghqv1alpha1.SystemProbeDebugfsVolumePath,
		},
		{
			Name:      datadoghqv1alpha1.SystemProbeConfigVolumeName,
			MountPath: datadoghqv1alpha1.SystemProbeConfigVolumePath,
		},
		{
			Name:      datadoghqv1alpha1.SystemProbeSocketVolumeName,
			MountPath: datadoghqv1alpha1.SystemProbeSocketVolumePath,
		},
		{
			Name:      datadoghqv1alpha1.ProcVolumeName,
			MountPath: datadoghqv1alpha1.ProcVolumePath,
			ReadOnly:  true,
		},
	}

	return volumeMounts
}

func getAgentVersion(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	// TODO implement this method
	return ""
}

func getAgentServiceAccount(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	saDefault := fmt.Sprintf("%s-agent", dad.Name)
	if dad.Spec.Agent == nil {
		return saDefault
	}
	if dad.Spec.Agent.Rbac.ServiceAccountName != nil {
		return *dad.Spec.Agent.Rbac.ServiceAccountName
	}
	return saDefault
}

// getAPIKeyFromSecret returns the Agent API key as an env var source
func getAPIKeyFromSecret(dad *datadoghqv1alpha1.DatadogAgentDeployment) *corev1.EnvVarSource {
	authTokenValue := &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: getAPIKeySecretName(dad),
			},
			Key: datadoghqv1alpha1.DefaultAPIKeyKey,
		},
	}
	return authTokenValue
}

// getClusterAgentAuthToken returns the Cluster Agent auth token as an env var source
func getClusterAgentAuthToken(dad *datadoghqv1alpha1.DatadogAgentDeployment) *corev1.EnvVarSource {
	authTokenValue := &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{},
	}
	authTokenValue.SecretKeyRef.Name = getAppKeySecretName(dad)
	authTokenValue.SecretKeyRef.Key = "token"
	return authTokenValue
}

func getAppKeySecretName(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	if dad.Spec.Credentials.AppKeyExistingSecret != "" {
		return dad.Spec.Credentials.AppKeyExistingSecret
	}
	return fmt.Sprintf("%s-%s", dad.Name, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix)
}

// getAppKeyFromSecret returns the Agent API key as an env var source
func getAppKeyFromSecret(dad *datadoghqv1alpha1.DatadogAgentDeployment) *corev1.EnvVarSource {
	authTokenValue := &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: getAppKeySecretName(dad),
			},
			Key: datadoghqv1alpha1.DefaultAPPKeyKey,
		},
	}
	return authTokenValue
}

func getAPIKeySecretName(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	if dad.Spec.Credentials.APIKeyExistingSecret != "" {
		return dad.Spec.Credentials.APIKeyExistingSecret
	}
	return dad.Name
}

func getClusterAgentServiceName(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	return fmt.Sprintf("%s-%s", dad.Name, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix)
}

func getClusterAgentServiceAccount(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	saDefault := fmt.Sprintf("%s-%s", dad.Name, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix)
	if dad.Spec.ClusterAgent == nil {
		return saDefault
	}
	if dad.Spec.ClusterAgent.Rbac.ServiceAccountName != nil {
		return *dad.Spec.ClusterAgent.Rbac.ServiceAccountName
	}
	return saDefault
}

func getClusterAgentVersion(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	// TODO implement this method
	return ""
}

func getMetricsServerServiceName(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	return fmt.Sprintf("%s-%s", dad.Name, datadoghqv1alpha1.DefaultMetricsServerResourceSuffix)
}

func getClusterAgentRbacResourcesName(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	return fmt.Sprintf("%s-%s", dad.Name, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix)
}

func getAgentRbacResourcesName(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	return fmt.Sprintf("%s-%s", dad.Name, datadoghqv1alpha1.DefaultAgentResourceSuffix)
}

func getClusterChecksRunnerRbacResourcesName(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	return fmt.Sprintf("%s-%s", dad.Name, datadoghqv1alpha1.DefaultClusterChecksRunnerResourceSuffix)
}

func getHPAClusterRoleBindingName(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	return fmt.Sprintf(authDelegatorName, getClusterAgentRbacResourcesName(dad))
}

func getClusterChecksRunnerServiceAccount(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	saDefault := fmt.Sprintf("%s-%s", dad.Name, datadoghqv1alpha1.DefaultClusterChecksRunnerResourceSuffix)
	if dad.Spec.ClusterChecksRunner == nil {
		return saDefault
	}
	if dad.Spec.ClusterChecksRunner.Rbac.ServiceAccountName != nil {
		return *dad.Spec.ClusterChecksRunner.Rbac.ServiceAccountName
	}
	return saDefault
}

func getDefaultLabels(dad *datadoghqv1alpha1.DatadogAgentDeployment, instanceName, version string) map[string]string {
	// TODO implement this method
	labels := make(map[string]string)
	labels[kubernetes.AppKubernetesNameLabelKey] = "datadog-agent-deployment"
	labels[kubernetes.AppKubernetesInstanceLabelKey] = instanceName
	labels[kubernetes.AppKubernetesPartOfLabelKey] = dad.Name
	labels[kubernetes.AppKubernetesVersionLabelKey] = version
	labels[kubernetes.AppKubernetesManageByLabelKey] = "datadog-operator"
	return labels
}

func getDefaultAnnotations(dad *datadoghqv1alpha1.DatadogAgentDeployment) map[string]string {
	// TODO implement this method
	return make(map[string]string)
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

func generateRandomString(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func shouldReturn(result reconcile.Result, err error) bool {
	if err != nil || result.Requeue || result.RequeueAfter > 0 {
		return true
	}
	return false
}

func isMetricsProviderEnabled(spec *datadoghqv1alpha1.DatadogAgentDeploymentSpecClusterAgentSpec) bool {
	if spec == nil {
		return false
	}
	if datadoghqv1alpha1.BoolValue(spec.Config.MetricsProviderEnabled) {
		return true
	}
	return false
}

func isCreateRBACEnabled(config datadoghqv1alpha1.RbacConfig) bool {
	return datadoghqv1alpha1.BoolValue(config.Create)
}

func getDefaultLivenessProbe() *corev1.Probe {
	livenessProbe := &corev1.Probe{
		InitialDelaySeconds: datadoghqv1alpha1.DefaultLivenessProveInitialDelaySeconds,
		PeriodSeconds:       datadoghqv1alpha1.DefaultLivenessProvePeriodSeconds,
		TimeoutSeconds:      datadoghqv1alpha1.DefaultLivenessProveTimeoutSeconds,
		SuccessThreshold:    datadoghqv1alpha1.DefaultLivenessProveSuccessThreshold,
		FailureThreshold:    datadoghqv1alpha1.DefaultLivenessProveFailureThreshold,
	}
	livenessProbe.HTTPGet = &corev1.HTTPGetAction{
		Path: datadoghqv1alpha1.DefaultLivenessProveHTTPPath,
		Port: intstr.IntOrString{
			IntVal: datadoghqv1alpha1.DefaultAgentHealthPort,
		},
	}
	return livenessProbe
}

func getDefaultAPMAgentLivenessProbe() *corev1.Probe {
	livenessProbe := &corev1.Probe{
		InitialDelaySeconds: datadoghqv1alpha1.DefaultLivenessProveInitialDelaySeconds,
		PeriodSeconds:       datadoghqv1alpha1.DefaultLivenessProvePeriodSeconds,
		TimeoutSeconds:      datadoghqv1alpha1.DefaultLivenessProveTimeoutSeconds,
	}
	livenessProbe.TCPSocket = &corev1.TCPSocketAction{
		Port: intstr.IntOrString{
			IntVal: datadoghqv1alpha1.DefaultAPMAgentTCPPort,
		},
	}
	return livenessProbe
}

func getPodAffinity(affinity *corev1.Affinity, labelValue string) *corev1.Affinity {
	if affinity != nil {
		return affinity
	}

	return &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
				{
					LabelSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": labelValue,
						},
					},
					TopologyKey: "kubernetes.io/hostname",
				},
			},
		},
	}
}

func updateDeploymentStatus(dep *appsv1.Deployment, depStatus *datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStatus, updateTime *metav1.Time) *datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStatus {
	if depStatus == nil {
		depStatus = &datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStatus{}
	}
	depStatus.CurrentHash = getHashAnnotation(dep.Annotations)
	if updateTime != nil {
		depStatus.LastUpdate = updateTime
	}
	depStatus.Replicas = dep.Status.Replicas
	depStatus.UpdatedReplicas = dep.Status.UpdatedReplicas
	depStatus.AvailableReplicas = dep.Status.AvailableReplicas
	depStatus.UnavailableReplicas = dep.Status.UnavailableReplicas
	depStatus.ReadyReplicas = dep.Status.ReadyReplicas
	depStatus.State = datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStateRunning
	return depStatus
}

func ownedByDatadogOperator(owners []metav1.OwnerReference) bool {
	for _, owner := range owners {
		if owner.Kind == datadogOperatorName {
			return true
		}
	}
	return false
}

func getLogLevel(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	logLevel := datadoghqv1alpha1.DefaultLogLevel
	if dad.Spec.Agent.Config.LogLevel != nil {
		logLevel = *dad.Spec.Agent.Config.LogLevel
	}
	return logLevel
}
