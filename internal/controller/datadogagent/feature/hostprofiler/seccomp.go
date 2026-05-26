// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package hostprofiler

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
)

const (
	securityVolumeName               = "host-profiler-security"
	securityVolumePath               = "/etc/config/host-profiler"
	seccompKey                       = "host-profiler-seccomp.json"
	agentSecurityConfigMapSuffixName = "host-profiler-seccomp"
	seccompProfileName               = "host-profiler"
)

func defaultCapabilities() []corev1.Capability {
	return []corev1.Capability{
		"BPF",
		"PERFMON",
		"SYS_PTRACE",
		"SYS_RESOURCE",
		"DAC_READ_SEARCH",
		"SYSLOG",
		"CHECKPOINT_RESTORE",
	}
}

func defaultSyscalls() []string {
	return []string{
		"accept4",
		"access",
		"arch_prctl",
		"bind",
		"bpf",
		"brk",
		"chmod",
		"clone",
		"clone3",
		"close",
		"connect",
		"dup3",
		"epoll_create1",
		"epoll_ctl",
		"epoll_pwait",
		"epoll_wait",
		"eventfd2",
		"execve",
		"exit",
		"exit_group",
		"faccessat2",
		"fcntl",
		"fstat",
		"fstatfs",
		"futex",
		"getcwd",
		"getdents64",
		"getpeername",
		"getpid",
		"getrandom",
		"getsockname",
		"getsockopt",
		"gettid",
		"getrlimit",
		"ioctl",
		"listen",
		"lseek",
		"madvise",
		"mmap",
		"mprotect",
		"munmap",
		"nanosleep",
		"newfstatat",
		"openat",
		"openat2",
		"perf_event_open",
		"pidfd_open",
		"pidfd_send_signal",
		"pipe2",
		"prctl",
		"pread64",
		"prlimit64",
		"process_vm_readv",
		"read",
		"readlinkat",
		"recvmsg",
		"restart_syscall",
		"rseq",
		"rt_sigaction",
		"rt_sigprocmask",
		"rt_sigreturn",
		"sched_getaffinity",
		"sched_yield",
		"sendto",
		"set_robust_list",
		"set_tid_address",
		"setrlimit",
		"setsockopt",
		"sigaltstack",
		"socket",
		"statfs",
		"statx",
		"sysinfo",
		"tgkill",
		"umask",
		"uname",
		"unlinkat",
		"wait4",
		"waitid",
		"write",
	}
}

func defaultSeccompConfigData() map[string]string {
	syscalls := fmt.Sprintf(`["%s"]`, strings.Join(defaultSyscalls(), `","`))

	return map[string]string{
		seccompKey: fmt.Sprintf(`{
			"defaultAction": "SCMP_ACT_ERRNO",
			"architectures": [
				"SCMP_ARCH_X86_64",
				"SCMP_ARCH_AARCH64"
			],
			"syscalls": [
				{
				"names": %s,
				"action": "SCMP_ACT_ALLOW"
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
				"comment": "allow process liveness check via kill(pid, 0)"
				}
			]
		}
		`, syscalls),
	}
}

func buildSeccompSetupInitContainer(image string) corev1.Container {
	return corev1.Container{
		Name:  "host-profiler-seccomp-setup",
		Image: image,
		Command: []string{
			"cp",
			fmt.Sprintf("%s/%s", securityVolumePath, seccompKey),
			fmt.Sprintf("%s/%s", common.SeccompRootVolumePath, seccompProfileName),
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      securityVolumeName,
				MountPath: securityVolumePath,
			},
			common.GetVolumeMountForSeccomp(),
		},
	}
}
