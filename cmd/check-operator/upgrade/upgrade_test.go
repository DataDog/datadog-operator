// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package upgrade

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
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

// testNode returns a node with the taints that the node lifecycle controller adds when it is not Ready.
func testNode(name string, ready corev1.ConditionStatus) *corev1.Node {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Status: corev1.NodeStatus{Conditions: []corev1.NodeCondition{{
			Type:   corev1.NodeReady,
			Status: ready,
		}}},
	}
	taintKey := map[corev1.ConditionStatus]string{
		corev1.ConditionFalse:   corev1.TaintNodeNotReady,
		corev1.ConditionUnknown: corev1.TaintNodeUnreachable,
	}[ready]
	if taintKey != "" {
		node.Spec.Taints = []corev1.Taint{
			{Key: taintKey, Effect: corev1.TaintEffectNoSchedule},
			{Key: taintKey, Effect: corev1.TaintEffectNoExecute},
		}
	}
	return node
}

// testAgentDaemonSet returns an Agent DaemonSet that tolerates every taint, like production Agents,
// with a status that matches its spec.
func testAgentDaemonSet(name, ddaName, generation string) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Generation:  2,
			Namespace:   "test-namespace",
			UID:         types.UID(name + "-uid"),
			Annotations: map[string]string{appsv1.DeprecatedTemplateGeneration: generation},
			Labels: map[string]string{
				apicommon.AgentDeploymentNameLabelKey:      ddaName,
				apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
			},
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
				Tolerations: []corev1.Toleration{{Operator: corev1.TolerationOpExists}},
			}},
		},
		Status: appsv1.DaemonSetStatus{ObservedGeneration: 2},
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
				Build(),
			UserNamespace: "test-namespace",
		},
		datadogAgentName:   "agent",
		agentCompletionPct: 0.95,
		agentCompletionMin: 0,
		skipNotReadyNodes:  true,
		maxNodeLookups:     300,
		maxSkippedPct:      10,
	}
}

// revertedOldest returns a pod on generation 3 with the revision hash of generation 5.
func revertedOldest(name string, ds *appsv1.DaemonSet, nodeName string) *corev1.Pod {
	pod := testAgentPod(name, ds, "3", nodeName, 2*time.Hour)
	pod.Labels[appsv1.DefaultDaemonSetUniqueLabelKey] = "hash-5"
	return pod
}

func getOutdatedNotReadyNodes(t *testing.T, o *Options, minSkipped, maxSkipped int) []string {
	t.Helper()
	daemonSets, err := o.listAgentDaemonSets(context.Background())
	require.NoError(t, err)
	skipped, err := o.getOutdatedNotReadyNodes(context.Background(), daemonSets, minSkipped, maxSkipped)
	require.NoError(t, err)
	return skipped
}

func TestGetOutdatedNotReadyNodes(t *testing.T) {
	defaultDS := testAgentDaemonSet("agent-agent", "agent", "5")
	profileDS := testAgentDaemonSet("profile-agent", "agent", "3")
	// Without the not-ready and unreachable NoSchedule tolerations, the status leaves not-Ready nodes out.
	strictDS := testAgentDaemonSet("strict-agent", "agent", "5")
	strictDS.Spec.Template.Spec.Tolerations = nil
	otherDDADS := testAgentDaemonSet("other-agent", "other", "5")
	otherComponentDS := testAgentDaemonSet("agent-cluster-checks", "agent", "5")
	otherComponentDS.Labels[apicommon.AgentDeploymentComponentLabelKey] = "cluster-checks-runner"
	// The current template no longer targets "node-unlabeled", e.g. a profile label was added to it.
	selectorDS := testAgentDaemonSet("selector-agent", "agent", "2")
	selectorDS.Spec.Template.Spec.NodeSelector = map[string]string{"pool": "agents"}
	labeledNode := testNode("node-labeled", corev1.ConditionUnknown)
	labeledNode.Labels = map[string]string{"pool": "agents"}

	objects := []client.Object{
		labeledNode,
		testNode("node-ready", corev1.ConditionTrue),
		testNode("node-not-ready", corev1.ConditionFalse),
		testNode("node-unknown", corev1.ConditionUnknown),
		&corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: "node-no-condition"}},
		testNode("node-two-outdated", corev1.ConditionUnknown),
		testNode("node-oldest-reverted-a", corev1.ConditionUnknown),
		testNode("node-oldest-reverted-z", corev1.ConditionUnknown),
		testNode("node-terminal", corev1.ConditionUnknown),
		testNode("node-unlabeled", corev1.ConditionUnknown),
		defaultDS, profileDS, strictDS, otherDDADS, otherComponentDS, selectorDS,
		// Pods on the current generation, so that the current revision hashes are known.
		testAgentPod("updated-ready", defaultDS, "5", "node-ready", time.Minute),
		testAgentPod("profile-updated-ready", profileDS, "3", "node-ready", time.Minute),
		testAgentPod("strict-updated-ready", strictDS, "5", "node-ready", time.Minute),
		testAgentPod("other-dda-updated-ready", otherDDADS, "5", "node-ready", time.Minute),
		testAgentPod("other-component-updated-ready", otherComponentDS, "5", "node-ready", time.Minute),
		testAgentPod("selector-updated-ready", selectorDS, "2", "node-ready", time.Minute),

		// Skipped.
		testAgentPod("outdated-not-ready", defaultDS, "4", "node-not-ready", time.Hour),
		deleting(testAgentPod("outdated-stuck-terminating", defaultDS, "4", "node-unknown", time.Hour), corev1.PodRunning),
		testAgentPod("outdated-no-condition", defaultDS, "4", "node-no-condition", time.Hour),
		testAgentPod("outdated-a", defaultDS, "3", "node-two-outdated", 2*time.Hour),
		testAgentPod("outdated-b", defaultDS, "4", "node-two-outdated", time.Hour),
		testAgentPod("profile-outdated-unknown", profileDS, "2", "node-unknown", time.Hour),
		testAgentPod("selector-outdated-labeled", selectorDS, "1", "node-labeled", time.Hour),

		// Not skipped.
		testAgentPod("outdated-ready", defaultDS, "4", "node-ready", time.Hour),
		// The oldest pod has the current revision hash (template revert). The younger out-of-date
		// pod is listed first on one node and last on the other, so that only picking the oldest
		// pod passes.
		revertedOldest("b-oldest-reverted", defaultDS, "node-oldest-reverted-a"),
		testAgentPod("a-younger-outdated", defaultDS, "4", "node-oldest-reverted-a", time.Hour),
		revertedOldest("b-oldest-reverted-z", defaultDS, "node-oldest-reverted-z"),
		testAgentPod("z-younger-outdated", defaultDS, "4", "node-oldest-reverted-z", time.Hour),
		deleting(testAgentPod("outdated-terminal-deleted", defaultDS, "4", "node-terminal", time.Hour), corev1.PodFailed),
		testAgentPod("outdated-deleted-node", defaultDS, "4", "node-deleted", time.Hour),
		testAgentPod("strict-outdated-unknown", strictDS, "4", "node-unknown", time.Hour),
		testAgentPod("other-dda-outdated-unknown", otherDDADS, "4", "node-unknown", time.Hour),
		testAgentPod("other-component-outdated-unknown", otherComponentDS, "4", "node-unknown", time.Hour),
		testAgentPod("selector-outdated-unlabeled", selectorDS, "1", "node-unlabeled", time.Hour),
	}

	o := testOptions(t, objects...)

	skipped := getOutdatedNotReadyNodes(t, o, 1, 100)
	assert.ElementsMatch(t, []string{
		"agent-agent/node-not-ready",
		"agent-agent/node-unknown",
		"agent-agent/node-no-condition",
		"agent-agent/node-two-outdated",
		"profile-agent/node-unknown",
		"selector-agent/node-labeled",
	}, skipped)

	skipped = getOutdatedNotReadyNodes(t, o, 1, 5)
	assert.Empty(t, skipped, "above maxSkipped, no node is skipped")
}

func TestGetOutdatedNotReadyNodesUnknownRevisionHash(t *testing.T) {
	ds := testAgentDaemonSet("agent-agent", "agent", "5")
	// No pod on generation 5 yet, so the current revision hash is unknown. After a template revert,
	// "hash-5" may be the current hash and the DaemonSet status may count this pod as updated.
	revertedPod := testAgentPod("reverted", ds, "4", "node-revert", time.Hour)
	revertedPod.Labels[appsv1.DefaultDaemonSetUniqueLabelKey] = "hash-5"

	o := testOptions(t,
		ds,
		testNode("node-revert", corev1.ConditionUnknown),
		testNode("node-unknown", corev1.ConditionUnknown),
		revertedPod,
		testAgentPod("outdated-unknown", ds, "4", "node-unknown", time.Hour),
	)

	assert.Empty(t, getOutdatedNotReadyNodes(t, o, 1, 100))
}

func TestGetOutdatedNotReadyNodesSharedSelector(t *testing.T) {
	// With update-metadata disabled, the Agent DaemonSets share a selector. The only pod on
	// generation 5 belongs to the other DaemonSet, so the current hash of "agent-agent" is unknown.
	ds := testAgentDaemonSet("agent-agent", "agent", "5")
	otherDS := testAgentDaemonSet("profile-agent", "agent", "5")
	otherDS.Spec.Selector = ds.Spec.Selector

	o := testOptions(t,
		ds, otherDS,
		testNode("node-ready", corev1.ConditionTrue),
		testNode("node-unknown", corev1.ConditionUnknown),
		testAgentPod("other-updated", otherDS, "5", "node-ready", time.Minute),
		testAgentPod("outdated-unknown", ds, "4", "node-unknown", time.Hour),
	)

	assert.Empty(t, getOutdatedNotReadyNodes(t, o, 1, 100))
}

// deadNodes returns an Agent DaemonSet on 40 nodes, 37 of them up to date, with one up-to-date pod,
// and 3 not-Ready nodes with an out-of-date pod.
func deadNodes(dsStatus ...func(*appsv1.DaemonSetStatus)) []client.Object {
	ds := testAgentDaemonSet("agent-agent", "agent", "5")
	ds.Status.CurrentNumberScheduled, ds.Status.UpdatedNumberScheduled = 40, 37
	for _, f := range dsStatus {
		if f != nil {
			f(&ds.Status)
		}
	}
	objects := []client.Object{ds, testAgentPod("updated", ds, "5", "node-ready", time.Minute)}
	for _, name := range []string{"dead-1", "dead-2", "dead-3"} {
		objects = append(objects, testNode(name, corev1.ConditionUnknown), testAgentPod(name, ds, "4", name, time.Hour))
	}
	return objects
}

func TestGetOutdatedNotReadyNodesLimits(t *testing.T) {
	o := testOptions(t, deadNodes()...)
	skipped := getOutdatedNotReadyNodes(t, o, 1, 100)
	assert.Len(t, skipped, 3)

	skipped = getOutdatedNotReadyNodes(t, o, 4, 100)
	assert.Empty(t, skipped, "below minSkipped, no node is skipped")

	o.maxNodeLookups = 2
	skipped = getOutdatedNotReadyNodes(t, o, 1, 100)
	assert.Empty(t, skipped, "above maxNodeLookups, no node is skipped")
}

func TestIsAgentDoneSkippingNotReadyNodesAPIError(t *testing.T) {
	o := testOptions(t, deadNodes()...)
	o.Client = interceptor.NewClient(o.Client.(client.WithWatch), interceptor.Funcs{
		Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
			return errors.NewForbidden(schema.GroupResource{Resource: "nodes"}, key.Name, nil)
		},
	})

	// 37/40 passes if the 3 dead nodes are skipped, but the node lookups fail: plain threshold.
	assert.False(t, o.isAgentDoneSkippingNotReadyNodes(context.Background(), &v2alpha1.DaemonSetStatus{Current: 40, UpToDate: 37}))
	assert.Contains(t, o.Out.(*bytes.Buffer).String(), "unable to count not-Ready nodes, not skipping any")
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

	// 92/100 needs 4 nodes skipped, above a 3% cap: no API call.
	o.maxSkippedPct = 3
	assert.False(t, o.isAgentDoneSkippingNotReadyNodes(context.Background(), &v2alpha1.DaemonSetStatus{Current: 100, UpToDate: 92}))
	assert.Equal(t, 0, lists)

	// Within the 10% cap, the check lists the DaemonSets.
	o.maxSkippedPct = 10
	assert.False(t, o.isAgentDoneSkippingNotReadyNodes(context.Background(), &v2alpha1.DaemonSetStatus{Current: 100, UpToDate: 92}))
	assert.Positive(t, lists)
}

func TestRunSkipsNotReadyNodes(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "test-namespace"},
		Status: v2alpha1.DatadogAgentStatus{
			Agent: &v2alpha1.DaemonSetStatus{Current: 40, UpToDate: 37},
		},
	}

	tests := []struct {
		name     string
		skip     bool
		dsStatus func(*appsv1.DaemonSetStatus)
		wantPass bool
	}{
		{name: "disabled", skip: false, wantPass: false},
		{name: "enabled", skip: true, wantPass: true},
		// A new rollout started and the DatadogAgent status still shows the previous one.
		{name: "DaemonSet status behind its spec", skip: true, dsStatus: func(s *appsv1.DaemonSetStatus) { s.ObservedGeneration = 1 }, wantPass: false},
		{name: "live DaemonSet status behind the DatadogAgent status", skip: true, dsStatus: func(s *appsv1.DaemonSetStatus) { s.UpdatedNumberScheduled = 10 }, wantPass: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := testOptions(t, append(deadNodes(tt.dsStatus), dda, testNode("node-ready", corev1.ConditionTrue))...)
			o.skipNotReadyNodes = tt.skip
			o.checkPeriod = 10 * time.Millisecond
			o.checkTimeout = 100 * time.Millisecond

			err := o.Run()
			if tt.wantPass {
				assert.NoError(t, err, "the 3 not-Ready nodes are skipped")
			} else {
				assert.True(t, wait.Interrupted(err), "the check times out: %v", err)
			}
		})
	}
}

func TestNewOptionsSkipSettings(t *testing.T) {
	tests := []struct {
		env            map[string]string
		wantSkip       bool
		wantLookups    int
		wantMaxSkipPct float64
	}{
		{env: map[string]string{}, wantSkip: false, wantLookups: 300, wantMaxSkipPct: 10},
		{
			env:      map[string]string{"AGENT_SKIP_NOT_READY_NODES": "true", "AGENT_SKIP_NOT_READY_MAX_NODE_LOOKUPS": "500", "AGENT_SKIP_NOT_READY_MAX_PCT": "15"},
			wantSkip: true, wantLookups: 500, wantMaxSkipPct: 15,
		},
		{
			env:      map[string]string{"AGENT_SKIP_NOT_READY_NODES": "yes", "AGENT_SKIP_NOT_READY_MAX_NODE_LOOKUPS": "0", "AGENT_SKIP_NOT_READY_MAX_PCT": "101"},
			wantSkip: false, wantLookups: 300, wantMaxSkipPct: 10,
		},
		{
			env:      map[string]string{"AGENT_SKIP_NOT_READY_MAX_NODE_LOOKUPS": "-1", "AGENT_SKIP_NOT_READY_MAX_PCT": "-1"},
			wantSkip: false, wantLookups: 300, wantMaxSkipPct: 10,
		},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.env), func(t *testing.T) {
			for key, val := range tt.env {
				t.Setenv(key, val)
			}
			o := NewOptions(genericclioptions.IOStreams{})
			assert.Equal(t, tt.wantSkip, o.skipNotReadyNodes)
			assert.Equal(t, tt.wantLookups, o.maxNodeLookups)
			assert.InDelta(t, tt.wantMaxSkipPct, o.maxSkippedPct, 1e-9)
		})
	}
}
