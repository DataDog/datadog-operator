// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDatadogAgentBuilder_WithKSMTags(t *testing.T) {
	tags := []string{"env:prod", "team:cont-p"}
	dda := NewDatadogAgentBuilder().WithKSMEnabled(true).WithKSMTags(tags).Build()

	assert.NotNil(t, dda.Spec.Features.KubeStateMetricsCore)
	assert.Equal(t, tags, dda.Spec.Features.KubeStateMetricsCore.Tags)
}

func TestDatadogAgentBuilder_WithKSMLabelsAsTags(t *testing.T) {
	m := map[string]map[string]string{
		"pod":  {"app": "app"},
		"node": {"zone": "zone"},
	}
	dda := NewDatadogAgentBuilder().WithKSMEnabled(true).WithKSMLabelsAsTags(m).Build()

	assert.NotNil(t, dda.Spec.Features.KubeStateMetricsCore)
	assert.Equal(t, m, dda.Spec.Features.KubeStateMetricsCore.LabelsAsTags)
}

func TestDatadogAgentBuilder_WithKSMAnnotationsAsTags(t *testing.T) {
	m := map[string]map[string]string{
		"pod": {"tags_datadoghq_com_version": "version"},
	}
	dda := NewDatadogAgentBuilder().WithKSMEnabled(true).WithKSMAnnotationsAsTags(m).Build()

	assert.NotNil(t, dda.Spec.Features.KubeStateMetricsCore)
	assert.Equal(t, m, dda.Spec.Features.KubeStateMetricsCore.AnnotationsAsTags)
}
