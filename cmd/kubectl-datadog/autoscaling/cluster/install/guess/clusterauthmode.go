package guess

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	ekstypes "github.com/aws/aws-sdk-go-v2/service/eks/types"
)

// SupportsAPIAuthenticationMode checks if the EKS cluster's authentication mode
// supports EKS Access Entries (API or API_AND_CONFIG_MAP).
// EKS Access Entries are only supported when the cluster authentication mode
// includes API authentication.
func SupportsAPIAuthenticationMode(ctx context.Context, client *eks.Client, clusterName string) (bool, error) {
	cluster, err := client.DescribeCluster(ctx, &eks.DescribeClusterInput{
		Name: aws.String(clusterName),
	})
	if err != nil {
		return false, fmt.Errorf("failed to describe cluster %s: %w", clusterName, err)
	}

	if cluster.Cluster == nil || cluster.Cluster.AccessConfig == nil {
		// If no access config is available, assume it's using the default CONFIG_MAP mode
		return false, nil
	}

	authMode := cluster.Cluster.AccessConfig.AuthenticationMode
	// EKS Access Entries are supported when authentication mode is API or API_AND_CONFIG_MAP
	return authMode == ekstypes.AuthenticationModeApi ||
		authMode == ekstypes.AuthenticationModeApiAndConfigMap, nil
}
