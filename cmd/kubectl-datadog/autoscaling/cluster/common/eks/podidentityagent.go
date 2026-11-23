package eks

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/smithy-go"
)

// DescribeAddonAPI is the subset of the EKS client used by
// IsThereUnmanagedEKSPodIdentityAgentInstalled.
// Defined as an interface to allow mocking in tests.
type DescribeAddonAPI interface {
	DescribeAddon(ctx context.Context, params *eks.DescribeAddonInput, optFns ...func(*eks.Options)) (*eks.DescribeAddonOutput, error)
}

// IsThereUnmanagedEKSPodIdentityAgentInstalled reports whether the eks-pod-identity-agent addon
// is installed on the cluster by someone other than kubectl-datadog.
func IsThereUnmanagedEKSPodIdentityAgentInstalled(ctx context.Context, client DescribeAddonAPI, clusterName string) (bool, error) {
	const podIdentityAgentAddonName = "eks-pod-identity-agent"

	addon, err := client.DescribeAddon(
		ctx,
		&eks.DescribeAddonInput{
			ClusterName: aws.String(clusterName),
			AddonName:   aws.String(podIdentityAgentAddonName),
		},
	)
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) && apiErr.ErrorCode() == "ResourceNotFoundException" {
			return false, nil
		}
		return false, fmt.Errorf("failed to describe addon %s for cluster %s: %w", podIdentityAgentAddonName, clusterName, err)
	}

	if addon.Addon == nil {
		return false, fmt.Errorf("failed to describe addon %s for cluster %s: empty response", podIdentityAgentAddonName, clusterName)
	}

	if managedBy, found := addon.Addon.Tags["managed-by"]; found && managedBy == "dd-karpenter" {
		return false, nil
	}

	return true, nil
}
