// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"path"
	"reflect"
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

func TestDefaultFeatures(t *testing.T) {
	defaultWant := &DatadogFeatures{
		OrchestratorExplorer: &OrchestratorExplorerConfig{
			Enabled: NewBoolPointer(true),
			Scrubbing: &Scrubbing{
				Containers: NewBoolPointer(true),
			},
		},
		KubeStateMetricsCore: &KubeStateMetricsCore{Enabled: NewBoolPointer(false), ClusterCheck: NewBoolPointer(false)},
		PrometheusScrape: &PrometheusScrapeConfig{
			Enabled:          NewBoolPointer(false),
			ServiceEndpoints: NewBoolPointer(false),
		},
		LogCollection: &LogCollectionConfig{
			Enabled:                       NewBoolPointer(false),
			LogsConfigContainerCollectAll: NewBoolPointer(false),
			ContainerCollectUsingFiles:    NewBoolPointer(true),
			ContainerLogsPath:             NewStringPointer("/var/lib/docker/containers"),
			PodLogsPath:                   NewStringPointer("/var/log/pods"),
			TempStoragePath:               NewStringPointer("/var/lib/datadog-agent/logs"),
			OpenFilesLimit:                NewInt32Pointer(100),
		},
	}

	tests := []struct {
		name     string
		ft       *DatadogFeatures
		wantFunc func() *DatadogFeatures
	}{
		{
			name:     "empty",
			ft:       &DatadogFeatures{},
			wantFunc: func() *DatadogFeatures { return defaultWant },
		},
		{
			name: "enable LogCollection",
			ft: &DatadogFeatures{
				LogCollection: &LogCollectionConfig{Enabled: NewBoolPointer(true)},
			},
			wantFunc: func() *DatadogFeatures {
				want := defaultWant.DeepCopy()
				want.LogCollection.Enabled = NewBoolPointer(true)
				return want
			},
		},
		{
			name: "disable Orchestrator",
			ft: &DatadogFeatures{
				OrchestratorExplorer: &OrchestratorExplorerConfig{Enabled: NewBoolPointer(false)},
			},
			wantFunc: func() *DatadogFeatures {
				want := defaultWant.DeepCopy()
				want.OrchestratorExplorer = &OrchestratorExplorerConfig{Enabled: NewBoolPointer(false)}

				return want
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.wantFunc()
			if got := DefaultFeatures(tt.ft); !reflect.DeepEqual(got, want) {
				t.Errorf("DefaultFeatures() = %v, want %v\n diff: %s", got, want, cmp.Diff(got, want))
			}
		})
	}
}
