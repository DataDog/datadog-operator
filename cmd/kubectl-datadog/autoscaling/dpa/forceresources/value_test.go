// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package forceresources

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
)

// The cases below extend TestParseForceResourcesAnnotation in datadog-agent:
// the CLI rejects what the agent ignores, as a whole or per value.
func TestParseValueMatchesTheAgent(t *testing.T) {
	f, err := parseValue(`[{"name": "app", "requests": {"cpu": "2", "memory": "200Mi"}, "limits": {"cpu": "4"}}, {"name": "sidecar", "limits": {"memory": "1Gi"}}]`)
	require.NoError(t, err)
	require.Len(t, f, 2)
	assert.Equal(t, "app", f[0].Name, "the annotation order is kept")
	assert.True(t, f[0].Requests[corev1.ResourceCPU].Equal(resource.MustParse("2")))
	assert.True(t, f[0].Limits[corev1.ResourceCPU].Equal(resource.MustParse("4")))
	assert.NotContains(t, f[0].Limits, corev1.ResourceMemory)
	assert.Equal(t, "sidecar", f[1].Name)
	assert.NotContains(t, f[1].Requests, corev1.ResourceMemory)

	f, err = parseValue("")
	assert.NoError(t, err)
	assert.Empty(t, f)

	for name, value := range map[string]string{
		// Ignored as a whole by the agent.
		"empty list":       `[]`,
		"bad JSON":         `[{"name": `,
		"object, not list": `{"app": {"cpu": {"request": "1"}}}`,
		"invalid quantity": `[{"name": "app", "requests": {"memory": "200Mb"}}]`,
		// Skipped by the agent.
		"empty container name": `[{"name": "", "requests": {"cpu": "1"}}]`,
		"no resource":          `[{"name": "app"}]`,
		"unsupported resource": `[{"name": "app", "requests": {"nvidia.com/gpu": "1"}}]`,
		"zero quantity":        `[{"name": "app", "requests": {"cpu": "0"}}]`,
		"negative quantity":    `[{"name": "app", "limits": {"cpu": "-1"}}]`,
		"runtime":              `[{"name": "app", "requests": {"cpu": "1"}, "runtime": {}}]`,
		// Silently ignored or adjusted by the agent.
		"unknown field":              `[{"name": "app", "request": {"cpu": "1"}}]`,
		"duplicate container":        `[{"name": "app", "requests": {"cpu": "1"}}, {"name": "app", "limits": {"cpu": "2"}}]`,
		"request greater than limit": `[{"name": "app", "requests": {"cpu": "4"}, "limits": {"cpu": "2"}}]`,
		"one invalid container":      `[{"name": "app", "requests": {"cpu": "1"}}, {"name": "other", "requests": {"memory": "0"}}]`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseValue(value)
			assert.Error(t, err)
		})
	}
}

// Every value the CLI writes must be accepted by the agent parser.
func TestValueRoundTrip(t *testing.T) {
	requests, err := parseResourceList("cpu=500m,memory=1Gi")
	require.NoError(t, err)
	limits, err := parseResourceList("cpu=1")
	require.NoError(t, err)

	v, err := forcedResources{}.merge([]string{"app", "sidecar"}, requests, limits).value()
	require.NoError(t, err)
	value := v.Value
	assert.Equal(t, `[{"name":"app","limits":{"cpu":"1"},"requests":{"cpu":"500m","memory":"1Gi"}},{"name":"sidecar","limits":{"cpu":"1"},"requests":{"cpu":"500m","memory":"1Gi"}}]`, value)

	var agent []datadoghqcommon.DatadogPodAutoscalerContainerResources
	require.NoError(t, json.Unmarshal([]byte(value), &agent), "parseForceResourcesAnnotation accepts it")
	_, err = parseValue(value)
	assert.NoError(t, err)
}

func TestParseResourceList(t *testing.T) {
	list, err := parseResourceList("cpu=2, memory=512Mi")
	require.NoError(t, err)
	assert.Equal(t, corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2"), corev1.ResourceMemory: resource.MustParse("512Mi")}, list)

	list, err = parseResourceList("")
	assert.NoError(t, err)
	assert.Nil(t, list)

	for name, spec := range map[string]string{
		"missing =":        "cpu",
		"invalid quantity": "memory=200Mb",
		"duplicate":        "cpu=1, cpu=2",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseResourceList(spec)
			assert.Error(t, err)
		})
	}
}
