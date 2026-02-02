// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autoscalingsuite

import (
	"context"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-operator/test/e2e/common"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// autoscalingSuite tests kubectl datadog autoscaling cluster install and uninstall commands
type autoscalingSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
	kubeconfigPath string
	clusterName    string
	awsCreds       common.AWSCredentials
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

	creds, err := cfg.Credentials.Retrieve(s.T().Context())
	require.NoError(s.T(), err, "Failed to retrieve AWS credentials")

	s.awsCreds = common.AWSCredentials{
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
	output, err := common.RunAutoscalingUninstall(ctx, s.kubeconfigPath, s.awsCreds, s.clusterName)
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
		s.testInstallWithDefaults()
	})

	s.Run("Install is idempotent", func() {
		s.testInstallWithDefaults()
	})

	s.Run("Uninstall cleans up resources", func() {
		s.testUninstallCleansUp()
	})

	s.Run("Uninstall is idempotent", func() {
		s.testUninstallCleansUp()
	})

	s.Run("Install with create-karpenter-resources=none", func() {
		s.testInstallWithNoResources()
	})

	s.Run("Install with inference-method=nodes", func() {
		s.testInstallWithNodesInference()
	})
}

// testInstallWithDefaults tests the default install flow
func (s *autoscalingSuite) testInstallWithDefaults() {
	ctx, cancel := context.WithTimeout(s.T().Context(), 15*time.Minute)
	defer cancel()

	// Run install
	output, err := common.RunAutoscalingInstall(ctx, s.kubeconfigPath, s.awsCreds, s.clusterName)
	require.NoError(s.T(), err, "Install command failed. Output: %s", output)
	s.T().Logf("Install output: %s", output)

	// Verify installation
	s.verifyKarpenterInstalled(ctx)
}

// testUninstallCleansUp tests that uninstall removes all resources
func (s *autoscalingSuite) testUninstallCleansUp() {
	ctx, cancel := context.WithTimeout(s.T().Context(), 20*time.Minute)
	defer cancel()

	// Run uninstall
	output, err := common.RunAutoscalingUninstall(ctx, s.kubeconfigPath, s.awsCreds, s.clusterName)
	require.NoError(s.T(), err, "Uninstall command failed. Output: %s", output)
	s.T().Logf("Uninstall output: %s", output)

	// Verify cleanup
	s.verifyCleanUninstall(ctx)
}

// testInstallWithNoResources tests install with --create-karpenter-resources=none
func (s *autoscalingSuite) testInstallWithNoResources() {
	ctx, cancel := context.WithTimeout(s.T().Context(), 15*time.Minute)
	defer cancel()

	// Run install with --create-karpenter-resources=none
	output, err := common.RunAutoscalingInstall(ctx, s.kubeconfigPath, s.awsCreds, s.clusterName, "--create-karpenter-resources=none")
	require.NoError(s.T(), err, "Install with --create-karpenter-resources=none failed. Output: %s", output)
	s.T().Logf("Install output: %s", output)

	// Verify Karpenter pods are running
	err = common.WaitForKarpenterPods(ctx, s.Env().KubernetesCluster.Client(), common.KarpenterNamespace, 5*time.Minute)
	require.NoError(s.T(), err, "Karpenter pods not running")

	// Verify CloudFormation stacks exist
	cfnClient, err := common.CloudFormationClient(ctx, s.awsCreds)
	require.NoError(s.T(), err, "Failed to create CloudFormation client")

	err = common.VerifyCloudFormationStacks(ctx, cfnClient, s.clusterName)
	require.NoError(s.T(), err, "CloudFormation stacks verification failed")

	// Note: We don't verify Karpenter CRs here because they were not created
	// The actual behavior depends on whether previous test runs left CRs

	// Cleanup for next test
	output, err = common.RunAutoscalingUninstall(ctx, s.kubeconfigPath, s.awsCreds, s.clusterName)
	require.NoError(s.T(), err, "Cleanup uninstall failed. Output: %s", output)

	// Wait for cleanup to complete
	time.Sleep(10 * time.Second)
}

// testInstallWithNodesInference tests install with --inference-method=nodes
func (s *autoscalingSuite) testInstallWithNodesInference() {
	ctx, cancel := context.WithTimeout(s.T().Context(), 15*time.Minute)
	defer cancel()

	// Run install with --inference-method=nodes
	output, err := common.RunAutoscalingInstall(ctx, s.kubeconfigPath, s.awsCreds, s.clusterName, "--inference-method=nodes")
	require.NoError(s.T(), err, "Install with --inference-method=nodes failed. Output: %s", output)
	s.T().Logf("Install output: %s", output)

	// Verify installation
	s.verifyKarpenterInstalled(ctx)

	// Cleanup
	output, err = common.RunAutoscalingUninstall(ctx, s.kubeconfigPath, s.awsCreds, s.clusterName)
	require.NoError(s.T(), err, "Cleanup uninstall failed. Output: %s", output)
}

// verifyKarpenterInstalled verifies that Karpenter is fully installed
func (s *autoscalingSuite) verifyKarpenterInstalled(ctx context.Context) {
	s.T().Log("Verifying Karpenter installation...")

	// Create CloudFormation client
	cfnClient, err := common.CloudFormationClient(ctx, s.awsCreds)
	require.NoError(s.T(), err, "Failed to create CloudFormation client")

	// Verify CloudFormation stacks
	s.Assert().EventuallyWithT(func(c *assert.CollectT) {
		err := common.VerifyCloudFormationStacks(ctx, cfnClient, s.clusterName)
		assert.NoError(c, err, "CloudFormation stacks not found")
	}, 5*time.Minute, 30*time.Second, "CloudFormation stacks verification failed")

	// Verify Karpenter pods
	s.Assert().EventuallyWithT(func(c *assert.CollectT) {
		err := common.VerifyKarpenterPods(ctx, s.Env().KubernetesCluster.Client(), common.KarpenterNamespace)
		assert.NoError(c, err, "Karpenter pods not running")
	}, 5*time.Minute, 30*time.Second, "Karpenter pods verification failed")

	// Verify Helm release exists
	s.Assert().EventuallyWithT(func(c *assert.CollectT) {
		exists, err := common.VerifyHelmReleaseExists(ctx, s.Env().KubernetesCluster.Client(), common.KarpenterNamespace, "karpenter")
		assert.NoError(c, err, "Failed to check Helm release")
		assert.True(c, exists, "Karpenter Helm release not found")
	}, 2*time.Minute, 10*time.Second, "Helm release verification failed")

	s.T().Log("Karpenter installation verified successfully")
}

// verifyCleanUninstall verifies that all Karpenter resources have been cleaned up
func (s *autoscalingSuite) verifyCleanUninstall(ctx context.Context) {
	s.T().Log("Verifying clean uninstall...")

	// Create CloudFormation client
	cfnClient, err := common.CloudFormationClient(ctx, s.awsCreds)
	require.NoError(s.T(), err, "Failed to create CloudFormation client")

	// Verify CloudFormation stacks are deleted
	s.Assert().EventuallyWithT(func(c *assert.CollectT) {
		err := common.VerifyCloudFormationStacksDeleted(ctx, cfnClient, s.clusterName)
		assert.NoError(c, err, "CloudFormation stacks still exist")
	}, 15*time.Minute, 30*time.Second, "CloudFormation stacks deletion verification failed")

	// Verify no Karpenter pods
	s.Assert().EventuallyWithT(func(c *assert.CollectT) {
		err := common.VerifyNoKarpenterPods(ctx, s.Env().KubernetesCluster.Client(), common.KarpenterNamespace)
		assert.NoError(c, err, "Karpenter pods still exist")
	}, 5*time.Minute, 30*time.Second, "Karpenter pods still exist")

	// Verify Helm release is gone
	s.Assert().EventuallyWithT(func(c *assert.CollectT) {
		exists, err := common.VerifyHelmReleaseExists(ctx, s.Env().KubernetesCluster.Client(), common.KarpenterNamespace, "karpenter")
		if err == nil {
			assert.False(c, exists, "Karpenter Helm release still exists")
		}
		// If error (namespace doesn't exist), that's fine
	}, 2*time.Minute, 10*time.Second, "Helm release still exists")

	s.T().Log("Clean uninstall verified successfully")
}
