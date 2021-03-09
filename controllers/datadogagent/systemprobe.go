// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"fmt"
	"path/filepath"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
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
	result, err := r.manageConfigMap(logger, dda, getSystemProbeConfigConfigMapName(dda.Name), buildSystemProbeConfigConfiMap)
	if shouldReturn(result, err) {
		return result, err
	}

	if dda.Spec.Agent != nil && getSeccompProfileName(&dda.Spec.Agent.SystemProbe) == datadoghqv1alpha1.DefaultSeccompProfileName && dda.Spec.Agent.SystemProbe.SecCompCustomProfileConfigMap == "" {
		result, err = r.manageConfigMap(logger, dda, getSecCompConfigMapName(dda.Name), buildSystemProbeSecCompConfigMap)
		if shouldReturn(result, err) {
			return result, err
		}
	}

	return reconcile.Result{}, nil
}

func buildSystemProbeConfigConfiMap(dda *datadoghqv1alpha1.DatadogAgent) (*corev1.ConfigMap, error) {
	if !isSystemProbeEnabled(&dda.Spec) {
		return nil, nil
	}

	spec := &dda.Spec.Agent.SystemProbe
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        getSystemProbeConfigConfigMapName(dda.Name),
			Namespace:   dda.Namespace,
			Labels:      getDefaultLabels(dda, dda.Name, getAgentVersion(dda)),
			Annotations: getDefaultAnnotations(dda),
		},
		Data: map[string]string{
			datadoghqv1alpha1.SystemProbeConfigVolumeSubPath: fmt.Sprintf(systemProbeAgentSecurityDataTmpl,
				spec.DebugPort,
				filepath.Join(datadoghqv1alpha1.SystemProbeSocketVolumePath, "sysprobe.sock"),
				datadoghqv1alpha1.BoolToString(spec.ConntrackEnabled),
				datadoghqv1alpha1.BoolToString(spec.BPFDebugEnabled),
				datadoghqv1alpha1.BoolToString(spec.EnableTCPQueueLength),
				datadoghqv1alpha1.BoolToString(spec.EnableOOMKill),
				datadoghqv1alpha1.BoolToString(spec.CollectDNSStats),
				isRuntimeSecurityEnabled(&dda.Spec),
				filepath.Join(datadoghqv1alpha1.SystemProbeSocketVolumePath, "runtime-security.sock"),
				datadoghqv1alpha1.SecurityAgentRuntimePoliciesDirVolumePath,
				isSyscallMonitorEnabled(&dda.Spec),
			),
		},
	}

	return configMap, nil
}

const systemProbeAgentSecurityDataTmpl = `system_probe_config:
  enabled: true
  debug_port: %d
  sysprobe_socket: %s
  enable_conntrack: %s
  bpf_debug: %s
  enable_tcp_queue_length: %s
  enable_oom_kill: %s
  collect_dns_stats: %s
runtime_security_config:
  enabled: %v
  debug: false
  socket: %s
  policies:
    dir: %s
  syscall_monitor:
    enabled: %v
`

func buildSystemProbeSecCompConfigMap(dda *datadoghqv1alpha1.DatadogAgent) (*corev1.ConfigMap, error) {
	if !isSystemProbeEnabled(&dda.Spec) {
		return nil, nil
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        getSecCompConfigMapName(dda.Name),
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

func getSystemProbeConfigConfigMapName(prefix string) string {
	return fmt.Sprintf("%s-%s", prefix, SystemProbeConfigMapSuffixName)
}
