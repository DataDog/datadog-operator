// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_SetNode(t *testing.T) {
	tests := []struct {
		name          string
		node          v1.Node
		existingNodes map[string]map[string]string
		wantNodes     map[string]map[string]string
	}{
		{
			name: "Set new node in empty node store",
			node: v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
					Labels: map[string]string{
						"some-label":  "val",
						"other-label": "val",
					},
				},
			},
			existingNodes: nil,
			wantNodes: map[string]map[string]string{
				"node-1": {
					"some-label":  "val",
					"other-label": "val",
				},
			},
		},
		{
			name: "Set new node in existing node store",
			node: v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-2",
					Labels: map[string]string{
						"some-label":  "val",
						"other-label": "val",
					},
				},
			},
			existingNodes: map[string]map[string]string{
				"foo": {
					"some-label":  "val",
					"other-label": "val",
				},
			},
			wantNodes: map[string]map[string]string{
				"foo": {
					"some-label":  "val",
					"other-label": "val",
				},
				"node-2": {
					"some-label":  "val",
					"other-label": "val",
				},
			},
		},
		{
			name: "Update node labels with new labels for existing node",
			node: v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
					Labels: map[string]string{
						"some-label":  "val",
						"other-label": "val",
						"new-label":   "new-val",
					},
				},
			},
			existingNodes: map[string]map[string]string{
				"node-1": {
					"some-label":  "val",
					"other-label": "val",
				},
			},
			wantNodes: map[string]map[string]string{
				"node-1": {
					"some-label":  "val",
					"other-label": "val",
					"new-label":   "new-val",
				},
			},
		},
		{
			name: "Update node labels with new label values for existing node",
			node: v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
					Labels: map[string]string{
						"some-label":  "val",
						"other-label": "new-val",
					},
				},
			},
			existingNodes: map[string]map[string]string{
				"node-1": {
					"some-label":  "val",
					"other-label": "val",
				},
			},
			wantNodes: map[string]map[string]string{
				"node-1": {
					"some-label":  "val",
					"other-label": "new-val",
				},
			},
		},
		{
			name: "Update node labels with new labels and values for existing node (more than 1 existing nodes)",
			node: v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-2",
					Labels: map[string]string{
						"some-label":  "val",
						"other-label": "new-val",
						"new-label":   "new-val",
					},
				},
			},
			existingNodes: map[string]map[string]string{
				"foo": {
					"some-label":  "val",
					"other-label": "val",
				},
				"node-2": {
					"some-label":  "val",
					"other-label": "val",
				},
			},
			wantNodes: map[string]map[string]string{
				"foo": {
					"some-label":  "val",
					"other-label": "val",
				},
				"node-2": {
					"some-label":  "val",
					"other-label": "new-val",
					"new-label":   "new-val",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeStore := NewNodeStore()
			if tt.existingNodes != nil && len(tt.existingNodes) > 0 {
				nodeStore.nodes = tt.existingNodes
			}
			nodeStore.SetNode(&tt.node)
			assert.EqualValues(t, tt.wantNodes, nodeStore.nodes)
		})
	}
}

func Test_GetNodes(t *testing.T) {
	tests := []struct {
		name      string
		wantNodes map[string]map[string]string
	}{
		{
			name:      "Get empty node store",
			wantNodes: map[string]map[string]string{},
		},
		{
			name: "Get node store with 1 existing node",
			wantNodes: map[string]map[string]string{
				"foo": {
					"some-label":  "val",
					"other-label": "val",
				},
			},
		},
		{
			name: "Get node store with multiple nodes",
			wantNodes: map[string]map[string]string{
				"foo": {
					"some-label":  "val",
					"other-label": "val",
				},
				"bar": {
					"some-label":  "val",
					"other-label": "new-val",
					"new-label":   "new-val",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeStore := NewNodeStore()
			if tt.wantNodes != nil && len(tt.wantNodes) > 0 {
				nodeStore.nodes = tt.wantNodes
			}
			gotNodes := nodeStore.GetNodes()
			assert.EqualValues(t, tt.wantNodes, gotNodes)
		})
	}
}

func Test_UnsetNodes(t *testing.T) {
	tests := []struct {
		name          string
		node          v1.Node
		existingNodes map[string]map[string]string
		wantNodes     map[string]map[string]string
	}{
		{
			name: "Unset node from empty node store",
			node: v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
					Labels: map[string]string{
						"some-label":  "val",
						"other-label": "new-val",
						"new-label":   "new-val",
					},
				},
			},
			existingNodes: nil,
			wantNodes:     map[string]map[string]string{},
		},
		{
			name: "Unset node from node store with 1 existing node",
			node: v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-1",
					Labels: map[string]string{
						"some-label":  "val",
						"other-label": "new-val",
						"new-label":   "new-val",
					},
				},
			},
			existingNodes: map[string]map[string]string{
				"node-1": {
					"some-label":  "val",
					"other-label": "val",
				},
			},
			wantNodes: map[string]map[string]string{},
		},
		{
			name: "Unset node from node store with multiple existing nodes",
			node: v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node-2",
					Labels: map[string]string{
						"some-label":  "val",
						"other-label": "val",
					},
				},
			},
			existingNodes: map[string]map[string]string{
				"foo": {
					"some-label":  "val",
					"other-label": "val",
				},
				"node-2": {
					"some-label":  "val",
					"other-label": "val",
				},
			},
			wantNodes: map[string]map[string]string{
				"foo": {
					"some-label":  "val",
					"other-label": "val",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nodeStore := NewNodeStore()
			if tt.existingNodes != nil && len(tt.existingNodes) > 0 {
				nodeStore.nodes = tt.existingNodes
			}
			nodeStore.UnsetNode(tt.node.Name)
			assert.EqualValues(t, tt.wantNodes, nodeStore.nodes)
		})
	}
}
