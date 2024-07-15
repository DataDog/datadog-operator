// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"fmt"
	"strconv"

	edsv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component"
	componentdca "github.com/DataDog/datadog-operator/controllers/datadogagent/component/clusteragent"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/defaulting"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewDefaultAgentDaemonset return a new default agent DaemonSet
func NewDefaultAgentDaemonset(dda metav1.Object, edsOptions *ExtendedDaemonsetOptions, agentComponent feature.RequiredComponent) *appsv1.DaemonSet {
	daemonset := NewDaemonset(dda, edsOptions, apicommon.DefaultAgentResourceSuffix, component.GetAgentName(dda), component.GetAgentVersion(dda), nil)
	podTemplate := NewDefaultAgentPodTemplateSpec(dda, agentComponent, daemonset.GetLabels())
	daemonset.Spec.Template = *podTemplate
	return daemonset
}

// NewDefaultAgentExtendedDaemonset return a new default agent DaemonSet
func NewDefaultAgentExtendedDaemonset(dda metav1.Object, edsOptions *ExtendedDaemonsetOptions, agentComponent feature.RequiredComponent) *edsv1alpha1.ExtendedDaemonSet {
	edsDaemonset := NewExtendedDaemonset(dda, edsOptions, apicommon.DefaultAgentResourceSuffix, component.GetAgentName(dda), component.GetAgentVersion(dda), nil)
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

func getDefaultServiceAccountName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), apicommon.DefaultAgentResourceSuffix)
}

func agentImage() string {
	return fmt.Sprintf("%s/%s:%s", apicommon.DefaultImageRegistry, apicommon.DefaultAgentImageName, defaulting.AgentLatestVersion)
}

func initContainers(dda metav1.Object, requiredContainers []common.AgentContainerName) []corev1.Container {
	initContainers := []corev1.Container{
		initVolumeContainer(),
		initConfigContainer(dda),
	}
	for _, containerName := range requiredContainers {
		if containerName == common.SystemProbeContainerName {
			initContainers = append(initContainers, initSeccompSetupContainer())
		}
	}

	return initContainers
}

func agentSingleContainer(dda metav1.Object) []corev1.Container {
	agentSingleContainer := corev1.Container{
		Name:           string(common.UnprivilegedSingleAgentContainerName),
		Image:          agentImage(),
		Env:            envVarsForCoreAgent(dda),
		VolumeMounts:   volumeMountsForCoreAgent(),
		LivenessProbe:  apicommon.GetDefaultLivenessProbe(),
		ReadinessProbe: apicommon.GetDefaultReadinessProbe(),
	}

	containers := []corev1.Container{
		agentSingleContainer,
	}

	return containers
}

func agentOptimizedContainers(dda metav1.Object, requiredContainers []common.AgentContainerName) []corev1.Container {
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
		case common.OtelAgent:
			containers = append(containers, otelAgentContainer(dda))
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
		StartupProbe:   apicommon.GetDefaultStartupProbe(),
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
		Env:           envVarsForTraceAgent(dda),
		VolumeMounts:  volumeMountsForTraceAgent(),
		LivenessProbe: apicommon.GetDefaultTraceAgentProbe(),
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
		Env:          commonEnvVars(dda),
		VolumeMounts: volumeMountsForProcessAgent(),
	}
}

func otelAgentContainer(dda metav1.Object) corev1.Container {
	return corev1.Container{
		Name:  string(common.OtelAgent),
		Image: agentImage(),
		Command: []string{
			"/otel-agent",
			fmt.Sprintf("--config=%s", apicommon.OtelCustomConfigVolumePath),
		},
		Env:          envVarsForOtelAgent(dda),
		VolumeMounts: volumeMountsForOtelAgent(),
		Ports: []corev1.ContainerPort{
			{
				Name:          "grpc",
				ContainerPort: 4317,
				HostPort:      4317,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "http",
				ContainerPort: 4318,
				HostPort:      4318,
				Protocol:      corev1.ProtocolTCP,
			},
		},
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
		Env:          envVarsForSecurityAgent(dda),
		VolumeMounts: volumeMountsForSecurityAgent(),
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
		Env:          commonEnvVars(dda),
		VolumeMounts: volumeMountsForSystemProbe(),
		SecurityContext: &corev1.SecurityContext{
			SeccompProfile: &corev1.SeccompProfile{
				Type:             corev1.SeccompProfileTypeLocalhost,
				LocalhostProfile: apiutils.NewStringPointer(apicommon.SystemProbeSeccompProfileName),
			},
		},
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
			fmt.Sprintf("%s/%s", apicommon.SeccompSecurityVolumePath, apicommon.SystemProbeSeccompKey),
			fmt.Sprintf("%s/%s", apicommon.SeccompRootVolumePath, apicommon.SystemProbeSeccompProfileName),
		},
		VolumeMounts: volumeMountsForSeccompSetup(),
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
			Value: apicommon.EnvVarTrueValue,
		},
		{
			// we want to default it in 7.49.0
			// but in 7.50.0 it will be already defaulted in the agent process.
			Name:  apicommon.DDContainerImageEnabled,
			Value: apicommon.EnvVarTrueValue,
		},
	}

	return append(envs, commonEnvVars(dda)...)
}

func envVarsForTraceAgent(dda metav1.Object) []corev1.EnvVar {
	envs := []corev1.EnvVar{
		{
			Name:  apicommon.DDAPMInstrumentationInstallId,
			Value: utils.GetDatadogAgentResourceUID(dda),
		},
		{
			Name:  apicommon.DDAPMInstrumentationInstallTime,
			Value: utils.GetDatadogAgentResourceCreationTime(dda),
		},
		{
			Name:  apicommon.DDAPMInstrumentationInstallType,
			Value: component.DefaultAgentInstallType,
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

func envVarsForOtelAgent(dda metav1.Object) []corev1.EnvVar {
	envs := []corev1.EnvVar{
		// TODO: add additional env vars here
	}

	return append(envs, commonEnvVars(dda)...)
}

func volumeMountsForInitConfig() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		component.GetVolumeMountForLogs(),
		component.GetVolumeMountForChecksd(),
		component.GetVolumeMountForAuth(false),
		component.GetVolumeMountForConfd(),
		component.GetVolumeMountForConfig(),
		component.GetVolumeMountForProc(),
		component.GetVolumeMountForRuntimeSocket(true),
	}
}

func volumesForAgent(dda metav1.Object, requiredContainers []common.AgentContainerName) []corev1.Volume {
	volumes := []corev1.Volume{
		component.GetVolumeForLogs(),
		component.GetVolumeForAuth(),
		component.GetVolumeInstallInfo(dda),
		component.GetVolumeForChecksd(),
		component.GetVolumeForConfd(),
		component.GetVolumeForConfig(),
		component.GetVolumeForProc(),
		component.GetVolumeForCgroups(),
		component.GetVolumeForDogstatsd(),
		component.GetVolumeForRuntimeSocket(),
	}

	for _, containerName := range requiredContainers {
		if containerName == common.SystemProbeContainerName {
			sysProbeVolumes := []corev1.Volume{
				component.GetVolumeForSecurity(dda),
				component.GetVolumeForSeccomp(),
			}
			volumes = append(volumes, sysProbeVolumes...)
		}
	}

	return volumes
}

func volumeMountsForCoreAgent() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		component.GetVolumeMountForLogs(),
		component.GetVolumeMountForAuth(false),
		component.GetVolumeMountForInstallInfo(),
		component.GetVolumeMountForConfig(),
		component.GetVolumeMountForProc(),
		component.GetVolumeMountForCgroups(),
		component.GetVolumeMountForDogstatsdSocket(false),
		component.GetVolumeMountForRuntimeSocket(true),
	}
}

func volumeMountsForTraceAgent() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		component.GetVolumeMountForLogs(),
		component.GetVolumeMountForProc(),
		component.GetVolumeMountForCgroups(),
		component.GetVolumeMountForAuth(true),
		component.GetVolumeMountForConfig(),
		component.GetVolumeMountForDogstatsdSocket(false),
		component.GetVolumeMountForRuntimeSocket(true),
	}
}

func volumeMountsForProcessAgent() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		component.GetVolumeMountForLogs(),
		component.GetVolumeMountForAuth(true),
		component.GetVolumeMountForConfig(),
		component.GetVolumeMountForDogstatsdSocket(false),
		component.GetVolumeMountForRuntimeSocket(true),
		component.GetVolumeMountForProc(),
	}
}

func volumeMountsForSecurityAgent() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		component.GetVolumeMountForLogs(),
		component.GetVolumeMountForAuth(true),
		component.GetVolumeMountForConfig(),
		component.GetVolumeMountForDogstatsdSocket(false),
		component.GetVolumeMountForRuntimeSocket(true),
	}
}

func volumeMountsForSystemProbe() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		component.GetVolumeMountForLogs(),
		component.GetVolumeMountForAuth(true),
		component.GetVolumeMountForConfig(),
	}
}

func volumeMountsForSeccompSetup() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		component.GetVolumeMountForSecurity(),
		component.GetVolumeMountForSeccomp(),
	}
}

func volumeMountsForOtelAgent() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		// TODO: add/remove volume mounts
		component.GetVolumeMountForLogs(),
		component.GetVolumeMountForAuth(true),
		component.GetVolumeMountForConfig(),
		component.GetVolumeMountForDogstatsdSocket(false),
		component.GetVolumeMountForRuntimeSocket(true),
		component.GetVolumeMountForProc(),
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
					"clock_gettime",
					"clone",
					"clone3",
					"close",
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
					"flock",
					"fstat",
					"fstat64",
					"fstatfs",
					"fsync",
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
					"pipe",
					"pipe2",
					"poll",
					"ppoll",
					"prctl",
					"pread64",
					"prlimit64",
					"pselect6",
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
