// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	byocdefaults "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/defaults"
	byocimage "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/image"
)

func TestBuildObservabilityPipelinesWorker(t *testing.T) {
	digest := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	defaultImage := byocimage.ResolvedImage{
		Repository: "registry.example.com/observability-pipelines-worker",
		Tag:        "2.10.0",
	}
	wantDefaultWorker := func() *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker {
		return &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{
			ObjectMeta: metav1.ObjectMeta{Name: "byoc-pipeline", Namespace: "testing"},
			Spec: datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec{
				DatadogBYOCClusterPipelineComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{
					DatadogBYOCClusterStatefulComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
						DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
							Replicas: ptr.To[int32](2),
							Env: []corev1.EnvVar{
								{Name: pipelineDestinationEndpointEnvName, Value: "http://byoc-indexer:7280"},
								{Name: pipelineSourceOTLPGRPCAddressEnvName, Value: "0.0.0.0:4317"},
								{Name: pipelineSourceOTLPHTTPAddressEnvName, Value: "0.0.0.0:4318"},
							},
							Resources: &corev1.ResourceRequirements{
								Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")},
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("4Gi"),
								},
							},
							Labels: map[string]string{
								"app.kubernetes.io/name":       "cloudprem",
								"app.kubernetes.io/instance":   "byoc",
								"app.kubernetes.io/component":  PipelineComponentName,
								"app.kubernetes.io/managed-by": "datadog-operator",
							},
							Annotations: map[string]string{},
							PodDisruptionBudget: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{
								MaxUnavailable: ptr.To(intstr.FromInt32(1)),
							},
							TerminationGracePeriodSeconds: ptr.To[int64](70),
						},
						Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{
							VolumeClaimTemplate: &datadoghqv1alpha1.DatadogBYOCClusterEmbeddedPersistentVolumeClaim{
								Spec: corev1.PersistentVolumeClaimSpec{
									AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
									Resources: corev1.VolumeResourceRequirements{
										Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("30Gi")},
									},
								},
							},
						},
					},
					PipelineID: ptr.To("existing-pipeline"),
				},
				Datadog: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerDatadogSpec{
					Site: ptr.To("datadoghq.eu"),
					APIKeySecretRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "datadog-secret"},
						Key:                  "api-key",
					},
				},
				Image: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerImageSpec{
					Repository: ptr.To("registry.example.com/observability-pipelines-worker"),
					Tag:        ptr.To("2.10.0"),
					PullPolicy: ptr.To(corev1.PullIfNotPresent),
				},
				Ports: []datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerPort{
					{Name: "otlp-grpc", Port: 4317, Protocol: corev1.ProtocolTCP},
					{Name: "otlp-http", Port: 4318, Protocol: corev1.ProtocolTCP},
				},
			},
		}
	}

	tests := []struct {
		name       string
		nodeConfig *runtime.RawExtension
		global     datadoghqv1alpha1.DatadogBYOCClusterGlobalSpec
		pipeline   *datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec
		image      byocimage.ResolvedImage
		want       *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker
		wantErr    string
	}{
		{
			name:     "pipeline with defaults",
			pipeline: &datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{PipelineID: ptr.To("existing-pipeline")},
			image:    defaultImage,
			want:     wantDefaultWorker(),
		},
		{
			name:       "REST TLS uses HTTPS",
			nodeConfig: &runtime.RawExtension{Raw: []byte(`{"rest":{"tls":{"cert_path":"/etc/tls/tls.crt","key_path":"/etc/tls/tls.key"}}}`)},
			pipeline:   &datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{PipelineID: ptr.To("existing-pipeline")},
			image:      defaultImage,
			global: datadoghqv1alpha1.DatadogBYOCClusterGlobalSpec{
				Env: []corev1.EnvVar{{Name: pipelineDestinationEndpointEnvName, Value: "http://global-override:7280"}},
			},
			want: func() *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker {
				want := wantDefaultWorker()
				want.Spec.Env[0].Value = "https://byoc-indexer:7280"
				return want
			}(),
		},
		{
			name: "gRPC TLS only",
			nodeConfig: &runtime.RawExtension{Raw: []byte(`grpc:
  tls:
    cert_path: /etc/tls/tls.crt
    key_path: /etc/tls/tls.key
`)},
			pipeline: &datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{PipelineID: ptr.To("existing-pipeline")},
			image:    defaultImage,
			want:     wantDefaultWorker(),
		},
		{
			name: "existing pipeline with resolved image configuration",
			pipeline: &datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{
				PipelineID: ptr.To("existing-pipeline"),
			},
			image: byocimage.ResolvedImage{
				Repository:       "registry.example.com/observability-pipelines-worker",
				Tag:              "ignored-tag",
				Digest:           digest,
				ImagePullPolicy:  corev1.PullAlways,
				ImagePullSecrets: []corev1.LocalObjectReference{{Name: "registry-credentials"}},
			},
			want: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{
				ObjectMeta: metav1.ObjectMeta{Name: "byoc-pipeline", Namespace: "testing"},
				Spec: datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec{
					DatadogBYOCClusterPipelineComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{
						DatadogBYOCClusterStatefulComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
							DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
								Replicas: ptr.To[int32](2),
								Env: []corev1.EnvVar{
									{Name: pipelineDestinationEndpointEnvName, Value: "http://byoc-indexer:7280"},
									{Name: pipelineSourceOTLPGRPCAddressEnvName, Value: "0.0.0.0:4317"},
									{Name: pipelineSourceOTLPHTTPAddressEnvName, Value: "0.0.0.0:4318"},
								},
								Resources: &corev1.ResourceRequirements{
									Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")},
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("2"),
										corev1.ResourceMemory: resource.MustParse("4Gi"),
									},
								},
								Labels: map[string]string{
									"app.kubernetes.io/name":       "cloudprem",
									"app.kubernetes.io/instance":   "byoc",
									"app.kubernetes.io/component":  PipelineComponentName,
									"app.kubernetes.io/managed-by": "datadog-operator",
								},
								Annotations: map[string]string{},
								PodDisruptionBudget: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{
									MaxUnavailable: ptr.To(intstr.FromInt32(1)),
								},
								TerminationGracePeriodSeconds: ptr.To[int64](70),
							},
							Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{
								VolumeClaimTemplate: &datadoghqv1alpha1.DatadogBYOCClusterEmbeddedPersistentVolumeClaim{
									Spec: corev1.PersistentVolumeClaimSpec{
										AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
										Resources: corev1.VolumeResourceRequirements{
											Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("30Gi")},
										},
									},
								},
							},
						},
						PipelineID: ptr.To("existing-pipeline"),
					},
					Datadog: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerDatadogSpec{
						Site: ptr.To("datadoghq.eu"),
						APIKeySecretRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "datadog-secret"},
							Key:                  "api-key",
						},
					},
					Image: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerImageSpec{
						Repository:       ptr.To("registry.example.com/observability-pipelines-worker"),
						Digest:           ptr.To(digest),
						PullPolicy:       ptr.To(corev1.PullAlways),
						ImagePullSecrets: []corev1.LocalObjectReference{{Name: "registry-credentials"}},
					},
					Ports: []datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerPort{
						{Name: "otlp-grpc", Port: 4317, Protocol: corev1.ProtocolTCP},
						{Name: "otlp-http", Port: 4318, Protocol: corev1.ProtocolTCP},
					},
				},
			},
		},
		{
			name: "component settings override global settings",
			global: datadoghqv1alpha1.DatadogBYOCClusterGlobalSpec{
				Labels:      map[string]string{"team": "global"},
				Annotations: map[string]string{"example.com/owner": "global"},
				Env: []corev1.EnvVar{
					{Name: "SHARED_SETTING", Value: "global"},
					{Name: "GLOBAL_SETTING", Value: "global"},
				},
				Volumes: []corev1.Volume{
					{Name: "shared", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
					{Name: "global", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
				},
				VolumeMounts: []corev1.VolumeMount{
					{Name: "shared-global", MountPath: "/shared"},
					{Name: "global", MountPath: "/global"},
				},
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
					{TopologyKey: "zone", WhenUnsatisfiable: corev1.DoNotSchedule, MaxSkew: 2},
				},
			},
			pipeline: &datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{
				DatadogBYOCClusterStatefulComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
					DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
						Labels:      map[string]string{"team": "pipeline"},
						Annotations: map[string]string{"example.com/owner": "pipeline"},
						Env: []corev1.EnvVar{
							{Name: "SHARED_SETTING", Value: "pipeline"},
							{Name: pipelineDestinationEndpointEnvName, Value: "replaced"},
							{Name: pipelineSourceOTLPGRPCAddressEnvName, Value: "replaced"},
							{Name: pipelineSourceOTLPHTTPAddressEnvName, Value: "replaced"},
						},
						Volumes: []corev1.Volume{
							{Name: "shared", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "pipeline"}}},
						},
						VolumeMounts: []corev1.VolumeMount{
							{Name: "shared-pipeline", MountPath: "/shared"},
						},
						TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
							{TopologyKey: "zone", WhenUnsatisfiable: corev1.DoNotSchedule, MaxSkew: 1},
						},
					},
				},
			},
			image: defaultImage,
			want: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{
				ObjectMeta: metav1.ObjectMeta{Name: "byoc-pipeline", Namespace: "testing"},
				Spec: datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec{
					DatadogBYOCClusterPipelineComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{
						DatadogBYOCClusterStatefulComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
							DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
								Replicas: ptr.To[int32](2),
								Env: []corev1.EnvVar{
									{Name: "SHARED_SETTING", Value: "pipeline"},
									{Name: "GLOBAL_SETTING", Value: "global"},
									{Name: pipelineDestinationEndpointEnvName, Value: "http://byoc-indexer:7280"},
									{Name: pipelineSourceOTLPGRPCAddressEnvName, Value: "0.0.0.0:4317"},
									{Name: pipelineSourceOTLPHTTPAddressEnvName, Value: "0.0.0.0:4318"},
								},
								Volumes: []corev1.Volume{
									{Name: "shared", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "pipeline"}}},
									{Name: "global", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
								},
								VolumeMounts: []corev1.VolumeMount{
									{Name: "shared-pipeline", MountPath: "/shared"},
									{Name: "global", MountPath: "/global"},
								},
								Resources: &corev1.ResourceRequirements{
									Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")},
									Requests: corev1.ResourceList{
										corev1.ResourceCPU:    resource.MustParse("2"),
										corev1.ResourceMemory: resource.MustParse("4Gi"),
									},
								},
								Labels: map[string]string{
									"app.kubernetes.io/name":       "cloudprem",
									"app.kubernetes.io/instance":   "byoc",
									"app.kubernetes.io/component":  PipelineComponentName,
									"app.kubernetes.io/managed-by": "datadog-operator",
									"team":                         "pipeline",
								},
								Annotations: map[string]string{"example.com/owner": "pipeline"},
								PodDisruptionBudget: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{
									MaxUnavailable: ptr.To(intstr.FromInt32(1)),
								},
								TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
									{TopologyKey: "zone", WhenUnsatisfiable: corev1.DoNotSchedule, MaxSkew: 1},
								},
								TerminationGracePeriodSeconds: ptr.To[int64](70),
							},
							Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{
								VolumeClaimTemplate: &datadoghqv1alpha1.DatadogBYOCClusterEmbeddedPersistentVolumeClaim{
									Spec: corev1.PersistentVolumeClaimSpec{
										AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
										Resources: corev1.VolumeResourceRequirements{
											Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("30Gi")},
										},
									},
								},
							},
						},
					},
					Datadog: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerDatadogSpec{
						Site: ptr.To("datadoghq.eu"),
						APIKeySecretRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "datadog-secret"},
							Key:                  "api-key",
						},
					},
					Image: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerImageSpec{
						Repository: ptr.To("registry.example.com/observability-pipelines-worker"),
						Tag:        ptr.To("2.10.0"),
						PullPolicy: ptr.To(corev1.PullIfNotPresent),
					},
					Ports: []datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerPort{
						{Name: "otlp-grpc", Port: 4317, Protocol: corev1.ProtocolTCP},
						{Name: "otlp-http", Port: 4318, Protocol: corev1.ProtocolTCP},
					},
				},
			},
		},
		{
			name:       "invalid node config",
			nodeConfig: &runtime.RawExtension{Raw: []byte("rest: [")},
			pipeline:   &datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{PipelineID: ptr.To("existing-pipeline")},
			image:      defaultImage,
			wantErr:    "decode spec.nodeConfig:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := byocdefaults.Apply(&datadoghqv1alpha1.DatadogBYOCCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "byoc", Namespace: "testing"},
				Spec: datadoghqv1alpha1.DatadogBYOCClusterSpec{
					Datadog: &datadoghqv1alpha1.DatadogBYOCClusterDatadogSpec{
						Site: ptr.To("datadoghq.eu"),
						APIKeySecretRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "datadog-secret"},
							Key:                  "api-key",
						},
					},
					NodeConfig: tt.nodeConfig,
					Global:     tt.global,
					Components: &datadoghqv1alpha1.DatadogBYOCClusterComponentsSpec{
						Metastore:    &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{},
						Indexer:      &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{},
						Searcher:     &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{},
						Pipeline:     tt.pipeline,
						ControlPlane: &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
						Janitor:      &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
					},
				},
			})

			got, err := BuildObservabilityPipelinesWorker(cluster, tt.image)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("BuildObservabilityPipelinesWorker() error = %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("BuildObservabilityPipelinesWorker() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("BuildObservabilityPipelinesWorker() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
