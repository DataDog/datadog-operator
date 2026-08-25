// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"fmt"
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

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

func TestBuildResources_StatefulSetReplicasOwnership(t *testing.T) {
	tests := []struct {
		name         string
		autoscaling  *datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec
		wantReplicas *int32
		wantHPA      bool
	}{
		{
			name:         "fixed replicas",
			wantReplicas: ptr.To[int32](2),
		},
		{
			name:        "replicas managed by HPA",
			autoscaling: &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{},
			wantHPA:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := testCluster()
			cluster.Spec.Components.Indexer.Autoscaling = tt.autoscaling

			resources, err := BuildResources(cluster, testRelease())
			if err != nil {
				t.Fatalf("BuildResources() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.wantReplicas, resources.indexer.StatefulSet.Spec.Replicas); diff != "" {
				t.Errorf("indexer.StatefulSet.Spec.Replicas mismatch (-want +got):\n%s", diff)
			}
			if got := resources.indexer.HPA != nil; got != tt.wantHPA {
				t.Errorf("indexer.HPA presence = %t, want %t", got, tt.wantHPA)
			}
		})
	}
}

func TestBuildResources_ComponentObjects(t *testing.T) {
	defaultCluster := testCluster()
	optionalResourcesCluster := testCluster()
	optionalResourcesCluster.Spec.Components.Indexer.Autoscaling = &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{}
	optionalResourcesCluster.Spec.Components.ReadOnlyMetastore = &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{}
	optionalResourcesCluster.Spec.Components.Compactor = &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{}

	tests := []struct {
		name    string
		cluster *datadoghqv1alpha1.DatadogBYOCCluster
		want    [][]string
	}{
		{
			name:    "required resources",
			cluster: defaultCluster,
			want: [][]string{
				{"*v1.ConfigMap/byoc", "*v1.ServiceAccount/byoc", "*v1.Service/byoc-headless"},
				{"*v1.Service/byoc-metastore", "*v1.Deployment/byoc-metastore"},
				{"*v1.Service/byoc-indexer", "*v1.StatefulSet/byoc-indexer"},
				{"*v1.Service/byoc-searcher", "*v1.StatefulSet/byoc-searcher"},
				{"*v1.Service/byoc-control-plane", "*v1.Deployment/byoc-control-plane"},
				{"*v1.Service/byoc-janitor", "*v1.Deployment/byoc-janitor"},
				{},
				{},
			},
		},
		{
			name:    "optional resources",
			cluster: optionalResourcesCluster,
			want: [][]string{
				{"*v1.ConfigMap/byoc", "*v1.ServiceAccount/byoc", "*v1.Service/byoc-headless"},
				{"*v1.Service/byoc-metastore", "*v1.Deployment/byoc-metastore"},
				{"*v1.Service/byoc-indexer", "*v1.StatefulSet/byoc-indexer", "*v2.HorizontalPodAutoscaler/byoc-indexer"},
				{"*v1.Service/byoc-searcher", "*v1.StatefulSet/byoc-searcher"},
				{"*v1.Service/byoc-control-plane", "*v1.Deployment/byoc-control-plane"},
				{"*v1.Service/byoc-janitor", "*v1.Deployment/byoc-janitor"},
				{"*v1.Service/byoc-metastore-ro", "*v1.Deployment/byoc-metastore-ro"},
				{"*v1.Service/byoc-compactor", "*v1.Deployment/byoc-compactor"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resources, err := BuildResources(tt.cluster, testRelease())
			if err != nil {
				t.Fatalf("BuildResources() unexpected error: %v", err)
			}

			groups := [][]client.Object{
				resources.Shared(),
				resources.Metastore().Objects(),
				resources.Indexer().Objects(),
				resources.Searcher().Objects(),
				resources.ControlPlane().Objects(),
				resources.Janitor().Objects(),
				resources.ReadOnlyMetastore().Objects(),
				resources.Compactor().Objects(),
			}
			got := make([][]string, 0, len(groups))
			for _, group := range groups {
				objects := make([]string, 0, len(group))
				for _, object := range group {
					objects = append(objects, fmt.Sprintf("%T/%s", object, object.GetName()))
				}
				got = append(got, objects)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("component objects mismatch (-want +got):\n%s", diff)
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
