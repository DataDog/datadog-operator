package guess

import (
	"context"
	"strings"

	"k8s.io/client-go/tools/clientcmd/api"
)

// GetClusterNameFromKubeconfig attempts to extract the EKS cluster name from the current kubectl context
func GetClusterNameFromKubeconfig(ctx context.Context, rawConfig api.Config, kubeContext string) (clusterName string) {
	if kubeContext == "" {
		kubeContext = rawConfig.CurrentContext
	}
	if kubeContext == "" {
		return ""
	}

	context, exists := rawConfig.Contexts[kubeContext]
	if !exists {
		return ""
	}

	clusterName = context.Cluster

	// For EKS, the cluster name in kubeconfig is often an ARN
	// Format: arn:aws:eks:region:account:cluster/cluster-name
	if strings.HasPrefix(clusterName, "arn:aws:eks:") {
		var found bool
		if _, clusterName, found = strings.Cut(clusterName, "/"); found {
			return clusterName
		}
	}

	// For eksctl, the cluster name in kubeconfig is an eksctl.io suffixed FQDN
	// Format: cluster-name.region.eksctl.io
	if strings.HasSuffix(clusterName, ".eksctl.io") {
		var found bool
		if clusterName, _, found = strings.Cut(clusterName, "."); found {
			return clusterName
		}
	}

	// If it's not an ARN, check if it looks like a regular cluster name
	// (doesn't contain colons which would indicate it's some other format)
	if clusterName != "" && !strings.Contains(clusterName, ":") {
		return clusterName
	}

	return ""
}
