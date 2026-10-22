// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package upgrade

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	controllercommon "github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/constants"
	plugincommon "github.com/DataDog/datadog-operator/pkg/plugin/common"
)

func TestGetDDAIs(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))

	matchingDDAI := &v1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{
		Name:      "agent",
		Namespace: "test-namespace",
		Labels:    map[string]string{apicommon.DatadogAgentNameLabelKey: "agent"},
	}}
	matchingProfileDDAI := &v1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{
		Name:      "agent-profile-a",
		Namespace: "test-namespace",
		Labels:    map[string]string{apicommon.DatadogAgentNameLabelKey: "agent"},
	}}
	otherDDAI := &v1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{
		Name:      "other-agent",
		Namespace: "test-namespace",
		Labels:    map[string]string{apicommon.DatadogAgentNameLabelKey: "other"},
	}}
	otherNamespaceDDAI := &v1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{
		Name:      "agent",
		Namespace: "other-namespace",
		Labels:    map[string]string{apicommon.DatadogAgentNameLabelKey: "agent"},
	}}

	o := &Options{
		Options: plugincommon.Options{
			Client: fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(matchingDDAI, matchingProfileDDAI, otherDDAI, otherNamespaceDDAI).
				Build(),
			UserNamespace: "test-namespace",
		},
		datadogAgentName: "agent",
	}

	ddais, err := o.getDDAIs(context.Background())
	require.NoError(t, err)
	require.Len(t, ddais, 2)
	assert.ElementsMatch(t, []string{matchingDDAI.Name, matchingProfileDDAI.Name}, []string{ddais[0].Name, ddais[1].Name})
}

func TestIsReconcileError(t *testing.T) {
	tests := []struct {
		name       string
		conditions []metav1.Condition
		wantError  string
	}{
		{
			name: "DDAI error propagated to DDA",
			conditions: []metav1.Condition{{
				Type:    controllercommon.DatadogAgentInternalReconcileErrorConditionType,
				Status:  metav1.ConditionTrue,
				Message: "agent: forbidden",
			}},
			wantError: "datadogAgentInternal reconciliation error message: agent: forbidden",
		},
		{
			name: "DDAI direct reconcile error",
			conditions: []metav1.Condition{{
				Type:    controllercommon.DatadogAgentReconcileErrorConditionType,
				Status:  metav1.ConditionTrue,
				Message: "forbidden",
			}},
			wantError: "datadogAgent reconciliation error message: forbidden",
		},
		{
			name: "resolved DDAI reconcile error",
			conditions: []metav1.Condition{{
				Type:    controllercommon.DatadogAgentReconcileErrorConditionType,
				Status:  metav1.ConditionFalse,
				Message: "reconcile ok",
			}},
		},
		{
			name: "agent component error",
			conditions: []metav1.Condition{{
				Type:    controllercommon.AgentReconcileConditionType,
				Status:  metav1.ConditionFalse,
				Message: "daemonset failed",
			}},
			wantError: "agent reconciliation error message: daemonset failed",
		},
		{
			name: "cluster agent component error",
			conditions: []metav1.Condition{{
				Type:    controllercommon.ClusterAgentReconcileConditionType,
				Status:  metav1.ConditionFalse,
				Message: "deployment failed",
			}},
			wantError: "cluster Agent reconciliation error message: deployment failed",
		},
		{
			name: "cluster checks runner component error",
			conditions: []metav1.Condition{{
				Type:    controllercommon.ClusterChecksRunnerReconcileConditionType,
				Status:  metav1.ConditionFalse,
				Message: "deployment failed",
			}},
			wantError: "cluster Check Runner reconciliation error message: deployment failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := isReconcileError(tt.conditions)
			if tt.wantError == "" {
				assert.NoError(t, err)
				return
			}
			require.EqualError(t, err, tt.wantError)
		})
	}
}

func testNode(name string, ready corev1.ConditionStatus) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{
			Type:   corev1.NodeReady,
			Status: ready,
		}}},
	}
}

func testAgentDaemonSet(name, ddaName, generation string) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   "test-namespace",
			UID:         types.UID(name + "-uid"),
			Annotations: map[string]string{daemonSetTemplateGenerationAnnotation: generation},
			Labels: map[string]string{
				apicommon.AgentDeploymentNameLabelKey:      ddaName,
				apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
			},
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
		},
	}
}

// testAgentPod returns a pod of the DaemonSet. The revision hash is "hash-<generation>".
func testAgentPod(name string, ds *appsv1.DaemonSet, generation, nodeName string, age time.Duration) *corev1.Pod {
	isController := true
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         ds.Namespace,
			CreationTimestamp: metav1.NewTime(time.Now().Add(-age)),
			Labels: map[string]string{
				"app":                                 ds.Spec.Selector.MatchLabels["app"],
				podTemplateGenerationLabel:            generation,
				appsv1.DefaultDaemonSetUniqueLabelKey: "hash-" + generation,
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1",
				Kind:       "DaemonSet",
				Name:       ds.Name,
				UID:        ds.UID,
				Controller: &isController,
			}},
		},
		Spec:   corev1.PodSpec{NodeName: nodeName},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}
}

func deleting(pod *corev1.Pod, phase corev1.PodPhase) *corev1.Pod {
	now := metav1.Now()
	pod.DeletionTimestamp = &now
	pod.Finalizers = []string{"test"}
	pod.Status.Phase = phase
	return pod
}

func testOptions(t *testing.T, objects ...client.Object) *Options {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, v2alpha1.AddToScheme(scheme))
	return &Options{
		IOStreams: genericclioptions.IOStreams{Out: &bytes.Buffer{}},
		Options: plugincommon.Options{
			Client: fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				WithIndex(&corev1.Pod{}, "spec.nodeName", func(obj client.Object) []string {
					return []string{obj.(*corev1.Pod).Spec.NodeName}
				}).
				Build(),
			UserNamespace: "test-namespace",
		},
		datadogAgentName:   "agent",
		agentCompletionPct: 0.95,
		agentCompletionMin: 0,
		skipNotReadyNodes:  true,
	}
}

func TestCountOutdatedNotReadyNodes(t *testing.T) {
	defaultDS := testAgentDaemonSet("agent-agent", "agent", "5")
	profileDS := testAgentDaemonSet("profile-agent", "agent", "3")
	profileDS.Spec.Template.Spec.Tolerations = []corev1.Toleration{{Operator: corev1.TolerationOpExists}}
	otherDDADS := testAgentDaemonSet("other-agent", "other", "5")

	// Generation changed, but the revision hash is the current one (template revert).
	revertedPod := testAgentPod("reverted", defaultDS, "4", "node-revert", time.Hour)
	revertedPod.Labels[appsv1.DefaultDaemonSetUniqueLabelKey] = "hash-5"

	// A not-Ready node with the NoSchedule taint that the node lifecycle controller adds. The
	// DaemonSet default tolerations do not cover it.
	taintedNode := testNode("node-tainted", corev1.ConditionUnknown)
	taintedNode.Spec.Taints = []corev1.Taint{{Key: corev1.TaintNodeUnreachable, Effect: corev1.TaintEffectNoSchedule}}

	objects := []client.Object{
		taintedNode,
		testNode("node-ready", corev1.ConditionTrue),
		testNode("node-not-ready", corev1.ConditionFalse),
		testNode("node-unknown", corev1.ConditionUnknown),
		testNode("node-two-outdated", corev1.ConditionUnknown),
		testNode("node-oldest-updated", corev1.ConditionUnknown),
		testNode("node-terminal", corev1.ConditionUnknown),
		testNode("node-revert", corev1.ConditionUnknown),
		defaultDS, profileDS, otherDDADS,
		// A pod on the current generation, so that "hash-5" is the current revision hash.
		testAgentPod("updated-ready", defaultDS, "5", "node-ready", time.Minute),

		// Counted: 1 node each.
		testAgentPod("outdated-not-ready", defaultDS, "4", "node-not-ready", time.Hour),
		deleting(testAgentPod("outdated-stuck-terminating", defaultDS, "4", "node-unknown", time.Hour), corev1.PodRunning),
		testAgentPod("outdated-a", defaultDS, "3", "node-two-outdated", 2*time.Hour),
		testAgentPod("outdated-b", defaultDS, "4", "node-two-outdated", time.Hour),
		testAgentPod("profile-outdated-unknown", profileDS, "2", "node-unknown", time.Hour),
		// The profile tolerates every taint, so the DaemonSet status counts this node.
		testAgentPod("profile-outdated-tainted", profileDS, "2", "node-tainted", time.Hour),

		// Not counted.
		testAgentPod("outdated-ready", defaultDS, "4", "node-ready", time.Hour),
		testAgentPod("oldest-updated", defaultDS, "5", "node-oldest-updated", 2*time.Hour),
		// Named to be listed first, so the test fails if the oldest pod is not picked.
		testAgentPod("a-younger-outdated", defaultDS, "4", "node-oldest-updated", time.Hour),
		deleting(testAgentPod("outdated-terminal-deleted", defaultDS, "4", "node-terminal", time.Hour), corev1.PodFailed),
		revertedPod,
		testAgentPod("outdated-deleted-node", defaultDS, "4", "node-deleted", time.Hour),
		testAgentPod("other-dda-outdated-unknown", otherDDADS, "4", "node-unknown", time.Hour),
		// The default DaemonSet does not tolerate the taint, so its status does not count this node.
		testAgentPod("outdated-tainted", defaultDS, "4", "node-tainted", time.Hour),
	}

	o := testOptions(t, objects...)

	skipped, err := o.countOutdatedNotReadyNodes(context.Background(), 100)
	require.NoError(t, err)
	assert.Equal(t, int32(5), skipped)

	skipped, err = o.countOutdatedNotReadyNodes(context.Background(), 4)
	require.NoError(t, err)
	assert.Equal(t, int32(0), skipped, "above maxSkipped, no node is skipped")
}

func TestIsAgentDoneSkippingNotReadyNodes(t *testing.T) {
	ds := testAgentDaemonSet("agent-agent", "agent", "5")
	objects := []client.Object{ds, testAgentPod("updated", ds, "5", "node-ready", time.Minute), testNode("node-ready", corev1.ConditionTrue)}
	for _, name := range []string{"dead-1", "dead-2", "dead-3"} {
		objects = append(objects, testNode(name, corev1.ConditionUnknown), testAgentPod(name, ds, "4", name, time.Hour))
	}

	// 3 of 40 nodes are out of date and not Ready. Without them, 37/37 nodes are up to date.
	status := &v2alpha1.DaemonSetStatus{Current: 40, UpToDate: 37}

	tests := []struct {
		name              string
		skipNotReadyNodes bool
		status            *v2alpha1.DaemonSetStatus
		want              bool
	}{
		{name: "disabled", skipNotReadyNodes: false, status: status, want: false},
		{name: "enabled", skipNotReadyNodes: true, status: status, want: true},
		{name: "above the 10% skip cap", skipNotReadyNodes: true, status: &v2alpha1.DaemonSetStatus{Current: 20, UpToDate: 17}, want: false},
		{name: "plain check passes", skipNotReadyNodes: false, status: &v2alpha1.DaemonSetStatus{Current: 40, UpToDate: 40}, want: true},
		{name: "no status", skipNotReadyNodes: true, status: nil, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := testOptions(t, objects...)
			o.skipNotReadyNodes = tt.skipNotReadyNodes
			assert.Equal(t, tt.want, o.isAgentDoneSkippingNotReadyNodes(context.Background(), tt.status))
		})
	}
}

func TestIsAgentDoneSkippingNotReadyNodesPrecheck(t *testing.T) {
	var lists int
	o := testOptions(t)
	o.Client = interceptor.NewClient(o.Client.(client.WithWatch), interceptor.Funcs{
		List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
			lists++
			return c.List(ctx, list, opts...)
		},
	})

	// Even with 10% of the nodes skipped, 50/90 is below the threshold: no API call.
	assert.False(t, o.isAgentDoneSkippingNotReadyNodes(context.Background(), &v2alpha1.DaemonSetStatus{Current: 100, UpToDate: 50}))
	assert.Equal(t, 0, lists)

	// 92/100 can pass if 4 nodes are skipped: the check lists the DaemonSets.
	assert.False(t, o.isAgentDoneSkippingNotReadyNodes(context.Background(), &v2alpha1.DaemonSetStatus{Current: 100, UpToDate: 92}))
	assert.Positive(t, lists)
}

func TestRunSkipsNotReadyNodes(t *testing.T) {
	ds := testAgentDaemonSet("agent-agent", "agent", "5")
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "test-namespace"},
		Status: v2alpha1.DatadogAgentStatus{
			Agent: &v2alpha1.DaemonSetStatus{Current: 40, UpToDate: 37},
		},
	}
	objects := []client.Object{dda, ds, testAgentPod("updated", ds, "5", "node-ready", time.Minute), testNode("node-ready", corev1.ConditionTrue)}
	for _, name := range []string{"dead-1", "dead-2", "dead-3"} {
		objects = append(objects, testNode(name, corev1.ConditionUnknown), testAgentPod(name, ds, "4", name, time.Hour))
	}

	for _, skip := range []bool{false, true} {
		o := testOptions(t, objects...)
		o.skipNotReadyNodes = skip
		o.checkPeriod = 10 * time.Millisecond
		o.checkTimeout = 100 * time.Millisecond

		err := o.Run()
		if skip {
			assert.NoError(t, err, "the 3 not-Ready nodes are skipped")
		} else {
			assert.Error(t, err, "the check times out on the 3 not-Ready nodes")
		}
	}
}

func TestNewOptionsSkipNotReadyNodes(t *testing.T) {
	t.Setenv("AGENT_SKIP_NOT_READY_NODES", "false")
	assert.False(t, NewOptions(genericclioptions.IOStreams{}).skipNotReadyNodes)

	t.Setenv("AGENT_SKIP_NOT_READY_NODES", "true")
	assert.True(t, NewOptions(genericclioptions.IOStreams{}).skipNotReadyNodes)
}
