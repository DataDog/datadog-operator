// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package enabledefault

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
