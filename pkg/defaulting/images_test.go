// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaulting

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetLatestAgentImage(t *testing.T) {
	tests := []struct {
		name string
		opts []ImageOptions
		want string
	}{
		{
			name: "default registry",
			want: fmt.Sprintf("gcr.io/datadoghq/agent:%s", AgentLatestVersion),
		},

		{
			name: "docker.io",
			opts: []ImageOptions{
				WithRegistry(DockerHubContainerRegistry),
			},
			want: fmt.Sprintf("docker.io/datadog/agent:%s", AgentLatestVersion),
		},
		{
			name: "with tag",
			opts: []ImageOptions{
				WithTag("latest"),
			},
			want: "gcr.io/datadoghq/agent:latest",
		},
		{
			name: "with image name",
			opts: []ImageOptions{
				WithImageName("foo"),
				WithTag("latest"),
			},
			want: "gcr.io/datadoghq/foo:latest",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetLatestAgentImage(tt.opts...); got != tt.want {
				t.Errorf("GetLatestAgentImage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetLatestClusterAgentImage(t *testing.T) {
	tests := []struct {
		name string
		opts []ImageOptions
		want string
	}{
		{
			name: "default registry",
			want: fmt.Sprintf("gcr.io/datadoghq/cluster-agent:%s", ClusterAgentLatestVersion),
		},

		{
			name: "docker.io",
			opts: []ImageOptions{
				WithRegistry(DockerHubContainerRegistry),
			},
			want: fmt.Sprintf("docker.io/datadog/cluster-agent:%s", ClusterAgentLatestVersion),
		},
		{
			name: "with tag",
			opts: []ImageOptions{
				WithTag("latest"),
			},
			want: "gcr.io/datadoghq/cluster-agent:latest",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetLatestClusterAgentImage(tt.opts...); got != tt.want {
				t.Errorf("GetLatestClusterAgentImage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewImage(t *testing.T) {
	type args struct {
		name  string
		tag   string
		isJMX bool
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "default",
			args: args{
				name: "foo",
				tag:  "bar",
			},
			want: "gcr.io/datadoghq/foo:bar",
		},
		{
			name: "jmx option",
			args: args{
				name:  "foo",
				tag:   "bar",
				isJMX: true,
			},
			want: "gcr.io/datadoghq/foo:bar-jmx",
		},
		{
			name: "jmx tag",
			args: args{
				name: "foo",
				tag:  "bar-jmx",
			},
			want: "gcr.io/datadoghq/foo:bar-jmx",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewImage(tt.args.name, tt.args.tag, tt.args.isJMX).String(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewImage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsImageNameContainsTag(t *testing.T) {
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
		assert.Equal(t, expected, IsImageNameContainsTag(tc))
	}
}
