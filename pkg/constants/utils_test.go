// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
package constants

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/pkg/defaulting"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_GetImage(t *testing.T) {
	emptyRegistry := ""
	tests := []struct {
		name      string
		imageSpec *v2alpha1.AgentImageConfig
		registry  *string
		want      string
	}{
		{
			name: "backward compatible",
			imageSpec: &v2alpha1.AgentImageConfig{
				Name: defaulting.GetLatestAgentImage(),
			},
			registry: nil,
			want:     defaulting.GetLatestAgentImage(),
		},
		{
			name: "nominal case",
			imageSpec: &v2alpha1.AgentImageConfig{
				Name: "agent",
				Tag:  "7",
			},
			registry: apiutils.NewStringPointer("public.ecr.aws/datadog"),
			want:     "public.ecr.aws/datadog/agent:7",
		},
		{
			name: "prioritize the full path",
			imageSpec: &v2alpha1.AgentImageConfig{
				Name: "docker.io/datadog/agent:7.28.1-rc.3",
				Tag:  "latest",
			},
			registry: apiutils.NewStringPointer("gcr.io/datadoghq"),
			want:     "docker.io/datadog/agent:7.28.1-rc.3",
		},
		{
			name: "default registry",
			imageSpec: &v2alpha1.AgentImageConfig{
				Name: "agent",
				Tag:  "latest",
			},
			registry: &emptyRegistry,
			want:     "gcr.io/datadoghq/agent:latest",
		},
		{
			name: "add jmx",
			imageSpec: &v2alpha1.AgentImageConfig{
				Name:       "agent",
				Tag:        defaulting.AgentLatestVersion,
				JMXEnabled: true,
			},
			registry: nil,
			want:     defaulting.GetLatestAgentImageJMX(),
		},
		{
			name: "cluster-agent",
			imageSpec: &v2alpha1.AgentImageConfig{
				Name:       "cluster-agent",
				Tag:        defaulting.ClusterAgentLatestVersion,
				JMXEnabled: false,
			},
			registry: nil,
			want:     defaulting.GetLatestClusterAgentImage(),
		},
		{
			name: "do not duplicate jmx",
			imageSpec: &v2alpha1.AgentImageConfig{
				Name:       "agent",
				Tag:        "latest-jmx",
				JMXEnabled: true,
			},
			registry: nil,
			want:     "gcr.io/datadoghq/agent:latest-jmx",
		},
		{
			name: "do not add jmx",
			imageSpec: &v2alpha1.AgentImageConfig{
				Name:       "agent",
				Tag:        "latest-jmx",
				JMXEnabled: true,
			},
			registry: nil,
			want:     "gcr.io/datadoghq/agent:latest-jmx",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, GetImage(tt.imageSpec, tt.registry))
		})
	}
}

func TestServiceAccountNameOverride(t *testing.T) {
	customServiceAccount := "fake"
	ddaName := "test-dda"
	tests := []struct {
		name string
		dda  *v2alpha1.DatadogAgent
		want map[v2alpha1.ComponentName]string
	}{
		{
			name: "custom serviceaccount for dca and clc",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: v1.ObjectMeta{
					Name: ddaName,
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.ClusterAgentComponentName: {
							ServiceAccountName: &customServiceAccount,
						},
						v2alpha1.ClusterChecksRunnerComponentName: {
							ServiceAccountName: &customServiceAccount,
						},
					},
				},
			},
			want: map[v2alpha1.ComponentName]string{
				v2alpha1.ClusterAgentComponentName:        customServiceAccount,
				v2alpha1.NodeAgentComponentName:           fmt.Sprintf("%s-%s", ddaName, DefaultAgentResourceSuffix),
				v2alpha1.ClusterChecksRunnerComponentName: customServiceAccount,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := map[v2alpha1.ComponentName]string{}
			res[v2alpha1.NodeAgentComponentName] = GetAgentServiceAccount(tt.dda)
			res[v2alpha1.ClusterChecksRunnerComponentName] = GetClusterChecksRunnerServiceAccount(tt.dda)
			res[v2alpha1.ClusterAgentComponentName] = GetClusterAgentServiceAccount(tt.dda)
			for name, sa := range tt.want {
				if res[name] != sa {
					t.Errorf("Service Account Override error = %v, want %v", res[name], tt.want[name])
				}
			}
		})
	}
}

func TestServiceAccountAnnotationOverride(t *testing.T) {
	customServiceAccount := "fake"
	customServiceAccountAnnotations := map[string]string{
		"eks.amazonaws.com/role-arn": "arn:aws:iam::123456789012:role/datadog-role",
		"really.important":           "annotation",
	}
	ddaName := "test-dda"
	tests := []struct {
		name string
		dda  *v2alpha1.DatadogAgent
		want map[v2alpha1.ComponentName]map[string]interface{}
	}{
		{
			name: "custom serviceaccount annotations for dda, dca and clc",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: v1.ObjectMeta{
					Name: ddaName,
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.ClusterAgentComponentName: {
							ServiceAccountName:        &customServiceAccount,
							ServiceAccountAnnotations: customServiceAccountAnnotations,
						},
						v2alpha1.ClusterChecksRunnerComponentName: {
							ServiceAccountAnnotations: customServiceAccountAnnotations,
						},
						v2alpha1.NodeAgentComponentName: {
							ServiceAccountAnnotations: customServiceAccountAnnotations,
						},
					},
				},
			},
			want: map[v2alpha1.ComponentName]map[string]interface{}{
				v2alpha1.ClusterAgentComponentName: {
					"name":        customServiceAccount,
					"annotations": customServiceAccountAnnotations,
				},
				v2alpha1.NodeAgentComponentName: {
					"name":        fmt.Sprintf("%s-%s", ddaName, DefaultAgentResourceSuffix),
					"annotations": customServiceAccountAnnotations,
				},
				v2alpha1.ClusterChecksRunnerComponentName: {
					"name":        fmt.Sprintf("%s-%s", ddaName, DefaultClusterChecksRunnerResourceSuffix),
					"annotations": customServiceAccountAnnotations,
				},
			},
		},
		{
			name: "custom serviceaccount annotations for dca",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: v1.ObjectMeta{
					Name: ddaName,
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.ClusterAgentComponentName: {
							ServiceAccountName:        &customServiceAccount,
							ServiceAccountAnnotations: customServiceAccountAnnotations,
						},
					},
				},
			},
			want: map[v2alpha1.ComponentName]map[string]interface{}{
				v2alpha1.NodeAgentComponentName: {
					"name":        fmt.Sprintf("%s-%s", ddaName, DefaultAgentResourceSuffix),
					"annotations": map[string]string{},
				},
				v2alpha1.ClusterAgentComponentName: {
					"name":        customServiceAccount,
					"annotations": customServiceAccountAnnotations,
				},
				v2alpha1.ClusterChecksRunnerComponentName: {
					"name":        fmt.Sprintf("%s-%s", ddaName, DefaultClusterChecksRunnerResourceSuffix),
					"annotations": map[string]string{},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := map[v2alpha1.ComponentName]map[string]interface{}{
				v2alpha1.NodeAgentComponentName: {
					"name":        GetAgentServiceAccount(tt.dda),
					"annotations": GetAgentServiceAccountAnnotations(tt.dda),
				},
				v2alpha1.ClusterChecksRunnerComponentName: {
					"name":        GetClusterChecksRunnerServiceAccount(tt.dda),
					"annotations": GetClusterChecksRunnerServiceAccountAnnotations(tt.dda),
				},
				v2alpha1.ClusterAgentComponentName: {
					"name":        GetClusterAgentServiceAccount(tt.dda),
					"annotations": GetClusterAgentServiceAccountAnnotations(tt.dda),
				},
			}
			for componentName, sa := range tt.want {
				if res[componentName]["name"] != sa["name"] {
					t.Errorf("Service Account Override Name error = %v, want %v", res[componentName], tt.want[componentName])
				}
				if !mapsEqual(res[componentName]["annotations"].(map[string]string), sa["annotations"].(map[string]string)) {
					t.Errorf("Service Account Override Annotation error = %v, want %v", res[componentName], tt.want[componentName])
				}
			}
		})
	}
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if bValue, ok := b[key]; !ok || value != bValue {
			return false
		}
	}
	return true
}
