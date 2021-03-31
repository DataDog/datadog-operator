package datadogagent

import (
	"fmt"
	"strconv"
	"testing"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
	"github.com/DataDog/datadog-operator/api/v1alpha1/test"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/orchestrator"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	assert "github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var testClusterAgentReplicas int32 = 1

func clusterAgentDefaultPodSpec() corev1.PodSpec {
	return corev1.PodSpec{
		ServiceAccountName: "foo-cluster-agent",
		Containers: []corev1.Container{
			{
				Name:            "cluster-agent",
				Image:           "gcr.io/datadoghq/cluster-agent:latest",
				ImagePullPolicy: corev1.PullIfNotPresent,
				Resources:       corev1.ResourceRequirements{},
				Ports: []corev1.ContainerPort{
					{
						ContainerPort: 5005,
						Name:          "agentport",
						Protocol:      "TCP",
					},
				},
				Env: clusterAgentDefaultEnvVars(),
				VolumeMounts: []corev1.VolumeMount{
					{Name: "installinfo", ReadOnly: true, SubPath: "install_info", MountPath: "/etc/datadog-agent/install_info"},
					{Name: "confd", ReadOnly: true, MountPath: "/conf.d"},
				},
			},
		},
		Volumes: []corev1.Volume{
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
				Name:         "confd",
				VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}},
			},
		},
	}
}

func clusterAgentDefaultEnvVars() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  "DD_CLUSTER_NAME",
			Value: "",
		},
		{
			Name:  "DD_CLUSTER_CHECKS_ENABLED",
			Value: "false",
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
			Name:  "DD_LEADER_ELECTION",
			Value: "true",
		},
		{
			Name:  "DD_COMPLIANCE_CONFIG_ENABLED",
			Value: "false",
		},
		{
			Name:  "DD_COLLECT_KUBERNETES_EVENTS",
			Value: "false",
		},
		{
			Name:      "DD_API_KEY",
			ValueFrom: apiKeyValue(),
		},
	}
}

type clusterAgentDeploymentFromInstanceTest struct {
	name            string
	agentdeployment *datadoghqv1alpha1.DatadogAgent
	selector        *metav1.LabelSelector
	newStatus       *datadoghqv1alpha1.DatadogAgentStatus
	want            *appsv1.Deployment
	wantErr         bool
}

func (test clusterAgentDeploymentFromInstanceTest) Run(t *testing.T) {
	t.Helper()
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	logger := logf.Log.WithName(t.Name())
	got, _, err := newClusterAgentDeploymentFromInstance(logger, test.agentdeployment, test.selector)
	if test.wantErr {
		assert.Error(t, err, "newClusterAgentDeploymentFromInstance() expected an error")
	} else {
		assert.NoError(t, err, "newClusterAgentDeploymentFromInstance() unexpected error: %v", err)
		deploymentSpecHash, _ := comparison.GenerateMD5ForSpec(test.want.Spec)
		if test.want.Annotations == nil {
			test.want.Annotations = map[string]string{}
		}
		test.want.Annotations["agent.datadoghq.com/agentspechash"] = deploymentSpecHash
	}
	assert.True(t, apiequality.Semantic.DeepEqual(got, test.want), "newClusterAgentDeploymentFromInstance() = %#v, want %#v\ndiff = %s", got, test.want,
		cmp.Diff(got, test.want))
}

type clusterAgentDeploymentFromInstanceTestSuite []clusterAgentDeploymentFromInstanceTest

func (tests clusterAgentDeploymentFromInstanceTestSuite) Run(t *testing.T) {
	t.Helper()
	for _, tt := range tests {
		t.Run(tt.name, tt.Run)
	}
}

func Test_newClusterAgentDeploymentFromInstance(t *testing.T) {
	tests := clusterAgentDeploymentFromInstanceTestSuite{
		{
			name:            "defaulted case",
			agentdeployment: test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{ClusterAgentEnabled: true}),
			newStatus:       &datadoghqv1alpha1.DatadogAgentStatus{},
			wantErr:         false,
			want: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "bar",
					Name:      "foo-cluster-agent",
					Labels: map[string]string{"agent.datadoghq.com/name": "foo",
						"agent.datadoghq.com/component": "cluster-agent",
						"app.kubernetes.io/instance":    "cluster-agent",
						"app.kubernetes.io/managed-by":  "datadog-operator",
						"app.kubernetes.io/name":        "datadog-agent-deployment",
						"app.kubernetes.io/part-of":     "foo",
						"app.kubernetes.io/version":     "",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"agent.datadoghq.com/name":      "foo",
								"agent.datadoghq.com/component": "cluster-agent",
								"app.kubernetes.io/instance":    "cluster-agent",
								"app.kubernetes.io/managed-by":  "datadog-operator",
								"app.kubernetes.io/name":        "datadog-agent-deployment",
								"app.kubernetes.io/part-of":     "foo",
								"app.kubernetes.io/version":     "",
							},
						},
						Spec: clusterAgentDefaultPodSpec(),
					},
					Replicas: &testClusterAgentReplicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "cluster-agent",
						},
					},
				},
			},
		},
		{
			name:            "defaulted case with DatadogFeature orchestrator Explorer",
			agentdeployment: test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{ClusterAgentEnabled: true, OrchestratorExplorerEnabled: true}),
			newStatus:       &datadoghqv1alpha1.DatadogAgentStatus{},
			wantErr:         false,
			want: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "bar",
					Name:      "foo-cluster-agent",
					Labels: map[string]string{"agent.datadoghq.com/name": "foo",
						"agent.datadoghq.com/component": "cluster-agent",
						"app.kubernetes.io/instance":    "cluster-agent",
						"app.kubernetes.io/managed-by":  "datadog-operator",
						"app.kubernetes.io/name":        "datadog-agent-deployment",
						"app.kubernetes.io/part-of":     "foo",
						"app.kubernetes.io/version":     "",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"agent.datadoghq.com/name":      "foo",
								"agent.datadoghq.com/component": "cluster-agent",
								"app.kubernetes.io/instance":    "cluster-agent",
								"app.kubernetes.io/managed-by":  "datadog-operator",
								"app.kubernetes.io/name":        "datadog-agent-deployment",
								"app.kubernetes.io/part-of":     "foo",
								"app.kubernetes.io/version":     "",
							},
						},
						Spec: clusterAgentPodSpecOrchestratorEnabled(),
					},
					Replicas: &testClusterAgentReplicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "cluster-agent",
						},
					},
				},
			},
		},
		{
			name:            "with labels and annotations",
			agentdeployment: test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{ClusterAgentEnabled: true, Labels: map[string]string{"label-foo-key": "label-bar-value"}, Annotations: map[string]string{"annotations-foo-key": "annotations-bar-value"}}),
			newStatus:       &datadoghqv1alpha1.DatadogAgentStatus{},
			wantErr:         false,
			want: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "bar",
					Name:      "foo-cluster-agent",
					Labels: map[string]string{
						"agent.datadoghq.com/name":      "foo",
						"agent.datadoghq.com/component": "cluster-agent",
						"label-foo-key":                 "label-bar-value",
						"app.kubernetes.io/instance":    "cluster-agent",
						"app.kubernetes.io/managed-by":  "datadog-operator",
						"app.kubernetes.io/name":        "datadog-agent-deployment",
						"app.kubernetes.io/part-of":     "foo",
						"app.kubernetes.io/version":     "",
					},
					Annotations: map[string]string{"annotations-foo-key": "annotations-bar-value"},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"agent.datadoghq.com/name":      "foo",
								"agent.datadoghq.com/component": "cluster-agent",
								"label-foo-key":                 "label-bar-value",
								"app.kubernetes.io/instance":    "cluster-agent",
								"app.kubernetes.io/managed-by":  "datadog-operator",
								"app.kubernetes.io/name":        "datadog-agent-deployment",
								"app.kubernetes.io/part-of":     "foo",
								"app.kubernetes.io/version":     "",
							},
							Annotations: map[string]string{"annotations-foo-key": "annotations-bar-value"},
						},
						Spec: clusterAgentDefaultPodSpec(),
					},
					Replicas: &testClusterAgentReplicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "cluster-agent",
						},
					},
				},
			},
		},
	}
	tests.Run(t)
}

func clusterAgentPodSpecOrchestratorEnabled() v1.PodSpec {
	clusterAgentSpec := clusterAgentDefaultPodSpec()
	cnt := clusterAgentSpec.Containers[0]
	cnt.Env = append(cnt.Env, []corev1.EnvVar{
		{Name: "DD_ORCHESTRATOR_EXPLORER_ENABLED", Value: "true"},
		{Name: "DD_ORCHESTRATOR_EXPLORER_CONTAINER_SCRUBBING_ENABLED", Value: "true"},
	}...)
	clusterAgentSpec.Containers[0] = cnt
	return clusterAgentSpec
}

func Test_newClusterAgentDeploymentMountKSMCore(t *testing.T) {
	enabledFeature := true
	// test proper mount of volume
	ksmCore := datadoghqv1alpha1.KubeStateMetricsCore{
		Enabled: &enabledFeature,
		Conf: &datadoghqv1alpha1.CustomConfigSpec{
			ConfigMap: &datadoghqv1alpha1.ConfigFileConfigMapSpec{
				Name:    "bla",
				FileKey: "ksm_core.yaml",
			},
		},
	}
	envVars := []v1.EnvVar{
		{
			Name:  datadoghqv1alpha1.DDKubeStateMetricsCoreEnabled,
			Value: "true",
		},
		{
			Name:  datadoghqv1alpha1.DDKubeStateMetricsCoreConfigMap,
			Value: "bla",
		},
		{
			Name:  orchestrator.DDOrchestratorExplorerEnabled,
			Value: "true",
		},
		{
			Name:  orchestrator.DDOrchestratorExplorerContainerScrubbingEnabled,
			Value: "true",
		},
	}
	userVolumes := []corev1.Volume{
		{
			Name: "ksm-core-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "bla",
					},
				},
			},
		},
	}
	userVolumeMounts := []corev1.VolumeMount{
		{
			Name:      "ksm-core-config",
			MountPath: "/etc/datadog-agent/conf.d/kubernetes_state_core.yaml",
			SubPath:   "ksm_core.yaml",
			ReadOnly:  true,
		},
	}
	clusterAgentPodSpec := clusterAgentDefaultPodSpec()
	clusterAgentPodSpec.Volumes = append(clusterAgentPodSpec.Volumes, userVolumes...)
	clusterAgentPodSpec.Containers[0].VolumeMounts = append(clusterAgentPodSpec.Containers[0].VolumeMounts, userVolumeMounts...)
	clusterAgentPodSpec.Containers[0].Env = append(clusterAgentPodSpec.Containers[0].Env, envVars...)
	clusterAgentDeployment := test.NewDefaultedDatadogAgent(
		"bar",
		"foo",
		&test.NewDatadogAgentOptions{
			ClusterAgentEnabled:  true,
			KubeStateMetricsCore: &ksmCore,
		},
	)
	testDCA := clusterAgentDeploymentFromInstanceTest{
		name:            "with KSM core check custom conf volumes and mounts",
		agentdeployment: clusterAgentDeployment,
		newStatus:       &datadoghqv1alpha1.DatadogAgentStatus{},
		wantErr:         false,
		want: &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "bar",
				Name:      "foo-cluster-agent",
				Labels: map[string]string{"agent.datadoghq.com/name": "foo",
					"agent.datadoghq.com/component": "cluster-agent",
					"app.kubernetes.io/instance":    "cluster-agent",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "foo",
					"app.kubernetes.io/version":     "",
				},
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "cluster-agent",
							"app.kubernetes.io/instance":    "cluster-agent",
							"app.kubernetes.io/managed-by":  "datadog-operator",
							"app.kubernetes.io/name":        "datadog-agent-deployment",
							"app.kubernetes.io/part-of":     "foo",
							"app.kubernetes.io/version":     "",
						},
					},
					Spec: clusterAgentPodSpec,
				},
				Replicas: &testClusterAgentReplicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"agent.datadoghq.com/name":      "foo",
						"agent.datadoghq.com/component": "cluster-agent",
					},
				},
			},
		},
	}
	testDCA.Run(t)
}

func Test_newClusterAgentPrometheusScrapeEnabled(t *testing.T) {
	clusterAgentPodSpec := clusterAgentDefaultPodSpec()
	clusterAgentDeployment := test.NewDefaultedDatadogAgent(
		"bar",
		"foo",
		&test.NewDatadogAgentOptions{
			ClusterAgentEnabled: true,
			Features: &datadoghqv1alpha1.DatadogFeatures{
				OrchestratorExplorer: &datadoghqv1alpha1.OrchestratorExplorerConfig{Enabled: datadoghqv1alpha1.NewBoolPointer(false)},
				PrometheusScrape:     &datadoghqv1alpha1.PrometheusScrapeConfig{Enabled: datadoghqv1alpha1.NewBoolPointer(true), ServiceEndpoints: datadoghqv1alpha1.NewBoolPointer(true)}},
		},
	)

	logger := logf.Log.WithName(t.Name())
	clusterAgentPodSpec.Containers[0].Env = append(clusterAgentPodSpec.Containers[0].Env, prometheusScrapeEnvVars(logger, clusterAgentDeployment)...)

	testDCA := clusterAgentDeploymentFromInstanceTest{
		name:            "Prometheus scrape enabled",
		agentdeployment: clusterAgentDeployment,
		newStatus:       &datadoghqv1alpha1.DatadogAgentStatus{},
		wantErr:         false,
		want: &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "bar",
				Name:      "foo-cluster-agent",
				Labels: map[string]string{"agent.datadoghq.com/name": "foo",
					"agent.datadoghq.com/component": "cluster-agent",
					"app.kubernetes.io/instance":    "cluster-agent",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "foo",
					"app.kubernetes.io/version":     "",
				},
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "cluster-agent",
							"app.kubernetes.io/instance":    "cluster-agent",
							"app.kubernetes.io/managed-by":  "datadog-operator",
							"app.kubernetes.io/name":        "datadog-agent-deployment",
							"app.kubernetes.io/part-of":     "foo",
							"app.kubernetes.io/version":     "",
						},
					},
					Spec: clusterAgentPodSpec,
				},
				Replicas: &testClusterAgentReplicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"agent.datadoghq.com/name":      "foo",
						"agent.datadoghq.com/component": "cluster-agent",
					},
				},
			},
		},
	}
	testDCA.Run(t)
}

func Test_newClusterAgentDeploymentFromInstance_UserVolumes(t *testing.T) {
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
	userMountsPodSpec := clusterAgentDefaultPodSpec()
	userMountsPodSpec.Volumes = append(userMountsPodSpec.Volumes, userVolumes...)
	userMountsPodSpec.Containers[0].VolumeMounts = append(userMountsPodSpec.Containers[0].VolumeMounts, userVolumeMounts...)
	userMountsAgentDeployment := test.NewDefaultedDatadogAgent(
		"bar",
		"foo",
		&test.NewDatadogAgentOptions{
			ClusterAgentEnabled:      true,
			ClusterAgentVolumes:      userVolumes,
			ClusterAgentVolumeMounts: userVolumeMounts,
		},
	)
	test := clusterAgentDeploymentFromInstanceTest{
		name:            "with user volumes and mounts",
		agentdeployment: userMountsAgentDeployment,
		newStatus:       &datadoghqv1alpha1.DatadogAgentStatus{},
		wantErr:         false,
		want: &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "bar",
				Name:      "foo-cluster-agent",
				Labels: map[string]string{"agent.datadoghq.com/name": "foo",
					"agent.datadoghq.com/component": "cluster-agent",
					"app.kubernetes.io/instance":    "cluster-agent",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "foo",
					"app.kubernetes.io/version":     "",
				},
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "cluster-agent",
							"app.kubernetes.io/instance":    "cluster-agent",
							"app.kubernetes.io/managed-by":  "datadog-operator",
							"app.kubernetes.io/name":        "datadog-agent-deployment",
							"app.kubernetes.io/part-of":     "foo",
							"app.kubernetes.io/version":     "",
						},
					},
					Spec: userMountsPodSpec,
				},
				Replicas: &testClusterAgentReplicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"agent.datadoghq.com/name":      "foo",
						"agent.datadoghq.com/component": "cluster-agent",
					},
				},
			},
		},
	}
	test.Run(t)
}

func Test_newClusterAgentDeploymentFromInstance_EnvVars(t *testing.T) {
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
	podSpec := clusterAgentDefaultPodSpec()
	podSpec.Containers[0].Env = append(podSpec.Containers[0].Env, envVars...)

	envVarsAgentDeployment := test.NewDefaultedDatadogAgent(
		"bar",
		"foo",
		&test.NewDatadogAgentOptions{
			ClusterAgentEnabled: true,
			ClusterAgentEnvVars: envVars,
		},
	)

	test := clusterAgentDeploymentFromInstanceTest{
		name:            "with extra env vars",
		agentdeployment: envVarsAgentDeployment,
		newStatus:       &datadoghqv1alpha1.DatadogAgentStatus{},
		wantErr:         false,
		want: &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "bar",
				Name:      "foo-cluster-agent",
				Labels: map[string]string{"agent.datadoghq.com/name": "foo",
					"agent.datadoghq.com/component": "cluster-agent",
					"app.kubernetes.io/instance":    "cluster-agent",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "foo",
					"app.kubernetes.io/version":     "",
				},
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "cluster-agent",
							"app.kubernetes.io/instance":    "cluster-agent",
							"app.kubernetes.io/managed-by":  "datadog-operator",
							"app.kubernetes.io/name":        "datadog-agent-deployment",
							"app.kubernetes.io/part-of":     "foo",
							"app.kubernetes.io/version":     "",
						},
					},
					Spec: podSpec,
				},
				Replicas: &testClusterAgentReplicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"agent.datadoghq.com/name":      "foo",
						"agent.datadoghq.com/component": "cluster-agent",
					},
				},
			},
		},
	}
	test.Run(t)
}

func Test_newClusterAgentDeploymentFromInstance_CustomDeploymentName(t *testing.T) {
	customDeploymentName := "custom-cluster-agent-deployment"
	deploymentNamePodSpec := clusterAgentDefaultPodSpec()
	deploymentNamePodSpec.Affinity = nil

	deploymentNameAgentDeployment := test.NewDefaultedDatadogAgent("bar", "foo",
		&test.NewDatadogAgentOptions{
			UseEDS:                     true,
			ClusterAgentEnabled:        true,
			ClusterAgentDeploymentName: customDeploymentName,
		})

	test := clusterAgentDeploymentFromInstanceTest{
		name:            "with custom deployment name and selector",
		agentdeployment: deploymentNameAgentDeployment,
		selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"app": "datadog-monitoring",
			},
		},
		newStatus: &datadoghqv1alpha1.DatadogAgentStatus{},
		wantErr:   false,
		want: &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "bar",
				Name:      customDeploymentName,
				Labels: map[string]string{"agent.datadoghq.com/name": "foo",
					"agent.datadoghq.com/component": "cluster-agent",
					"app.kubernetes.io/instance":    "cluster-agent",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "foo",
					"app.kubernetes.io/version":     "",
					"app":                           "datadog-monitoring",
				},
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "cluster-agent",
							"app.kubernetes.io/instance":    "cluster-agent",
							"app.kubernetes.io/managed-by":  "datadog-operator",
							"app.kubernetes.io/name":        "datadog-agent-deployment",
							"app.kubernetes.io/part-of":     "foo",
							"app.kubernetes.io/version":     "",
							"app":                           "datadog-monitoring",
						},
					},
					Spec: deploymentNamePodSpec,
				},
				Replicas: &testClusterAgentReplicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"app": "datadog-monitoring",
					},
				},
			},
		},
	}
	test.Run(t)
}

func Test_newClusterAgentDeploymentFromInstance_MetricsServer(t *testing.T) {
	metricsServerPodSpec := clusterAgentDefaultPodSpec()
	metricsServerPort := int32(4443)
	metricsServerPodSpec.Containers[0].Ports = append(metricsServerPodSpec.Containers[0].Ports, corev1.ContainerPort{
		ContainerPort: metricsServerPort,
		Name:          "metricsapi",
		Protocol:      "TCP",
	})

	metricsServerPodSpec.Containers[0].Env = append(metricsServerPodSpec.Containers[0].Env,
		[]corev1.EnvVar{
			{
				Name:  "DD_EXTERNAL_METRICS_PROVIDER_ENABLED",
				Value: "true",
			},
			{
				Name:  "DD_EXTERNAL_METRICS_PROVIDER_PORT",
				Value: strconv.Itoa(int(metricsServerPort)),
			},
			{
				Name:      "DD_APP_KEY",
				ValueFrom: appKeyValue(),
			},
			{
				Name:  datadoghqv1alpha1.DDMetricsProviderUseDatadogMetric,
				Value: "false",
			},
			{
				Name:  datadoghqv1alpha1.DDMetricsProviderWPAController,
				Value: "false",
			},
		}...,
	)

	probe := &corev1.Probe{
		Handler: corev1.Handler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/healthz",
				Port: intstr.IntOrString{
					IntVal: metricsServerPort,
				},
				Scheme: corev1.URISchemeHTTPS,
			},
		},
	}

	metricsServerPodSpec.Containers[0].LivenessProbe = probe
	metricsServerPodSpec.Containers[0].ReadinessProbe = probe

	metricsServerAgentDeployment := test.NewDefaultedDatadogAgent("bar", "foo",
		&test.NewDatadogAgentOptions{
			UseEDS:               true,
			ClusterAgentEnabled:  true,
			MetricsServerEnabled: true,
			MetricsServerPort:    metricsServerPort,
		})

	metricsServerWithSitePodSpec := clusterAgentDefaultPodSpec()
	metricsServerWithSitePodSpec.Containers[0].Ports = append(metricsServerWithSitePodSpec.Containers[0].Ports, corev1.ContainerPort{
		ContainerPort: metricsServerPort,
		Name:          "metricsapi",
		Protocol:      "TCP",
	})

	metricsServerWithSitePodSpec.Containers[0].Env = append(metricsServerWithSitePodSpec.Containers[0].Env,
		[]corev1.EnvVar{
			{
				Name:  "DD_EXTERNAL_METRICS_PROVIDER_ENABLED",
				Value: "true",
			},
			{
				Name:  "DD_EXTERNAL_METRICS_PROVIDER_PORT",
				Value: strconv.Itoa(int(metricsServerPort)),
			},
			{
				Name:      "DD_APP_KEY",
				ValueFrom: appKeyValue(),
			},
			{
				Name:  datadoghqv1alpha1.DDMetricsProviderUseDatadogMetric,
				Value: "true",
			},
			{
				Name:  datadoghqv1alpha1.DDMetricsProviderWPAController,
				Value: "true",
			},
			{
				Name:  datadoghqv1alpha1.DDExternalMetricsProviderEndpoint,
				Value: "https://app.datadoghq.eu",
			},
			{
				Name:      datadoghqv1alpha1.DDExternalMetricsProviderAPIKey,
				ValueFrom: buildEnvVarFromSecret("foo-metrics-server", "api_key"),
			},
			{
				Name:      datadoghqv1alpha1.DDExternalMetricsProviderAppKey,
				ValueFrom: buildEnvVarFromSecret("extmetrics-app-key-secret-name", "appkey"),
			},
		}...,
	)
	metricsServerWithSitePodSpec.Containers[0].LivenessProbe = probe
	metricsServerWithSitePodSpec.Containers[0].ReadinessProbe = probe

	metricsServerAgentWithEndpointDeployment := test.NewDefaultedDatadogAgent("bar", "foo",
		&test.NewDatadogAgentOptions{
			UseEDS:                        true,
			ClusterAgentEnabled:           true,
			MetricsServerEnabled:          true,
			MetricsServerUseDatadogMetric: true,
			MetricsServerWPAController:    true,
			MetricsServerEndpoint:         "https://app.datadoghq.eu",
			MetricsServerPort:             metricsServerPort,
			MetricsServerCredentials: &datadoghqv1alpha1.DatadogCredentials{
				APIKey: "extmetrics-api-key-literal-foo",
				APPSecret: &datadoghqv1alpha1.Secret{
					SecretName: "extmetrics-app-key-secret-name",
					KeyName:    "appkey",
				},
			},
		})

	tests := clusterAgentDeploymentFromInstanceTestSuite{
		{
			name:            "with metrics server",
			agentdeployment: metricsServerAgentDeployment,
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "datadog-monitoring",
				},
			},
			newStatus: &datadoghqv1alpha1.DatadogAgentStatus{},
			wantErr:   false,
			want: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "bar",
					Name:      "foo-cluster-agent",
					Labels: map[string]string{"agent.datadoghq.com/name": "foo",
						"agent.datadoghq.com/component": "cluster-agent",
						"app.kubernetes.io/instance":    "cluster-agent",
						"app.kubernetes.io/managed-by":  "datadog-operator",
						"app.kubernetes.io/name":        "datadog-agent-deployment",
						"app.kubernetes.io/part-of":     "foo",
						"app.kubernetes.io/version":     "",
						"app":                           "datadog-monitoring",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"agent.datadoghq.com/name":      "foo",
								"agent.datadoghq.com/component": "cluster-agent",
								"app.kubernetes.io/instance":    "cluster-agent",
								"app.kubernetes.io/managed-by":  "datadog-operator",
								"app.kubernetes.io/name":        "datadog-agent-deployment",
								"app.kubernetes.io/part-of":     "foo",
								"app.kubernetes.io/version":     "",
								"app":                           "datadog-monitoring",
							},
						},
						Spec: metricsServerPodSpec,
					},
					Replicas: &testClusterAgentReplicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "datadog-monitoring",
						},
					},
				},
			},
		},
		{
			name:            "with metrics server and endpoint and custom API/APPKeys",
			agentdeployment: metricsServerAgentWithEndpointDeployment,
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "datadog-monitoring",
				},
			},
			newStatus: &datadoghqv1alpha1.DatadogAgentStatus{},
			wantErr:   false,
			want: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "bar",
					Name:      "foo-cluster-agent",
					Labels: map[string]string{"agent.datadoghq.com/name": "foo",
						"agent.datadoghq.com/component": "cluster-agent",
						"app.kubernetes.io/instance":    "cluster-agent",
						"app.kubernetes.io/managed-by":  "datadog-operator",
						"app.kubernetes.io/name":        "datadog-agent-deployment",
						"app.kubernetes.io/part-of":     "foo",
						"app.kubernetes.io/version":     "",
						"app":                           "datadog-monitoring",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"agent.datadoghq.com/name":      "foo",
								"agent.datadoghq.com/component": "cluster-agent",
								"app.kubernetes.io/instance":    "cluster-agent",
								"app.kubernetes.io/managed-by":  "datadog-operator",
								"app.kubernetes.io/name":        "datadog-agent-deployment",
								"app.kubernetes.io/part-of":     "foo",
								"app.kubernetes.io/version":     "",
								"app":                           "datadog-monitoring",
							},
							Annotations: map[string]string{},
						},
						Spec: metricsServerWithSitePodSpec,
					},
					Replicas: &testClusterAgentReplicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "datadog-monitoring",
						},
					},
				},
			},
		},
	}
	tests.Run(t)
}

func Test_newClusterAgentDeploymentFromInstance_AdmissionController(t *testing.T) {
	commonLabels := map[string]string{
		"agent.datadoghq.com/name":      "foo",
		"agent.datadoghq.com/component": "cluster-agent",
		"app.kubernetes.io/instance":    "cluster-agent",
		"app.kubernetes.io/managed-by":  "datadog-operator",
		"app.kubernetes.io/name":        "datadog-agent-deployment",
		"app.kubernetes.io/part-of":     "foo",
		"app.kubernetes.io/version":     "",
		"app":                           "datadog-monitoring",
	}

	admissionControllerPodSpec := clusterAgentDefaultPodSpec()
	admissionControllerPodSpec.Containers[0].Env = append(admissionControllerPodSpec.Containers[0].Env,
		[]corev1.EnvVar{
			{
				Name:  "DD_ADMISSION_CONTROLLER_ENABLED",
				Value: "true",
			},
			{
				Name:  "DD_ADMISSION_CONTROLLER_MUTATE_UNLABELLED",
				Value: "false",
			},
			{
				Name:  "DD_ADMISSION_CONTROLLER_SERVICE_NAME",
				Value: "datadog-admission-controller",
			},
		}...,
	)

	admissionControllerDatadogAgent := test.NewDefaultedDatadogAgent("bar", "foo",
		&test.NewDatadogAgentOptions{
			UseEDS:                     true,
			ClusterAgentEnabled:        true,
			AdmissionControllerEnabled: true,
		})

	admissionControllerPodSpecCustom := clusterAgentDefaultPodSpec()
	admissionControllerPodSpecCustom.Containers[0].Env = append(admissionControllerPodSpecCustom.Containers[0].Env,
		[]corev1.EnvVar{
			{
				Name:  "DD_ADMISSION_CONTROLLER_ENABLED",
				Value: "true",
			},
			{
				Name:  "DD_ADMISSION_CONTROLLER_MUTATE_UNLABELLED",
				Value: "true",
			},
			{
				Name:  "DD_ADMISSION_CONTROLLER_SERVICE_NAME",
				Value: "custom-service-name",
			},
		}...,
	)

	admissionControllerDatadogAgentCustom := test.NewDefaultedDatadogAgent("bar", "foo",
		&test.NewDatadogAgentOptions{
			UseEDS:                     true,
			ClusterAgentEnabled:        true,
			AdmissionControllerEnabled: true,
			AdmissionMutateUnlabelled:  true,
			AdmissionServiceName:       "custom-service-name",
		})

	tests := clusterAgentDeploymentFromInstanceTestSuite{
		{
			name:            "with admission controller",
			agentdeployment: admissionControllerDatadogAgent,
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "datadog-monitoring",
				},
			},
			newStatus: &datadoghqv1alpha1.DatadogAgentStatus{},
			wantErr:   false,
			want: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "bar",
					Name:      "foo-cluster-agent",
					Labels:    commonLabels,
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: commonLabels,
						},
						Spec: admissionControllerPodSpec,
					},
					Replicas: &testClusterAgentReplicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "datadog-monitoring",
						},
					},
				},
			},
		},
		{
			name:            "with custom admission controller config",
			agentdeployment: admissionControllerDatadogAgentCustom,
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "datadog-monitoring",
				},
			},
			newStatus: &datadoghqv1alpha1.DatadogAgentStatus{},
			wantErr:   false,
			want: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "bar",
					Name:      "foo-cluster-agent",
					Labels:    commonLabels,
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: commonLabels,
						},
						Spec: admissionControllerPodSpecCustom,
					},
					Replicas: &testClusterAgentReplicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "datadog-monitoring",
						},
					},
				},
			},
		},
	}
	tests.Run(t)
}

func Test_newClusterAgentDeploymentFromInstance_UserProvidedSecret(t *testing.T) {
	podSpec := clusterAgentDefaultPodSpec()
	for _, e := range podSpec.Containers[0].Env {
		if e.Name == "DD_API_KEY" {
			e.ValueFrom.SecretKeyRef.LocalObjectReference.Name = "my_secret"
		}
	}

	tests := clusterAgentDeploymentFromInstanceTestSuite{
		{
			name: "user provided secret for API key",
			agentdeployment: test.NewDefaultedDatadogAgent(
				"bar",
				"foo",
				&test.NewDatadogAgentOptions{
					ClusterAgentEnabled: true,
					APISecret: &datadoghqv1alpha1.Secret{
						SecretName: "my_secret",
					},
				},
			),
			newStatus: &datadoghqv1alpha1.DatadogAgentStatus{},
			wantErr:   false,
			want: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "bar",
					Name:      "foo-cluster-agent",
					Labels: map[string]string{"agent.datadoghq.com/name": "foo",
						"agent.datadoghq.com/component": "cluster-agent",
						"app.kubernetes.io/instance":    "cluster-agent",
						"app.kubernetes.io/managed-by":  "datadog-operator",
						"app.kubernetes.io/name":        "datadog-agent-deployment",
						"app.kubernetes.io/part-of":     "foo",
						"app.kubernetes.io/version":     "",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"agent.datadoghq.com/name":      "foo",
								"agent.datadoghq.com/component": "cluster-agent",
								"app.kubernetes.io/instance":    "cluster-agent",
								"app.kubernetes.io/managed-by":  "datadog-operator",
								"app.kubernetes.io/name":        "datadog-agent-deployment",
								"app.kubernetes.io/part-of":     "foo",
								"app.kubernetes.io/version":     "",
							},
						},
						Spec: podSpec,
					},
					Replicas: &testClusterAgentReplicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "cluster-agent",
						},
					},
				},
			},
		},
		{
			name: "user provided secret for API key",
			agentdeployment: test.NewDefaultedDatadogAgent(
				"bar",
				"foo",
				&test.NewDatadogAgentOptions{
					ClusterAgentEnabled:  true,
					APIKeyExistingSecret: "my_secret",
				},
			),
			newStatus: &datadoghqv1alpha1.DatadogAgentStatus{},
			wantErr:   false,
			want: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "bar",
					Name:      "foo-cluster-agent",
					Labels: map[string]string{"agent.datadoghq.com/name": "foo",
						"agent.datadoghq.com/component": "cluster-agent",
						"app.kubernetes.io/instance":    "cluster-agent",
						"app.kubernetes.io/managed-by":  "datadog-operator",
						"app.kubernetes.io/name":        "datadog-agent-deployment",
						"app.kubernetes.io/part-of":     "foo",
						"app.kubernetes.io/version":     "",
					},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"agent.datadoghq.com/name":      "foo",
								"agent.datadoghq.com/component": "cluster-agent",
								"app.kubernetes.io/instance":    "cluster-agent",
								"app.kubernetes.io/managed-by":  "datadog-operator",
								"app.kubernetes.io/name":        "datadog-agent-deployment",
								"app.kubernetes.io/part-of":     "foo",
								"app.kubernetes.io/version":     "",
							},
						},
						Spec: podSpec,
					},
					Replicas: &testClusterAgentReplicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "cluster-agent",
						},
					},
				},
			},
		},
	}
	tests.Run(t)
}

func Test_newClusterAgentDeploymentFromInstance_Compliance(t *testing.T) {

	podSpec := clusterAgentDefaultPodSpec()
	podSpec.Containers[0].Env = addEnvVar(podSpec.Containers[0].Env, "DD_COMPLIANCE_CONFIG_ENABLED", "true")

	agentDeployment := test.NewDefaultedDatadogAgent(
		"bar",
		"foo",
		&test.NewDatadogAgentOptions{
			ClusterAgentEnabled: true,
			ComplianceEnabled:   true,
		},
	)

	test := clusterAgentDeploymentFromInstanceTest{
		name:            "with compliance",
		agentdeployment: agentDeployment,
		newStatus:       &datadoghqv1alpha1.DatadogAgentStatus{},
		wantErr:         false,
		want: &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "bar",
				Name:      "foo-cluster-agent",
				Labels: map[string]string{"agent.datadoghq.com/name": "foo",
					"agent.datadoghq.com/component": "cluster-agent",
					"app.kubernetes.io/instance":    "cluster-agent",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "foo",
					"app.kubernetes.io/version":     "",
				},
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "cluster-agent",
							"app.kubernetes.io/instance":    "cluster-agent",
							"app.kubernetes.io/managed-by":  "datadog-operator",
							"app.kubernetes.io/name":        "datadog-agent-deployment",
							"app.kubernetes.io/part-of":     "foo",
							"app.kubernetes.io/version":     "",
						},
					},
					Spec: podSpec,
				},
				Replicas: &testClusterAgentReplicas,
				Selector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"agent.datadoghq.com/name":      "foo",
						"agent.datadoghq.com/component": "cluster-agent",
					},
				},
			},
		},
	}
	test.Run(t)
}

func TestReconcileDatadogAgent_createNewClusterAgentDeployment(t *testing.T) {
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconcileDatadogAgent_createNewClusterAgentDeployment"})
	forwarders := dummyManager{}

	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	localLog := logf.Log.WithName("TestReconcileDatadogAgent_createNewClusterAgentDeployment")

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogAgent{})

	type fields struct {
		client   client.Client
		scheme   *runtime.Scheme
		recorder record.EventRecorder
	}
	type args struct {
		logger          logr.Logger
		agentdeployment *datadoghqv1alpha1.DatadogAgent
		newStatus       *datadoghqv1alpha1.DatadogAgentStatus
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    reconcile.Result
		wantErr bool
	}{
		{
			name: "create new DCA",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				logger:          localLog,
				agentdeployment: test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{ClusterAgentEnabled: true}),
				newStatus:       &datadoghqv1alpha1.DatadogAgentStatus{},
			},
			want:    reconcile.Result{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{
				client:     tt.fields.client,
				scheme:     tt.fields.scheme,
				recorder:   recorder,
				forwarders: forwarders,
			}
			got, err := r.createNewClusterAgentDeployment(tt.args.logger, tt.args.agentdeployment, tt.args.newStatus)
			if tt.wantErr {
				assert.Error(t, err, "ReconcileDatadogAgent.createNewClusterAgentDeployment() should return an error")
			} else {
				assert.NoError(t, err, "ReconcileDatadogAgent.createNewClusterAgentDeployment() unexpected error: %v", err)
			}
			assert.Equal(t, tt.want, got, "ReconcileDatadogAgent.createNewClusterAgentDeployment() unexpected result")
		})
	}
}
