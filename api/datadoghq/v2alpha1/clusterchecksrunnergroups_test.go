// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newTestObjectWithAnnotations(annotations map[string]string) *DatadogAgent {
	return &DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "datadog",
			Namespace:   "default",
			Annotations: annotations,
		},
	}
}

func TestGetEffectiveClusterChecksRunnerGroups(t *testing.T) {
	annotationGroups := []ClusterChecksRunnerGroup{
		{Name: "kafka", ChecksInclude: []string{"kafka_consumer"}},
	}
	rawAnnotationGroups, err := json.Marshal(annotationGroups)
	require.NoError(t, err)

	tests := []struct {
		name        string
		annotations map[string]string
		wantErr     bool
		want        []ClusterChecksRunnerGroup
	}{
		{
			name:        "no annotations",
			annotations: nil,
			want:        nil,
		},
		{
			name: "knob off: annotation groups only",
			annotations: map[string]string{
				AnnotationExperimentalClusterChecksRunnerGroups: string(rawAnnotationGroups),
			},
			want: annotationGroups,
		},
		{
			name: "knob on and no annotation groups: built-in kube group only",
			annotations: map[string]string{
				AnnotationExperimentalKubeChecksRunnerDefault: "true",
			},
			want: []ClusterChecksRunnerGroup{
				{
					Name:          KubeChecksRunnerGroupName,
					ChecksInclude: KubeChecksRunnerGroupChecksInclude,
					Override: &DatadogAgentComponentOverride{
						Replicas: new(int32(2)),
					},
				},
			},
		},
		{
			name: "knob on with annotation groups: kube group prepended",
			annotations: map[string]string{
				AnnotationExperimentalKubeChecksRunnerDefault:   "true",
				AnnotationExperimentalClusterChecksRunnerGroups: string(rawAnnotationGroups),
			},
			want: append([]ClusterChecksRunnerGroup{
				{
					Name:          KubeChecksRunnerGroupName,
					ChecksInclude: KubeChecksRunnerGroupChecksInclude,
					Override: &DatadogAgentComponentOverride{
						Replicas: new(int32(2)),
					},
				},
			}, annotationGroups...),
		},
		{
			name: "user-declared kube group replaces the built-in",
			annotations: map[string]string{
				AnnotationExperimentalKubeChecksRunnerDefault:   "true",
				AnnotationExperimentalClusterChecksRunnerGroups: `[{"name":"kube","checksInclude":["kubernetes_state_core"],"override":{"replicas":5}}]`,
			},
			want: []ClusterChecksRunnerGroup{
				{
					Name:          "kube",
					ChecksInclude: []string{"kubernetes_state_core"},
					Override: &DatadogAgentComponentOverride{
						Replicas: new(int32(5)),
					},
				},
			},
		},
		{
			name: "malformed groups annotation errors",
			annotations: map[string]string{
				AnnotationExperimentalClusterChecksRunnerGroups: "not-json",
			},
			wantErr: true,
		},
		{
			name:        "invalid group name errors",
			annotations: map[string]string{AnnotationExperimentalClusterChecksRunnerGroups: `[{"name":"Not_DNS","checksInclude":["a"]}]`},
			wantErr:     true,
		},
		{
			name:        "duplicate group names error",
			annotations: map[string]string{AnnotationExperimentalClusterChecksRunnerGroups: `[{"name":"a","checksInclude":["x"]},{"name":"a","checksInclude":["y"]}]`},
			wantErr:     true,
		},
		{
			name:        "group without checksInclude errors",
			annotations: map[string]string{AnnotationExperimentalClusterChecksRunnerGroups: `[{"name":"a"}]`},
			wantErr:     true,
		},
		{
			name:        "check claimed by two groups errors",
			annotations: map[string]string{AnnotationExperimentalClusterChecksRunnerGroups: `[{"name":"a","checksInclude":["x"]},{"name":"b","checksInclude":["x"]}]`},
			wantErr:     true,
		},
		{
			name: "user group overlapping the built-in kube group errors",
			annotations: map[string]string{
				AnnotationExperimentalKubeChecksRunnerDefault:   "true",
				AnnotationExperimentalClusterChecksRunnerGroups: `[{"name":"ksm","checksInclude":["kubernetes_state_core"]}]`,
			},
			wantErr: true,
		},
		{
			name: "knob off ignores nothing extra",
			annotations: map[string]string{
				AnnotationExperimentalKubeChecksRunnerDefault: "false",
			},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetEffectiveClusterChecksRunnerGroups(newTestObjectWithAnnotations(tt.annotations))
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
