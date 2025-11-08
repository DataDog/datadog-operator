package aws

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/smithy-go"
)

func CreateOrUpdateEKSAddon(ctx context.Context, client *eks.Client, clusterName string, addonName string) error {
	exist, err := doesAddonExist(ctx, client, clusterName, addonName)
	if err != nil {
		return err
	}

	if exist {
		return updateAddon(ctx, client, clusterName, addonName)
	} else {
		return createAddon(ctx, client, clusterName, addonName)
	}
}

func doesAddonExist(ctx context.Context, client *eks.Client, clusterName string, addonName string) (bool, error) {
	_, err := client.DescribeAddon(
		ctx,
		&eks.DescribeAddonInput{
			ClusterName: aws.String(clusterName),
			AddonName:   aws.String(addonName),
		},
	)

	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) &&
			apiErr.ErrorCode() == "ResourceNotFoundException" &&
			strings.Contains(apiErr.ErrorMessage(), "not found") {
			return false, nil
		}
		return false, fmt.Errorf("failed to describe addon %s for cluster %s: %w", addonName, clusterName, err)
	}

	return true, nil
}

func createAddon(ctx context.Context, client *eks.Client, clusterName string, addonName string) error {
	log.Printf("Creating EKS Addon %s for cluster %s", addonName, clusterName)

	_, err := client.CreateAddon(
		ctx,
		&eks.CreateAddonInput{
			ClusterName: aws.String(clusterName),
			AddonName:   aws.String(addonName),
		},
	)

	log.Printf("EKS Addon %s for cluster %s created", addonName, clusterName)

	return err
}

func updateAddon(ctx context.Context, client *eks.Client, clusterName string, addonName string) error {
	log.Printf("Updating EKS Addon %s for cluster %s", addonName, clusterName)

	_, err := client.UpdateAddon(
		ctx,
		&eks.UpdateAddonInput{
			ClusterName: aws.String(clusterName),
			AddonName:   aws.String(addonName),
		},
	)

	log.Printf("EKS Addon %s for cluster %s updated", addonName, clusterName)

	return err
}
