// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func TestRESTEndpoint(t *testing.T) {
	tests := []struct {
		name      string
		cluster   string
		namespace string
		want      string
	}{
		{name: "default", cluster: "cloudprem", namespace: "logs", want: "http://cloudprem-searcher.logs.svc:7280"},
		{name: "other namespace", cluster: "byoc", namespace: "datadog", want: "http://byoc-searcher.datadog.svc:7280"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster := &datadoghqv1alpha1.DatadogBYOCCluster{ObjectMeta: metav1.ObjectMeta{Name: tt.cluster, Namespace: tt.namespace}}
			if got := RESTEndpoint(cluster); got != tt.want {
				t.Errorf("RESTEndpoint() = %q, want %q", got, tt.want)
			}
		})
	}
}
