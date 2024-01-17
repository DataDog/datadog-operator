// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	"sync"

	v1 "k8s.io/api/core/v1"
)

// NodeStore stores a list of object metadata for each Node running in the cluster
type NodeStore struct {
	nodes map[string]map[string]string
	mutex sync.Mutex
}

// NewNodeStore generates an empty NodeStore instance
func NewNodeStore() *NodeStore {
	return &NodeStore{
		nodes: map[string]map[string]string{},
	}
}

// SetNode stores a mapping of a node name to its labels in the NodeStore
func (n *NodeStore) SetNode(node *v1.Node) {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	n.nodes[node.Name] = node.Labels
}

// UnsetNode removes an entry from the NodeStore by node name
func (n *NodeStore) UnsetNode(nodeName string) {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	delete(n.nodes, nodeName)
}

// GetNodes retrieves list of nodes metadata from NodeStore
func (n *NodeStore) GetNodes() map[string]map[string]string {
	n.mutex.Lock()
	defer n.mutex.Unlock()

	return n.nodes
}
