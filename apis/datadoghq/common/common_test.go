package common

import (
	"testing"

	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/pkg/defaulting"
	"github.com/stretchr/testify/assert"
)

func Test_GetImage(t *testing.T) {
	tests := []struct {
		name      string
		imageSpec *commonv1.AgentImageConfig
		registry  *string
		want      string
	}{
		{
			name: "backward compatible",
			imageSpec: &commonv1.AgentImageConfig{
				Name: defaulting.GetLatestAgentImage(),
			},
			registry: nil,
			want:     defaulting.GetLatestAgentImage(),
		},
		{
			name: "nominal case",
			imageSpec: &commonv1.AgentImageConfig{
				Name: "agent",
				Tag:  "7",
			},
			registry: apiutils.NewStringPointer("public.ecr.aws/datadog"),
			want:     "public.ecr.aws/datadog/agent:7",
		},
		{
			name: "prioritize the full path",
			imageSpec: &commonv1.AgentImageConfig{
				Name: "docker.io/datadog/agent:7.28.1-rc.3",
				Tag:  "latest",
			},
			registry: apiutils.NewStringPointer("gcr.io/datadoghq"),
			want:     "docker.io/datadog/agent:7.28.1-rc.3",
		},
		{
			name: "default registry",
			imageSpec: &commonv1.AgentImageConfig{
				Name: "agent",
				Tag:  "latest",
			},
			registry: nil,
			want:     "gcr.io/datadoghq/agent:latest",
		},
		{
			name: "add jmx",
			imageSpec: &commonv1.AgentImageConfig{
				Name:       "agent",
				Tag:        defaulting.AgentLatestVersion,
				JMXEnabled: true,
			},
			registry: nil,
			want:     defaulting.GetLatestAgentImageJMX(),
		},
		{
			name: "cluster-agent",
			imageSpec: &commonv1.AgentImageConfig{
				Name:       "cluster-agent",
				Tag:        defaulting.ClusterAgentLatestVersion,
				JMXEnabled: false,
			},
			registry: nil,
			want:     defaulting.GetLatestClusterAgentImage(),
		},
		{
			name: "do not duplicate jmx",
			imageSpec: &commonv1.AgentImageConfig{
				Name:       "agent",
				Tag:        "latest-jmx",
				JMXEnabled: true,
			},
			registry: nil,
			want:     "gcr.io/datadoghq/agent:latest-jmx",
		},
		{
			name: "do not add jmx",
			imageSpec: &commonv1.AgentImageConfig{
				Name:       "agent",
				Tag:        "latest-jmx",
				JMXEnabled: true,
			},
			registry: nil,
			want:     "gcr.io/datadoghq/agent:latest-jmx",
		},
		{
			name: "image:tag and registry input, prefix registry",
			imageSpec: &commonv1.AgentImageConfig{
				Name: "agent:7",
				Tag:  "8",
			},
			registry: apiutils.NewStringPointer("docker.io/datadog"),
			want:     "docker.io/datadog/agent:7",
		},
		{
			name: "image:tag input, prefix default registry",
			imageSpec: &commonv1.AgentImageConfig{
				Name: "agent:7",
				Tag:  "8",
			},
			registry: nil,
			want:     "gcr.io/datadoghq/agent:7",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, GetImage(tt.imageSpec, tt.registry))
		})
	}
}
