// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autoscalingsuite

import (
	"context"
	"time"

	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// TestEvictLegacyNodes exercises `kubectl datadog autoscaling cluster
// evict-legacy-nodes` end-to-end against the shared EKS cluster. The subtests
// run sequentially in a single method to keep cluster state flowing in a
// controlled order: the dry-run must run while the EKS managed node groups
// still have nodes, and the real drain (which scales those node groups to zero)
// must run after it and before uninstall.
func (s *autoscalingSuite) TestEvictLegacyNodes() {
	// aws.LabelEKSNodegroup ("eks.amazonaws.com/nodegroup") marks the harness'
	// two managed node groups (amd64 + arm64) — the "legacy" (non-Datadog)
	// groups evict-legacy-nodes drains. Reusing the production constant keeps
	// this test coupled to the label the drain code actually keys off.
	//
	// karpenterNodePoolLabel is set by Karpenter on the nodes it provisions;
	// after a drain, every workload pod must land on such a node.
	const karpenterNodePoolLabel = "karpenter.sh/nodepool"

	ctx := s.T().Context()

	s.Run("Refuses when Karpenter not installed", func() {
		t := s.T()
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
		defer cancel()

		// --dry-run so a violated no-Karpenter precondition can't drain the
		// cluster before the "Karpenter is not installed" gate fires (the gate
		// runs before any mutation, dry-run or not, so the assertion still holds).
		output, err := s.runEvict(ctx, "--all", "--dry-run")
		require.Errorf(t, err, "evict-legacy-nodes should fail when no Karpenter is installed; got success.\nOutput:\n%s", output)
		require.Containsf(t, output, "Karpenter is not installed",
			"Expected explanatory error pointing at 'install'.\nOutput:\n%s", output)
	})

	s.Run("Validation errors", func() {
		cases := []struct {
			name     string
			args     []string
			wantText string
		}{
			{
				name:     "all and target are mutually exclusive",
				args:     []string{"--all", "--target=asg/foo"},
				wantText: "mutually exclusive",
			},
			{
				name:     "neither all nor target",
				args:     []string{},
				wantText: "at least one of --all or --target",
			},
			{
				name:     "non-positive eviction-timeout",
				args:     []string{"--all", "--eviction-timeout=0"},
				wantText: "--eviction-timeout must be positive",
			},
			{
				name:     "fargate is not a valid target",
				args:     []string{"--target=fargate/foo"},
				wantText: "--target=fargate is not supported",
			},
		}

		for _, tc := range cases {
			s.Run(tc.name, func() {
				t := s.T()
				ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
				defer cancel()

				output, err := s.runEvict(ctx, tc.args...)
				require.Errorf(t, err, "expected a validation error.\nOutput:\n%s", output)
				require.Containsf(t, output, tc.wantText, "unexpected error message.\nOutput:\n%s", output)
			})
		}
	})

	s.Run("Install", func() {
		s.testInstall()
		s.waitForAllPodsRunning(ctx)
	})

	s.Run("Dry-run does not mutate", func() {
		t := s.T()
		dctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
		defer cancel()

		output, err := s.runEvict(dctx, "--all", "--dry-run", "--yes")
		require.NoErrorf(t, err, "dry-run should succeed.\nOutput:\n%s", output)
		assert.Containsf(t, output, "[dry-run]", "dry-run output should be flagged as such.\nOutput:\n%s", output)

		// Fresh context so the pod-running wait keeps its full budget regardless
		// of how long the command above took (see the drain subtest for details).
		vctx, cancelV := context.WithTimeout(t.Context(), 12*time.Minute)
		defer cancelV()

		// Dry-run must not touch the cluster: the legacy EKS managed node group
		// nodes (the drain targets) must still be present and uncordoned. We
		// only inspect those nodes, not every node, so an unrelated Karpenter
		// consolidation cordon cannot make this assertion flaky.
		nodes, err := s.Env().KubernetesCluster.Client().CoreV1().Nodes().List(vctx, metav1.ListOptions{})
		require.NoError(t, err, "listing nodes")

		mngNodes := 0
		for _, node := range nodes.Items {
			if _, ok := node.Labels[aws.LabelEKSNodegroup]; !ok {
				continue
			}
			mngNodes++
			assert.Falsef(t, node.Spec.Unschedulable, "EKS managed node group node %s should not be cordoned by a dry-run", node.Name)
		}
		assert.Positive(t, mngNodes, "EKS managed node group nodes should still be present after a dry-run")

		s.waitForAllPodsRunning(vctx)
	})

	s.Run("Drain all legacy node groups", func() {
		t := s.T()

		// This scales the EKS managed node groups to zero and does not restore
		// them; that only works because the e2e framework provisions an ephemeral
		// cluster per run. The command drains the two groups sequentially, each
		// waiting up to --node-timeout (15m default) to empty, so give it a wide
		// budget to avoid killing a slow-but-valid run mid-migration.
		ectx, cancel := context.WithTimeout(t.Context(), 45*time.Minute)
		defer cancel()

		output, err := s.runEvict(ectx, "--all", "--yes")
		require.NoErrorf(t, err, "evict-legacy-nodes --all failed.\nOutput:\n%s", output)
		assert.Containsf(t, output, "Legacy nodes drained from cluster", "expected success message.\nOutput:\n%s", output)

		// Verify against a fresh context so the assertions below always have
		// their full budget, however long the command above took. Sized to
		// comfortably cover the node-disappearance poll plus the pod-running wait.
		vctx, cancelV := context.WithTimeout(t.Context(), 20*time.Minute)
		defer cancelV()

		// The command scales the EKS managed node groups to zero and waits for
		// their nodes to disappear before returning, so they should be gone.
		client := s.Env().KubernetesCluster.Client()
		require.Eventuallyf(t, func() bool {
			nodes, err := client.CoreV1().Nodes().List(vctx, metav1.ListOptions{})
			if err != nil {
				t.Logf("listing nodes: %v", err)
				return false
			}
			for _, node := range nodes.Items {
				if _, ok := node.Labels[aws.LabelEKSNodegroup]; ok {
					return false
				}
			}
			return true
		}, 5*time.Minute, 10*time.Second, "EKS managed node group nodes should be drained after eviction")

		// All workload pods must have rescheduled onto Datadog-managed Karpenter nodes.
		s.waitForAllPodsRunning(vctx)

		// List pods before nodes so the node set is at least as fresh as the pod
		// placements it is checked against (any node hosting a pod still exists
		// when the node set is built moments later).
		pods, err := client.CoreV1().Pods(testWorkloadNamespace).List(vctx, metav1.ListOptions{
			LabelSelector: labels.FormatLabels(testWorkloadSelector),
		})
		require.NoError(t, err, "listing workload pods")

		nodeList, err := client.CoreV1().Nodes().List(vctx, metav1.ListOptions{})
		require.NoError(t, err, "listing nodes")
		karpenterNodes := make(map[string]bool)
		for _, node := range nodeList.Items {
			if _, ok := node.Labels[karpenterNodePoolLabel]; ok {
				karpenterNodes[node.Name] = true
			}
		}

		for _, pod := range pods.Items {
			require.NotEmptyf(t, pod.Spec.NodeName, "pod %s has no node assigned", pod.Name)
			assert.Truef(t, karpenterNodes[pod.Spec.NodeName],
				"pod %s should run on a Karpenter node; got %s", pod.Name, pod.Spec.NodeName)
		}
	})

	s.Run("Evict is idempotent", func() {
		t := s.T()
		dctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
		defer cancel()

		output, err := s.runEvict(dctx, "--all", "--yes")
		require.NoErrorf(t, err, "a second eviction should be a no-op.\nOutput:\n%s", output)
		assert.Containsf(t, output, "Nothing to evict", "expected no-op message.\nOutput:\n%s", output)
	})

	s.Run("Uninstall", func() {
		s.testUninstall()
		s.waitForPendingPods(ctx, 1)
	})
}

// runEvict invokes `autoscaling cluster evict-legacy-nodes` against the shared
// cluster, appending extraArgs to the common command prefix.
func (s *autoscalingSuite) runEvict(ctx context.Context, extraArgs ...string) (string, error) {
	args := append([]string{"autoscaling", "cluster", "evict-legacy-nodes", "--cluster-name", s.clusterName}, extraArgs...)
	return s.runKubectlDatadog(ctx, args...)
}
