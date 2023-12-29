package kubernetes

import (
	"sync"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// NodeStore stores a list of object metadata for each Node running in the cluster
type NodeStore struct {
	nodesMeta []metav1.ObjectMeta
	log       logr.Logger
	mutex     sync.Mutex
}

// NewNodeStore generates an empty NodeStore instance
func NewNodeStore(log logr.Logger) *NodeStore {
	return &NodeStore{
		nodesMeta: make([]metav1.ObjectMeta, 0),
		log:       log,
	}
}

// SetOrUpdateNode creates a Nodes entry for a new Node if needed
func (n *NodeStore) SetOrUpdateNode(obj client.Object) {
	if node, ok := obj.(*v1.Node); ok {
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

// SetNode creates a Node metadata entry for a new Node
func (n *NodeStore) SetNode(node *v1.Node) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	// add a new node definition
	n.nodesMeta = append(n.nodesMeta, node.ObjectMeta)
}

// UpdateNode updates Node metadata entry for an existing node
func (n *NodeStore) UpdateNode(nodeIdx int, newNode v1.Node) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	// update node metadata definition
	n.nodesMeta[nodeIdx] = newNode.ObjectMeta
}

// UnsetNode removes node metadata entry from NodeStore
func (n *NodeStore) UnsetNode(obj client.Object) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	nodeUID := string(obj.GetUID())
	if _, idx, ok := n.findNode(nodeUID); ok {
		n.nodesMeta = append(n.nodesMeta[:idx], n.nodesMeta[idx+1:]...)
	}
}

// GetNodes retrieves list of nodes metadata from NodeStore
func (n *NodeStore) GetNodes() []metav1.ObjectMeta {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	return n.nodesMeta
}

// findNode queries NodeStore for a node metadata entry by node UID
func (n *NodeStore) findNode(nodeUID string) (metav1.ObjectMeta, int, bool) {
	for i, node := range n.nodesMeta {
		if string(node.UID) == nodeUID {
			return node, i, true
		}
	}
	return metav1.ObjectMeta{}, 0, false
}
