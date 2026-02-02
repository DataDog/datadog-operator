// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	// KarpenterNamespace is the default namespace for Karpenter
	KarpenterNamespace = "dd-karpenter"
)

// AWSCredentials holds AWS credentials loaded from the SDK's default credential chain
type AWSCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string // may be empty for static credentials
	Region          string
}

// KubectlDatadogPath returns the path to the kubectl-datadog binary
func KubectlDatadogPath() string {
	return filepath.Join(ProjectRootPath, "bin", "kubectl-datadog")
}

// RunKubectlDatadog executes kubectl-datadog with explicit AWS credentials
func RunKubectlDatadog(ctx context.Context, kubeconfigPath string, awsCreds AWSCredentials, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, KubectlDatadogPath(), args...)

	// Set minimal environment with explicit credentials
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"KUBECONFIG=" + kubeconfigPath,
		"AWS_ACCESS_KEY_ID=" + awsCreds.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY=" + awsCreds.SecretAccessKey,
		"AWS_REGION=" + awsCreds.Region,
	}
	if awsCreds.SessionToken != "" {
		cmd.Env = append(cmd.Env, "AWS_SESSION_TOKEN="+awsCreds.SessionToken)
	}

	output, err := cmd.CombinedOutput()
	return string(output), err
}

// RunAutoscalingInstall runs kubectl datadog autoscaling cluster install
func RunAutoscalingInstall(ctx context.Context, kubeconfigPath string, awsCreds AWSCredentials, clusterName string, extraArgs ...string) (string, error) {
	args := []string{"autoscaling", "cluster", "install", "--cluster-name", clusterName}
	args = append(args, extraArgs...)
	return RunKubectlDatadog(ctx, kubeconfigPath, awsCreds, args...)
}

// RunAutoscalingUninstall runs kubectl datadog autoscaling cluster uninstall
func RunAutoscalingUninstall(ctx context.Context, kubeconfigPath string, awsCreds AWSCredentials, clusterName string) (string, error) {
	return RunKubectlDatadog(ctx, kubeconfigPath, awsCreds, "autoscaling", "cluster", "uninstall", "--cluster-name", clusterName, "--yes")
}
