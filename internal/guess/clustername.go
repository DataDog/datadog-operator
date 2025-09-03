package guess

import (
	"context"
	"strings"

	"k8s.io/client-go/tools/clientcmd"
)

// GetClusterNameFromKubeconfig attempts to extract the EKS cluster name from the current kubectl context
func GetClusterNameFromKubeconfig(ctx context.Context) string {
	config := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		clientcmd.NewDefaultClientConfigLoadingRules(),
		&clientcmd.ConfigOverrides{},
	)

	rawConfig, err := config.RawConfig()
	if err != nil {
		return ""
	}

	currentContext := rawConfig.CurrentContext
	if currentContext == "" {
		return ""
	}

	context, exists := rawConfig.Contexts[currentContext]
	if !exists {
		return ""
	}

	clusterName := context.Cluster

	// For EKS, the cluster name in kubeconfig is often an ARN
	// Format: arn:aws:eks:region:account:cluster/cluster-name
	if strings.HasPrefix(clusterName, "arn:aws:eks:") {
		if _, clusterName, found := strings.Cut(clusterName, "/"); found {
			return clusterName
		}
	}

	// For eksctl, the cluster name in kubeconfig is an eksctl.io suffixed FQDN
	// Format: cluster-name.region.eksctl.io
	if strings.HasSuffix(clusterName, ".eksctl.io") {
		if clusterName, _, found := strings.Cut(clusterName, "."); found {
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
