// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagent

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	edsdatadoghqv1alpha1 "github.com/datadog/extendeddaemonset/pkg/apis/datadoghq/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	authDelegatorName   = "%s-auth-delegator"
	datadogOperatorName = "DatadogAgent"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// getTokenSecretName returns the token secret name
func getAuthTokenSecretName(dda *datadoghqv1alpha1.DatadogAgent) string {
	return dda.Name
}

// newAgentPodTemplate generates a PodTemplate from a DatadogAgent spec
func newAgentPodTemplate(agentdeployment *datadoghqv1alpha1.DatadogAgent, selector *metav1.LabelSelector) (*corev1.PodTemplateSpec, error) {
	// copy Agent Spec to configure Agent Pod Template
	labels := getDefaultLabels(agentdeployment, "agent", getAgentVersion(agentdeployment))
	labels[datadoghqv1alpha1.AgentDeploymentNameLabelKey] = agentdeployment.Name
	labels[datadoghqv1alpha1.AgentDeploymentComponentLabelKey] = "agent"

	for key, val := range agentdeployment.Spec.Agent.AdditionalLabels {
		labels[key] = val
	}

	if selector != nil {
		for key, val := range selector.MatchLabels {
			labels[key] = val
		}
	}

	annotations := getDefaultAnnotations(agentdeployment)
	if isSystemProbeEnabled(agentdeployment) {
		annotations[datadoghqv1alpha1.SysteProbeAppArmorAnnotationKey] = getAppArmorProfileName(&agentdeployment.Spec.Agent.SystemProbe)
		annotations[datadoghqv1alpha1.SysteProbeSeccompAnnotationKey] = getSeccompProfileName(&agentdeployment.Spec.Agent.SystemProbe)
	}

	for key, val := range agentdeployment.Spec.Agent.AdditionalAnnotations {
		annotations[key] = val
	}

	containers := []corev1.Container{}
	agentContainer, err := getAgentContainer(agentdeployment)
	if err != nil {
		return nil, err
	}
	containers = append(containers, *agentContainer)

	if isAPMEnabled(agentdeployment) {
		var apmContainers []corev1.Container

		apmContainers, err = getAPMAgentContainers(agentdeployment)
		if err != nil {
			return nil, err
		}
		containers = append(containers, apmContainers...)
	}
	if isProcessEnabled(agentdeployment) {
		var processContainers []corev1.Container

		processContainers, err = getProcessContainers(agentdeployment)
		if err != nil {
			return nil, err
		}
		containers = append(containers, processContainers...)
	}
	if isSystemProbeEnabled(agentdeployment) {
		var systemProbeContainers []corev1.Container

		systemProbeContainers, err = getSystemProbeContainers(agentdeployment)
		if err != nil {
			return nil, err
		}
		containers = append(containers, systemProbeContainers...)
	}

	var initContainers []corev1.Container
	initContainers, err = getInitContainers(agentdeployment)
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
			PriorityClassName:  agentdeployment.Spec.Agent.PriorityClassName,
			HostNetwork:        agentdeployment.Spec.Agent.HostNetwork,
			HostPID:            agentdeployment.Spec.Agent.HostPID,
			DNSPolicy:          agentdeployment.Spec.Agent.DNSPolicy,
			DNSConfig:          agentdeployment.Spec.Agent.DNSConfig,
		},
	}, nil
}

func isAPMEnabled(dda *datadoghqv1alpha1.DatadogAgent) bool {
	if dda.Spec.Agent == nil {
		return false
	}
	return datadoghqv1alpha1.BoolValue(dda.Spec.Agent.Apm.Enabled)
}

func isProcessEnabled(dda *datadoghqv1alpha1.DatadogAgent) bool {
	if dda.Spec.Agent == nil {
		return false
	}
	return datadoghqv1alpha1.BoolValue(dda.Spec.Agent.Process.Enabled)
}

func isSystemProbeEnabled(dda *datadoghqv1alpha1.DatadogAgent) bool {
	if dda.Spec.Agent == nil {
		return false
	}
	return datadoghqv1alpha1.BoolValue(dda.Spec.Agent.SystemProbe.Enabled)
}

func getAgentContainer(dda *datadoghqv1alpha1.DatadogAgent) (*corev1.Container, error) {
	agentSpec := dda.Spec.Agent
	envVars, err := getEnvVarsForAgent(dda)
	if err != nil {
		return nil, err
	}

	udpPort := corev1.ContainerPort{
		ContainerPort: datadoghqv1alpha1.DefaultDogstatsdPort,
		Name:          "dogstatsdport",
		Protocol:      corev1.ProtocolUDP,
	}

	if agentSpec.Config.HostPort != nil {
		// Create the host port configuration
		udpPort.HostPort = *agentSpec.Config.HostPort
		// If HostNetwork is enabled, set the container port
		// and the DD_DOGSTATSD_PORT environment variable to match the host port
		// so that Dogstatsd can be reached on the port configured in HostPort
		if agentSpec.HostNetwork {
			udpPort.ContainerPort = *agentSpec.Config.HostPort
			envVars = append(envVars, corev1.EnvVar{
				Name:  datadoghqv1alpha1.DDDogstatsdPort,
				Value: strconv.Itoa(int(*agentSpec.Config.HostPort)),
			})
		}
	}

	agentContainer := &corev1.Container{
		Name:            "agent",
		Image:           agentSpec.Image.Name,
		ImagePullPolicy: *agentSpec.Image.PullPolicy,
		Command: []string{
			"agent",
			"run",
		},
		Resources: *agentSpec.Config.Resources,
		Ports: []corev1.ContainerPort{
			udpPort,
		},
		Env:            envVars,
		VolumeMounts:   getVolumeMountsForAgent(&dda.Spec),
		LivenessProbe:  getDefaultLivenessProbe(),
		ReadinessProbe: getDefaultReadinessProbe(),
	}

	return agentContainer, nil
}

func getAPMAgentContainers(dda *datadoghqv1alpha1.DatadogAgent) ([]corev1.Container, error) {
	agentSpec := dda.Spec.Agent
	envVars, err := getEnvVarsForAPMAgent(dda)
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

	apmContainer := corev1.Container{
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

	return []corev1.Container{apmContainer}, nil
}

func getProcessContainers(dda *datadoghqv1alpha1.DatadogAgent) ([]corev1.Container, error) {
	agentSpec := dda.Spec.Agent
	envVars, err := getEnvVarsForProcessAgent(dda)
	if err != nil {
		return nil, err
	}

	process := corev1.Container{
		Name:            "process-agent",
		Image:           agentSpec.Image.Name,
		ImagePullPolicy: *agentSpec.Image.PullPolicy,
		Command: []string{
			"process-agent",
			"-config=/etc/datadog-agent/datadog.yaml",
		},
		Env:          envVars,
		VolumeMounts: getVolumeMountsForProcessAgent(&dda.Spec),
	}

	if agentSpec.Process.Resources != nil {
		process.Resources = *agentSpec.Process.Resources
	}

	return []corev1.Container{process}, nil
}

func getSystemProbeContainers(dda *datadoghqv1alpha1.DatadogAgent) ([]corev1.Container, error) {
	agentSpec := dda.Spec.Agent
	systemProbeEnvVars, err := getEnvVarsForSystemProbe(dda)
	if err != nil {
		return nil, err
	}
	systemProbe := corev1.Container{
		Name:            "system-probe",
		Image:           agentSpec.Image.Name,
		ImagePullPolicy: *agentSpec.Image.PullPolicy,
		Command: []string{
			"/opt/datadog-agent/embedded/bin/system-probe",
			fmt.Sprintf("-config=%s", datadoghqv1alpha1.SystemProbeConfigVolumePath),
		},
		SecurityContext: &corev1.SecurityContext{
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{"SYS_ADMIN", "SYS_RESOURCE", "SYS_PTRACE", "NET_ADMIN", "IPC_LOCK"},
			},
		},
		Env:          systemProbeEnvVars,
		VolumeMounts: getVolumeMountsForSystemProbe(),
	}
	if agentSpec.SystemProbe.SecurityContext != nil {
		systemProbe.SecurityContext = agentSpec.SystemProbe.SecurityContext.DeepCopy()
	}
	if agentSpec.SystemProbe.Resources != nil {
		systemProbe.Resources = *agentSpec.SystemProbe.Resources
	}

	return []corev1.Container{systemProbe}, nil
}

func getInitContainers(dda *datadoghqv1alpha1.DatadogAgent) ([]corev1.Container, error) {
	spec := &dda.Spec
	volumeMounts := getVolumeMountsForAgent(spec)
	envVars, err := getEnvVarsForAgent(dda)
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
	if isSystemProbeEnabled(dda) {
		if getSeccompProfileName(&dda.Spec.Agent.SystemProbe) == datadoghqv1alpha1.DefaultSeccompProfileName || dda.Spec.Agent.SystemProbe.SecCompCustomProfileConfigMap != "" {
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
	}

	return containers, nil
}

// getEnvVarsForAPMAgent converts APM Agent Config into container env vars
func getEnvVarsForAPMAgent(dda *datadoghqv1alpha1.DatadogAgent) ([]corev1.EnvVar, error) {
	envVars := []corev1.EnvVar{
		{
			Name:  datadoghqv1alpha1.DDAPMEnabled,
			Value: strconv.FormatBool(isAPMEnabled(dda)),
		},
	}
	commonEnvVars, err := getEnvVarsCommon(dda, true)
	if err != nil {
		return nil, err
	}
	envVars = append(envVars, commonEnvVars...)
	envVars = append(envVars, dda.Spec.Agent.Apm.Env...)
	return envVars, nil
}

// getEnvVarsForProcessAgent converts Process Agent Config into container env vars
func getEnvVarsForProcessAgent(dda *datadoghqv1alpha1.DatadogAgent) ([]corev1.EnvVar, error) {
	envVars := []corev1.EnvVar{
		{
			Name:  datadoghqv1alpha1.DDProcessAgentEnabled,
			Value: strconv.FormatBool(isProcessEnabled(dda)),
		},
		{
			Name:  datadoghqv1alpha1.DDSystemProbeAgentEnabled,
			Value: strconv.FormatBool(isSystemProbeEnabled(dda)),
		},
	}
	commonEnvVars, err := getEnvVarsCommon(dda, true)
	if err != nil {
		return nil, err
	}
	envVars = append(envVars, commonEnvVars...)
	envVars = append(envVars, dda.Spec.Agent.Process.Env...)
	return envVars, nil
}

// getEnvVarsForSystemProbe converts System Probe Config into container env vars
func getEnvVarsForSystemProbe(dda *datadoghqv1alpha1.DatadogAgent) ([]corev1.EnvVar, error) {
	envVars := []corev1.EnvVar{}
	commonEnvVars, err := getEnvVarsCommon(dda, false)
	if err != nil {
		return nil, err
	}
	envVars = append(envVars, commonEnvVars...)
	envVars = append(envVars, dda.Spec.Agent.SystemProbe.Env...)
	return envVars, nil
}

func getEnvVarsCommon(dda *datadoghqv1alpha1.DatadogAgent, needAPIKey bool) ([]corev1.EnvVar, error) {
	envVars := []corev1.EnvVar{
		{
			Name:  datadoghqv1alpha1.DDLogLevel,
			Value: getLogLevel(dda),
		},
		{
			Name:  datadoghqv1alpha1.DDSite,
			Value: dda.Spec.Site,
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

	if needAPIKey {
		envVars = append(envVars, corev1.EnvVar{
			Name:      datadoghqv1alpha1.DDAPIKey,
			ValueFrom: getAPIKeyFromSecret(dda),
		})
	}

	if len(dda.Spec.Agent.Config.Tags) > 0 {
		tags, err := json.Marshal(dda.Spec.Agent.Config.Tags)
		if err != nil {
			return nil, err
		}

		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDTags,
			Value: string(tags),
		})
	}

	if dda.Spec.Agent.Config.DDUrl != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDddURL,
			Value: *dda.Spec.Agent.Config.DDUrl,
		})
	}

	if dda.Spec.Agent.Config.CriSocket != nil && dda.Spec.Agent.Config.CriSocket.CriSocketPath != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDCriSocketPath,
			Value: filepath.Join(datadoghqv1alpha1.HostCriSocketPathPrefix, *dda.Spec.Agent.Config.CriSocket.CriSocketPath),
		})

		if strings.HasSuffix(*dda.Spec.Agent.Config.CriSocket.CriSocketPath, "docker.sock") {
			envVars = append(envVars, corev1.EnvVar{
				Name:  datadoghqv1alpha1.DockerHost,
				Value: "unix://" + filepath.Join(datadoghqv1alpha1.HostCriSocketPathPrefix, *dda.Spec.Agent.Config.CriSocket.CriSocketPath),
			})
		}
	}

	envVars = append(envVars, dda.Spec.Agent.Env...)

	return envVars, nil
}

// getEnvVarsForAgent converts Agent Config into container env vars
func getEnvVarsForAgent(dda *datadoghqv1alpha1.DatadogAgent) ([]corev1.EnvVar, error) {
	spec := dda.Spec
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
			Name:  datadoghqv1alpha1.DDLogsContainerCollectUsingFiles,
			Value: strconv.FormatBool(*spec.Agent.Log.ContainerCollectUsingFiles),
		},
		{
			Name:  datadoghqv1alpha1.DDDogstatsdOriginDetection,
			Value: strconv.FormatBool(*spec.Agent.Config.Dogstatsd.DogstatsdOriginDetection),
		},
	}
	commonEnvVars, err := getEnvVarsCommon(dda, true)
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
				Value: getClusterAgentServiceName(dda),
			},
			{
				Name:      datadoghqv1alpha1.DDClusterAgentAuthToken,
				ValueFrom: getClusterAgentAuthToken(dda),
			},
		}
		if datadoghqv1alpha1.BoolValue(spec.ClusterAgent.Config.ClusterChecksEnabled) {
			if spec.ClusterChecksRunner == nil {
				clusterEnv = append(clusterEnv, corev1.EnvVar{
					Name:  datadoghqv1alpha1.DDExtraConfigProviders,
					Value: datadoghqv1alpha1.ClusterAndEndpointsConfigPoviders,
				})
			} else {
				clusterEnv = append(clusterEnv, corev1.EnvVar{
					Name:  datadoghqv1alpha1.DDExtraConfigProviders,
					Value: datadoghqv1alpha1.EndpointsChecksConfigProvider,
				})
			}
		}
		envVars = append(envVars, clusterEnv...)
	}

	return append(envVars, spec.Agent.Config.Env...), nil
}

// getVolumesForAgent defines volumes for the Agent
func getVolumesForAgent(dda *datadoghqv1alpha1.DatadogAgent) []corev1.Volume {
	confdVolumeSource := corev1.VolumeSource{
		EmptyDir: &corev1.EmptyDirVolumeSource{},
	}
	if dda.Spec.Agent.Config.Confd != nil {
		confdVolumeSource = corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: dda.Spec.Agent.Config.Confd.ConfigMapName,
				},
			},
		}
	}
	checksdVolumeSource := corev1.VolumeSource{
		EmptyDir: &corev1.EmptyDirVolumeSource{},
	}
	if dda.Spec.Agent.Config.Checksd != nil {
		checksdVolumeSource = corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: dda.Spec.Agent.Config.Checksd.ConfigMapName,
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

	if dda.Spec.Agent.CustomConfig != nil {
		volume := getVolumeFromCustomConfigSpec(dda.Spec.Agent.CustomConfig, getAgentCustomConfigConfigMapName(dda), datadoghqv1alpha1.AgentCustomConfigVolumeName)
		volumes = append(volumes, volume)
	}

	if dda.Spec.Agent.Config.CriSocket != nil && dda.Spec.Agent.Config.CriSocket.UseCriSocketVolume != nil && *dda.Spec.Agent.Config.CriSocket.UseCriSocketVolume {
		path := "/var/run/docker.sock"
		if dda.Spec.Agent.Config.CriSocket.CriSocketPath != nil {
			path = *dda.Spec.Agent.Config.CriSocket.CriSocketPath
		}
		criVolume := corev1.Volume{
			Name: datadoghqv1alpha1.CriSocketVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: filepath.Dir(path),
				},
			},
		}
		volumes = append(volumes, criVolume)
	}
	if datadoghqv1alpha1.BoolValue(dda.Spec.Agent.Process.Enabled) {
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
	if datadoghqv1alpha1.BoolValue(dda.Spec.Agent.SystemProbe.Enabled) {
		seccompConfigMapName := getSecCompConfigMapName(dda.Name)
		if dda.Spec.Agent.SystemProbe.SecCompCustomProfileConfigMap != "" {
			seccompConfigMapName = dda.Spec.Agent.SystemProbe.SecCompCustomProfileConfigMap
		}
		systemProbeVolumes := []corev1.Volume{
			{
				Name: datadoghqv1alpha1.SystemProbeAgentSecurityVolumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: seccompConfigMapName,
						},
					},
				},
			},
			{
				Name: datadoghqv1alpha1.SystemProbeConfigVolumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: getSystemProbeConfiConfigMapName(dda.Name),
						},
					},
				},
			},
			{
				Name: datadoghqv1alpha1.SystemProbeSecCompRootVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: getSecCompRootPath(&dda.Spec.Agent.SystemProbe),
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

	if datadoghqv1alpha1.BoolValue(dda.Spec.Agent.Log.Enabled) {
		volumes = append(volumes, []corev1.Volume{
			{
				Name: datadoghqv1alpha1.PointerVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: *dda.Spec.Agent.Log.TempStoragePath,
					},
				},
			},
			{
				Name: datadoghqv1alpha1.LogPodVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: *dda.Spec.Agent.Log.PodLogsPath,
					},
				},
			},
			{
				Name: datadoghqv1alpha1.LogContainerVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: *dda.Spec.Agent.Log.ContainerLogsPath,
					},
				},
			},
		}...)
	}

	volumes = append(volumes, dda.Spec.Agent.Config.Volumes...)
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

func getSeccompProfileName(spec *datadoghqv1alpha1.SystemProbeSpec) string {
	if spec.SecCompProfileName != "" {
		return spec.SecCompProfileName
	}
	return datadoghqv1alpha1.DefaultSeccompProfileName
}

func getVolumeFromCustomConfigSpec(cfcm *datadoghqv1alpha1.CustomConfigSpec, defaultConfigMapName, volumeName string) corev1.Volume {
	configMapName := defaultConfigMapName
	if cfcm.ConfigMap != nil {
		configMapName = cfcm.ConfigMap.Name
	}

	return corev1.Volume{
		Name: volumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configMapName,
				},
			},
		},
	}
}

func getVolumeMountFromCustomConfigSpec(cfcm *datadoghqv1alpha1.CustomConfigSpec, volumeName, volumePath, defaultSubPath string) corev1.VolumeMount {
	subPath := defaultSubPath
	if cfcm.ConfigMap != nil && cfcm.ConfigMap.FileKey != "" {
		subPath = cfcm.ConfigMap.FileKey
	}

	return corev1.VolumeMount{
		Name:      volumeName,
		MountPath: volumePath,
		SubPath:   subPath,
		ReadOnly:  true,
	}
}

// getVolumeMountsForAgent defines mounted volumes for the Agent
func getVolumeMountsForAgent(spec *datadoghqv1alpha1.DatadogAgentSpec) []corev1.VolumeMount {
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

	// Custom config (datadog.yaml) volume
	if spec.Agent.CustomConfig != nil {
		volumeMount := getVolumeMountFromCustomConfigSpec(spec.Agent.CustomConfig, datadoghqv1alpha1.AgentCustomConfigVolumeName, datadoghqv1alpha1.AgentCustomConfigVolumePath, datadoghqv1alpha1.AgentCustomConfigVolumeSubPath)
		volumeMounts = append(volumeMounts, volumeMount)
	}

	// Cri socket volume
	if *spec.Agent.Config.CriSocket.UseCriSocketVolume {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      datadoghqv1alpha1.CriSocketVolumeName,
			MountPath: filepath.Join(datadoghqv1alpha1.HostCriSocketPathPrefix, filepath.Dir(*spec.Agent.Config.CriSocket.CriSocketPath)),
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
func getVolumeMountsForProcessAgent(spec *datadoghqv1alpha1.DatadogAgentSpec) []corev1.VolumeMount {
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
			Name:      datadoghqv1alpha1.CriSocketVolumeName,
			MountPath: filepath.Join(datadoghqv1alpha1.HostCriSocketPathPrefix, filepath.Dir(*spec.Agent.Config.CriSocket.CriSocketPath)),
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
func getVolumeMountsForSystemProbe() []corev1.VolumeMount {
	// Default mounted volumes
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      datadoghqv1alpha1.SystemProbeDebugfsVolumeName,
			MountPath: datadoghqv1alpha1.SystemProbeDebugfsVolumePath,
		},
		{
			Name:      datadoghqv1alpha1.SystemProbeConfigVolumeName,
			MountPath: datadoghqv1alpha1.SystemProbeConfigVolumePath,
			SubPath:   datadoghqv1alpha1.SystemProbeConfigVolumeSubPath,
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

func getAgentVersion(dda *datadoghqv1alpha1.DatadogAgent) string {
	// TODO implement this method
	return ""
}

func getAgentServiceAccount(dda *datadoghqv1alpha1.DatadogAgent) string {
	saDefault := fmt.Sprintf("%s-agent", dda.Name)
	if dda.Spec.Agent == nil {
		return saDefault
	}
	if dda.Spec.Agent.Rbac.ServiceAccountName != nil {
		return *dda.Spec.Agent.Rbac.ServiceAccountName
	}
	return saDefault
}

// getAPIKeyFromSecret returns the Agent API key as an env var source
func getAPIKeyFromSecret(dda *datadoghqv1alpha1.DatadogAgent) *corev1.EnvVarSource {
	secretName, secretKeyName := utils.GetAPIKeySecret(dda)
	authTokenValue := &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: secretName,
			},
			Key: secretKeyName,
		},
	}
	return authTokenValue
}

// getClusterAgentAuthToken returns the Cluster Agent auth token as an env var source
func getClusterAgentAuthToken(dda *datadoghqv1alpha1.DatadogAgent) *corev1.EnvVarSource {
	authTokenValue := &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: getAuthTokenSecretName(dda),
			},
			Key: datadoghqv1alpha1.DefaultTokenKey,
		},
	}
	return authTokenValue
}

// getAppKeyFromSecret returns the Agent API key as an env var source
func getAppKeyFromSecret(dda *datadoghqv1alpha1.DatadogAgent) *corev1.EnvVarSource {
	secretName, secretKeyName := utils.GetAppKeySecret(dda)
	authTokenValue := &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: secretName,
			},
			Key: secretKeyName,
		},
	}
	return authTokenValue
}

func getClusterAgentServiceName(dda *datadoghqv1alpha1.DatadogAgent) string {
	return fmt.Sprintf("%s-%s", dda.Name, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix)
}

func getClusterAgentServiceAccount(dda *datadoghqv1alpha1.DatadogAgent) string {
	saDefault := fmt.Sprintf("%s-%s", dda.Name, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix)
	if dda.Spec.ClusterAgent == nil {
		return saDefault
	}
	if dda.Spec.ClusterAgent.Rbac.ServiceAccountName != nil {
		return *dda.Spec.ClusterAgent.Rbac.ServiceAccountName
	}
	return saDefault
}

func getClusterAgentVersion(dda *datadoghqv1alpha1.DatadogAgent) string {
	// TODO implement this method
	return ""
}

func getClusterAgentPDBName(dda *datadoghqv1alpha1.DatadogAgent) string {
	return fmt.Sprintf("%s-%s", dda.Name, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix)
}

func getClusterChecksRunnerPDBName(dda *datadoghqv1alpha1.DatadogAgent) string {
	return fmt.Sprintf("%s-%s", dda.Name, datadoghqv1alpha1.DefaultClusterChecksRunnerResourceSuffix)
}

func getMetricsServerServiceName(dda *datadoghqv1alpha1.DatadogAgent) string {
	return fmt.Sprintf("%s-%s", dda.Name, datadoghqv1alpha1.DefaultMetricsServerResourceSuffix)
}

func getClusterAgentRbacResourcesName(dda *datadoghqv1alpha1.DatadogAgent) string {
	return fmt.Sprintf("%s-%s", dda.Name, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix)
}

func getAgentRbacResourcesName(dda *datadoghqv1alpha1.DatadogAgent) string {
	return fmt.Sprintf("%s-%s", dda.Name, datadoghqv1alpha1.DefaultAgentResourceSuffix)
}

func getClusterChecksRunnerRbacResourcesName(dda *datadoghqv1alpha1.DatadogAgent) string {
	return fmt.Sprintf("%s-%s", dda.Name, datadoghqv1alpha1.DefaultClusterChecksRunnerResourceSuffix)
}

func getHPAClusterRoleBindingName(dda *datadoghqv1alpha1.DatadogAgent) string {
	return fmt.Sprintf(authDelegatorName, getClusterAgentRbacResourcesName(dda))
}

func getClusterChecksRunnerServiceAccount(dda *datadoghqv1alpha1.DatadogAgent) string {
	saDefault := fmt.Sprintf("%s-%s", dda.Name, datadoghqv1alpha1.DefaultClusterChecksRunnerResourceSuffix)
	if dda.Spec.ClusterChecksRunner == nil {
		return saDefault
	}
	if dda.Spec.ClusterChecksRunner.Rbac.ServiceAccountName != nil {
		return *dda.Spec.ClusterChecksRunner.Rbac.ServiceAccountName
	}
	return saDefault
}

func getDefaultLabels(dda *datadoghqv1alpha1.DatadogAgent, instanceName, version string) map[string]string {
	labels := make(map[string]string)
	labels[kubernetes.AppKubernetesNameLabelKey] = "datadog-agent-deployment"
	labels[kubernetes.AppKubernetesInstanceLabelKey] = instanceName
	labels[kubernetes.AppKubernetesPartOfLabelKey] = dda.Name
	labels[kubernetes.AppKubernetesVersionLabelKey] = version
	labels[kubernetes.AppKubernetesManageByLabelKey] = "datadog-operator"
	return labels
}

func getDefaultAnnotations(dda *datadoghqv1alpha1.DatadogAgent) map[string]string {
	// TODO implement this method
	_ = dda.Annotations
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

func isMetricsProviderEnabled(spec *datadoghqv1alpha1.DatadogAgentSpecClusterAgentSpec) bool {
	if spec == nil {
		return false
	}
	return spec.Config.ExternalMetrics != nil && spec.Config.ExternalMetrics.Enabled
}

func isAdmissionControllerEnabled(spec *datadoghqv1alpha1.DatadogAgentSpecClusterAgentSpec) bool {
	if spec == nil {
		return false
	}
	return spec.Config.AdmissionController != nil && spec.Config.AdmissionController.Enabled
}

func isCreateRBACEnabled(config datadoghqv1alpha1.RbacConfig) bool {
	return datadoghqv1alpha1.BoolValue(config.Create)
}

func getDefaultLivenessProbe() *corev1.Probe {
	livenessProbe := &corev1.Probe{
		InitialDelaySeconds: datadoghqv1alpha1.DefaultLivenessProbeInitialDelaySeconds,
		PeriodSeconds:       datadoghqv1alpha1.DefaultLivenessProbePeriodSeconds,
		TimeoutSeconds:      datadoghqv1alpha1.DefaultLivenessProbeTimeoutSeconds,
		SuccessThreshold:    datadoghqv1alpha1.DefaultLivenessProbeSuccessThreshold,
		FailureThreshold:    datadoghqv1alpha1.DefaultLivenessProbeFailureThreshold,
	}
	livenessProbe.HTTPGet = &corev1.HTTPGetAction{
		Path: datadoghqv1alpha1.DefaultLivenessProbeHTTPPath,
		Port: intstr.IntOrString{
			IntVal: datadoghqv1alpha1.DefaultAgentHealthPort,
		},
	}
	return livenessProbe
}

func getDefaultReadinessProbe() *corev1.Probe {
	readinessProbe := &corev1.Probe{
		InitialDelaySeconds: datadoghqv1alpha1.DefaultReadinessProbeInitialDelaySeconds,
		PeriodSeconds:       datadoghqv1alpha1.DefaultReadinessProbePeriodSeconds,
		TimeoutSeconds:      datadoghqv1alpha1.DefaultReadinessProbeTimeoutSeconds,
		SuccessThreshold:    datadoghqv1alpha1.DefaultReadinessProbeSuccessThreshold,
		FailureThreshold:    datadoghqv1alpha1.DefaultReadinessProbeFailureThreshold,
	}
	readinessProbe.HTTPGet = &corev1.HTTPGetAction{
		Path: datadoghqv1alpha1.DefaultReadinessProbeHTTPPath,
		Port: intstr.IntOrString{
			IntVal: datadoghqv1alpha1.DefaultAgentHealthPort,
		},
	}
	return readinessProbe
}

func getDefaultAPMAgentLivenessProbe() *corev1.Probe {
	livenessProbe := &corev1.Probe{
		InitialDelaySeconds: datadoghqv1alpha1.DefaultLivenessProbeInitialDelaySeconds,
		PeriodSeconds:       datadoghqv1alpha1.DefaultLivenessProbePeriodSeconds,
		TimeoutSeconds:      datadoghqv1alpha1.DefaultLivenessProbeTimeoutSeconds,
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

func updateDaemonSetStatus(ds *appsv1.DaemonSet, dsStatus *datadoghqv1alpha1.DaemonSetStatus, updateTime *metav1.Time) *datadoghqv1alpha1.DaemonSetStatus {
	if dsStatus == nil {
		dsStatus = &datadoghqv1alpha1.DaemonSetStatus{}
	}
	if ds == nil {
		dsStatus.State = string(datadoghqv1alpha1.DatadogAgentStateFailed)
		dsStatus.Status = string(datadoghqv1alpha1.DatadogAgentStateFailed)
		return dsStatus
	}
	if updateTime != nil {
		dsStatus.LastUpdate = updateTime
	}

	dsStatus.CurrentHash = getHashAnnotation(ds.Annotations)
	dsStatus.Desired = ds.Status.DesiredNumberScheduled
	dsStatus.Current = ds.Status.CurrentNumberScheduled
	dsStatus.Ready = ds.Status.NumberReady
	dsStatus.Available = ds.Status.NumberAvailable
	dsStatus.UpToDate = ds.Status.UpdatedNumberScheduled

	var deploymentState datadoghqv1alpha1.DatadogAgentState
	switch {
	case dsStatus.UpToDate != dsStatus.Desired:
		deploymentState = datadoghqv1alpha1.DatadogAgentStateUpdating
	case dsStatus.Ready == 0:
		deploymentState = datadoghqv1alpha1.DatadogAgentStateProgressing
	default:
		deploymentState = datadoghqv1alpha1.DatadogAgentStateRunning
	}

	dsStatus.State = fmt.Sprintf("%v", deploymentState)
	dsStatus.Status = fmt.Sprintf("%v (%d/%d/%d)", deploymentState, dsStatus.Desired, dsStatus.Ready, dsStatus.UpToDate)
	dsStatus.DaemonsetName = ds.ObjectMeta.Name
	return dsStatus
}

func updateExtendedDaemonSetStatus(eds *edsdatadoghqv1alpha1.ExtendedDaemonSet, dsStatus *datadoghqv1alpha1.DaemonSetStatus, updateTime *metav1.Time) *datadoghqv1alpha1.DaemonSetStatus {
	if dsStatus == nil {
		dsStatus = &datadoghqv1alpha1.DaemonSetStatus{}
	}
	if updateTime != nil {
		dsStatus.LastUpdate = updateTime
	}
	dsStatus.CurrentHash = getHashAnnotation(eds.Annotations)
	dsStatus.Desired = eds.Status.Desired
	dsStatus.Current = eds.Status.Current
	dsStatus.Ready = eds.Status.Ready
	dsStatus.Available = eds.Status.Available
	dsStatus.UpToDate = eds.Status.UpToDate

	var deploymentState datadoghqv1alpha1.DatadogAgentState
	switch {
	case eds.Status.Canary != nil:
		deploymentState = datadoghqv1alpha1.DatadogAgentStateCanary
	case dsStatus.UpToDate != dsStatus.Desired:
		deploymentState = datadoghqv1alpha1.DatadogAgentStateUpdating
	case dsStatus.Ready == 0:
		deploymentState = datadoghqv1alpha1.DatadogAgentStateProgressing
	default:
		deploymentState = datadoghqv1alpha1.DatadogAgentStateRunning
	}

	dsStatus.State = fmt.Sprintf("%v", deploymentState)
	dsStatus.Status = fmt.Sprintf("%v (%d/%d/%d)", deploymentState, dsStatus.Desired, dsStatus.Ready, dsStatus.UpToDate)
	dsStatus.DaemonsetName = eds.ObjectMeta.Name
	return dsStatus
}

func updateDeploymentStatus(dep *appsv1.Deployment, depStatus *datadoghqv1alpha1.DeploymentStatus, updateTime *metav1.Time) *datadoghqv1alpha1.DeploymentStatus {
	if depStatus == nil {
		depStatus = &datadoghqv1alpha1.DeploymentStatus{}
	}
	if dep == nil {
		depStatus.State = string(datadoghqv1alpha1.DatadogAgentStateFailed)
		depStatus.Status = string(datadoghqv1alpha1.DatadogAgentStateFailed)
		return depStatus
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

	// Deciding on deployment status based on Deployment status
	var deploymentState datadoghqv1alpha1.DatadogAgentState
	for _, condition := range dep.Status.Conditions {
		if condition.Type == appsv1.DeploymentReplicaFailure && condition.Status == corev1.ConditionTrue {
			deploymentState = datadoghqv1alpha1.DatadogAgentStateFailed
		}
	}

	if deploymentState == "" {
		switch {
		case depStatus.UpdatedReplicas != depStatus.Replicas:
			deploymentState = datadoghqv1alpha1.DatadogAgentStateUpdating
		case depStatus.ReadyReplicas == 0:
			deploymentState = datadoghqv1alpha1.DatadogAgentStateProgressing
		default:
			deploymentState = datadoghqv1alpha1.DatadogAgentStateRunning
		}
	}

	depStatus.State = fmt.Sprintf("%v", deploymentState)
	depStatus.Status = fmt.Sprintf("%v (%d/%d/%d)", deploymentState, depStatus.Replicas, depStatus.ReadyReplicas, depStatus.UpdatedReplicas)
	depStatus.DeploymentName = dep.ObjectMeta.Name
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

func getLogLevel(dda *datadoghqv1alpha1.DatadogAgent) string {
	logLevel := datadoghqv1alpha1.DefaultLogLevel
	if dda.Spec.Agent.Config.LogLevel != nil {
		logLevel = *dda.Spec.Agent.Config.LogLevel
	}
	return logLevel
}

// SetOwnerReference sets owner as a OwnerReference.
func SetOwnerReference(owner, object metav1.Object, scheme *runtime.Scheme) error {
	ro, ok := owner.(runtime.Object)
	if !ok {
		return fmt.Errorf("%T is not a runtime.Object, cannot call SetControllerReference", owner)
	}

	gvk, err := apiutil.GVKForObject(ro, scheme)
	if err != nil {
		return err
	}

	// Create a new ref
	ref := *newOwnerRef(owner, schema.GroupVersionKind{Group: gvk.Group, Version: gvk.Version, Kind: gvk.Kind})

	existingRefs := object.GetOwnerReferences()
	fi := -1
	for i, r := range existingRefs {
		if referSameObject(ref, r) {
			fi = i
		}
	}
	if fi == -1 {
		existingRefs = append(existingRefs, ref)
	} else {
		existingRefs[fi] = ref
	}

	// Update owner references
	object.SetOwnerReferences(existingRefs)
	return nil
}

// newOwnerRef creates an OwnerReference pointing to the given owner.
func newOwnerRef(owner metav1.Object, gvk schema.GroupVersionKind) *metav1.OwnerReference {
	blockOwnerDeletion := true
	isController := true
	return &metav1.OwnerReference{
		APIVersion:         gvk.GroupVersion().String(),
		Kind:               gvk.Kind,
		Name:               owner.GetName(),
		UID:                owner.GetUID(),
		BlockOwnerDeletion: &blockOwnerDeletion,
		Controller:         &isController,
	}
}

// Returns true if a and b point to the same object
func referSameObject(a, b metav1.OwnerReference) bool {
	aGV, err := schema.ParseGroupVersion(a.APIVersion)
	if err != nil {
		return false
	}

	bGV, err := schema.ParseGroupVersion(b.APIVersion)
	if err != nil {
		return false
	}

	return aGV == bGV && a.Kind == b.Kind && a.Name == b.Name
}

// namespacedName implements the datadog.MonitoredObject interface
// used to convert reconcile.Request into datadog.MonitoredObject
type namespacedName struct {
	reconcile.Request
}

func (nsn namespacedName) GetNamespace() string {
	return nsn.Namespace
}

func (nsn namespacedName) GetName() string {
	return nsn.Name
}

// getMonitoredObj returns a namespacedName from a reconcile.Request object
func getMonitoredObj(req reconcile.Request) namespacedName {
	return namespacedName{req}
}
