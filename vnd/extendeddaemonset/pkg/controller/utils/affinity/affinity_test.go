// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package affinity

import (
	"testing"

	"github.com/stretchr/testify/assert"

	v1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
)

func TestReplaceNodeNameNodeAffinity(t *testing.T) {
	nodeName := "foo-node"
	nodeNameSelReq := v1.NodeSelectorRequirement{
		Key:      NodeFieldSelectorKeyNodeName,
		Operator: v1.NodeSelectorOpIn,
		Values:   []string{nodeName},
	}

	nodeLabelExcludeSelReq := v1.NodeSelectorRequirement{
		Key:      "extendeddaemonset.datadoghq.com/exclude",
		Operator: v1.NodeSelectorOpIn,
		Values:   []string{"true"},
	}

	type args struct {
		affinity *v1.Affinity
		nodename string
	}
	tests := []struct {
		name string
		args args
		want *v1.Affinity
	}{
		{
			name: "empty affinity",
			args: args{
				affinity: &v1.Affinity{},
				nodename: nodeName,
			},
			want: &v1.Affinity{
				NodeAffinity: &v1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
						NodeSelectorTerms: []v1.NodeSelectorTerm{
							{
								MatchFields: []v1.NodeSelectorRequirement{nodeNameSelReq},
							},
						},
					},
				},
			},
		},
		{
			name: "affinity container another term",
			args: args{
				affinity: &v1.Affinity{
					NodeAffinity: &v1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
							NodeSelectorTerms: []v1.NodeSelectorTerm{
								{
									MatchExpressions: []v1.NodeSelectorRequirement{nodeLabelExcludeSelReq},
								},
							},
						},
					},
				},
				nodename: nodeName,
			},
			want: &v1.Affinity{
				NodeAffinity: &v1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
						NodeSelectorTerms: []v1.NodeSelectorTerm{
							{
								MatchFields:      []v1.NodeSelectorRequirement{nodeNameSelReq},
								MatchExpressions: []v1.NodeSelectorRequirement{nodeLabelExcludeSelReq},
							},
						},
					},
				},
			},
		},
		{
			name: "override node name field selector if already set",
			args: args{
				affinity: &v1.Affinity{
					NodeAffinity: &v1.NodeAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
							NodeSelectorTerms: []v1.NodeSelectorTerm{
								{
									MatchFields: []v1.NodeSelectorRequirement{
										{
											Key:      NodeFieldSelectorKeyNodeName,
											Operator: v1.NodeSelectorOpIn,
											Values:   []string{"previous-node-name"},
										},
									},
									MatchExpressions: []v1.NodeSelectorRequirement{nodeLabelExcludeSelReq},
								},
							},
						},
					},
				},
				nodename: nodeName,
			},
			want: &v1.Affinity{
				NodeAffinity: &v1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
						NodeSelectorTerms: []v1.NodeSelectorTerm{
							{
								MatchFields:      []v1.NodeSelectorRequirement{nodeNameSelReq},
								MatchExpressions: []v1.NodeSelectorRequirement{nodeLabelExcludeSelReq},
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ReplaceNodeNameNodeAffinity(tt.args.affinity, tt.args.nodename); !apiequality.Semantic.DeepEqual(got, tt.want) {
				t.Errorf("ReplaceNodeNameNodeAffinity() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetNodeNameFromAffinity(t *testing.T) {
	// nil case
	got := GetNodeNameFromAffinity(nil)
	assert.Equal(t, "", got)

	// empty case
	affinity := &v1.Affinity{}
	got = GetNodeNameFromAffinity(affinity)
	assert.Equal(t, "", got)

	// non-nil case
	nodeName := "foo-node"
	nodeNameSelReq := v1.NodeSelectorRequirement{
		Key:      NodeFieldSelectorKeyNodeName,
		Operator: v1.NodeSelectorOpIn,
		Values:   []string{nodeName},
	}

	affinity = &v1.Affinity{
		NodeAffinity: &v1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
				NodeSelectorTerms: []v1.NodeSelectorTerm{
					{
						MatchFields: []v1.NodeSelectorRequirement{nodeNameSelReq},
					},
				},
			},
		},
	}
	got = GetNodeNameFromAffinity(affinity)
	assert.Equal(t, "foo-node", got)
}
