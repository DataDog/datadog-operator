// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagent

import (
	"fmt"
	"testing"
	"time"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	test "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1/test"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	edsdatadoghqv1alpha1 "github.com/datadog/extendeddaemonset/pkg/apis/datadoghq/v1alpha1"
	"github.com/google/go-cmp/cmp"
	assert "github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

const testDadName = "foo"

func authTokenValue() *corev1.EnvVarSource {
	return &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: fmt.Sprintf("%s-%s", testDadName, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix),
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
		Handler: corev1.Handler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/health",
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
			Name: datadoghqv1alpha1.ConfdVolumeName,
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
			Name: datadoghqv1alpha1.ConfigVolumeName,
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
			Name: "runtimesocket",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/run/docker.sock",
				},
			},
		},
	}
}

func defaultMountVolume() []corev1.VolumeMount {
	return []corev1.VolumeMount{
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
			Name:      "config",
			MountPath: "/etc/datadog-agent",
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
			Name:      "runtimesocket",
			MountPath: "/var/run/docker.sock",
			ReadOnly:  true,
		},
	}
}

func defaultEnvVars() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "DD_CLUSTER_NAME",
			Value: "",
		},
		{
			Name:  "DD_SITE",
			Value: "",
		},
		{
			Name:  "DD_DD_URL",
			Value: "https://app.datadoghq.com",
		},
		{
			Name:  "DD_HEALTH_PORT",
			Value: "5555",
		},
		{
			Name:  "DD_KUBERNETES_POD_LABELS_AS_TAGS",
			Value: "{}",
		},
		{
			Name:  "DD_KUBERNETES_POD_ANNOTATIONS_AS_TAGS",
			Value: "{}",
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
			Name:  "DD_LOGS_ENABLED",
			Value: "false",
		},
		{
			Name:  "DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL",
			Value: "false",
		},
		{
			Name:  "DD_DOGSTATSD_ORIGIN_DETECTION",
			Value: "false",
		},
		{
			Name:  "DD_LOG_LEVEL",
			Value: "INFO",
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
			Name: datadoghqv1alpha1.DDHostname,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: FieldPathSpecNodeName,
				},
			},
		},
		{
			Name:  "KUBERNETES",
			Value: "yes",
		},
		{
			Name:  "DD_API_KEY",
			Value: "",
		},
		{
			Name:  "DD_CLUSTER_AGENT_ENABLED",
			Value: "true",
		},
		{
			Name:  "DD_CLUSTER_AGENT_KUBERNETES_SERVICE_NAME",
			Value: fmt.Sprintf("%s-%s", testDadName, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix),
		},
		{
			Name:      "DD_CLUSTER_AGENT_AUTH_TOKEN",
			ValueFrom: authTokenValue(),
		},
		{
			Name:  "DD_CRI_SOCKET_PATH",
			Value: "/var/run/docker.sock",
		},
	}
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

func defaultPodSpec() corev1.PodSpec {
	return corev1.PodSpec{
		ServiceAccountName: "foo-agent",
		InitContainers: []corev1.Container{
			{
				Name:            "init-volume",
				Image:           "datadog/agent:latest",
				ImagePullPolicy: corev1.PullIfNotPresent,
				Resources:       corev1.ResourceRequirements{},
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
				Image:           "datadog/agent:latest",
				ImagePullPolicy: corev1.PullIfNotPresent,
				Resources:       corev1.ResourceRequirements{},
				Command:         []string{"bash", "-c"},
				Args:            []string{"for script in $(find /etc/cont-init.d/ -type f -name '*.sh' | sort) ; do bash $script ; done"},
				Env:             defaultEnvVars(),
				VolumeMounts:    defaultMountVolume(),
			},
		},
		Containers: []corev1.Container{
			{
				Name:            "agent",
				Image:           "datadog/agent:latest",
				ImagePullPolicy: corev1.PullIfNotPresent,
				Command: []string{
					"agent",
					"start",
				},
				Resources: corev1.ResourceRequirements{},
				Ports: []corev1.ContainerPort{
					{
						ContainerPort: 8125,
						Name:          "dogstatsdport",
						Protocol:      "UDP",
					},
				},
				Env:           defaultEnvVars(),
				VolumeMounts:  defaultMountVolume(),
				LivenessProbe: defaultLivenessProbe(),
			},
		},
		Volumes: defaultVolumes(),
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
	logf.SetLogger(logf.ZapLogger(true))
	reqLogger := log.WithValues("test:", test.name)
	got, _, err := newExtendedDaemonSetFromInstance(reqLogger, test.agentdeployment, test.selector)
	if test.wantErr {
		assert.Error(t, err, "newExtendedDaemonSetFromInstance() expected an error")
	} else {
		assert.NoError(t, err, "newExtendedDaemonSetFromInstance() unexpected error: %v", err)
	}
	assert.True(t, apiequality.Semantic.DeepEqual(got, test.want), "newExtendedDaemonSetFromInstance() = %#v\n\nwant %#v\ndiff: %s",
		got, test.want, cmp.Diff(got, test.want))
}

type extendedDaemonSetFromInstanceTestSuite []extendedDaemonSetFromInstanceTest

func (tests extendedDaemonSetFromInstanceTestSuite) Run(t *testing.T) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, tt.Run)
	}
}

func Test_newExtendedDaemonSetFromInstance(t *testing.T) {
	tests := extendedDaemonSetFromInstanceTestSuite{
		{
			name:            "defaulted case",
			agentdeployment: test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{UseEDS: true, ClusterAgentEnabled: true}),
			wantErr:         false,
			want: &edsdatadoghqv1alpha1.ExtendedDaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "bar",
					Name:      "foo",
					Labels: map[string]string{
						"agent.datadoghq.com/name":      "foo",
						"agent.datadoghq.com/component": "agent",
						"app.kubernetes.io/instance":    "agent",
						"app.kubernetes.io/managed-by":  "datadog-operator",
						"app.kubernetes.io/name":        "datadog-agent-deployment",
						"app.kubernetes.io/part-of":     "foo",
						"app.kubernetes.io/version":     "",
					},
					Annotations: map[string]string{"agent.datadoghq.com/agentspechash": defaultAgentHash},
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
								"app.kubernetes.io/part-of":     "foo",
								"app.kubernetes.io/version":     "",
							},
							Annotations: make(map[string]string),
						},
						Spec: defaultPodSpec(),
					},
					Strategy: getDefaultEDSStrategy(),
				},
			},
		},
		{
			name:            "with labels and annotations",
			agentdeployment: test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{UseEDS: true, ClusterAgentEnabled: true, Labels: map[string]string{"label-foo-key": "label-bar-value"}, Annotations: map[string]string{"annotations-foo-key": "annotations-bar-value"}}),
			wantErr:         false,
			want: &edsdatadoghqv1alpha1.ExtendedDaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "bar",
					Name:      "foo",
					Labels: map[string]string{
						"agent.datadoghq.com/name":      "foo",
						"agent.datadoghq.com/component": "agent",
						"label-foo-key":                 "label-bar-value",
						"app.kubernetes.io/instance":    "agent",
						"app.kubernetes.io/managed-by":  "datadog-operator",
						"app.kubernetes.io/name":        "datadog-agent-deployment",
						"app.kubernetes.io/part-of":     "foo",
						"app.kubernetes.io/version":     "",
					},
					Annotations: map[string]string{"agent.datadoghq.com/agentspechash": defaultAgentHash},
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
								"app.kubernetes.io/part-of":     "foo",
								"app.kubernetes.io/version":     "",
							},
						},
						Spec: defaultPodSpec(),
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

	customConfigMapsPodSpec := defaultPodSpec()
	customConfigMapsPodSpec.Volumes = []corev1.Volume{
		{
			Name: datadoghqv1alpha1.ConfdVolumeName,
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
			Name: datadoghqv1alpha1.ConfigVolumeName,
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
			Name: "runtimesocket",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/run/docker.sock",
				},
			},
		},
	}

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

	customConfigMapAgentHash, _ := comparison.GenerateMD5ForSpec(customConfigMapAgentDeployment.Spec)

	test := extendedDaemonSetFromInstanceTest{
		name:            "with custom confd and checksd volume mounts",
		agentdeployment: customConfigMapAgentDeployment,
		wantErr:         false,
		want: &edsdatadoghqv1alpha1.ExtendedDaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "bar",
				Name:      "foo",
				Labels: map[string]string{
					"agent.datadoghq.com/name":      "foo",
					"agent.datadoghq.com/component": "agent",
					"app.kubernetes.io/instance":    "agent",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "foo",
					"app.kubernetes.io/version":     "",
				},
				Annotations: map[string]string{"agent.datadoghq.com/agentspechash": customConfigMapAgentHash},
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
							"app.kubernetes.io/part-of":     "foo",
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
	customConfigMapCustomDatadogYaml := test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{UseEDS: true, ClusterAgentEnabled: true, CustomConfig: "foo: bar\nbar: foo"})
	customConfigMapCustomDatadogYamlSpec := defaultPodSpec()
	customConfigMapCustomDatadogYamlSpec.Volumes = []corev1.Volume{
		{
			Name: datadoghqv1alpha1.ConfdVolumeName,
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
			Name: datadoghqv1alpha1.ConfigVolumeName,
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
			Name: "runtimesocket",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/var/run/docker.sock",
				},
			},
		},
	}
	customConfigMapCustomDatadogYamlSpec.Containers[0].VolumeMounts = []corev1.VolumeMount{
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
			Name:      "config",
			MountPath: "/etc/datadog-agent",
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
			Name:      "custom-datadog-yaml",
			MountPath: "/etc/datadog-agent/datadog.yaml",
			SubPath:   "datadog.yaml",
			ReadOnly:  true,
		},
		{
			Name:      "runtimesocket",
			MountPath: "/var/run/docker.sock",
			ReadOnly:  true,
		},
	}
	customConfigMapCustomDatadogYamlSpec.InitContainers[1].VolumeMounts = customConfigMapCustomDatadogYamlSpec.Containers[0].VolumeMounts
	customConfigMagCustomDatadogYamlHash, _ := comparison.GenerateMD5ForSpec(customConfigMapCustomDatadogYaml.Spec)

	test := extendedDaemonSetFromInstanceTest{
		name:            "with custom config (datadog.yaml)",
		agentdeployment: customConfigMapCustomDatadogYaml,
		wantErr:         false,
		want: &edsdatadoghqv1alpha1.ExtendedDaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "bar",
				Name:      "foo",
				Labels: map[string]string{
					"agent.datadoghq.com/name":      "foo",
					"agent.datadoghq.com/component": "agent",
					"app.kubernetes.io/instance":    "agent",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "foo",
					"app.kubernetes.io/version":     "",
				},
				Annotations: map[string]string{"agent.datadoghq.com/agentspechash": customConfigMagCustomDatadogYamlHash},
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
							"app.kubernetes.io/part-of":     "foo",
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
	userMountsPodSpec := defaultPodSpec()
	userMountsPodSpec.Volumes = append(userMountsPodSpec.Volumes, userVolumes...)
	userMountsPodSpec.Containers[0].VolumeMounts = append(userMountsPodSpec.Containers[0].VolumeMounts, userVolumeMounts...)
	userMountsPodSpec.InitContainers[1].VolumeMounts = append(userMountsPodSpec.InitContainers[1].VolumeMounts, userVolumeMounts...)

	userMountsAgentDeployment := test.NewDefaultedDatadogAgent("bar", "foo",
		&test.NewDatadogAgentOptions{
			UseEDS:              true,
			ClusterAgentEnabled: true,
			Volumes:             userVolumes,
			VolumeMounts:        userVolumeMounts,
		})

	userMountsAgentHash, _ := comparison.GenerateMD5ForSpec(userMountsAgentDeployment.Spec)

	test := extendedDaemonSetFromInstanceTest{
		name:            "with user volumes and mounts",
		agentdeployment: userMountsAgentDeployment,
		wantErr:         false,
		want: &edsdatadoghqv1alpha1.ExtendedDaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "bar",
				Name:      "foo",
				Labels: map[string]string{
					"agent.datadoghq.com/name":      "foo",
					"agent.datadoghq.com/component": "agent",
					"app.kubernetes.io/instance":    "agent",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "foo",
					"app.kubernetes.io/version":     "",
				},
				Annotations: map[string]string{"agent.datadoghq.com/agentspechash": userMountsAgentHash},
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
							"app.kubernetes.io/part-of":     "foo",
							"app.kubernetes.io/version":     "",
						},
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

	daemonsetNameAgentHash, _ := comparison.GenerateMD5ForSpec(daemonsetNameAgentDeployment.Spec)

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
					"app.kubernetes.io/part-of":     "foo",
					"app.kubernetes.io/version":     "",
				},
				Annotations: map[string]string{"agent.datadoghq.com/agentspechash": daemonsetNameAgentHash},
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
							"app.kubernetes.io/part-of":     "foo",
							"app.kubernetes.io/version":     "",
							"app":                           "datadog-monitoring",
						},
					},
					Spec: defaultPodSpec(),
				},
				Strategy: getDefaultEDSStrategy(),
			},
		},
	}
	test.Run(t)
}

func Test_newExtendedDaemonSetFromInstance_LogsEnabled(t *testing.T) {
	logsEnabledPodSpec := defaultPodSpec()
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
	}

	logsEnabledPodSpec.Volumes = append(logsEnabledPodSpec.Volumes, logsVolumes...)
	logsEnabledPodSpec.Containers[0].VolumeMounts = append(logsEnabledPodSpec.Containers[0].VolumeMounts, logsVolumeMounts...)
	logsEnabledPodSpec.Containers[0].Env = addEnvVar(logsEnabledPodSpec.Containers[0].Env, "DD_LOGS_ENABLED", "true")

	logsEnabledPodSpec.InitContainers[1].VolumeMounts = append(logsEnabledPodSpec.InitContainers[1].VolumeMounts, logsVolumeMounts...)
	logsEnabledPodSpec.InitContainers[1].Env = addEnvVar(logsEnabledPodSpec.InitContainers[1].Env, "DD_LOGS_ENABLED", "true")

	dda := test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{
		UseEDS:              true,
		ClusterAgentEnabled: true,
	})
	logEnabled := true
	dda.Spec.Agent.Log.Enabled = &logEnabled

	ddaHash, _ := comparison.GenerateMD5ForSpec(dda.Spec)

	test := extendedDaemonSetFromInstanceTest{
		name:            "with logs enabled",
		agentdeployment: dda,
		wantErr:         false,
		want: &edsdatadoghqv1alpha1.ExtendedDaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "bar",
				Name:      "foo",
				Labels: map[string]string{
					"agent.datadoghq.com/name":      "foo",
					"agent.datadoghq.com/component": "agent",
					"app.kubernetes.io/instance":    "agent",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "foo",
					"app.kubernetes.io/version":     "",
				},
				Annotations: map[string]string{"agent.datadoghq.com/agentspechash": ddaHash},
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
							"app.kubernetes.io/part-of":     "foo",
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
