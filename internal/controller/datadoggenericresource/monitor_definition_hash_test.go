// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"testing"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func TestMonitorDefinitionHashSuppliedFields(t *testing.T) {
	instance := &v1alpha1.DatadogGenericResource{
		Spec: v1alpha1.DatadogGenericResourceSpec{
			JsonSpec: `{"query":"q","type":"query alert","future_setting":{"enabled":true}}`,
		},
	}
	original := datadogV1.Monitor{
		Query:                "q",
		Type:                 datadogV1.MONITORTYPE_QUERY_ALERT,
		AdditionalProperties: map[string]any{"future_setting": map[string]any{"enabled": true}},
	}
	edited := original
	edited.AdditionalProperties = map[string]any{"future_setting": map[string]any{"enabled": false}}

	baseline := monitorDefinitionHash(instance, original)
	require.NotEmpty(t, baseline)
	assert.NotEqual(t, baseline, monitorDefinitionHash(instance, edited), "supplied unknown fields participate in drift detection")

	instance.Spec.JsonSpec = `{"query":"q","type":"query alert"}`
	baseline = monitorDefinitionHash(instance, original)
	require.NotEmpty(t, baseline)
	assert.Equal(t, baseline, monitorDefinitionHash(instance, edited), "unspecified response-only fields remain ignored")
}

func TestMonitorDefinitionHashUnobservableFieldDisablesSkip(t *testing.T) {
	instance := &v1alpha1.DatadogGenericResource{
		Spec: v1alpha1.DatadogGenericResourceSpec{JsonSpec: `{"future_setting":null}`},
	}
	monitor := datadogV1.Monitor{Query: "q", Type: datadogV1.MONITORTYPE_QUERY_ALERT}
	assert.Empty(t, monitorDefinitionHash(instance, monitor), "a missing supplied field cannot establish a trusted baseline")

	monitor.AdditionalProperties = map[string]any{"future_setting": nil}
	assert.NotEmpty(t, monitorDefinitionHash(instance, monitor), "explicit null is observable and differs from a missing field")
}
