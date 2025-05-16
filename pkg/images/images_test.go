// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package images

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

func TestGetLatestAgentImage(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{
			name: "default registry",
			want: fmt.Sprintf("gcr.io/datadoghq/agent:%s", AgentLatestVersion),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetLatestAgentImage(); got != tt.want {
				t.Errorf("GetLatestAgentImage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetLatestClusterAgentImage(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{
			name: "default registry",
			want: fmt.Sprintf("gcr.io/datadoghq/cluster-agent:%s", ClusterAgentLatestVersion),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetLatestClusterAgentImage(); got != tt.want {
				t.Errorf("GetLatestClusterAgentImage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_newImage(t *testing.T) {
	type args struct {
		registry string
		name     string
		tag      string
		isJMX    bool
		isFIPS   bool
		isFull   bool
	}
	tests := []struct {
		name string
		args args
		want *Image
	}{
		{
			name: "default",
			args: args{
				name: "foo",
				tag:  "bar",
			},
			want: &Image{
				registry: "gcr.io/datadoghq",
				name:     "foo",
				tag:      "bar",
			},
		},
		{
			name: "jmx option",
			args: args{
				name:  "foo",
				tag:   "bar",
				isJMX: true,
			},
			want: &Image{
				registry: "gcr.io/datadoghq",
				name:     "foo",
				tag:      "bar",
				isJMX:    true,
			},
		},
		{
			name: "fips suffix",
			args: args{
				name:   "foo",
				tag:    "bar",
				isFIPS: true,
			},
			want: &Image{
				registry: "gcr.io/datadoghq",
				name:     "foo",
				tag:      "bar",
				isFIPS:   true,
			},
		},
		{
			name: "jmx and fips suffix",
			args: args{
				name:   "foo",
				tag:    "bar",
				isJMX:  true,
				isFIPS: true,
			},
			want: &Image{
				registry: "gcr.io/datadoghq",
				name:     "foo",
				tag:      "bar",
				isJMX:    true,
				isFIPS:   true,
			},
		},
		{
			name: "full suffix",
			args: args{
				name:   "foo",
				tag:    "bar",
				isJMX:  false,
				isFIPS: false,
				isFull: true,
			},
			want: &Image{
				registry: "gcr.io/datadoghq",
				name:     "foo",
				tag:      "bar",
				isJMX:    false,
				isFIPS:   false,
				isFull:   true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newImage("gcr.io/datadoghq", tt.args.name, tt.args.tag, tt.args.isJMX, tt.args.isFIPS, tt.args.isFull); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("newImage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_imageNameContainsTag(t *testing.T) {
	cases := map[string]bool{
		"foo:bar":             true,
		"foo/bar:baz":         true,
		"foo/bar:baz:tar":     true,
		"foo/bar:baz-tar":     true,
		"foo/bar:baz_tar":     true,
		"foo/bar:baz.tar":     true,
		"foo/foo/bar:baz:tar": true,
		"foo":                 false,
		":foo":                false,
		"foo:foo/bar":         false,
	}
	for tc, expected := range cases {
		assert.Equal(t, expected, imageNameContainsTag(tc))
	}
}

func Test_AssembleImage(t *testing.T) {
	tests := []struct {
		name      string
		imageSpec *v2alpha1.AgentImageConfig
		registry  string
		want      string
	}{
		{
			name: "backward compatible",
			imageSpec: &v2alpha1.AgentImageConfig{
				Name: GetLatestAgentImage(),
			},
			want: GetLatestAgentImage(),
		},
		{
			name: "nominal case",
			imageSpec: &v2alpha1.AgentImageConfig{
				Name: "agent",
				Tag:  "7",
			},
			registry: "public.ecr.aws/datadog",
			want:     "public.ecr.aws/datadog/agent:7",
		},
		{
			name: "prioritize the full path",
			imageSpec: &v2alpha1.AgentImageConfig{
				Name: "docker.io/datadog/agent:7.28.1-rc.3",
				Tag:  "latest",
			},
			registry: "gcr.io/datadoghq",
			want:     "docker.io/datadog/agent:7.28.1-rc.3",
		},
		{
			name: "default registry",
			imageSpec: &v2alpha1.AgentImageConfig{
				Name: "agent",
				Tag:  "latest",
			},
			want: "gcr.io/datadoghq/agent:latest",
		},
		{
			name: "cluster-agent",
			imageSpec: &v2alpha1.AgentImageConfig{
				Name:       "cluster-agent",
				Tag:        ClusterAgentLatestVersion,
				JMXEnabled: false,
			},
			want: GetLatestClusterAgentImage(),
		},
		{
			name: "do not duplicate jmx",
			imageSpec: &v2alpha1.AgentImageConfig{
				Name:       "agent",
				Tag:        "latest-jmx",
				JMXEnabled: true,
			},
			want: "gcr.io/datadoghq/agent:latest-jmx",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, AssembleImage(tt.imageSpec, tt.registry))
		})
	}
}

// Test ToString
func Test_ToString(t *testing.T) {
	tests := []struct {
		name  string
		image *Image
		want  string
	}{
		{
			name: "default",
			image: &Image{
				registry: "gcr.io/datadoghq",
				name:     "agent",
				tag:      "7.64.0",
			},
			want: "gcr.io/datadoghq/agent:7.64.0",
		},
		{
			name: "with jmx",
			image: &Image{
				registry: "gcr.io/datadoghq",
				name:     "agent",
				tag:      "7.64.0",
				isJMX:    true,
			},
			want: "gcr.io/datadoghq/agent:7.64.0-jmx",
		},
		{
			name: "with fips",
			image: &Image{
				registry: "gcr.io/datadoghq",
				name:     "agent",
				tag:      "7.64.0",
				isFIPS:   true,
			},
			want: "gcr.io/datadoghq/agent:7.64.0-fips",
		},
		{
			name: "with jmx and fips",
			image: &Image{
				registry: "gcr.io/datadoghq",
				name:     "agent",
				tag:      "7.64.0",
				isJMX:    true,
				isFIPS:   true,
			},
			want: "gcr.io/datadoghq/agent:7.64.0-fips-jmx",
		},
		{
			name: "with full",
			image: &Image{
				registry: "gcr.io/datadoghq",
				name:     "agent",
				tag:      "7.64.0",
				isJMX:    false,
				isFIPS:   false,
				isFull:   true,
			},
			want: "gcr.io/datadoghq/agent:7.64.0-full",
		},
		{
			name: "with full and jmx",
			image: &Image{
				registry: "gcr.io/datadoghq",
				name:     "agent",
				tag:      "7.64.0",
				isJMX:    true,
				isFIPS:   false,
				isFull:   true,
			},
			want: "gcr.io/datadoghq/agent:7.64.0-full",
		},
		{
			name: "with full and fips",
			image: &Image{
				registry: "gcr.io/datadoghq",
				name:     "agent",
				tag:      "7.64.0",
				isJMX:    false,
				isFIPS:   true,
				isFull:   true,
			},
			want: "gcr.io/datadoghq/agent:7.64.0-fips",
		},
		{
			name: "with full, jmx and fips",
			image: &Image{
				registry: "gcr.io/datadoghq",
				name:     "agent",
				tag:      "7.64.0",
				isJMX:    true,
				isFIPS:   true,
				isFull:   true,
			},
			want: "gcr.io/datadoghq/agent:7.64.0-fips-jmx",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.image.ToString())
		})
	}
}

// Test FromString
func Test_FromString(t *testing.T) {
	tests := []struct {
		name        string
		imageString string
		want        *Image
	}{
		{
			name:        "default",
			imageString: "gcr.io/datadoghq/agent:7.64.0",
			want: &Image{
				registry: "gcr.io/datadoghq",
				name:     "agent",
				tag:      "7.64.0",
			},
		},
		{
			name:        "with jmx",
			imageString: "gcr.io/datadoghq/agent:7.64.0-jmx",
			want: &Image{
				registry: "gcr.io/datadoghq",
				name:     "agent",
				tag:      "7.64.0",
				isJMX:    true,
			},
		},
		{
			name:        "with fips",
			imageString: "gcr.io/datadoghq/agent:7.64.0-fips",
			want: &Image{
				registry: "gcr.io/datadoghq",
				name:     "agent",
				tag:      "7.64.0",
				isFIPS:   true,
			},
		},
		{
			name:        "with jmx and fips",
			imageString: "gcr.io/datadoghq/agent:7.64.0-fips-jmx",
			want: &Image{
				registry: "gcr.io/datadoghq",
				name:     "agent",
				tag:      "7.64.0",
				isJMX:    true,
				isFIPS:   true,
			},
		},
		{
			name:        "with full",
			imageString: "gcr.io/datadoghq/agent:7.64.0-full",
			want: &Image{
				registry: "gcr.io/datadoghq",
				name:     "agent",
				tag:      "7.64.0",
				isFull:   true,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, FromString(tt.imageString))
		})
	}
}

// Test OverrideAgentImage
func Test_OverrideAgentImage(t *testing.T) {
	tests := []struct {
		name              string
		currentImage      string
		overrideImageSpec *v2alpha1.AgentImageConfig
		want              string
	}{
		{
			name:         "override image name",
			currentImage: "gcr.io/datadoghq/agent:7.64.0",
			overrideImageSpec: &v2alpha1.AgentImageConfig{
				Name: "custom-agent",
				Tag:  "7.64.0",
			},
			want: "gcr.io/datadoghq/custom-agent:7.64.0",
		},
		{
			name:         "override image tag",
			currentImage: "gcr.io/datadoghq/agent:7.64.0",
			overrideImageSpec: &v2alpha1.AgentImageConfig{
				Name: "agent",
				Tag:  "latest",
			},
			want: "gcr.io/datadoghq/agent:latest",
		},
		{
			name:         "override image with jmx suffix",
			currentImage: "gcr.io/datadoghq/agent:7.64.0",
			overrideImageSpec: &v2alpha1.AgentImageConfig{
				Name:       "agent",
				Tag:        "7.64.0",
				JMXEnabled: true,
			},
			want: "gcr.io/datadoghq/agent:7.64.0-jmx",
		},
		{
			name:         "override image with jmx and tag suffix",
			currentImage: "gcr.io/datadoghq/agent:7.64.0",
			overrideImageSpec: &v2alpha1.AgentImageConfig{
				Name:       "agent",
				Tag:        "7.64.0-jmx",
				JMXEnabled: true,
			},
			want: "gcr.io/datadoghq/agent:7.64.0-jmx",
		},
		{
			name:         "override image tag and jmx suffix",
			currentImage: "gcr.io/datadoghq/agent:7.64.0",
			overrideImageSpec: &v2alpha1.AgentImageConfig{
				Name:       "agent",
				Tag:        "latest",
				JMXEnabled: true,
			},
			want: "gcr.io/datadoghq/agent:latest-jmx",
		},
		{
			name:         "override image name is full name",
			currentImage: "gcr.io/datadoghq/agent:7.64.0",
			overrideImageSpec: &v2alpha1.AgentImageConfig{
				Name: "docker.io/datadog/agent:latest",
			},
			want: "docker.io/datadog/agent:latest",
		},
		{
			name:         "override image name is full name, ignore tag",
			currentImage: "gcr.io/datadoghq/agent:7.64.0",
			overrideImageSpec: &v2alpha1.AgentImageConfig{
				Name: "docker.io/datadog/agent:latest",
				Tag:  "other-tag",
			},
			want: "docker.io/datadog/agent:latest",
		},
		{
			name:         "override image name is full name, ignore JMX",
			currentImage: "gcr.io/datadoghq/agent:7.64.0",
			overrideImageSpec: &v2alpha1.AgentImageConfig{
				Name:       "docker.io/datadog/agent:latest",
				JMXEnabled: true,
			},
			want: "docker.io/datadog/agent:latest",
		},
		{
			name:         "override image name is name:tag",
			currentImage: "gcr.io/datadoghq/agent:7.64.0",
			overrideImageSpec: &v2alpha1.AgentImageConfig{
				Name: "agent:latest",
			},
			want: "gcr.io/datadoghq/agent:latest",
		},
		{
			name:         "override image name is name:tag, ignore tag",
			currentImage: "gcr.io/datadoghq/agent:7.64.0",
			overrideImageSpec: &v2alpha1.AgentImageConfig{
				Name: "agent:latest",
				Tag:  "other-tag",
			},
			want: "gcr.io/datadoghq/agent:latest",
		},
		{
			name:         "override image name is name:tag, ignore JMX",
			currentImage: "gcr.io/datadoghq/agent:7.64.0",
			overrideImageSpec: &v2alpha1.AgentImageConfig{
				Name:       "agent:latest",
				JMXEnabled: true,
			},
			want: "gcr.io/datadoghq/agent:latest",
		},
		{
			name:         "current image includes FIPS suffix and override enables jmx",
			currentImage: "gcr.io/datadoghq/agent:7.64.0-fips",
			overrideImageSpec: &v2alpha1.AgentImageConfig{
				JMXEnabled: true,
			},
			want: "gcr.io/datadoghq/agent:7.64.0-fips-jmx",
		},
		{
			name:         "current image includes FIPS suffix and override also includes FIPS suffix",
			currentImage: "gcr.io/datadoghq/agent:7.64.0-fips",
			overrideImageSpec: &v2alpha1.AgentImageConfig{
				Name:       "agent:7.65.0-fips",
				JMXEnabled: true,
			},
			want: "gcr.io/datadoghq/agent:7.65.0-fips",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, OverrideAgentImage(tt.currentImage, tt.overrideImageSpec))
		})
	}
}
