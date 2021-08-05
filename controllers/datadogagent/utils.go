// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/orchestrator"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/gobwas/glob"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/yaml"
)

const (
	authDelegatorName         string = "%s-auth-delegator"
	externalMetricsReaderName string = "%s-metrics-reader"
	localDogstatsdSocketPath  string = "/var/run/datadog/statsd"
	localAPMSocketPath        string = "/var/run/datadog/apm"
	defaultRuntimeDir         string = "/var/run"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// getTokenSecretName returns the token secret name
func getAuthTokenSecretName(dda *datadoghqv1alpha1.DatadogAgent) string {
	return dda.Name
}

// newAgentPodTemplate generates a PodTemplate from a DatadogAgent spec
func newAgentPodTemplate(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, selector *metav1.LabelSelector) (*corev1.PodTemplateSpec, error) {
	// copy Agent Spec to configure Agent Pod Template
	labels := getDefaultLabels(dda, "agent", getAgentVersion(dda))
	labels[datadoghqv1alpha1.AgentDeploymentNameLabelKey] = dda.Name
	labels[datadoghqv1alpha1.AgentDeploymentComponentLabelKey] = "agent"

	for key, val := range dda.Spec.Agent.AdditionalLabels {
		labels[key] = val
	}

	if selector != nil {
		for key, val := range selector.MatchLabels {
			labels[key] = val
		}
	}

	annotations := getDefaultAnnotations(dda)
	if isSystemProbeEnabled(&dda.Spec) {
		annotations[datadoghqv1alpha1.SysteProbeAppArmorAnnotationKey] = getAppArmorProfileName(dda.Spec.Agent.SystemProbe)
		annotations[datadoghqv1alpha1.SysteProbeSeccompAnnotationKey] = getSeccompProfileName(dda.Spec.Agent.SystemProbe)
	}

	for key, val := range dda.Spec.Agent.AdditionalAnnotations {
		annotations[key] = val
	}

	image := getImage(dda.Spec.Agent.Image, dda.Spec.Registry, true)
	containers := []corev1.Container{}
	agentContainer, err := getAgentContainer(logger, dda, image)
	if err != nil {
		return nil, err
	}
	containers = append(containers, *agentContainer)

	if isAPMEnabled(&dda.Spec) {
		var apmContainers []corev1.Container

		apmContainers, err = getAPMAgentContainers(dda, image)
		if err != nil {
			return nil, err
		}
		containers = append(containers, apmContainers...)
	}

	if shouldAddProcessContainer(dda) {
		var processContainers []corev1.Container

		processContainers, err = getProcessContainers(dda, image)
		if err != nil {
			return nil, err
		}
		containers = append(containers, processContainers...)
	}
	if isSystemProbeEnabled(&dda.Spec) {
		var systemProbeContainers []corev1.Container

		systemProbeContainers, err = getSystemProbeContainers(dda, image)
		if err != nil {
			return nil, err
		}
		containers = append(containers, systemProbeContainers...)
	}
	if isSecurityAgentEnabled(&dda.Spec) {
		var securityAgentContainer *corev1.Container

		securityAgentContainer, err = getSecurityAgentContainer(dda, image)
		if err != nil {
			return nil, err
		}
		containers = append(containers, *securityAgentContainer)
	}

	var initContainers []corev1.Container
	initContainers, err = getInitContainers(logger, dda, image)
	if err != nil {
		return nil, err
	}

	return &corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: dda.Name,
			Namespace:    dda.Namespace,
			Labels:       labels,
			Annotations:  annotations,
		},
		Spec: corev1.PodSpec{
			SecurityContext:    dda.Spec.Agent.Config.SecurityContext,
			ServiceAccountName: getAgentServiceAccount(dda),
			InitContainers:     initContainers,
			Containers:         containers,
			Volumes:            getVolumesForAgent(dda),
			Tolerations:        dda.Spec.Agent.Config.Tolerations,
			PriorityClassName:  dda.Spec.Agent.PriorityClassName,
			HostNetwork:        dda.Spec.Agent.HostNetwork,
			HostPID:            dda.Spec.Agent.HostPID || isComplianceEnabled(&dda.Spec),
			DNSPolicy:          dda.Spec.Agent.DNSPolicy,
			DNSConfig:          dda.Spec.Agent.DNSConfig,
			Affinity:           dda.Spec.Agent.Affinity,
		},
	}, nil
}

func isClusterChecksEnabled(spec *datadoghqv1alpha1.DatadogAgentSpec) bool {
	if spec.ClusterAgent.Config == nil {
		return false
	}
	return datadoghqv1alpha1.BoolValue(spec.ClusterAgent.Config.ClusterChecksEnabled)
}

func isAPMEnabled(spec *datadoghqv1alpha1.DatadogAgentSpec) bool {
	if spec.Agent.Apm == nil {
		return false
	}
	return datadoghqv1alpha1.BoolValue(spec.Agent.Apm.Enabled)
}

func isAPMUDSEnabled(spec *datadoghqv1alpha1.DatadogAgentSpec) bool {
	if !isAPMEnabled(spec) || spec.Agent.Apm.UnixDomainSocket == nil {
		return false
	}
	return datadoghqv1alpha1.BoolValue(spec.Agent.Apm.UnixDomainSocket.Enabled)
}

func isSystemProbeEnabled(spec *datadoghqv1alpha1.DatadogAgentSpec) bool {
	if spec.Agent.SystemProbe == nil {
		return false
	}
	return datadoghqv1alpha1.BoolValue(spec.Agent.SystemProbe.Enabled)
}

func isNetworkMonitoringEnabled(spec *datadoghqv1alpha1.DatadogAgentSpec) bool {
	if spec.Features.NetworkMonitoring == nil {
		return false
	}
	return datadoghqv1alpha1.BoolValue(spec.Features.NetworkMonitoring.Enabled)
}

func isComplianceEnabled(spec *datadoghqv1alpha1.DatadogAgentSpec) bool {
	if spec.Agent.Security == nil {
		return false
	}
	return datadoghqv1alpha1.BoolValue(spec.Agent.Security.Compliance.Enabled)
}

func isRuntimeSecurityEnabled(spec *datadoghqv1alpha1.DatadogAgentSpec) bool {
	if spec.Agent.Security == nil {
		return false
	}
	return datadoghqv1alpha1.BoolValue(spec.Agent.Security.Runtime.Enabled)
}

func isSecurityAgentEnabled(spec *datadoghqv1alpha1.DatadogAgentSpec) bool {
	if spec.Agent.Security == nil {
		return false
	}
	return datadoghqv1alpha1.BoolValue(spec.Agent.Security.Compliance.Enabled) || datadoghqv1alpha1.BoolValue(spec.Agent.Security.Runtime.Enabled)
}

func isSyscallMonitorEnabled(spec *datadoghqv1alpha1.DatadogAgentSpec) bool {
	if !isRuntimeSecurityEnabled(spec) {
		return false
	}
	if spec.Agent.Security.Runtime.SyscallMonitor == nil {
		return false
	}
	return datadoghqv1alpha1.BoolValue(spec.Agent.Security.Runtime.SyscallMonitor.Enabled)
}

func isDogstatsdConfigured(spec *datadoghqv1alpha1.DatadogAgentSpec) bool {
	if spec.Agent.Config == nil || spec.Agent.Config.Dogstatsd == nil {
		return false
	}
	return true
}

func isDogstatsdUDSEnabled(spec *datadoghqv1alpha1.DatadogAgentSpec) bool {
	if !isDogstatsdConfigured(spec) || spec.Agent.Config.Dogstatsd.UnixDomainSocket == nil {
		return false
	}
	return datadoghqv1alpha1.BoolValue(spec.Agent.Config.Dogstatsd.UnixDomainSocket.Enabled)
}

// shouldAddProcessContainer returns whether the process container should be added.
// It returns false if the feature is "disabled" or neither OrchestratorExplorer nor ProcessContainer are set
// Note: the container will still be added even if it is set to "false".
func shouldAddProcessContainer(dda *datadoghqv1alpha1.DatadogAgent) bool {
	// we need to have the process-agent if the orchestrator explorer is activated
	if dda.Spec.Agent.Process == nil {
		return false
	}
	return datadoghqv1alpha1.BoolValue(dda.Spec.Agent.Process.Enabled) || isOrchestratorExplorerEnabled(dda)
}

// processCollectionEnabled
// only collect process information if it is directly specified.
func processCollectionEnabled(dda *datadoghqv1alpha1.DatadogAgent) bool {
	if dda.Spec.Agent.Process == nil {
		return false
	}
	return datadoghqv1alpha1.BoolValue(dda.Spec.Agent.Process.ProcessCollectionEnabled) &&
		datadoghqv1alpha1.BoolValue(dda.Spec.Agent.Process.Enabled)
}

func isOrchestratorExplorerEnabled(dda *datadoghqv1alpha1.DatadogAgent) bool {
	if dda.Spec.Features.OrchestratorExplorer == nil {
		return false
	}
	return datadoghqv1alpha1.BoolValue(dda.Spec.Features.OrchestratorExplorer.Enabled)
}

func getAgentDeploymentStrategy(dda *datadoghqv1alpha1.DatadogAgent) (*datadoghqv1alpha1.DaemonSetDeploymentStrategy, error) {
	if dda.Spec.Agent.DeploymentStrategy == nil {
		return nil, fmt.Errorf("could not get a defaulted DaemonSetDeploymentStrategy")
	}
	return dda.Spec.Agent.DeploymentStrategy, nil
}

func getAgentContainer(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, image string) (*corev1.Container, error) {
	agentSpec := dda.Spec.Agent
	envVars, err := getEnvVarsForAgent(logger, dda)
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
		Image:           image,
		ImagePullPolicy: *agentSpec.Image.PullPolicy,
		Command:         getDefaultIfEmpty(dda.Spec.Agent.Config.Command, []string{"agent", "run"}),
		Args:            getDefaultIfEmpty(dda.Spec.Agent.Config.Args, nil),
		Resources:       *agentSpec.Config.Resources,
		Ports: []corev1.ContainerPort{
			udpPort,
		},
		Env:            envVars,
		VolumeMounts:   getVolumeMountsForAgent(dda),
		LivenessProbe:  dda.Spec.Agent.Config.LivenessProbe,
		ReadinessProbe: dda.Spec.Agent.Config.ReadinessProbe,
	}

	return agentContainer, nil
}

func getAPMAgentContainers(dda *datadoghqv1alpha1.DatadogAgent, image string) ([]corev1.Container, error) {
	agentSpec := dda.Spec.Agent
	envVars, err := getEnvVarsForAPMAgent(dda)
	if err != nil {
		return nil, err
	}
	tcpPort := corev1.ContainerPort{
		ContainerPort: *dda.Spec.Agent.Apm.HostPort,
		Name:          "traceport",
		Protocol:      corev1.ProtocolTCP,
		HostPort:      *dda.Spec.Agent.Apm.HostPort,
	}

	apmContainer := corev1.Container{
		Name:            "trace-agent",
		Image:           image,
		ImagePullPolicy: *agentSpec.Image.PullPolicy,
		Command:         getDefaultIfEmpty(dda.Spec.Agent.Apm.Command, []string{"trace-agent", fmt.Sprintf("--config=%s", datadoghqv1alpha1.AgentCustomConfigVolumePath)}),
		Args:            getDefaultIfEmpty(dda.Spec.Agent.Apm.Args, nil),
		Ports: []corev1.ContainerPort{
			tcpPort,
		},
		Env:           envVars,
		LivenessProbe: dda.Spec.Agent.Apm.LivenessProbe,
		VolumeMounts:  getVolumeMountsForAPMAgent(dda),
	}
	if agentSpec.Apm.Resources != nil {
		apmContainer.Resources = *agentSpec.Apm.Resources
	}

	return []corev1.Container{apmContainer}, nil
}

func getProcessContainers(dda *datadoghqv1alpha1.DatadogAgent, image string) ([]corev1.Container, error) {
	agentSpec := dda.Spec.Agent
	envVars, err := getEnvVarsForProcessAgent(dda)
	if err != nil {
		return nil, err
	}

	process := corev1.Container{
		Name:            "process-agent",
		Image:           image,
		ImagePullPolicy: *agentSpec.Image.PullPolicy,
		Command: getDefaultIfEmpty(dda.Spec.Agent.Process.Command, []string{
			"process-agent", fmt.Sprintf("--config=%s", datadoghqv1alpha1.AgentCustomConfigVolumePath),
			fmt.Sprintf("--sysprobe-config=%s", datadoghqv1alpha1.SystemProbeConfigVolumePath),
		}),
		Args:         getDefaultIfEmpty(dda.Spec.Agent.Process.Args, nil),
		Env:          envVars,
		VolumeMounts: getVolumeMountsForProcessAgent(dda),
	}

	if agentSpec.Process.Resources != nil {
		process.Resources = *agentSpec.Process.Resources
	}

	return []corev1.Container{process}, nil
}

func getSystemProbeContainers(dda *datadoghqv1alpha1.DatadogAgent, image string) ([]corev1.Container, error) {
	agentSpec := dda.Spec.Agent
	systemProbeEnvVars, err := getEnvVarsForSystemProbe(dda)
	if err != nil {
		return nil, err
	}

	systemProbe := corev1.Container{
		Name:            "system-probe",
		Image:           image,
		ImagePullPolicy: *agentSpec.Image.PullPolicy,
		Command:         getDefaultIfEmpty(dda.Spec.Agent.SystemProbe.Command, []string{"system-probe", fmt.Sprintf("--config=%s", datadoghqv1alpha1.SystemProbeConfigVolumePath)}),
		Args:            getDefaultIfEmpty(dda.Spec.Agent.SystemProbe.Args, nil),
		SecurityContext: &corev1.SecurityContext{
			Capabilities: &corev1.Capabilities{
				Add: []corev1.Capability{
					"SYS_ADMIN",
					"SYS_RESOURCE",
					"SYS_PTRACE",
					"NET_ADMIN",
					"NET_BROADCAST",
					"NET_RAW",
					"IPC_LOCK",
				},
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

func getSecurityAgentContainer(dda *datadoghqv1alpha1.DatadogAgent, image string) (*corev1.Container, error) {
	agentSpec := dda.Spec.Agent
	envVars, err := getEnvVarsForSecurityAgent(dda)
	if err != nil {
		return nil, err
	}

	securityAgentContainer := &corev1.Container{
		Name:            "security-agent",
		Image:           image,
		ImagePullPolicy: *agentSpec.Image.PullPolicy,
		Command:         getDefaultIfEmpty(dda.Spec.Agent.Security.Command, []string{"security-agent", "start", fmt.Sprintf("-c=%s", datadoghqv1alpha1.AgentCustomConfigVolumePath)}),
		Args:            getDefaultIfEmpty(dda.Spec.Agent.Security.Args, nil),
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

func getInitContainers(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, image string) ([]corev1.Container, error) {
	spec := &dda.Spec
	volumeMounts := getVolumeMountsForAgent(dda)
	envVars, err := getEnvVarsForAgent(logger, dda)
	if err != nil {
		return nil, err
	}

	containers := getConfigInitContainers(spec, volumeMounts, envVars, image)

	if shouldInstallSeccompProfileFromConfigMap(dda) {
		systemProbeInit := corev1.Container{
			Name:            "seccomp-setup",
			Image:           image,
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

func getInitContainer(spec *datadoghqv1alpha1.DatadogAgentSpec, name string, commands []string, volumeMounts []corev1.VolumeMount, envVars []corev1.EnvVar, image string) corev1.Container {
	return corev1.Container{
		Name:            name,
		Image:           image,
		ImagePullPolicy: *spec.Agent.Image.PullPolicy,
		Resources:       *spec.Agent.Config.Resources,
		Command:         []string{"bash", "-c"},
		Args:            []string{strings.Join(commands, ";")},
		VolumeMounts:    volumeMounts,
		Env:             envVars,
	}
}

// getConfigInitContainers returns the init containers necessary to set up the
// agent's configuration volume.
func getConfigInitContainers(spec *datadoghqv1alpha1.DatadogAgentSpec, volumeMounts []corev1.VolumeMount, envVars []corev1.EnvVar, image string) []corev1.Container {
	configVolumeMounts := []corev1.VolumeMount{{
		Name:      datadoghqv1alpha1.ConfigVolumeName,
		MountPath: "/opt/datadog-agent",
	}}

	copyCommands := []string{"cp -vnr /etc/datadog-agent /opt"}

	if isRuntimeSecurityEnabled(spec) && spec.Agent.Security.Runtime.PoliciesDir != nil {
		configVolumeMounts = append(
			configVolumeMounts,
			corev1.VolumeMount{
				Name:      datadoghqv1alpha1.SecurityAgentRuntimeCustomPoliciesVolumeName,
				MountPath: "/etc/datadog-agent-runtime-policies",
			},
			corev1.VolumeMount{
				Name:      datadoghqv1alpha1.SecurityAgentRuntimePoliciesDirVolumeName,
				MountPath: "/opt/datadog-agent/runtime-security.d",
			},
		)
		copyCommands = append(copyCommands, "cp -v /etc/datadog-agent-runtime-policies/* /opt/datadog-agent/runtime-security.d/")
	}

	return []corev1.Container{
		getInitContainer(
			spec, "init-volume",
			copyCommands, configVolumeMounts, nil,
			image,
		),
		getInitContainer(
			spec, "init-config",
			[]string{"for script in $(find /etc/cont-init.d/ -type f -name '*.sh' | sort) ; do bash $script ; done"},
			volumeMounts, envVars,
			image,
		),
	}
}

func getEnvVarDogstatsdSocket(dda *datadoghqv1alpha1.DatadogAgent) corev1.EnvVar {
	return corev1.EnvVar{
		Name:  datadoghqv1alpha1.DDDogstatsdSocket,
		Value: getLocalFilepath(*dda.Spec.Agent.Config.Dogstatsd.UnixDomainSocket.HostFilepath, localDogstatsdSocketPath),
	}
}

// getEnvVarsForAPMAgent converts APM Agent Config into container env vars
func getEnvVarsForAPMAgent(dda *datadoghqv1alpha1.DatadogAgent) ([]corev1.EnvVar, error) {
	envVars := []corev1.EnvVar{
		{
			Name:  datadoghqv1alpha1.DDAPMEnabled,
			Value: strconv.FormatBool(isAPMEnabled(&dda.Spec)),
		},
		getEnvVarDogstatsdSocket(dda),
	}

	// APM Unix Domain Socket configuration
	if datadoghqv1alpha1.BoolValue(dda.Spec.Agent.Apm.UnixDomainSocket.Enabled) {
		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDPPMReceiverSocket,
			Value: getLocalFilepath(*dda.Spec.Agent.Apm.UnixDomainSocket.HostFilepath, localAPMSocketPath),
		})
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
		getEnvVarDogstatsdSocket(dda),
	}

	if isSystemProbeEnabled(&dda.Spec) {
		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDSystemProbeSocketPath,
			Value: filepath.Join(datadoghqv1alpha1.SystemProbeSocketVolumePath, "sysprobe.sock"),
		})

		envVars = addBoolEnVar(isNetworkMonitoringEnabled(&dda.Spec), datadoghqv1alpha1.DDSystemProbeNPMEnabled, envVars)
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

		// The process agent retrieves the cluster id from the Cluster Agent
		envVars = append(envVars, envForClusterAgentConnection(dda)...)
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

	envVars = append(envVars,
		corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDSystemProbeDebugPort,
			Value: strconv.FormatInt(int64(dda.Spec.Agent.SystemProbe.DebugPort), 10),
		},
		corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDSystemProbeSocketPath,
			Value: filepath.Join(datadoghqv1alpha1.SystemProbeSocketVolumePath, "sysprobe.sock"),
		},
	)

	// We do not set env vars to false if *bool is nil as it will override content from config file
	envVars = addBoolPointerEnVar(dda.Spec.Agent.SystemProbe.ConntrackEnabled, datadoghqv1alpha1.DDSystemProbeConntrackEnabled, envVars)
	envVars = addBoolPointerEnVar(dda.Spec.Agent.SystemProbe.BPFDebugEnabled, datadoghqv1alpha1.DDSystemProbeBPFDebugEnabled, envVars)
	envVars = addBoolPointerEnVar(dda.Spec.Agent.SystemProbe.EnableTCPQueueLength, datadoghqv1alpha1.DDSystemProbeTCPQueueLengthEnabled, envVars)
	envVars = addBoolPointerEnVar(dda.Spec.Agent.SystemProbe.EnableOOMKill, datadoghqv1alpha1.DDSystemProbeOOMKillEnabled, envVars)
	envVars = addBoolPointerEnVar(dda.Spec.Agent.SystemProbe.CollectDNSStats, datadoghqv1alpha1.DDSystemProbeCollectDNSStatsEnabled, envVars)
	envVars = addBoolEnVar(isNetworkMonitoringEnabled(&dda.Spec), datadoghqv1alpha1.DDSystemProbeNPMEnabled, envVars)
	envVars = addBoolEnVar(isRuntimeSecurityEnabled(&dda.Spec), datadoghqv1alpha1.DDRuntimeSecurityConfigEnabled, envVars)
	envVars = addBoolEnVar(isSyscallMonitorEnabled(&dda.Spec), datadoghqv1alpha1.DDRuntimeSecurityConfigSyscallMonitorEnabled, envVars)
	// For now don't expose the remote_tagger setting to user, since it is an implementation detail.
	envVars = addBoolEnVar(isRuntimeSecurityEnabled(&dda.Spec), datadoghqv1alpha1.DDRuntimeSecurityConfigRemoteTaggerEnabled, envVars)

	if isRuntimeSecurityEnabled(&dda.Spec) {
		envVars = append(envVars,
			corev1.EnvVar{
				Name:  datadoghqv1alpha1.DDRuntimeSecurityConfigSocket,
				Value: filepath.Join(datadoghqv1alpha1.SystemProbeSocketVolumePath, "runtime-security.sock"),
			},
			corev1.EnvVar{
				Name:  datadoghqv1alpha1.DDRuntimeSecurityConfigPoliciesDir,
				Value: datadoghqv1alpha1.SecurityAgentRuntimePoliciesDirVolumePath,
			},
			corev1.EnvVar{
				Name:  datadoghqv1alpha1.DDAuthTokenFilePath,
				Value: filepath.Join(datadoghqv1alpha1.AuthVolumePath, "token"),
			},
		)
	}

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
			Name:  datadoghqv1alpha1.KubernetesEnvvarName,
			Value: "yes",
		},
	}

	envVars = append(envVars, getKubeletEnvVars(dda)...)

	if dda.Spec.ClusterName != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDClusterName,
			Value: dda.Spec.ClusterName,
		})
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
		}
		if dda.Spec.Agent.Config.CriSocket.DockerSocketPath != nil {
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

func getEnvVarsForLogCollection(logSpec *datadoghqv1alpha1.LogCollectionConfig) []corev1.EnvVar {
	if logSpec == nil {
		return []corev1.EnvVar{}
	}

	envVars := []corev1.EnvVar{
		{
			Name:  datadoghqv1alpha1.DDLogsEnabled,
			Value: strconv.FormatBool(datadoghqv1alpha1.BoolValue(logSpec.Enabled)),
		},
		{
			Name:  datadoghqv1alpha1.DDLogsConfigContainerCollectAll,
			Value: strconv.FormatBool(datadoghqv1alpha1.BoolValue(logSpec.LogsConfigContainerCollectAll)),
		},
		{
			Name:  datadoghqv1alpha1.DDLogsContainerCollectUsingFiles,
			Value: strconv.FormatBool(datadoghqv1alpha1.BoolValue(logSpec.ContainerCollectUsingFiles)),
		},
	}
	if logSpec.OpenFilesLimit != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDLogsConfigOpenFilesLimit,
			Value: strconv.FormatInt(int64(*logSpec.OpenFilesLimit), 10),
		})
	}

	return envVars
}

// getEnvVarsForAgent converts Agent Config into container env vars
func getEnvVarsForAgent(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) ([]corev1.EnvVar, error) {
	spec := dda.Spec
	// Marshal tag fields
	var envVars []corev1.EnvVar
	config := dda.Spec.Agent.Config
	if config != nil {
		podLabelsAsTags, err := json.Marshal(spec.Agent.Config.PodLabelsAsTags)
		if err != nil {
			return nil, err
		}
		podAnnotationsAsTags, err := json.Marshal(spec.Agent.Config.PodAnnotationsAsTags)
		if err != nil {
			return nil, err
		}
		envVars = []corev1.EnvVar{
			{
				Name:  datadoghqv1alpha1.DDHealthPort,
				Value: strconv.Itoa(int(*spec.Agent.Config.HealthPort)),
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
		}
	}
	envVars = append(envVars, getEnvVarsForLogCollection(spec.Features.LogCollection)...)
	commonEnvVars, err := getEnvVarsCommon(dda, true)
	if err != nil {
		return nil, err
	}
	envVars = append(envVars, commonEnvVars...)

	if isDogstatsdConfigured(&spec) {
		envVars = append(envVars,
			corev1.EnvVar{
				Name:  datadoghqv1alpha1.DDDogstatsdOriginDetection,
				Value: strconv.FormatBool(*spec.Agent.Config.Dogstatsd.DogstatsdOriginDetection),
			},
		)
		// Always add DD_DOGSTATSD_SOCKET env var, to allow JMX-Fetch to use it inside pod's containers.
		envVars = append(envVars, getEnvVarDogstatsdSocket(dda))

		if dda.Spec.Agent.Config.Dogstatsd.MapperProfiles != nil {
			if dsdMapperProfilesEnv := dsdMapperProfilesEnvVar(logger, dda); dsdMapperProfilesEnv != nil {
				envVars = append(envVars, *dsdMapperProfilesEnv)
			}
		}
	}

	if isSystemProbeEnabled(&dda.Spec) {
		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDSystemProbeSocketPath,
			Value: filepath.Join(datadoghqv1alpha1.SystemProbeSocketVolumePath, "sysprobe.sock"),
		})
		envVars = addBoolPointerEnVar(dda.Spec.Agent.SystemProbe.EnableTCPQueueLength, datadoghqv1alpha1.DDSystemProbeTCPQueueLengthEnabled, envVars)
		envVars = addBoolPointerEnVar(dda.Spec.Agent.SystemProbe.EnableOOMKill, datadoghqv1alpha1.DDSystemProbeOOMKillEnabled, envVars)
	}

	if isClusterAgentEnabled(dda.Spec.ClusterAgent) {
		clusterEnv := envForClusterAgentConnection(dda)
		if spec.ClusterAgent.Config != nil && datadoghqv1alpha1.BoolValue(spec.ClusterAgent.Config.ClusterChecksEnabled) {
			if !datadoghqv1alpha1.BoolValue(dda.Spec.ClusterChecksRunner.Enabled) {
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

	envVars = append(envVars, prometheusScrapeEnvVars(logger, dda)...)

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
		{
			Name:  "HOST_ROOT",
			Value: datadoghqv1alpha1.HostRootVolumePath,
		},
		getEnvVarDogstatsdSocket(dda),
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

	if isClusterAgentEnabled(dda.Spec.ClusterAgent) {
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
	if spec.Agent.Config != nil {
		envVars = append(envVars, spec.Agent.Config.Env...)
	}

	return envVars, nil
}

// getVolumesForAgent defines volumes for the Agent
func getVolumesForAgent(dda *datadoghqv1alpha1.DatadogAgent) []corev1.Volume {
	volumes := []corev1.Volume{
		{
			Name: datadoghqv1alpha1.LogDatadogVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		getVolumeForAuth(),
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

	// Kubelet volumes
	volumes = append(volumes, getKubeletVolumes(dda)...)

	if dda.Spec.Agent.CustomConfig != nil {
		volume := getVolumeFromCustomConfigSpec(dda.Spec.Agent.CustomConfig, getAgentCustomConfigConfigMapName(dda), datadoghqv1alpha1.AgentCustomConfigVolumeName)
		volumes = append(volumes, volume)
	}

	// Dogstatsd volume
	dsdsocketVolume := corev1.Volume{
		Name: datadoghqv1alpha1.DogstatsdSocketVolumeName,
	}
	if isDogstatsdUDSEnabled(&dda.Spec) {
		volumeType := corev1.HostPathDirectoryOrCreate
		hostPath := getDirFromFilepath(*dda.Spec.Agent.Config.Dogstatsd.UnixDomainSocket.HostFilepath)

		dsdsocketVolume.VolumeSource = corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: hostPath,
				Type: &volumeType,
			},
		}
	} else {
		// By default use an emptyDir to store the socket
		dsdsocketVolume.VolumeSource = corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		}
	}
	volumes = append(volumes, dsdsocketVolume)

	// APM volume
	if isAPMUDSEnabled(&dda.Spec) {
		volumeType := corev1.HostPathDirectoryOrCreate
		hostPath := getDirFromFilepath(*dda.Spec.Agent.Apm.UnixDomainSocket.HostFilepath)

		dsdsocketVolume := corev1.Volume{
			Name: datadoghqv1alpha1.APMSocketVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: hostPath,
					Type: &volumeType,
				},
			},
		}
		volumes = append(volumes, dsdsocketVolume)
	}

	runtimeVolume := corev1.Volume{
		Name: datadoghqv1alpha1.CriSocketVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: defaultRuntimeDir,
			},
		},
	}

	if dda.Spec.Agent.Config != nil && dda.Spec.Agent.Config.CriSocket != nil {
		if dda.Spec.Agent.Config.CriSocket.CriSocketPath != nil {
			runtimeVolume.VolumeSource.HostPath.Path = filepath.Dir(*dda.Spec.Agent.Config.CriSocket.CriSocketPath)
		} else if dda.Spec.Agent.Config.CriSocket.DockerSocketPath != nil {
			runtimeVolume.VolumeSource.HostPath.Path = filepath.Dir(*dda.Spec.Agent.Config.CriSocket.DockerSocketPath)
		}
	}

	volumes = append(volumes, runtimeVolume)

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
		fileOrCreate := corev1.HostPathFileOrCreate
		systemProbeVolumes := []corev1.Volume{
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
			{
				Name: datadoghqv1alpha1.SystemProbeOSReleaseDirVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: datadoghqv1alpha1.SystemProbeOSReleaseDirVolumePath,
						Type: &fileOrCreate,
					},
				},
			},
		}

		if shouldMountSystemProbeConfigConfigMap(dda) {
			systemProbeVolumes = append(systemProbeVolumes, corev1.Volume{
				Name: datadoghqv1alpha1.SystemProbeConfigVolumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: getSystemProbeConfigConfigMapName(dda),
						},
					},
				},
			})
		}

		if shouldInstallSeccompProfileFromConfigMap(dda) {
			systemProbeVolumes = append(systemProbeVolumes, corev1.Volume{
				Name: datadoghqv1alpha1.SystemProbeAgentSecurityVolumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: getSecCompConfigMapName(dda),
						},
					},
				},
			})
			systemProbeVolumes = append(systemProbeVolumes, corev1.Volume{
				Name: datadoghqv1alpha1.SystemProbeSecCompRootVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: getSecCompRootPath(dda.Spec.Agent.SystemProbe),
					},
				},
			})
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

	logConfig := dda.Spec.Features.LogCollection
	if logConfig != nil && datadoghqv1alpha1.BoolValue(logConfig.Enabled) {
		if logConfig.TempStoragePath != nil {
			volumes = append(volumes, corev1.Volume{
				Name: datadoghqv1alpha1.PointerVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: *logConfig.TempStoragePath,
					},
				},
			})
		}
		if logConfig.PodLogsPath != nil {
			volumes = append(volumes, corev1.Volume{
				Name: datadoghqv1alpha1.LogPodVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: *logConfig.PodLogsPath,
					},
				},
			})
		}
		if logConfig.ContainerLogsPath != nil {
			volumes = append(volumes, corev1.Volume{
				Name: datadoghqv1alpha1.LogContainerVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: *logConfig.ContainerLogsPath,
					},
				},
			})
		}
		if logConfig.ContainerSymlinksPath != nil {
			volumes = append(volumes, corev1.Volume{
				Name: datadoghqv1alpha1.SymlinkContainerVolumeName,
				VolumeSource: corev1.VolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: *logConfig.ContainerSymlinksPath,
					},
				},
			})
		}
	}

	if isSecurityAgentEnabled(&dda.Spec) {
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
	}

	if isComplianceEnabled(&dda.Spec) {
		if dda.Spec.Agent.Security.Compliance.ConfigDir != nil {
			volumes = append(volumes, getVolumeFromConfigDirSpec(datadoghqv1alpha1.SecurityAgentComplianceConfigDirVolumeName, dda.Spec.Agent.Security.Compliance.ConfigDir))
		}
	}

	if isRuntimeSecurityEnabled(&dda.Spec) && dda.Spec.Agent.Security.Runtime.PoliciesDir != nil {
		volumes = append(volumes,
			getVolumeFromConfigDirSpec(datadoghqv1alpha1.SecurityAgentRuntimeCustomPoliciesVolumeName, dda.Spec.Agent.Security.Runtime.PoliciesDir),
			corev1.Volume{
				Name: datadoghqv1alpha1.SecurityAgentRuntimePoliciesDirVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			})
	}

	volumes = append(volumes, dda.Spec.Agent.Config.Volumes...)
	return volumes
}

func getDirFromFilepath(filePath string) string {
	return filepath.Dir(filePath)
}

func getLocalFilepath(filePath, localPath string) string {
	base := filepath.Base(filePath)
	return path.Join(localPath, base)
}

func getVolumeForConfd(dda *datadoghqv1alpha1.DatadogAgent) corev1.Volume {
	return getVolumeFromConfigDirSpec(datadoghqv1alpha1.ConfdVolumeName, dda.Spec.Agent.Config.Confd)
}

func getVolumeFromConfigDirSpec(volumeName string, conf *datadoghqv1alpha1.ConfigDirSpec) corev1.Volume {
	source := corev1.VolumeSource{
		EmptyDir: &corev1.EmptyDirVolumeSource{},
	}
	if conf != nil {
		source = corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: conf.ConfigMapName,
				},
			},
		}

		if len(conf.Items) > 0 {
			for _, val := range conf.Items {
				source.ConfigMap.Items = append(source.ConfigMap.Items, corev1.KeyToPath{
					Key:  val.Key,
					Path: val.Path,
				})
			}
		}
	}

	return corev1.Volume{
		Name:         volumeName,
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

func getVolumeForAuth() corev1.Volume {
	return corev1.Volume{
		Name: datadoghqv1alpha1.AuthVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
}

func getSecCompRootPath(spec *datadoghqv1alpha1.SystemProbeSpec) string {
	return spec.SecCompRootPath
}

func getAppArmorProfileName(spec *datadoghqv1alpha1.SystemProbeSpec) string {
	return spec.AppArmorProfileName
}

func getSeccompProfileName(spec *datadoghqv1alpha1.SystemProbeSpec) string {
	return spec.SecCompProfileName
}

func getVolumeFromCustomConfigSpec(cfcm *datadoghqv1alpha1.CustomConfigSpec, defaultConfigMapName, volumeName string) corev1.Volume {
	confdVolumeSource := *buildVolumeSourceFromCustomConfigSpec(cfcm, defaultConfigMapName)

	return corev1.Volume{
		Name:         volumeName,
		VolumeSource: confdVolumeSource,
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
func getVolumeMountsForAgent(dda *datadoghqv1alpha1.DatadogAgent) []corev1.VolumeMount {
	// Default mounted volumes
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      datadoghqv1alpha1.LogDatadogVolumeName,
			MountPath: datadoghqv1alpha1.LogDatadogVolumePath,
		},
		getVolumeMountForAuth(false),
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

	// Kubelet volumeMounts
	volumeMounts = append(volumeMounts, getKubeletVolumeMounts(dda)...)

	// Add configuration volumeMounts default and extra config (datadog.yaml) volume
	volumeMounts = append(volumeMounts, getVolumeMountForConfig(dda.Spec.Agent.CustomConfig)...)

	// Cri socket volume
	volumeMounts = append(volumeMounts, getVolumeMountForRuntimeSockets(dda.Spec.Agent.Config.CriSocket))

	// Dogstatsd volume
	volumeMounts = append(volumeMounts, getVolumeMountDogstatsdSocket(false))

	// Log volumes
	if datadoghqv1alpha1.BoolValue(dda.Spec.Features.LogCollection.Enabled) {
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
		}...)
		if dda.Spec.Features.LogCollection.ContainerLogsPath != nil {
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      datadoghqv1alpha1.LogContainerVolumeName,
				MountPath: *dda.Spec.Features.LogCollection.ContainerLogsPath,
				ReadOnly:  datadoghqv1alpha1.LogContainerVolumeReadOnly,
			})
		}
		if dda.Spec.Features.LogCollection.ContainerSymlinksPath != nil {
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      datadoghqv1alpha1.SymlinkContainerVolumeName,
				MountPath: *dda.Spec.Features.LogCollection.ContainerSymlinksPath,
				ReadOnly:  datadoghqv1alpha1.SymlinkContainerVolumeReadOnly,
			})
		}
	}

	// SystemProbe volumes
	if isSystemProbeEnabled(&dda.Spec) {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      datadoghqv1alpha1.SystemProbeSocketVolumeName,
			MountPath: datadoghqv1alpha1.SystemProbeSocketVolumePath,
			ReadOnly:  true,
		})
	}

	if shouldMountSystemProbeConfigConfigMap(dda) {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      datadoghqv1alpha1.SystemProbeConfigVolumeName,
			MountPath: datadoghqv1alpha1.SystemProbeConfigVolumePath,
			SubPath:   getSystemProbeConfigFileName(dda),
		})
	}

	return append(volumeMounts, dda.Spec.Agent.Config.VolumeMounts...)
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

func getVolumeMountForAuth(readOnly bool) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      datadoghqv1alpha1.AuthVolumeName,
		MountPath: datadoghqv1alpha1.AuthVolumePath,
		ReadOnly:  readOnly,
	}
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

func getVolumeMountDogstatsdSocket(readOnly bool) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      datadoghqv1alpha1.DogstatsdSocketVolumeName,
		MountPath: datadoghqv1alpha1.DogstatsdSocketVolumePath,
		ReadOnly:  readOnly,
	}
}

func getVolumeMountForRuntimeSockets(criSocket *datadoghqv1alpha1.CRISocketConfig) corev1.VolumeMount {
	var socketPath string
	if criSocket != nil {
		if criSocket.CriSocketPath != nil {
			socketPath = *criSocket.CriSocketPath
		} else if criSocket.DockerSocketPath != nil {
			socketPath = *criSocket.DockerSocketPath
		}
	}

	if socketPath == "" {
		socketPath = defaultRuntimeDir
	} else {
		socketPath = filepath.Dir(socketPath)
	}

	return corev1.VolumeMount{
		Name:      datadoghqv1alpha1.CriSocketVolumeName,
		MountPath: filepath.Join(datadoghqv1alpha1.HostCriSocketPathPrefix, socketPath),
		ReadOnly:  true,
	}
}

// getVolumeMountsForAgent defines mounted volumes for the Process Agent
func getVolumeMountsForProcessAgent(dda *datadoghqv1alpha1.DatadogAgent) []corev1.VolumeMount {
	// Default mounted volumes
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      datadoghqv1alpha1.LogDatadogVolumeName,
			MountPath: datadoghqv1alpha1.LogDatadogVolumePath,
		},
		// Add auth token volume mount
		getVolumeMountForAuth(true),
		getVolumeMountDogstatsdSocket(true),
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

	// Kubelet volumeMounts
	volumeMounts = append(volumeMounts, getKubeletVolumeMounts(dda)...)

	// Add configuration mount
	volumeMounts = append(volumeMounts, getVolumeMountForConfig(dda.Spec.Agent.CustomConfig)...)

	// Cri socket volume
	volumeMounts = append(volumeMounts, getVolumeMountForRuntimeSockets(dda.Spec.Agent.Config.CriSocket))

	if isSystemProbeEnabled(&dda.Spec) {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      datadoghqv1alpha1.SystemProbeSocketVolumeName,
			MountPath: datadoghqv1alpha1.SystemProbeSocketVolumePath,
			ReadOnly:  true,
		})
	}

	if shouldMountSystemProbeConfigConfigMap(dda) {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      datadoghqv1alpha1.SystemProbeConfigVolumeName,
			MountPath: datadoghqv1alpha1.SystemProbeConfigVolumePath,
			SubPath:   getSystemProbeConfigFileName(dda),
		})
	}

	// Add extra volume mounts
	volumeMounts = append(volumeMounts, dda.Spec.Agent.Process.VolumeMounts...)

	return volumeMounts
}

// getVolumeMountsForAgent defines mounted volumes for the Process Agent
func getVolumeMountsForAPMAgent(dda *datadoghqv1alpha1.DatadogAgent) []corev1.VolumeMount {
	// Default mounted volumes
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      datadoghqv1alpha1.LogDatadogVolumeName,
			MountPath: datadoghqv1alpha1.LogDatadogVolumePath,
		},
		// Add auth token volume mount
		getVolumeMountForAuth(true),
	}

	// Dogstatsd UDS (always mounted)
	volumeMounts = append(volumeMounts, getVolumeMountDogstatsdSocket(true))

	// APM UDS
	if datadoghqv1alpha1.BoolValue(dda.Spec.Agent.Apm.UnixDomainSocket.Enabled) {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      datadoghqv1alpha1.APMSocketVolumeName,
			MountPath: datadoghqv1alpha1.APMSocketVolumePath,
		})
	}

	// Kubelet volumeMounts
	volumeMounts = append(volumeMounts, getKubeletVolumeMounts(dda)...)

	// Add configuration volumesMount default and custom config (datadog.yaml) volume
	volumeMounts = append(volumeMounts, getVolumeMountForConfig(dda.Spec.Agent.CustomConfig)...)

	// Add extra volume mounts
	volumeMounts = append(volumeMounts, dda.Spec.Agent.Apm.VolumeMounts...)

	return volumeMounts
}

// getVolumeMountsForSystemProbe defines mounted volumes for the SystemProbe
func getVolumeMountsForSystemProbe(dda *datadoghqv1alpha1.DatadogAgent) []corev1.VolumeMount {
	// Default mounted volumes
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      datadoghqv1alpha1.LogDatadogVolumeName,
			MountPath: datadoghqv1alpha1.LogDatadogVolumePath,
		},
		getVolumeMountForAuth(true),
		{
			Name:      datadoghqv1alpha1.SystemProbeDebugfsVolumeName,
			MountPath: datadoghqv1alpha1.SystemProbeDebugfsVolumePath,
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
		{
			Name:      datadoghqv1alpha1.SystemProbeOSReleaseDirVolumeName,
			MountPath: datadoghqv1alpha1.SystemProbeOSReleaseDirMountPath,
			ReadOnly:  true,
		},
	}

	if shouldMountSystemProbeConfigConfigMap(dda) {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      datadoghqv1alpha1.SystemProbeConfigVolumeName,
			MountPath: datadoghqv1alpha1.SystemProbeConfigVolumePath,
			SubPath:   getSystemProbeConfigFileName(dda),
		})
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

	if isRuntimeSecurityEnabled(&dda.Spec) {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      datadoghqv1alpha1.SecurityAgentRuntimePoliciesDirVolumeName,
			MountPath: datadoghqv1alpha1.SecurityAgentRuntimePoliciesDirVolumePath,
			ReadOnly:  true,
		})
	}

	// Add extra volume mounts
	volumeMounts = append(volumeMounts, dda.Spec.Agent.SystemProbe.VolumeMounts...)

	return volumeMounts
}

// getVolumeMountsForSecurityAgent defines mounted volumes for the Security Agent
func getVolumeMountsForSecurityAgent(dda *datadoghqv1alpha1.DatadogAgent) []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      datadoghqv1alpha1.LogDatadogVolumeName,
			MountPath: datadoghqv1alpha1.LogDatadogVolumePath,
		},
		getVolumeMountForAuth(true),
		getVolumeMountDogstatsdSocket(true),
		{
			Name:      datadoghqv1alpha1.ConfigVolumeName,
			MountPath: datadoghqv1alpha1.ConfigVolumePath,
		},
		{
			Name:      datadoghqv1alpha1.HostRootVolumeName,
			MountPath: datadoghqv1alpha1.HostRootVolumePath,
			ReadOnly:  true,
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
		}...)
	}

	if runtimeEnabled {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      datadoghqv1alpha1.SecurityAgentRuntimePoliciesDirVolumeName,
			MountPath: datadoghqv1alpha1.SecurityAgentRuntimePoliciesDirVolumePath,
			ReadOnly:  true,
		})
	}

	spec := dda.Spec

	if spec.Agent.CustomConfig != nil {
		volumeMount := getVolumeMountFromCustomConfigSpec(spec.Agent.CustomConfig, datadoghqv1alpha1.AgentCustomConfigVolumeName, datadoghqv1alpha1.AgentCustomConfigVolumePath, datadoghqv1alpha1.AgentCustomConfigVolumeSubPath)
		volumeMounts = append(volumeMounts, volumeMount)
	}

	// Add extra volume mounts
	volumeMounts = append(volumeMounts, spec.Agent.Security.VolumeMounts...)

	// Cri socket volume
	runtimeVolume := getVolumeMountForRuntimeSockets(dda.Spec.Agent.Config.CriSocket)
	volumeMounts = append(volumeMounts, runtimeVolume)
	if complianceEnabled {
		// Additional mount for runtime socket under hostroot
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      datadoghqv1alpha1.CriSocketVolumeName,
			MountPath: strings.Replace(runtimeVolume.MountPath, datadoghqv1alpha1.HostCriSocketPathPrefix, datadoghqv1alpha1.HostRootVolumePath, 1),
			ReadOnly:  true,
		})
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

	return volumeMounts
}

func getAgentVersion(dda *datadoghqv1alpha1.DatadogAgent) string {
	// TODO implement this method
	return ""
}

func getAgentServiceAccount(dda *datadoghqv1alpha1.DatadogAgent) string {
	saDefault := fmt.Sprintf("%s-agent", dda.Name)
	if !datadoghqv1alpha1.BoolValue(dda.Spec.Agent.Enabled) {
		return saDefault
	}
	if dda.Spec.Agent.Rbac != nil && dda.Spec.Agent.Rbac.ServiceAccountName != nil {
		return *dda.Spec.Agent.Rbac.ServiceAccountName
	}
	return saDefault
}

// getAPIKeyFromSecret returns the Agent API key as an env var source
func getAPIKeyFromSecret(dda *datadoghqv1alpha1.DatadogAgent) *corev1.EnvVarSource {
	_, name, key := utils.GetAPIKeySecret(&dda.Spec.Credentials.DatadogCredentials, utils.GetDefaultCredentialsSecretName(dda))
	return buildEnvVarFromSecret(name, key)
}

// getClusterAgentAuthToken returns the Cluster Agent auth token as an env var source
func getClusterAgentAuthToken(dda *datadoghqv1alpha1.DatadogAgent) *corev1.EnvVarSource {
	return buildEnvVarFromSecret(getAuthTokenSecretName(dda), datadoghqv1alpha1.DefaultTokenKey)
}

// getAppKeyFromSecret returns the Agent API key as an env var source
func getAppKeyFromSecret(dda *datadoghqv1alpha1.DatadogAgent) *corev1.EnvVarSource {
	_, name, key := utils.GetAppKeySecret(&dda.Spec.Credentials.DatadogCredentials, utils.GetDefaultCredentialsSecretName(dda))
	return buildEnvVarFromSecret(name, key)
}

func buildEnvVarFromSecret(name, key string) *corev1.EnvVarSource {
	return &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: name,
			},
			Key: key,
		},
	}
}

func getClusterAgentServiceName(dda *datadoghqv1alpha1.DatadogAgent) string {
	return fmt.Sprintf("%s-%s", dda.Name, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix)
}

func getClusterAgentServiceAccount(dda *datadoghqv1alpha1.DatadogAgent) string {
	saDefault := fmt.Sprintf("%s-%s", dda.Name, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix)
	if !isClusterAgentEnabled(dda.Spec.ClusterAgent) {
		return saDefault
	}
	if dda.Spec.ClusterAgent.Rbac != nil && dda.Spec.ClusterAgent.Rbac.ServiceAccountName != nil {
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
	return "v1beta1.external.metrics.k8s.io"
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

	if !datadoghqv1alpha1.BoolValue(dda.Spec.ClusterChecksRunner.Enabled) {
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

	// Copy Datadog labels from DDA Labels
	for k, v := range dda.Labels {
		if strings.HasPrefix(k, datadogTagPrefix) {
			labels[k] = v
		}
	}

	return labels
}

func getDefaultAnnotations(*datadoghqv1alpha1.DatadogAgent) map[string]string {
	// Currently we don't have any annotation to set by default
	return map[string]string{}
}

func mergeAnnotationsLabels(logger logr.Logger, previousVal map[string]string, newVal map[string]string, filter string) map[string]string {
	var globFilter glob.Glob
	var err error
	if filter != "" {
		globFilter, err = glob.Compile(filter)
		if err != nil {
			logger.Error(err, "Unable to parse glob filter for metadata/annotations - discarding everything", "filter", filter)
		}
	}

	mergedMap := make(map[string]string, len(newVal))
	for k, v := range newVal {
		mergedMap[k] = v
	}

	// Copy from previous if not in new match and matches globfilter
	for k, v := range previousVal {
		if _, found := newVal[k]; !found {
			if (globFilter != nil && globFilter.Match(k)) || strings.Contains(k, "datadoghq.com") {
				mergedMap[k] = v
			}
		}
	}

	return mergedMap
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
	if dda.Spec.Features.KubeStateMetricsCore == nil {
		return false
	}
	return datadoghqv1alpha1.BoolValue(dda.Spec.Features.KubeStateMetricsCore.Enabled)
}

func isKSMCoreClusterCheck(dda *datadoghqv1alpha1.DatadogAgent) bool {
	return isKSMCoreEnabled(dda) && datadoghqv1alpha1.BoolValue(dda.Spec.Features.KubeStateMetricsCore.ClusterCheck)
}

// GetKubeStateMetricsConfName get the name of the Configmap for the KSM Core check.
func GetKubeStateMetricsConfName(dcaConf *datadoghqv1alpha1.DatadogAgent) string {
	// `configData` and `configMap` can't be set together.
	// Return the default if the conf is not overridden or if it is just overridden with the ConfigData.
	if dcaConf.Spec.Features.KubeStateMetricsCore.Conf != nil && dcaConf.Spec.Features.KubeStateMetricsCore.Conf.ConfigMap != nil {
		return dcaConf.Spec.Features.KubeStateMetricsCore.Conf.ConfigMap.Name
	}
	return fmt.Sprintf("%s-%s", dcaConf.Name, datadoghqv1alpha1.DefaultKubeStateMetricsCoreConf)
}

func prometheusScrapeEnvVars(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) []corev1.EnvVar {
	envVars := []corev1.EnvVar{}

	if datadoghqv1alpha1.BoolValue(dda.Spec.Features.PrometheusScrape.Enabled) {
		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDPrometheusScrapeEnabled,
			Value: datadoghqv1alpha1.BoolToString(dda.Spec.Features.PrometheusScrape.Enabled),
		})

		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDPrometheusScrapeServiceEndpoints,
			Value: datadoghqv1alpha1.BoolToString(dda.Spec.Features.PrometheusScrape.ServiceEndpoints),
		})

		if dda.Spec.Features.PrometheusScrape.AdditionalConfigs != nil {
			jsonValue, err := yaml.YAMLToJSON([]byte(*dda.Spec.Features.PrometheusScrape.AdditionalConfigs))
			if err != nil {
				logger.Error(err, "Invalid additional prometheus config, ignoring it")
			} else {
				envVars = append(envVars, corev1.EnvVar{
					Name:  datadoghqv1alpha1.DDPrometheusScrapeChecks,
					Value: string(jsonValue),
				})
			}
		}
	}

	return envVars
}

func dsdMapperProfilesEnvVar(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) *corev1.EnvVar {
	if dda.Spec.Agent.Config.Dogstatsd.MapperProfiles.ConfigData != nil {
		if dda.Spec.Agent.Config.Dogstatsd.MapperProfiles.ConfigMap != nil {
			logger.Info("configData and configMap cannot be set simultaneously for dogstastd mapper profiles, ignoring the config map")
		}
		jsonValue, err := yaml.YAMLToJSON([]byte(*dda.Spec.Agent.Config.Dogstatsd.MapperProfiles.ConfigData))
		if err != nil {
			logger.Error(err, "Invalid dogstatsd mapper profiles config, ignoring it")
			return nil
		}
		return &corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDDogstatsdMapperProfiles,
			Value: string(jsonValue),
		}
	}

	if dda.Spec.Agent.Config.Dogstatsd.MapperProfiles.ConfigMap != nil {
		cmSelector := corev1.ConfigMapKeySelector{}
		cmSelector.Name = dda.Spec.Agent.Config.Dogstatsd.MapperProfiles.ConfigMap.Name
		cmSelector.Key = dda.Spec.Agent.Config.Dogstatsd.MapperProfiles.ConfigMap.FileKey
		return &corev1.EnvVar{
			Name:      datadoghqv1alpha1.DDDogstatsdMapperProfiles,
			ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &cmSelector},
		}
	}

	return nil
}

func isClusterAgentEnabled(spec datadoghqv1alpha1.DatadogAgentSpecClusterAgentSpec) bool {
	return datadoghqv1alpha1.BoolValue(spec.Enabled)
}

func isMetricsProviderEnabled(spec datadoghqv1alpha1.DatadogAgentSpecClusterAgentSpec) bool {
	if !isClusterAgentEnabled(spec) {
		return false
	}
	if spec.Config == nil || spec.Config.ExternalMetrics == nil {
		return false
	}
	return datadoghqv1alpha1.BoolValue(spec.Config.ExternalMetrics.Enabled)
}

func hasMetricsProviderCustomCredentials(spec datadoghqv1alpha1.DatadogAgentSpecClusterAgentSpec) bool {
	return isMetricsProviderEnabled(spec) && spec.Config.ExternalMetrics.Credentials != nil
}

func isAdmissionControllerEnabled(spec datadoghqv1alpha1.DatadogAgentSpecClusterAgentSpec) bool {
	if spec.Config == nil || spec.Config.AdmissionController == nil {
		return false
	}
	return datadoghqv1alpha1.BoolValue(spec.Config.AdmissionController.Enabled)
}

func isCreateRBACEnabled(config *datadoghqv1alpha1.RbacConfig) bool {
	if config == nil {
		return false
	}
	return datadoghqv1alpha1.BoolValue(config.Create)
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

func getLogLevel(dda *datadoghqv1alpha1.DatadogAgent) string {
	return *dda.Spec.Agent.Config.LogLevel
}

// CheckOwnerReference return true if owner is the owner of the object
func CheckOwnerReference(owner, object metav1.Object) bool {
	return metav1.IsControlledBy(object, owner)
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

// envForClusterAgentConnection returns the environment variables required to connect to the Cluster Agent
func envForClusterAgentConnection(dda *datadoghqv1alpha1.DatadogAgent) []corev1.EnvVar {
	if isClusterAgentEnabled(dda.Spec.ClusterAgent) {
		return []corev1.EnvVar{
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
	}
	return []corev1.EnvVar{}
}

func getDefaultIfEmpty(val, def []string) []string {
	if len(val) > 0 {
		return val
	}

	return def
}

func addBoolEnVar(b bool, varName string, varList []corev1.EnvVar) []corev1.EnvVar {
	return addBoolPointerEnVar(&b, varName, varList)
}

func addBoolPointerEnVar(b *bool, varName string, varList []corev1.EnvVar) []corev1.EnvVar {
	if b != nil {
		varList = append(varList, corev1.EnvVar{
			Name:  varName,
			Value: datadoghqv1alpha1.BoolToString(b),
		})
	}

	return varList
}

// imageHasTag identifies whether an image string contains a tag suffix
// Ref: https://github.com/distribution/distribution/blob/v2.7.1/reference/reference.go
var imageHasTag = regexp.MustCompile(`.+:[\w][\w.-]{0,127}$`)

// getImage builds the image string based on ImageConfig and the registry configuration.
func getImage(imageSpec *datadoghqv1alpha1.ImageConfig, registry *string, checkJMX bool) string {
	if imageHasTag.MatchString(imageSpec.Name) {
		// The image name corresponds to a full image string
		return imageSpec.Name
	}

	image := "/" + imageSpec.Name + ":" + imageSpec.Tag

	if checkJMX && imageSpec.JmxEnabled && !strings.HasSuffix(imageSpec.Tag, datadoghqv1alpha1.JMXTagSuffix) {
		image += datadoghqv1alpha1.JMXTagSuffix
	}

	if registry != nil {
		return *registry + image
	}

	return datadoghqv1alpha1.DefaultImageRegistry + image
}

// getReplicas returns the desired replicas of a
// deployment based on the current and new replica values.
func getReplicas(currentReplicas, newReplicas *int32) *int32 {
	if newReplicas == nil {
		if currentReplicas != nil {
			// Do not overwrite the current value
			// It's most likely managed by an autoscaler
			return datadoghqv1alpha1.NewInt32Pointer(*currentReplicas)
		}

		// Both new and current are nil
		return nil
	}

	return datadoghqv1alpha1.NewInt32Pointer(*newReplicas)
}
