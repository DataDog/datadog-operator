// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func TestBuildResources_ServiceAccount(t *testing.T) {
	tests := []struct {
		name       string
		workerFunc func(*datadoghqv1alpha1.DatadogObservabilityPipelinesWorker)
		want       *corev1.ServiceAccount
	}{
		{
			name:       "dedicated ServiceAccount",
			workerFunc: func(*datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) {},
			want: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "byoc-pipeline",
					Namespace: "testing",
					Labels: map[string]string{
						"app.kubernetes.io/name":       "observability-pipelines-worker",
						"app.kubernetes.io/instance":   "byoc-pipeline",
						"app.kubernetes.io/managed-by": "datadog-operator",
						"app.kubernetes.io/component":  "pipeline",
						"team":                         "logs",
					},
					Annotations: map[string]string{"example.com/owner": "operator"},
				},
				AutomountServiceAccountToken: ptr.To(false),
			},
		},
		{
			name: "existing ServiceAccount",
			workerFunc: func(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) {
				worker.Spec.Identity = &datadoghqv1alpha1.DatadogBYOCClusterIdentitySpec{ServiceAccountName: ptr.To("existing-worker")}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worker := testWorker()
			tt.workerFunc(worker)
			resources, err := BuildResources(worker)
			if err != nil {
				t.Fatalf("BuildResources() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, resources.ServiceAccount); diff != "" {
				t.Errorf("ServiceAccount mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBuildResources_Services(t *testing.T) {
	tests := []struct {
		name               string
		workerFunc         func(*datadoghqv1alpha1.DatadogObservabilityPipelinesWorker)
		wantService        *corev1.Service
		wantContainerPorts []corev1.ContainerPort
	}{
		{
			name: "exposed ports",
			workerFunc: func(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) {
				worker.Spec.Service = &datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerServiceSpec{Type: corev1.ServiceTypeLoadBalancer}
			},
			wantService: &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "byoc-pipeline",
					Namespace: "testing",
					Labels: map[string]string{
						"app.kubernetes.io/name":       "observability-pipelines-worker",
						"app.kubernetes.io/instance":   "byoc-pipeline",
						"app.kubernetes.io/managed-by": "datadog-operator",
						"app.kubernetes.io/component":  "pipeline",
						"team":                         "logs",
					},
					Annotations: map[string]string{"example.com/owner": "operator"},
				},
				Spec: corev1.ServiceSpec{
					Type: corev1.ServiceTypeLoadBalancer,
					Selector: map[string]string{
						"app.kubernetes.io/name":     "observability-pipelines-worker",
						"app.kubernetes.io/instance": "byoc-pipeline",
					},
					Ports: []corev1.ServicePort{
						{Name: "otlp-grpc", Port: 4317, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt32(4317)},
						{Name: "otlp-http", Port: 4318, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromInt32(4318)},
					},
				},
			},
			wantContainerPorts: []corev1.ContainerPort{
				{Name: "otlp-grpc", ContainerPort: 4317, Protocol: corev1.ProtocolTCP},
				{Name: "otlp-http", ContainerPort: 4318, Protocol: corev1.ProtocolTCP},
				{Name: "api", ContainerPort: 8686, Protocol: corev1.ProtocolTCP},
			},
		},
		{
			name: "no exposed ports",
			workerFunc: func(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) {
				worker.Spec.Ports = nil
			},
			wantContainerPorts: []corev1.ContainerPort{{Name: "api", ContainerPort: 8686, Protocol: corev1.ProtocolTCP}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worker := testWorker()
			tt.workerFunc(worker)
			resources, err := BuildResources(worker)
			if err != nil {
				t.Fatalf("BuildResources() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.wantService, resources.Service); diff != "" {
				t.Errorf("Service mismatch (-want +got):\n%s", diff)
			}
			if diff := cmp.Diff(tt.wantContainerPorts, resources.StatefulSet.Spec.Template.Spec.Containers[0].Ports); diff != "" {
				t.Errorf("Container ports mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBuildResources_StatefulSet(t *testing.T) {
	pipelineID := "pipeline-id"
	tests := []struct {
		name       string
		workerFunc func(*datadoghqv1alpha1.DatadogObservabilityPipelinesWorker)
		want       *appsv1.StatefulSet
		wantError  string
	}{
		{
			name:       "persistent worker",
			workerFunc: func(*datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) {},
			want: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "byoc-pipeline",
					Namespace: "testing",
					Labels: map[string]string{
						"app.kubernetes.io/name":       "observability-pipelines-worker",
						"app.kubernetes.io/instance":   "byoc-pipeline",
						"app.kubernetes.io/managed-by": "datadog-operator",
						"app.kubernetes.io/component":  "pipeline",
						"team":                         "logs",
					},
					Annotations: map[string]string{"example.com/owner": "operator"},
				},
				Spec: appsv1.StatefulSetSpec{
					Replicas:            ptr.To[int32](2),
					ServiceName:         "byoc-pipeline",
					PodManagementPolicy: appsv1.ParallelPodManagement,
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
						"app.kubernetes.io/name":     "observability-pipelines-worker",
						"app.kubernetes.io/instance": "byoc-pipeline",
					}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/name":       "observability-pipelines-worker",
								"app.kubernetes.io/instance":   "byoc-pipeline",
								"app.kubernetes.io/managed-by": "datadog-operator",
								"app.kubernetes.io/component":  "pipeline",
								"team":                         "logs",
							},
							Annotations: map[string]string{"example.com/owner": "operator"},
						},
						Spec: corev1.PodSpec{
							ServiceAccountName: "byoc-pipeline",
							DNSPolicy:          corev1.DNSClusterFirst,
							ImagePullSecrets:   []corev1.LocalObjectReference{{Name: "registry-secret"}},
							InitContainers:     []corev1.Container{{Name: "init", Image: "init:latest"}},
							Containers: []corev1.Container{{
								Name:            "worker",
								Image:           "public.ecr.aws/datadog/observability-pipelines-worker:2.21.1",
								ImagePullPolicy: corev1.PullAlways,
								Args:            []string{"run"},
								Env: []corev1.EnvVar{
									{Name: "DD_OP_DESTINATION_CLOUDPREM_ENDPOINT_URL", Value: "http://byoc-indexer:7280"},
									{Name: "DD_OP_SOURCE_OTEL_GRPC_ADDRESS", Value: "0.0.0.0:4317"},
									{Name: "DD_OP_SOURCE_OTEL_HTTP_ADDRESS", Value: "0.0.0.0:4318"},
									{Name: "DD_SITE", Value: "datadoghq.com"},
									{Name: "DD_API_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "datadog-secret"}, Key: "api-key"}}},
									{Name: "DD_OP_PIPELINE_ID", Value: pipelineID},
									{Name: "DD_OP_DATA_DIR", Value: "/var/lib/observability-pipelines-worker"},
									{Name: "DD_OP_API_ENABLED", Value: "true"},
									{Name: "DD_OP_API_ADDRESS", Value: "0.0.0.0:8686"},
									{Name: "DD_OP_GRACEFUL_SHUTDOWN_LIMIT_SECS", Value: "60"},
								},
								EnvFrom: []corev1.EnvFromSource{{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "worker-env"}}}},
								Ports: []corev1.ContainerPort{
									{Name: "otlp-grpc", ContainerPort: 4317, Protocol: corev1.ProtocolTCP},
									{Name: "otlp-http", ContainerPort: 4318, Protocol: corev1.ProtocolTCP},
									{Name: "api", ContainerPort: 8686, Protocol: corev1.ProtocolTCP},
								},
								Resources: corev1.ResourceRequirements{
									Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")},
									Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2"), corev1.ResourceMemory: resource.MustParse("4Gi")},
								},
								VolumeMounts: []corev1.VolumeMount{{Name: "certificates", MountPath: "/etc/certificates", ReadOnly: true}, {Name: "data", MountPath: "/var/lib/observability-pipelines-worker"}},
								LivenessProbe: &corev1.Probe{
									ProbeHandler:        corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt32(8686)}},
									InitialDelaySeconds: 15,
									TimeoutSeconds:      15,
									PeriodSeconds:       10,
									SuccessThreshold:    1,
									FailureThreshold:    5,
								},
								ReadinessProbe: &corev1.Probe{
									ProbeHandler:        corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt32(8686)}},
									InitialDelaySeconds: 15,
									TimeoutSeconds:      15,
									PeriodSeconds:       10,
									SuccessThreshold:    1,
									FailureThreshold:    3,
								},
							}},
							Volumes:                       []corev1.Volume{{Name: "certificates", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "certificates"}}}},
							NodeSelector:                  map[string]string{"role": "pipeline"},
							Affinity:                      &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{}},
							Tolerations:                   []corev1.Toleration{{Key: "dedicated", Value: "pipeline", Effect: corev1.TaintEffectNoSchedule}},
							TopologySpreadConstraints:     []corev1.TopologySpreadConstraint{{MaxSkew: 1, TopologyKey: corev1.LabelTopologyZone, WhenUnsatisfiable: corev1.ScheduleAnyway}},
							TerminationGracePeriodSeconds: ptr.To[int64](70),
						},
					},
					VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{
						ObjectMeta: metav1.ObjectMeta{Name: "data", Labels: map[string]string{"storage": "buffer"}, Annotations: map[string]string{"resize": "enabled"}},
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							StorageClassName: ptr.To("gp3"),
							Resources:        corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")}},
						},
					}},
				},
			},
		},
		{
			name: "autoscaling emptyDir worker",
			workerFunc: func(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) {
				worker.Spec.Identity = &datadoghqv1alpha1.DatadogBYOCClusterIdentitySpec{ServiceAccountName: ptr.To("existing-worker")}
				worker.Spec.Image = &datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerImageSpec{
					Repository: ptr.To("registry.example.com/worker"),
					Digest:     ptr.To("sha256:deadbeef"),
				}
				worker.Spec.Storage = &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: ptr.To(resource.MustParse("1Gi"))}}
				worker.Spec.Autoscaling = &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{
					MinReplicas: ptr.To[int32](2),
					MaxReplicas: ptr.To[int32](10),
					Metrics: []autoscalingv2.MetricSpec{{
						Type: autoscalingv2.ResourceMetricSourceType,
						Resource: &autoscalingv2.ResourceMetricSource{
							Name:   corev1.ResourceCPU,
							Target: autoscalingv2.MetricTarget{Type: autoscalingv2.UtilizationMetricType, AverageUtilization: ptr.To[int32](70)},
						},
					}},
					Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{ScaleDown: &autoscalingv2.HPAScalingRules{StabilizationWindowSeconds: ptr.To[int32](300)}},
				}
				worker.Spec.TerminationGracePeriodSeconds = ptr.To[int64](15)
			},
			want: &appsv1.StatefulSet{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "byoc-pipeline",
					Namespace: "testing",
					Labels: map[string]string{
						"app.kubernetes.io/name":       "observability-pipelines-worker",
						"app.kubernetes.io/instance":   "byoc-pipeline",
						"app.kubernetes.io/managed-by": "datadog-operator",
						"app.kubernetes.io/component":  "pipeline",
						"team":                         "logs",
					},
					Annotations: map[string]string{"example.com/owner": "operator"},
				},
				Spec: appsv1.StatefulSetSpec{
					ServiceName:         "byoc-pipeline",
					PodManagementPolicy: appsv1.ParallelPodManagement,
					Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
						"app.kubernetes.io/name":     "observability-pipelines-worker",
						"app.kubernetes.io/instance": "byoc-pipeline",
					}},
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"app.kubernetes.io/name":       "observability-pipelines-worker",
								"app.kubernetes.io/instance":   "byoc-pipeline",
								"app.kubernetes.io/managed-by": "datadog-operator",
								"app.kubernetes.io/component":  "pipeline",
								"team":                         "logs",
							},
							Annotations: map[string]string{"example.com/owner": "operator"},
						},
						Spec: corev1.PodSpec{
							ServiceAccountName: "existing-worker",
							DNSPolicy:          corev1.DNSClusterFirst,
							InitContainers:     []corev1.Container{{Name: "init", Image: "init:latest"}},
							Containers: []corev1.Container{{
								Name:            "worker",
								Image:           "registry.example.com/worker@sha256:deadbeef",
								ImagePullPolicy: corev1.PullIfNotPresent,
								Args:            []string{"run"},
								Env: []corev1.EnvVar{
									{Name: "DD_OP_DESTINATION_CLOUDPREM_ENDPOINT_URL", Value: "http://byoc-indexer:7280"},
									{Name: "DD_OP_SOURCE_OTEL_GRPC_ADDRESS", Value: "0.0.0.0:4317"},
									{Name: "DD_OP_SOURCE_OTEL_HTTP_ADDRESS", Value: "0.0.0.0:4318"},
									{Name: "DD_SITE", Value: "datadoghq.com"},
									{Name: "DD_API_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "datadog-secret"}, Key: "api-key"}}},
									{Name: "DD_OP_PIPELINE_ID", Value: pipelineID},
									{Name: "DD_OP_DATA_DIR", Value: "/var/lib/observability-pipelines-worker"},
									{Name: "DD_OP_API_ENABLED", Value: "true"},
									{Name: "DD_OP_API_ADDRESS", Value: "0.0.0.0:8686"},
									{Name: "DD_OP_GRACEFUL_SHUTDOWN_LIMIT_SECS", Value: "10"},
								},
								EnvFrom: []corev1.EnvFromSource{{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "worker-env"}}}},
								Ports: []corev1.ContainerPort{
									{Name: "otlp-grpc", ContainerPort: 4317, Protocol: corev1.ProtocolTCP},
									{Name: "otlp-http", ContainerPort: 4318, Protocol: corev1.ProtocolTCP},
									{Name: "api", ContainerPort: 8686, Protocol: corev1.ProtocolTCP},
								},
								Resources: corev1.ResourceRequirements{
									Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")},
									Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2"), corev1.ResourceMemory: resource.MustParse("4Gi")},
								},
								VolumeMounts: []corev1.VolumeMount{{Name: "certificates", MountPath: "/etc/certificates", ReadOnly: true}, {Name: "data", MountPath: "/var/lib/observability-pipelines-worker"}},
								LivenessProbe: &corev1.Probe{
									ProbeHandler:        corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt32(8686)}},
									InitialDelaySeconds: 15,
									TimeoutSeconds:      15,
									PeriodSeconds:       10,
									SuccessThreshold:    1,
									FailureThreshold:    5,
								},
								ReadinessProbe: &corev1.Probe{
									ProbeHandler:        corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt32(8686)}},
									InitialDelaySeconds: 15,
									TimeoutSeconds:      15,
									PeriodSeconds:       10,
									SuccessThreshold:    1,
									FailureThreshold:    3,
								},
							}},
							Volumes: []corev1.Volume{
								{Name: "certificates", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "certificates"}}},
								{Name: "data", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{SizeLimit: ptr.To(resource.MustParse("1Gi"))}}},
							},
							NodeSelector:                  map[string]string{"role": "pipeline"},
							Affinity:                      &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{}},
							Tolerations:                   []corev1.Toleration{{Key: "dedicated", Value: "pipeline", Effect: corev1.TaintEffectNoSchedule}},
							TopologySpreadConstraints:     []corev1.TopologySpreadConstraint{{MaxSkew: 1, TopologyKey: corev1.LabelTopologyZone, WhenUnsatisfiable: corev1.ScheduleAnyway}},
							TerminationGracePeriodSeconds: ptr.To[int64](15),
						},
					},
				},
			},
		},
		{
			name: "missing image",
			workerFunc: func(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) {
				worker.Spec.Image = nil
			},
			wantError: "worker image is required",
		},
		{
			name: "tag and digest",
			workerFunc: func(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) {
				image := datadoghqv1alpha1.DatadogBYOCImageSpec(*worker.Spec.Image)
				image.Digest = ptr.To("sha256:deadbeef")
				worker.Spec.Image = (*datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerImageSpec)(&image)
			},
			wantError: "worker image must specify exactly one of tag or digest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worker := testWorker()
			tt.workerFunc(worker)
			resources, err := BuildResources(worker)
			if err != nil {
				if err.Error() != tt.wantError {
					t.Errorf("BuildResources() error = %q, want %q", err, tt.wantError)
				}
				return
			}
			if tt.wantError != "" {
				t.Fatalf("BuildResources() expected error %q", tt.wantError)
			}
			if diff := cmp.Diff(tt.want, resources.StatefulSet); diff != "" {
				t.Errorf("StatefulSet mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBuildResources_HPA(t *testing.T) {
	tests := []struct {
		name       string
		workerFunc func(*datadoghqv1alpha1.DatadogObservabilityPipelinesWorker)
		want       *autoscalingv2.HorizontalPodAutoscaler
		wantError  string
	}{
		{
			name:       "disabled",
			workerFunc: func(*datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) {},
		},
		{
			name: "configured",
			workerFunc: func(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) {
				worker.Spec.Autoscaling = &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{
					MinReplicas: ptr.To[int32](2),
					MaxReplicas: ptr.To[int32](10),
					Metrics: []autoscalingv2.MetricSpec{{
						Type: autoscalingv2.ResourceMetricSourceType,
						Resource: &autoscalingv2.ResourceMetricSource{
							Name:   corev1.ResourceCPU,
							Target: autoscalingv2.MetricTarget{Type: autoscalingv2.UtilizationMetricType, AverageUtilization: ptr.To[int32](70)},
						},
					}},
					Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{ScaleDown: &autoscalingv2.HPAScalingRules{StabilizationWindowSeconds: ptr.To[int32](300)}},
				}
			},
			want: &autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "byoc-pipeline",
					Namespace: "testing",
					Labels: map[string]string{
						"app.kubernetes.io/name":       "observability-pipelines-worker",
						"app.kubernetes.io/instance":   "byoc-pipeline",
						"app.kubernetes.io/managed-by": "datadog-operator",
						"app.kubernetes.io/component":  "pipeline",
						"team":                         "logs",
					},
					Annotations: map[string]string{"example.com/owner": "operator"},
				},
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{APIVersion: "apps/v1", Kind: "StatefulSet", Name: "byoc-pipeline"},
					MinReplicas:    ptr.To[int32](2),
					MaxReplicas:    10,
					Metrics: []autoscalingv2.MetricSpec{{
						Type: autoscalingv2.ResourceMetricSourceType,
						Resource: &autoscalingv2.ResourceMetricSource{
							Name: corev1.ResourceCPU,
							Target: autoscalingv2.MetricTarget{
								Type:               autoscalingv2.UtilizationMetricType,
								AverageUtilization: ptr.To[int32](70),
							},
						},
					}},
					Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
						ScaleDown: &autoscalingv2.HPAScalingRules{StabilizationWindowSeconds: ptr.To[int32](300)},
					},
				},
			},
		},
		{
			name: "missing maximum",
			workerFunc: func(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) {
				worker.Spec.Autoscaling = &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{}
			},
			wantError: "worker autoscaling maxReplicas is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worker := testWorker()
			tt.workerFunc(worker)
			resources, err := BuildResources(worker)
			if err != nil {
				if err.Error() != tt.wantError {
					t.Errorf("BuildResources() error = %q, want %q", err, tt.wantError)
				}
				return
			}
			if tt.wantError != "" {
				t.Fatalf("BuildResources() expected error %q", tt.wantError)
			}
			if diff := cmp.Diff(tt.want, resources.HPA); diff != "" {
				t.Errorf("HPA mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBuildResources_PodDisruptionBudget(t *testing.T) {
	minAvailable := intstr.FromString("50%")
	maxUnavailable := intstr.FromInt32(1)
	podDisruptionBudget := func(minAvailable, maxUnavailable *intstr.IntOrString) *policyv1.PodDisruptionBudget {
		return &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "byoc-pipeline",
				Namespace: "testing",
				Labels: map[string]string{
					"app.kubernetes.io/name":       "observability-pipelines-worker",
					"app.kubernetes.io/instance":   "byoc-pipeline",
					"app.kubernetes.io/managed-by": "datadog-operator",
					"app.kubernetes.io/component":  "pipeline",
					"team":                         "logs",
				},
				Annotations: map[string]string{"example.com/owner": "operator"},
			},
			Spec: policyv1.PodDisruptionBudgetSpec{
				MinAvailable:   minAvailable,
				MaxUnavailable: maxUnavailable,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{
					"app.kubernetes.io/name":     "observability-pipelines-worker",
					"app.kubernetes.io/instance": "byoc-pipeline",
				}},
			},
		}
	}
	tests := []struct {
		name       string
		workerFunc func(*datadoghqv1alpha1.DatadogObservabilityPipelinesWorker)
		want       *policyv1.PodDisruptionBudget
		wantError  string
	}{
		{
			name:       "Helm default",
			workerFunc: func(*datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) {},
		},
		{
			name: "configured",
			workerFunc: func(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) {
				worker.Spec.PodDisruptionBudget = &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{MinAvailable: &minAvailable}
			},
			want: podDisruptionBudget(&minAvailable, nil),
		},
		{
			name: "disabled",
			workerFunc: func(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) {
				worker.Spec.PodDisruptionBudget = &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{}
			},
		},
		{
			name: "invalid",
			workerFunc: func(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) {
				worker.Spec.PodDisruptionBudget = &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{MinAvailable: &minAvailable, MaxUnavailable: &maxUnavailable}
			},
			wantError: "worker pod disruption budget minAvailable and maxUnavailable are mutually exclusive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worker := testWorker()
			tt.workerFunc(worker)
			resources, err := BuildResources(worker)
			if err != nil {
				if err.Error() != tt.wantError {
					t.Errorf("BuildResources() error = %q, want %q", err, tt.wantError)
				}
				return
			}
			if tt.wantError != "" {
				t.Fatalf("BuildResources() expected error %q", tt.wantError)
			}
			if diff := cmp.Diff(tt.want, resources.PodDisruptionBudget); diff != "" {
				t.Errorf("PodDisruptionBudget mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func testWorker() *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker {
	image := datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerImageSpec{
		Repository:       ptr.To("public.ecr.aws/datadog/observability-pipelines-worker"),
		Tag:              ptr.To("2.21.1"),
		PullPolicy:       ptr.To(corev1.PullAlways),
		ImagePullSecrets: []corev1.LocalObjectReference{{Name: "registry-secret"}},
	}
	return &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{
		ObjectMeta: metav1.ObjectMeta{Name: "byoc-pipeline", Namespace: "testing"},
		Spec: datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec{
			DatadogBYOCClusterPipelineComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{
				PipelineID: ptr.To("pipeline-id"),
				DatadogBYOCClusterStatefulComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
					DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
						Replicas: ptr.To[int32](2),
						Env: []corev1.EnvVar{
							{Name: "DD_OP_DESTINATION_CLOUDPREM_ENDPOINT_URL", Value: "http://byoc-indexer:7280"},
							{Name: "DD_OP_SOURCE_OTEL_GRPC_ADDRESS", Value: "0.0.0.0:4317"},
							{Name: "DD_OP_SOURCE_OTEL_HTTP_ADDRESS", Value: "0.0.0.0:4318"},
							{Name: "DD_SITE", Value: "invalid.example.com"},
						},
						EnvFrom:                       []corev1.EnvFromSource{{ConfigMapRef: &corev1.ConfigMapEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "worker-env"}}}},
						Volumes:                       []corev1.Volume{{Name: "certificates", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "certificates"}}}},
						VolumeMounts:                  []corev1.VolumeMount{{Name: "certificates", MountPath: "/etc/certificates", ReadOnly: true}},
						Resources:                     &corev1.ResourceRequirements{Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")}, Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2"), corev1.ResourceMemory: resource.MustParse("4Gi")}},
						Annotations:                   map[string]string{"example.com/owner": "operator"},
						Labels:                        map[string]string{"app.kubernetes.io/name": "cloudprem", "app.kubernetes.io/instance": "byoc", "app.kubernetes.io/managed-by": "datadog-operator", "app.kubernetes.io/component": "pipeline", "team": "logs"},
						NodeSelector:                  map[string]string{"role": "pipeline"},
						Tolerations:                   []corev1.Toleration{{Key: "dedicated", Value: "pipeline", Effect: corev1.TaintEffectNoSchedule}},
						Affinity:                      &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{}},
						TopologySpreadConstraints:     []corev1.TopologySpreadConstraint{{MaxSkew: 1, TopologyKey: corev1.LabelTopologyZone, WhenUnsatisfiable: corev1.ScheduleAnyway}},
						InitContainers:                []corev1.Container{{Name: "init", Image: "init:latest"}},
						TerminationGracePeriodSeconds: ptr.To[int64](70),
					},
					Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{
						VolumeClaimTemplate: &datadoghqv1alpha1.DatadogBYOCClusterEmbeddedPersistentVolumeClaim{
							Metadata: datadoghqv1alpha1.DatadogBYOCClusterEmbeddedObjectMetadata{Labels: map[string]string{"storage": "buffer"}, Annotations: map[string]string{"resize": "enabled"}},
							Spec: corev1.PersistentVolumeClaimSpec{
								AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
								StorageClassName: ptr.To("gp3"),
								Resources:        corev1.VolumeResourceRequirements{Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("10Gi")}},
							},
						},
					},
				},
			},
			Datadog: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerDatadogSpec{
				Site:            ptr.To("datadoghq.com"),
				APIKeySecretRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "datadog-secret"}, Key: "api-key"},
			},
			Image: &image,
			Ports: []datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerPort{
				{Name: "otlp-grpc", Port: 4317, Protocol: corev1.ProtocolTCP},
				{Name: "otlp-http", Port: 4318, Protocol: corev1.ProtocolTCP},
			},
		},
	}
}
