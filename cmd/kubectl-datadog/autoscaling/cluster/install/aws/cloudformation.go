// Package aws provides AWS-specific functionality for managing CloudFormation stacks,
// IAM configurations, and other AWS resources required for Karpenter installation.
package aws

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/smithy-go"
)

const (
	maxWaitDuration = 30 * time.Minute
)

func CreateOrUpdateStack(ctx context.Context, client *cloudformation.Client, stackName string, templateBody string, params map[string]string) error {
	exist, err := doesStackExist(ctx, client, stackName)
	if err != nil {
		return err
	}

	var parameters []types.Parameter
	for key, value := range params {
		parameters = append(parameters, types.Parameter{
			ParameterKey:   aws.String(key),
			ParameterValue: aws.String(value),
		})
	}

	if exist {
		return updateStack(ctx, client, stackName, templateBody, parameters)
	} else {
		return createStack(ctx, client, stackName, templateBody, parameters)
	}
}

func doesStackExist(ctx context.Context, client *cloudformation.Client, stackName string) (bool, error) {
	_, err := client.DescribeStacks(
		ctx,
		&cloudformation.DescribeStacksInput{
			StackName: aws.String(stackName),
		},
	)

	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) &&
			apiErr.ErrorCode() == "ValidationError" &&
			strings.Contains(apiErr.ErrorMessage(), "does not exist") {
			return false, nil
		}
		return false, fmt.Errorf("failed to describe stack %s: %w", stackName, err)
	}

	return true, nil
}

func createStack(ctx context.Context, client *cloudformation.Client, stackName string, templateBody string, parameters []types.Parameter) error {
	log.Printf("Creating stack %s…", stackName)

	out, err := client.CreateStack(
		ctx,
		&cloudformation.CreateStackInput{
			StackName:    aws.String(stackName),
			TemplateBody: aws.String(templateBody),
			Parameters:   parameters,
			Capabilities: []types.Capability{
				types.CapabilityCapabilityNamedIam,
			},
		},
	)
	if err != nil {
		return fmt.Errorf("failed to create stack %s: %w", stackName, err)
	}

	waiter := cloudformation.NewStackCreateCompleteWaiter(client)
	if err := waiter.Wait(
		ctx,
		&cloudformation.DescribeStacksInput{
			StackName: aws.String(stackName),
		},
		maxWaitDuration,
	); err != nil {
		log.Printf("Failed to create stack %s.", stackName)
		describeStack(ctx, client, stackName)

		return fmt.Errorf("failed to wait for stack %s creation: %w", stackName, err)
	}

	log.Printf("Created stack %s with id %s.", stackName, aws.ToString(out.StackId))

	return nil
}

func updateStack(ctx context.Context, client *cloudformation.Client, stackName string, templateBody string, parameters []types.Parameter) error {
	log.Printf("Updating stack %s…", stackName)

	out, err := client.UpdateStack(
		ctx,
		&cloudformation.UpdateStackInput{
			StackName:    aws.String(stackName),
			TemplateBody: aws.String(templateBody),
			Parameters:   parameters,
			Capabilities: []types.Capability{
				types.CapabilityCapabilityNamedIam,
			},
		},
	)
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) &&
			apiErr.ErrorCode() == "ValidationError" &&
			strings.Contains(apiErr.ErrorMessage(), "No updates are to be performed") {
			log.Printf("Stack %s is already up-to-date.", stackName)
			return nil
		}
		return fmt.Errorf("failed to update stack %s: %w", stackName, err)
	}

	waiter := cloudformation.NewStackUpdateCompleteWaiter(client)
	if err := waiter.Wait(
		ctx,
		&cloudformation.DescribeStacksInput{
			StackName: aws.String(stackName),
		},
		maxWaitDuration,
	); err != nil {
		log.Printf("Failed to update stack %s.", stackName)
		describeStack(ctx, client, stackName)

		return fmt.Errorf("failed to wait for stack %s update: %w", stackName, err)
	}

	log.Printf("Updated stack %s with id %s.", stackName, aws.ToString(out.StackId))

	return nil
}

func describeStack(ctx context.Context, client *cloudformation.Client, stackName string) error {
	out, err := client.DescribeStacks(
		ctx,
		&cloudformation.DescribeStacksInput{
			StackName: aws.String(stackName),
		},
	)
	if err != nil {
		return err
	}
	if len(out.Stacks) == 0 {
		return errors.New("no stack found")
	}

	stack := out.Stacks[0]

	log.Printf("Stack status: %s", stack.StackStatus)
	if stack.StackStatusReason != nil {
		log.Printf("Status reason: %s", *stack.StackStatusReason)
	}

	log.Print("Stack events:")
	out2, err := client.DescribeStackEvents(ctx, &cloudformation.DescribeStackEventsInput{
		StackName: aws.String(stackName),
	})
	if err != nil {
		return err
	}
	for _, event := range out2.StackEvents {
		log.Printf("  %s: %s — %s", event.Timestamp.Format(time.RFC3339), event.ResourceStatus, aws.ToString(event.ResourceStatusReason))
	}

	return nil
}
