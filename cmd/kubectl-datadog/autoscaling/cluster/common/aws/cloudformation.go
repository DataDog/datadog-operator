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
	"github.com/samber/lo"

	"github.com/DataDog/datadog-operator/pkg/version"
)

const (
	maxWaitDuration = 30 * time.Minute
)

// ManagedByTag and ManagedByTagValue identify CloudFormation stacks owned by
// kubectl-datadog. The pair is also propagated from the stack to its
// resources (e.g. AWS::EKS::FargateProfile) by CloudFormation's tag
// inheritance, so downstream code reading those resource tags can rely on
// the same constants.
const (
	ManagedByTag      = "managed-by"
	ManagedByTagValue = "kubectl-datadog"
)

func CreateOrUpdateStack(ctx context.Context, client *cloudformation.Client, stackName string, templateBody string, params map[string]string, extraTags map[string]string) error {
	existing, err := GetStack(ctx, client, stackName)
	if err != nil {
		return err
	}
	return createOrUpdateStack(ctx, client, stackName, templateBody, params, extraTags, existing != nil)
}

// CreateOrUpdateStackWithExisting is like CreateOrUpdateStack but takes a
// pre-fetched stack (nil if it does not exist), saving a DescribeStacks call
// for callers that already looked it up.
func CreateOrUpdateStackWithExisting(ctx context.Context, client *cloudformation.Client, stackName string, templateBody string, params map[string]string, extraTags map[string]string, existing *Stack) error {
	return createOrUpdateStack(ctx, client, stackName, templateBody, params, extraTags, existing != nil)
}

func createOrUpdateStack(ctx context.Context, client *cloudformation.Client, stackName string, templateBody string, params map[string]string, extraTags map[string]string, exists bool) error {
	parameters := lo.MapToSlice(params, func(key, value string) types.Parameter {
		return types.Parameter{ParameterKey: aws.String(key), ParameterValue: aws.String(value)}
	})
	if exists {
		return updateStack(ctx, client, stackName, templateBody, parameters, buildTags(extraTags))
	} else {
		return createStack(ctx, client, stackName, templateBody, parameters, buildTags(extraTags))
	}
}

// buildTags returns the base tags (managed-by, version) plus any extra tags
// passed by the caller. Base tags are passed last to lo.Assign so they
// override any extra entry sharing the same key.
func buildTags(extraTags map[string]string) []types.Tag {
	base := map[string]string{
		ManagedByTag: ManagedByTagValue,
		"version":    version.GetVersion(),
	}
	return lo.MapToSlice(lo.Assign(extraTags, base), func(k, v string) types.Tag {
		return types.Tag{Key: aws.String(k), Value: aws.String(v)}
	})
}

// Stack wraps *types.Stack with map accessors for tags, parameters and
// outputs. All accessors tolerate a nil receiver and return a nil map.
type Stack struct {
	*types.Stack
}

// GetStack returns the named CloudFormation stack, or (nil, nil) if the stack
// does not exist. Callers read tags, parameters and outputs from the returned
// wrapper rather than issuing additional DescribeStacks calls.
func GetStack(ctx context.Context, client *cloudformation.Client, stackName string) (*Stack, error) {
	out, err := client.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: aws.String(stackName),
	})
	if err != nil {
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) &&
			apiErr.ErrorCode() == "ValidationError" &&
			strings.Contains(apiErr.ErrorMessage(), "does not exist") {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to describe stack %s: %w", stackName, err)
	}
	if len(out.Stacks) == 0 {
		return nil, nil
	}
	return &Stack{&out.Stacks[0]}, nil
}

// TagMap returns the stack's tags as a map, or nil if the receiver is nil.
// Method name differs from the embedded `Tags` field to avoid selector
// ambiguity.
func (s *Stack) TagMap() map[string]string {
	if s == nil {
		return nil
	}
	return lo.SliceToMap(s.Tags, func(t types.Tag) (string, string) {
		return aws.ToString(t.Key), aws.ToString(t.Value)
	})
}

// ParameterMap returns the stack's parameters as a map, or nil if the
// receiver is nil.
func (s *Stack) ParameterMap() map[string]string {
	if s == nil {
		return nil
	}
	return lo.SliceToMap(s.Parameters, func(p types.Parameter) (string, string) {
		return aws.ToString(p.ParameterKey), aws.ToString(p.ParameterValue)
	})
}

// OutputMap returns the stack's outputs as a map, or nil if the receiver is
// nil.
func (s *Stack) OutputMap() map[string]string {
	if s == nil {
		return nil
	}
	return lo.SliceToMap(s.Outputs, func(o types.Output) (string, string) {
		return aws.ToString(o.OutputKey), aws.ToString(o.OutputValue)
	})
}

// DoesStackExist checks if a CloudFormation stack exists.
func DoesStackExist(ctx context.Context, client *cloudformation.Client, stackName string) (bool, error) {
	stack, err := GetStack(ctx, client, stackName)
	return stack != nil, err
}

func createStack(ctx context.Context, client *cloudformation.Client, stackName string, templateBody string, parameters []types.Parameter, tags []types.Tag) error {
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
			Tags: tags,
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

func updateStack(ctx context.Context, client *cloudformation.Client, stackName string, templateBody string, parameters []types.Parameter, tags []types.Tag) error {
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
			Tags: tags,
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

func DeleteStack(ctx context.Context, client *cloudformation.Client, stackName string) error {
	exist, err := DoesStackExist(ctx, client, stackName)
	if err != nil {
		return err
	}

	if !exist {
		log.Printf("Stack %s does not exist, skipping deletion.", stackName)
		return nil
	}

	log.Printf("Deleting stack %s…", stackName)

	_, err = client.DeleteStack(
		ctx,
		&cloudformation.DeleteStackInput{
			StackName: aws.String(stackName),
		},
	)
	if err != nil {
		return fmt.Errorf("failed to delete stack %s: %w", stackName, err)
	}

	waiter := cloudformation.NewStackDeleteCompleteWaiter(client)
	if err := waiter.Wait(
		ctx,
		&cloudformation.DescribeStacksInput{
			StackName: aws.String(stackName),
		},
		maxWaitDuration,
	); err != nil {
		log.Printf("Failed to delete stack %s.", stackName)
		describeStack(ctx, client, stackName)

		return fmt.Errorf("failed to wait for stack %s deletion: %w", stackName, err)
	}

	log.Printf("Deleted stack %s.", stackName)

	return nil
}

func describeStack(ctx context.Context, client *cloudformation.Client, stackName string) error {
	stack, err := GetStack(ctx, client, stackName)
	if err != nil {
		return err
	}
	if stack == nil {
		return errors.New("no stack found")
	}

	log.Printf("Stack status: %s", stack.StackStatus)
	if stack.StackStatusReason != nil {
		log.Printf("Status reason: %s", *stack.StackStatusReason)
	}

	log.Print("Stack events:")
	events, err := client.DescribeStackEvents(ctx, &cloudformation.DescribeStackEventsInput{
		StackName: aws.String(stackName),
	})
	if err != nil {
		return err
	}
	for _, event := range events.StackEvents {
		log.Printf("  %s: %s — %s", event.Timestamp.Format(time.RFC3339), event.ResourceStatus, aws.ToString(event.ResourceStatusReason))
	}

	return nil
}
