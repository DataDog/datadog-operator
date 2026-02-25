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
	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/utils/ptr"
)

const (
	karpenterNamespace    = "dd-karpenter"
	testWorkloadName      = "karpenter-test-workload"
	testWorkloadNamespace = "default"
	testWorkloadReplicas  = 5 // Must be > initial number of nodes to force Pending pods
)

var (
	testWorkloadSelector = map[string]string{"app": testWorkloadName}
)

// autoscalingSuite tests kubectl datadog autoscaling cluster install and uninstall commands
type autoscalingSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
	kubeconfigPath string
	clusterName    string
	awsCfg         awssdk.Config
	cfnClient      *cloudformation.Client
}

func (s *autoscalingSuite) SetupSuite() {
	t := s.T()

	s.BaseSuite.SetupSuite()
	s.extractClusterInfo()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	s.deployTestWorkload(ctx)

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
		defer cancel()
		s.deleteTestWorkload(ctx)

		s.cleanupKarpenterResources()
	})
}

// extractClusterInfo extracts kubeconfig, cluster name, and AWS credentials from the EKS environment
func (s *autoscalingSuite) extractClusterInfo() {
	t := s.T()

	kubeconfigFile, err := os.CreateTemp("", "kubeconfig.autoscaling-e2e.*")
	require.NoError(t, err)

	_, err = kubeconfigFile.WriteString(s.Env().KubernetesCluster.KubeConfig)
	require.NoError(t, err, "Failed to write kubeconfig to file %s", kubeconfigFile.Name())

	err = kubeconfigFile.Close()
	require.NoError(t, err, "Failed to close kubeconfig to file %s", kubeconfigFile.Name())

	s.kubeconfigPath = kubeconfigFile.Name()

	s.clusterName = s.Env().KubernetesCluster.ClusterName

	cfg, err := config.LoadDefaultConfig(t.Context())
	require.NoError(t, err, "Failed to load AWS config")

	if cfg.Region == "" {
		cfg.Region = "us-east-1" // Fallback default
	}

	s.awsCfg = cfg
	s.cfnClient = cloudformation.NewFromConfig(cfg)

	t.Logf("EKS cluster name: %s", s.clusterName)
	t.Logf("Kubeconfig path: %s", s.kubeconfigPath)
	t.Logf("AWS region: %s", s.awsCfg.Region)
}

// cleanupKarpenterResources ensures Karpenter resources are cleaned up at the end of the suite
func (s *autoscalingSuite) cleanupKarpenterResources() {
	t := s.T()

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Minute)
	defer cancel()

	t.Log("Cleaning up Karpenter resources...")

	// Run uninstall to clean up
	output, err := s.runKubectlDatadog(ctx, "autoscaling", "cluster", "uninstall", "--cluster-name", s.clusterName, "--yes")
	if err != nil {
		t.Logf("Warning: cleanup uninstall failed (may be expected if already cleaned): %v\nOutput: %s", err, output)
	}

	// Clean up kubeconfig file
	if s.kubeconfigPath != "" {
		os.Remove(s.kubeconfigPath)
	}
}

// deployTestWorkload creates a Deployment with anti-affinity to force 1 pod per node
func (s *autoscalingSuite) deployTestWorkload(ctx context.Context) {
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      testWorkloadName,
			Namespace: testWorkloadNamespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To(int32(testWorkloadReplicas)),
			Selector: &metav1.LabelSelector{
				MatchLabels: testWorkloadSelector,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: testWorkloadSelector,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "pause",
						Image: "registry.k8s.io/pause",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("100m"),
								corev1.ResourceMemory: resource.MustParse("128Mi"),
							},
						},
					}},
					Affinity: &corev1.Affinity{
						PodAntiAffinity: &corev1.PodAntiAffinity{
							RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: testWorkloadSelector,
								},
								TopologyKey: "kubernetes.io/hostname",
							}},
						},
					},
				},
			},
		},
	}

	_, err := s.Env().KubernetesCluster.Client().AppsV1().Deployments(testWorkloadNamespace).Create(ctx, deployment, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		require.NoError(s.T(), err, "Failed to create test workload")
	}
}

// deleteTestWorkload removes the test workload
func (s *autoscalingSuite) deleteTestWorkload(ctx context.Context) {
	err := s.Env().KubernetesCluster.Client().AppsV1().Deployments(testWorkloadNamespace).Delete(ctx, testWorkloadName, metav1.DeleteOptions{})
	if err != nil && !errors.IsNotFound(err) {
		s.T().Logf("Warning: failed to delete test workload: %v", err)
	}
}

// countPodsByPhase counts pods of the test workload by phase
func (s *autoscalingSuite) countPodsByPhase(ctx context.Context) (running, pending int) {
	pods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(testWorkloadNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: labels.FormatLabels(testWorkloadSelector),
	})
	require.NoError(s.T(), err, "Failed to list pods")

	for _, pod := range pods.Items {
		switch pod.Status.Phase {
		case corev1.PodRunning:
			running++
		case corev1.PodPending:
			pending++
		}
	}
	return running, pending
}

// waitForPendingPods waits until at least minPending pods are in Pending state
func (s *autoscalingSuite) waitForPendingPods(ctx context.Context, minPending int) {
	t := s.T()
	t.Logf("Waiting for at least %d pending pods...", minPending)

	require.Eventually(t, func() bool {
		_, pending := s.countPodsByPhase(ctx)
		t.Logf("Current state: pending=%d", pending)
		return pending >= minPending
	}, 2*time.Minute, 5*time.Second, "Expected at least %d pending pods", minPending)
}

// waitForAllPodsRunning waits until all pods are Running
func (s *autoscalingSuite) waitForAllPodsRunning(ctx context.Context) {
	t := s.T()
	t.Log("Waiting for all pods to be running...")

	require.Eventually(t, func() bool {
		running, pending := s.countPodsByPhase(ctx)
		t.Logf("Current state: running=%d, pending=%d", running, pending)
		return running == testWorkloadReplicas && pending == 0
	}, 10*time.Minute, 10*time.Second, "Expected all %d pods to be running", testWorkloadReplicas)
}

// TestAutoscaling runs all autoscaling tests sequentially on the shared EKS cluster
func (s *autoscalingSuite) TestAutoscalingDefault() {
	ctx := s.T().Context()

	s.Run("Install with defaults", func() {
		s.testInstall()
		s.waitForAllPodsRunning(ctx)
	})

	s.Run("Install is idempotent", func() {
		s.testInstall()
		s.waitForAllPodsRunning(ctx)
	})

	s.Run("Uninstall cleans up resources", func() {
		s.testUninstall()
		s.waitForPendingPods(ctx, 1)
	})

	s.Run("Uninstall is idempotent", func() {
		s.testUninstall()
		s.waitForPendingPods(ctx, 1)
	})
}

func (s *autoscalingSuite) TestAutoscalingNoNodePool() {
	ctx := s.T().Context()

	s.Run("Install with create-karpenter-resources=none", func() {
		s.testInstall("--create-karpenter-resources=none")
		// Without EC2NodeClass and NodePools, pods will remain pending
	})

	s.Run("Uninstall cleans up resources", func() {
		s.testUninstall()
		s.waitForPendingPods(ctx, 1)
	})
}

func (s *autoscalingSuite) TestAutoscalingInferenceMethodNodeGroup() {
	ctx := s.T().Context()

	s.Run("Install with inference-method=nodegroups", func() {
		s.testInstall("--inference-method=nodegroups")
		s.waitForAllPodsRunning(ctx)
	})

	s.Run("Uninstall cleans up resources", func() {
		s.testUninstall()
		s.waitForPendingPods(ctx, 1)
	})
}

func (s *autoscalingSuite) TestAutoscalingInferenceMethodNodes() {
	ctx := s.T().Context()

	s.Run("Install with inference-method=nodes", func() {
		s.testInstall("--inference-method=nodes")
		s.waitForAllPodsRunning(ctx)
	})

	s.Run("Uninstall cleans up resources", func() {
		s.testUninstall()
		s.waitForPendingPods(ctx, 1)
	})
}

// testInstall tests the default install flow
func (s *autoscalingSuite) testInstall(extraArgs ...string) {
	t := s.T()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Minute)
	defer cancel()

	// Run install
	args := append([]string{"autoscaling", "cluster", "install", "--cluster-name", s.clusterName}, extraArgs...)
	output, err := s.runKubectlDatadog(ctx, args...)
	require.NoErrorf(t, err, "Install command failed. Output:\n%s", output)
	t.Logf("Install output:\n%s", output)

	// Verify installation
	s.verifyKarpenterInstalled(ctx)
}

// testUninstall tests that uninstall removes all resources
func (s *autoscalingSuite) testUninstall() {
	t := s.T()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Minute)
	defer cancel()

	// Run uninstall
	output, err := s.runKubectlDatadog(ctx, "autoscaling", "cluster", "uninstall", "--cluster-name", s.clusterName, "--yes")
	require.NoErrorf(t, err, "Uninstall command failed. Output:\n%s", output)
	t.Logf("Uninstall output:\n%s", output)

	// Verify cleanup
	s.verifyCleanUninstall(ctx)
}

// runKubectlDatadog executes kubectl-datadog with the AWS profile-based
// credential chain so the subprocess can refresh its own STS tokens during
// long-running operations (e.g. CloudFormation stack creation).
func (s *autoscalingSuite) runKubectlDatadog(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, filepath.Join(common.ProjectRootPath, "bin", "kubectl-datadog"), args...)

	// Propagate the AWS profile and config files instead of static STS
	// tokens. This lets the subprocess call AssumeRole itself and refresh
	// credentials if they expire during long-running CloudFormation waits.
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"KUBECONFIG=" + s.kubeconfigPath,
		"AWS_REGION=" + s.awsCfg.Region,
	}
	if v, ok := os.LookupEnv("AWS_PROFILE"); ok {
		cmd.Env = append(cmd.Env, "AWS_PROFILE="+v)
	}
	if v, ok := os.LookupEnv("AWS_CONFIG_FILE"); ok {
		cmd.Env = append(cmd.Env, "AWS_CONFIG_FILE="+v)
	}
	if v, ok := os.LookupEnv("AWS_SHARED_CREDENTIALS_FILE"); ok {
		cmd.Env = append(cmd.Env, "AWS_SHARED_CREDENTIALS_FILE="+v)
	}

	output, err := cmd.CombinedOutput()
	return string(output), err
}

// verifyKarpenterInstalled verifies that Karpenter is fully installed
func (s *autoscalingSuite) verifyKarpenterInstalled(ctx context.Context) {
	t := s.T()
	t.Log("Verifying Karpenter installation...")

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
	configFlags := genericclioptions.NewConfigFlags(false)
	configFlags.KubeConfig = &s.kubeconfigPath
	actionConfig, err := helm.NewActionConfig(configFlags, karpenterNamespace)
	s.Assert().NoErrorf(err, "Error creating Helm action config")

	exists, err := helm.Exists(ctx, actionConfig, "karpenter")
	s.Assert().NoErrorf(err, "Error checking Helm release")
	s.Assert().Truef(exists, "Karpenter Helm release not found")

	t.Log("Karpenter installation verified successfully")
}

// verifyCleanUninstall verifies that all Karpenter resources have been cleaned up
func (s *autoscalingSuite) verifyCleanUninstall(ctx context.Context) {
	t := s.T()
	t.Log("Verifying clean uninstall...")

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
	configFlags := genericclioptions.NewConfigFlags(false)
	configFlags.KubeConfig = &s.kubeconfigPath
	actionConfig, err := helm.NewActionConfig(configFlags, karpenterNamespace)
	s.Assert().NoErrorf(err, "Error creating Helm action config")

	exists, err := helm.Exists(ctx, actionConfig, "karpenter")
	s.Assert().NoErrorf(err, "Error checking Helm release")
	s.Assert().Falsef(exists, "Karpenter Helm release still exists")

	t.Log("Clean uninstall verified successfully")
}
