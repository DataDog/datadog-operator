// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"fmt"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// SystemProbeConfigMapSuffixName SystemProbe Config configmap name
	SystemProbeConfigMapSuffixName = "system-probe-config"
	// SystemProbeAgentSecurityConfigMapSuffixName AgentSecurity configmap name
	SystemProbeAgentSecurityConfigMapSuffixName = "system-probe-seccomp"
)

func (r *Reconciler) manageSystemProbeDependencies(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	result, err := r.manageConfigMap(logger, dda, getSystemProbeConfigConfigMapName(dda), buildSystemProbeConfigConfigMap)
	if utils.ShouldReturn(result, err) {
		return result, err
	}
	if apiutils.BoolValue(dda.Spec.Agent.Enabled) && getSeccompProfileName(dda.Spec.Agent.SystemProbe) == datadoghqv1alpha1.DefaultSeccompProfileName && dda.Spec.Agent.SystemProbe.SecCompCustomProfileConfigMap == "" {
		result, err = r.manageConfigMap(logger, dda, getSecCompConfigMapName(dda), buildSystemProbeSecCompConfigMap)
		if utils.ShouldReturn(result, err) {
			return result, err
		}
	}

	return reconcile.Result{}, nil
}

func shouldCreateSystemProbeConfigConfigMap(dda *datadoghqv1alpha1.DatadogAgent) bool {
	return isSystemProbeEnabled(&dda.Spec) &&
		(dda.Spec.Agent.SystemProbe == nil || dda.Spec.Agent.SystemProbe.CustomConfig == nil ||
			dda.Spec.Agent.SystemProbe.CustomConfig.ConfigMap == nil)
}

func shouldMountSystemProbeConfigConfigMap(dda *datadoghqv1alpha1.DatadogAgent) bool {
	return isSystemProbeEnabled(&dda.Spec)
}

func getSystemProbeConfigConfigMapName(dda *datadoghqv1alpha1.DatadogAgent) string {
	if dda.Spec.Agent.SystemProbe != nil && dda.Spec.Agent.SystemProbe.CustomConfig != nil && dda.Spec.Agent.SystemProbe.CustomConfig.ConfigMap != nil {
		return dda.Spec.Agent.SystemProbe.CustomConfig.ConfigMap.Name
	}
	return fmt.Sprintf("%s-%s", dda.Name, SystemProbeConfigMapSuffixName)
}

func getSystemProbeConfigFileName(dda *datadoghqv1alpha1.DatadogAgent) string {
	if dda.Spec.Agent.SystemProbe != nil &&
		dda.Spec.Agent.SystemProbe.CustomConfig != nil &&
		dda.Spec.Agent.SystemProbe.CustomConfig.ConfigMap != nil &&
		dda.Spec.Agent.SystemProbe.CustomConfig.ConfigMap.FileKey != "" {
		return dda.Spec.Agent.SystemProbe.CustomConfig.ConfigMap.FileKey
	}

	return datadoghqv1alpha1.SystemProbeConfigVolumeSubPath
}

func buildSystemProbeConfigConfigMap(dda *datadoghqv1alpha1.DatadogAgent) (*corev1.ConfigMap, error) {
	if !shouldCreateSystemProbeConfigConfigMap(dda) {
		return nil, nil
	}

	// Always create a ConfigMap with empty file as it may trigger WARN logs in the Agent
	customConfig := dda.Spec.Agent.SystemProbe.CustomConfig
	if customConfig == nil || customConfig.ConfigData == nil || *customConfig.ConfigData == "" {
		customConfig = &datadoghqv1alpha1.CustomConfigSpec{
			ConfigData: apiutils.NewStringPointer(" "),
		}
	}

	return buildConfigurationConfigMap(dda, datadoghqv1alpha1.ConvertCustomConfig(customConfig), getSystemProbeConfigConfigMapName(dda), getSystemProbeConfigFileName(dda))
}

func buildSystemProbeSecCompConfigMap(dda *datadoghqv1alpha1.DatadogAgent) (*corev1.ConfigMap, error) {
	if !shouldCreateSeccompConfigMap(dda) {
		return nil, nil
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        getSecCompConfigMapName(dda),
			Namespace:   dda.Namespace,
			Labels:      getDefaultLabels(dda, dda.Name, getAgentVersion(dda)),
			Annotations: getDefaultAnnotations(dda),
		},
		Data: map[string]string{
			"system-probe-seccomp.json": systemProbeSecCompData,
		},
	}

	return configMap, nil
}

const systemProbeSecCompData = `{
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
			"clock_gettime",
			"clone",
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
			"epoll_wait",
			"epoll_wait_old",
			"eventfd",
			"eventfd2",
			"execve",
			"execveat",
			"exit",
			"exit_group",
			"fchmod",
			"fchmodat",
			"fchown",
			"fchown32",
			"fchownat",
			"fcntl",
			"fcntl64",
			"fstat",
			"fstat64",
			"fstatfs",
			"fsync",
			"futex",
			"getcwd",
			"getdents",
			"getdents64",
			"getegid",
			"geteuid",
			"getgid",
			"getpeername",
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
			"ioctl",
			"ipc",
			"listen",
			"lseek",
			"lstat",
			"lstat64",
			"madvise",
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
			"restart_syscall",
			"rmdir",
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
			"setns",
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
			"sysinfo",
			"tgkill",
			"umask",
			"uname",
			"unlink",
			"unlinkat",
			"wait4",
			"waitid",
			"waitpid",
			"write",
			"getgroups",
			"getpgrp",
			"setpgid"
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
		}
	]
}
`

func shouldInstallSeccompProfileFromConfigMap(dda *datadoghqv1alpha1.DatadogAgent) bool {
	return shouldCreateSeccompConfigMap(dda) || dda.Spec.Agent.SystemProbe.SecCompCustomProfileConfigMap != ""
}

func shouldCreateSeccompConfigMap(dda *datadoghqv1alpha1.DatadogAgent) bool {
	return apiutils.BoolValue(dda.Spec.Agent.Enabled) &&
		isSystemProbeEnabled(&dda.Spec) &&
		getSeccompProfileName(dda.Spec.Agent.SystemProbe) == datadoghqv1alpha1.DefaultSeccompProfileName &&
		dda.Spec.Agent.SystemProbe.SecCompCustomProfileConfigMap == ""
}

func getSecCompConfigMapName(dda *datadoghqv1alpha1.DatadogAgent) string {
	if apiutils.BoolValue(dda.Spec.Agent.Enabled) && dda.Spec.Agent.SystemProbe.SecCompCustomProfileConfigMap != "" {
		return dda.Spec.Agent.SystemProbe.SecCompCustomProfileConfigMap
	}
	return fmt.Sprintf("%s-%s", dda.Name, SystemProbeAgentSecurityConfigMapSuffixName)
}
