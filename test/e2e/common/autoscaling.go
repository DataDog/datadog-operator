// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	cftypes "github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/smithy-go"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const (
	// KarpenterNamespace is the default namespace for Karpenter
	KarpenterNamespace = "dd-karpenter"
)

// KubectlDatadogPath returns the path to the kubectl-datadog binary
func KubectlDatadogPath() string {
	return filepath.Join(ProjectRootPath, "bin", "kubectl-datadog")
}

// RunKubectlDatadog executes kubectl-datadog command with the specified kubeconfig, AWS profile and region
func RunKubectlDatadog(ctx context.Context, kubeconfigPath, awsProfile, awsRegion string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, KubectlDatadogPath(), args...)
	cmd.Env = append(cmd.Environ(), "KUBECONFIG="+kubeconfigPath)
	if awsProfile != "" {
		cmd.Env = append(cmd.Env, "AWS_PROFILE="+awsProfile)
	}
	if awsRegion != "" {
		cmd.Env = append(cmd.Env, "AWS_REGION="+awsRegion)
	}
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// RunAutoscalingInstall runs kubectl datadog autoscaling cluster install
func RunAutoscalingInstall(ctx context.Context, kubeconfigPath, awsProfile, awsRegion, clusterName string, extraArgs ...string) (string, error) {
	args := []string{"autoscaling", "cluster", "install", "--cluster-name", clusterName}
	args = append(args, extraArgs...)
	return RunKubectlDatadog(ctx, kubeconfigPath, awsProfile, awsRegion, args...)
}

// RunAutoscalingUninstall runs kubectl datadog autoscaling cluster uninstall
func RunAutoscalingUninstall(ctx context.Context, kubeconfigPath, awsProfile, awsRegion, clusterName string) (string, error) {
	return RunKubectlDatadog(ctx, kubeconfigPath, awsProfile, awsRegion, "autoscaling", "cluster", "uninstall", "--cluster-name", clusterName, "--yes")
}

// CloudFormationClient creates a CloudFormation client using the default AWS config
func CloudFormationClient(ctx context.Context, region string) (*cloudformation.Client, error) {
	var opts []func(*config.LoadOptions) error
	if region != "" {
		opts = append(opts, config.WithRegion(region))
	}
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}
	return cloudformation.NewFromConfig(cfg), nil
}

// VerifyCloudFormationStackExists checks if a CloudFormation stack exists
func VerifyCloudFormationStackExists(ctx context.Context, cfnClient *cloudformation.Client, stackName string) (bool, error) {
	_, err := cfnClient.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
		StackName: aws.String(stackName),
	})
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

// VerifyCloudFormationStacks checks that the expected CloudFormation stacks exist for Karpenter
func VerifyCloudFormationStacks(ctx context.Context, cfnClient *cloudformation.Client, clusterName string) error {
	stackNames := []string{
		"dd-karpenter-" + clusterName + "-karpenter",
		"dd-karpenter-" + clusterName + "-dd-karpenter",
	}

	for _, stackName := range stackNames {
		exists, err := VerifyCloudFormationStackExists(ctx, cfnClient, stackName)
		if err != nil {
			return fmt.Errorf("error checking stack %s: %w", stackName, err)
		}
		if !exists {
			return fmt.Errorf("CloudFormation stack %s not found", stackName)
		}
	}
	return nil
}

// VerifyCloudFormationStacksDeleted checks that the CloudFormation stacks have been deleted
func VerifyCloudFormationStacksDeleted(ctx context.Context, cfnClient *cloudformation.Client, clusterName string) error {
	stackNames := []string{
		"dd-karpenter-" + clusterName + "-karpenter",
		"dd-karpenter-" + clusterName + "-dd-karpenter",
	}

	for _, stackName := range stackNames {
		exists, err := VerifyCloudFormationStackExists(ctx, cfnClient, stackName)
		if err != nil {
			return fmt.Errorf("error checking stack %s: %w", stackName, err)
		}
		if exists {
			// Check if the stack is in DELETE_IN_PROGRESS or similar
			output, err := cfnClient.DescribeStacks(ctx, &cloudformation.DescribeStacksInput{
				StackName: aws.String(stackName),
			})
			if err == nil && len(output.Stacks) > 0 {
				status := output.Stacks[0].StackStatus
				if status != cftypes.StackStatusDeleteInProgress &&
					status != cftypes.StackStatusDeleteComplete {
					return fmt.Errorf("CloudFormation stack %s still exists with status %s", stackName, status)
				}
			}
		}
	}
	return nil
}

// VerifyKarpenterPods checks that Karpenter controller pods are running
func VerifyKarpenterPods(ctx context.Context, k8sClient kubernetes.Interface, namespace string) error {
	pods, err := k8sClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=karpenter",
	})
	if err != nil {
		return fmt.Errorf("failed to list Karpenter pods: %w", err)
	}
	if len(pods.Items) == 0 {
		return fmt.Errorf("no Karpenter pods found in namespace %s", namespace)
	}

	// Check that at least one pod is running
	runningCount := 0
	for _, pod := range pods.Items {
		if pod.Status.Phase == "Running" {
			runningCount++
		}
	}
	if runningCount == 0 {
		return fmt.Errorf("no Karpenter pods are running in namespace %s", namespace)
	}
	return nil
}

// VerifyNoKarpenterPods checks that no Karpenter pods exist
func VerifyNoKarpenterPods(ctx context.Context, k8sClient kubernetes.Interface, namespace string) error {
	pods, err := k8sClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=karpenter",
	})
	if err != nil {
		// If namespace doesn't exist, that's fine - no pods exist
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to list pods: %w", err)
	}
	if len(pods.Items) > 0 {
		return fmt.Errorf("found %d Karpenter pods in namespace %s, expected none", len(pods.Items), namespace)
	}
	return nil
}

// VerifyHelmReleaseExists checks if a Helm release exists by looking for its secret
func VerifyHelmReleaseExists(ctx context.Context, k8sClient kubernetes.Interface, namespace, releaseName string) (bool, error) {
	secrets, err := k8sClient.CoreV1().Secrets(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("name=%s,owner=helm", releaseName),
	})
	if err != nil {
		return false, fmt.Errorf("failed to list Helm secrets: %w", err)
	}
	return len(secrets.Items) > 0, nil
}

// WaitForKarpenterPods waits for Karpenter pods to be running with a timeout
func WaitForKarpenterPods(ctx context.Context, k8sClient kubernetes.Interface, namespace string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for Karpenter pods to be running")
			}
			err := VerifyKarpenterPods(ctx, k8sClient, namespace)
			if err == nil {
				return nil
			}
		}
	}
}
