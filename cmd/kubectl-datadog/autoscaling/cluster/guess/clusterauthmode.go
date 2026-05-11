package guess

import (
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
)

// SupportsAPIAuthenticationMode reports whether the EKS cluster's
// authentication mode supports EKS Access Entries (API or API_AND_CONFIG_MAP).
func SupportsAPIAuthenticationMode(cluster *ekstypes.Cluster) bool {
	if cluster == nil || cluster.AccessConfig == nil {
		return false
	}
	authMode := cluster.AccessConfig.AuthenticationMode
	return authMode == ekstypes.AuthenticationModeApi ||
		authMode == ekstypes.AuthenticationModeApiAndConfigMap
}
