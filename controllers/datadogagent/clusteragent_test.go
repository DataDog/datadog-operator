package datadogagent

import (
	"fmt"
	"reflect"
	"strconv"
	"testing"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1/test"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/orchestrator"
	"github.com/DataDog/datadog-operator/pkg/defaulting"
	"github.com/DataDog/datadog-operator/pkg/testutils"

	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"

	"github.com/go-logr/logr"
	assert "github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func clusterAgentDefaultPodSpec() corev1.PodSpec {
	return corev1.PodSpec{
		Affinity:           getClusterAgentAffinity(nil),
		ServiceAccountName: "foo-cluster-agent",
		Containers: []corev1.Container{
			{
				Name:            "cluster-agent",
				Image:           defaulting.GetLatestClusterAgentImage(),
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
					{Name: "orchestrator-explorer-config", ReadOnly: true, MountPath: "/etc/datadog-agent/conf.d/orchestrator.d"},
					{Name: "logdatadog", ReadOnly: false, MountPath: "/var/log/datadog"},
					{Name: "tmp", ReadOnly: false, MountPath: "/tmp"},
					{Name: "certificates", ReadOnly: false, MountPath: "/etc/datadog-agent/certificates"},
				},
				LivenessProbe:  defaultLivenessProbe(),
				ReadinessProbe: defaultReadinessProbe(),
				SecurityContext: &corev1.SecurityContext{
					ReadOnlyRootFilesystem:   apiutils.NewBoolPointer(true),
					AllowPrivilegeEscalation: apiutils.NewBoolPointer(false),
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
				VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
			},
			{
				Name: "orchestrator-explorer-config",
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "foo-orchestrator-explorer-config",
						},
					},
				},
			},
			{
				Name: "logdatadog",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
			{
				Name: "tmp",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
			{
				Name: "certificates",
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		},
		// To be uncommented when the cluster-agent Dockerfile will be updated to use a non-root user by default
		// SecurityContext: &corev1.PodSecurityContext{
		// 	RunAsNonRoot: apiutils.NewBoolPointer(true),
		// },
	}
}

func clusterAgentPodSpectWithConfd(configDirSpec *datadoghqv1alpha1.ConfigDirSpec) corev1.PodSpec {
	spec := clusterAgentDefaultPodSpec()

	if configDirSpec != nil {
		builder := NewVolumeBuilder(spec.Volumes, nil)
		builder.Add(&corev1.Volume{
			Name: "confd",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configDirSpec.ConfigMapName,
					},
					Items: configDirSpec.Items,
				},
			},
		})
		spec.Volumes = builder.Build()
	}

	return spec
}

func clusterAgentDefaultEnvVars() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name: "DD_POD_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		{
			Name:  "DD_CLUSTER_CHECKS_ENABLED",
			Value: "false",
		},
		{
			Name:  "DD_CLUSTER_AGENT_KUBERNETES_SERVICE_NAME",
			Value: fmt.Sprintf("%s-%s", testDdaName, apicommon.DefaultClusterAgentResourceSuffix),
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
			Name:  "DD_LEADER_LEASE_NAME",
			Value: fmt.Sprintf("%s-leader-election", testDdaName),
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
			Name:  "DD_HEALTH_PORT",
			Value: "5555",
		},
		{
			Name:  "DD_LOG_LEVEL",
			Value: "INFO",
		},
		{
			Name:      "DD_API_KEY",
			ValueFrom: apiKeyValue(),
		},
		{
			Name:  "DD_ORCHESTRATOR_EXPLORER_ENABLED",
			Value: "true",
		},
		{
			Name:  "DD_ORCHESTRATOR_EXPLORER_CONTAINER_SCRUBBING_ENABLED",
			Value: "true",
		},
		{
			Name:  "DD_CLUSTER_AGENT_TOKEN_NAME",
			Value: fmt.Sprintf("%stoken", testDdaName),
		},
		{
			Name:  "DD_KUBE_RESOURCES_NAMESPACE",
			Value: testDdaNamespace,
		},
	}
}

func clusterAgentWithAdmissionControllerDefaultEnvVars(webhookService, agentService string, mode *string, unlabelled bool) []corev1.EnvVar {
	builder := NewEnvVarsBuilder(clusterAgentDefaultEnvVars(), nil)
	builder.Add(&corev1.EnvVar{
		Name:  "DD_ADMISSION_CONTROLLER_ENABLED",
		Value: "true",
	})
	builder.Add(&corev1.EnvVar{
		Name:  "DD_ADMISSION_CONTROLLER_MUTATE_UNLABELLED",
		Value: apiutils.BoolToString(&unlabelled),
	})
	builder.Add(&corev1.EnvVar{
		Name:  "DD_ADMISSION_CONTROLLER_SERVICE_NAME",
		Value: webhookService,
	})
	if mode != nil {
		builder.Add(&corev1.EnvVar{
			Name:  "DD_ADMISSION_CONTROLLER_INJECT_CONFIG_MODE",
			Value: *mode,
		})
	}
	builder.Add(&corev1.EnvVar{
		Name:  "DD_ADMISSION_CONTROLLER_INJECT_CONFIG_LOCAL_SERVICE_NAME",
		Value: agentService,
	})
	builder.Add(&corev1.EnvVar{
		Name:  "DD_ADMISSION_CONTROLLER_WEBHOOK_NAME",
		Value: "datadog-webhook",
	})

	return builder.Build()
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
	features, _ := feature.BuildFeaturesV1(test.agentdeployment, &feature.Options{Logger: logger})
	got, _, err := newClusterAgentDeploymentFromInstance(logger, features, test.agentdeployment, test.selector)
	if test.wantErr {
		assert.Error(t, err, "newClusterAgentDeploymentFromInstance() expected an error")
	} else {
		assert.NoError(t, err, "newClusterAgentDeploymentFromInstance() unexpected error: %v", err)
		if test.want.Annotations == nil {
			test.want.Annotations = map[string]string{}
		}
		delete(got.Annotations, "agent.datadoghq.com/agentspechash")
	}

	diff := testutils.CompareKubeResource(got, test.want)
	assert.True(t, len(diff) == 0, diff)
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
					Labels: map[string]string{
						"agent.datadoghq.com/name":      "foo",
						"agent.datadoghq.com/component": "cluster-agent",
						"app.kubernetes.io/component":   "cluster-agent",
						"app.kubernetes.io/instance":    "foo-cluster-agent",
						"app.kubernetes.io/managed-by":  "datadog-operator",
						"app.kubernetes.io/name":        "datadog-agent-deployment",
						"app.kubernetes.io/part-of":     "bar-foo",
						"app.kubernetes.io/version":     "",
					},
					Annotations: map[string]string{},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"agent.datadoghq.com/name":      "foo",
								"agent.datadoghq.com/component": "cluster-agent",
								"app.kubernetes.io/component":   "cluster-agent",
								"app.kubernetes.io/instance":    "foo-cluster-agent",
								"app.kubernetes.io/managed-by":  "datadog-operator",
								"app.kubernetes.io/name":        "datadog-agent-deployment",
								"app.kubernetes.io/part-of":     "bar-foo",
								"app.kubernetes.io/version":     "",
							},
							Annotations: map[string]string{},
						},
						Spec: clusterAgentDefaultPodSpec(),
					},
					Replicas: nil,
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
			agentdeployment: test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{ClusterAgentEnabled: true}),
			newStatus:       &datadoghqv1alpha1.DatadogAgentStatus{},
			wantErr:         false,
			want: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "bar",
					Name:      "foo-cluster-agent",
					Labels: map[string]string{
						"agent.datadoghq.com/name":      "foo",
						"agent.datadoghq.com/component": "cluster-agent",
						"app.kubernetes.io/component":   "cluster-agent",
						"app.kubernetes.io/instance":    "foo-cluster-agent",
						"app.kubernetes.io/managed-by":  "datadog-operator",
						"app.kubernetes.io/name":        "datadog-agent-deployment",
						"app.kubernetes.io/part-of":     "bar-foo",
						"app.kubernetes.io/version":     "",
					},
					Annotations: map[string]string{},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"agent.datadoghq.com/name":      "foo",
								"agent.datadoghq.com/component": "cluster-agent",
								"app.kubernetes.io/component":   "cluster-agent",
								"app.kubernetes.io/instance":    "foo-cluster-agent",
								"app.kubernetes.io/managed-by":  "datadog-operator",
								"app.kubernetes.io/name":        "datadog-agent-deployment",
								"app.kubernetes.io/part-of":     "bar-foo",
								"app.kubernetes.io/version":     "",
							},
							Annotations: map[string]string{},
						},
						Spec: clusterAgentDefaultPodSpec(),
					},
					Replicas: nil,
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
						"app.kubernetes.io/component":   "cluster-agent",
						"app.kubernetes.io/instance":    "foo-cluster-agent",
						"app.kubernetes.io/managed-by":  "datadog-operator",
						"app.kubernetes.io/name":        "datadog-agent-deployment",
						"app.kubernetes.io/part-of":     "bar-foo",
						"app.kubernetes.io/version":     "",
					},
					Annotations: map[string]string{},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"agent.datadoghq.com/name":      "foo",
								"agent.datadoghq.com/component": "cluster-agent",
								"app.kubernetes.io/component":   "cluster-agent",
								"app.kubernetes.io/instance":    "foo-cluster-agent",
								"app.kubernetes.io/managed-by":  "datadog-operator",
								"app.kubernetes.io/name":        "datadog-agent-deployment",
								"app.kubernetes.io/part-of":     "bar-foo",
								"app.kubernetes.io/version":     "",
							},
							Annotations: map[string]string{},
						},
						Spec: clusterAgentDefaultPodSpec(),
					},
					Replicas: nil,
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
			name: "confd_configmap_with_nested_path",
			agentdeployment: test.NewDefaultedDatadogAgent("bar", "foo",
				&test.NewDatadogAgentOptions{
					ClusterAgentEnabled: true,
					ClusterAgentConfd: &datadoghqv1alpha1.ClusterAgentConfig{
						Confd: &datadoghqv1alpha1.ConfigDirSpec{
							ConfigMapName: "my-confd",
							Items: []corev1.KeyToPath{
								{
									Key:  "foo.d--foo.yaml",
									Path: "foo.d/foo.yaml",
								},
							},
						},
					},
				}),
			newStatus: &datadoghqv1alpha1.DatadogAgentStatus{},
			wantErr:   false,
			want: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "bar",
					Name:      "foo-cluster-agent",
					Labels: map[string]string{
						"agent.datadoghq.com/name":      "foo",
						"agent.datadoghq.com/component": "cluster-agent",
						"app.kubernetes.io/component":   "cluster-agent",
						"app.kubernetes.io/instance":    "foo-cluster-agent",
						"app.kubernetes.io/managed-by":  "datadog-operator",
						"app.kubernetes.io/name":        "datadog-agent-deployment",
						"app.kubernetes.io/part-of":     "bar-foo",
						"app.kubernetes.io/version":     "",
					},
					Annotations: map[string]string{},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"agent.datadoghq.com/name":      "foo",
								"agent.datadoghq.com/component": "cluster-agent",
								"app.kubernetes.io/component":   "cluster-agent",
								"app.kubernetes.io/instance":    "foo-cluster-agent",
								"app.kubernetes.io/managed-by":  "datadog-operator",
								"app.kubernetes.io/name":        "datadog-agent-deployment",
								"app.kubernetes.io/part-of":     "bar-foo",
								"app.kubernetes.io/version":     "",
							},
							Annotations: map[string]string{},
						},
						Spec: clusterAgentPodSpectWithConfd(
							&datadoghqv1alpha1.ConfigDirSpec{
								ConfigMapName: "my-confd",
								Items: []corev1.KeyToPath{
									{
										Key:  "foo.d--foo.yaml",
										Path: "foo.d/foo.yaml",
									},
								},
							},
						),
					},
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

func Test_newClusterAgentDeploymentMountKSMCore(t *testing.T) {
	enabledFeature := true
	// test proper mount of volume
	ksmCore := datadoghqv1alpha1.KubeStateMetricsCore{
		Enabled: &enabledFeature,
		Conf: &datadoghqv1alpha1.CustomConfigSpec{
			ConfigMap: &datadoghqv1alpha1.ConfigFileConfigMapSpec{
				Name:    "foo",
				FileKey: "ksm_core.yaml",
			},
		},
	}
	userVolumes := []corev1.Volume{
		{
			Name: "ksm-core-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "foo",
					},
					Items: []corev1.KeyToPath{{Key: "ksm_core.yaml", Path: "ksm_core.yaml"}},
				},
			},
		},
	}
	userVolumeMounts := []corev1.VolumeMount{
		{
			Name:      "ksm-core-config",
			MountPath: "/etc/datadog-agent/conf.d/kubernetes_state_core.d",
			ReadOnly:  true,
		},
	}
	clusterAgentPodSpec := clusterAgentDefaultPodSpec()
	clusterAgentPodSpec.Volumes = append(clusterAgentPodSpec.Volumes, userVolumes...)
	clusterAgentPodSpec.Containers[0].VolumeMounts = append(clusterAgentPodSpec.Containers[0].VolumeMounts, userVolumeMounts...)
	envVars := []corev1.EnvVar{
		{
			Name:  apicommon.DDKubeStateMetricsCoreEnabled,
			Value: "true",
		},
		{
			Name:  apicommon.DDKubeStateMetricsCoreConfigMap,
			Value: "foo",
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
				Labels: map[string]string{
					"agent.datadoghq.com/name":      "foo",
					"agent.datadoghq.com/component": "cluster-agent",
					"app.kubernetes.io/component":   "cluster-agent",
					"app.kubernetes.io/instance":    "foo-cluster-agent",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "bar-foo",
					"app.kubernetes.io/version":     "",
				},
				Annotations: map[string]string{},
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "cluster-agent",
							"app.kubernetes.io/component":   "cluster-agent",
							"app.kubernetes.io/instance":    "foo-cluster-agent",
							"app.kubernetes.io/managed-by":  "datadog-operator",
							"app.kubernetes.io/name":        "datadog-agent-deployment",
							"app.kubernetes.io/part-of":     "bar-foo",
							"app.kubernetes.io/version":     "",
						},
						Annotations: map[string]string{},
					},
					Spec: clusterAgentPodSpec,
				},
				Replicas: nil,
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
				OrchestratorExplorer: &datadoghqv1alpha1.OrchestratorExplorerConfig{Enabled: apiutils.NewBoolPointer(true)},
				PrometheusScrape:     &datadoghqv1alpha1.PrometheusScrapeConfig{Enabled: apiutils.NewBoolPointer(true), ServiceEndpoints: apiutils.NewBoolPointer(true)},
			},
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
				Labels: map[string]string{
					"agent.datadoghq.com/name":      "foo",
					"agent.datadoghq.com/component": "cluster-agent",
					"app.kubernetes.io/component":   "cluster-agent",
					"app.kubernetes.io/instance":    "foo-cluster-agent",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "bar-foo",
					"app.kubernetes.io/version":     "",
				},
				Annotations: map[string]string{},
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "cluster-agent",
							"app.kubernetes.io/component":   "cluster-agent",
							"app.kubernetes.io/instance":    "foo-cluster-agent",
							"app.kubernetes.io/managed-by":  "datadog-operator",
							"app.kubernetes.io/name":        "datadog-agent-deployment",
							"app.kubernetes.io/part-of":     "bar-foo",
							"app.kubernetes.io/version":     "",
						},
						Annotations: map[string]string{},
					},
					Spec: clusterAgentPodSpec,
				},
				Replicas: nil,
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
				Labels: map[string]string{
					"agent.datadoghq.com/name":      "foo",
					"agent.datadoghq.com/component": "cluster-agent",
					"app.kubernetes.io/component":   "cluster-agent",
					"app.kubernetes.io/instance":    "foo-cluster-agent",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "bar-foo",
					"app.kubernetes.io/version":     "",
				},
				Annotations: map[string]string{},
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "cluster-agent",
							"app.kubernetes.io/component":   "cluster-agent",
							"app.kubernetes.io/instance":    "foo-cluster-agent",
							"app.kubernetes.io/managed-by":  "datadog-operator",
							"app.kubernetes.io/name":        "datadog-agent-deployment",
							"app.kubernetes.io/part-of":     "bar-foo",
							"app.kubernetes.io/version":     "",
						},
						Annotations: map[string]string{},
					},
					Spec: userMountsPodSpec,
				},
				Replicas: nil,
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
				Labels: map[string]string{
					"agent.datadoghq.com/name":      "foo",
					"agent.datadoghq.com/component": "cluster-agent",
					"app.kubernetes.io/component":   "cluster-agent",
					"app.kubernetes.io/instance":    "foo-cluster-agent",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "bar-foo",
					"app.kubernetes.io/version":     "",
				},
				Annotations: map[string]string{},
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "cluster-agent",
							"app.kubernetes.io/component":   "cluster-agent",
							"app.kubernetes.io/instance":    "foo-cluster-agent",
							"app.kubernetes.io/managed-by":  "datadog-operator",
							"app.kubernetes.io/name":        "datadog-agent-deployment",
							"app.kubernetes.io/part-of":     "bar-foo",
							"app.kubernetes.io/version":     "",
						},
						Annotations: map[string]string{},
					},
					Spec: podSpec,
				},
				Replicas: nil,
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
	deploymentNamePodSpec.Affinity = getClusterAgentAffinity(nil)

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
				Labels: map[string]string{
					"agent.datadoghq.com/name":      "foo",
					"agent.datadoghq.com/component": "cluster-agent",
					"app.kubernetes.io/component":   "cluster-agent",
					"app.kubernetes.io/instance":    "custom-cluster-agent-deployment",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "bar-foo",
					"app.kubernetes.io/version":     "",
					"app":                           "datadog-monitoring",
				},
				Annotations: map[string]string{},
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "cluster-agent",
							"app.kubernetes.io/component":   "cluster-agent",
							"app.kubernetes.io/instance":    "custom-cluster-agent-deployment",
							"app.kubernetes.io/managed-by":  "datadog-operator",
							"app.kubernetes.io/name":        "datadog-agent-deployment",
							"app.kubernetes.io/part-of":     "bar-foo",
							"app.kubernetes.io/version":     "",
							"app":                           "datadog-monitoring",
						},
						Annotations: map[string]string{},
					},
					Spec: deploymentNamePodSpec,
				},
				Replicas: nil,
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
				Name:  apicommon.DDExternalMetricsProviderUseDatadogMetric,
				Value: "false",
			},
			{
				Name:  apicommon.DDExternalMetricsProviderWPAController,
				Value: "false",
			},
			{
				Name:  orchestrator.DDOrchestratorExplorerEnabled,
				Value: "true",
			},
			{
				Name:  orchestrator.DDOrchestratorExplorerContainerScrubbingEnabled,
				Value: "true",
			},
		}...,
	)

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
				Name:  apicommon.DDExternalMetricsProviderUseDatadogMetric,
				Value: "true",
			},
			{
				Name:  apicommon.DDExternalMetricsProviderWPAController,
				Value: "true",
			},
			{
				Name:  apicommon.DDExternalMetricsProviderEndpoint,
				Value: "https://app.datadoghq.eu",
			},
			{
				Name:      apicommon.DDExternalMetricsProviderAPIKey,
				ValueFrom: buildEnvVarFromSecret("foo-metrics-server", "api_key"),
			},
			{
				Name:      apicommon.DDExternalMetricsProviderAppKey,
				ValueFrom: buildEnvVarFromSecret("extmetrics-app-key-secret-name", "appkey"),
			},
			{
				Name:  orchestrator.DDOrchestratorExplorerEnabled,
				Value: "true",
			},
			{
				Name:  orchestrator.DDOrchestratorExplorerContainerScrubbingEnabled,
				Value: "true",
			},
		}...,
	)

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
				APPSecret: &commonv1.SecretConfig{
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
					Labels: map[string]string{
						"agent.datadoghq.com/name":      "foo",
						"agent.datadoghq.com/component": "cluster-agent",
						"app.kubernetes.io/component":   "cluster-agent",
						"app.kubernetes.io/instance":    "foo-cluster-agent",
						"app.kubernetes.io/managed-by":  "datadog-operator",
						"app.kubernetes.io/name":        "datadog-agent-deployment",
						"app.kubernetes.io/part-of":     "bar-foo",
						"app.kubernetes.io/version":     "",
						"app":                           "datadog-monitoring",
					},
					Annotations: map[string]string{},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"agent.datadoghq.com/name":      "foo",
								"agent.datadoghq.com/component": "cluster-agent",
								"app.kubernetes.io/component":   "cluster-agent",
								"app.kubernetes.io/instance":    "foo-cluster-agent",
								"app.kubernetes.io/managed-by":  "datadog-operator",
								"app.kubernetes.io/name":        "datadog-agent-deployment",
								"app.kubernetes.io/part-of":     "bar-foo",
								"app.kubernetes.io/version":     "",
								"app":                           "datadog-monitoring",
							},
							Annotations: map[string]string{},
						},
						Spec: metricsServerPodSpec,
					},
					Replicas: nil,
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
					Labels: map[string]string{
						"agent.datadoghq.com/name":      "foo",
						"agent.datadoghq.com/component": "cluster-agent",
						"app.kubernetes.io/component":   "cluster-agent",
						"app.kubernetes.io/instance":    "foo-cluster-agent",
						"app.kubernetes.io/managed-by":  "datadog-operator",
						"app.kubernetes.io/name":        "datadog-agent-deployment",
						"app.kubernetes.io/part-of":     "bar-foo",
						"app.kubernetes.io/version":     "",
						"app":                           "datadog-monitoring",
					},
					Annotations: map[string]string{},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"agent.datadoghq.com/name":      "foo",
								"agent.datadoghq.com/component": "cluster-agent",
								"app.kubernetes.io/component":   "cluster-agent",
								"app.kubernetes.io/instance":    "foo-cluster-agent",
								"app.kubernetes.io/managed-by":  "datadog-operator",
								"app.kubernetes.io/name":        "datadog-agent-deployment",
								"app.kubernetes.io/part-of":     "bar-foo",
								"app.kubernetes.io/version":     "",
								"app":                           "datadog-monitoring",
							},
							Annotations: map[string]string{},
						},
						Spec: metricsServerWithSitePodSpec,
					},
					Replicas: nil,
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
		"app.kubernetes.io/component":   "cluster-agent",
		"app.kubernetes.io/instance":    "foo-cluster-agent",
		"app.kubernetes.io/managed-by":  "datadog-operator",
		"app.kubernetes.io/name":        "datadog-agent-deployment",
		"app.kubernetes.io/part-of":     "bar-foo",
		"app.kubernetes.io/version":     "",
		"app":                           "datadog-monitoring",
	}

	admissionControllerDatadogAgent := test.NewDefaultedDatadogAgent("bar", "foo",
		&test.NewDatadogAgentOptions{
			UseEDS:                     true,
			ClusterAgentEnabled:        true,
			AdmissionControllerEnabled: true,
		})
	agentService := getAgentServiceName(admissionControllerDatadogAgent)
	admissionControllerPodSpec := clusterAgentDefaultPodSpec()
	admissionControllerPodSpec.Containers[0].Env = clusterAgentWithAdmissionControllerDefaultEnvVars("datadog-admission-controller", agentService, nil, false)

	admissionControllerDatadogAgentCustom := test.NewDefaultedDatadogAgent("bar", "foo",
		&test.NewDatadogAgentOptions{
			UseEDS:                     true,
			ClusterAgentEnabled:        true,
			AdmissionControllerEnabled: true,
			AdmissionMutateUnlabelled:  true,
			AdmissionServiceName:       "custom-service-name",
			AdmissionCommunicationMode: "service",
		})

	agentService = getAgentServiceName(admissionControllerDatadogAgentCustom)
	admissionControllerPodSpecCustom := clusterAgentDefaultPodSpec()
	admissionControllerPodSpecCustom.Containers[0].Env = clusterAgentWithAdmissionControllerDefaultEnvVars("custom-service-name", agentService, apiutils.NewStringPointer("service"), true)

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
							Labels:      commonLabels,
							Annotations: map[string]string{},
						},
						Spec: admissionControllerPodSpec,
					},
					Replicas: nil,
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
							Labels:      commonLabels,
							Annotations: map[string]string{},
						},
						Spec: admissionControllerPodSpecCustom,
					},
					Replicas: nil,
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
					APISecret: &commonv1.SecretConfig{
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
					Labels: map[string]string{
						"agent.datadoghq.com/name":      "foo",
						"agent.datadoghq.com/component": "cluster-agent",
						"app.kubernetes.io/component":   "cluster-agent",
						"app.kubernetes.io/instance":    "foo-cluster-agent",
						"app.kubernetes.io/managed-by":  "datadog-operator",
						"app.kubernetes.io/name":        "datadog-agent-deployment",
						"app.kubernetes.io/part-of":     "bar-foo",
						"app.kubernetes.io/version":     "",
					},
					Annotations: map[string]string{},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"agent.datadoghq.com/name":      "foo",
								"agent.datadoghq.com/component": "cluster-agent",
								"app.kubernetes.io/component":   "cluster-agent",
								"app.kubernetes.io/instance":    "foo-cluster-agent",
								"app.kubernetes.io/managed-by":  "datadog-operator",
								"app.kubernetes.io/name":        "datadog-agent-deployment",
								"app.kubernetes.io/part-of":     "bar-foo",
								"app.kubernetes.io/version":     "",
							},
							Annotations: map[string]string{},
						},
						Spec: podSpec,
					},
					Replicas: nil,
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
					Labels: map[string]string{
						"agent.datadoghq.com/name":      "foo",
						"agent.datadoghq.com/component": "cluster-agent",
						"app.kubernetes.io/component":   "cluster-agent",
						"app.kubernetes.io/instance":    "foo-cluster-agent",
						"app.kubernetes.io/managed-by":  "datadog-operator",
						"app.kubernetes.io/name":        "datadog-agent-deployment",
						"app.kubernetes.io/part-of":     "bar-foo",
						"app.kubernetes.io/version":     "",
					},
					Annotations: map[string]string{},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"agent.datadoghq.com/name":      "foo",
								"agent.datadoghq.com/component": "cluster-agent",
								"app.kubernetes.io/component":   "cluster-agent",
								"app.kubernetes.io/instance":    "foo-cluster-agent",
								"app.kubernetes.io/managed-by":  "datadog-operator",
								"app.kubernetes.io/name":        "datadog-agent-deployment",
								"app.kubernetes.io/part-of":     "bar-foo",
								"app.kubernetes.io/version":     "",
							},
							Annotations: map[string]string{},
						},
						Spec: podSpec,
					},
					Replicas: nil,
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
				Labels: map[string]string{
					"agent.datadoghq.com/name":      "foo",
					"agent.datadoghq.com/component": "cluster-agent",
					"app.kubernetes.io/component":   "cluster-agent",
					"app.kubernetes.io/instance":    "foo-cluster-agent",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "bar-foo",
					"app.kubernetes.io/version":     "",
				},
				Annotations: map[string]string{},
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "cluster-agent",
							"app.kubernetes.io/component":   "cluster-agent",
							"app.kubernetes.io/instance":    "foo-cluster-agent",
							"app.kubernetes.io/managed-by":  "datadog-operator",
							"app.kubernetes.io/name":        "datadog-agent-deployment",
							"app.kubernetes.io/part-of":     "bar-foo",
							"app.kubernetes.io/version":     "",
						},
						Annotations: map[string]string{},
					},
					Spec: podSpec,
				},
				Replicas: nil,
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

func Test_newClusterAgentDeploymentFromInstance_CustomReplicas(t *testing.T) {
	customReplicas := int32(7)
	deploymentNamePodSpec := clusterAgentDefaultPodSpec()
	deploymentNamePodSpec.Affinity = getClusterAgentAffinity(nil)

	deploymentNameAgentDeployment := test.NewDefaultedDatadogAgent("bar", "foo",
		&test.NewDatadogAgentOptions{
			UseEDS:               true,
			ClusterAgentEnabled:  true,
			ClusterAgentReplicas: &customReplicas,
		})

	test := clusterAgentDeploymentFromInstanceTest{
		name:            "with replicas",
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
				Name:      "foo-cluster-agent",
				Labels: map[string]string{
					"agent.datadoghq.com/name":      "foo",
					"agent.datadoghq.com/component": "cluster-agent",
					"app.kubernetes.io/component":   "cluster-agent",
					"app.kubernetes.io/instance":    "foo-cluster-agent",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "bar-foo",
					"app.kubernetes.io/version":     "",
					"app":                           "datadog-monitoring",
				},
				Annotations: map[string]string{},
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "cluster-agent",
							"app.kubernetes.io/component":   "cluster-agent",
							"app.kubernetes.io/instance":    "foo-cluster-agent",
							"app.kubernetes.io/managed-by":  "datadog-operator",
							"app.kubernetes.io/name":        "datadog-agent-deployment",
							"app.kubernetes.io/part-of":     "bar-foo",
							"app.kubernetes.io/version":     "",
							"app":                           "datadog-monitoring",
						},
						Annotations: map[string]string{},
					},
					Spec: deploymentNamePodSpec,
				},
				Replicas: &customReplicas,
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
		features        []feature.Feature
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
				client:   fake.NewClientBuilder().Build(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				logger:          localLog,
				agentdeployment: test.NewDefaultedDatadogAgent("bar", "foo", &test.NewDatadogAgentOptions{ClusterAgentEnabled: true}),
				features:        nil,
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
			got, err := r.createNewClusterAgentDeployment(tt.args.logger, nil, tt.args.agentdeployment, tt.args.newStatus)
			if tt.wantErr {
				assert.Error(t, err, "ReconcileDatadogAgent.createNewClusterAgentDeployment() should return an error")
			} else {
				assert.NoError(t, err, "ReconcileDatadogAgent.createNewClusterAgentDeployment() unexpected error: %v", err)
			}
			assert.Equal(t, tt.want, got, "ReconcileDatadogAgent.createNewClusterAgentDeployment() unexpected result")
		})
	}
}

func Test_PodAntiAffinity(t *testing.T) {
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
					PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
						{
							Weight: 50,
							PodAffinityTerm: corev1.PodAffinityTerm{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										apicommon.AgentDeploymentComponentLabelKey: apicommon.DefaultClusterAgentResourceSuffix,
									},
								},
								TopologyKey: "kubernetes.io/hostname",
							},
						},
					},
				},
			},
		},

		{
			name: "user-defined affinity",
			affinity: &corev1.Affinity{
				PodAntiAffinity: &corev1.PodAntiAffinity{
					PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
						{
							Weight: 50,
							PodAffinityTerm: corev1.PodAffinityTerm{
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
			want: &corev1.Affinity{
				PodAntiAffinity: &corev1.PodAntiAffinity{
					PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
						{
							Weight: 50,
							PodAffinityTerm: corev1.PodAffinityTerm{
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
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getClusterAgentAffinity(tt.affinity); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getClusterAgentAffinity() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_newClusterAgentDeploymentFromInstance_CustomSecurityContext(t *testing.T) {
	podSpec := clusterAgentDefaultPodSpec()
	podSpec.SecurityContext = &corev1.PodSecurityContext{
		RunAsGroup: apiutils.NewInt64Pointer(42),
	}

	agentDeployment := test.NewDefaultedDatadogAgent(
		"bar",
		"foo",
		&test.NewDatadogAgentOptions{
			ClusterAgentEnabled: true,
		},
	)
	agentDeployment.Spec.ClusterAgent.Config.SecurityContext = &corev1.PodSecurityContext{
		RunAsGroup: apiutils.NewInt64Pointer(42),
	}

	test := clusterAgentDeploymentFromInstanceTest{
		name:            "with custom security context",
		agentdeployment: agentDeployment,
		newStatus:       &datadoghqv1alpha1.DatadogAgentStatus{},
		wantErr:         false,
		want: &appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "bar",
				Name:      "foo-cluster-agent",
				Labels: map[string]string{
					"agent.datadoghq.com/name":      "foo",
					"agent.datadoghq.com/component": "cluster-agent",
					"app.kubernetes.io/component":   "cluster-agent",
					"app.kubernetes.io/instance":    "foo-cluster-agent",
					"app.kubernetes.io/managed-by":  "datadog-operator",
					"app.kubernetes.io/name":        "datadog-agent-deployment",
					"app.kubernetes.io/part-of":     "bar-foo",
					"app.kubernetes.io/version":     "",
				},
				Annotations: map[string]string{},
			},
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: map[string]string{
							"agent.datadoghq.com/name":      "foo",
							"agent.datadoghq.com/component": "cluster-agent",
							"app.kubernetes.io/component":   "cluster-agent",
							"app.kubernetes.io/instance":    "foo-cluster-agent",
							"app.kubernetes.io/managed-by":  "datadog-operator",
							"app.kubernetes.io/name":        "datadog-agent-deployment",
							"app.kubernetes.io/part-of":     "bar-foo",
							"app.kubernetes.io/version":     "",
						},
						Annotations: map[string]string{},
					},
					Spec: podSpec,
				},
				Replicas: nil,
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
