package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"k8s.io/client-go/tools/clientcmd"
)

//go:embed cloudformation.yaml
var cloudformationTemplate string

// getClusterNameFromKubeconfig attempts to extract the EKS cluster name from the current kubectl context
func getClusterNameFromKubeconfig() string {
	// NewDefaultClientConfigLoadingRules already handles KUBECONFIG env var and ~/.kube/config fallback
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	config := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules,
		&clientcmd.ConfigOverrides{},
	)
	// Get the raw config to access context and cluster information
	rawConfig, err := config.RawConfig()
	if err != nil {
		return ""
	}
	// Get current context name
	currentContext := rawConfig.CurrentContext
	if currentContext == "" {
		return ""
	}
	// Get the context
	context, exists := rawConfig.Contexts[currentContext]
	if !exists {
		return ""
	}
	// Get the cluster name from context
	clusterName := context.Cluster
	// For EKS, the cluster name in kubeconfig is often an ARN
	// Format: arn:aws:eks:region:account:cluster/cluster-name
	if strings.Contains(clusterName, "arn:aws:eks:") {
		parts := strings.Split(clusterName, "/")
		if len(parts) >= 2 {
			return parts[len(parts)-1]
		}
	}
	// If it's not an ARN, check if it looks like a regular cluster name
	// (doesn't contain colons which would indicate it's some other format)
	if clusterName != "" && !strings.Contains(clusterName, ":") {
		return clusterName
	}
	// Also check the actual cluster entry for server URL patterns
	if cluster, exists := rawConfig.Clusters[clusterName]; exists {
		// EKS API server URLs have format: https://[ID].gr7.region.eks.amazonaws.com
		// We can't extract cluster name from this, but we tried
		_ = cluster
	}
	return ""
}

func main() {
	// Try to detect cluster name early for help message
	detectedCluster := getClusterNameFromKubeconfig()
	var (
		stackName   = flag.String("stack-name", "karpenter-stack", "Name of the CloudFormation stack")
		clusterName = flag.String("cluster-name", "", fmt.Sprintf("EKS cluster name (default: %s)", func() string {
			if detectedCluster != "" {
				return detectedCluster
			}
			return "auto-detect from kubeconfig"
		}()))
		timeout     = flag.Duration("timeout", 30*time.Minute, "Timeout for stack creation")
	)
	flag.Parse()

	// If cluster name not provided, use the detected one
	if *clusterName == "" {
		if detectedCluster != "" {
			*clusterName = detectedCluster
			fmt.Printf("Using auto-detected cluster name from kubeconfig: %s\n", detectedCluster)
		} else {
			log.Fatal("Error: --cluster-name is required or kubectl must be configured with an EKS cluster context")
		}
	}

	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}

	client := cloudformation.NewFromConfig(cfg)

	fmt.Printf("Creating CloudFormation stack '%s' for cluster '%s'...\n", *stackName, *clusterName)

	createInput := &cloudformation.CreateStackInput{
		StackName:    aws.String(*stackName),
		TemplateBody: aws.String(cloudformationTemplate),
		Parameters: []types.Parameter{
			{
				ParameterKey:   aws.String("ClusterName"),
				ParameterValue: aws.String(*clusterName),
			},
		},
		Capabilities: []types.Capability{
			types.CapabilityCapabilityIam,
			types.CapabilityCapabilityNamedIam,
		},
		OnFailure: types.OnFailureRollback,
		Tags: []types.Tag{
			{
				Key:   aws.String("ManagedBy"),
				Value: aws.String("karpenter-installer"),
			},
			{
				Key:   aws.String("ClusterName"),
				Value: aws.String(*clusterName),
			},
		},
	}

	createOutput, err := client.CreateStack(context.TODO(), createInput)
	if err != nil {
		log.Fatalf("Failed to create stack: %v", err)
	}

	fmt.Printf("Stack creation initiated. Stack ID: %s\n", *createOutput.StackId)
	fmt.Println("Waiting for stack creation to complete...")

	waiter := cloudformation.NewStackCreateCompleteWaiter(client)
	maxWaitTime := *timeout

	describeInput := &cloudformation.DescribeStacksInput{
		StackName: aws.String(*stackName),
	}

	err = waiter.Wait(context.TODO(), describeInput, maxWaitTime)
	if err != nil {
		fmt.Printf("Error waiting for stack creation: %v\n", err)
		describeOutput, descErr := client.DescribeStacks(context.TODO(), describeInput)
		if descErr == nil && len(describeOutput.Stacks) > 0 {
			stack := describeOutput.Stacks[0]
			fmt.Printf("Stack Status: %s\n", stack.StackStatus)
			if stack.StackStatusReason != nil {
				fmt.Printf("Status Reason: %s\n", *stack.StackStatusReason)
			}
			fmt.Println("\nStack Events:")
			eventsInput := &cloudformation.DescribeStackEventsInput{
				StackName: aws.String(*stackName),
			}
			eventsOutput, eventsErr := client.DescribeStackEvents(context.TODO(), eventsInput)
			if eventsErr == nil {
				for i := 0; i < len(eventsOutput.StackEvents) && i < 10; i++ {
					event := eventsOutput.StackEvents[i]
					fmt.Printf("  %s: %s - %s\n",
						event.Timestamp.Format(time.RFC3339),
						event.ResourceStatus,
						aws.ToString(event.ResourceStatusReason))
				}
			}
		}
		os.Exit(1)
	}

	fmt.Println("Stack created successfully!")

	describeOutput, err := client.DescribeStacks(context.TODO(), describeInput)
	if err != nil {
		log.Fatalf("Failed to describe stack: %v", err)
	}

	if len(describeOutput.Stacks) > 0 {
		stack := describeOutput.Stacks[0]
		fmt.Printf("\nStack Details:\n")
		fmt.Printf("  Name: %s\n", aws.ToString(stack.StackName))
		fmt.Printf("  Status: %s\n", stack.StackStatus)
		fmt.Printf("  Created: %s\n", stack.CreationTime.Format(time.RFC3339))

		if len(stack.Outputs) > 0 {
			fmt.Println("\nStack Outputs:")
			for _, output := range stack.Outputs {
				fmt.Printf("  %s: %s\n",
					aws.ToString(output.OutputKey),
					aws.ToString(output.OutputValue))
			}
		}

		fmt.Println("\nCreated Resources:")
		resourcesInput := &cloudformation.ListStackResourcesInput{
			StackName: aws.String(*stackName),
		}
		resourcesOutput, err := client.ListStackResources(context.TODO(), resourcesInput)
		if err == nil {
			for _, resource := range resourcesOutput.StackResourceSummaries {
				fmt.Printf("  %s (%s): %s\n",
					aws.ToString(resource.LogicalResourceId),
					aws.ToString(resource.ResourceType),
					resource.ResourceStatus)
			}
		}
	}
}
