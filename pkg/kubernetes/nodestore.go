package kubernetes

import (
	"sync"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
)

// NodeStore stores a list of object metadata for each Node running in the cluster
type NodeStore struct {
	nodeLabels map[string]map[string]string
	log        logr.Logger
	mutex      sync.Mutex
}

// NewNodeStore generates an empty NodeStore instance
func NewNodeStore(log logr.Logger) *NodeStore {
	return &NodeStore{
		nodeLabels: map[string]map[string]string{},
		log:        log,
	}
}

// SetOrUpdateNode creates a Nodes entry for a new Node if needed
func (n *NodeStore) SetOrUpdateNode(node *v1.Node) {
	// update node if present in node store
	nodeUID := string(node.UID)

	if _, ok := n.findLabelsByNode(nodeUID); ok {
		n.updateNode(*node)
		return
	}

	// add a new node definition if not present in node store
	if _, ok := n.findLabelsByNode(nodeUID); !ok {
		n.log.Info("New node detected", "node", nodeUID)
		n.setNode(node)
	}

}

// SetNode creates a Node metadata entry for a new Node
func (n *NodeStore) setNode(node *v1.Node) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	// add a new node definition
	n.nodeLabels[string(node.UID)] = node.Labels
}

// UpdateNode updates Node metadata entry for an existing node
func (n *NodeStore) updateNode(newNode v1.Node) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	// update node metadata definition
	n.nodeLabels[string(newNode.UID)] = newNode.Labels
}

// UnsetNode removes nodeLabels entry from NodeStore
func (n *NodeStore) UnsetNode(nodeUID string) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	if _, ok := n.findLabelsByNode(nodeUID); ok {
		delete(n.nodeLabels, nodeUID)
	}
}

// do we really need this?
// GetNodes retrieves list of nodes metadata from NodeStore
func (n *NodeStore) GetNodes() map[string]map[string]string {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	return n.nodeLabels
}

// findLabelsByNode queries NodeStore for a node labels entry by node UID
func (n *NodeStore) findLabelsByNode(nodeUID string) (map[string]string, bool) {
	if labels, ok := n.nodeLabels[nodeUID]; ok {
		return labels, true
	}
	return map[string]string{}, false
}
