package kubernetes

import (
	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"testing"
)

var (
	nodeUID1 = generateRandomNodeUUID()
	nodeUID2 = generateRandomNodeUUID()
)

func Test_SetOrUpdateNode(t *testing.T) {
	tests := []struct {
		name               string
		node               v1.Node
		existingNodeLabels map[string]map[string]string
		wantNodeLabels     map[string]map[string]string
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
					UID: types.UID(nodeUID1),
				},
			},
			existingNodeLabels: nil,
			wantNodeLabels: map[string]map[string]string{
				nodeUID1: {
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
					UID: types.UID(nodeUID2),
				},
			},
			existingNodeLabels: map[string]map[string]string{
				nodeUID1: {
					"some-label":  "val",
					"other-label": "val",
				},
			},
			wantNodeLabels: map[string]map[string]string{
				nodeUID1: {
					"some-label":  "val",
					"other-label": "val",
				},
				nodeUID2: {
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
					UID: types.UID(nodeUID1),
				},
			},
			existingNodeLabels: map[string]map[string]string{
				nodeUID1: {
					"some-label":  "val",
					"other-label": "val",
				},
			},
			wantNodeLabels: map[string]map[string]string{
				nodeUID1: {
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
					UID: types.UID(nodeUID1),
				},
			},
			existingNodeLabels: map[string]map[string]string{
				nodeUID1: {
					"some-label":  "val",
					"other-label": "val",
				},
			},
			wantNodeLabels: map[string]map[string]string{
				nodeUID1: {
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
					UID: types.UID(nodeUID2),
				},
			},
			existingNodeLabels: map[string]map[string]string{
				nodeUID1: {
					"some-label":  "val",
					"other-label": "val",
				},
				nodeUID2: {
					"some-label":  "val",
					"other-label": "val",
				},
			},
			wantNodeLabels: map[string]map[string]string{
				nodeUID1: {
					"some-label":  "val",
					"other-label": "val",
				},
				nodeUID2: {
					"some-label":  "val",
					"other-label": "new-val",
					"new-label":   "new-val",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logf.Log.WithName(t.Name())
			nodeStore := NewNodeStore(logger)
			if tt.existingNodeLabels != nil && len(tt.existingNodeLabels) > 0 {
				nodeStore.nodeLabels = tt.existingNodeLabels
			}
			nodeStore.SetOrUpdateNode(&tt.node)
			assert.EqualValues(t, tt.wantNodeLabels, nodeStore.nodeLabels)
		})
	}
}

func Test_GetNodes(t *testing.T) {
	tests := []struct {
		name      string
		wantNodes *NodeStore
	}{
		{
			name:      "Get empty node store",
			wantNodes: &NodeStore{nodeLabels: map[string]map[string]string{}},
		},
		{
			name: "Get node store with 1 existing node",
			wantNodes: &NodeStore{
				nodeLabels: map[string]map[string]string{
					nodeUID1: {
						"some-label":  "val",
						"other-label": "val",
					},
				},
			},
		},
		{
			name: "Get node store with multiple nodes",
			wantNodes: &NodeStore{
				nodeLabels: map[string]map[string]string{
					nodeUID1: {
						"some-label":  "val",
						"other-label": "val",
					},
					nodeUID2: {
						"some-label":  "val",
						"other-label": "new-val",
						"new-label":   "new-val",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logf.Log.WithName(t.Name())
			nodeStore := NewNodeStore(logger)
			if tt.wantNodes != nil && len(tt.wantNodes.nodeLabels) > 0 {
				nodeStore = tt.wantNodes
			}
			gotNodesLabels := nodeStore.GetNodes()
			assert.EqualValues(t, tt.wantNodes.nodeLabels, gotNodesLabels)
		})
	}
}

func Test_UnsetNodes(t *testing.T) {
	tests := []struct {
		name          string
		node          v1.Node
		existingNodes *NodeStore
		wantNodes     *NodeStore
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
					UID: types.UID(nodeUID1),
				},
			},
			existingNodes: nil,
			wantNodes: &NodeStore{
				nodeLabels: map[string]map[string]string{},
				log:        logr.Logger{},
			},
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
					UID: types.UID(nodeUID1),
				},
			},
			existingNodes: &NodeStore{
				nodeLabels: map[string]map[string]string{
					nodeUID1: {
						"some-label":  "val",
						"other-label": "val",
					},
				},
			},
			wantNodes: &NodeStore{nodeLabels: map[string]map[string]string{}},
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
					UID: types.UID(nodeUID2),
				},
			},
			existingNodes: &NodeStore{
				nodeLabels: map[string]map[string]string{
					nodeUID1: {
						"some-label":  "val",
						"other-label": "val",
					},
					nodeUID2: {
						"some-label":  "val",
						"other-label": "val",
					},
				},
			},
			wantNodes: &NodeStore{
				nodeLabels: map[string]map[string]string{
					nodeUID1: {
						"some-label":  "val",
						"other-label": "val",
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := logr.Logger{}
			nodeStore := NewNodeStore(logger)
			if tt.existingNodes != nil && len(tt.existingNodes.nodeLabels) > 0 {
				nodeStore = tt.existingNodes
			}
			nodeStore.UnsetNode(string(tt.node.UID))
			assert.EqualValues(t, tt.wantNodes, nodeStore)
		})
	}
}

func generateRandomNodeUUID() string {
	nodeUUID := uuid.New()
	return nodeUUID.String()
}
