package guess

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/smithy-go"
)

func IsThereUnmanagedEKSPodIdentityAgentInstalled(ctx context.Context, client *eks.Client, clusterName string) (bool, error) {
	addon, err := client.DescribeAddon(
		ctx,
		&eks.DescribeAddonInput{
			ClusterName: aws.String(clusterName),
			AddonName:   aws.String("eks-pod-identity-agent"),
		},
	)

	if err != nil || addon.Addon == nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) &&
			apiErr.ErrorCode() == "ResourceNotFoundException" &&
			strings.Contains(apiErr.ErrorMessage(), "No addon: eks-pod-identity-agent found in cluster") {
			return false, nil
		}
		return false, fmt.Errorf("failed to describe addon eks-pod-identity-agent for cluster %s: %w", clusterName, err)
	}

	if managedBy, found := addon.Addon.Tags["managed-by"]; found && managedBy == "dd-karpenter" {
		return false, nil
	}

	return true, nil
}
