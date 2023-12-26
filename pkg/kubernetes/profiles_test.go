package kubernetes

import (
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
		name          string
		node          v1.Node
		existingNodes *NodeStore
		wantNodes     *NodeStore
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
			existingNodes: nil,
			wantNodes: &NodeStore{
				nodes: v1.NodeList{
					Items: []v1.Node{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "node-1",
								Labels: map[string]string{
									"some-label":  "val",
									"other-label": "val",
								},
								UID: types.UID(nodeUID1),
							},
						},
					},
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
			existingNodes: &NodeStore{
				nodes: v1.NodeList{
					Items: []v1.Node{
						{ObjectMeta: metav1.ObjectMeta{
							Name: "node-1",
							Labels: map[string]string{
								"some-label":  "val",
								"other-label": "val",
							},
							UID: types.UID(nodeUID1),
						}},
					},
				},
			},
			wantNodes: &NodeStore{
				nodes: v1.NodeList{
					Items: []v1.Node{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "node-1",
								Labels: map[string]string{
									"some-label":  "val",
									"other-label": "val",
								},
								UID: types.UID(nodeUID1),
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "node-2",
								Labels: map[string]string{
									"some-label":  "val",
									"other-label": "val",
								},
								UID: types.UID(nodeUID2),
							},
						},
					},
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
			existingNodes: &NodeStore{
				nodes: v1.NodeList{
					Items: []v1.Node{
						{ObjectMeta: metav1.ObjectMeta{
							Name: "node-1",
							Labels: map[string]string{
								"some-label":  "val",
								"other-label": "val",
							},
							UID: types.UID(nodeUID1),
						}},
					},
				},
			},
			wantNodes: &NodeStore{
				nodes: v1.NodeList{
					Items: []v1.Node{
						{ObjectMeta: metav1.ObjectMeta{
							Name: "node-1",
							Labels: map[string]string{
								"some-label":  "val",
								"other-label": "val",
								"new-label":   "new-val",
							},
							UID: types.UID(nodeUID1),
						},
						},
					},
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
			existingNodes: &NodeStore{
				nodes: v1.NodeList{
					Items: []v1.Node{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "node-1",
								Labels: map[string]string{
									"some-label":  "val",
									"other-label": "val",
								},
								UID: types.UID(nodeUID1),
							},
						},
					},
				},
			},
			wantNodes: &NodeStore{
				nodes: v1.NodeList{
					Items: []v1.Node{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "node-1",
								Labels: map[string]string{
									"some-label":  "val",
									"other-label": "new-val",
								},
								UID: types.UID(nodeUID1),
							},
						},
					},
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
			existingNodes: &NodeStore{
				nodes: v1.NodeList{
					Items: []v1.Node{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "node-1",
								Labels: map[string]string{
									"some-label":  "val",
									"other-label": "val",
								},
								UID: types.UID(nodeUID1),
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "node-2",
								Labels: map[string]string{
									"some-label":  "val",
									"other-label": "val",
								},
								UID: types.UID(nodeUID2),
							},
						},
					},
				},
			},
			wantNodes: &NodeStore{
				nodes: v1.NodeList{
					Items: []v1.Node{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "node-1",
								Labels: map[string]string{
									"some-label":  "val",
									"other-label": "val",
								},
								UID: types.UID(nodeUID1),
							},
						},
						{
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
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var nodeStore *NodeStore
			logger := logf.Log.WithName(t.Name())
			if tt.existingNodes != nil && len(tt.existingNodes.nodes.Items) > 0 {
				nodeStore = tt.existingNodes
				nodeStore.log = logger
			} else {
				nodeStore = NewNodeStore(logger)
			}
			nodeStore.SetOrUpdateNode(&tt.node)
			assert.EqualValues(t, tt.wantNodes.nodes, nodeStore.nodes)
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
			wantNodes: &NodeStore{nodes: v1.NodeList{}},
		},
		{
			name: "Get node store with 1 existing node",
			wantNodes: &NodeStore{
				nodes: v1.NodeList{
					Items: []v1.Node{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "node-1",
								Labels: map[string]string{
									"some-label":  "val",
									"other-label": "val",
								},
								UID: types.UID(nodeUID1),
							},
						},
					},
				},
			},
		},
		{
			name: "Get node store with multiple nodes",
			wantNodes: &NodeStore{nodes: v1.NodeList{
				Items: []v1.Node{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "node-1",
							Labels: map[string]string{
								"some-label":  "val",
								"other-label": "val",
							},
							UID: types.UID(nodeUID1),
						},
					},
					{
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
				},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var nodeStore *NodeStore
			logger := logf.Log.WithName(t.Name())
			if tt.wantNodes != nil && len(tt.wantNodes.nodes.Items) > 0 {
				nodeStore = tt.wantNodes
				nodeStore.log = logger
			} else {
				nodeStore = NewNodeStore(logger)
			}
			gotNodes := nodeStore.GetNodes()
			assert.EqualValues(t, tt.wantNodes.nodes, gotNodes)
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
			wantNodes:     &NodeStore{nodes: v1.NodeList{}},
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
				nodes: v1.NodeList{
					Items: []v1.Node{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "node-1",
								Labels: map[string]string{
									"some-label":  "val",
									"other-label": "val",
								},
								UID: types.UID(nodeUID1),
							},
						},
					},
				},
			},
			wantNodes: &NodeStore{nodes: v1.NodeList{Items: []v1.Node{}}},
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
				nodes: v1.NodeList{
					Items: []v1.Node{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "node-1",
								Labels: map[string]string{
									"some-label":  "val",
									"other-label": "val",
								},
								UID: types.UID(nodeUID1),
							},
						},
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "node-2",
								Labels: map[string]string{
									"some-label":  "val",
									"other-label": "val",
								},
								UID: types.UID(nodeUID2),
							},
						},
					},
				},
			},
			wantNodes: &NodeStore{
				nodes: v1.NodeList{
					Items: []v1.Node{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name: "node-1",
								Labels: map[string]string{
									"some-label":  "val",
									"other-label": "val",
								},
								UID: types.UID(nodeUID1),
							},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var nodeStore *NodeStore
			logger := logf.Log.WithName(t.Name())
			if tt.existingNodes != nil && len(tt.existingNodes.nodes.Items) > 0 {
				nodeStore = tt.existingNodes
				nodeStore.log = logger
			} else {
				nodeStore = NewNodeStore(logger)
			}
			nodeStore.UnsetNode(&tt.node)
			assert.EqualValues(t, tt.wantNodes.nodes, nodeStore.nodes)
		})
	}
}

func generateRandomNodeUUID() string {
	nodeUUID := uuid.New()
	return nodeUUID.String()
}
