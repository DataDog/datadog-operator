package datadogagent

import (
	"fmt"
	"reflect"
	"testing"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
	test "github.com/DataDog/datadog-operator/api/v1alpha1/test"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"

	"github.com/google/go-cmp/cmp"
	assert "github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var testClusterChecksRunnerReplicas int32 = 2

func clusterChecksRunnerDefaultPodSpec() corev1.PodSpec {
	return corev1.PodSpec{
		Affinity:           getPodAffinity(nil),
		ServiceAccountName: "foo-cluster-checks-runner",
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
				Env:             clusterChecksRunnerDefaultEnvVars(),
				VolumeMounts:    clusterChecksRunnerDefaultVolumeMounts(),
			},
		},
		Containers: []corev1.Container{
			{
				Name:            "cluster-checks-runner",
				Image:           "datadog/agent:latest",
				ImagePullPolicy: corev1.PullIfNotPresent,
				Resources:       corev1.ResourceRequirements{},
				Env:             clusterChecksRunnerDefaultEnvVars(),
				VolumeMounts:    clusterChecksRunnerDefaultVolumeMounts(),
				LivenessProbe:   getDefaultLivenessProbe(),
				ReadinessProbe:  getDefaultReadinessProbe(),
				Command:         []string{"agent", "run"},
			},
		},
		Volumes: clusterChecksRunnerDefaultVolumes(),
	}
}

func clusterChecksRunnerDefaultVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      datadoghqv1alpha1.ChecksdVolumeName,
			MountPath: datadoghqv1alpha1.ChecksdVolumePath,
			ReadOnly:  true,
		},
		{
			Name:      datadoghqv1alpha1.ConfigVolumeName,
			MountPath: datadoghqv1alpha1.ConfigVolumePath,
		},
		{
			Name:      "installinfo",
			SubPath:   "install_info",
			MountPath: "/etc/datadog-agent/install_info",
			ReadOnly:  true,
		},
		{
			Name:      "remove-corechecks",
			MountPath: fmt.Sprintf("%s/%s", datadoghqv1alpha1.ConfigVolumePath, "conf.d"),
		},
	}
}

func clusterChecksRunnerDefaultVolumes() []corev1.Volume {
	return []corev1.Volume{
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
			Name: "installinfo",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "foo-install-info",
					},
				},
			},
		},
		{
			Name: "remove-corechecks",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}
}

func clusterChecksRunnerDefaultEnvVars() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "DD_CLUSTER_NAME",
			Value: "",
		},
		{
			Name:      "DD_API_KEY",
			ValueFrom: apiKeyValue(),
		},
		{
			Name:  "DD_SITE",
			Value: "",
		},
		{
			Name:  "DD_CLUSTER_CHECKS_ENABLED",
			Value: "true",
		},
		{
			Name:  "DD_CLUSTER_AGENT_ENABLED",
			Value: "true",
		},
		{
			Name:  "DD_CLUSTER_AGENT_KUBERNETES_SERVICE_NAME",
			Value: fmt.Sprintf("%s-%s", testDdaName, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix),
		},
		{
			Name:      "DD_CLUSTER_AGENT_AUTH_TOKEN",
			ValueFrom: authTokenValue(),
		},
		{
			Name:  "DD_EXTRA_CONFIG_PROVIDERS",
			Value: "clusterchecks",
		},
		{
			Name:  "DD_HEALTH_PORT",
			Value: "5555",
		},
		{
			Name:  "DD_APM_ENABLED",
			Value: "false",
		},
		{
			Name:  "DD_PROCESS_AGENT_ENABLED",
			Value: "false",
		},
		{
			Name:  "DD_LOGS_ENABLED",
			Value: "false",
		},
		{
			Name:  "DD_ENABLE_METADATA_COLLECTION",
			Value: "false",
		},
		{
			Name:  "DD_CLC_RUNNER_ENABLED",
			Value: "true",
		},
		{
			Name: "DD_CLC_RUNNER_HOST",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "status.podIP",
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
	}
}

type clusterChecksRunnerDeploymentFromInstanceTest struct {
	name            string
	agentdeployment *datadoghqv1alpha1.DatadogAgent
	selector        *metav1.LabelSelector
	newStatus       *datadoghqv1alpha1.DatadogAgentStatus
	want            *appsv1.Deployment
	wantErr         bool
}

func (test clusterChecksRunnerDeploymentFromInstanceTest) Run(t *testing.T) {
	t.Helper()
	logf.SetLogger(logf.ZapLogger(true))
	got, _, err := newClusterChecksRunnerDeploymentFromInstance(test.agentdeployment, test.selector)
	if test.wantErr {
		assert.Error(t, err, "newClusterChecksRunnerDeploymentFromInstance() expected an error")
	} else {
		assert.NoError(t, err, "newClusterChecksRunnerDeploymentFromInstance() unexpected error: %v", err)
	}
	assert.True(t, apiequality.Semantic.DeepEqual(got, test.want), "newClusterChecksRunnerDeploymentFromInstance() = %#v, want %#v\ndiff = %s", got, test.want,
		cmp.Diff(got, test.want))
}

type clusterChecksRunnerDeploymentFromInstanceTestSuite []clusterChecksRunnerDeploymentFromInstanceTest

func (tests clusterChecksRunnerDeploymentFromInstanceTestSuite) Run(t *testing.T) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, tt.Run)
	}
}

func Test_newClusterChecksRunnerDeploymentFromInstance_UserVolumes(t *testing.T) {
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
	userMountsPodSpec := clusterChecksRunnerDefaultPodSpec()
	userMountsPodSpec.Volumes = append(userMountsPodSpec.Volumes, userVolumes...)
	userMountsPodSpec.InitContainers[1].VolumeMounts = append(userMountsPodSpec.InitContainers[1].VolumeMounts, userVolumeMounts...)
	userMountsPodSpec.Containers[0].VolumeMounts = append(userMountsPodSpec.Containers[0].VolumeMounts, userVolumeMounts...)

	envVarsAgentDeployment := test.NewDefaultedDatadogAgent(
		"bar",
		"foo",
		&test.NewDatadogAgentOptions{
			ClusterAgentEnabled:             true,
			ClusterChecksRunnerEnabled:      true,
			ClusterChecksRunnerVolumes:      userVolumes,
			ClusterChecksRunnerVolumeMounts: userVolumeMounts,
		},
	)
	envVarsClusterChecksRunnerAgentHash, _ := comparison.GenerateMD5ForSpec(envVarsAgentDeployment.Spec.ClusterChecksRunner)

	test := clusterChecksRunnerDeploymentFromInstanceTest{
		name:            "with user volumes",
		agentdeployment: envVarsAgentDeployment,
		newStatus:       &datadoghqv1alpha1.DatadogAgentStatus{},
		wantErr:         false,
		want: &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "bar",
				Name:      "foo-cluster-checks-runner",
				Labels: map[string]string{"agent.datadoghq.com/name": "foo",
					"agent.datadoghq.com/component": "cluster-checks-runner",
					"app.kubernetes.io/instance":    "cluster-checks-runner",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "foo",
					"app.kubernetes.io/version":     "",
				},
				Annotations: map[string]string{"agent.datadoghq.com/agentspechash": envVarsClusterChecksRunnerAgentHash},
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "cluster-checks-runner",
							"app.kubernetes.io/instance":    "cluster-checks-runner",
							"app.kubernetes.io/managed-by":  "datadog-operator",
							"app.kubernetes.io/name":        "datadog-agent-deployment",
							"app.kubernetes.io/part-of":     "foo",
							"app.kubernetes.io/version":     "",
						},
						Annotations: map[string]string{"agent.datadoghq.com/agentspechash": envVarsClusterChecksRunnerAgentHash},
					},
					Spec: userMountsPodSpec,
				},
				Replicas: &testClusterChecksRunnerReplicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"agent.datadoghq.com/name":      "foo",
						"agent.datadoghq.com/component": "cluster-checks-runner",
					},
				},
			},
		},
	}
	test.Run(t)
}

func Test_newClusterChecksRunnerDeploymentFromInstance_EnvVars(t *testing.T) {
	envVars := []corev1.EnvVar{
		{
			Name:  "ExtraEnvVar",
			Value: "ExtraEnvVarValue",
		},
		{
			Name: "ExtraEnvVarFromSpec",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "status.podIP",
				},
			},
		},
	}
	podSpec := clusterChecksRunnerDefaultPodSpec()
	podSpec.InitContainers[1].Env = append(podSpec.InitContainers[1].Env, envVars...)
	podSpec.Containers[0].Env = append(podSpec.Containers[0].Env, envVars...)

	envVarsAgentDeployment := test.NewDefaultedDatadogAgent(
		"bar",
		"foo",
		&test.NewDatadogAgentOptions{
			ClusterAgentEnabled:        true,
			ClusterChecksRunnerEnabled: true,
			ClusterChecksRunnerEnvVars: envVars,
		},
	)
	envVarsClusterChecksRunnerAgentHash, _ := comparison.GenerateMD5ForSpec(envVarsAgentDeployment.Spec.ClusterChecksRunner)

	test := clusterChecksRunnerDeploymentFromInstanceTest{
		name:            "with extra env vars",
		agentdeployment: envVarsAgentDeployment,
		newStatus:       &datadoghqv1alpha1.DatadogAgentStatus{},
		wantErr:         false,
		want: &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "bar",
				Name:      "foo-cluster-checks-runner",
				Labels: map[string]string{"agent.datadoghq.com/name": "foo",
					"agent.datadoghq.com/component": "cluster-checks-runner",
					"app.kubernetes.io/instance":    "cluster-checks-runner",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "foo",
					"app.kubernetes.io/version":     "",
				},
				Annotations: map[string]string{"agent.datadoghq.com/agentspechash": envVarsClusterChecksRunnerAgentHash},
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "cluster-checks-runner",
							"app.kubernetes.io/instance":    "cluster-checks-runner",
							"app.kubernetes.io/managed-by":  "datadog-operator",
							"app.kubernetes.io/name":        "datadog-agent-deployment",
							"app.kubernetes.io/part-of":     "foo",
							"app.kubernetes.io/version":     "",
						},
						Annotations: map[string]string{"agent.datadoghq.com/agentspechash": envVarsClusterChecksRunnerAgentHash},
					},
					Spec: podSpec,
				},
				Replicas: &testClusterChecksRunnerReplicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"agent.datadoghq.com/name":      "foo",
						"agent.datadoghq.com/component": "cluster-checks-runner",
					},
				},
			},
		},
	}
	test.Run(t)
}

func Test_getPodAffinity(t *testing.T) {
	tests := []struct {
		name     string
		affinity *corev1.Affinity
		want     *corev1.Affinity
	}{
		{
			name:     "no user-defined affinity - apply default",
			affinity: nil,
			want: &corev1.Affinity{
				PodAntiAffinity: &corev1.PodAntiAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"agent.datadoghq.com/component": "cluster-checks-runner",
								},
							},
							TopologyKey: "kubernetes.io/hostname",
						},
					},
				},
			},
		},
		{
			name: "user-defined affinity",
			affinity: &corev1.Affinity{
				PodAntiAffinity: &corev1.PodAntiAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"foo": "bar",
								},
							},
							TopologyKey: "baz",
						},
					},
				},
			},
			want: &corev1.Affinity{
				PodAntiAffinity: &corev1.PodAntiAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"foo": "bar",
								},
							},
							TopologyKey: "baz",
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getPodAffinity(tt.affinity); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getPodAffinity() = %v, want %v", got, tt.want)
			}
		})
	}
}
