// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestBuildObservabilityPipelinesWorker_Args(t *testing.T) {
	t.Run("args copied", func(t *testing.T) {
		cluster := testCluster()
		pipeline := &cluster.Spec.Components.Pipelines[0]
		pipeline.Args = []string{"run", "--config", "/etc/x.yaml"}

		worker, err := BuildObservabilityPipelinesWorker(cluster, pipeline, testImages().ObservabilityPipelinesWorker)
		if err != nil {
			t.Fatalf("BuildObservabilityPipelinesWorker() unexpected error: %v", err)
		}
		if diff := cmp.Diff([]string{"run", "--config", "/etc/x.yaml"}, worker.Spec.Args); diff != "" {
			t.Errorf("worker args mismatch (-want +got):\n%s", diff)
		}
	})

	t.Run("not aliased", func(t *testing.T) {
		cluster := testCluster()
		pipeline := &cluster.Spec.Components.Pipelines[0]
		pipeline.Args = []string{"run", "--config", "/etc/x.yaml"}

		worker, err := BuildObservabilityPipelinesWorker(cluster, pipeline, testImages().ObservabilityPipelinesWorker)
		if err != nil {
			t.Fatalf("BuildObservabilityPipelinesWorker() unexpected error: %v", err)
		}
		pipeline.Args[1] = "mutated"
		if worker.Spec.Args[1] != "--config" {
			t.Errorf("worker args share memory with the cluster: got %v", worker.Spec.Args)
		}
	})
}
