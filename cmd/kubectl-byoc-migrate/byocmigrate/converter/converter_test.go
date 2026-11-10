// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package converter

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func TestParseManifest(t *testing.T) {
	tests := []struct {
		name     string
		manifest string
		want     []string
		wantErr  string
	}{
		{
			name: "typed and unknown kinds with Helm source comments",
			manifest: `---
# Source: chart/templates/serviceaccount.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: worker
---
# Source: chart/templates/servicemonitor.yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: worker
---
# Source: chart/templates/disabled.yaml
`,
			want: []string{"*v1.ServiceAccount/worker", "*unstructured.Unstructured/worker"},
		},
		{
			name:     "empty manifest",
			manifest: "",
		},
		{
			name:     "invalid document",
			manifest: "kind: [",
			wantErr:  "convert manifest document to JSON",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects, err := ParseManifest(tt.manifest)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ParseManifest() error = %v, want error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseManifest() unexpected error: %v", err)
			}
			var got []string
			for _, object := range objects {
				got = append(got, fmt.Sprintf("%T/%s", object, object.GetName()))
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("ParseManifest() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// The golden files were reviewed by hand. The manifests in testdata were rendered with helm template, with the
// command recorded at the top of each file.
func TestConvert(t *testing.T) {
	tests := []struct {
		name      string
		cloudprem string
		opw       string
		want      string
	}{
		{
			name:      "chart defaults",
			cloudprem: "cloudprem.yaml",
			opw:       "opw.yaml",
			want:      "cluster.yaml",
		},
		{
			name:      "persistent volume, autoscaling, budgets, read-only metastore, and compactor",
			cloudprem: "cloudprem-full.yaml",
			opw:       "opw-full.yaml",
			want:      "cluster-full.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Convert(testManifest(t, "cloudprem", tt.cloudprem), []PipelineManifest{{Pipeline: "logs", Manifest: testManifest(t, "logs", tt.opw)}})
			if err != nil {
				t.Fatalf("Convert() unexpected error: %v", err)
			}
			got, err := MarshalCluster(result.Cluster)
			if err != nil {
				t.Fatalf("MarshalCluster() unexpected error: %v", err)
			}
			if diff := cmp.Diff(readGolden(t, tt.want), string(got)); diff != "" {
				t.Errorf("Convert() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestConvert_Findings(t *testing.T) {
	tests := []struct {
		name      string
		cloudprem string
		opw       string
		want      []string
	}{
		{
			name:      "chart defaults",
			cloudprem: "cloudprem.yaml",
			opw:       "opw.yaml",
		},
		{
			name:      "persistent volume, autoscaling, budgets, read-only metastore, and compactor",
			cloudprem: "cloudprem-full.yaml",
			opw:       "opw-full.yaml",
		},
		{
			name:      "CloudPrem rendered with a release name different from the fullname",
			cloudprem: "cloudprem-release-logs.yaml",
			opw:       "opw.yaml",
			want: []string{
				`Blocker: Deployment/logs-cloudprem-control-plane: spec.selector is "app.kubernetes.io/component=control-plane,app.kubernetes.io/instance=logs,app.kubernetes.io/name=cloudprem" in the manifest, but the controller sets "app.kubernetes.io/component=control-plane,app.kubernetes.io/instance=logs-cloudprem,app.kubernetes.io/name=cloudprem"`,
				`Blocker: Deployment/logs-cloudprem-janitor: spec.selector is "app.kubernetes.io/component=janitor,app.kubernetes.io/instance=logs,app.kubernetes.io/name=cloudprem" in the manifest, but the controller sets "app.kubernetes.io/component=janitor,app.kubernetes.io/instance=logs-cloudprem,app.kubernetes.io/name=cloudprem"`,
				`Blocker: Deployment/logs-cloudprem-metastore: spec.selector is "app.kubernetes.io/component=metastore,app.kubernetes.io/instance=logs,app.kubernetes.io/name=cloudprem" in the manifest, but the controller sets "app.kubernetes.io/component=metastore,app.kubernetes.io/instance=logs-cloudprem,app.kubernetes.io/name=cloudprem"`,
				`Blocker: StatefulSet/logs-cloudprem-indexer: spec.selector is "app.kubernetes.io/component=indexer,app.kubernetes.io/instance=logs,app.kubernetes.io/name=cloudprem" in the manifest, but the controller sets "app.kubernetes.io/component=indexer,app.kubernetes.io/instance=logs-cloudprem,app.kubernetes.io/name=cloudprem"`,
				`Blocker: StatefulSet/logs-cloudprem-searcher: spec.selector is "app.kubernetes.io/component=searcher,app.kubernetes.io/instance=logs,app.kubernetes.io/name=cloudprem" in the manifest, but the controller sets "app.kubernetes.io/component=searcher,app.kubernetes.io/instance=logs-cloudprem,app.kubernetes.io/name=cloudprem"`,
			},
		},
		{
			name:      "Worker LoadBalancer Service and IRSA role",
			cloudprem: "cloudprem.yaml",
			opw:       "opw-loadbalancer.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Convert(testManifest(t, "logs", tt.cloudprem), []PipelineManifest{{Pipeline: "logs", Manifest: testManifest(t, "logs", tt.opw)}})
			if err != nil {
				t.Fatalf("Convert() unexpected error: %v", err)
			}
			var got []string
			for _, finding := range result.Findings {
				got = append(got, finding.String())
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Convert() findings mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestFindings_Blockers checks a converted cluster edited by hand, as with --from-file.
func TestFindings_Blockers(t *testing.T) {
	tests := []struct {
		name      string
		cloudprem string
		opw       string
		edit      func(*datadoghqv1alpha1.DatadogBYOCCluster)
		want      []string
	}{
		{
			name:      "unedited",
			cloudprem: "cloudprem-full.yaml",
			opw:       "opw-full.yaml",
			edit:      func(*datadoghqv1alpha1.DatadogBYOCCluster) {},
		},
		{
			name:      "autoscaling removed from a component with a HorizontalPodAutoscaler",
			cloudprem: "cloudprem-full.yaml",
			opw:       "opw-full.yaml",
			edit: func(cluster *datadoghqv1alpha1.DatadogBYOCCluster) {
				cluster.Spec.Components.Indexer.Autoscaling = nil
			},
			want: []string{"Blocker: HorizontalPodAutoscaler/cloudprem-indexer: it scales StatefulSet/cloudprem-indexer, which the cluster does not autoscale, so it would fight the replicas that the controller sets; set the autoscaling of the component or delete this object before the migration"},
		},
		{
			name:      "Worker Pods labeled with the instance of the Worker",
			cloudprem: "cloudprem.yaml",
			opw:       "opw-worker-name.yaml",
			edit:      func(*datadoghqv1alpha1.DatadogBYOCCluster) {},
			want:      []string{"Blocker: pipeline logs: the Pods of manifest logs have the app.kubernetes.io/instance label cloudprem-pipeline-logs of the Worker, so the Services, PodDisruptionBudget, and HorizontalPodAutoscaler of the Worker would also select them; rename the pipeline"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cloudprem := testManifest(t, "cloudprem", tt.cloudprem)
			pipelines := []PipelineManifest{{Pipeline: "logs", Manifest: testManifest(t, "logs", tt.opw)}}
			result, err := Convert(cloudprem, pipelines)
			if err != nil {
				t.Fatalf("Convert() unexpected error: %v", err)
			}
			tt.edit(result.Cluster)
			plan, err := NewPlan(result.Cluster, cloudprem, pipelines)
			if err != nil {
				t.Fatalf("NewPlan() unexpected error: %v", err)
			}

			findings := plan.Findings()
			var got []string
			for _, finding := range findings {
				if finding.Severity == SeverityBlocker {
					got = append(got, finding.String())
				}
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Findings() blockers mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// TestFindings_WithoutPipelineManifests checks a CloudPrem manifest converted without Observability Pipelines Worker
// manifests.
func TestFindings_WithoutPipelineManifests(t *testing.T) {
	tests := []struct {
		name         string
		edit         func(*datadoghqv1alpha1.DatadogBYOCCluster)
		want         []string
		wantPipeline []string
	}{
		{
			name: "no pipeline",
			edit: func(*datadoghqv1alpha1.DatadogBYOCCluster) {},
		},
		{
			name: "pipeline added by hand",
			edit: func(cluster *datadoghqv1alpha1.DatadogBYOCCluster) {
				cluster.Spec.ImageOverrides.ObservabilityPipelinesWorker = &datadoghqv1alpha1.DatadogBYOCImageSpec{Repository: new("gcr.io/datadoghq/observability-pipelines-worker"), Tag: new("2.23.0")}
				cluster.Spec.Components.Pipelines = []datadoghqv1alpha1.DatadogBYOCClusterPipelineSpec{{
					Name: "logs",
					DatadogBYOCClusterPipelineComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{
						PipelineID: new("pipeline-id"),
						Ports:      []datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerPort{{Name: "http", Port: 8088}},
					},
				}}
			},
			wantPipeline: []string{"logs: Worker cloudprem-pipeline-logs"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cloudprem := testManifest(t, "cloudprem", "cloudprem.yaml")
			result, err := Convert(cloudprem, nil)
			if err != nil {
				t.Fatalf("Convert() unexpected error: %v", err)
			}
			tt.edit(result.Cluster)
			plan, err := NewPlan(result.Cluster, cloudprem, nil)
			if err != nil {
				t.Fatalf("NewPlan() unexpected error: %v", err)
			}

			findings := plan.Findings()
			var got []string
			for _, finding := range findings {
				got = append(got, finding.String())
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Findings() mismatch (-want +got):\n%s", diff)
			}

			var gotPipelines []string
			for _, worker := range plan.Workers() {
				gotPipelines = append(gotPipelines, fmt.Sprintf("%s: Worker %s", worker.Pipeline, worker.Worker.Name))
			}
			if diff := cmp.Diff(tt.wantPipeline, gotPipelines); diff != "" {
				t.Errorf("Plan.Workers() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFindings_Namespaces(t *testing.T) {
	tests := []struct {
		name               string
		configMapNamespace string
		want               []string
	}{
		{
			name:               "objects in the namespace of the manifest",
			configMapNamespace: "logs",
		},
		{
			name:               "object in another namespace",
			configMapNamespace: "other",
			want:               []string{"Blocker: ConfigMap/cloudprem: the manifest puts it in namespace other, but the cluster is in namespace logs; run the command in the namespace of the manifest"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cloudprem := testManifest(t, "cloudprem", "cloudprem.yaml")
			pipelines := []PipelineManifest{{Pipeline: "logs", Manifest: testManifest(t, "logs", "opw.yaml")}}
			result, err := Convert(cloudprem, pipelines)
			if err != nil {
				t.Fatalf("Convert() unexpected error: %v", err)
			}
			configMap, _ := findObject(cloudprem.Objects, func(configMap *corev1.ConfigMap) bool { return configMap.Name == "cloudprem" })
			configMap.Namespace = tt.configMapNamespace
			plan, err := NewPlan(result.Cluster, cloudprem, pipelines)
			if err != nil {
				t.Fatalf("NewPlan() unexpected error: %v", err)
			}

			findings := plan.Findings()
			var got []string
			for _, finding := range findings {
				if finding.Severity == SeverityBlocker {
					got = append(got, finding.String())
				}
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("Findings() blockers mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNewPlan(t *testing.T) {
	tests := []struct {
		name      string
		cloudprem string
		opw       string
		edit      func(*datadoghqv1alpha1.DatadogBYOCCluster)
		want      []string
	}{
		{
			name:      "persistent volume, autoscaling, budgets, read-only metastore, and compactor",
			cloudprem: "cloudprem-full.yaml",
			opw:       "opw-full.yaml",
			edit:      func(*datadoghqv1alpha1.DatadogBYOCCluster) {},
			want: []string{
				"Adopt PodDisruptionBudget/cloudprem-metastore",
				"Adopt PodDisruptionBudget/cloudprem-searcher",
				"Adopt ServiceAccount/cloudprem",
				"Adopt ConfigMap/cloudprem",
				"Adopt Service/cloudprem-searcher",
				"Adopt Service/cloudprem-headless",
				"Adopt Service/cloudprem-indexer",
				"Adopt Service/cloudprem-metastore",
				"Replace Service/cloudprem-metastore-ro",
				"Adopt Service/cloudprem-control-plane",
				"Adopt Service/cloudprem-janitor",
				"Adopt Service/cloudprem-compactor",
				"Adopt Deployment/cloudprem-compactor",
				"Adopt Deployment/cloudprem-control-plane",
				"Adopt Deployment/cloudprem-janitor",
				"Adopt Deployment/cloudprem-metastore",
				"Replace Deployment/cloudprem-metastore-ro",
				"Adopt HorizontalPodAutoscaler/cloudprem-indexer",
				"Adopt HorizontalPodAutoscaler/cloudprem-compactor",
				"Adopt StatefulSet/cloudprem-indexer",
				"Adopt StatefulSet/cloudprem-searcher",
				"Keep Ingress/cloudprem-internal",
				"logs: Replace PodDisruptionBudget/logs-observability-pipelines-worker",
				"logs: Replace ServiceAccount/logs-observability-pipelines-worker",
				"logs: Replace Service/logs-observability-pipelines-worker-headless",
				"logs: Replace Service/logs-observability-pipelines-worker",
				"logs: Replace HorizontalPodAutoscaler/logs-observability-pipelines-worker",
				"logs: Replace StatefulSet/logs-observability-pipelines-worker",
			},
		},
		{
			name:      "budget disabled for a component with a PodDisruptionBudget",
			cloudprem: "cloudprem.yaml",
			opw:       "opw.yaml",
			edit: func(cluster *datadoghqv1alpha1.DatadogBYOCCluster) {
				cluster.Spec.Components.Metastore.PodDisruptionBudget = &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{}
			},
			want: []string{
				"Replace PodDisruptionBudget/cloudprem-metastore",
				"Adopt ServiceAccount/cloudprem",
				"Adopt ConfigMap/cloudprem",
				"Adopt Service/cloudprem-searcher",
				"Adopt Service/cloudprem-headless",
				"Adopt Service/cloudprem-indexer",
				"Adopt Service/cloudprem-metastore",
				"Adopt Service/cloudprem-control-plane",
				"Adopt Service/cloudprem-janitor",
				"Adopt Deployment/cloudprem-control-plane",
				"Adopt Deployment/cloudprem-janitor",
				"Adopt Deployment/cloudprem-metastore",
				"Adopt StatefulSet/cloudprem-indexer",
				"Adopt StatefulSet/cloudprem-searcher",
				"Keep Ingress/cloudprem-internal",
				"logs: Replace ServiceAccount/logs-observability-pipelines-worker",
				"logs: Replace Service/logs-observability-pipelines-worker-headless",
				"logs: Replace Service/logs-observability-pipelines-worker",
				"logs: Replace StatefulSet/logs-observability-pipelines-worker",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cloudprem := testManifest(t, "cloudprem", tt.cloudprem)
			pipelines := []PipelineManifest{{Pipeline: "logs", Manifest: testManifest(t, "logs", tt.opw)}}
			result, err := Convert(cloudprem, pipelines)
			if err != nil {
				t.Fatalf("Convert() unexpected error: %v", err)
			}
			tt.edit(result.Cluster)

			plan, err := NewPlan(result.Cluster, cloudprem, pipelines)
			if err != nil {
				t.Fatalf("NewPlan() unexpected error: %v", err)
			}
			var got []string
			for _, planned := range plan.CloudPremObjects {
				got = append(got, fmt.Sprintf("%s %s", planned.Action, objectName(planned.Object)))
			}
			for _, pipeline := range plan.Pipelines {
				for _, planned := range pipeline.Objects {
					got = append(got, fmt.Sprintf("%s: %s %s", pipeline.Pipeline, planned.Action, objectName(planned.Object)))
				}
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("NewPlan() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestNewPlan_Errors(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		cluster   string
		wantErr   string
	}{
		{
			name:      "cluster in another namespace than the manifest",
			namespace: "other",
			cluster:   "cloudprem",
			wantErr:   `DatadogBYOCCluster cloudprem is in namespace "other", but manifest cloudprem is in namespace "logs"`,
		},
		{
			name:      "cluster not named after the Helm fullname",
			namespace: "logs",
			cluster:   "renamed",
			wantErr:   "DatadogBYOCCluster renamed must be named cloudprem, after the Helm fullname of manifest cloudprem",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cloudprem := testManifest(t, "cloudprem", "cloudprem.yaml")
			pipelines := []PipelineManifest{{Pipeline: "logs", Manifest: testManifest(t, "logs", "opw.yaml")}}
			result, err := Convert(cloudprem, pipelines)
			if err != nil {
				t.Fatalf("Convert() unexpected error: %v", err)
			}
			result.Cluster.Namespace = tt.namespace
			result.Cluster.Name = tt.cluster

			_, err = NewPlan(result.Cluster, cloudprem, pipelines)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("NewPlan() error = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestConvert_Errors(t *testing.T) {
	tests := []struct {
		name      string
		cloudprem string
		pipelines []PipelineManifest
		wantErr   string
	}{
		{
			name:      "manifest without an indexer",
			cloudprem: "opw.yaml",
			pipelines: []PipelineManifest{{Pipeline: "logs", Manifest: testManifest(t, "logs", "opw.yaml")}},
			wantErr:   "manifest cloudprem has no indexer StatefulSet",
		},
		{
			name:      "two pipelines from the same manifest",
			cloudprem: "cloudprem.yaml",
			pipelines: []PipelineManifest{
				{Pipeline: "logs", Manifest: testManifest(t, "logs", "opw.yaml")},
				{Pipeline: "audit", Manifest: testManifest(t, "logs", "opw-full.yaml")},
			},
			wantErr: "pipelines logs and audit use the same manifest logs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Convert(testManifest(t, "cloudprem", tt.cloudprem), tt.pipelines)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Convert() error = %v, want error containing %q", err, tt.wantErr)
			}
		})
	}
}

// testManifest loads a manifest rendered with helm template.
func testManifest(t *testing.T, name, file string) Manifest {
	t.Helper()
	manifest, err := os.ReadFile(filepath.Join("testdata", file))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := NewManifest(name, "logs", string(manifest))
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func readGolden(t *testing.T, file string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", "golden", file))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
