package kubernetes

import (
	"sync"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type NodeStore struct {
	nodes v1.NodeList
	log   logr.Logger
	mutex sync.Mutex
}

// NewNodeStore generates an empty NodeStore instance
func NewNodeStore(log logr.Logger) *NodeStore {
	return &NodeStore{
		nodes: v1.NodeList{},
		log:   log,
	}
}

// SetOrUpdateNode creates a Nodes entry for a new Node if needed
func (n *NodeStore) SetOrUpdateNode(obj client.Object) {
	if node, ok := obj.(*v1.Node); ok {
		if node.DeepCopyObject() != nil {
			// update node if present in node store
			nodeUID := string(node.GetUID())

			if _, nodeIdx, ok := n.findNode(nodeUID); ok {
				n.log.Info("New node labels detected. Updating node store", "node", nodeUID)
				n.UpdateNode(nodeIdx, *node)
				return
			}

			// add a new node definition if not present in node store
			if _, _, ok := n.findNode(nodeUID); !ok {
				n.log.Info("New node detected", "node", nodeUID)
				n.SetNode(node)
			}
		}
	}
}

// SetNode creates a Nodes entry for a new Node
func (n *NodeStore) SetNode(node *v1.Node) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	// add a new node definition
	n.nodes.Items = append(n.nodes.Items, *node)
}

func (n *NodeStore) UpdateNode(nodeIdx int, newNode v1.Node) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	// update node definition
	n.nodes.Items[nodeIdx] = newNode
}

func (n *NodeStore) UnsetNode(obj client.Object) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	nodeUID := string(obj.GetUID())
	if _, idx, ok := n.findNode(nodeUID); ok {
		n.nodes.Items = append(n.nodes.Items[:idx], n.nodes.Items[idx+1:]...)
	}
}

func (n *NodeStore) GetNodes() v1.NodeList {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	return n.nodes
}

func (n *NodeStore) findNode(nodeUID string) (v1.Node, int, bool) {
	for i, node := range n.nodes.Items {
		if string(node.UID) == nodeUID {
			return node, i, true
		}
	}
	return v1.Node{}, 0, false
}
