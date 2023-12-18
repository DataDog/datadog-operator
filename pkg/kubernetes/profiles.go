package kubernetes

import (
	"sync"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type NodeStore struct {
	nodes map[string]v1.Node
	log   logr.Logger
	mutex sync.Mutex
}

// NewNodeStore generates an empty NodeStore instance
func NewNodeStore(log logr.Logger) *NodeStore {
	return &NodeStore{
		nodes: map[string]v1.Node{},
		log:   log,
	}
}

// SetOrUpdateNode creates a Nodes entry for a new Node if needed
func (n *NodeStore) SetOrUpdateNode(obj client.Object) {
	nodeLabels := obj.GetLabels()
	nodeUID := string(obj.GetUID())
	nodeName := obj.GetName()

	// update node if present in node store
	if _, ok := n.nodes[nodeUID]; ok {
		n.log.Info("New node labels detected. Updating node store", "node", nodeUID, "node name", nodeName)
		n.SetNode(nodeUID, nodeName, nodeLabels)
		return
	}

	// add a new node definition if not present in node store
	if _, ok := n.nodes[nodeUID]; !ok {
		n.log.Info("New node detected", "node", nodeUID, "node name", nodeName)
		n.SetNode(nodeUID, nodeName, nodeLabels)
	}

}

// SetNode creates a Nodes entry for a new Node
func (n *NodeStore) SetNode(nodeUID string, nodeName string, nodeLabels map[string]string) {
	n.mutex.Lock()
	defer n.mutex.Unlock()
	// add a new node definition
	n.nodes[nodeUID] = v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:   nodeName,
			Labels: nodeLabels,
			UID:    types.UID(nodeUID),
		},
	}
}

func (n *NodeStore) UnsetNode(obj client.Object) {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	nodeUID := string(obj.GetUID())
	delete(n.nodes, nodeUID)

}

func (n *NodeStore) GetNodes() *map[string]v1.Node {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	return &n.nodes
}
