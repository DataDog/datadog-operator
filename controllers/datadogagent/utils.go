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

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/orchestrator"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/version"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	authDelegatorName         = "%s-auth-delegator"
	datadogOperatorName       = "DatadogAgent"
	externalMetricsReaderName = "%s-metrics-reader"
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
	if isSystemProbeEnabled(&agentdeployment.Spec) {
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

	if isAPMEnabled(&agentdeployment.Spec) {
		var apmContainers []corev1.Container

		apmContainers, err = getAPMAgentContainers(agentdeployment)
		if err != nil {
			return nil, err
		}
		containers = append(containers, apmContainers...)
	}

	if shouldAddProcessContainer(agentdeployment) {
		var processContainers []corev1.Container

		processContainers, err = getProcessContainers(agentdeployment)
		if err != nil {
			return nil, err
		}
		containers = append(containers, processContainers...)
	}
	if isSystemProbeEnabled(&agentdeployment.Spec) {
		var systemProbeContainers []corev1.Container

		systemProbeContainers, err = getSystemProbeContainers(agentdeployment)
		if err != nil {
			return nil, err
		}
		containers = append(containers, systemProbeContainers...)
	}
	if isSecurityAgentEnabled(&agentdeployment.Spec) {
		var securityAgentContainer *corev1.Container

		securityAgentContainer, err = getSecurityAgentContainer(agentdeployment)
		if err != nil {
			return nil, err
		}
		containers = append(containers, *securityAgentContainer)
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
			SecurityContext:    agentdeployment.Spec.Agent.Config.SecurityContext,
			ServiceAccountName: getAgentServiceAccount(agentdeployment),
			InitContainers:     initContainers,
			Containers:         containers,
			Volumes:            getVolumesForAgent(agentdeployment),
			Tolerations:        agentdeployment.Spec.Agent.Config.Tolerations,
			PriorityClassName:  agentdeployment.Spec.Agent.PriorityClassName,
			HostNetwork:        agentdeployment.Spec.Agent.HostNetwork,
			HostPID:            agentdeployment.Spec.Agent.HostPID || isComplianceEnabled(&agentdeployment.Spec),
			DNSPolicy:          agentdeployment.Spec.Agent.DNSPolicy,
			DNSConfig:          agentdeployment.Spec.Agent.DNSConfig,
		},
	}, nil
}

func isAPMEnabled(spec *datadoghqv1alpha1.DatadogAgentSpec) bool {
	return datadoghqv1alpha1.BoolValue(spec.Agent.Apm.Enabled)
}

func isSystemProbeEnabled(spec *datadoghqv1alpha1.DatadogAgentSpec) bool {
	return datadoghqv1alpha1.BoolValue(spec.Agent.SystemProbe.Enabled) || datadoghqv1alpha1.BoolValue(spec.Agent.Security.Runtime.Enabled)
}

func isComplianceEnabled(spec *datadoghqv1alpha1.DatadogAgentSpec) bool {
	return datadoghqv1alpha1.BoolValue(spec.Agent.Security.Compliance.Enabled)
}

func isRuntimeSecurityEnabled(spec *datadoghqv1alpha1.DatadogAgentSpec) bool {
	return datadoghqv1alpha1.BoolValue(spec.Agent.Security.Runtime.Enabled)
}

func isSecurityAgentEnabled(spec *datadoghqv1alpha1.DatadogAgentSpec) bool {
	return datadoghqv1alpha1.BoolValue(spec.Agent.Security.Compliance.Enabled) || datadoghqv1alpha1.BoolValue(spec.Agent.Security.Runtime.Enabled)
}

func isSyscallMonitorEnabled(spec *datadoghqv1alpha1.DatadogAgentSpec) bool {
	if !isRuntimeSecurityEnabled(spec) {
		return false
	}

	return spec.Agent.Security.Runtime.SyscallMonitor != nil && datadoghqv1alpha1.BoolValue(spec.Agent.Security.Runtime.SyscallMonitor.Enabled)
}

// shouldAddProcessContainer returns whether the process container should be added.
// It returns false if the feature is "disabled" or neither OrchestratorExplorer nor ProcessContainer are set
// Note: the container will still be added even if it is set to "false".
func shouldAddProcessContainer(dda *datadoghqv1alpha1.DatadogAgent) bool {
	// we still want to have the process-agent if the orchestrator explorer is activated
	if dda.Spec.Agent == nil || dda.Spec.Agent.Process.Enabled == nil {
		return isOrchestratorExplorerEnabled(dda)
	}
	return datadoghqv1alpha1.BoolValue(dda.Spec.Agent.Process.Enabled) || isOrchestratorExplorerEnabled(dda)
}

// processCollectionEnabled
// only collect process information if it is directly specified.
func processCollectionEnabled(dda *datadoghqv1alpha1.DatadogAgent) bool {
	if dda.Spec.Agent == nil || dda.Spec.Agent.Process.Enabled == nil || dda.Spec.Agent.Process.ProcessCollectionEnabled == nil {
		return false
	}
	if *dda.Spec.Agent.Process.ProcessCollectionEnabled && *dda.Spec.Agent.Process.Enabled {
		return true
	}
	return false
}

func isOrchestratorExplorerEnabled(dda *datadoghqv1alpha1.DatadogAgent) bool {
	features := dda.Spec.Features
	if features == nil || features.OrchestratorExplorer == nil {
		return false
	}
	return datadoghqv1alpha1.BoolValue(features.OrchestratorExplorer.Enabled)
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
		VolumeMounts:  getVolumeMountsForAPMAgent(&dda.Spec),
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
				Add: []corev1.Capability{"SYS_ADMIN", "SYS_RESOURCE", "SYS_PTRACE", "NET_ADMIN", "NET_BROADCAST", "IPC_LOCK"},
			},
		},
		Env:          systemProbeEnvVars,
		VolumeMounts: getVolumeMountsForSystemProbe(dda),
	}
	if agentSpec.SystemProbe.SecurityContext != nil {
		systemProbe.SecurityContext = agentSpec.SystemProbe.SecurityContext.DeepCopy()
	}
	if agentSpec.SystemProbe.Resources != nil {
		systemProbe.Resources = *agentSpec.SystemProbe.Resources
	}

	return []corev1.Container{systemProbe}, nil
}

func getSecurityAgentContainer(dda *datadoghqv1alpha1.DatadogAgent) (*corev1.Container, error) {
	agentSpec := dda.Spec.Agent
	envVars, err := getEnvVarsForSecurityAgent(dda)
	if err != nil {
		return nil, err
	}

	securityAgentContainer := &corev1.Container{
		Name:            "security-agent",
		Image:           agentSpec.Image.Name,
		ImagePullPolicy: *agentSpec.Image.PullPolicy,
		Command: []string{
			"security-agent",
			"start",
			"-c=/etc/datadog-agent/datadog.yaml",
		},
		SecurityContext: &corev1.SecurityContext{
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{"AUDIT_CONTROL", "AUDIT_READ"},
			},
		},
		Resources:    *agentSpec.Config.Resources,
		Env:          envVars,
		VolumeMounts: getVolumeMountsForSecurityAgent(dda),
	}

	return securityAgentContainer, nil
}

func getInitContainers(dda *datadoghqv1alpha1.DatadogAgent) ([]corev1.Container, error) {
	spec := &dda.Spec
	volumeMounts := getVolumeMountsForAgent(spec)
	envVars, err := getEnvVarsForAgent(dda)
	if err != nil {
		return nil, err
	}

	containers := getConfigInitContainers(spec, volumeMounts, envVars)

	if isSystemProbeEnabled(&dda.Spec) {
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

// getConfigInitContainers returns the init containers necessary to set up the
// agent's configuration volume.
func getConfigInitContainers(spec *datadoghqv1alpha1.DatadogAgentSpec, volumeMounts []corev1.VolumeMount, envVars []corev1.EnvVar) []corev1.Container {
	return []corev1.Container{
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
}

// getEnvVarsForAPMAgent converts APM Agent Config into container env vars
func getEnvVarsForAPMAgent(dda *datadoghqv1alpha1.DatadogAgent) ([]corev1.EnvVar, error) {
	envVars := []corev1.EnvVar{
		{
			Name:  datadoghqv1alpha1.DDAPMEnabled,
			Value: strconv.FormatBool(isAPMEnabled(&dda.Spec)),
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
			Name:  datadoghqv1alpha1.DDSystemProbeAgentEnabled,
			Value: strconv.FormatBool(isSystemProbeEnabled(&dda.Spec)),
		},
	}

	if processCollectionEnabled(dda) {
		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDProcessAgentEnabled,
			Value: "true",
		})
	}

	if isOrchestratorExplorerEnabled(dda) {
		envs, err := orchestrator.EnvVars(dda.Spec.Features.OrchestratorExplorer)
		if err != nil {
			return nil, err
		}
		envVars = append(envVars, envs...)
		envVars = append(envVars, orchestrator.ClusterID())
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
			Name: datadoghqv1alpha1.DDKubeletHost,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: FieldPathStatusHostIP,
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

	if dda.Spec.Agent.Config.CriSocket != nil {
		if dda.Spec.Agent.Config.CriSocket.CriSocketPath != nil {
			envVars = append(envVars, corev1.EnvVar{
				Name:  datadoghqv1alpha1.DDCriSocketPath,
				Value: filepath.Join(datadoghqv1alpha1.HostCriSocketPathPrefix, *dda.Spec.Agent.Config.CriSocket.CriSocketPath),
			})
		} else if dda.Spec.Agent.Config.CriSocket.DockerSocketPath != nil {
			envVars = append(envVars, corev1.EnvVar{
				Name:  datadoghqv1alpha1.DockerHost,
				Value: "unix://" + filepath.Join(datadoghqv1alpha1.HostCriSocketPathPrefix, *dda.Spec.Agent.Config.CriSocket.DockerSocketPath),
			})
		}
	}

	envVars = append(envVars, dda.Spec.Agent.Env...)

	if dda.Spec.Site != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDSite,
			Value: dda.Spec.Site,
		})
	}

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
			Name:  datadoghqv1alpha1.DDLogsConfigOpenFilesLimit,
			Value: strconv.FormatInt(int64(*spec.Agent.Log.OpenFilesLimit), 10),
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
			// Remove ksm v1 conf if the cluster checks are enabled and the ksm core is enabled
			if isKSMCoreEnabled(dda) {
				ignoreAutoConfMutated := false
				for i, e := range spec.Agent.Config.Env {
					if e.Name == datadoghqv1alpha1.DDIgnoreAutoConf {
						spec.Agent.Config.Env[i].Value = fmt.Sprintf("%s kubernetes_state", e.Value)
						ignoreAutoConfMutated = true
					}
				}
				if !ignoreAutoConfMutated {
					envVars = append(envVars, corev1.EnvVar{
						Name:  datadoghqv1alpha1.DDIgnoreAutoConf,
						Value: "kubernetes_state",
					})
				}
			}
		}
		envVars = append(envVars, clusterEnv...)
	}

	return append(envVars, spec.Agent.Config.Env...), nil
}

// getEnvVarsForSecurityAgent returns env vars for security agent
func getEnvVarsForSecurityAgent(dda *datadoghqv1alpha1.DatadogAgent) ([]corev1.EnvVar, error) {
	spec := dda.Spec

	complianceEnabled := isComplianceEnabled(&dda.Spec)
	runtimeEnabled := isRuntimeSecurityEnabled(&dda.Spec)

	envVars := []corev1.EnvVar{
		{
			Name:  datadoghqv1alpha1.DDComplianceConfigEnabled,
			Value: strconv.FormatBool(complianceEnabled),
		},
	}
	if complianceEnabled {
		if dda.Spec.Agent.Security.Compliance.CheckInterval != nil {
			envVars = append(envVars, corev1.EnvVar{
				Name:  datadoghqv1alpha1.DDComplianceConfigCheckInterval,
				Value: strconv.FormatInt(dda.Spec.Agent.Security.Compliance.CheckInterval.Nanoseconds(), 10),
			})
		}

		if dda.Spec.Agent.Security.Compliance.ConfigDir != nil {
			envVars = append(envVars, corev1.EnvVar{
				Name:  datadoghqv1alpha1.DDComplianceConfigDir,
				Value: datadoghqv1alpha1.SecurityAgentComplianceConfigDirVolumePath,
			})
		}

		envVars = append(envVars, corev1.EnvVar{
			Name:  "HOST_ROOT",
			Value: datadoghqv1alpha1.HostRootVolumePath,
		})

	}

	envVars = append(envVars, corev1.EnvVar{
		Name:  datadoghqv1alpha1.DDRuntimeSecurityConfigEnabled,
		Value: strconv.FormatBool(runtimeEnabled),
	})

	if runtimeEnabled {
		if dda.Spec.Agent.Security.Runtime.PoliciesDir != nil {
			envVars = append(envVars, corev1.EnvVar{
				Name:  datadoghqv1alpha1.DDRuntimeSecurityConfigPoliciesDir,
				Value: datadoghqv1alpha1.SecurityAgentRuntimePoliciesDirVolumePath,
			})
		}
		envVars = append(envVars, []corev1.EnvVar{
			{
				Name:  datadoghqv1alpha1.DDRuntimeSecurityConfigSocket,
				Value: filepath.Join(datadoghqv1alpha1.SystemProbeSocketVolumePath, "runtime-security.sock"),
			},
			{
				Name:  datadoghqv1alpha1.DDRuntimeSecurityConfigSyscallMonitorEnabled,
				Value: strconv.FormatBool(isSyscallMonitorEnabled(&dda.Spec)),
			},
		}...)
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
		envVars = append(envVars, clusterEnv...)
	}

	return append(envVars, spec.Agent.Config.Env...), nil
}

// getVolumesForAgent defines volumes for the Agent
func getVolumesForAgent(dda *datadoghqv1alpha1.DatadogAgent) []corev1.Volume {
	volumes := []corev1.Volume{
		{
			Name: datadoghqv1alpha1.InstallInfoVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: getInstallInfoConfigMapName(dda),
					},
				},
			},
		},
		getVolumeForConfd(dda),
		getVolumeForChecksd(dda),
		getVolumeForConfig(),
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

	// Dogstatsd volume
	if datadoghqv1alpha1.BoolValue(dda.Spec.Agent.Config.Dogstatsd.UseDogStatsDSocketVolume) {
		volumeType := corev1.HostPathDirectoryOrCreate
		hostPath := *dda.Spec.Agent.Config.Dogstatsd.HostSocketPath

		dsdsockerVolume := corev1.Volume{
			Name: datadoghqv1alpha1.DogstatsdSockerVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: hostPath,
					Type: &volumeType,
				},
			},
		}
		volumes = append(volumes, dsdsockerVolume)
	}

	if dda.Spec.Agent.Config.CriSocket != nil {
		path := ""
		if dda.Spec.Agent.Config.CriSocket.DockerSocketPath != nil {
			path = *dda.Spec.Agent.Config.CriSocket.DockerSocketPath
		} else if dda.Spec.Agent.Config.CriSocket.CriSocketPath != nil {
			path = *dda.Spec.Agent.Config.CriSocket.CriSocketPath
		}
		if path != "" {
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
	}
	if shouldAddProcessContainer(dda) || isComplianceEnabled(&dda.Spec) {
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

	if isSystemProbeEnabled(&dda.Spec) {
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
							Name: getSystemProbeConfigConfigMapName(dda.Name),
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

		if datadoghqv1alpha1.BoolValue(dda.Spec.Agent.SystemProbe.EnableTCPQueueLength) ||
			datadoghqv1alpha1.BoolValue(dda.Spec.Agent.SystemProbe.EnableOOMKill) {
			volumes = append(volumes, []corev1.Volume{
				{
					Name: datadoghqv1alpha1.SystemProbeLibModulesVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: datadoghqv1alpha1.SystemProbeLibModulesVolumePath,
						},
					},
				},
				{
					Name: datadoghqv1alpha1.SystemProbeUsrSrcVolumeName,
					VolumeSource: corev1.VolumeSource{
						HostPath: &corev1.HostPathVolumeSource{
							Path: datadoghqv1alpha1.SystemProbeUsrSrcVolumePath,
						},
					},
				},
			}...)
		}
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

	if isComplianceEnabled(&dda.Spec) {
		groupVolume := corev1.Volume{
			Name: datadoghqv1alpha1.GroupVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: datadoghqv1alpha1.GroupVolumePath,
				},
			},
		}
		volumes = append(volumes, groupVolume)

		hostRootVolume := corev1.Volume{
			Name: datadoghqv1alpha1.HostRootVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/",
				},
			},
		}
		volumes = append(volumes, hostRootVolume)

		if dda.Spec.Agent.Security.Compliance.ConfigDir != nil {
			volumes = append(volumes, corev1.Volume{
				Name: datadoghqv1alpha1.SecurityAgentComplianceConfigDirVolumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: dda.Spec.Agent.Security.Compliance.ConfigDir.ConfigMapName,
						},
					},
				},
			})
		}
	}

	if isRuntimeSecurityEnabled(&dda.Spec) {
		if dda.Spec.Agent.Security.Runtime.PoliciesDir != nil {
			volumes = append(volumes, corev1.Volume{
				Name: datadoghqv1alpha1.SecurityAgentRuntimePoliciesDirVolumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: dda.Spec.Agent.Security.Runtime.PoliciesDir.ConfigMapName,
						},
					},
				},
			})
		}
	}

	volumes = append(volumes, dda.Spec.Agent.Config.Volumes...)
	return volumes
}

func getVolumeForConfd(dda *datadoghqv1alpha1.DatadogAgent) corev1.Volume {
	source := corev1.VolumeSource{
		EmptyDir: &corev1.EmptyDirVolumeSource{},
	}
	if dda.Spec.Agent.Config.Confd != nil {
		source = corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: dda.Spec.Agent.Config.Confd.ConfigMapName,
				},
			},
		}
	}

	return corev1.Volume{
		Name:         datadoghqv1alpha1.ConfdVolumeName,
		VolumeSource: source,
	}
}

func getVolumeForChecksd(dda *datadoghqv1alpha1.DatadogAgent) corev1.Volume {
	source := corev1.VolumeSource{
		EmptyDir: &corev1.EmptyDirVolumeSource{},
	}
	if dda.Spec.Agent.Config.Checksd != nil {
		source = corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: dda.Spec.Agent.Config.Checksd.ConfigMapName,
				},
			},
		}
	}

	return corev1.Volume{
		Name:         datadoghqv1alpha1.ChecksdVolumeName,
		VolumeSource: source,
	}
}

func getVolumeForConfig() corev1.Volume {
	return corev1.Volume{
		Name: datadoghqv1alpha1.ConfigVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
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
			Name:      datadoghqv1alpha1.InstallInfoVolumeName,
			SubPath:   datadoghqv1alpha1.InstallInfoVolumeSubPath,
			MountPath: datadoghqv1alpha1.InstallInfoVolumePath,
			ReadOnly:  datadoghqv1alpha1.InstallInfoVolumeReadOnly,
		},
		getVolumeMountForConfd(),
		getVolumeMountForChecksd(),
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

	// Add configuration volumesMount default and extra config (datadog.yaml) volume
	volumeMounts = append(volumeMounts, getVolumeMountForConfig(spec.Agent.CustomConfig)...)

	// Cri socket volume
	if spec.Agent.Config.CriSocket != nil {
		path := ""
		if spec.Agent.Config.CriSocket.DockerSocketPath != nil {
			path = *spec.Agent.Config.CriSocket.DockerSocketPath
		} else if spec.Agent.Config.CriSocket.CriSocketPath != nil {
			path = *spec.Agent.Config.CriSocket.CriSocketPath
		}
		if path != "" {
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      datadoghqv1alpha1.CriSocketVolumeName,
				MountPath: filepath.Join(datadoghqv1alpha1.HostCriSocketPathPrefix, filepath.Dir(path)),
				ReadOnly:  true,
			})
		}
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

	// SystemProbe volumes
	if datadoghqv1alpha1.BoolValue(spec.Agent.SystemProbe.Enabled) {
		volumeMounts = append(volumeMounts, []corev1.VolumeMount{
			{
				Name:      datadoghqv1alpha1.SystemProbeSocketVolumeName,
				MountPath: datadoghqv1alpha1.SystemProbeSocketVolumePath,
				ReadOnly:  true,
			},
			{
				Name:      datadoghqv1alpha1.SystemProbeConfigVolumeName,
				MountPath: datadoghqv1alpha1.SystemProbeConfigVolumePath,
				SubPath:   datadoghqv1alpha1.SystemProbeConfigVolumeSubPath,
			},
		}...)
	}

	return append(volumeMounts, spec.Agent.Config.VolumeMounts...)
}

func getVolumeMountForConfig(customConfig *datadoghqv1alpha1.CustomConfigSpec) []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      datadoghqv1alpha1.ConfigVolumeName,
			MountPath: datadoghqv1alpha1.ConfigVolumePath,
		},
	}

	// Custom config (datadog.yaml) volume
	if customConfig != nil {
		volumeMount := getVolumeMountFromCustomConfigSpec(customConfig, datadoghqv1alpha1.AgentCustomConfigVolumeName, datadoghqv1alpha1.AgentCustomConfigVolumePath, datadoghqv1alpha1.AgentCustomConfigVolumeSubPath)
		volumeMounts = append(volumeMounts, volumeMount)
	}

	return volumeMounts
}

func getVolumeMountForConfd() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      datadoghqv1alpha1.ConfdVolumeName,
		MountPath: datadoghqv1alpha1.ConfdVolumePath,
		ReadOnly:  true,
	}
}
func getVolumeMountForChecksd() corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      datadoghqv1alpha1.ChecksdVolumeName,
		MountPath: datadoghqv1alpha1.ChecksdVolumePath,
		ReadOnly:  true,
	}
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

	// Add configuration mount
	volumeMounts = append(volumeMounts, getVolumeMountForConfig(spec.Agent.CustomConfig)...)

	// Cri socket volume
	if spec.Agent.Config.CriSocket != nil {
		path := ""
		if spec.Agent.Config.CriSocket.DockerSocketPath != nil {
			path = *spec.Agent.Config.CriSocket.DockerSocketPath
		} else if spec.Agent.Config.CriSocket.CriSocketPath != nil {
			path = *spec.Agent.Config.CriSocket.CriSocketPath
		}
		if path != "" {
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      datadoghqv1alpha1.CriSocketVolumeName,
				MountPath: filepath.Join(datadoghqv1alpha1.HostCriSocketPathPrefix, filepath.Dir(path)),
				ReadOnly:  true,
			})
		}
	}

	if datadoghqv1alpha1.BoolValue(spec.Agent.SystemProbe.Enabled) {
		volumeMounts = append(volumeMounts, []corev1.VolumeMount{
			{
				Name:      datadoghqv1alpha1.SystemProbeSocketVolumeName,
				MountPath: datadoghqv1alpha1.SystemProbeSocketVolumePath,
				ReadOnly:  true,
			},
			{
				Name:      datadoghqv1alpha1.SystemProbeConfigVolumeName,
				MountPath: datadoghqv1alpha1.SystemProbeConfigVolumePath,
				SubPath:   datadoghqv1alpha1.SystemProbeConfigVolumeSubPath,
			},
		}...)
	}

	return volumeMounts
}

// getVolumeMountsForAgent defines mounted volumes for the Process Agent
func getVolumeMountsForAPMAgent(spec *datadoghqv1alpha1.DatadogAgentSpec) []corev1.VolumeMount {
	// Default mounted volumes
	volumeMounts := []corev1.VolumeMount{}

	// Add configuration volumesMount default and custom config (datadog.yaml) volume
	volumeMounts = append(volumeMounts, getVolumeMountForConfig(spec.Agent.CustomConfig)...)

	return volumeMounts
}

// getVolumeMountsForSystemProbe defines mounted volumes for the SystemProbe
func getVolumeMountsForSystemProbe(dda *datadoghqv1alpha1.DatadogAgent) []corev1.VolumeMount {
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

	if datadoghqv1alpha1.BoolValue(dda.Spec.Agent.SystemProbe.EnableTCPQueueLength) ||
		datadoghqv1alpha1.BoolValue(dda.Spec.Agent.SystemProbe.EnableOOMKill) {
		volumeMounts = append(volumeMounts, []corev1.VolumeMount{
			{
				Name:      datadoghqv1alpha1.SystemProbeLibModulesVolumeName,
				MountPath: datadoghqv1alpha1.SystemProbeLibModulesVolumePath,
				ReadOnly:  true,
			},
			{
				Name:      datadoghqv1alpha1.SystemProbeUsrSrcVolumeName,
				MountPath: datadoghqv1alpha1.SystemProbeUsrSrcVolumePath,
				ReadOnly:  true,
			},
		}...)
	}

	if isRuntimeSecurityEnabled(&dda.Spec) && dda.Spec.Agent.Security.Runtime.PoliciesDir != nil {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      datadoghqv1alpha1.SecurityAgentRuntimePoliciesDirVolumeName,
			MountPath: datadoghqv1alpha1.SecurityAgentRuntimePoliciesDirVolumePath,
			ReadOnly:  true,
		})
	}

	return volumeMounts
}

// getVolumeMountsForSecurityAgent defines mounted volumes for the Security Agent
func getVolumeMountsForSecurityAgent(dda *datadoghqv1alpha1.DatadogAgent) []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      datadoghqv1alpha1.ConfigVolumeName,
			MountPath: datadoghqv1alpha1.ConfigVolumePath,
		},
	}

	complianceEnabled := isComplianceEnabled(&dda.Spec)
	runtimeEnabled := isRuntimeSecurityEnabled(&dda.Spec)

	if complianceEnabled {
		volumeMounts = append(volumeMounts, []corev1.VolumeMount{
			{
				Name:      datadoghqv1alpha1.CgroupsVolumeName,
				MountPath: datadoghqv1alpha1.CgroupsVolumePath,
				ReadOnly:  true,
			},
			{
				Name:      datadoghqv1alpha1.PasswdVolumeName,
				MountPath: datadoghqv1alpha1.PasswdVolumePath,
				ReadOnly:  true,
			},
			{
				Name:      datadoghqv1alpha1.GroupVolumeName,
				MountPath: datadoghqv1alpha1.GroupVolumePath,
				ReadOnly:  true,
			},
			{
				Name:      datadoghqv1alpha1.ProcVolumeName,
				MountPath: datadoghqv1alpha1.ProcVolumePath,
				ReadOnly:  true,
			},
			{
				Name:      datadoghqv1alpha1.HostRootVolumeName,
				MountPath: datadoghqv1alpha1.HostRootVolumePath,
				ReadOnly:  true,
			},
		}...)

	}

	spec := dda.Spec
	// Cri socket volume
	if spec.Agent.Config.CriSocket != nil {
		path := ""
		if spec.Agent.Config.CriSocket.DockerSocketPath != nil {
			path = *spec.Agent.Config.CriSocket.DockerSocketPath
		} else if spec.Agent.Config.CriSocket.CriSocketPath != nil {
			path = *spec.Agent.Config.CriSocket.CriSocketPath
		}
		if path != "" {
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      datadoghqv1alpha1.CriSocketVolumeName,
				MountPath: filepath.Join(datadoghqv1alpha1.HostCriSocketPathPrefix, filepath.Dir(path)),
				ReadOnly:  true,
			})
			if complianceEnabled {
				// Additional mount for runtime socket under hostroot
				volumeMounts = append(volumeMounts, corev1.VolumeMount{
					Name:      datadoghqv1alpha1.CriSocketVolumeName,
					MountPath: filepath.Join(datadoghqv1alpha1.HostRootVolumePath, filepath.Dir(path)),
					ReadOnly:  true,
				})
			}
		}
	}

	if runtimeEnabled {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      datadoghqv1alpha1.SystemProbeSocketVolumeName,
			MountPath: datadoghqv1alpha1.SystemProbeSocketVolumePath,
			ReadOnly:  true,
		})

	}

	if complianceEnabled && dda.Spec.Agent.Security.Compliance.ConfigDir != nil {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      datadoghqv1alpha1.SecurityAgentComplianceConfigDirVolumeName,
			MountPath: datadoghqv1alpha1.SecurityAgentComplianceConfigDirVolumePath,
			ReadOnly:  true,
		})
	}
	if runtimeEnabled && dda.Spec.Agent.Security.Runtime.PoliciesDir != nil {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      datadoghqv1alpha1.SecurityAgentRuntimePoliciesDirVolumeName,
			MountPath: datadoghqv1alpha1.SecurityAgentRuntimePoliciesDirVolumePath,
			ReadOnly:  true,
		})
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

func getMetricsServerAPIServiceName() string {
	return fmt.Sprintf("v1beta1.external.metrics.k8s.io")
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

func getExternalMetricsReaderClusterRoleName(dda *datadoghqv1alpha1.DatadogAgent, versionInfo *version.Info) string {
	if versionInfo != nil && strings.Contains(versionInfo.GitVersion, "-gke.") {
		// For GKE clusters the name of the role is hardcoded and cannot be changed - HPA controller expects this name
		return "external-metrics-reader"
	}
	return fmt.Sprintf(externalMetricsReaderName, getClusterAgentRbacResourcesName(dda))
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

func isKSMCoreEnabled(dda *datadoghqv1alpha1.DatadogAgent) bool {
	if dda.Spec.Features == nil || dda.Spec.Features.KubeStateMetricsCore == nil {
		return false
	}
	if dda.Spec.Features.KubeStateMetricsCore.Enabled != nil {
		return *dda.Spec.Features.KubeStateMetricsCore.Enabled
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
