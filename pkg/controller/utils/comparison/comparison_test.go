// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package comparison

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

func newBenchSpec() *v2alpha1.DatadogAgentSpec {
	envVars := make([]corev1.EnvVar, 0, 20)
	for i := range 20 {
		envVars = append(envVars, corev1.EnvVar{
			Name:  fmt.Sprintf("DD_ENV_%02d", i),
			Value: fmt.Sprintf("value-%02d", i),
		})
	}
	return &v2alpha1.DatadogAgentSpec{
		Global: &v2alpha1.GlobalConfig{
			ClusterName: ptr.To("prod-cluster-01"),
			Site:        ptr.To("datadoghq.com"),
			Registry:    ptr.To("gcr.io/datadoghq"),
			Tags:        []string{"env:prod", "region:us-east-1", "cluster:prod-cluster-01"},
		},
		Features: &v2alpha1.DatadogFeatures{
			APM:           &v2alpha1.APMFeatureConfig{Enabled: ptr.To(true)},
			LogCollection: &v2alpha1.LogCollectionFeatureConfig{Enabled: ptr.To(true), ContainerCollectAll: ptr.To(true)},
			NPM:           &v2alpha1.NPMFeatureConfig{Enabled: ptr.To(true)},
			USM:           &v2alpha1.USMFeatureConfig{Enabled: ptr.To(true)},
			CWS:           &v2alpha1.CWSFeatureConfig{Enabled: ptr.To(true)},
		},
		Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
			v2alpha1.NodeAgentComponentName: {
				Image: &v2alpha1.AgentImageConfig{Tag: "7.78.0"},
				Env:   envVars,
				Containers: map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{
					apicommon.CoreAgentContainerName: {
						Resources: &corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("200m"),
								corev1.ResourceMemory: resource.MustParse("256Mi"),
							},
						},
					},
				},
			},
		},
	}
}

func TestGenerateMD5ForSpec(t *testing.T) {
	spec := newBenchSpec()
	got, err := GenerateMD5ForSpec(spec)
	require.NoError(t, err)

	b, err := json.Marshal(spec)
	require.NoError(t, err)
	sum := md5.Sum(b)
	want := hex.EncodeToString(sum[:])
	assert.Equal(t, want, got)

	// Verify exact match with json.Marshal + md5 for nil, empty, plain, HTML, escaped, and unicode maps.
	mapCases := []map[string]string{
		nil,
		{},
		{"z": "1", "a": "2", "m": "3"},
		{"html": "<tag>&\"val\"\\path\nline"},
		{"unicode": "café-\u2028-\u2029"},
	}
	for _, mc := range mapCases {
		gotMap, err := GenerateMD5ForSpec(mc)
		require.NoError(t, err)
		mb, err := json.Marshal(mc)
		require.NoError(t, err)
		msum := md5.Sum(mb)
		assert.Equal(t, hex.EncodeToString(msum[:]), gotMap)
	}

	// Verify error propagation on unmarshalable value
	_, err = GenerateMD5ForSpec(func() {})
	assert.Error(t, err)
}

func BenchmarkGenerateMD5ForSpec_DatadogAgentSpec(b *testing.B) {
	spec := newBenchSpec()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := GenerateMD5ForSpec(spec); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGenerateMD5ForSpec_LabelsMap(b *testing.B) {
	labels := map[string]string{
		"agent.datadoghq.com/component":            "agent",
		"agent.datadoghq.com/name":                 "datadog",
		"agent.datadoghq.com/datadogagentprofile":  "profile-pool-01",
		"app.kubernetes.io/instance":               "datadog-agent",
		"app.kubernetes.io/managed-by":             "datadog-operator",
		"app.kubernetes.io/name":                   "datadog-agent-deployment",
		"app.kubernetes.io/part-of":                "datadog-agent",
		"env":                                      "prod",
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := GenerateMD5ForSpec(labels); err != nil {
			b.Fatal(err)
		}
	}
}
