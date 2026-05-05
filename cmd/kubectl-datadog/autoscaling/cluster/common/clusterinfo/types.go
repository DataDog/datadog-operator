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
	APIVersion     string                                      `yaml:"apiVersion"`
	ClusterName    string                                      `yaml:"clusterName"`
	GeneratedAt    time.Time                                   `yaml:"generatedAt"`
	NodeManagement map[NodeManager]map[string]NodeManagerEntry `yaml:"nodeManagement"`
	Autoscaling    Autoscaling                                 `yaml:"autoscaling"`
}

// NodeManagerEntry describes the nodes attached to a single management entity
// (a Fargate profile, a Karpenter NodePool, an EKS managed node group, etc.).
// ManagedByDatadog tells the migration tool whether the entity should be
// preserved (true) or drained and removed (false).
type NodeManagerEntry struct {
	Nodes            []string `yaml:"nodes"`
	ManagedByDatadog bool     `yaml:"managedByDatadog,omitempty"`
}

// Autoscaling groups the autoscaling solutions detected on the cluster. The
// migration tool reads this to warn about overlap (e.g. a legacy
// cluster-autoscaler still running alongside Karpenter) or to short-circuit
// when EKS auto-mode already provides Karpenter.
type Autoscaling struct {
	ClusterAutoscaler ClusterAutoscaler `yaml:"clusterAutoscaler"`
	Karpenter         Karpenter         `yaml:"karpenter"`
	EKSAutoMode       EKSAutoMode       `yaml:"eksAutoMode"`
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

// Karpenter captures the running Karpenter controller, if any. Version is the
// app version extracted from the controller image tag. ManagedByDatadog
// reflects the kubectl-datadog sentinel label written by the install command;
// InstallerVersion is the kubectl-datadog version recorded at install time.
type Karpenter struct {
	Present          bool   `yaml:"present"`
	Namespace        string `yaml:"namespace,omitempty"`
	Name             string `yaml:"name,omitempty"`
	Version          string `yaml:"version,omitempty"`
	ManagedByDatadog bool   `yaml:"managedByDatadog,omitempty"`
	InstallerVersion string `yaml:"installerVersion,omitempty"`
}

// EKSAutoMode reports whether EKS auto-mode is active. Wrapped in a struct
// (rather than a bare bool on ClusterInfo) for symmetry with the other
// autoscaling entries and to leave room for future fields.
type EKSAutoMode struct {
	Enabled bool `yaml:"enabled"`
}
