// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
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

func TestServiceAccountOverride(t *testing.T) {
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
				NodeAgentComponentName:           fmt.Sprintf("%s-%s", ddaName, common.DefaultAgentResourceSuffix),
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
