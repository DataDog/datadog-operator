// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	apiutils "github.com/DataDog/datadog-operator/api/crds/utils"
	"github.com/DataDog/datadog-operator/pkg/defaulting"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_GetImage(t *testing.T) {
	emptyRegistry := ""
	tests := []struct {
		name      string
		imageSpec *AgentImageConfig
		registry  *string
		want      string
	}{
		{
			name: "backward compatible",
			imageSpec: &AgentImageConfig{
				Name: defaulting.GetLatestAgentImage(),
			},
			registry: nil,
			want:     defaulting.GetLatestAgentImage(),
		},
		{
			name: "nominal case",
			imageSpec: &AgentImageConfig{
				Name: "agent",
				Tag:  "7",
			},
			registry: apiutils.NewStringPointer("public.ecr.aws/datadog"),
			want:     "public.ecr.aws/datadog/agent:7",
		},
		{
			name: "prioritize the full path",
			imageSpec: &AgentImageConfig{
				Name: "docker.io/datadog/agent:7.28.1-rc.3",
				Tag:  "latest",
			},
			registry: apiutils.NewStringPointer("gcr.io/datadoghq"),
			want:     "docker.io/datadog/agent:7.28.1-rc.3",
		},
		{
			name: "default registry",
			imageSpec: &AgentImageConfig{
				Name: "agent",
				Tag:  "latest",
			},
			registry: &emptyRegistry,
			want:     "gcr.io/datadoghq/agent:latest",
		},
		{
			name: "add jmx",
			imageSpec: &AgentImageConfig{
				Name:       "agent",
				Tag:        defaulting.AgentLatestVersion,
				JMXEnabled: true,
			},
			registry: nil,
			want:     defaulting.GetLatestAgentImageJMX(),
		},
		{
			name: "cluster-agent",
			imageSpec: &AgentImageConfig{
				Name:       "cluster-agent",
				Tag:        defaulting.ClusterAgentLatestVersion,
				JMXEnabled: false,
			},
			registry: nil,
			want:     defaulting.GetLatestClusterAgentImage(),
		},
		{
			name: "do not duplicate jmx",
			imageSpec: &AgentImageConfig{
				Name:       "agent",
				Tag:        "latest-jmx",
				JMXEnabled: true,
			},
			registry: nil,
			want:     "gcr.io/datadoghq/agent:latest-jmx",
		},
		{
			name: "do not add jmx",
			imageSpec: &AgentImageConfig{
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
		dda  *DatadogAgent
		want map[ComponentName]string
	}{
		{
			name: "custom serviceaccount for dca and clc",
			dda: &DatadogAgent{
				ObjectMeta: v1.ObjectMeta{
					Name: ddaName,
				},
				Spec: DatadogAgentSpec{
					Override: map[ComponentName]*DatadogAgentComponentOverride{
						ClusterAgentComponentName: {
							ServiceAccountName: &customServiceAccount,
						},
						ClusterChecksRunnerComponentName: {
							ServiceAccountName: &customServiceAccount,
						},
					},
				},
			},
			want: map[ComponentName]string{
				ClusterAgentComponentName:        customServiceAccount,
				NodeAgentComponentName:           fmt.Sprintf("%s-%s", ddaName, DefaultAgentResourceSuffix),
				ClusterChecksRunnerComponentName: customServiceAccount,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := map[ComponentName]string{}
			res[NodeAgentComponentName] = GetAgentServiceAccount(tt.dda)
			res[ClusterChecksRunnerComponentName] = GetClusterChecksRunnerServiceAccount(tt.dda)
			res[ClusterAgentComponentName] = GetClusterAgentServiceAccount(tt.dda)
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
		dda  *DatadogAgent
		want map[ComponentName]map[string]interface{}
	}{
		{
			name: "custom serviceaccount annotations for dda, dca and clc",
			dda: &DatadogAgent{
				ObjectMeta: v1.ObjectMeta{
					Name: ddaName,
				},
				Spec: DatadogAgentSpec{
					Override: map[ComponentName]*DatadogAgentComponentOverride{
						ClusterAgentComponentName: {
							ServiceAccountName:        &customServiceAccount,
							ServiceAccountAnnotations: customServiceAccountAnnotations,
						},
						ClusterChecksRunnerComponentName: {
							ServiceAccountAnnotations: customServiceAccountAnnotations,
						},
						NodeAgentComponentName: {
							ServiceAccountAnnotations: customServiceAccountAnnotations,
						},
					},
				},
			},
			want: map[ComponentName]map[string]interface{}{
				ClusterAgentComponentName: {
					"name":        customServiceAccount,
					"annotations": customServiceAccountAnnotations,
				},
				NodeAgentComponentName: {
					"name":        fmt.Sprintf("%s-%s", ddaName, DefaultAgentResourceSuffix),
					"annotations": customServiceAccountAnnotations,
				},
				ClusterChecksRunnerComponentName: {
					"name":        fmt.Sprintf("%s-%s", ddaName, DefaultClusterChecksRunnerResourceSuffix),
					"annotations": customServiceAccountAnnotations,
				},
			},
		},
		{
			name: "custom serviceaccount annotations for dca",
			dda: &DatadogAgent{
				ObjectMeta: v1.ObjectMeta{
					Name: ddaName,
				},
				Spec: DatadogAgentSpec{
					Override: map[ComponentName]*DatadogAgentComponentOverride{
						ClusterAgentComponentName: {
							ServiceAccountName:        &customServiceAccount,
							ServiceAccountAnnotations: customServiceAccountAnnotations,
						},
					},
				},
			},
			want: map[ComponentName]map[string]interface{}{
				NodeAgentComponentName: {
					"name":        fmt.Sprintf("%s-%s", ddaName, DefaultAgentResourceSuffix),
					"annotations": map[string]string{},
				},
				ClusterAgentComponentName: {
					"name":        customServiceAccount,
					"annotations": customServiceAccountAnnotations,
				},
				ClusterChecksRunnerComponentName: {
					"name":        fmt.Sprintf("%s-%s", ddaName, DefaultClusterChecksRunnerResourceSuffix),
					"annotations": map[string]string{},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := map[ComponentName]map[string]interface{}{
				NodeAgentComponentName: {
					"name":        GetAgentServiceAccount(tt.dda),
					"annotations": GetAgentServiceAccountAnnotations(tt.dda),
				},
				ClusterChecksRunnerComponentName: {
					"name":        GetClusterChecksRunnerServiceAccount(tt.dda),
					"annotations": GetClusterChecksRunnerServiceAccountAnnotations(tt.dda),
				},
				ClusterAgentComponentName: {
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
