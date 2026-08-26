// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	byocrelease "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/release"
)

func TestBuildResources_ConfigMap(t *testing.T) {
	tests := []struct {
		name       string
		nodeConfig *runtime.RawExtension
		want       *corev1.ConfigMap
	}{
		{
			name: "default",
			want: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "byoc",
					Namespace: "testing",
					Labels: map[string]string{
						"app.kubernetes.io/name":       "cloudprem",
						"app.kubernetes.io/instance":   "byoc",
						"app.kubernetes.io/managed-by": "datadog-operator",
						"team":                         "search",
					},
					Annotations: map[string]string{"example.com/owner": "operator"},
				},
				Data: map[string]string{nodeConfigFileName: `cloudprem:
  create_dd_logs_index: true
  create_dd_metrics_index: false
  create_dd_sketches_index: false
  create_dd_traces_index: false
  mtls_header: X-Amzn-Mtls-Clientcert
cloudprem_listen_port: 7283
data_dir: /quickwit/qwdata
docs_clustering:
- fingerprint:
  - kind: structure
- fingerprint:
  - kind: raw
    path: source
- fingerprint:
  - kind: raw
    path: status
- fingerprint:
  - kind: tokenized
    path: message
gossip_listen_port: 7282
grpc:
  keep_alive:
    interval: 30s
    timeout: 10s
health:
  listen_port: 7284
indexer:
  split_store_max_num_bytes: 200G
  split_store_max_num_splits: 10000
ingest_api:
  max_queue_disk_usage: 7.2GiB
  max_queue_memory_usage: 3.6GiB
listen_address: 0.0.0.0
searcher:
  aggregation_memory_limit: 500M
  fast_field_cache_capacity: 4.875G
  max_num_concurrent_split_searches: 40
  partial_request_cache_capacity: 187.5M
  split_footer_cache_capacity: 375M
version: 0.8`},
			},
		},
		{
			name: "with node config override",
			nodeConfig: &runtime.RawExtension{Raw: []byte(`searcher:
  aggregation_memory_limit: 1G`)},
			want: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "byoc",
					Namespace: "testing",
					Labels: map[string]string{
						"app.kubernetes.io/name":       "cloudprem",
						"app.kubernetes.io/instance":   "byoc",
						"app.kubernetes.io/managed-by": "datadog-operator",
						"team":                         "search",
					},
					Annotations: map[string]string{"example.com/owner": "operator"},
				},
				Data: map[string]string{nodeConfigFileName: `cloudprem:
  create_dd_logs_index: true
  create_dd_metrics_index: false
  create_dd_sketches_index: false
  create_dd_traces_index: false
  mtls_header: X-Amzn-Mtls-Clientcert
cloudprem_listen_port: 7283
data_dir: /quickwit/qwdata
docs_clustering:
- fingerprint:
  - kind: structure
- fingerprint:
  - kind: raw
    path: source
- fingerprint:
  - kind: raw
    path: status
- fingerprint:
  - kind: tokenized
    path: message
gossip_listen_port: 7282
grpc:
  keep_alive:
    interval: 30s
    timeout: 10s
health:
  listen_port: 7284
indexer:
  split_store_max_num_bytes: 200G
  split_store_max_num_splits: 10000
ingest_api:
  max_queue_disk_usage: 7.2GiB
  max_queue_memory_usage: 3.6GiB
listen_address: 0.0.0.0
searcher:
  aggregation_memory_limit: 1G
  fast_field_cache_capacity: 4.875G
  max_num_concurrent_split_searches: 40
  partial_request_cache_capacity: 187.5M
  split_footer_cache_capacity: 375M
version: 0.8`},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := testCluster()
			cluster.Spec.NodeConfig = tt.nodeConfig

			resources, err := BuildResources(cluster, testRelease())
			if err != nil {
				t.Fatalf("BuildResources() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, resources.configMap); diff != "" {
				t.Errorf("configMap mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBuildResources_HeadlessService(t *testing.T) {
	cluster := testCluster()
	want := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "byoc-headless",
			Namespace: "testing",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "cloudprem",
				"app.kubernetes.io/instance":   "byoc",
				"app.kubernetes.io/managed-by": "datadog-operator",
				"team":                         "search",
			},
			Annotations: map[string]string{"example.com/owner": "operator"},
		},
		Spec: corev1.ServiceSpec{
			Type:                     corev1.ServiceTypeClusterIP,
			ClusterIP:                corev1.ClusterIPNone,
			PublishNotReadyAddresses: true,
			Selector: map[string]string{
				"app.kubernetes.io/name":     "cloudprem",
				"app.kubernetes.io/instance": "byoc",
			},
			Ports: []corev1.ServicePort{
				{Name: "tcp-http", Port: 7280, Protocol: corev1.ProtocolTCP},
				{Name: "tcp-grpc", Port: 7281, Protocol: corev1.ProtocolTCP},
				{Name: "udp", Port: 7282, Protocol: corev1.ProtocolUDP},
				{Name: "tcp-cloudprem", Port: 7283, Protocol: corev1.ProtocolTCP},
				{Name: "tcp-health", Port: 7284, Protocol: corev1.ProtocolTCP},
			},
		},
	}

	resources, err := BuildResources(cluster, testRelease())
	if err != nil {
		t.Fatalf("BuildResources() unexpected error: %v", err)
	}
	if diff := cmp.Diff(want, resources.headlessService); diff != "" {
		t.Errorf("headlessService mismatch (-want +got):\n%s", diff)
	}
}

func TestBuildResources_ServiceAccount(t *testing.T) {
	tests := []struct {
		name     string
		identity *datadoghqv1alpha1.DatadogBYOCClusterIdentitySpec
		provider *datadoghqv1alpha1.DatadogBYOCClusterProviderSpec
		want     *corev1.ServiceAccount
	}{
		{
			name: "defaults",
			want: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "byoc",
					Namespace: "testing",
					Labels: map[string]string{
						"app.kubernetes.io/name":       "cloudprem",
						"app.kubernetes.io/instance":   "byoc",
						"app.kubernetes.io/managed-by": "datadog-operator",
						"team":                         "search",
					},
					Annotations: map[string]string{"example.com/owner": "operator"},
				},
				AutomountServiceAccountToken: ptr.To(false),
			},
		},
		{
			name:     "custom identity and IRSA",
			identity: &datadoghqv1alpha1.DatadogBYOCClusterIdentitySpec{ServiceAccountName: ptr.To("custom")},
			provider: &datadoghqv1alpha1.DatadogBYOCClusterProviderSpec{AWS: &datadoghqv1alpha1.DatadogBYOCClusterAWSSpec{
				IRSARoleARN: ptr.To("arn:aws:iam::123456789012:role/byoc"),
			}},
			want: &corev1.ServiceAccount{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "custom",
					Namespace: "testing",
					Labels: map[string]string{
						"app.kubernetes.io/name":       "cloudprem",
						"app.kubernetes.io/instance":   "byoc",
						"app.kubernetes.io/managed-by": "datadog-operator",
						"team":                         "search",
					},
					Annotations: map[string]string{
						"example.com/owner":                        "operator",
						"eks.amazonaws.com/role-arn":               "arn:aws:iam::123456789012:role/byoc",
						"eks.amazonaws.com/sts-regional-endpoints": "true",
					},
				},
				AutomountServiceAccountToken: ptr.To(false),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := testCluster()
			cluster.Spec.Identity = tt.identity
			cluster.Spec.Provider = tt.provider

			resources, err := BuildResources(cluster, testRelease())
			if err != nil {
				t.Fatalf("BuildResources() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, resources.serviceAccount); diff != "" {
				t.Errorf("serviceAccount mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestResolveAffinity(t *testing.T) {
	cluster := testCluster()
	customAffinity := &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{{
				Weight:     50,
				Preference: corev1.NodeSelectorTerm{MatchExpressions: []corev1.NodeSelectorRequirement{{Key: "example.com/node", Operator: corev1.NodeSelectorOpExists}}},
			}},
		},
	}
	tests := []struct {
		name      string
		global    *corev1.Affinity
		component *corev1.Affinity
		want      *corev1.Affinity
	}{
		{
			name: "default pod anti-affinity",
			want: &corev1.Affinity{
				PodAntiAffinity: &corev1.PodAntiAffinity{
					PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{{
						Weight: 100,
						PodAffinityTerm: corev1.PodAffinityTerm{
							LabelSelector: &metav1.LabelSelector{MatchLabels: selectorLabels(cluster, "indexer")},
							TopologyKey:   corev1.LabelHostname,
						},
					}},
				},
			},
		},
		{
			name:   "explicit empty global affinity disables default",
			global: &corev1.Affinity{},
			want:   &corev1.Affinity{},
		},
		{
			name:      "explicit empty component affinity disables default",
			component: &corev1.Affinity{},
			want:      &corev1.Affinity{},
		},
		{
			name:      "custom affinity replaces default",
			component: customAffinity,
			want:      customAffinity,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveAffinity(cluster, "indexer", tt.global, tt.component)
			if err != nil {
				t.Fatalf("resolveAffinity() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("resolveAffinity() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestPodDisruptionBudgetBuilder(t *testing.T) {
	minAvailable := intstr.FromInt32(2)
	maxUnavailable := intstr.FromString("25%")
	workload := workloadValues{
		Metadata: metav1.ObjectMeta{Name: "byoc-indexer", Namespace: "testing"},
		Selector: map[string]string{"app.kubernetes.io/component": "indexer"},
	}
	podDisruptionBudget := func(minAvailable, maxUnavailable *intstr.IntOrString) *policyv1.PodDisruptionBudget {
		return &policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{Name: "byoc-indexer", Namespace: "testing"},
			Spec: policyv1.PodDisruptionBudgetSpec{
				MinAvailable:   minAvailable,
				MaxUnavailable: maxUnavailable,
				Selector:       &metav1.LabelSelector{MatchLabels: map[string]string{"app.kubernetes.io/component": "indexer"}},
			},
		}
	}
	tests := []struct {
		name      string
		global    *datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec
		component *datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec
		want      *policyv1.PodDisruptionBudget
		wantErr   bool
	}{
		{
			name: "operator default",
			want: podDisruptionBudget(nil, ptr.To(intstr.FromInt32(1))),
		},
		{
			name:   "global override",
			global: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{MinAvailable: &minAvailable},
			want:   podDisruptionBudget(&minAvailable, nil),
		},
		{
			name:      "component override takes precedence",
			global:    &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{MinAvailable: &minAvailable},
			component: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{MaxUnavailable: &maxUnavailable},
			want:      podDisruptionBudget(nil, &maxUnavailable),
		},
		{
			name:   "empty global setting disables budget",
			global: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{},
		},
		{
			name:      "empty component setting disables budget",
			global:    &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{MinAvailable: &minAvailable},
			component: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{},
		},
		{
			name: "minAvailable and maxUnavailable conflict",
			component: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{
				MinAvailable:   &minAvailable,
				MaxUnavailable: &maxUnavailable,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newPodDisruptionBudgetBuilder(workload.Metadata, workload.Selector, tt.global, tt.component).build()
			if gotErr := err != nil; gotErr != tt.wantErr {
				t.Fatalf("build() error = %v, wantErr %v", err, tt.wantErr)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("build() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBuildResources_Indexer(t *testing.T) {
	wantDefault := func() *StatefulSetResources {
		want := wantDefaultStatefulSet(wantStatefulSetOptions{
			component:                     "indexer",
			configVolumeMount:             corev1.VolumeMount{Name: "config", MountPath: "/quickwit/"},
			additionalEnv:                 []corev1.EnvVar{{Name: "QW_INGEST_DECOMMISSION_TIMEOUT", Value: "270s"}},
			terminationGracePeriodSeconds: ptr.To[int64](300),
		})
		want.StatefulSet.Spec.Template.Spec.Volumes = want.StatefulSet.Spec.Template.Spec.Volumes[:1]
		want.StatefulSet.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{{
			ObjectMeta: metav1.ObjectMeta{Name: "data"},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("250Gi")},
				},
			},
		}}
		return want
	}

	tests := []struct {
		name    string
		indexer *datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec
		want    func() *StatefulSetResources
	}{
		{
			name:    "defaults",
			indexer: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{},
			want:    wantDefault,
		},
		{
			name: "autoscaling",
			indexer: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
				Autoscaling: &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{},
			},
			want: func() *StatefulSetResources {
				want := wantDefault()
				want.StatefulSet.Spec.Replicas = nil
				want.HPA = wantAutoscaling(wantAutoscalingOptions{
					component:          "indexer",
					averageUtilization: 70,
					scaleUpWindow:      0,
					scaleDownWindow:    300,
				})
				return want
			},
		},
		{
			name: "volume claim template",
			indexer: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
				Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{
					VolumeClaimTemplate: &datadoghqv1alpha1.DatadogBYOCClusterEmbeddedPersistentVolumeClaim{
						TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "PersistentVolumeClaim"},
						DatadogBYOCClusterEmbeddedObjectMetadata: datadoghqv1alpha1.DatadogBYOCClusterEmbeddedObjectMetadata{
							Labels:      map[string]string{"storage": "indexer"},
							Annotations: map[string]string{"example.com/storage": "indexer"},
						},
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							StorageClassName: ptr.To("gp3"),
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("100Gi")},
							},
						},
					},
				},
			},
			want: func() *StatefulSetResources {
				want := wantDefault()
				want.StatefulSet.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{{
					TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "PersistentVolumeClaim"},
					ObjectMeta: metav1.ObjectMeta{
						Name:        "data",
						Labels:      map[string]string{"storage": "indexer"},
						Annotations: map[string]string{"example.com/storage": "indexer"},
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						StorageClassName: ptr.To("gp3"),
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("100Gi")},
						},
					},
				}}
				return want
			},
		},
		{
			name: "empty dir",
			indexer: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
				Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{EmptyDir: &corev1.EmptyDirVolumeSource{}},
			},
			want: func() *StatefulSetResources {
				want := wantDefault()
				want.StatefulSet.Spec.VolumeClaimTemplates = nil
				want.StatefulSet.Spec.Template.Spec.Volumes = append(want.StatefulSet.Spec.Template.Spec.Volumes, defaultDataVolume())
				return want
			},
		},
		{
			name: "volume claim template metadata with defaults",
			indexer: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
				Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{
					VolumeClaimTemplate: &datadoghqv1alpha1.DatadogBYOCClusterEmbeddedPersistentVolumeClaim{
						DatadogBYOCClusterEmbeddedObjectMetadata: datadoghqv1alpha1.DatadogBYOCClusterEmbeddedObjectMetadata{
							Annotations: map[string]string{"example.com/defaulted": "true"},
						},
					},
				},
			},
			want: func() *StatefulSetResources {
				want := wantDefault()
				want.StatefulSet.Spec.VolumeClaimTemplates[0].Annotations = map[string]string{"example.com/defaulted": "true"}
				return want
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := testCluster()
			cluster.Spec.Components.Indexer = tt.indexer

			resources, err := BuildResources(cluster, testRelease())
			if err != nil {
				t.Fatalf("BuildResources() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want(), resources.indexer, ignoreConfigChecksum); diff != "" {
				t.Errorf("indexer mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBuildResources_Searcher(t *testing.T) {
	wantDefault := func() *StatefulSetResources {
		return wantDefaultStatefulSet(wantStatefulSetOptions{
			component: "searcher",
			additionalServicePorts: []corev1.ServicePort{{
				Name:       "cloudprem",
				Port:       7283,
				Protocol:   corev1.ProtocolTCP,
				TargetPort: intstr.FromString("cloudprem"),
			}},
			configVolumeMount: corev1.VolumeMount{
				Name:      "config",
				MountPath: "/quickwit/node.yaml",
				SubPath:   "node.yaml",
			},
		})
	}

	tests := []struct {
		name     string
		searcher *datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec
		want     func() *StatefulSetResources
	}{
		{
			name:     "defaults",
			searcher: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{},
			want:     wantDefault,
		},
		{
			name: "autoscaling",
			searcher: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
				Autoscaling: &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{},
			},
			want: func() *StatefulSetResources {
				want := wantDefault()
				want.StatefulSet.Spec.Replicas = nil
				want.HPA = wantAutoscaling(wantAutoscalingOptions{
					component:          "searcher",
					averageUtilization: 50,
					scaleUpWindow:      60,
					scaleDownWindow:    300,
				})
				return want
			},
		},
		{
			name: "volume claim template",
			searcher: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
				Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{
					VolumeClaimTemplate: &datadoghqv1alpha1.DatadogBYOCClusterEmbeddedPersistentVolumeClaim{
						Spec: corev1.PersistentVolumeClaimSpec{
							AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
							StorageClassName: ptr.To("gp3"),
							Resources: corev1.VolumeResourceRequirements{
								Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("100Gi")},
							},
						},
					},
				},
			},
			want: func() *StatefulSetResources {
				want := wantDefault()
				want.StatefulSet.Spec.Template.Spec.Volumes = want.StatefulSet.Spec.Template.Spec.Volumes[:1]
				want.StatefulSet.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{{
					ObjectMeta: metav1.ObjectMeta{Name: "data"},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						StorageClassName: ptr.To("gp3"),
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("100Gi")},
						},
					},
				}}
				return want
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := testCluster()
			cluster.Spec.Components.Searcher = tt.searcher

			resources, err := BuildResources(cluster, testRelease())
			if err != nil {
				t.Fatalf("BuildResources() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want(), resources.searcher, ignoreConfigChecksum); diff != "" {
				t.Errorf("searcher mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBuildResources_Metastore(t *testing.T) {
	wantDefault := func() *DeploymentResources {
		return wantDefaultDeployment(wantDeploymentOptions{
			component: "metastore",
			service:   "metastore",
			replicas:  2,
			resources: wantDefaultDeploymentResources(),
		})
	}

	tests := []struct {
		name      string
		metastore *datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec
		want      func() *DeploymentResources
	}{
		{
			name:      "defaults",
			metastore: &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{},
			want:      wantDefault,
		},
		{
			name: "database URI secret",
			metastore: &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{
				Database: &datadoghqv1alpha1.DatadogBYOCClusterDatabaseSpec{
					URISecretRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "metastore-database"},
						Key:                  "uri",
					},
				},
			},
			want: func() *DeploymentResources {
				return wantDefaultDeployment(wantDeploymentOptions{
					component: "metastore",
					service:   "metastore",
					replicas:  2,
					resources: wantDefaultDeploymentResources(),
					additionalEnv: []corev1.EnvVar{{
						Name: "QW_METASTORE_URI",
						ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "metastore-database"},
							Key:                  "uri",
						}},
					}},
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := testCluster()
			cluster.Spec.Components.Metastore = tt.metastore

			resources, err := BuildResources(cluster, testRelease())
			if err != nil {
				t.Fatalf("BuildResources() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want(), resources.metastore, ignoreConfigChecksum); diff != "" {
				t.Errorf("metastore mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBuildResources_ControlPlane(t *testing.T) {
	tests := []struct {
		name         string
		controlPlane *datadoghqv1alpha1.DatadogBYOCClusterComponentSpec
		want         func() *DeploymentResources
	}{
		{
			name:         "defaults",
			controlPlane: &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
			want: func() *DeploymentResources {
				return wantDefaultDeployment(wantDeploymentOptions{
					component: "control-plane",
					service:   "control_plane",
					replicas:  1,
					strategy:  appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
					resources: wantDefaultDeploymentResources(),
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := testCluster()
			cluster.Spec.Components.ControlPlane = tt.controlPlane

			resources, err := BuildResources(cluster, testRelease())
			if err != nil {
				t.Fatalf("BuildResources() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want(), resources.controlPlane, ignoreConfigChecksum); diff != "" {
				t.Errorf("controlPlane mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBuildResources_Janitor(t *testing.T) {
	tests := []struct {
		name    string
		janitor *datadoghqv1alpha1.DatadogBYOCClusterComponentSpec
		want    func() *DeploymentResources
	}{
		{
			name:    "defaults",
			janitor: &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
			want: func() *DeploymentResources {
				return wantDefaultDeployment(wantDeploymentOptions{
					component: "janitor",
					service:   "janitor",
					replicas:  1,
					strategy:  appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
					resources: wantDefaultDeploymentResources(),
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := testCluster()
			cluster.Spec.Components.Janitor = tt.janitor

			resources, err := BuildResources(cluster, testRelease())
			if err != nil {
				t.Fatalf("BuildResources() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want(), resources.janitor, ignoreConfigChecksum); diff != "" {
				t.Errorf("janitor mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBuildResources_ReadOnlyMetastore(t *testing.T) {
	tests := []struct {
		name              string
		readOnlyMetastore *datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec
		want              func() *DeploymentResources
	}{
		{
			name: "disabled",
			want: func() *DeploymentResources {
				return nil
			},
		},
		{
			name:              "defaults",
			readOnlyMetastore: &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{},
			want: func() *DeploymentResources {
				return wantDefaultDeployment(wantDeploymentOptions{
					component: "metastore-ro",
					service:   "metastore_read_replica",
					replicas:  2,
					resources: wantDefaultDeploymentResources(),
				})
			},
		},
		{
			name: "database URI secret",
			readOnlyMetastore: &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{
				Database: &datadoghqv1alpha1.DatadogBYOCClusterDatabaseSpec{
					URISecretRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: "metastore-read-replica-database"},
						Key:                  "uri",
					},
				},
			},
			want: func() *DeploymentResources {
				return wantDefaultDeployment(wantDeploymentOptions{
					component: "metastore-ro",
					service:   "metastore_read_replica",
					replicas:  2,
					resources: wantDefaultDeploymentResources(),
					additionalEnv: []corev1.EnvVar{{
						Name: "QW_METASTORE_READ_REPLICA_URI",
						ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "metastore-read-replica-database"},
							Key:                  "uri",
						}},
					}},
				})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := testCluster()
			cluster.Spec.Components.ReadOnlyMetastore = tt.readOnlyMetastore

			resources, err := BuildResources(cluster, testRelease())
			if err != nil {
				t.Fatalf("BuildResources() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want(), resources.readOnlyMetastore, ignoreConfigChecksum); diff != "" {
				t.Errorf("readOnlyMetastore mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBuildResources_Compactor(t *testing.T) {
	wantDefault := func(terminationGracePeriodSeconds int64, decommissionTimeout string) *DeploymentResources {
		return wantDefaultDeployment(wantDeploymentOptions{
			component:                     "compactor",
			service:                       "compactor",
			replicas:                      1,
			resources:                     corev1.ResourceRequirements{},
			terminationGracePeriodSeconds: ptr.To(terminationGracePeriodSeconds),
			additionalEnv: []corev1.EnvVar{
				{Name: "QW_ENABLE_STANDALONE_COMPACTORS", Value: "true"},
				{Name: "QW_COMPACTOR_DECOMMISSION_TIMEOUT", Value: decommissionTimeout},
			},
		})
	}

	tests := []struct {
		name      string
		compactor *datadoghqv1alpha1.DatadogBYOCClusterComponentSpec
		want      func() *DeploymentResources
	}{
		{
			name: "disabled",
			want: func() *DeploymentResources {
				return nil
			},
		},
		{
			name:      "defaults",
			compactor: &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
			want: func() *DeploymentResources {
				return wantDefault(60, "54s")
			},
		},
		{
			name: "termination grace period",
			compactor: &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
				TerminationGracePeriodSeconds: ptr.To[int64](120),
			},
			want: func() *DeploymentResources {
				return wantDefault(120, "108s")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := testCluster()
			cluster.Spec.Components.Compactor = tt.compactor

			resources, err := BuildResources(cluster, testRelease())
			if err != nil {
				t.Fatalf("BuildResources() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want(), resources.compactor, ignoreConfigChecksum); diff != "" {
				t.Errorf("compactor mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func testCluster() *datadoghqv1alpha1.DatadogBYOCCluster {
	return &datadoghqv1alpha1.DatadogBYOCCluster{
		ObjectMeta: metav1.ObjectMeta{Name: "byoc", Namespace: "testing"},
		Spec: datadoghqv1alpha1.DatadogBYOCClusterSpec{
			Global: datadoghqv1alpha1.DatadogBYOCClusterGlobalSpec{
				Labels:      map[string]string{"team": "search"},
				Annotations: map[string]string{"example.com/owner": "operator"},
			},
			Datadog: &datadoghqv1alpha1.DatadogBYOCClusterDatadogSpec{
				Site:          ptr.To("datadoghq.com"),
				BYOCTelemetry: ptr.To(true),
				DogstatsdServer: &datadoghqv1alpha1.DatadogBYOCClusterDogstatsdServerSpec{
					Port: ptr.To[int32](8125),
				},
			},
			Components: &datadoghqv1alpha1.DatadogBYOCClusterComponentsSpec{
				Metastore:    &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{},
				Indexer:      &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{},
				Searcher:     &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{},
				ControlPlane: &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
				Janitor:      &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
			},
		},
	}
}

func testRelease() *byocrelease.ResolvedRelease {
	return &byocrelease.ResolvedRelease{
		Release: byocrelease.BYOCRelease{
			Images: byocrelease.BYOCReleaseImages{
				Pomsky: byocrelease.BYOCReleaseImage{
					Repository: "registry.example.com/cloudprem",
					Tag:        "v1.2.3",
				},
			},
		},
	}
}

var ignoreConfigChecksum = cmpopts.IgnoreMapEntries(func(key, _ string) bool {
	return key == configChecksumAnnotation
})

func wantDefaultDeploymentResources() corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")},
		Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2"), corev1.ResourceMemory: resource.MustParse("4Gi")},
	}
}

type wantStatefulSetOptions struct {
	component                     string
	additionalServicePorts        []corev1.ServicePort
	configVolumeMount             corev1.VolumeMount
	additionalEnv                 []corev1.EnvVar
	terminationGracePeriodSeconds *int64
}

func wantDefaultStatefulSet(options wantStatefulSetOptions) *StatefulSetResources {
	workload := wantDefaultWorkload(wantWorkloadOptions{
		component:              options.component,
		service:                options.component,
		replicas:               2,
		additionalServicePorts: options.additionalServicePorts,
		configVolumeMount:      options.configVolumeMount,
		additionalEnv:          options.additionalEnv,
		resources: corev1.ResourceRequirements{
			Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("13100Mi")},
			Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("3600m"), corev1.ResourceMemory: resource.MustParse("13100Mi")},
		},
		terminationGracePeriodSeconds: options.terminationGracePeriodSeconds,
	})

	return &StatefulSetResources{
		Service: workload.service,
		StatefulSet: &appsv1.StatefulSet{
			ObjectMeta: workload.metadata,
			Spec: appsv1.StatefulSetSpec{
				Replicas:            workload.replicas,
				ServiceName:         "byoc-headless",
				PodManagementPolicy: appsv1.OrderedReadyPodManagement,
				Selector:            workload.selector,
				Template:            workload.template,
			},
		},
		PodDisruptionBudget: wantDefaultPodDisruptionBudget(workload),
	}
}

type wantDeploymentOptions struct {
	component                     string
	service                       string
	replicas                      int32
	strategy                      appsv1.DeploymentStrategy
	additionalEnv                 []corev1.EnvVar
	resources                     corev1.ResourceRequirements
	terminationGracePeriodSeconds *int64
}

func wantDefaultDeployment(options wantDeploymentOptions) *DeploymentResources {
	workload := wantDefaultWorkload(wantWorkloadOptions{
		component:                     options.component,
		service:                       options.service,
		replicas:                      options.replicas,
		configVolumeMount:             defaultConfigVolumeMount(),
		additionalEnv:                 options.additionalEnv,
		resources:                     options.resources,
		terminationGracePeriodSeconds: options.terminationGracePeriodSeconds,
	})

	return &DeploymentResources{
		Service: workload.service,
		Deployment: &appsv1.Deployment{
			ObjectMeta: workload.metadata,
			Spec: appsv1.DeploymentSpec{
				Replicas: workload.replicas,
				Selector: workload.selector,
				Template: workload.template,
				Strategy: options.strategy,
			},
		},
		PodDisruptionBudget: wantDefaultPodDisruptionBudget(workload),
	}
}

type wantWorkloadOptions struct {
	component                     string
	service                       string
	replicas                      int32
	additionalServicePorts        []corev1.ServicePort
	configVolumeMount             corev1.VolumeMount
	additionalEnv                 []corev1.EnvVar
	resources                     corev1.ResourceRequirements
	terminationGracePeriodSeconds *int64
}

type wantWorkload struct {
	service  *corev1.Service
	metadata metav1.ObjectMeta
	replicas *int32
	selector *metav1.LabelSelector
	template corev1.PodTemplateSpec
}

func wantDefaultWorkload(options wantWorkloadOptions) wantWorkload {
	selector := map[string]string{
		"app.kubernetes.io/name":      "cloudprem",
		"app.kubernetes.io/instance":  "byoc",
		"app.kubernetes.io/component": options.component,
	}
	labels := map[string]string{
		"app.kubernetes.io/name":       "cloudprem",
		"app.kubernetes.io/instance":   "byoc",
		"app.kubernetes.io/component":  options.component,
		"app.kubernetes.io/managed-by": "datadog-operator",
		"team":                         "search",
	}
	field := func(fieldPath string) *corev1.EnvVarSource {
		return &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: fieldPath}}
	}
	resourceField := func(resourceName string) *corev1.EnvVarSource {
		return &corev1.EnvVarSource{ResourceFieldRef: &corev1.ResourceFieldSelector{ContainerName: "cloudprem", Resource: resourceName}}
	}
	env := []corev1.EnvVar{
		{Name: "KUBERNETES_NAMESPACE", ValueFrom: field("metadata.namespace")},
		{Name: "KUBERNETES_COMPONENT", ValueFrom: field("metadata.labels['app.kubernetes.io/component']")},
		{Name: "KUBERNETES_POD_NAME", ValueFrom: field("metadata.name")},
		{Name: "KUBERNETES_NODE_NAME", ValueFrom: field("spec.nodeName")},
		{Name: "KUBERNETES_POD_IP", ValueFrom: field("status.podIP")},
		{Name: "KUBERNETES_LIMITS_CPU", ValueFrom: resourceField("limits.cpu")},
		{Name: "KUBERNETES_LIMITS_MEMORY", ValueFrom: resourceField("limits.memory")},
		{Name: "KUBERNETES_REQUESTS_CPU", ValueFrom: resourceField("requests.cpu")},
		{Name: "QW_NUM_CPUS", ValueFrom: resourceField("requests.cpu")},
		{Name: "KUBERNETES_REQUESTS_MEMORY", ValueFrom: resourceField("requests.memory")},
		{Name: "QW_CONFIG", Value: "/quickwit/node.yaml"},
		{Name: "QW_CLUSTER_ID", Value: "testing-byoc"},
		{Name: "QW_NODE_ID", Value: "$(KUBERNETES_POD_NAME)"},
		{Name: "QW_AVAILABILITY_ZONE", ValueFrom: field("metadata.labels['topology.kubernetes.io/zone']")},
		{Name: "QW_PEER_SEEDS", Value: "byoc-headless"},
		{Name: "QW_ADVERTISE_ADDRESS", Value: "$(KUBERNETES_POD_IP)"},
		{Name: "QW_CLUSTER_ENDPOINT", Value: "http://byoc-metastore.testing.svc.cluster.local:7280"},
		{Name: "CP_DOGSTATSD_SERVER_HOST", ValueFrom: field("status.hostIP")},
		{Name: "CP_DOGSTATSD_SERVER_PORT", Value: "8125"},
		{Name: "CP_ENABLE_REVERSE_CONNECTION", Value: "true"},
		{Name: "CP_MIN_SHARDS", Value: "12"},
		{Name: "DD_SITE", Value: "datadoghq.com"},
		{Name: "QW_ENABLE_OPENTELEMETRY_OTLP_EXPORTER", Value: "true"},
		{Name: "BYOC_TELEMETRY_ENABLED", Value: "true"},
		{Name: "OTEL_RESOURCE_ATTRIBUTES", Value: "cluster_id=testing-byoc,node_id=$(QW_NODE_ID),host.name=$(KUBERNETES_NODE_NAME)"},
		{Name: "OTEL_EXPORTER_OTLP_PROTOCOL", Value: "http/protobuf"},
		{Name: "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", Value: "https://app.datadoghq.com/api/unstable/byoc-telemetry-intake/v1/logs"},
		{Name: "OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE", Value: "delta"},
		{Name: "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", Value: "https://app.datadoghq.com/api/unstable/byoc-telemetry-intake/v1/metrics"},
		{Name: "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", Value: "https://app.datadoghq.com/api/unstable/byoc-telemetry-intake/v1/traces"},
		{Name: "OTEL_TRACES_SAMPLER", Value: "parentbased_traceidratio"},
		{Name: "OTEL_TRACES_SAMPLER_ARG", Value: "0.2"},
		{Name: "IMAGE_NAME", Value: "registry.example.com/cloudprem"},
		{Name: "IMAGE_TAG", Value: "v1.2.3"},
	}
	env = append(env, options.additionalEnv...)
	env = append(env,
		corev1.EnvVar{Name: "NO_COLOR", Value: "true"},
		corev1.EnvVar{Name: "QW_DISABLE_INGEST_V1", Value: "true"},
		corev1.EnvVar{Name: "QW_DISABLE_TELEMETRY", Value: "true"},
		corev1.EnvVar{Name: "QW_LOG_FORMAT", Value: "DDG"},
		corev1.EnvVar{Name: "QW_RANDOM_SPLIT_PREFIX", Value: "true"},
	)
	resourceName := "byoc-" + options.component
	servicePorts := []corev1.ServicePort{
		{Name: "rest", Port: 7280, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromString("rest")},
		{Name: "grpc", Port: 7281, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromString("grpc")},
	}
	servicePorts = append(servicePorts, options.additionalServicePorts...)
	servicePorts = append(servicePorts, corev1.ServicePort{Name: "health", Port: 7284, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromString("health")})

	return wantWorkload{
		service: &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:        resourceName,
				Namespace:   "testing",
				Labels:      labels,
				Annotations: map[string]string{"example.com/owner": "operator"},
			},
			Spec: corev1.ServiceSpec{
				Type:     corev1.ServiceTypeClusterIP,
				Selector: selector,
				Ports:    servicePorts,
			},
		},
		metadata: metav1.ObjectMeta{
			Name:        resourceName,
			Namespace:   "testing",
			Labels:      labels,
			Annotations: map[string]string{"example.com/owner": "operator"},
		},
		replicas: ptr.To(options.replicas),
		selector: &metav1.LabelSelector{MatchLabels: selector},
		template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels:      labels,
				Annotations: map[string]string{"example.com/owner": "operator"},
			},
			Spec: corev1.PodSpec{
				ServiceAccountName: "byoc",
				SecurityContext:    &corev1.PodSecurityContext{FSGroup: ptr.To[int64](1005)},
				DNSConfig:          &corev1.PodDNSConfig{Options: []corev1.PodDNSConfigOption{{Name: "ndots", Value: ptr.To("1")}}},
				Containers: []corev1.Container{{
					Name:            "cloudprem",
					Image:           "registry.example.com/cloudprem:v1.2.3",
					ImagePullPolicy: corev1.PullIfNotPresent,
					Args:            []string{"run", "--service", options.service},
					Env:             env,
					Ports: []corev1.ContainerPort{
						{Name: "rest", ContainerPort: 7280, Protocol: corev1.ProtocolTCP},
						{Name: "grpc", ContainerPort: 7281, Protocol: corev1.ProtocolTCP},
						{Name: "discovery", ContainerPort: 7282, Protocol: corev1.ProtocolUDP},
						{Name: "cloudprem", ContainerPort: 7283, Protocol: corev1.ProtocolTCP},
						{Name: "health", ContainerPort: 7284, Protocol: corev1.ProtocolTCP},
					},
					Resources: options.resources,
					VolumeMounts: []corev1.VolumeMount{
						options.configVolumeMount,
						{Name: "data", MountPath: "/quickwit/qwdata"},
					},
					StartupProbe: &corev1.Probe{
						ProbeHandler:     corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/health/livez", Port: intstr.FromString("health")}},
						FailureThreshold: 12,
						PeriodSeconds:    5,
					},
					LivenessProbe:  &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/health/livez", Port: intstr.FromString("health")}}},
					ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: "/health/readyz", Port: intstr.FromString("health")}}},
					SecurityContext: &corev1.SecurityContext{
						RunAsNonRoot:           ptr.To(true),
						RunAsUser:              ptr.To[int64](1005),
						ReadOnlyRootFilesystem: ptr.To(true),
					},
				}},
				Volumes: []corev1.Volume{
					{
						Name: "config",
						VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: "byoc"},
							Items:                []corev1.KeyToPath{{Key: "node.yaml", Path: "node.yaml"}},
						}},
					},
					{Name: "data", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
				},
				Affinity: &corev1.Affinity{
					PodAntiAffinity: &corev1.PodAntiAffinity{
						PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{{
							Weight: 100,
							PodAffinityTerm: corev1.PodAffinityTerm{
								LabelSelector: &metav1.LabelSelector{MatchLabels: selector},
								TopologyKey:   corev1.LabelHostname,
							},
						}},
					},
				},
				TerminationGracePeriodSeconds: options.terminationGracePeriodSeconds,
			},
		},
	}
}

func wantDefaultPodDisruptionBudget(workload wantWorkload) *policyv1.PodDisruptionBudget {
	metadata := workload.metadata.DeepCopy()
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: *metadata,
		Spec: policyv1.PodDisruptionBudgetSpec{
			MaxUnavailable: ptr.To(intstr.FromInt32(1)),
			Selector:       workload.selector.DeepCopy(),
		},
	}
}

type wantAutoscalingOptions struct {
	component          string
	averageUtilization int32
	scaleUpWindow      int32
	scaleDownWindow    int32
}

func wantAutoscaling(options wantAutoscalingOptions) *autoscalingv2.HorizontalPodAutoscaler {
	resourceName := "byoc-" + options.component
	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      resourceName,
			Namespace: "testing",
			Labels: map[string]string{
				"app.kubernetes.io/name":       "cloudprem",
				"app.kubernetes.io/instance":   "byoc",
				"app.kubernetes.io/component":  options.component,
				"app.kubernetes.io/managed-by": "datadog-operator",
				"team":                         "search",
			},
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{APIVersion: "apps/v1", Kind: "StatefulSet", Name: resourceName},
			MinReplicas:    ptr.To[int32](2),
			MaxReplicas:    10,
			Metrics: []autoscalingv2.MetricSpec{{
				Type: autoscalingv2.ResourceMetricSourceType,
				Resource: &autoscalingv2.ResourceMetricSource{
					Name:   corev1.ResourceCPU,
					Target: autoscalingv2.MetricTarget{Type: autoscalingv2.UtilizationMetricType, AverageUtilization: ptr.To(options.averageUtilization)},
				},
			}},
			Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
				ScaleUp:   &autoscalingv2.HPAScalingRules{StabilizationWindowSeconds: ptr.To(options.scaleUpWindow)},
				ScaleDown: &autoscalingv2.HPAScalingRules{StabilizationWindowSeconds: ptr.To(options.scaleDownWindow)},
			},
		},
	}
}
