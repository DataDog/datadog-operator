// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagentdeployment

import (
	"fmt"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
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

func (r *ReconcileDatadogAgentDeployment) manageSystemProbeDependencies(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment) (reconcile.Result, error) {
	if !isSystemProbeEnabled(dad) {
		result, err := r.cleanupSystemProbeDependencies(logger, dad)
		return result, err
	}

	result, err := r.manageConfigMap(logger, dad, buildSystemProbeConfigConfiMap)
	if shouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageConfigMap(logger, dad, buildSystemProbeSecCompConfigMap)
	if shouldReturn(result, err) {
		return result, err
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileDatadogAgentDeployment) cleanupSystemProbeDependencies(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment) (reconcile.Result, error) {
	result, err := r.cleanupConfigMap(logger, dad, getSystemProbeConfiConfigMapName(dad.Name))
	if shouldReturn(result, err) {
		return result, err
	}
	result, err = r.cleanupConfigMap(logger, dad, getSecCompConfigMapName(dad.Name))
	return result, err
}

func buildSystemProbeConfigConfiMap(dad *datadoghqv1alpha1.DatadogAgentDeployment) (*corev1.ConfigMap, error) {
	spec := &dad.Spec.Agent.SystemProbe
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        getSystemProbeConfiConfigMapName(dad.Name),
			Namespace:   dad.Namespace,
			Labels:      getDefaultLabels(dad, dad.Name, getAgentVersion(dad)),
			Annotations: getDefaultAnnotations(dad),
		},
		Data: map[string]string{
			"system-probe.yaml": fmt.Sprintf(systemProbeAgentSecurityDataTmpl,
				spec.DebugPort,
				datadoghqv1alpha1.BoolToString(spec.ConntrackEnabled),
				datadoghqv1alpha1.BoolToString(spec.BPFDebugEnabled)),
		},
	}

	return configMap, nil
}

const systemProbeAgentSecurityDataTmpl = `system_probe_config:
  enabled: true
  debug_port: %d
  sysprobe_socket: /opt/datadog-agent/run/sysprobe.sock
  enable_conntrack : %s
  bpf_debug: %s
`

func buildSystemProbeSecCompConfigMap(dad *datadoghqv1alpha1.DatadogAgentDeployment) (*corev1.ConfigMap, error) {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        getSecCompConfigMapName(dad.Name),
			Namespace:   dad.Namespace,
			Labels:      getDefaultLabels(dad, dad.Name, getAgentVersion(dad)),
			Annotations: getDefaultAnnotations(dad),
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
			"arch_prctl",
			"bind",
			"bpf",
			"brk",
			"capget",
			"capset",
			"chdir",
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
			"fsync",
			"futex",
			"getdents",
			"getdents64",
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
			"getxattr",
			"ioctl",
			"ipc",
			"listen",
			"lstat",
			"lstat64",
			"mkdir",
			"mkdirat",
			"mmap",
			"mmap2",
			"mprotect",
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
			"prlimit64",
			"read",
			"recvfrom",
			"recvmmsg",
			"recvmsg",
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
			"seccomp",
			"select",
			"semtimedop",
			"send",
			"sendmmsg",
			"sendmsg",
			"sendto",
			"setgid",
			"setgid32",
			"setgroups",
			"setgroups32",
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
			"umask",
			"uname",
			"unlink",
			"unlinkat",
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
		}
	]
}
`

func getSecCompConfigMapName(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, SystemProbeAgentSecurityConfigMapSuffixName)
}

func getSystemProbeConfiConfigMapName(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, SystemProbeConfigMapSuffixName)
}
