// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package experimental

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

func TestGetAutopilotAllowlistVersionAnnotation(t *testing.T) {
	tests := []struct {
		name       string
		annotation string
		want       string
	}{
		{name: "no annotation", annotation: "", want: ""},
		{name: "explicit override", annotation: "v1.2.3", want: "v1.2.3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dda := &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{}},
			}
			if tt.annotation != "" {
				dda.Annotations[getExperimentalAnnotationKey(ExperimentalAutopilotAllowlistVersionSubkey)] = tt.annotation
			}
			got := getExperimentalAnnotation(dda, ExperimentalAutopilotAllowlistVersionSubkey)
			assert.Equal(t, tt.want, got)
		})
	}
}
