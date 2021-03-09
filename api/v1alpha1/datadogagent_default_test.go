// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"path"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestDefaultConfigDogstatsd(t *testing.T) {
	defaultPath := path.Join(defaultHostDogstatsdSocketPath, defaultHostDogstatsdSocketName)
	defaultAgentConfig := NodeAgentConfig{
		Dogstatsd: &DogstatsdConfig{
			DogstatsdOriginDetection: NewBoolPointer(false),
			UnixDomainSocket: &DSDUnixDomainSocketSpec{
				Enabled:      NewBoolPointer(false),
				HostFilepath: &defaultPath,
			},
		},
	}

	type args struct {
		config *NodeAgentConfig
	}
	tests := []struct {
		name string
		args args
		want *NodeAgentConfig
	}{
		{
			name: "dogtatsd not set",
			args: args{
				config: &NodeAgentConfig{},
			},
			want: &defaultAgentConfig,
		},
		{
			name: "dogtatsd missing defaulting: DogstatsdOriginDetection",
			args: args{
				config: &NodeAgentConfig{
					Dogstatsd: &DogstatsdConfig{
						UnixDomainSocket: &DSDUnixDomainSocketSpec{
							Enabled: NewBoolPointer(false),
						},
					},
				},
			},
			want: &defaultAgentConfig,
		},
		{
			name: "dogtatsd missing defaulting: UseDogStatsDSocketVolume",
			args: args{
				config: &NodeAgentConfig{
					Dogstatsd: &DogstatsdConfig{
						DogstatsdOriginDetection: NewBoolPointer(false),
					},
				},
			},
			want: &defaultAgentConfig,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			DefaultConfigDogstatsd(tt.args.config)
			if diff := cmp.Diff(tt.args.config, tt.want); diff != "" {
				t.Errorf("DefaultConfigDogstatsd err, diff:\n %s", diff)
			}
		})
	}
}
