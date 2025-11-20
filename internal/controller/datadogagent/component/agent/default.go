// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"fmt"
	"path/filepath"
	"strconv"

	edsv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component"
	componentdca "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusteragent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/secrets"
)

// NewDefaultAgentDaemonset return a new default agent DaemonSet
// TODO: remove instanceName once v2 reconcile is removed
func NewDefaultAgentDaemonset(dda metav1.Object, edsOptions *ExtendedDaemonsetOptions, agentComponent feature.RequiredComponent, instanceName string) *appsv1.DaemonSet {
	daemonset := NewDaemonset(dda, edsOptions, constants.DefaultAgentResourceSuffix, component.GetAgentName(dda), common.GetAgentVersion(dda), nil, instanceName)
	podTemplate := NewDefaultAgentPodTemplateSpec(dda, agentComponent, daemonset.GetLabels())
	daemonset.Spec.Template = *podTemplate
	return daemonset
}

// NewDefaultAgentExtendedDaemonset return a new default agent DaemonSet
func NewDefaultAgentExtendedDaemonset(dda metav1.Object, edsOptions *ExtendedDaemonsetOptions, agentComponent feature.RequiredComponent) *edsv1alpha1.ExtendedDaemonSet {
	edsDaemonset := NewExtendedDaemonset(dda, edsOptions, constants.DefaultAgentResourceSuffix, component.GetAgentName(dda), common.GetAgentVersion(dda), nil)
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

// DefaultSeccompConfigDataForSystemProbe returns configmap data for the default seccomp profile
func DefaultSeccompConfigDataForSystemProbe() map[string]string {
	return map[string]string{
		"system-probe-seccomp.json": `{
			"defaultAction": "SCMP_ACT_ERRNO",
			"syscalls": [
				{
				"names": [
					"accept4",
					"access",
					"arch_prctl",
					"bind",
					"bpf",
					"brk",
					"capget",
					"capset",
					"chdir",
					"chmod",
					"chown",
					"clock_gettime",
					"clone",
					"clone3",
					"close",
					"close_range",
					"connect",
					"copy_file_range",
					"creat",
					"dup",
					"dup2",
					"dup3",
					"epoll_create",
					"epoll_create1",
					"epoll_ctl",
					"epoll_ctl_old",
					"epoll_pwait",
					"epoll_wait",
					"epoll_wait_old",
					"eventfd",
					"eventfd2",
					"execve",
					"execveat",
					"exit",
					"exit_group",
					"faccessat",
					"faccessat2",
					"fchmod",
					"fchmodat",
					"fchown",
					"fchown32",
					"fchownat",
					"fcntl",
					"fcntl64",
					"fdatasync",
					"flock",
					"fstat",
					"fstat64",
					"fstatfs",
					"fsync",
					"ftruncate",
					"futex",
					"futimens",
					"getcwd",
					"getdents",
					"getdents64",
					"getegid",
					"geteuid",
					"getgid",
					"getgroups",
					"getpeername",
					"getpgid",
					"getpgrp",
					"getpid",
					"getppid",
					"getpriority",
					"getrandom",
					"getresgid",
					"getresgid32",
					"getresuid",
					"getresuid32",
					"getrlimit",
					"getrusage",
					"getsid",
					"getsockname",
					"getsockopt",
					"gettid",
					"gettimeofday",
					"getuid",
					"getxattr",
					"inotify_add_watch",
					"inotify_init",
					"inotify_init1",
					"inotify_rm_watch",
					"ioctl",
					"ipc",
					"listen",
					"lseek",
					"lstat",
					"lstat64",
					"madvise",
					"memfd_create",
					"mkdir",
					"mkdirat",
					"mknod",
					"mknodat",
					"mmap",
					"mmap2",
					"mprotect",
					"mremap",
					"munmap",
					"nanosleep",
					"newfstatat",
					"open",
					"openat",
					"openat2",
					"pause",
					"perf_event_open",
					"pidfd_open",
					"pidfd_send_signal",
					"pipe",
					"pipe2",
					"poll",
					"ppoll",
					"prctl",
					"pread64",
					"prlimit64",
					"pselect6",
					"pwrite64",
					"read",
					"readlink",
					"readlinkat",
					"recvfrom",
					"recvmmsg",
					"recvmsg",
					"rename",
					"renameat",
					"renameat2",
					"restart_syscall",
					"rmdir",
					"rseq",
					"rt_sigaction",
					"rt_sigpending",
					"rt_sigprocmask",
					"rt_sigqueueinfo",
					"rt_sigreturn",
					"rt_sigsuspend",
					"rt_sigtimedwait",
					"rt_tgsigqueueinfo",
					"sched_getaffinity",
					"sched_yield",
					"seccomp",
					"select",
					"semtimedop",
					"send",
					"sendmmsg",
					"sendmsg",
					"sendto",
					"set_robust_list",
					"set_tid_address",
					"setgid",
					"setgid32",
					"setgroups",
					"setgroups32",
					"setitimer",
					"setns",
					"setpgid",
					"setrlimit",
					"setsid",
					"setsidaccept4",
					"setsockopt",
					"setuid",
					"setuid32",
					"sigaltstack",
					"socket",
					"socketcall",
					"socketpair",
					"stat",
					"stat64",
					"statfs",
					"statx",
					"symlinkat",
					"sysinfo",
					"tgkill",
					"tkill",
					"umask",
					"uname",
					"unlink",
					"unlinkat",
					"utime",
					"utimensat",
					"utimes",
					"wait4",
					"waitid",
					"waitpid",
					"write"
				],
				"action": "SCMP_ACT_ALLOW",
				"args": null
				},
				{
				"names": [
					"setns"
				],
				"action": "SCMP_ACT_ALLOW",
				"args": [
					{
					"index": 1,
					"value": 1073741824,
					"valueTwo": 0,
					"op": "SCMP_CMP_EQ"
					}
				],
				"comment": "",
				"includes": {},
				"excludes": {}
				},
				{
				"names": [
					"kill"
				],
				"action": "SCMP_ACT_ALLOW",
				"args": [
					{
					"index": 1,
					"value": 0,
					"op": "SCMP_CMP_EQ"
					}
				],
				"comment": "allow process detection via kill",
				"includes": {},
				"excludes": {}
				}
			]
		}
		`,
	}
}

// GetAgentRoleName returns the name of the role for the Agent
func GetAgentRoleName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), constants.DefaultAgentResourceSuffix)
}

func getDefaultServiceAccountName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", constants.GetDDAName(dda), constants.DefaultAgentResourceSuffix)
}

func agentImage() string {
	return images.GetLatestAgentImage()
}

func ddotCollectorImage() string {
	return images.GetLatestDdotCollectorImage()
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
		LivenessProbe:  constants.GetDefaultLivenessProbe(),
		ReadinessProbe: constants.GetDefaultReadinessProbe(),
		StartupProbe:   constants.GetDefaultStartupProbe(),
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
		LivenessProbe:  constants.GetDefaultLivenessProbe(),
		ReadinessProbe: constants.GetDefaultReadinessProbe(),
		StartupProbe:   constants.GetDefaultStartupProbe(),
	}
}

func traceAgentContainer(dda metav1.Object) corev1.Container {
	return corev1.Container{
		Name:  string(apicommon.TraceAgentContainerName),
		Image: agentImage(),
		Command: []string{
			"trace-agent",
			fmt.Sprintf("--config=%s", agentCustomConfigVolumePath),
		},
		Env:           envVarsForTraceAgent(dda),
		VolumeMounts:  volumeMountsForTraceAgent(),
		LivenessProbe: constants.GetDefaultTraceAgentProbe(),
	}
}

func processAgentContainer(dda metav1.Object) corev1.Container {
	return corev1.Container{
		Name:  string(apicommon.ProcessAgentContainerName),
		Image: agentImage(),
		Command: []string{
			"process-agent", fmt.Sprintf("--config=%s", agentCustomConfigVolumePath),
			fmt.Sprintf("--sysprobe-config=%s", systemProbeConfigVolumePath),
		},
		Env:          commonEnvVars(dda),
		VolumeMounts: volumeMountsForProcessAgent(),
	}
}

func otelAgentContainer(dda metav1.Object) corev1.Container {
	return corev1.Container{
		Name:  string(apicommon.OtelAgent),
		Image: ddotCollectorImage(),
		Command: []string{
			"otel-agent",
			"--core-config=" + agentCustomConfigVolumePath,
			"--sync-delay=30s",
		},
		Env: commonEnvVars(dda),
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
			"start", fmt.Sprintf("-c=%s", agentCustomConfigVolumePath),
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
			fmt.Sprintf("--config=%s", systemProbeConfigVolumePath),
		},
		Env:          commonEnvVars(dda),
		VolumeMounts: volumeMountsForSystemProbe(),
		SecurityContext: &corev1.SecurityContext{
			SeccompProfile: &corev1.SeccompProfile{
				Type:             corev1.SeccompProfileTypeLocalhost,
				LocalhostProfile: apiutils.NewStringPointer(common.SystemProbeSeccompProfileName),
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
			fmt.Sprintf("--config=%s", agentCustomConfigVolumePath),
		},
		Env:            commonEnvVars(dda),
		VolumeMounts:   volumeMountsForAgentDataPlane(),
		LivenessProbe:  constants.GetDefaultAgentDataPlaneLivenessProbe(),
		ReadinessProbe: constants.GetDefaultAgentDataPlaneReadinessProbe(),
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
				Name:      common.ConfigVolumeName,
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
			fmt.Sprintf("%s/%s", common.SeccompSecurityVolumePath, common.SystemProbeSeccompKey),
			fmt.Sprintf("%s/%s", common.SeccompRootVolumePath, common.SystemProbeSeccompProfileName),
		},
		VolumeMounts: volumeMountsForSeccompSetup(),
	}
}

func commonEnvVars(dda metav1.Object) []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  common.KubernetesEnvVar,
			Value: "yes",
		},
		{
			Name:  common.DDClusterAgentEnabled,
			Value: strconv.FormatBool(true),
		},
		{
			Name:  common.DDClusterAgentKubeServiceName,
			Value: componentdca.GetClusterAgentServiceName(dda),
		},
		{
			Name:  common.DDClusterAgentTokenName,
			Value: secrets.GetDefaultDCATokenSecretName(dda),
		},
		{
			Name:  common.DDAuthTokenFilePath,
			Value: filepath.Join(common.AuthVolumePath, "token"),
		},
		{
			Name: common.DDKubeletHost,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: common.FieldPathStatusHostIP,
				},
			},
		},
	}
}

func envVarsForCoreAgent(dda metav1.Object) []corev1.EnvVar {
	envs := []corev1.EnvVar{
		{
			Name:  common.DDHealthPort,
			Value: strconv.Itoa(int(constants.DefaultAgentHealthPort)),
		},
		{
			// we want to default it in 7.49.0
			// but in 7.50.0 it will be already defaulted in the agent process.
			Name:  DDContainerImageEnabled,
			Value: "true",
		},
	}

	return append(envs, commonEnvVars(dda)...)
}

func envVarsForTraceAgent(dda metav1.Object) []corev1.EnvVar {
	envs := []corev1.EnvVar{
		{
			Name: common.DDAPMInstrumentationInstallId,
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: common.APMTelemetryConfigMapName,
					},
					Key: common.APMTelemetryInstallIdKey,
				},
			},
		},
		{
			Name: common.DDAPMInstrumentationInstallTime,
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: common.APMTelemetryConfigMapName,
					},
					Key: common.APMTelemetryInstallTimeKey,
				},
			},
		},
		{
			Name: common.DDAPMInstrumentationInstallType,
			ValueFrom: &corev1.EnvVarSource{
				ConfigMapKeyRef: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: common.APMTelemetryConfigMapName,
					},
					Key: common.APMTelemetryInstallTypeKey,
				},
			},
		},
	}

	return append(envs, commonEnvVars(dda)...)
}

func envVarsForSecurityAgent(dda metav1.Object) []corev1.EnvVar {
	envs := []corev1.EnvVar{
		{
			Name:  "HOST_ROOT",
			Value: common.HostRootMountPath,
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
		common.GetVolumeMountForDogstatsdSocket(false),
		common.GetVolumeMountForProc(),
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
