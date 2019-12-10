// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagentdeployment

import (
	"fmt"
	"testing"
	"time"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	test "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1/test"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	edsdatadoghqv1alpha1 "github.com/datadog/extendeddaemonset/pkg/apis/datadoghq/v1alpha1"
	"github.com/google/go-cmp/cmp"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

func Test_newExtendedDaemonSetFromInstance(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))
	authTokenValue := &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{},
	}
	dadName := "foo"
	authTokenValue.SecretKeyRef.Name = fmt.Sprintf("%s-%s", dadName, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix)
	authTokenValue.SecretKeyRef.Key = "token"

	defaultMountVolume := []corev1.VolumeMount{
		{
			Name:      "confd",
			MountPath: "/conf.d",
		},
		{
			Name:      "checksd",
			MountPath: "/checks.d",
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
	defaultEnvVars := []corev1.EnvVar{
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
			Value: fmt.Sprintf("%s-%s", dadName, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix),
		},
		{
			Name:      "DD_CLUSTER_AGENT_AUTH_TOKEN",
			ValueFrom: authTokenValue,
		},
	}
	defaultLivenessProbe := &corev1.Probe{
		InitialDelaySeconds: 15,
		PeriodSeconds:       15,
		TimeoutSeconds:      5,
		SuccessThreshold:    1,
		FailureThreshold:    6,
	}
	defaultLivenessProbe.HTTPGet = &corev1.HTTPGetAction{
		Path: "/health",
		Port: intstr.IntOrString{
			IntVal: 5555,
		},
	}
	defaultPodSpec := corev1.PodSpec{
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
				Env:             defaultEnvVars,
				VolumeMounts:    defaultMountVolume,
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
				Env:           defaultEnvVars,
				VolumeMounts:  defaultMountVolume,
				LivenessProbe: defaultLivenessProbe,
			},
		},
		Volumes: []corev1.Volume{
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
		},
	}

	customConfdConfigMapName := "confd-configmap"
	customChecksdConfigMapName := "checksd-configmap"

	customConfigMapsPodSpec := defaultPodSpec.DeepCopy()
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

	customConfigMapAgentDeployment := test.NewDefaultedDatadogAgentDeployment("bar", "foo", &test.NewDatadogAgentDeploymentOptions{
		UseEDS:              true,
		ClusterAgentEnabled: true,
		Confd: &datadoghqv1alpha1.DirConfig{
			ConfigMapName: customConfdConfigMapName,
		},
		Checksd: &datadoghqv1alpha1.DirConfig{
			ConfigMapName: customChecksdConfigMapName,
		},
	})

	customConfigMapAgentHash, _ := comparison.GenerateMD5ForSpec(customConfigMapAgentDeployment.Spec)

	tests := []struct {
		name            string
		agentdeployment *datadoghqv1alpha1.DatadogAgentDeployment
		want            *edsdatadoghqv1alpha1.ExtendedDaemonSet
		wantErr         bool
	}{
		{
			name:            "defaulted case",
			agentdeployment: test.NewDefaultedDatadogAgentDeployment("bar", "foo", &test.NewDatadogAgentDeploymentOptions{UseEDS: true, ClusterAgentEnabled: true}),
			wantErr:         false,
			want: &edsdatadoghqv1alpha1.ExtendedDaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "bar",
					Name:      "foo",
					Labels: map[string]string{
						"agentdeployment.datadoghq.com/name":      "foo",
						"agentdeployment.datadoghq.com/component": "agent",
						"app.kubernetes.io/instance":              "agent",
						"app.kubernetes.io/managed-by":            "datadog-operator",
						"app.kubernetes.io/name":                  "datadog-agent-deployment",
						"app.kubernetes.io/part-of":               "foo",
						"app.kubernetes.io/version":               "",
					},
					Annotations: map[string]string{"agentdeployment.datadoghq.com/agentspechash": defaultAgentHash},
				},
				Spec: edsdatadoghqv1alpha1.ExtendedDaemonSetSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							GenerateName: "foo",
							Namespace:    "bar",
							Labels: map[string]string{
								"agentdeployment.datadoghq.com/name":      "foo",
								"agentdeployment.datadoghq.com/component": "agent",
								"app.kubernetes.io/instance":              "agent",
								"app.kubernetes.io/managed-by":            "datadog-operator",
								"app.kubernetes.io/name":                  "datadog-agent-deployment",
								"app.kubernetes.io/part-of":               "foo",
								"app.kubernetes.io/version":               "",
							},
							Annotations: make(map[string]string),
						},
						Spec: defaultPodSpec,
					},
					Strategy: getDefaultEDSStrategy(),
				},
			},
		},
		{
			name:            "with labels and annotations",
			agentdeployment: test.NewDefaultedDatadogAgentDeployment("bar", "foo", &test.NewDatadogAgentDeploymentOptions{UseEDS: true, ClusterAgentEnabled: true, Labels: map[string]string{"label-foo-key": "label-bar-value"}, Annotations: map[string]string{"annotations-foo-key": "annotations-bar-value"}}),
			wantErr:         false,
			want: &edsdatadoghqv1alpha1.ExtendedDaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "bar",
					Name:      "foo",
					Labels: map[string]string{
						"agentdeployment.datadoghq.com/name":      "foo",
						"agentdeployment.datadoghq.com/component": "agent",
						"label-foo-key":                "label-bar-value",
						"app.kubernetes.io/instance":   "agent",
						"app.kubernetes.io/managed-by": "datadog-operator",
						"app.kubernetes.io/name":       "datadog-agent-deployment",
						"app.kubernetes.io/part-of":    "foo",
						"app.kubernetes.io/version":    "",
					},
					Annotations: map[string]string{"agentdeployment.datadoghq.com/agentspechash": defaultAgentHash},
				},
				Spec: edsdatadoghqv1alpha1.ExtendedDaemonSetSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							GenerateName: "foo",
							Namespace:    "bar",
							Labels: map[string]string{
								"agentdeployment.datadoghq.com/name":      "foo",
								"agentdeployment.datadoghq.com/component": "agent",
								"app.kubernetes.io/instance":              "agent",
								"app.kubernetes.io/managed-by":            "datadog-operator",
								"app.kubernetes.io/name":                  "datadog-agent-deployment",
								"app.kubernetes.io/part-of":               "foo",
								"app.kubernetes.io/version":               "",
							},
						},
						Spec: defaultPodSpec,
					},
					Strategy: getDefaultEDSStrategy(),
				},
			},
		},
		{
			name:            "with custom confd and checksd volume mounts",
			agentdeployment: customConfigMapAgentDeployment,
			wantErr:         false,
			want: &edsdatadoghqv1alpha1.ExtendedDaemonSet{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "bar",
					Name:      "foo",
					Labels: map[string]string{
						"agentdeployment.datadoghq.com/name":      "foo",
						"agentdeployment.datadoghq.com/component": "agent",
						"app.kubernetes.io/instance":              "agent",
						"app.kubernetes.io/managed-by":            "datadog-operator",
						"app.kubernetes.io/name":                  "datadog-agent-deployment",
						"app.kubernetes.io/part-of":               "foo",
						"app.kubernetes.io/version":               "",
					},
					Annotations: map[string]string{"agentdeployment.datadoghq.com/agentspechash": customConfigMapAgentHash},
				},
				Spec: edsdatadoghqv1alpha1.ExtendedDaemonSetSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							GenerateName: "foo",
							Namespace:    "bar",
							Labels: map[string]string{
								"agentdeployment.datadoghq.com/name":      "foo",
								"agentdeployment.datadoghq.com/component": "agent",
								"app.kubernetes.io/instance":              "agent",
								"app.kubernetes.io/managed-by":            "datadog-operator",
								"app.kubernetes.io/name":                  "datadog-agent-deployment",
								"app.kubernetes.io/part-of":               "foo",
								"app.kubernetes.io/version":               "",
							},
							Annotations: make(map[string]string),
						},
						Spec: *customConfigMapsPodSpec,
					},
					Strategy: getDefaultEDSStrategy(),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqLogger := log.WithValues("test:", tt.name)
			got, _, err := newExtendedDaemonSetFromInstance(reqLogger, tt.agentdeployment)
			if (err != nil) != tt.wantErr {
				t.Errorf("newExtendedDaemonSetFromInstance() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !apiequality.Semantic.DeepEqual(got, tt.want) {
				t.Errorf("newExtendedDaemonSetFromInstance() = %#v\n\nwant %#v\ndiff: %s", got, tt.want, cmp.Diff(got, tt.want))
			}
		})
	}
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
