// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autoscalingsuite

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/aws"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/helm"
	"github.com/DataDog/datadog-operator/test/e2e/common"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

const karpenterNamespace = "dd-karpenter"

// awsCredentials holds AWS credentials loaded from the SDK's default credential chain
type awsCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string // may be empty for static credentials
	Region          string
}

// autoscalingSuite tests kubectl datadog autoscaling cluster install and uninstall commands
type autoscalingSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
	kubeconfigPath string
	clusterName    string
	awsCreds       awsCredentials
	cfnClient      *cloudformation.Client
}

func (s *autoscalingSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()
	s.extractClusterInfo()

	s.T().Cleanup(func() {
		s.cleanupKarpenterResources()
	})
}

// extractClusterInfo extracts kubeconfig, cluster name, and AWS credentials from the EKS environment
func (s *autoscalingSuite) extractClusterInfo() {
	kubeconfigFile, err := os.CreateTemp("", "autoscaling-e2e-kubeconfig-*")
	require.NoError(s.T(), err)

	_, err = kubeconfigFile.WriteString(s.Env().KubernetesCluster.KubeConfig)
	require.NoError(s.T(), err, "Failed to write kubeconfig to file %s", kubeconfigFile.Name())

	err = kubeconfigFile.Close()
	require.NoError(s.T(), err, "Failed to close kubeconfig to file %s", kubeconfigFile.Name())

	s.kubeconfigPath = kubeconfigFile.Name()

	s.clusterName = s.Env().KubernetesCluster.ClusterName

	cfg, err := config.LoadDefaultConfig(s.T().Context())
	require.NoError(s.T(), err, "Failed to load AWS config")

	s.cfnClient = cloudformation.NewFromConfig(cfg)

	creds, err := cfg.Credentials.Retrieve(s.T().Context())
	require.NoError(s.T(), err, "Failed to retrieve AWS credentials")

	s.awsCreds = awsCredentials{
		AccessKeyID:     creds.AccessKeyID,
		SecretAccessKey: creds.SecretAccessKey,
		SessionToken:    creds.SessionToken,
		Region:          cfg.Region,
	}
	if s.awsCreds.Region == "" {
		s.awsCreds.Region = "us-east-1" // Fallback default
	}

	s.T().Logf("EKS cluster name: %s", s.clusterName)
	s.T().Logf("Kubeconfig path: %s", s.kubeconfigPath)
	s.T().Logf("AWS region: %s", s.awsCreds.Region)
}

// cleanupKarpenterResources ensures Karpenter resources are cleaned up at the end of the suite
func (s *autoscalingSuite) cleanupKarpenterResources() {
	ctx, cancel := context.WithTimeout(s.T().Context(), 20*time.Minute)
	defer cancel()

	s.T().Log("Cleaning up Karpenter resources...")

	// Run uninstall to clean up
	output, err := s.runKubectlDatadog(ctx, "autoscaling", "cluster", "uninstall", "--cluster-name", s.clusterName, "--yes")
	if err != nil {
		s.T().Logf("Warning: cleanup uninstall failed (may be expected if already cleaned): %v\nOutput: %s", err, output)
	}

	// Clean up kubeconfig file
	if s.kubeconfigPath != "" {
		os.Remove(s.kubeconfigPath)
	}
}

// TestAutoscaling runs all autoscaling tests sequentially on the shared EKS cluster
func (s *autoscalingSuite) TestAutoscaling() {
	s.Run("Install with defaults", func() {
		s.testInstall()
	})

	s.Run("Install is idempotent", func() {
		s.testInstall()
	})

	s.Run("Uninstall cleans up resources", func() {
		s.testUninstall()
	})

	s.Run("Uninstall is idempotent", func() {
		s.testUninstall()
	})

	s.Run("Install with create-karpenter-resources=none", func() {
		s.testInstall("--create-karpenter-resources=none")
	})

	s.Run("Uninstall cleans up resources", func() {
		s.testUninstall()
	})

	s.Run("Install with inference-method=nodes", func() {
		s.testInstall("--inference-method=nodes")
	})

	s.Run("Uninstall cleans up resources", func() {
		s.testUninstall()
	})
}

// testInstall tests the default install flow
func (s *autoscalingSuite) testInstall(extraArgs ...string) {
	ctx, cancel := context.WithTimeout(s.T().Context(), 15*time.Minute)
	defer cancel()

	// Run install
	args := append([]string{"autoscaling", "cluster", "install", "--cluster-name", s.clusterName}, extraArgs...)
	output, err := s.runKubectlDatadog(ctx, args...)
	require.NoErrorf(s.T(), err, "Install command failed. Output: %s", output)
	s.T().Logf("Install output: %s", output)

	// Verify installation
	s.verifyKarpenterInstalled(ctx)
}

// testUninstall tests that uninstall removes all resources
func (s *autoscalingSuite) testUninstall() {
	ctx, cancel := context.WithTimeout(s.T().Context(), 20*time.Minute)
	defer cancel()

	// Run uninstall
	output, err := s.runKubectlDatadog(ctx, "autoscaling", "cluster", "uninstall", "--cluster-name", s.clusterName, "--yes")
	require.NoErrorf(s.T(), err, "Uninstall command failed. Output: %s", output)
	s.T().Logf("Uninstall output: %s", output)

	// Verify cleanup
	s.verifyCleanUninstall(ctx)
}

// runKubectlDatadog executes kubectl-datadog with the suite's AWS credentials
func (s *autoscalingSuite) runKubectlDatadog(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, filepath.Join(common.ProjectRootPath, "bin", "kubectl-datadog"), args...)

	// Set minimal environment with explicit credentials
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"KUBECONFIG=" + s.kubeconfigPath,
		"AWS_ACCESS_KEY_ID=" + s.awsCreds.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY=" + s.awsCreds.SecretAccessKey,
		"AWS_REGION=" + s.awsCreds.Region,
	}
	if s.awsCreds.SessionToken != "" {
		cmd.Env = append(cmd.Env, "AWS_SESSION_TOKEN="+s.awsCreds.SessionToken)
	}

	output, err := cmd.CombinedOutput()
	return string(output), err
}

// verifyKarpenterInstalled verifies that Karpenter is fully installed
func (s *autoscalingSuite) verifyKarpenterInstalled(ctx context.Context) {
	s.T().Log("Verifying Karpenter installation...")

	// Verify CloudFormation stacks
	for _, stackName := range []string{
		"dd-karpenter-" + s.clusterName + "-karpenter",
		"dd-karpenter-" + s.clusterName + "-dd-karpenter",
	} {
		exists, err := aws.DoesStackExist(ctx, s.cfnClient, stackName)
		s.Assert().NoErrorf(err, "Error checking stack %s", stackName)
		s.Assert().Truef(exists, "CloudFormation stack %s not found", stackName)
	}

	// Verify Helm release
	actionConfig, err := helm.NewActionConfig(genericclioptions.NewConfigFlags(false), karpenterNamespace)
	s.Assert().NoErrorf(err, "Error creating Helm action config")

	exists, err := helm.DoesExist(ctx, actionConfig, "karpenter")
	s.Assert().NoErrorf(err, "Error checking Helm release")
	s.Assert().Truef(exists, "Karpenter Helm release not found")

	s.T().Log("Karpenter installation verified successfully")
}

// verifyCleanUninstall verifies that all Karpenter resources have been cleaned up
func (s *autoscalingSuite) verifyCleanUninstall(ctx context.Context) {
	s.T().Log("Verifying clean uninstall...")

	// Verify CloudFormation stacks
	for _, stackName := range []string{
		"dd-karpenter-" + s.clusterName + "-karpenter",
		"dd-karpenter-" + s.clusterName + "-dd-karpenter",
	} {
		exists, err := aws.DoesStackExist(ctx, s.cfnClient, stackName)
		s.Assert().NoErrorf(err, "Error checking stack %s", stackName)
		s.Assert().Falsef(exists, "CloudFormation stack %s still exists", stackName)
	}

	// Verify Helm release
	actionConfig, err := helm.NewActionConfig(genericclioptions.NewConfigFlags(false), karpenterNamespace)
	s.Assert().NoErrorf(err, "Error creating Helm action config")

	exists, err := helm.DoesExist(ctx, actionConfig, "karpenter")
	s.Assert().NoErrorf(err, "Error checking Helm release")
	s.Assert().Falsef(exists, "Karpenter Helm release still exists")

	s.T().Log("Clean uninstall verified successfully")
}
