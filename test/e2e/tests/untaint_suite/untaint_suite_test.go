// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package untaintsuite contains end-to-end tests for the operator's Untaint
// controller (--untaintControllerEnabled). The suite provisions a kind cluster
// with a pre-tainted worker node (carrying the agent-not-ready startup taint),
// deploys the operator with the untaint controller enabled, and verifies that a
// workload pinned to the tainted node stays Pending until the node Agent (and,
// in wait-for-CSI mode, the CSI node-server) becomes Ready and the controller
// removes the taint.
package untaintsuite

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentwithoperatorparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/operatorparams"
	frameworkkube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"

	"github.com/DataDog/datadog-operator/pkg/untaint"
	"github.com/DataDog/datadog-operator/test/e2e/common"
	"github.com/DataDog/datadog-operator/test/e2e/provisioners"
	"github.com/DataDog/datadog-operator/test/e2e/tests/utils"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	k8sclient "k8s.io/client-go/kubernetes"
)

const (
	// taintedNodeLabelKey/Value mark the worker node that is pre-tainted with the
	// agent-not-ready taint. The test workload is pinned to this node so that it
	// can only schedule once the untaint controller removes the taint.
	taintedNodeLabelKey   = "untaint-e2e/role"
	taintedNodeLabelValue = "tainted"

	// workloadName is the Pending workload pinned to the tainted node.
	workloadName      = "untaint-test-workload"
	workloadNamespace = "default"

	// ddaName is the DatadogAgent applied to bring up the node Agent.
	ddaName = "dda-untaint"

	// csiDriverName is the DatadogCSIDriver applied in wait-for-CSI mode.
	csiDriverName = "datadog-csi-driver"
)

var (
	workloadSelector = map[string]string{"app": workloadName}

	// datadogCSIDriverGVR is the GroupVersionResource for the DatadogCSIDriver CRD.
	datadogCSIDriverGVR = schema.GroupVersionResource{
		Group:    "datadoghq.com",
		Version:  "v1alpha1",
		Resource: "datadogcsidrivers",
	}
)

// untaintSuite validates the taint-based onboarding flow end-to-end.
//
// local      - use the local kind provisioner (vs. AWS kind-on-VM in CI).
// waitForCSI - run the dual-readiness flow (--untaintControllerWaitForCSIDriver),
//
//	requiring both the node Agent and the CSI node-server to be Ready before the
//	taint is removed. When false, only node Agent readiness is required.
type untaintSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
	local      bool
	waitForCSI bool

	kubeClient  k8sclient.Interface
	taintedNode string
}

func (s *untaintSuite) SetupSuite() {
	s.BaseSuite.SetupSuite()

	s.kubeClient = s.Env().KubernetesCluster.Client()
	s.identifyTaintedNode()
	s.waitForOperator()

	// The node is pre-tainted at cluster creation (kubeadm). Ensure the taint is
	// present in case the cluster is being reused across runs (e.g. E2E_DEV_MODE),
	// where a previous run's untaint controller already removed it.
	s.ensureAgentNotReadyTaint(s.taintedNode)
}

// ensureAgentNotReadyTaint makes sure the node carries the agent-not-ready taint,
// applying it if missing. Idempotent and safe on both fresh (kubeadm-tainted) and
// reused clusters.
func (s *untaintSuite) ensureAgentNotReadyTaint(nodeName string) {
	if s.nodeHasAgentNotReadyTaint(nodeName) {
		return
	}
	s.T().Logf("agent-not-ready taint missing on %s (reused cluster?); applying it", nodeName)
	node, err := s.kubeClient.CoreV1().Nodes().Get(s.T().Context(), nodeName, metav1.GetOptions{})
	s.Require().NoErrorf(err, "failed to get node %s", nodeName)
	node.Spec.Taints = append(node.Spec.Taints, untaint.AgentNotReadyTaint())
	_, err = s.kubeClient.CoreV1().Nodes().Update(s.T().Context(), node, metav1.UpdateOptions{})
	s.Require().NoErrorf(err, "failed to apply agent-not-ready taint to %s", nodeName)
}

// testName returns a stable provisioner/test name per mode. It must be identical
// across the initial provision and any UpdateEnv call so Pulumi reuses the same
// kind cluster instead of recreating it.
func testName(waitForCSI bool) string {
	if waitForCSI {
		return "e2e-untaint-csi"
	}
	return "e2e-untaint-agent"
}

// workerNodes returns the kind worker-node topology: one plain worker (hosts the
// operator, cluster-agent and other infra) plus one worker that is labeled and
// pre-tainted with the agent-not-ready startup taint.
func workerNodes() []frameworkkube.KindWorkerNode {
	return []frameworkkube.KindWorkerNode{
		{}, // untainted worker for operator / infra scheduling
		{
			Labels: []frameworkkube.Label{{Key: taintedNodeLabelKey, Value: taintedNodeLabelValue}},
			Taints: []frameworkkube.Taint{{
				Key:    untaint.AgentNotReadyTaintKey,
				Value:  untaint.AgentNotReadyTaintValue,
				Effect: string(corev1.TaintEffectNoSchedule),
			}},
		},
	}
}

// Operator Helm values live in testdata/*.yaml and are embedded at build time.
// The ${NAMESPACE} placeholder is substituted with the operator/DDA namespace.
//
//go:embed testdata/operator-values.yaml
var operatorValuesAgentOnly string

//go:embed testdata/operator-values-waitforcsi.yaml
var operatorValuesWaitForCSI string

// operatorHelmValues returns the Helm values used to deploy the operator with the
// untaint controller enabled, selecting the agent-only or wait-for-CSI variant.
func operatorHelmValues(waitForCSI bool) string {
	values := operatorValuesAgentOnly
	if waitForCSI {
		values = operatorValuesWaitForCSI
	}
	return strings.ReplaceAll(values, "${NAMESPACE}", common.NamespaceName)
}

// buildProvisionerOptions builds the provisioner options for the untaint suite.
// When withDDA is false the operator is deployed without a DatadogAgent (so the
// node Agent is absent and the taint stays); UpdateEnv with withDDA=true then
// rolls out the agent to trigger the untaint flow.
func buildProvisionerOptions(local, waitForCSI, withDDA bool) []provisioners.KubernetesProvisionerOption {
	operatorOptions := []operatorparams.Option{
		operatorparams.WithNamespace(common.NamespaceName),
		operatorparams.WithOperatorFullImagePath(common.OperatorImageName),
		operatorparams.WithHelmValues(operatorHelmValues(waitForCSI)),
	}

	opts := []provisioners.KubernetesProvisionerOption{
		provisioners.WithTestName(testName(waitForCSI)),
		provisioners.WithOperatorOptions(operatorOptions...),
		provisioners.WithKindWorkerNodes(workerNodes()...),
		provisioners.WithLocal(local),
	}

	if withDDA {
		ddaOptions := []agentwithoperatorparams.Option{
			agentwithoperatorparams.WithNamespace(common.NamespaceName),
			agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
				Name:         ddaName,
				YamlFilePath: common.DdaMinimalPath,
			}),
		}
		opts = append(opts, provisioners.WithDDAOptions(ddaOptions...))
	} else {
		opts = append(opts, provisioners.WithoutDDA())
	}

	return opts
}

// applyDDA rolls out the DatadogAgent by re-running the provisioner with a DDA.
func (s *untaintSuite) applyDDA() {
	s.T().Log("Deploying DatadogAgent to bring up the node Agent...")
	s.UpdateEnv(provisioners.KubernetesProvisioner(buildProvisionerOptions(s.local, s.waitForCSI, true)...))
}

// registerDatadogResourceCleanup deletes every DatadogAgent and
// DatadogAgentInternal in the operator namespace at the end of the test, while
// the operator is still running so it can clear their finalizers. Without this,
// the operator (Helm release) is torn down first during the Pulumi stack destroy
// and the operator-created DatadogAgentInternal is left with a dangling
// finalizer; deleting the datadogagents/datadogagentinternals CRDs then blocks
// forever on the customresourcecleanup finalizer and the destroy times out,
// failing the job even though the test assertions passed.
//
// Must be called from within a suite test method so the cleanup is scoped to
// that method's *testing.T and therefore runs before the framework's stack
// teardown (mirrors the k8s_suite cleanup).
func (s *untaintSuite) registerDatadogResourceCleanup() {
	s.T().Cleanup(func() {
		k8sConfig := s.Env().KubernetesCluster.KubernetesClient.K8sConfig
		if k8sConfig == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := utils.DeleteAllDatadogResources(ctx, k8sConfig, common.NamespaceName); err != nil {
			s.T().Logf("Warning: failed to delete Datadog resources during cleanup: %v", err)
		}
	})
}

// identifyTaintedNode finds the worker node carrying the tainted-node label.
func (s *untaintSuite) identifyTaintedNode() {
	nodes, err := s.kubeClient.CoreV1().Nodes().List(s.T().Context(), metav1.ListOptions{
		LabelSelector: fmt.Sprintf("%s=%s", taintedNodeLabelKey, taintedNodeLabelValue),
	})
	s.Require().NoError(err, "failed to list nodes")
	s.Require().Lenf(nodes.Items, 1, "expected exactly one node labeled %s=%s; check WithKindWorkerNodes",
		taintedNodeLabelKey, taintedNodeLabelValue)
	s.taintedNode = nodes.Items[0].Name
	s.T().Logf("Tainted worker node: %s", s.taintedNode)
}

// waitForOperator waits until the operator pod is Running.
func (s *untaintSuite) waitForOperator() {
	s.Require().Eventuallyf(func() bool {
		pods, err := s.kubeClient.CoreV1().Pods(common.NamespaceName).List(s.T().Context(), metav1.ListOptions{
			LabelSelector: "app.kubernetes.io/name=datadog-operator",
			FieldSelector: "status.phase=Running",
		})
		return err == nil && len(pods.Items) >= 1
	}, 5*time.Minute, 10*time.Second, "operator pod did not reach Running")
}

// nodeHasAgentNotReadyTaint reports whether the node still carries the
// agent-not-ready startup taint.
func (s *untaintSuite) nodeHasAgentNotReadyTaint(nodeName string) bool {
	node, err := s.kubeClient.CoreV1().Nodes().Get(s.T().Context(), nodeName, metav1.GetOptions{})
	if err != nil {
		s.T().Logf("warning: failed to get node %s: %v", nodeName, err)
		return false
	}
	for _, t := range node.Spec.Taints {
		if untaint.IsAgentNotReadyTaint(t) {
			return true
		}
	}
	return false
}

// deployPendingWorkload creates a single-replica Deployment pinned (via
// nodeSelector) to the tainted node with no toleration for the agent-not-ready
// taint, so it stays Pending until the taint is removed.
func (s *untaintSuite) deployPendingWorkload(ctx context.Context) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workloadName,
			Namespace: workloadNamespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptrInt32(1),
			Selector: &metav1.LabelSelector{MatchLabels: workloadSelector},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: workloadSelector},
				Spec: corev1.PodSpec{
					// Pin to the tainted node and do NOT tolerate the
					// agent-not-ready taint, so the pod is unschedulable until
					// the untaint controller removes it.
					NodeSelector: map[string]string{taintedNodeLabelKey: taintedNodeLabelValue},
					Containers: []corev1.Container{{
						Name:  "pause",
						Image: "registry.k8s.io/pause",
					}},
				},
			},
		},
	}

	deployments := s.kubeClient.AppsV1().Deployments(workloadNamespace)

	// Remove any leftover from a previous (reused-cluster) run so the workload
	// starts fresh and Pending.
	_ = deployments.Delete(ctx, workloadName, metav1.DeleteOptions{})
	s.Require().Eventually(func() bool {
		_, err := deployments.Get(ctx, workloadName, metav1.GetOptions{})
		return apierrors.IsNotFound(err)
	}, time.Minute, 2*time.Second, "leftover workload deployment was not deleted")

	_, err := deployments.Create(ctx, dep, metav1.CreateOptions{})
	s.Require().NoError(err, "failed to create pending workload")
	s.T().Cleanup(func() {
		_ = deployments.Delete(context.Background(), workloadName, metav1.DeleteOptions{})
	})
}

// getWorkloadPod returns the first workload pod, or nil if none exist yet.
func (s *untaintSuite) getWorkloadPod(ctx context.Context) *corev1.Pod {
	pods, err := s.kubeClient.CoreV1().Pods(workloadNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", workloadName),
	})
	if err != nil || len(pods.Items) == 0 {
		return nil
	}
	return &pods.Items[0]
}

// assertWorkloadEventuallyRunning waits for the workload to reach Running and, on
// each failed attempt, surfaces the pod phase, assigned node, PodScheduled
// condition, and whether the taint is still present — so a failure pinpoints why
// the pod did not schedule.
func (s *untaintSuite) assertWorkloadEventuallyRunning(ctx context.Context) {
	s.Require().EventuallyWithTf(func(c *assert.CollectT) {
		pod := s.getWorkloadPod(ctx)
		if !assert.NotNil(c, pod, "no workload pod found yet") {
			return
		}
		assert.Equalf(c, corev1.PodRunning, pod.Status.Phase,
			"workload pod %s not Running: phase=%s node=%q scheduled=[%s] taintStillPresent=%v",
			pod.Name, pod.Status.Phase, pod.Spec.NodeName, podScheduledMessage(pod), s.nodeHasAgentNotReadyTaint(s.taintedNode))
	}, 5*time.Minute, 10*time.Second, "workload should schedule and run after the taint is removed")
}

// podScheduledMessage renders the pod's PodScheduled condition for diagnostics.
func podScheduledMessage(pod *corev1.Pod) string {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodScheduled {
			return fmt.Sprintf("status=%s reason=%s msg=%q", cond.Status, cond.Reason, cond.Message)
		}
	}
	return "no PodScheduled condition"
}

func ptrInt32(i int32) *int32 { return &i }

// workloadRunning reports whether at least one workload pod is Running.
func (s *untaintSuite) workloadRunning(ctx context.Context) bool {
	pods, err := s.kubeClient.CoreV1().Pods(workloadNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app=%s", workloadName),
	})
	if err != nil {
		return false
	}
	for _, p := range pods.Items {
		if p.Status.Phase == corev1.PodRunning {
			return true
		}
	}
	return false
}

// waitForAgentReadyOnNode waits until a node Agent pod on the given node is Ready.
func (s *untaintSuite) waitForAgentReadyOnNode(ctx context.Context, nodeName string) {
	s.T().Logf("Waiting for node Agent to become Ready on %s...", nodeName)
	s.Require().Eventuallyf(func() bool {
		return s.podReadyOnNode(ctx, common.NamespaceName, common.NodeAgentSelector, nodeName)
	}, 10*time.Minute, 10*time.Second, "node Agent did not become Ready on %s", nodeName)
}

// waitForCSINodeServerReadyOnNode waits until a CSI node-server pod on the given
// node is Ready.
func (s *untaintSuite) waitForCSINodeServerReadyOnNode(ctx context.Context, nodeName string) {
	s.T().Logf("Waiting for CSI node-server to become Ready on %s...", nodeName)
	s.Require().Eventuallyf(func() bool {
		return s.podReadyOnNode(ctx, common.NamespaceName, "app=datadog-csi-driver-node-server", nodeName)
	}, 10*time.Minute, 10*time.Second, "CSI node-server did not become Ready on %s", nodeName)
}

// podReadyOnNode reports whether a pod matching selector is scheduled on nodeName
// and has all its containers Ready.
func (s *untaintSuite) podReadyOnNode(ctx context.Context, namespace, selector, nodeName string) bool {
	pods, err := s.kubeClient.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{LabelSelector: selector})
	if err != nil {
		return false
	}
	for _, pod := range pods.Items {
		if pod.Spec.NodeName != nodeName || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		ready := len(pod.Status.ContainerStatuses) > 0
		for _, cs := range pod.Status.ContainerStatuses {
			if !cs.Ready {
				ready = false
				break
			}
		}
		if ready {
			return true
		}
	}
	return false
}

// applyCSIDriver creates a DatadogCSIDriver CR so the operator's CSI controller
// rolls out the CSI node-server DaemonSet (with the agent-not-ready toleration
// injected by the operator) onto the tainted node.
func (s *untaintSuite) applyCSIDriver(ctx context.Context) {
	s.T().Log("Deploying DatadogCSIDriver to bring up the CSI node-server...")
	dynClient, err := dynamic.NewForConfig(s.Env().KubernetesCluster.KubernetesClient.K8sConfig)
	s.Require().NoError(err, "failed to create dynamic client")

	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "datadoghq.com/v1alpha1",
		"kind":       "DatadogCSIDriver",
		"metadata": map[string]interface{}{
			"name":      csiDriverName,
			"namespace": common.NamespaceName,
		},
		"spec": map[string]interface{}{},
	}}

	_, err = dynClient.Resource(datadogCSIDriverGVR).Namespace(common.NamespaceName).Create(ctx, obj, metav1.CreateOptions{})
	s.Require().NoError(err, "failed to create DatadogCSIDriver")
	s.T().Cleanup(func() {
		_ = dynClient.Resource(datadogCSIDriverGVR).Namespace(common.NamespaceName).Delete(context.Background(), csiDriverName, metav1.DeleteOptions{})
	})
}
