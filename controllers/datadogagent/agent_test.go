// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"fmt"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	test "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1/test"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/orchestrator"
	"github.com/DataDog/datadog-operator/pkg/defaulting"
	"github.com/DataDog/datadog-operator/pkg/testutils"

	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	assert "github.com/stretchr/testify/require"
)

const (
	testDdaName     = "foo"
	agentConfigFile = "/etc/datadog-agent/datadog.yaml"
)

func apiKeyValue() *corev1.EnvVarSource {
	return &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: testDdaName,
			},
			Key: "api_key",
		},
	}
}

func appKeyValue() *corev1.EnvVarSource {
	return &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: testDdaName,
			},
			Key: "app_key",
		},
	}
}

func authTokenValue() *corev1.EnvVarSource {
	return &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: testDdaName,
			},
			Key: "token",
		},
	}
}

func defaultLivenessProbe() *corev1.Probe {
	return &corev1.Probe{
		InitialDelaySeconds: 15,
		PeriodSeconds:       15,
		TimeoutSeconds:      5,
		SuccessThreshold:    1,
		FailureThreshold:    6,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/live",
				Port: intstr.IntOrString{
					IntVal: 5555,
				},
			},
		},
	}
}

func defaultAPMAgentLivenessProbe() *corev1.Probe {
	return &corev1.Probe{
		InitialDelaySeconds: 15,
		PeriodSeconds:       15,
		TimeoutSeconds:      5,
		SuccessThreshold:    0,
		FailureThreshold:    0,
		ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{
				Port: intstr.IntOrString{
					IntVal: 8126,
				},
			},
		},
	}
}

func defaultReadinessProbe() *corev1.Probe {
	return &corev1.Probe{
		InitialDelaySeconds: 15,
		PeriodSeconds:       15,
		TimeoutSeconds:      5,
		SuccessThreshold:    1,
		FailureThreshold:    6,
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/ready",
				Port: intstr.IntOrString{
					IntVal: 5555,
				},
			},
		},
	}
}

func defaultVolumes() []corev1.Volume {
	return []corev1.Volume{
		{
			Name: datadoghqv1alpha1.LogDatadogVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: apicommon.AuthVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: datadoghqv1alpha1.DogstatsdSocketVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: apicommon.InstallInfoVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "foo-install-info",
					},
				},
			},
		},
		{
			Name: apicommon.ConfdVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: datadoghqv1alpha1.ChecksdVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: apicommon.ConfigVolumeName,
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
		{
			Name: "runtimesocketdir",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/run",
				},
			},
		},
	}
}

func defaultSystemProbeVolumes() []corev1.Volume {
	fileOrCreate := corev1.HostPathFileOrCreate
	return []corev1.Volume{
		{
			Name: datadoghqv1alpha1.LogDatadogVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: apicommon.AuthVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: apicommon.InstallInfoVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "foo-install-info",
					},
				},
			},
		},
		{
			Name: apicommon.ConfdVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: datadoghqv1alpha1.ChecksdVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: apicommon.ConfigVolumeName,
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
		{
			Name: "runtimesocketdir",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/run",
				},
			},
		},
		{
			Name: datadoghqv1alpha1.SystemProbeAgentSecurityVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "foo-system-probe-seccomp",
					},
				},
			},
		},
		{
			Name: datadoghqv1alpha1.SystemProbeConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "foo-system-probe-config",
					},
				},
			},
		},
		{
			Name: datadoghqv1alpha1.SystemProbeSecCompRootVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/lib/kubelet/seccomp",
				},
			},
		},
		{
			Name: datadoghqv1alpha1.SystemProbeDebugfsVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/sys/kernel/debug",
				},
			},
		},
		{
			Name: datadoghqv1alpha1.DogstatsdSocketVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
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
}

func complianceSecurityAgentVolumes() []corev1.Volume {
	return []corev1.Volume{
		{
			Name: datadoghqv1alpha1.LogDatadogVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: apicommon.AuthVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: datadoghqv1alpha1.DogstatsdSocketVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: apicommon.InstallInfoVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "foo-install-info",
					},
				},
			},
		},
		{
			Name: apicommon.ConfdVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: datadoghqv1alpha1.ChecksdVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: datadoghqv1alpha1.SecurityAgentComplianceConfigDirVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: apicommon.ConfigVolumeName,
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
		{
			Name: "runtimesocketdir",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/run",
				},
			},
		},
		{
			Name: datadoghqv1alpha1.PasswdVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/passwd",
				},
			},
		},
		{
			Name: datadoghqv1alpha1.GroupVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/etc/group",
				},
			},
		},
		{
			Name: datadoghqv1alpha1.HostRootVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/",
				},
			},
		},
	}
}

func runtimeSecurityAgentVolumes() []corev1.Volume {
	fileOrCreate := corev1.HostPathFileOrCreate
	return []corev1.Volume{
		{
			Name: datadoghqv1alpha1.LogDatadogVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: apicommon.AuthVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: apicommon.InstallInfoVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "foo-install-info",
					},
				},
			},
		},
		{
			Name: apicommon.ConfdVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: datadoghqv1alpha1.ChecksdVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: apicommon.ConfigVolumeName,
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
		{
			Name: "runtimesocketdir",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/run",
				},
			},
		},
		{
			Name: datadoghqv1alpha1.SystemProbeAgentSecurityVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "foo-system-probe-seccomp",
					},
				},
			},
		},
		{
			Name: datadoghqv1alpha1.SystemProbeConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "foo-system-probe-config",
					},
				},
			},
		},
		{
			Name: datadoghqv1alpha1.SystemProbeSecCompRootVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/lib/kubelet/seccomp",
				},
			},
		},
		{
			Name: datadoghqv1alpha1.SystemProbeDebugfsVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/sys/kernel/debug",
				},
			},
		},
		{
			Name: datadoghqv1alpha1.DogstatsdSocketVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
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
		{
			Name: datadoghqv1alpha1.GroupVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: datadoghqv1alpha1.GroupVolumePath,
				},
			},
		},
		{
			Name: datadoghqv1alpha1.HostRootVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/",
				},
			},
		},
		{
			Name: datadoghqv1alpha1.SecurityAgentRuntimeCustomPoliciesVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "test-runtime-policies",
					},
				},
			},
		},
		{
			Name: datadoghqv1alpha1.SecurityAgentRuntimePoliciesDirVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}
}

func defaultMountVolume() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "logdatadog",
			MountPath: "/var/log/datadog",
		},
		{
			Name:      "datadog-agent-auth",
			MountPath: "/etc/datadog-agent/auth",
		},
		{
			Name:      "dsdsocket",
			MountPath: "/var/run/datadog/statsd",
			ReadOnly:  true,
		},

		{
			Name:      "dsdsocket",
			MountPath: "/var/run/datadog/statsd",
		},
		{
			Name:      "installinfo",
			SubPath:   "install_info",
			MountPath: "/etc/datadog-agent/install_info",
			ReadOnly:  true,
		},
		{
			Name:      "confd",
			MountPath: "/conf.d",
			ReadOnly:  true,
		},
		{
			Name:      "checksd",
			MountPath: "/checks.d",
			ReadOnly:  true,
		},
		{
			Name:      "procdir",
			MountPath: "/host/proc",
			ReadOnly:  true,
		},
		{
			Name:      "cgroups",
			MountPath: "/host/sys/fs/cgroup",
			ReadOnly:  true,
		},
		{
			Name:      "config",
			MountPath: "/etc/datadog-agent",
		},
		{
			Name:      "runtimesocketdir",
			MountPath: "/host/var/run",
			ReadOnly:  true,
		},
	}
}

func defaultProcessMountVolumes() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "logdatadog",
			MountPath: "/var/log/datadog",
		},
		{
			Name:      "datadog-agent-auth",
			MountPath: "/etc/datadog-agent/auth",
			ReadOnly:  true,
		},
		{
			Name:      "dsdsocket",
			MountPath: "/var/run/datadog/statsd",
			ReadOnly:  true,
		},
		{
			Name:      "cgroups",
			MountPath: "/host/sys/fs/cgroup",
			ReadOnly:  true,
		},
		{
			Name:      "procdir",
			MountPath: "/host/proc",
			ReadOnly:  true,
		},
		{
			Name:      "config",
			MountPath: "/etc/datadog-agent",
			ReadOnly:  false,
		},
		{
			Name:      "runtimesocketdir",
			MountPath: "/host/var/run",
			ReadOnly:  true,
		},
	}
}

func defaultSystemProbeMountVolume() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "logdatadog",
			MountPath: "/var/log/datadog",
		},
		{
			Name:      "datadog-agent-auth",
			MountPath: "/etc/datadog-agent/auth",
			ReadOnly:  true,
		},
		{
			Name:      "debugfs",
			MountPath: "/sys/kernel/debug",
		},
		{
			Name:      "system-probe-config",
			SubPath:   "system-probe.yaml",
			MountPath: "/etc/datadog-agent/system-probe.yaml",
		},
		{
			Name:      "sysprobe-socket-dir",
			MountPath: "/var/run/sysprobe",
		},
		{
			Name:      "procdir",
			MountPath: "/host/proc",
			ReadOnly:  true,
		},
		{
			Name:      "host-osrelease",
			MountPath: "/host/etc/os-release",
			ReadOnly:  true,
		},
	}
}

func complianceSecurityAgentMountVolume() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "logdatadog",
			MountPath: "/var/log/datadog",
		},
		{
			Name:      "datadog-agent-auth",
			MountPath: "/etc/datadog-agent/auth",
			ReadOnly:  true,
		},
		{
			Name:      "dsdsocket",
			MountPath: "/var/run/datadog/statsd",
			ReadOnly:  true,
		},
		{
			Name:      "config",
			MountPath: "/etc/datadog-agent",
		},
		{
			Name:      "hostroot",
			MountPath: "/host/root",
			ReadOnly:  true,
		},
		{
			Name:      "cgroups",
			MountPath: "/host/sys/fs/cgroup",
			ReadOnly:  true,
		},
		{
			Name:      "passwd",
			MountPath: "/etc/passwd",
			ReadOnly:  true,
		},
		{
			Name:      "group",
			MountPath: "/etc/group",
			ReadOnly:  true,
		},
		{
			Name:      "procdir",
			MountPath: "/host/proc",
			ReadOnly:  true,
		},
		{
			Name:      "runtimesocketdir",
			MountPath: "/host/var/run",
			ReadOnly:  true,
		},
		{
			Name:      "runtimesocketdir",
			MountPath: "/host/root/var/run",
			ReadOnly:  true,
		},
	}
}

func runtimeSecurityAgentMountVolume() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      "logdatadog",
			MountPath: "/var/log/datadog",
		},
		{
			Name:      "datadog-agent-auth",
			MountPath: "/etc/datadog-agent/auth",
			ReadOnly:  true,
		},
		{
			Name:      "dsdsocket",
			MountPath: "/var/run/datadog/statsd",
			ReadOnly:  true,
		},
		{
			Name:      "config",
			MountPath: "/etc/datadog-agent",
		},
		{
			Name:      "hostroot",
			MountPath: "/host/root",
			ReadOnly:  true,
		},
		{
			Name:      "runtimesocketdir",
			MountPath: "/host/var/run",
			ReadOnly:  true,
		},
		{
			Name:      "sysprobe-socket-dir",
			MountPath: "/var/run/sysprobe",
			ReadOnly:  true,
		},
	}
}

func defaultEnvVars(extraEnv map[string]string) []corev1.EnvVar {
	envs := []corev1.EnvVar{
		{
			Name:  "DD_HEALTH_PORT",
			Value: "5555",
		},
		{
			Name:  "DD_COLLECT_KUBERNETES_EVENTS",
			Value: "false",
		},
		{
			Name:  "DD_LEADER_ELECTION",
			Value: "false",
		},
		{
			Name:  "DD_DOGSTATSD_ORIGIN_DETECTION",
			Value: "false",
		},
		{
			Name:  "DD_DOGSTATSD_SOCKET",
			Value: "/var/run/datadog/statsd/statsd.sock",
		},
		{
			Name:  "DD_LOGS_ENABLED",
			Value: "false",
		},
		{
			Name:  "DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL",
			Value: "false",
		},
		{
			Name:  "DD_LOGS_CONFIG_K8S_CONTAINER_USE_FILE",
			Value: "false",
		},
		{
			Name:  "DD_LOG_LEVEL",
			Value: "INFO",
		},
		{
			Name:  "KUBERNETES",
			Value: "yes",
		},
		{
			Name:  "DD_DOGSTATSD_SOCKET",
			Value: "/var/run/datadog/statsd/statsd.sock",
		},
		{
			Name: "DD_KUBERNETES_KUBELET_HOST",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: FieldPathStatusHostIP,
				},
			},
		},
		{
			Name:      "DD_API_KEY",
			ValueFrom: apiKeyValue(),
		},
	}

	if ddSite := createEnvFromExtra(extraEnv, "DD_SITE"); ddSite != nil {
		envs = append(envs, *ddSite)
	}

	envs = append(envs, []corev1.EnvVar{
		{
			Name:  "DD_CLUSTER_AGENT_ENABLED",
			Value: "true",
		},
		{
			Name:  "DD_CLUSTER_AGENT_KUBERNETES_SERVICE_NAME",
			Value: fmt.Sprintf("%s-%s", testDdaName, apicommon.DefaultClusterAgentResourceSuffix),
		},
		{
			Name:      "DD_CLUSTER_AGENT_AUTH_TOKEN",
			ValueFrom: authTokenValue(),
		},
	}...)

	return envs
}

func defaultAPMContainerEnvVars() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "DD_APM_ENABLED",
			Value: "true",
		},
		{
			Name:  "DD_LOG_LEVEL",
			Value: "INFO",
		},
		{
			Name:  "KUBERNETES",
			Value: "yes",
		},
		{
			Name: "DD_KUBERNETES_KUBELET_HOST",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: FieldPathStatusHostIP,
				},
			},
		},
		{
			Name:      "DD_API_KEY",
			ValueFrom: apiKeyValue(),
		},
		{
			Name:  "DD_DOGSTATSD_SOCKET",
			Value: "/var/run/datadog/statsd/statsd.sock",
		},
	}
}

func defaultSystemProbeEnvVars() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "DD_LOG_LEVEL",
			Value: "INFO",
		},
		{
			Name:  "KUBERNETES",
			Value: "yes",
		},
		{
			Name: "DD_KUBERNETES_KUBELET_HOST",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: FieldPathStatusHostIP,
				},
			},
		},
		{
			Name:  datadoghqv1alpha1.DDRuntimeSecurityConfigEnabled,
			Value: "false",
		},
		{
			Name:  datadoghqv1alpha1.DDRuntimeSecurityConfigRemoteTaggerEnabled,
			Value: "false",
		},
		{
			Name:  datadoghqv1alpha1.DDRuntimeSecurityConfigSyscallMonitorEnabled,
			Value: "false",
		},
		{
			Name:  datadoghqv1alpha1.DDSystemProbeDebugPort,
			Value: "0",
		},
		{
			Name:  datadoghqv1alpha1.DDSystemProbeSocketPath,
			Value: "/var/run/sysprobe/sysprobe.sock",
		},
		{
			Name:  datadoghqv1alpha1.DDSystemProbeNPMEnabled,
			Value: "false",
		},
		{
			Name:  datadoghqv1alpha1.DDSystemProbeConntrackEnabled,
			Value: "false",
		},
		{
			Name:  datadoghqv1alpha1.DDSystemProbeBPFDebugEnabled,
			Value: "false",
		},
		{
			Name:  datadoghqv1alpha1.DDSystemProbeCollectDNSStatsEnabled,
			Value: "false",
		},
		{
			Name:  datadoghqv1alpha1.DDSystemProbeTCPQueueLengthEnabled,
			Value: "false",
		},
		{
			Name:  datadoghqv1alpha1.DDSystemProbeOOMKillEnabled,
			Value: "false",
		},
	}
}

func securityAgentEnvVars(compliance, runtime bool, policiesdir bool, extraEnv map[string]string) []corev1.EnvVar {
	env := []corev1.EnvVar{
		{
			Name:  "DD_COMPLIANCE_CONFIG_ENABLED",
			Value: strconv.FormatBool(compliance),
		},
		{
			Name:  "HOST_ROOT",
			Value: "/host/root",
		},
		{
			Name:  "DD_DOGSTATSD_SOCKET",
			Value: "/var/run/datadog/statsd/statsd.sock",
		},
	}

	if compliance {
		if envDuration := createEnvFromExtra(extraEnv, "DD_COMPLIANCE_CONFIG_CHECK_INTERVAL"); envDuration != nil {
			env = append(env, *envDuration)
		}
	}

	env = append(env, []corev1.EnvVar{
		{
			Name:  "DD_RUNTIME_SECURITY_CONFIG_ENABLED",
			Value: strconv.FormatBool(runtime),
		},
	}...)

	if runtime {
		env = append(env, []corev1.EnvVar{
			{
				Name:  "DD_RUNTIME_SECURITY_CONFIG_SOCKET",
				Value: "/var/run/sysprobe/runtime-security.sock",
			},
			{
				Name:  "DD_RUNTIME_SECURITY_CONFIG_SYSCALL_MONITOR_ENABLED",
				Value: "true",
			},
			{
				Name:  "DD_RUNTIME_SECURITY_CONFIG_POLICIES_DIR",
				Value: "/etc/datadog-agent/runtime-security.d",
			},
		}...)
	}

	env = append(env, []corev1.EnvVar{
		{
			Name:  "DD_LOG_LEVEL",
			Value: "INFO",
		},
		{
			Name:  "KUBERNETES",
			Value: "yes",
		},
		{
			Name: "DD_KUBERNETES_KUBELET_HOST",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: FieldPathStatusHostIP,
				},
			},
		},
		{
			Name:      "DD_API_KEY",
			ValueFrom: apiKeyValue(),
		},
	}...)

	if ddSite := createEnvFromExtra(extraEnv, "DD_SITE"); ddSite != nil {
		env = append(env, *ddSite)
	}

	env = append(env, []corev1.EnvVar{
		{
			Name:  "DD_CLUSTER_AGENT_ENABLED",
			Value: "true",
		},
		{
			Name:  "DD_CLUSTER_AGENT_KUBERNETES_SERVICE_NAME",
			Value: fmt.Sprintf("%s-%s", testDdaName, apicommon.DefaultClusterAgentResourceSuffix),
		},
		{
			Name:      "DD_CLUSTER_AGENT_AUTH_TOKEN",
			ValueFrom: authTokenValue(),
		},
	}...)
	return env
}

func createEnvFromExtra(extraEnv map[string]string, envKey string) *corev1.EnvVar {
	if extraEnv != nil && extraEnv[envKey] != "" {
		return &corev1.EnvVar{
			Name:  envKey,
			Value: extraEnv[envKey],
		}
	}
	return nil
}

func addEnvVar(currentVars []corev1.EnvVar, varName string, varValue string) []corev1.EnvVar {
	for i := range currentVars {
		if currentVars[i].Name == varName {
			currentVars[i].Value = varValue
			return currentVars
		}
	}

	return append(currentVars, corev1.EnvVar{Name: varName, Value: varValue})
}

func appendDefaultAPMAgentContainer(podSpec *corev1.PodSpec) {
	apmContainer := corev1.Container{
		Name:            "trace-agent",
		Image:           defaulting.GetLatestAgentImage(),
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"trace-agent", "--config=" + agentConfigFile},
		Resources:       corev1.ResourceRequirements{},
		Ports:           []corev1.ContainerPort{{Name: "traceport", ContainerPort: 8126, Protocol: "TCP", HostPort: 8126}},
		Env:             defaultAPMContainerEnvVars(),
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "logdatadog",
				MountPath: "/var/log/datadog",
			},
			{
				Name:      "datadog-agent-auth",
				MountPath: "/etc/datadog-agent/auth",
				ReadOnly:  true,
			},
			{
				Name:      "dsdsocket",
				MountPath: "/var/run/datadog/statsd",
				ReadOnly:  true,
			},
			{
				Name:      "config",
				MountPath: "/etc/datadog-agent",
			},
			{
				Name:      "custom-datadog-yaml",
				ReadOnly:  true,
				MountPath: agentConfigFile,
				SubPath:   "datadog.yaml",
			},
		},
		LivenessProbe: defaultAPMAgentLivenessProbe(),
	}
	podSpec.Containers = []corev1.Container{
		podSpec.Containers[0],
		apmContainer,
		podSpec.Containers[1],
	}
}

func defaultSystemProbePodSpec(dda *datadoghqv1alpha1.DatadogAgent) corev1.PodSpec {
	agentWithSystemProbeVolumeMounts := []corev1.VolumeMount{}
	agentWithSystemProbeVolumeMounts = append(agentWithSystemProbeVolumeMounts, defaultMountVolume()...)
	agentWithSystemProbeVolumeMounts = append(agentWithSystemProbeVolumeMounts, []corev1.VolumeMount{
		{
			Name:      "sysprobe-socket-dir",
			ReadOnly:  true,
			MountPath: "/var/run/sysprobe",
		},
		{
			Name:      "system-probe-config",
			MountPath: "/etc/datadog-agent/system-probe.yaml",
			SubPath:   "system-probe.yaml",
		},
	}...)

	agentEnvVars := addEnvVar(defaultEnvVars(nil), datadoghqv1alpha1.DDSystemProbeSocketPath, filepath.Join(datadoghqv1alpha1.SystemProbeSocketVolumePath, "sysprobe.sock"))
	agentEnvVars = append(agentEnvVars, []corev1.EnvVar{
		{
			Name:  datadoghqv1alpha1.DDSystemProbeTCPQueueLengthEnabled,
			Value: "false",
		},
		{
			Name:  datadoghqv1alpha1.DDSystemProbeOOMKillEnabled,
			Value: "false",
		},
	}...)

	return corev1.PodSpec{
		ServiceAccountName: "foo-agent",
		InitContainers: []corev1.Container{
			{
				Name:            "init-volume",
				Image:           defaulting.GetLatestAgentImage(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Resources:       corev1.ResourceRequirements{},
				Command:         []string{"bash", "-c"},
				Args:            []string{"cp -vnr /etc/datadog-agent /opt"},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      apicommon.ConfigVolumeName,
						MountPath: "/opt/datadog-agent",
					},
				},
			},
			{
				Name:            "init-config",
				Image:           defaulting.GetLatestAgentImage(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Resources:       corev1.ResourceRequirements{},
				Command:         []string{"bash", "-c"},
				Args:            []string{"for script in $(find /etc/cont-init.d/ -type f -name '*.sh' | sort) ; do bash $script ; done"},
				Env:             agentEnvVars,
				VolumeMounts:    agentWithSystemProbeVolumeMounts,
			},
			{
				Name:            "seccomp-setup",
				Image:           defaulting.GetLatestAgentImage(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Resources:       corev1.ResourceRequirements{},
				Command:         []string{"cp", "/etc/config/system-probe-seccomp.json", "/host/var/lib/kubelet/seccomp/system-probe"},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      datadoghqv1alpha1.SystemProbeAgentSecurityVolumeName,
						MountPath: "/etc/config",
					},
					{
						Name:      datadoghqv1alpha1.SystemProbeSecCompRootVolumeName,
						MountPath: "/host/var/lib/kubelet/seccomp",
					},
				},
			},
		},
		Containers: []corev1.Container{
			{
				Name:            "agent",
				Image:           defaulting.GetLatestAgentImage(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Command: []string{
					"agent",
					"run",
				},
				Resources: corev1.ResourceRequirements{},
				Ports: []corev1.ContainerPort{
					{
						ContainerPort: 8125,
						Name:          "dogstatsdport",
						Protocol:      "UDP",
					},
				},
				Env:            agentEnvVars,
				VolumeMounts:   agentWithSystemProbeVolumeMounts,
				LivenessProbe:  defaultLivenessProbe(),
				ReadinessProbe: defaultReadinessProbe(),
			},
			{
				Name:            "system-probe",
				Image:           defaulting.GetLatestAgentImage(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Command: []string{
					"system-probe",
					"--config=/etc/datadog-agent/system-probe.yaml",
				},
				SecurityContext: &corev1.SecurityContext{
					Capabilities: &corev1.Capabilities{
						Add: []corev1.Capability{"SYS_ADMIN", "SYS_RESOURCE", "SYS_PTRACE", "NET_ADMIN", "NET_BROADCAST", "NET_RAW", "IPC_LOCK", "CHOWN"},
					},
					RunAsUser: apiutils.NewInt64Pointer(0),
				},
				Resources:    corev1.ResourceRequirements{},
				Env:          defaultSystemProbeEnvVars(),
				VolumeMounts: defaultSystemProbeMountVolume(),
			},
		},
		Volumes: defaultSystemProbeVolumes(),
		SecurityContext: &corev1.PodSecurityContext{
			RunAsUser: apiutils.NewInt64Pointer(0),
		},
	}
}

func noSeccompInstallSystemProbeSpec(dda *datadoghqv1alpha1.DatadogAgent) corev1.PodSpec {
	agentWithSystemProbeVolumeMounts := []corev1.VolumeMount{}
	agentWithSystemProbeVolumeMounts = append(agentWithSystemProbeVolumeMounts, defaultMountVolume()...)
	agentWithSystemProbeVolumeMounts = append(agentWithSystemProbeVolumeMounts, []corev1.VolumeMount{
		{
			Name:      "sysprobe-socket-dir",
			ReadOnly:  true,
			MountPath: "/var/run/sysprobe",
		},
		{
			Name:      "system-probe-config",
			MountPath: "/etc/datadog-agent/system-probe.yaml",
			SubPath:   "system-probe.yaml",
		},
	}...)
	agentEnvVars := addEnvVar(defaultEnvVars(nil), datadoghqv1alpha1.DDSystemProbeSocketPath, filepath.Join(datadoghqv1alpha1.SystemProbeSocketVolumePath, "sysprobe.sock"))
	agentEnvVars = append(agentEnvVars, []corev1.EnvVar{
		{
			Name:  datadoghqv1alpha1.DDSystemProbeTCPQueueLengthEnabled,
			Value: "false",
		},
		{
			Name:  datadoghqv1alpha1.DDSystemProbeOOMKillEnabled,
			Value: "false",
		},
	}...)

	// Remove volumes for seccomp profile install
	var volumes []corev1.Volume
	for _, vol := range defaultSystemProbeVolumes() {
		if vol.Name == datadoghqv1alpha1.SystemProbeSecCompRootVolumeName || vol.Name == datadoghqv1alpha1.SystemProbeAgentSecurityVolumeName {
			continue
		}
		volumes = append(volumes, vol)
	}

	return corev1.PodSpec{
		ServiceAccountName: "foo-agent",
		InitContainers: []corev1.Container{
			{
				Name:            "init-volume",
				Image:           defaulting.GetLatestAgentImage(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Resources:       corev1.ResourceRequirements{},
				Command:         []string{"bash", "-c"},
				Args:            []string{"cp -vnr /etc/datadog-agent /opt"},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      apicommon.ConfigVolumeName,
						MountPath: "/opt/datadog-agent",
					},
				},
			},
			{
				Name:            "init-config",
				Image:           defaulting.GetLatestAgentImage(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Resources:       corev1.ResourceRequirements{},
				Command:         []string{"bash", "-c"},
				Args:            []string{"for script in $(find /etc/cont-init.d/ -type f -name '*.sh' | sort) ; do bash $script ; done"},
				Env:             agentEnvVars,
				VolumeMounts:    agentWithSystemProbeVolumeMounts,
			},
		},
		Containers: []corev1.Container{
			{
				Name:            "agent",
				Image:           defaulting.GetLatestAgentImage(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Command: []string{
					"agent",
					"run",
				},
				Resources: corev1.ResourceRequirements{},
				Ports: []corev1.ContainerPort{
					{
						ContainerPort: 8125,
						Name:          "dogstatsdport",
						Protocol:      "UDP",
					},
				},
				Env:            agentEnvVars,
				VolumeMounts:   agentWithSystemProbeVolumeMounts,
				LivenessProbe:  defaultLivenessProbe(),
				ReadinessProbe: defaultReadinessProbe(),
			},
			{
				Name:            "system-probe",
				Image:           defaulting.GetLatestAgentImage(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Command: []string{
					"system-probe",
					"--config=/etc/datadog-agent/system-probe.yaml",
				},
				SecurityContext: &corev1.SecurityContext{
					Capabilities: &corev1.Capabilities{
						Add: []corev1.Capability{"SYS_ADMIN", "SYS_RESOURCE", "SYS_PTRACE", "NET_ADMIN", "NET_BROADCAST", "NET_RAW", "IPC_LOCK", "CHOWN"},
					},
					RunAsUser: apiutils.NewInt64Pointer(0),
				},
				Resources:    corev1.ResourceRequirements{},
				Env:          defaultSystemProbeEnvVars(),
				VolumeMounts: defaultSystemProbeMountVolume(),
			},
		},
		Volumes: volumes,
		SecurityContext: &corev1.PodSecurityContext{
			RunAsUser: apiutils.NewInt64Pointer(0),
		},
	}
}

func defaultPodSpec(dda *datadoghqv1alpha1.DatadogAgent) corev1.PodSpec {
	cmd := []string{
		"cp -vnr /etc/datadog-agent /opt",
	}
	if isRuntimeSecurityEnabled(&dda.Spec) && dda.Spec.Agent.Security.Runtime.PoliciesDir != nil {
		cmd = append(cmd, "cp -v /etc/datadog-agent-runtime-policies/* /opt/datadog-agent/runtime-security.d/")
	}
	return corev1.PodSpec{
		ServiceAccountName: "foo-agent",
		InitContainers: []corev1.Container{
			{
				Name:            "init-volume",
				Image:           defaulting.GetLatestAgentImage(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Resources:       corev1.ResourceRequirements{},
				Command:         []string{"bash", "-c"},
				Args:            cmd,
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      apicommon.ConfigVolumeName,
						MountPath: "/opt/datadog-agent",
					},
				},
			},
			{
				Name:            "init-config",
				Image:           defaulting.GetLatestAgentImage(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Resources:       corev1.ResourceRequirements{},
				Command:         []string{"bash", "-c"},
				Args:            []string{"for script in $(find /etc/cont-init.d/ -type f -name '*.sh' | sort) ; do bash $script ; done"},
				Env:             defaultEnvVars(nil),
				VolumeMounts:    defaultMountVolume(),
			},
		},
		Containers: []corev1.Container{
			{
				Name:            "agent",
				Image:           defaulting.GetLatestAgentImage(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Command: []string{
					"agent",
					"run",
				},
				Resources: corev1.ResourceRequirements{},
				Ports: []corev1.ContainerPort{
					{
						ContainerPort: 8125,
						Name:          "dogstatsdport",
						Protocol:      "UDP",
					},
				},
				Env:            defaultEnvVars(nil),
				VolumeMounts:   defaultMountVolume(),
				LivenessProbe:  defaultLivenessProbe(),
				ReadinessProbe: defaultReadinessProbe(),
			},
			{
				Name:            "process-agent",
				Image:           defaulting.GetLatestAgentImage(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Command: []string{
					"process-agent",
					"--config=/etc/datadog-agent/datadog.yaml",
					"--sysprobe-config=/etc/datadog-agent/system-probe.yaml",
				},
				Resources:    corev1.ResourceRequirements{},
				Env:          defaultOrchestratorEnvVars(dda),
				VolumeMounts: defaultProcessMountVolumes(),
			},
		},
		Volumes: defaultProcessMount(),
		SecurityContext: &corev1.PodSecurityContext{
			RunAsUser: apiutils.NewInt64Pointer(0),
		},
	}
}

func defaultProcessMount() []corev1.Volume {
	return []corev1.Volume{
		{
			Name: datadoghqv1alpha1.LogDatadogVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: apicommon.AuthVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: datadoghqv1alpha1.DogstatsdSocketVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: apicommon.InstallInfoVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "foo-install-info",
					},
				},
			},
		},
		{
			Name: apicommon.ConfdVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: datadoghqv1alpha1.ChecksdVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: apicommon.ConfigVolumeName,
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
		{
			Name: "runtimesocketdir",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/run",
				},
			},
		},
	}
}

func defaultOrchestratorEnvVars(dda *datadoghqv1alpha1.DatadogAgent) []corev1.EnvVar {
	newVars := []corev1.EnvVar{
		{
			Name:  "DD_SYSTEM_PROBE_ENABLED",
			Value: "false",
		},
	}

	explorerConfig := datadoghqv1alpha1.OrchestratorExplorerConfig{
		Enabled:   apiutils.NewBoolPointer(true),
		Scrubbing: &datadoghqv1alpha1.Scrubbing{Containers: apiutils.NewBoolPointer(true)},
	}

	vars := []corev1.EnvVar{
		{
			Name:  "DD_LOG_LEVEL",
			Value: "INFO",
		},
		{
			Name:  "KUBERNETES",
			Value: "yes",
		},
		{
			Name:  "DD_DOGSTATSD_SOCKET",
			Value: "/var/run/datadog/statsd/statsd.sock",
		},
		{
			Name: "DD_KUBERNETES_KUBELET_HOST",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: FieldPathStatusHostIP,
				},
			},
		},
		{
			Name:      "DD_API_KEY",
			ValueFrom: apiKeyValue(),
		},
	}
	orchestratorEnvs, _ := orchestrator.EnvVars(&explorerConfig)
	newVars = append(newVars, orchestratorEnvs...)
	newVars = append(newVars, envForClusterAgentConnection(dda)...)

	return append(newVars, vars...)
}

func runtimeSecurityAgentPodSpec(extraEnv map[string]string, extraDir string) corev1.PodSpec {
	systemProbeEnv := defaultSystemProbeEnvVars()
	systemProbeEnv = addEnvVar(systemProbeEnv, datadoghqv1alpha1.DDAuthTokenFilePath, "/etc/datadog-agent/auth/token")
	systemProbeEnv = addEnvVar(systemProbeEnv, datadoghqv1alpha1.DDRuntimeSecurityConfigEnabled, "true")
	systemProbeEnv = addEnvVar(systemProbeEnv, datadoghqv1alpha1.DDRuntimeSecurityConfigPoliciesDir, "/etc/datadog-agent/runtime-security.d")
	systemProbeEnv = addEnvVar(systemProbeEnv, datadoghqv1alpha1.DDRuntimeSecurityConfigRemoteTaggerEnabled, "true")
	systemProbeEnv = addEnvVar(systemProbeEnv, datadoghqv1alpha1.DDRuntimeSecurityConfigSocket, "/var/run/sysprobe/runtime-security.sock")
	systemProbeEnv = addEnvVar(systemProbeEnv, datadoghqv1alpha1.DDRuntimeSecurityConfigSyscallMonitorEnabled, "true")

	agentWithSystemProbeVolumeMounts := []corev1.VolumeMount{}
	agentWithSystemProbeVolumeMounts = append(agentWithSystemProbeVolumeMounts, defaultMountVolume()...)
	agentWithSystemProbeVolumeMounts = append(agentWithSystemProbeVolumeMounts, []corev1.VolumeMount{
		{
			Name:      "sysprobe-socket-dir",
			ReadOnly:  true,
			MountPath: "/var/run/sysprobe",
		},
		{
			Name:      "system-probe-config",
			MountPath: "/etc/datadog-agent/system-probe.yaml",
			SubPath:   "system-probe.yaml",
		},
	}...)
	agentEnvVars := addEnvVar(defaultEnvVars(extraEnv), datadoghqv1alpha1.DDSystemProbeSocketPath, filepath.Join(datadoghqv1alpha1.SystemProbeSocketVolumePath, "sysprobe.sock"))
	agentEnvVars = append(agentEnvVars, []corev1.EnvVar{
		{
			Name:  datadoghqv1alpha1.DDSystemProbeTCPQueueLengthEnabled,
			Value: "false",
		},
		{
			Name:  datadoghqv1alpha1.DDSystemProbeOOMKillEnabled,
			Value: "false",
		},
	}...)
	command := []string{"cp -vnr /etc/datadog-agent /opt"}

	volumeMountsBuilder := NewVolumeMountBuilder([]corev1.VolumeMount{
		{
			Name:      apicommon.ConfigVolumeName,
			MountPath: "/opt/datadog-agent",
		},
	}, nil)

	volumesBuilder := NewVolumeBuilder(runtimeSecurityAgentVolumes(), nil)
	if extraDir != "" {
		command = []string{"cp -vnr /etc/datadog-agent /opt;cp -v /etc/datadog-agent-runtime-policies/* /opt/datadog-agent/runtime-security.d/"}

		volumeMountsBuilder.Add(&corev1.VolumeMount{
			Name:      datadoghqv1alpha1.SecurityAgentRuntimePoliciesDirVolumeName,
			MountPath: "/opt/datadog-agent/runtime-security.d",
		})
		volumeMountsBuilder.Add(&corev1.VolumeMount{
			Name:      datadoghqv1alpha1.SecurityAgentRuntimeCustomPoliciesVolumeName,
			MountPath: "/etc/datadog-agent-runtime-policies",
		})

		volumesBuilder.Add(&corev1.Volume{
			Name: datadoghqv1alpha1.SecurityAgentRuntimeCustomPoliciesVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: extraDir,
					},
				},
			},
		})

		volumesBuilder.Add(&corev1.Volume{
			Name: datadoghqv1alpha1.SecurityAgentRuntimePoliciesDirVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		})
	}
	return corev1.PodSpec{
		ServiceAccountName: "foo-agent",
		HostPID:            false,
		InitContainers: []corev1.Container{
			{
				Name:            "init-volume",
				Image:           defaulting.GetLatestAgentImage(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Resources:       corev1.ResourceRequirements{},
				Command:         []string{"bash", "-c"},
				Args:            command,
				VolumeMounts:    volumeMountsBuilder.Build(),
			},
			{
				Name:            "init-config",
				Image:           defaulting.GetLatestAgentImage(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Resources:       corev1.ResourceRequirements{},
				Command:         []string{"bash", "-c"},
				Args:            []string{"for script in $(find /etc/cont-init.d/ -type f -name '*.sh' | sort) ; do bash $script ; done"},
				Env:             agentEnvVars,
				VolumeMounts:    agentWithSystemProbeVolumeMounts,
			},
			{
				Name:            "seccomp-setup",
				Image:           defaulting.GetLatestAgentImage(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Resources:       corev1.ResourceRequirements{},
				Command:         []string{"cp", "/etc/config/system-probe-seccomp.json", "/host/var/lib/kubelet/seccomp/system-probe"},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      datadoghqv1alpha1.SystemProbeAgentSecurityVolumeName,
						MountPath: "/etc/config",
					},
					{
						Name:      datadoghqv1alpha1.SystemProbeSecCompRootVolumeName,
						MountPath: "/host/var/lib/kubelet/seccomp",
					},
				},
			},
		},
		Containers: []corev1.Container{
			{
				Name:            "agent",
				Image:           defaulting.GetLatestAgentImage(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Command: []string{
					"agent",
					"run",
				},
				Resources: corev1.ResourceRequirements{},
				Ports: []corev1.ContainerPort{
					{
						ContainerPort: 8125,
						Name:          "dogstatsdport",
						Protocol:      "UDP",
					},
				},
				Env:            agentEnvVars,
				VolumeMounts:   agentWithSystemProbeVolumeMounts,
				LivenessProbe:  defaultLivenessProbe(),
				ReadinessProbe: defaultReadinessProbe(),
			},
			{
				Name:            "system-probe",
				Image:           defaulting.GetLatestAgentImage(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Command: []string{
					"system-probe",
					"--config=/etc/datadog-agent/system-probe.yaml",
				},
				SecurityContext: &corev1.SecurityContext{
					Capabilities: &corev1.Capabilities{
						Add: []corev1.Capability{"SYS_ADMIN", "SYS_RESOURCE", "SYS_PTRACE", "NET_ADMIN", "NET_BROADCAST", "NET_RAW", "IPC_LOCK", "CHOWN"},
					},
					RunAsUser: apiutils.NewInt64Pointer(0),
				},
				Resources: corev1.ResourceRequirements{},
				Env:       systemProbeEnv,
				VolumeMounts: append(
					defaultSystemProbeMountVolume(),
					corev1.VolumeMount{
						Name:      "runtimepoliciesdir",
						MountPath: "/etc/datadog-agent/runtime-security.d",
						ReadOnly:  true,
					},
				),
			},
			{
				Name:            "security-agent",
				Image:           defaulting.GetLatestAgentImage(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Command: []string{
					"security-agent",
					"start",
					"-c=" + agentConfigFile,
				},
				SecurityContext: &corev1.SecurityContext{
					Capabilities: &corev1.Capabilities{
						Add: []corev1.Capability{"AUDIT_CONTROL", "AUDIT_READ"},
					},
					RunAsUser: apiutils.NewInt64Pointer(0),
				},
				Resources:    corev1.ResourceRequirements{},
				Env:          securityAgentEnvVars(false, true, true, extraEnv),
				VolumeMounts: runtimeSecurityAgentMountVolume(),
			},
		},
		Volumes: volumesBuilder.Build(),
		SecurityContext: &corev1.PodSecurityContext{
			RunAsUser: apiutils.NewInt64Pointer(0),
		},
	}
}

func complianceSecurityAgentPodSpec(extraEnv map[string]string) corev1.PodSpec {
	return corev1.PodSpec{
		ServiceAccountName: "foo-agent",
		HostPID:            true,
		InitContainers: []corev1.Container{
			{
				Name:            "init-volume",
				Image:           defaulting.GetLatestAgentImage(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Resources:       corev1.ResourceRequirements{},
				Command:         []string{"bash", "-c"},
				Args:            []string{"cp -vnr /etc/datadog-agent /opt"},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      apicommon.ConfigVolumeName,
						MountPath: "/opt/datadog-agent",
					},
				},
			},
			{
				Name:            "init-config",
				Image:           defaulting.GetLatestAgentImage(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Resources:       corev1.ResourceRequirements{},
				Command:         []string{"bash", "-c"},
				Args:            []string{"for script in $(find /etc/cont-init.d/ -type f -name '*.sh' | sort) ; do bash $script ; done"},
				Env:             defaultEnvVars(extraEnv),
				VolumeMounts:    defaultMountVolume(),
			},
		},
		Containers: []corev1.Container{
			{
				Name:            "agent",
				Image:           defaulting.GetLatestAgentImage(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Command: []string{
					"agent",
					"run",
				},
				Resources: corev1.ResourceRequirements{},
				Ports: []corev1.ContainerPort{
					{
						ContainerPort: 8125,
						Name:          "dogstatsdport",
						Protocol:      "UDP",
					},
				},
				Env:            defaultEnvVars(extraEnv),
				VolumeMounts:   defaultMountVolume(),
				LivenessProbe:  defaultLivenessProbe(),
				ReadinessProbe: defaultReadinessProbe(),
			},
			{
				Name:            "security-agent",
				Image:           defaulting.GetLatestAgentImage(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Command: []string{
					"security-agent",
					"start",
					"-c=" + agentConfigFile,
				},
				SecurityContext: &corev1.SecurityContext{
					Capabilities: &corev1.Capabilities{
						Add: []corev1.Capability{"AUDIT_CONTROL", "AUDIT_READ"},
					},
					RunAsUser: apiutils.NewInt64Pointer(0),
				},
				Resources:    corev1.ResourceRequirements{},
				Env:          securityAgentEnvVars(true, false, false, extraEnv),
				VolumeMounts: complianceSecurityAgentMountVolume(),
			},
		},
		Volumes: complianceSecurityAgentVolumes(),
		SecurityContext: &corev1.PodSecurityContext{
			RunAsUser: apiutils.NewInt64Pointer(0),
		},
	}
}

func customKubeletConfigPodSpec(kubeletConfig *commonv1.KubeletConfig) corev1.PodSpec {
	kubeletCAVolumeType := corev1.HostPathFile
	KubeletCAVolume := corev1.Volume{
		Name: "kubelet-ca",
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: kubeletConfig.HostCAPath,
				Type: &kubeletCAVolumeType,
			},
		},
	}
	VolumeBuilder := NewVolumeBuilder(defaultVolumes(), nil).Add(&KubeletCAVolume)

	volumeMountsBuilder := NewVolumeMountBuilder(defaultMountVolume(), nil)
	volumeMountsBuilder.Add(&corev1.VolumeMount{
		Name:      datadoghqv1alpha1.KubeletCAVolumeName,
		ReadOnly:  true,
		MountPath: kubeletConfig.AgentCAPath,
	})

	envVars := []corev1.EnvVar{
		{
			Name:  "DD_HEALTH_PORT",
			Value: "5555",
		},
		{
			Name:  "DD_COLLECT_KUBERNETES_EVENTS",
			Value: "false",
		},
		{
			Name:  "DD_LEADER_ELECTION",
			Value: "false",
		},
		{
			Name:  "DD_DOGSTATSD_ORIGIN_DETECTION",
			Value: "false",
		},
		{
			Name:  "DD_DOGSTATSD_SOCKET",
			Value: "/var/run/datadog/statsd/statsd.sock",
		},
		{
			Name:  "DD_LOGS_ENABLED",
			Value: "false",
		},
		{
			Name:  "DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL",
			Value: "false",
		},
		{
			Name:  "DD_LOGS_CONFIG_K8S_CONTAINER_USE_FILE",
			Value: "false",
		},
		{
			Name:  "DD_LOG_LEVEL",
			Value: "INFO",
		},
		{
			Name:  "KUBERNETES",
			Value: "yes",
		},
		{
			Name:      "DD_KUBERNETES_KUBELET_HOST",
			ValueFrom: kubeletConfig.Host,
		},
		{
			Name:  "DD_KUBELET_TLS_VERIFY",
			Value: apiutils.BoolToString(kubeletConfig.TLSVerify),
		},
		{
			Name:  "DD_KUBELET_CLIENT_CA",
			Value: kubeletConfig.AgentCAPath,
		},
		{
			Name:      "DD_API_KEY",
			ValueFrom: apiKeyValue(),
		},
		{
			Name:      "DD_CLUSTER_AGENT_AUTH_TOKEN",
			ValueFrom: authTokenValue(),
		},
		{
			Name:  "DD_CLUSTER_AGENT_ENABLED",
			Value: "true",
		},
		{
			Name:  "DD_CLUSTER_AGENT_KUBERNETES_SERVICE_NAME",
			Value: "foo-cluster-agent",
		},
	}

	return corev1.PodSpec{
		ServiceAccountName: "foo-agent",
		InitContainers: []corev1.Container{
			{
				Name:            "init-volume",
				Image:           defaulting.GetLatestAgentImage(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Resources:       corev1.ResourceRequirements{},
				Command:         []string{"bash", "-c"},
				Args:            []string{"cp -vnr /etc/datadog-agent /opt"},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      apicommon.ConfigVolumeName,
						MountPath: "/opt/datadog-agent",
					},
				},
			},
			{
				Name:            "init-config",
				Image:           defaulting.GetLatestAgentImage(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Resources:       corev1.ResourceRequirements{},
				Command:         []string{"bash", "-c"},
				Args:            []string{"for script in $(find /etc/cont-init.d/ -type f -name '*.sh' | sort) ; do bash $script ; done"},
				Env:             envVars,
				VolumeMounts:    volumeMountsBuilder.Build(),
			},
		},
		Containers: []corev1.Container{
			{
				Name:            "agent",
				Image:           defaulting.GetLatestAgentImage(),
				ImagePullPolicy: corev1.PullIfNotPresent,
				Command: []string{
					"agent",
					"run",
				},
				Resources: corev1.ResourceRequirements{},
				Ports: []corev1.ContainerPort{
					{
						ContainerPort: 8125,
						Name:          "dogstatsdport",
						Protocol:      "UDP",
					},
				},
				Env:            envVars,
				VolumeMounts:   volumeMountsBuilder.Build(),
				LivenessProbe:  defaultLivenessProbe(),
				ReadinessProbe: defaultReadinessProbe(),
			},
		},
		Volumes: VolumeBuilder.Build(),
		SecurityContext: &corev1.PodSecurityContext{
			RunAsUser: apiutils.NewInt64Pointer(0),
		},
	}
}

type extendedDaemonSetFromInstanceTest struct {
	name            string
	agentdeployment *datadoghqv1alpha1.DatadogAgent
	selector        *metav1.LabelSelector
	want            *edsdatadoghqv1alpha1.ExtendedDaemonSet
	wantErr         bool
}

func (test extendedDaemonSetFromInstanceTest) Run(t *testing.T) {
	t.Helper()
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	logger := logf.Log.WithName(t.Name())
	got, _, err := newExtendedDaemonSetFromInstance(logger, test.agentdeployment, test.selector)
	if test.wantErr {
		assert.Error(t, err, "newExtendedDaemonSetFromInstance() expected an error")
	} else {
		assert.NoError(t, err, "newExtendedDaemonSetFromInstance() unexpected error: %v", err)
	}

	// Remove the generated hash before comparison because it is not easy generate it in the test definition.
	delete(got.Annotations, apicommon.MD5AgentDeploymentAnnotationKey)

	diff := testutils.CompareKubeResource(got, test.want)
	assert.True(t, len(diff) == 0, diff)
}

type extendedDaemonSetFromInstanceTestSuite []extendedDaemonSetFromInstanceTest

func (tests extendedDaemonSetFromInstanceTestSuite) Run(t *testing.T) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, tt.Run)
	}
}

func Test_newExtendedDaemonSetFromInstance(t *testing.T) {
	defaultDatadogAgent := test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{UseEDS: true, ClusterAgentEnabled: true})

	// Create a Datadog Agent with a custom host port
	hostPortAgent := test.NewDefaultedDatadogAgent("bar", "foo",
		&test.NewDatadogAgentOptions{
			UseEDS:              true,
			ClusterAgentEnabled: true,
			HostPort:            apicommon.DefaultDogstatsdPort,
		})
	hostPortPodSpec := defaultPodSpec(hostPortAgent)
	hostPortPodSpec.Containers[0].Ports[0].HostPort = apicommon.DefaultDogstatsdPort

	// Create a Datadog Agent with a custom host port and host network set to true
	hostPortNetworkAgent := test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{
		UseEDS:              true,
		ClusterAgentEnabled: true,
		HostPort:            12345,
		HostNetwork:         true,
	})
	hostPortNetworkPodSpec := defaultPodSpec(hostPortNetworkAgent)
	hostPortNetworkPodSpec.HostNetwork = true
	hostPortNetworkPodSpec.Containers[0].Ports[0].ContainerPort = 12345
	hostPortNetworkPodSpec.Containers[0].Ports[0].HostPort = 12345
	hostPortNetworkPodSpec.Containers[0].Env = append(hostPortNetworkPodSpec.Containers[0].Env, corev1.EnvVar{
		Name:  datadoghqv1alpha1.DDDogstatsdPort,
		Value: strconv.Itoa(12345),
	})

	tests := extendedDaemonSetFromInstanceTestSuite{
		{
			name:            "defaulted case",
			agentdeployment: defaultDatadogAgent,
			wantErr:         false,
			want: &edsdatadoghqv1alpha1.ExtendedDaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "bar",
					Name:      "foo-agent",
					Labels: map[string]string{
						"agent.datadoghq.com/name":      "foo",
						"agent.datadoghq.com/component": "agent",
						"app.kubernetes.io/instance":    "agent",
						"app.kubernetes.io/managed-by":  "datadog-operator",
						"app.kubernetes.io/name":        "datadog-agent-deployment",
						"app.kubernetes.io/part-of":     "bar-foo",
						"app.kubernetes.io/version":     "",
					},
					Annotations: map[string]string{},
				},
				Spec: edsdatadoghqv1alpha1.ExtendedDaemonSetSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							GenerateName: "foo",
							Namespace:    "bar",
							Labels: map[string]string{
								"agent.datadoghq.com/name":      "foo",
								"agent.datadoghq.com/component": "agent",
								"app.kubernetes.io/instance":    "agent",
								"app.kubernetes.io/managed-by":  "datadog-operator",
								"app.kubernetes.io/name":        "datadog-agent-deployment",
								"app.kubernetes.io/part-of":     "bar-foo",
								"app.kubernetes.io/version":     "",
							},
							Annotations: make(map[string]string),
						},
						Spec: defaultPodSpec(defaultDatadogAgent),
					},
					Strategy: getDefaultEDSStrategy(),
				},
			},
		},
		{
			name:            "with labels and annotations",
			agentdeployment: test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{UseEDS: true, ClusterAgentEnabled: true, Labels: map[string]string{"label-foo-key": "label-bar-value", "tags.datadoghq.com/env": "test"}, Annotations: map[string]string{"annotations-foo-key": "annotations-bar-value"}}),
			wantErr:         false,
			want: &edsdatadoghqv1alpha1.ExtendedDaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "bar",
					Name:      "foo-agent",
					Labels: map[string]string{
						"agent.datadoghq.com/name":      "foo",
						"agent.datadoghq.com/component": "agent",
						"tags.datadoghq.com/env":        "test",
						"app.kubernetes.io/instance":    "agent",
						"app.kubernetes.io/managed-by":  "datadog-operator",
						"app.kubernetes.io/name":        "datadog-agent-deployment",
						"app.kubernetes.io/part-of":     "bar-foo",
						"app.kubernetes.io/version":     "",
					},
					Annotations: map[string]string{},
				},
				Spec: edsdatadoghqv1alpha1.ExtendedDaemonSetSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							GenerateName: "foo",
							Namespace:    "bar",
							Labels: map[string]string{
								"agent.datadoghq.com/name":      "foo",
								"agent.datadoghq.com/component": "agent",
								"app.kubernetes.io/instance":    "agent",
								"app.kubernetes.io/managed-by":  "datadog-operator",
								"app.kubernetes.io/name":        "datadog-agent-deployment",
								"app.kubernetes.io/part-of":     "bar-foo",
								"app.kubernetes.io/version":     "",
								"tags.datadoghq.com/env":        "test",
							},
							Annotations: map[string]string{},
						},
						Spec: defaultPodSpec(defaultDatadogAgent),
					},
					Strategy: getDefaultEDSStrategy(),
				},
			},
		},
		{
			name:            "with host port",
			agentdeployment: hostPortAgent,
			wantErr:         false,
			want: &edsdatadoghqv1alpha1.ExtendedDaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "bar",
					Name:      "foo-agent",
					Labels: map[string]string{
						"agent.datadoghq.com/name":      "foo",
						"agent.datadoghq.com/component": "agent",
						"app.kubernetes.io/instance":    "agent",
						"app.kubernetes.io/managed-by":  "datadog-operator",
						"app.kubernetes.io/name":        "datadog-agent-deployment",
						"app.kubernetes.io/part-of":     "bar-foo",
						"app.kubernetes.io/version":     "",
					},
					Annotations: map[string]string{},
				},
				Spec: edsdatadoghqv1alpha1.ExtendedDaemonSetSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							GenerateName: "foo",
							Namespace:    "bar",
							Labels: map[string]string{
								"agent.datadoghq.com/name":      "foo",
								"agent.datadoghq.com/component": "agent",
								"app.kubernetes.io/instance":    "agent",
								"app.kubernetes.io/managed-by":  "datadog-operator",
								"app.kubernetes.io/name":        "datadog-agent-deployment",
								"app.kubernetes.io/part-of":     "bar-foo",
								"app.kubernetes.io/version":     "",
							},
							Annotations: make(map[string]string),
						},
						Spec: hostPortPodSpec,
					},
					Strategy: getDefaultEDSStrategy(),
				},
			},
		},
		{
			name:            "with host port and host network",
			agentdeployment: hostPortNetworkAgent,
			wantErr:         false,
			want: &edsdatadoghqv1alpha1.ExtendedDaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "bar",
					Name:      "foo-agent",
					Labels: map[string]string{
						"agent.datadoghq.com/name":      "foo",
						"agent.datadoghq.com/component": "agent",
						"app.kubernetes.io/instance":    "agent",
						"app.kubernetes.io/managed-by":  "datadog-operator",
						"app.kubernetes.io/name":        "datadog-agent-deployment",
						"app.kubernetes.io/part-of":     "bar-foo",
						"app.kubernetes.io/version":     "",
					},
					Annotations: map[string]string{},
				},
				Spec: edsdatadoghqv1alpha1.ExtendedDaemonSetSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							GenerateName: "foo",
							Namespace:    "bar",
							Labels: map[string]string{
								"agent.datadoghq.com/name":      "foo",
								"agent.datadoghq.com/component": "agent",
								"app.kubernetes.io/instance":    "agent",
								"app.kubernetes.io/managed-by":  "datadog-operator",
								"app.kubernetes.io/name":        "datadog-agent-deployment",
								"app.kubernetes.io/part-of":     "bar-foo",
								"app.kubernetes.io/version":     "",
							},
							Annotations: make(map[string]string),
						},
						Spec: hostPortNetworkPodSpec,
					},
					Strategy: getDefaultEDSStrategy(),
				},
			},
		},
	}
	tests.Run(t)
}

func Test_newExtendedDaemonSetFromInstance_CustomConfigMaps(t *testing.T) {
	customConfdConfigMapName := "confd-configmap"
	customChecksdConfigMapName := "checksd-configmap"

	customConfigMapAgentDeployment := test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{
		UseEDS:              true,
		ClusterAgentEnabled: true,
		Confd: &datadoghqv1alpha1.ConfigDirSpec{
			ConfigMapName: customConfdConfigMapName,
		},
		Checksd: &datadoghqv1alpha1.ConfigDirSpec{
			ConfigMapName: customChecksdConfigMapName,
		},
	})

	customConfigMapsPodSpec := defaultPodSpec(customConfigMapAgentDeployment)
	customConfigMapsPodSpec.Volumes = []corev1.Volume{
		{
			Name: datadoghqv1alpha1.LogDatadogVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: apicommon.AuthVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: datadoghqv1alpha1.DogstatsdSocketVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: apicommon.InstallInfoVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "foo-install-info",
					},
				},
			},
		},
		{
			Name: apicommon.ConfdVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: customConfdConfigMapName,
					},
				},
			},
		},
		{
			Name: datadoghqv1alpha1.ChecksdVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: customChecksdConfigMapName,
					},
				},
			},
		},
		{
			Name: apicommon.ConfigVolumeName,
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
		{
			Name: "runtimesocketdir",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/run",
				},
			},
		},
	}

	test := extendedDaemonSetFromInstanceTest{
		name:            "with custom confd and checksd volume mounts",
		agentdeployment: customConfigMapAgentDeployment,
		wantErr:         false,
		want: &edsdatadoghqv1alpha1.ExtendedDaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "bar",
				Name:      "foo-agent",
				Labels: map[string]string{
					"agent.datadoghq.com/name":      "foo",
					"agent.datadoghq.com/component": "agent",
					"app.kubernetes.io/instance":    "agent",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "bar-foo",
					"app.kubernetes.io/version":     "",
				},
				Annotations: map[string]string{},
			},
			Spec: edsdatadoghqv1alpha1.ExtendedDaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "foo",
						Namespace:    "bar",
						Labels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "agent",
							"app.kubernetes.io/instance":    "agent",
							"app.kubernetes.io/managed-by":  "datadog-operator",
							"app.kubernetes.io/name":        "datadog-agent-deployment",
							"app.kubernetes.io/part-of":     "bar-foo",
							"app.kubernetes.io/version":     "",
						},
						Annotations: make(map[string]string),
					},
					Spec: customConfigMapsPodSpec,
				},
				Strategy: getDefaultEDSStrategy(),
			},
		},
	}

	test.Run(t)
}

func Test_newExtendedDaemonSetFromInstance_CustomDatadogYaml(t *testing.T) {
	customConfigMapCustomDatadogYaml := test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{UseEDS: true, ClusterAgentEnabled: true, APMEnabled: true, CustomConfig: "foo: bar\nbar: foo"})
	customConfigMapCustomDatadogYamlSpec := defaultPodSpec(customConfigMapCustomDatadogYaml)
	appendDefaultAPMAgentContainer(&customConfigMapCustomDatadogYamlSpec)

	customConfigMapCustomDatadogYamlSpec.Volumes = []corev1.Volume{
		{
			Name: datadoghqv1alpha1.LogDatadogVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: apicommon.AuthVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: datadoghqv1alpha1.DogstatsdSocketVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: apicommon.InstallInfoVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "foo-install-info",
					},
				},
			},
		},
		{
			Name: apicommon.ConfdVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: datadoghqv1alpha1.ChecksdVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: apicommon.ConfigVolumeName,
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
		{
			Name: datadoghqv1alpha1.AgentCustomConfigVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "foo-datadog-yaml",
					},
				},
			},
		},
		{
			Name: "runtimesocketdir",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/run",
				},
			},
		},
	}
	customConfigMapCustomDatadogYamlSpec.Containers[0].VolumeMounts = []corev1.VolumeMount{
		{
			Name:      "logdatadog",
			MountPath: "/var/log/datadog",
		},
		{
			Name:      "datadog-agent-auth",
			MountPath: "/etc/datadog-agent/auth",
		},
		{
			Name:      "dsdsocket",
			MountPath: "/var/run/datadog/statsd",
		},
		{
			Name:      "installinfo",
			SubPath:   "install_info",
			MountPath: "/etc/datadog-agent/install_info",
			ReadOnly:  true,
		},
		{
			Name:      "confd",
			MountPath: "/conf.d",
			ReadOnly:  true,
		},
		{
			Name:      "checksd",
			MountPath: "/checks.d",
			ReadOnly:  true,
		},
		{
			Name:      "procdir",
			MountPath: "/host/proc",
			ReadOnly:  true,
		},
		{
			Name:      "cgroups",
			MountPath: "/host/sys/fs/cgroup",
			ReadOnly:  true,
		},
		{
			Name:      "config",
			MountPath: "/etc/datadog-agent",
		},
		{
			Name:      "custom-datadog-yaml",
			MountPath: agentConfigFile,
			SubPath:   "datadog.yaml",
			ReadOnly:  true,
		},
		{
			Name:      "runtimesocketdir",
			MountPath: "/host/var/run",
			ReadOnly:  true,
		},
	}
	customConfigMapCustomDatadogYamlSpec.Containers[2].VolumeMounts = []corev1.VolumeMount{
		{
			Name:      "logdatadog",
			MountPath: "/var/log/datadog",
		},
		{
			Name:      "datadog-agent-auth",
			MountPath: "/etc/datadog-agent/auth",
			ReadOnly:  true,
		},
		{
			Name:      "dsdsocket",
			MountPath: "/var/run/datadog/statsd",
			ReadOnly:  true,
		},
		{
			Name:      "cgroups",
			MountPath: "/host/sys/fs/cgroup",
			ReadOnly:  true,
		},
		{
			Name:      "procdir",
			MountPath: "/host/proc",
			ReadOnly:  true,
		},
		{
			Name:      "config",
			MountPath: "/etc/datadog-agent",
		},
		{
			Name:      "custom-datadog-yaml",
			MountPath: agentConfigFile,
			SubPath:   "datadog.yaml",
			ReadOnly:  true,
		},
		{
			Name:      "runtimesocketdir",
			MountPath: "/host/var/run",
			ReadOnly:  true,
		},
	}
	customConfigMapCustomDatadogYamlSpec.InitContainers[1].VolumeMounts = customConfigMapCustomDatadogYamlSpec.Containers[0].VolumeMounts

	test := extendedDaemonSetFromInstanceTest{
		name:            "with custom config (datadog.yaml)",
		agentdeployment: customConfigMapCustomDatadogYaml,
		wantErr:         false,
		want: &edsdatadoghqv1alpha1.ExtendedDaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "bar",
				Name:      "foo-agent",
				Labels: map[string]string{
					"agent.datadoghq.com/name":      "foo",
					"agent.datadoghq.com/component": "agent",
					"app.kubernetes.io/instance":    "agent",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "bar-foo",
					"app.kubernetes.io/version":     "",
				},
				Annotations: map[string]string{},
			},
			Spec: edsdatadoghqv1alpha1.ExtendedDaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "foo",
						Namespace:    "bar",
						Labels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "agent",
							"app.kubernetes.io/instance":    "agent",
							"app.kubernetes.io/managed-by":  "datadog-operator",
							"app.kubernetes.io/name":        "datadog-agent-deployment",
							"app.kubernetes.io/part-of":     "bar-foo",
							"app.kubernetes.io/version":     "",
						},
						Annotations: make(map[string]string),
					},
					Spec: customConfigMapCustomDatadogYamlSpec,
				},
				Strategy: getDefaultEDSStrategy(),
			},
		},
	}
	test.Run(t)
}

func Test_ExtraParameters(t *testing.T) {
	site := "datadoghq.eu"
	options := &test.NewDatadogAgentOptions{
		UseEDS:                         true,
		ClusterAgentEnabled:            true,
		Annotations:                    map[string]string{"foo": "bar"},
		Labels:                         map[string]string{"bar": "foo"},
		AgentSpecAdditionalLabels:      map[string]string{"pod-foo": "bar"},
		AgentSpecAdditionalAnnotations: map[string]string{"pod-bar": "foo"},
		Site:                           site,
		ComplianceEnabled:              true,
		ComplianceCheckInterval:        metav1.Duration{Duration: time.Minute},
		OrchestratorExplorerDisabled:   true,
	}
	datadogAgent := test.NewDefaultedDatadogAgent("bar", "foo", options)

	extraEnvs := map[string]string{
		"DD_COMPLIANCE_CONFIG_CHECK_INTERVAL": "60000000000",
		"DD_SITE":                             site,
	}
	podSpec := complianceSecurityAgentPodSpec(extraEnvs)

	test := extendedDaemonSetFromInstanceTest{
		name:            "with custom config (datadog.yaml)",
		agentdeployment: datadogAgent,
		wantErr:         false,
		want: &edsdatadoghqv1alpha1.ExtendedDaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "bar",
				Name:      "foo-agent",
				Labels: map[string]string{
					"agent.datadoghq.com/name":      "foo",
					"agent.datadoghq.com/component": "agent",
					"app.kubernetes.io/instance":    "agent",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "bar-foo",
					"app.kubernetes.io/version":     "",
				},
				Annotations: map[string]string{},
			},
			Spec: edsdatadoghqv1alpha1.ExtendedDaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "foo",
						Namespace:    "bar",
						Labels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "agent",
							"app.kubernetes.io/instance":    "agent",
							"app.kubernetes.io/managed-by":  "datadog-operator",
							"app.kubernetes.io/name":        "datadog-agent-deployment",
							"app.kubernetes.io/part-of":     "bar-foo",
							"app.kubernetes.io/version":     "",
							"pod-foo":                       "bar",
						},
						Annotations: map[string]string{"pod-bar": "foo"},
					},
					Spec: podSpec,
				},
				Strategy: getDefaultEDSStrategy(),
			},
		},
	}
	test.Run(t)
}

func Test_newExtendedDaemonSetFromInstance_CustomVolumes(t *testing.T) {
	userVolumes := []corev1.Volume{
		{
			Name: "tmp",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/tmp",
				},
			},
		},
	}
	userVolumeMounts := []corev1.VolumeMount{
		{
			Name:      "tmp",
			MountPath: "/some/path",
			ReadOnly:  true,
		},
	}

	userMountsAgentDeployment := test.NewDefaultedDatadogAgent("bar", "foo",
		&test.NewDatadogAgentOptions{
			UseEDS:              true,
			ClusterAgentEnabled: true,
			Volumes:             userVolumes,
			VolumeMounts:        userVolumeMounts,
		})

	userMountsPodSpec := defaultPodSpec(userMountsAgentDeployment)
	userMountsPodSpec.Volumes = append(userMountsPodSpec.Volumes, userVolumes...)
	userMountsPodSpec.Containers[0].VolumeMounts = append(userMountsPodSpec.Containers[0].VolumeMounts, userVolumeMounts...) // core agent
	userMountsPodSpec.Containers[1].VolumeMounts = append(userMountsPodSpec.Containers[1].VolumeMounts, userVolumeMounts...) // proces agent
	userMountsPodSpec.InitContainers[1].VolumeMounts = append(userMountsPodSpec.InitContainers[1].VolumeMounts, userVolumeMounts...)

	test := extendedDaemonSetFromInstanceTest{
		name:            "with user volumes and mounts",
		agentdeployment: userMountsAgentDeployment,
		wantErr:         false,
		want: &edsdatadoghqv1alpha1.ExtendedDaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "bar",
				Name:      "foo-agent",
				Labels: map[string]string{
					"agent.datadoghq.com/name":      "foo",
					"agent.datadoghq.com/component": "agent",
					"app.kubernetes.io/instance":    "agent",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "bar-foo",
					"app.kubernetes.io/version":     "",
				},
				Annotations: map[string]string{},
			},
			Spec: edsdatadoghqv1alpha1.ExtendedDaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "foo",
						Namespace:    "bar",
						Labels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "agent",
							"app.kubernetes.io/instance":    "agent",
							"app.kubernetes.io/managed-by":  "datadog-operator",
							"app.kubernetes.io/name":        "datadog-agent-deployment",
							"app.kubernetes.io/part-of":     "bar-foo",
							"app.kubernetes.io/version":     "",
						},
						Annotations: map[string]string{},
					},
					Spec: userMountsPodSpec,
				},
				Strategy: getDefaultEDSStrategy(),
			},
		},
	}
	test.Run(t)
}

func Test_newExtendedDaemonSetFromInstance_DaemonSetNameAndSelector(t *testing.T) {
	daemonsetNameAgentDeployment := test.NewDefaultedDatadogAgent("bar", "foo",
		&test.NewDatadogAgentOptions{
			UseEDS:              true,
			ClusterAgentEnabled: true,
			AgentDaemonsetName:  "custom-agent-daemonset",
		})

	test := extendedDaemonSetFromInstanceTest{
		name:            "with user daemonset name and selector",
		agentdeployment: daemonsetNameAgentDeployment,
		selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"app": "datadog-monitoring",
			},
		},
		wantErr: false,
		want: &edsdatadoghqv1alpha1.ExtendedDaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "bar",
				Name:      "custom-agent-daemonset",
				Labels: map[string]string{
					"agent.datadoghq.com/name":      "foo",
					"agent.datadoghq.com/component": "agent",
					"app.kubernetes.io/instance":    "agent",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "bar-foo",
					"app.kubernetes.io/version":     "",
				},
				Annotations: map[string]string{},
			},
			Spec: edsdatadoghqv1alpha1.ExtendedDaemonSetSpec{
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "datadog-monitoring",
					},
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "foo",
						Namespace:    "bar",
						Labels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "agent",
							"app.kubernetes.io/instance":    "agent",
							"app.kubernetes.io/managed-by":  "datadog-operator",
							"app.kubernetes.io/name":        "datadog-agent-deployment",
							"app.kubernetes.io/part-of":     "bar-foo",
							"app.kubernetes.io/version":     "",
							"app":                           "datadog-monitoring",
						},
						Annotations: map[string]string{},
					},
					Spec: defaultPodSpec(daemonsetNameAgentDeployment),
				},
				Strategy: getDefaultEDSStrategy(),
			},
		},
	}
	test.Run(t)
}

func Test_newExtendedDaemonSetFromInstance_LogsEnabled(t *testing.T) {
	dda := test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{
		UseEDS:              true,
		ClusterAgentEnabled: true,
		Features: &datadoghqv1alpha1.DatadogFeatures{
			LogCollection: &datadoghqv1alpha1.LogCollectionConfig{
				Enabled: apiutils.NewBoolPointer(true),
			},
		},
	})
	logsEnabledPodSpec := defaultPodSpec(dda)
	logsVolumes := []corev1.Volume{
		{
			Name: "pointerdir",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/lib/datadog-agent/logs",
				},
			},
		},
		{
			Name: "logpodpath",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/log/pods",
				},
			},
		},
		{
			Name: "logcontainerpath",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/lib/docker/containers",
				},
			},
		},
		{
			Name: "symlinkcontainerpath",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/log/containers",
				},
			},
		},
	}
	logsVolumeMounts := []corev1.VolumeMount{
		{
			Name:      "pointerdir",
			MountPath: "/opt/datadog-agent/run",
		},
		{
			Name:      "logpodpath",
			MountPath: "/var/log/pods",
			ReadOnly:  true,
		},
		{
			Name:      "logcontainerpath",
			MountPath: "/var/lib/docker/containers",
			ReadOnly:  true,
		},
		{
			Name:      "symlinkcontainerpath",
			MountPath: "/var/log/containers",
			ReadOnly:  true,
		},
	}

	logsEnabledPodSpec.Volumes = append(logsEnabledPodSpec.Volumes, logsVolumes...)
	logsEnabledPodSpec.Containers[0].VolumeMounts = append(logsEnabledPodSpec.Containers[0].VolumeMounts, logsVolumeMounts...)
	logsEnabledPodSpec.Containers[0].Env = addEnvVar(logsEnabledPodSpec.Containers[0].Env, "DD_LOGS_ENABLED", "true")
	logsEnabledPodSpec.Containers[0].Env = addEnvVar(logsEnabledPodSpec.Containers[0].Env, "DD_LOGS_CONFIG_K8S_CONTAINER_USE_FILE", "true")
	logsEnabledPodSpec.InitContainers[1].VolumeMounts = append(logsEnabledPodSpec.InitContainers[1].VolumeMounts, logsVolumeMounts...)
	logsEnabledPodSpec.InitContainers[1].Env = addEnvVar(logsEnabledPodSpec.InitContainers[1].Env, "DD_LOGS_ENABLED", "true")
	logsEnabledPodSpec.InitContainers[1].Env = addEnvVar(logsEnabledPodSpec.Containers[0].Env, "DD_LOGS_CONFIG_K8S_CONTAINER_USE_FILE", "true")

	test := extendedDaemonSetFromInstanceTest{
		name:            "with logs enabled",
		agentdeployment: dda,
		wantErr:         false,
		want: &edsdatadoghqv1alpha1.ExtendedDaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "bar",
				Name:      "foo-agent",
				Labels: map[string]string{
					"agent.datadoghq.com/name":      "foo",
					"agent.datadoghq.com/component": "agent",
					"app.kubernetes.io/instance":    "agent",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "bar-foo",
					"app.kubernetes.io/version":     "",
				},
				Annotations: map[string]string{},
			},
			Spec: edsdatadoghqv1alpha1.ExtendedDaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "foo",
						Namespace:    "bar",
						Labels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "agent",
							"app.kubernetes.io/instance":    "agent",
							"app.kubernetes.io/managed-by":  "datadog-operator",
							"app.kubernetes.io/name":        "datadog-agent-deployment",
							"app.kubernetes.io/part-of":     "bar-foo",
							"app.kubernetes.io/version":     "",
						},
						Annotations: make(map[string]string),
					},
					Spec: logsEnabledPodSpec,
				},
				Strategy: getDefaultEDSStrategy(),
			},
		},
	}

	test.Run(t)
}

func Test_newExtendedDaemonSetFromInstance_clusterChecksConfig(t *testing.T) {
	dda := test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{
		UseEDS:               true,
		ClusterAgentEnabled:  true,
		ClusterChecksEnabled: true,
	})

	clusterChecksPodSpec := defaultPodSpec(dda)
	clusterChecksPodSpec.Containers[0].Env = addEnvVar(clusterChecksPodSpec.Containers[0].Env, "DD_EXTRA_CONFIG_PROVIDERS", "clusterchecks endpointschecks")
	clusterChecksPodSpec.InitContainers[1].Env = addEnvVar(clusterChecksPodSpec.InitContainers[1].Env, "DD_EXTRA_CONFIG_PROVIDERS", "clusterchecks endpointschecks")

	apiutils.BoolValue(dda.Spec.ClusterChecksRunner.Enabled)

	test := extendedDaemonSetFromInstanceTest{
		name:            "with cluster checks / clc runners",
		agentdeployment: dda,
		wantErr:         false,
		want: &edsdatadoghqv1alpha1.ExtendedDaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "bar",
				Name:      "foo-agent",
				Labels: map[string]string{
					"agent.datadoghq.com/name":      "foo",
					"agent.datadoghq.com/component": "agent",
					"app.kubernetes.io/instance":    "agent",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "bar-foo",
					"app.kubernetes.io/version":     "",
				},
				Annotations: map[string]string{},
			},
			Spec: edsdatadoghqv1alpha1.ExtendedDaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "foo",
						Namespace:    "bar",
						Labels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "agent",
							"app.kubernetes.io/instance":    "agent",
							"app.kubernetes.io/managed-by":  "datadog-operator",
							"app.kubernetes.io/name":        "datadog-agent-deployment",
							"app.kubernetes.io/part-of":     "bar-foo",
							"app.kubernetes.io/version":     "",
						},
						Annotations: make(map[string]string),
					},
					Spec: clusterChecksPodSpec,
				},
				Strategy: getDefaultEDSStrategy(),
			},
		},
	}

	test.Run(t)
}

func Test_newExtendedDaemonSetFromInstance_endpointsChecksConfig(t *testing.T) {
	dda := test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{
		UseEDS:                     true,
		ClusterAgentEnabled:        true,
		ClusterChecksEnabled:       true,
		ClusterChecksRunnerEnabled: true,
	})
	endpointChecksChecksPodSpec := defaultPodSpec(dda)
	endpointChecksChecksPodSpec.Containers[0].Env = addEnvVar(endpointChecksChecksPodSpec.Containers[0].Env, "DD_EXTRA_CONFIG_PROVIDERS", "endpointschecks")
	endpointChecksChecksPodSpec.InitContainers[1].Env = addEnvVar(endpointChecksChecksPodSpec.InitContainers[1].Env, "DD_EXTRA_CONFIG_PROVIDERS", "endpointschecks")

	test := extendedDaemonSetFromInstanceTest{
		name:            "with cluster checks / with clc runners",
		agentdeployment: dda,
		wantErr:         false,
		want: &edsdatadoghqv1alpha1.ExtendedDaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "bar",
				Name:      "foo-agent",
				Labels: map[string]string{
					"agent.datadoghq.com/name":      "foo",
					"agent.datadoghq.com/component": "agent",
					"app.kubernetes.io/instance":    "agent",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "bar-foo",
					"app.kubernetes.io/version":     "",
				},
				Annotations: map[string]string{},
			},
			Spec: edsdatadoghqv1alpha1.ExtendedDaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "foo",
						Namespace:    "bar",
						Labels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "agent",
							"app.kubernetes.io/instance":    "agent",
							"app.kubernetes.io/managed-by":  "datadog-operator",
							"app.kubernetes.io/name":        "datadog-agent-deployment",
							"app.kubernetes.io/part-of":     "bar-foo",
							"app.kubernetes.io/version":     "",
						},
						Annotations: make(map[string]string),
					},
					Spec: endpointChecksChecksPodSpec,
				},
				Strategy: getDefaultEDSStrategy(),
			},
		},
	}

	test.Run(t)
}

func extendedDaemonSetWithSystemProbe(podSpec corev1.PodSpec) *edsdatadoghqv1alpha1.ExtendedDaemonSet {
	return &edsdatadoghqv1alpha1.ExtendedDaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "bar",
			Name:      "foo-agent",
			Labels: map[string]string{
				"agent.datadoghq.com/name":      "foo",
				"agent.datadoghq.com/component": "agent",
				"app.kubernetes.io/instance":    "agent",
				"app.kubernetes.io/managed-by":  "datadog-operator",
				"app.kubernetes.io/name":        "datadog-agent-deployment",
				"app.kubernetes.io/part-of":     "bar-foo",
				"app.kubernetes.io/version":     "",
			},
			Annotations: map[string]string{},
		},
		Spec: edsdatadoghqv1alpha1.ExtendedDaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "foo",
					Namespace:    "bar",
					Labels: map[string]string{
						"agent.datadoghq.com/name":      "foo",
						"agent.datadoghq.com/component": "agent",
						"app.kubernetes.io/instance":    "agent",
						"app.kubernetes.io/managed-by":  "datadog-operator",
						"app.kubernetes.io/name":        "datadog-agent-deployment",
						"app.kubernetes.io/part-of":     "bar-foo",
						"app.kubernetes.io/version":     "",
					},
					Annotations: map[string]string{
						"container.apparmor.security.beta.kubernetes.io/system-probe": "unconfined",
						"container.seccomp.security.alpha.kubernetes.io/system-probe": "localhost/system-probe",
					},
				},
				Spec: podSpec,
			},
			Strategy: getDefaultEDSStrategy(),
		},
	}
}

func extendedDaemonSetDefault(podSpec corev1.PodSpec) *edsdatadoghqv1alpha1.ExtendedDaemonSet {
	return &edsdatadoghqv1alpha1.ExtendedDaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "bar",
			Name:      "foo-agent",
			Labels: map[string]string{
				"agent.datadoghq.com/name":      "foo",
				"agent.datadoghq.com/component": "agent",
				"app.kubernetes.io/instance":    "agent",
				"app.kubernetes.io/managed-by":  "datadog-operator",
				"app.kubernetes.io/name":        "datadog-agent-deployment",
				"app.kubernetes.io/part-of":     "bar-foo",
				"app.kubernetes.io/version":     "",
			},
			Annotations: map[string]string{},
		},
		Spec: edsdatadoghqv1alpha1.ExtendedDaemonSetSpec{
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "foo",
					Namespace:    "bar",
					Labels: map[string]string{
						"agent.datadoghq.com/name":      "foo",
						"agent.datadoghq.com/component": "agent",
						"app.kubernetes.io/instance":    "agent",
						"app.kubernetes.io/managed-by":  "datadog-operator",
						"app.kubernetes.io/name":        "datadog-agent-deployment",
						"app.kubernetes.io/part-of":     "bar-foo",
						"app.kubernetes.io/version":     "",
					},
					Annotations: map[string]string{},
				},

				Spec: podSpec,
			},
			Strategy: getDefaultEDSStrategy(),
		},
	}
}

func Test_newExtendedDaemonSetFromInstance_SystemProbe(t *testing.T) {
	dda := test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{
		UseEDS:                       true,
		ClusterAgentEnabled:          true,
		SystemProbeEnabled:           true,
		OrchestratorExplorerDisabled: true,
	})

	systemProbePodSpec := defaultSystemProbePodSpec(dda)
	systemProbeExtraMountsSpec := systemProbePodSpec.DeepCopy()
	systemProbeExtraMountsSpec.Volumes = append(systemProbeExtraMountsSpec.Volumes, []corev1.Volume{
		{
			Name: "modules",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/lib/modules",
				},
			},
		},
		{
			Name: "src",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/usr/src",
				},
			},
		},
	}...)
	for idx := range systemProbeExtraMountsSpec.Containers {
		if systemProbeExtraMountsSpec.Containers[idx].Name == "system-probe" {
			systemProbeExtraMountsSpec.Containers[idx].VolumeMounts = append(systemProbeExtraMountsSpec.Containers[idx].VolumeMounts, []corev1.VolumeMount{
				{
					Name:      "modules",
					MountPath: "/lib/modules",
					ReadOnly:  true,
				},
				{
					Name:      "src",
					MountPath: "/usr/src",
					ReadOnly:  true,
				},
			}...)
			break
		}
	}

	oomKillSpec := systemProbeExtraMountsSpec.DeepCopy()
	addEnvVar(oomKillSpec.Containers[0].Env, datadoghqv1alpha1.DDSystemProbeOOMKillEnabled, "true")
	addEnvVar(oomKillSpec.Containers[1].Env, datadoghqv1alpha1.DDSystemProbeOOMKillEnabled, "true")
	addEnvVar(oomKillSpec.InitContainers[1].Env, datadoghqv1alpha1.DDSystemProbeOOMKillEnabled, "true")

	tpcQueueLengthSpec := systemProbeExtraMountsSpec.DeepCopy()
	addEnvVar(tpcQueueLengthSpec.Containers[0].Env, datadoghqv1alpha1.DDSystemProbeTCPQueueLengthEnabled, "true")
	addEnvVar(tpcQueueLengthSpec.Containers[1].Env, datadoghqv1alpha1.DDSystemProbeTCPQueueLengthEnabled, "true")
	addEnvVar(tpcQueueLengthSpec.InitContainers[1].Env, datadoghqv1alpha1.DDSystemProbeTCPQueueLengthEnabled, "true")

	ddaOOMKill := test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{
		UseEDS:                       true,
		ClusterAgentEnabled:          true,
		SystemProbeEnabled:           true,
		OrchestratorExplorerDisabled: true,
		SystemProbeOOMKillEnabled:    true,
	})

	ddaTCPQueueLength := test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{
		UseEDS:                           true,
		ClusterAgentEnabled:              true,
		SystemProbeEnabled:               true,
		SystemProbeTCPQueueLengthEnabled: true,
		OrchestratorExplorerDisabled:     true,
	})

	ddaSeccomp := test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{
		UseEDS:                        true,
		ClusterAgentEnabled:           true,
		SystemProbeEnabled:            true,
		OrchestratorExplorerDisabled:  true,
		SystemProbeSeccompProfileName: "host-profile",
	})

	edsSeccomp := extendedDaemonSetWithSystemProbe(noSeccompInstallSystemProbeSpec(ddaSeccomp))
	edsSeccomp.Spec.Template.Annotations["container.seccomp.security.alpha.kubernetes.io/system-probe"] = "host-profile"

	ddaCustomConfigMap := test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{
		UseEDS:                         true,
		ClusterAgentEnabled:            true,
		SystemProbeEnabled:             true,
		OrchestratorExplorerDisabled:   true,
		SystemProbeCustomConfigMapName: "custom-cm",
	})
	customConfigMapSpec := systemProbePodSpec.DeepCopy()
	for i := range customConfigMapSpec.Volumes {
		if customConfigMapSpec.Volumes[i].Name == datadoghqv1alpha1.SystemProbeConfigVolumeName {
			customConfigMapSpec.Volumes[i].ConfigMap.Name = "custom-cm"
		}
	}

	tests := []extendedDaemonSetFromInstanceTest{
		{
			name:            "with default settings",
			agentdeployment: dda,
			wantErr:         false,
			want:            extendedDaemonSetWithSystemProbe(systemProbePodSpec),
		},
		{
			name:            "with oom kill",
			agentdeployment: ddaOOMKill,
			wantErr:         false,
			want:            extendedDaemonSetWithSystemProbe(*oomKillSpec),
		},
		{
			name:            "with tcp queue length",
			agentdeployment: ddaTCPQueueLength,
			wantErr:         false,
			want:            extendedDaemonSetWithSystemProbe(*tpcQueueLengthSpec),
		},
		{
			name:            "with on-host seccomp profile",
			agentdeployment: ddaSeccomp,
			wantErr:         false,
			want:            edsSeccomp,
		},
		{
			name:            "with custom configmap",
			agentdeployment: ddaCustomConfigMap,
			wantErr:         false,
			want:            extendedDaemonSetWithSystemProbe(*customConfigMapSpec),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.Run(t)
		})
	}
}

func Test_newExtendedDaemonSetFromInstance_Orchestrator(t *testing.T) {
	dda := test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{
		UseEDS:              true,
		ClusterAgentEnabled: true,
	})

	orchestratorPodSpec := defaultPodSpec(dda)

	tests := []extendedDaemonSetFromInstanceTest{
		{
			name:            "with default settings",
			agentdeployment: dda,
			wantErr:         false,
			want:            extendedDaemonSetDefault(orchestratorPodSpec),
		},
	}

	for _, instanceTest := range tests {
		t.Run(instanceTest.name, func(t *testing.T) {
			instanceTest.Run(t)
		})
	}
}

func Test_newExtendedDaemonSetFromInstance_PrometheusScrape(t *testing.T) {
	logger := logf.Log.WithName(t.Name())

	dda := test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{
		UseEDS:              true,
		ClusterAgentEnabled: true,
		Features: &datadoghqv1alpha1.DatadogFeatures{
			OrchestratorExplorer: &datadoghqv1alpha1.OrchestratorExplorerConfig{Enabled: apiutils.NewBoolPointer(true)},
			PrometheusScrape: &datadoghqv1alpha1.PrometheusScrapeConfig{
				Enabled:          apiutils.NewBoolPointer(true),
				ServiceEndpoints: apiutils.NewBoolPointer(true),
			},
		},
	})

	promEnabledPodSpec := defaultPodSpec(dda)
	promEnabledPodSpec.Containers[0].Env = append(promEnabledPodSpec.Containers[0].Env, prometheusScrapeEnvVars(logger, dda)...)
	promEnabledPodSpec.InitContainers[1].Env = append(promEnabledPodSpec.InitContainers[1].Env, prometheusScrapeEnvVars(logger, dda)...)

	additionalConfig := `- configurations:
  - timeout: 5
    send_distribution_buckets: true
  autodiscovery:
    kubernetes_annotations:
      include:
        custom_label: true`
	ddaWithAdditionalConf := test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{
		UseEDS:              true,
		ClusterAgentEnabled: true,
		Features: &datadoghqv1alpha1.DatadogFeatures{
			OrchestratorExplorer: &datadoghqv1alpha1.OrchestratorExplorerConfig{Enabled: apiutils.NewBoolPointer(true)},
			PrometheusScrape: &datadoghqv1alpha1.PrometheusScrapeConfig{
				Enabled:           apiutils.NewBoolPointer(true),
				ServiceEndpoints:  apiutils.NewBoolPointer(false),
				AdditionalConfigs: &additionalConfig,
			},
		},
	})

	promAdditionalConfPodSpec := defaultPodSpec(ddaWithAdditionalConf)
	promAdditionalConfPodSpec.Containers[0].Env = append(promAdditionalConfPodSpec.Containers[0].Env, prometheusScrapeEnvVars(logger, ddaWithAdditionalConf)...)
	promAdditionalConfPodSpec.InitContainers[1].Env = append(promAdditionalConfPodSpec.InitContainers[1].Env, prometheusScrapeEnvVars(logger, ddaWithAdditionalConf)...)

	tests := []extendedDaemonSetFromInstanceTest{
		{
			name:            "Prometheus scrape enabled",
			agentdeployment: dda,
			wantErr:         false,
			want:            extendedDaemonSetDefault(promEnabledPodSpec),
		},
		{
			name:            "With additional config",
			agentdeployment: ddaWithAdditionalConf,
			wantErr:         false,
			want:            extendedDaemonSetDefault(promAdditionalConfPodSpec),
		},
	}

	for _, instanceTest := range tests {
		t.Run(instanceTest.name, func(t *testing.T) {
			instanceTest.Run(t)
		})
	}
}

func Test_newExtendedDaemonSetFromInstance_SecurityAgent_Compliance(t *testing.T) {
	securityAgentPodSpec := complianceSecurityAgentPodSpec(nil)

	dda := test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{
		UseEDS:                       true,
		ClusterAgentEnabled:          true,
		ComplianceEnabled:            true,
		RuntimeSyscallMonitorEnabled: true,
		OrchestratorExplorerDisabled: true,
	})

	test := extendedDaemonSetFromInstanceTest{
		name:            "with compliance agent enabled",
		agentdeployment: dda,
		wantErr:         false,
		want: &edsdatadoghqv1alpha1.ExtendedDaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "bar",
				Name:      "foo-agent",
				Labels: map[string]string{
					"agent.datadoghq.com/name":      "foo",
					"agent.datadoghq.com/component": "agent",
					"app.kubernetes.io/instance":    "agent",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "bar-foo",
					"app.kubernetes.io/version":     "",
				},
				Annotations: map[string]string{},
			},
			Spec: edsdatadoghqv1alpha1.ExtendedDaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "foo",
						Namespace:    "bar",
						Labels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "agent",
							"app.kubernetes.io/instance":    "agent",
							"app.kubernetes.io/managed-by":  "datadog-operator",
							"app.kubernetes.io/name":        "datadog-agent-deployment",
							"app.kubernetes.io/part-of":     "bar-foo",
							"app.kubernetes.io/version":     "",
						},
						Annotations: map[string]string{},
					},
					Spec: securityAgentPodSpec,
				},
				Strategy: getDefaultEDSStrategy(),
			},
		},
	}

	test.Run(t)
}

func Test_newExtendedDaemonSetFromInstance_SecurityAgent_Runtime(t *testing.T) {
	securityAgentPodSpec := runtimeSecurityAgentPodSpec(nil, "test-runtime-policies")
	securityAgentPodSpec.Containers[2].Env = addEnvVar(securityAgentPodSpec.Containers[2].Env, "DD_RUNTIME_SECURITY_CONFIG_POLICIES_DIR", "/etc/datadog-agent/runtime-security.d")
	securityAgentPodSpec.Containers[0].VolumeMounts = append(securityAgentPodSpec.Containers[0].VolumeMounts, []corev1.VolumeMount{
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
	securityAgentPodSpec.Containers[1].VolumeMounts = append(securityAgentPodSpec.Containers[1].VolumeMounts, []corev1.VolumeMount{
		{
			Name:      datadoghqv1alpha1.SecurityAgentRuntimePoliciesDirVolumeName,
			MountPath: "/etc/datadog-agent/runtime-security.d",
			ReadOnly:  true,
		},
	}...)
	securityAgentPodSpec.Containers[2].VolumeMounts = append(securityAgentPodSpec.Containers[2].VolumeMounts, []corev1.VolumeMount{
		{
			Name:      datadoghqv1alpha1.SecurityAgentRuntimePoliciesDirVolumeName,
			MountPath: "/etc/datadog-agent/runtime-security.d",
			ReadOnly:  true,
		},
	}...)

	securityAgentPodSpec.InitContainers[1].VolumeMounts = append(securityAgentPodSpec.Containers[0].VolumeMounts, []corev1.VolumeMount{
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
	dda := test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{
		UseEDS:                       true,
		ClusterAgentEnabled:          true,
		RuntimeSecurityEnabled:       true,
		RuntimeSyscallMonitorEnabled: true,
		RuntimePoliciesDir: &datadoghqv1alpha1.ConfigDirSpec{
			ConfigMapName: "test-runtime-policies",
		},
		SystemProbeEnabled:           true,
		OrchestratorExplorerDisabled: true,
	})

	test := extendedDaemonSetFromInstanceTest{
		name:            "with runtime security agent enabled",
		agentdeployment: dda,
		wantErr:         false,
		want: &edsdatadoghqv1alpha1.ExtendedDaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "bar",
				Name:      "foo-agent",
				Labels: map[string]string{
					"agent.datadoghq.com/name":      "foo",
					"agent.datadoghq.com/component": "agent",
					"app.kubernetes.io/instance":    "agent",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "bar-foo",
					"app.kubernetes.io/version":     "",
				},
				Annotations: map[string]string{},
			},
			Spec: edsdatadoghqv1alpha1.ExtendedDaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "foo",
						Namespace:    "bar",
						Labels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "agent",
							"app.kubernetes.io/instance":    "agent",
							"app.kubernetes.io/managed-by":  "datadog-operator",
							"app.kubernetes.io/name":        "datadog-agent-deployment",
							"app.kubernetes.io/part-of":     "bar-foo",
							"app.kubernetes.io/version":     "",
						},
						Annotations: map[string]string{
							"container.apparmor.security.beta.kubernetes.io/system-probe": "unconfined",
							"container.seccomp.security.alpha.kubernetes.io/system-probe": "localhost/system-probe",
						},
					},
					Spec: securityAgentPodSpec,
				},
				Strategy: getDefaultEDSStrategy(),
			},
		},
	}

	test.Run(t)
}

func Test_newExtendedDaemonSetFromInstance_KubeletConfiguration(t *testing.T) {
	dda := test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{
		UseEDS:                       true,
		ClusterAgentEnabled:          true,
		OrchestratorExplorerDisabled: true,
	})

	dda.Spec.Agent.Config.Kubelet = &commonv1.KubeletConfig{
		Host: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: FieldPathSpecNodeName,
			},
		},
		TLSVerify:   apiutils.NewBoolPointer(false),
		HostCAPath:  "/foo/bar/kubeletca.crt",
		AgentCAPath: "/agent/foo/bar/ca.crt",
	}

	test := extendedDaemonSetFromInstanceTest{
		name:            "with custom Kubelet config",
		agentdeployment: dda,
		wantErr:         false,
		want: &edsdatadoghqv1alpha1.ExtendedDaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "bar",
				Name:      "foo-agent",
				Labels: map[string]string{
					"agent.datadoghq.com/name":      "foo",
					"agent.datadoghq.com/component": "agent",
					"app.kubernetes.io/instance":    "agent",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "bar-foo",
					"app.kubernetes.io/version":     "",
				},
				Annotations: map[string]string{},
			},
			Spec: edsdatadoghqv1alpha1.ExtendedDaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "foo",
						Namespace:    "bar",
						Labels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "agent",
							"app.kubernetes.io/instance":    "agent",
							"app.kubernetes.io/managed-by":  "datadog-operator",
							"app.kubernetes.io/name":        "datadog-agent-deployment",
							"app.kubernetes.io/part-of":     "bar-foo",
							"app.kubernetes.io/version":     "",
						},
						Annotations: map[string]string{},
					},
					Spec: customKubeletConfigPodSpec(dda.Spec.Agent.Config.Kubelet),
				},
				Strategy: getDefaultEDSStrategy(),
			},
		},
	}

	test.Run(t)
}

func Test_newExtendedDaemonSetFromInstance_ArgsCommandOverride(t *testing.T) {
	dda := test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{
		UseEDS:              true,
		ProcessEnabled:      true,
		ClusterAgentEnabled: true,
	})

	dda.Spec.Agent.Config.Command = []string{"my-custom-agent"}
	dda.Spec.Agent.Config.Args = []string{"my-custom-args"}
	dda.Spec.Agent.Process.Args = []string{"my-extra-args"}

	wantSpec := defaultPodSpec(dda)
	wantSpec.Containers[0].Command = []string{"my-custom-agent"}
	wantSpec.Containers[0].Args = []string{"my-custom-args"}
	wantSpec.Containers[1].Args = []string{"my-extra-args"}

	test := extendedDaemonSetFromInstanceTest{
		name:            "with custom Command/Args",
		agentdeployment: dda,
		wantErr:         false,
		want: &edsdatadoghqv1alpha1.ExtendedDaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "bar",
				Name:      "foo-agent",
				Labels: map[string]string{
					"agent.datadoghq.com/name":      "foo",
					"agent.datadoghq.com/component": "agent",
					"app.kubernetes.io/instance":    "agent",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "bar-foo",
					"app.kubernetes.io/version":     "",
				},
				Annotations: map[string]string{},
			},
			Spec: edsdatadoghqv1alpha1.ExtendedDaemonSetSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						GenerateName: "foo",
						Namespace:    "bar",
						Labels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "agent",
							"app.kubernetes.io/instance":    "agent",
							"app.kubernetes.io/managed-by":  "datadog-operator",
							"app.kubernetes.io/name":        "datadog-agent-deployment",
							"app.kubernetes.io/part-of":     "bar-foo",
							"app.kubernetes.io/version":     "",
						},
						Annotations: map[string]string{},
					},
					Spec: wantSpec,
				},
				Strategy: getDefaultEDSStrategy(),
			},
		},
	}

	test.Run(t)
}

func getDefaultEDSStrategy() edsdatadoghqv1alpha1.ExtendedDaemonSetSpecStrategy {
	var defaultMaxParallelPodCreation int32 = 250
	return edsdatadoghqv1alpha1.ExtendedDaemonSetSpecStrategy{
		Canary: &edsdatadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanary{
			Replicas: &intstr.IntOrString{
				IntVal: 1,
			},
			Duration: &metav1.Duration{
				Duration: 10 * time.Minute,
			},
			NoRestartsDuration: &metav1.Duration{
				Duration: 5 * time.Minute,
			},
			NodeSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{},
			},
			AutoPause:      edsdatadoghqv1alpha1.DefaultExtendedDaemonSetSpecStrategyCanaryAutoPause(&edsdatadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoPause{}),
			AutoFail:       edsdatadoghqv1alpha1.DefaultExtendedDaemonSetSpecStrategyCanaryAutoFail(&edsdatadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanaryAutoFail{}),
			ValidationMode: edsdatadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanaryValidationModeAuto,
		},
		ReconcileFrequency: &metav1.Duration{
			Duration: 10 * time.Second,
		},
		RollingUpdate: edsdatadoghqv1alpha1.ExtendedDaemonSetSpecStrategyRollingUpdate{
			MaxUnavailable: &intstr.IntOrString{
				Type:   intstr.String,
				StrVal: "10%",
			},
			MaxPodSchedulerFailure: &intstr.IntOrString{
				Type:   intstr.String,
				StrVal: "10%",
			},
			MaxParallelPodCreation: &defaultMaxParallelPodCreation,
			SlowStartIntervalDuration: &metav1.Duration{
				Duration: 1 * time.Minute,
			},
			SlowStartAdditiveIncrease: &intstr.IntOrString{
				Type:   intstr.String,
				StrVal: "5",
			},
		},
	}
}
