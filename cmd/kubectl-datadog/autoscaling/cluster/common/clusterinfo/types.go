// Package clusterinfo classifies cluster nodes by their management method
// (Fargate, Karpenter, EKS managed node group, ASG, standalone, unknown) and
// persists the classification in a ConfigMap. The snapshot drives the
// follow-up migration of workloads from the existing nodes to Karpenter.
package clusterinfo

import "time"

// APIVersion is the schema version of the ConfigMap payload. Bump on
// backward-incompatible shape changes so future readers can branch.
const APIVersion = "v1"

// ConfigMapName is the name of the ConfigMap that stores the snapshot.
const ConfigMapName = "dd-cluster-info"

// ConfigMapDataKey is the key under .data containing the YAML payload.
const ConfigMapDataKey = "cluster-info"

// NodeManager identifies the management method for a Kubernetes node.
type NodeManager string

const (
	NodeManagerFargate             NodeManager = "fargate"
	NodeManagerKarpenter           NodeManager = "karpenter"
	NodeManagerEKSManagedNodeGroup NodeManager = "eksManagedNodeGroup"
	NodeManagerASG                 NodeManager = "asg"
	NodeManagerStandalone          NodeManager = "standalone"
	NodeManagerUnknown             NodeManager = "unknown"
)

// ClusterInfo is the snapshot persisted in the ConfigMap.
type ClusterInfo struct {
	APIVersion        string                              `yaml:"apiVersion"`
	ClusterName       string                              `yaml:"clusterName"`
	GeneratedAt       time.Time                           `yaml:"generatedAt"`
	NodeManagement    map[NodeManager]map[string][]string `yaml:"nodeManagement"`
	ClusterAutoscaler ClusterAutoscaler                   `yaml:"clusterAutoscaler"`
}

// ClusterAutoscaler captures whether a legacy cluster-autoscaler Deployment
// is running and where, so the migration can warn the user to stop it before
// scaling EKS managed node groups (per the Karpenter migration guide).
type ClusterAutoscaler struct {
	Present   bool   `yaml:"present"`
	Namespace string `yaml:"namespace,omitempty"`
	Name      string `yaml:"name,omitempty"`
	Version   string `yaml:"version,omitempty"`
}
