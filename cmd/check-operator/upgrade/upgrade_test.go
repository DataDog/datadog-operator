// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package upgrade

import (
	"bytes"
	"context"
	"strings"
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
		maxNodeLookups:     300,
		maxSkippedPct:      10,
	}
}

func TestGetOutdatedNotReadyNodes(t *testing.T) {
	defaultDS := testAgentDaemonSet("agent-agent", "agent", "5")
	profileDS := testAgentDaemonSet("profile-agent", "agent", "3")
	profileDS.Spec.Template.Spec.Tolerations = []corev1.Toleration{{Operator: corev1.TolerationOpExists}}
	otherDDADS := testAgentDaemonSet("other-agent", "other", "5")
	otherComponentDS := testAgentDaemonSet("agent-cluster-checks", "agent", "5")
	otherComponentDS.Labels[apicommon.AgentDeploymentComponentLabelKey] = "cluster-checks-runner"
	// The current template no longer targets "node-unlabeled", e.g. a profile label was added to it.
	selectorDS := testAgentDaemonSet("selector-agent", "agent", "2")
	selectorDS.Spec.Template.Spec.NodeSelector = map[string]string{"pool": "agents"}

	// Generation changed, but the revision hash is the current one (template revert).
	revertedPod := testAgentPod("reverted", defaultDS, "4", "node-revert", time.Hour)
	revertedPod.Labels[appsv1.DefaultDaemonSetUniqueLabelKey] = "hash-5"

	// A not-Ready node with the NoSchedule taint that the node lifecycle controller adds. The
	// DaemonSet default tolerations do not cover it.
	taintedNode := testNode("node-tainted", corev1.ConditionUnknown)
	taintedNode.Spec.Taints = []corev1.Taint{{Key: corev1.TaintNodeUnreachable, Effect: corev1.TaintEffectNoSchedule}}
	labeledNode := testNode("node-labeled", corev1.ConditionUnknown)
	labeledNode.Labels = map[string]string{"pool": "agents"}

	objects := []client.Object{
		taintedNode,
		labeledNode,
		testNode("node-ready", corev1.ConditionTrue),
		testNode("node-not-ready", corev1.ConditionFalse),
		testNode("node-unknown", corev1.ConditionUnknown),
		testNode("node-two-outdated", corev1.ConditionUnknown),
		testNode("node-oldest-updated-a", corev1.ConditionUnknown),
		testNode("node-oldest-updated-z", corev1.ConditionUnknown),
		testNode("node-terminal", corev1.ConditionUnknown),
		testNode("node-revert", corev1.ConditionUnknown),
		testNode("node-unlabeled", corev1.ConditionUnknown),
		defaultDS, profileDS, otherDDADS, otherComponentDS, selectorDS,
		// Pods on the current generation, so that the current revision hashes are known.
		testAgentPod("updated-ready", defaultDS, "5", "node-ready", time.Minute),
		testAgentPod("profile-updated-ready", profileDS, "3", "node-ready", time.Minute),
		testAgentPod("other-dda-updated-ready", otherDDADS, "5", "node-ready", time.Minute),
		testAgentPod("other-component-updated-ready", otherComponentDS, "5", "node-ready", time.Minute),
		testAgentPod("selector-updated-ready", selectorDS, "2", "node-ready", time.Minute),

		// Skipped.
		testAgentPod("outdated-not-ready", defaultDS, "4", "node-not-ready", time.Hour),
		deleting(testAgentPod("outdated-stuck-terminating", defaultDS, "4", "node-unknown", time.Hour), corev1.PodRunning),
		testAgentPod("outdated-a", defaultDS, "3", "node-two-outdated", 2*time.Hour),
		testAgentPod("outdated-b", defaultDS, "4", "node-two-outdated", time.Hour),
		testAgentPod("profile-outdated-unknown", profileDS, "2", "node-unknown", time.Hour),
		// The profile tolerates every taint, so the DaemonSet status counts this node.
		testAgentPod("profile-outdated-tainted", profileDS, "2", "node-tainted", time.Hour),
		testAgentPod("selector-outdated-labeled", selectorDS, "1", "node-labeled", time.Hour),

		// Not skipped.
		testAgentPod("outdated-ready", defaultDS, "4", "node-ready", time.Hour),
		// The oldest pod is up to date. The younger out-of-date pod is listed first on one node
		// and last on the other, so that only picking the oldest pod passes.
		testAgentPod("b-oldest-updated", defaultDS, "5", "node-oldest-updated-a", 2*time.Hour),
		testAgentPod("a-younger-outdated", defaultDS, "4", "node-oldest-updated-a", time.Hour),
		testAgentPod("b-oldest-updated-z", defaultDS, "5", "node-oldest-updated-z", 2*time.Hour),
		testAgentPod("z-younger-outdated", defaultDS, "4", "node-oldest-updated-z", time.Hour),
		deleting(testAgentPod("outdated-terminal-deleted", defaultDS, "4", "node-terminal", time.Hour), corev1.PodFailed),
		revertedPod,
		testAgentPod("outdated-deleted-node", defaultDS, "4", "node-deleted", time.Hour),
		testAgentPod("other-dda-outdated-unknown", otherDDADS, "4", "node-unknown", time.Hour),
		testAgentPod("other-component-outdated-unknown", otherComponentDS, "4", "node-unknown", time.Hour),
		// The default DaemonSet does not tolerate the taint, so its status does not count this node.
		testAgentPod("outdated-tainted", defaultDS, "4", "node-tainted", time.Hour),
		testAgentPod("selector-outdated-unlabeled", selectorDS, "1", "node-unlabeled", time.Hour),
	}

	o := testOptions(t, objects...)

	skipped, err := o.getOutdatedNotReadyNodes(context.Background(), 1, 100)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		"agent-agent/node-not-ready",
		"agent-agent/node-unknown",
		"agent-agent/node-two-outdated",
		"profile-agent/node-unknown",
		"profile-agent/node-tainted",
		"selector-agent/node-labeled",
	}, skipped)

	skipped, err = o.getOutdatedNotReadyNodes(context.Background(), 1, 5)
	require.NoError(t, err)
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

	skipped, err := o.getOutdatedNotReadyNodes(context.Background(), 1, 100)
	require.NoError(t, err)
	assert.Empty(t, skipped)
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

	skipped, err := o.getOutdatedNotReadyNodes(context.Background(), 1, 100)
	require.NoError(t, err)
	assert.Empty(t, skipped)
}

func TestGetOutdatedNotReadyNodesLookupCap(t *testing.T) {
	ds := testAgentDaemonSet("agent-agent", "agent", "5")
	objects := []client.Object{ds, testAgentPod("updated", ds, "5", "node-ready", time.Minute)}
	for _, name := range []string{"dead-1", "dead-2", "dead-3"} {
		objects = append(objects, testNode(name, corev1.ConditionUnknown), testAgentPod(name, ds, "4", name, time.Hour))
	}

	o := testOptions(t, objects...)
	skipped, err := o.getOutdatedNotReadyNodes(context.Background(), 1, 100)
	require.NoError(t, err)
	assert.Len(t, skipped, 3)

	skipped, err = o.getOutdatedNotReadyNodes(context.Background(), 4, 100)
	require.NoError(t, err)
	assert.Empty(t, skipped, "below minSkipped, no node is skipped")

	o.maxNodeLookups = 2
	skipped, err = o.getOutdatedNotReadyNodes(context.Background(), 1, 100)
	require.NoError(t, err)
	assert.Empty(t, skipped, "above maxNodeLookups, no node is skipped")
}

func TestGetOutdatedNotReadyNodesAPIErrors(t *testing.T) {
	ds := testAgentDaemonSet("agent-agent", "agent", "5")
	objects := []client.Object{
		ds,
		testAgentPod("updated", ds, "5", "node-ready", time.Minute),
		testNode("node-unknown", corev1.ConditionUnknown),
		testAgentPod("outdated", ds, "4", "node-unknown", time.Hour),
	}
	errAPI := errors.NewForbidden(schema.GroupResource{}, "", nil)

	tests := []struct {
		name      string
		funcs     interceptor.Funcs
		wantError string
	}{
		{
			name: "list DaemonSets",
			funcs: interceptor.Funcs{List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				if _, ok := list.(*appsv1.DaemonSetList); ok {
					return errAPI
				}
				return c.List(ctx, list, opts...)
			}},
			wantError: "unable to list Agent DaemonSets",
		},
		{
			name:      "list out-of-date pods",
			funcs:     failPodList(func(lo *client.ListOptions) bool { return strings.Contains(lo.LabelSelector.String(), "!=") }, errAPI),
			wantError: "unable to list out-of-date pods of DaemonSet agent-agent",
		},
		{
			name:      "list current pods",
			funcs:     failPodList(func(lo *client.ListOptions) bool { return lo.Limit == 1 }, errAPI),
			wantError: "unable to list current pods of DaemonSet agent-agent",
		},
		{
			name:      "list pods on node",
			funcs:     failPodList(func(lo *client.ListOptions) bool { return lo.FieldSelector != nil }, errAPI),
			wantError: "unable to list pods of DaemonSet agent-agent on node node-unknown",
		},
		{
			name: "get node",
			funcs: interceptor.Funcs{Get: func(ctx context.Context, c client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				return errAPI
			}},
			wantError: "unable to get node node-unknown",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := testOptions(t, objects...)
			o.Client = interceptor.NewClient(o.Client.(client.WithWatch), tt.funcs)

			_, err := o.getOutdatedNotReadyNodes(context.Background(), 1, 100)
			require.ErrorContains(t, err, tt.wantError)

			// The check falls back to the plain threshold: 19/20 is not above 95%.
			assert.False(t, o.isAgentDoneSkippingNotReadyNodes(context.Background(), &v2alpha1.DaemonSetStatus{Current: 20, UpToDate: 19}))
			assert.Contains(t, o.Out.(*bytes.Buffer).String(), "unable to count not-Ready nodes, not skipping any")
		})
	}
}

// failPodList fails the pod Lists whose options match.
func failPodList(match func(*client.ListOptions) bool, err error) interceptor.Funcs {
	return interceptor.Funcs{List: func(ctx context.Context, c client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
		if _, ok := list.(*corev1.PodList); ok && match((&client.ListOptions{}).ApplyOptions(opts)) {
			return err
		}
		return c.List(ctx, list, opts...)
	}}
}

func TestIsCountedDaemonPod(t *testing.T) {
	ds := testAgentDaemonSet("agent-agent", "agent", "5")
	assert.True(t, isCountedDaemonPod(testAgentPod("bound", ds, "5", "node", time.Minute), ds))
	assert.False(t, isCountedDaemonPod(testAgentPod("unbound", ds, "5", "", time.Minute), ds))
	assert.False(t, isCountedDaemonPod(testAgentPod("other-owner", testAgentDaemonSet("other", "agent", "5"), "5", "node", time.Minute), ds))
	assert.True(t, isCountedDaemonPod(deleting(testAgentPod("terminating", ds, "5", "node", time.Minute), corev1.PodRunning), ds))
	assert.False(t, isCountedDaemonPod(deleting(testAgentPod("terminal", ds, "5", "node", time.Minute), corev1.PodSucceeded), ds))
}

func TestIsNodeReady(t *testing.T) {
	assert.True(t, isNodeReady(testNode("ready", corev1.ConditionTrue)))
	assert.False(t, isNodeReady(testNode("not-ready", corev1.ConditionFalse)))
	assert.False(t, isNodeReady(testNode("unknown", corev1.ConditionUnknown)))
	assert.False(t, isNodeReady(&corev1.Node{}), "no Ready condition")
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
		maxSkippedPct     float64
		status            *v2alpha1.DaemonSetStatus
		want              bool
	}{
		{name: "disabled", skipNotReadyNodes: false, status: status, want: false},
		{name: "enabled", skipNotReadyNodes: true, status: status, want: true},
		{name: "above the 10% skip cap", skipNotReadyNodes: true, status: &v2alpha1.DaemonSetStatus{Current: 20, UpToDate: 17}, want: false},
		{name: "within a raised 15% skip cap", skipNotReadyNodes: true, maxSkippedPct: 15, status: &v2alpha1.DaemonSetStatus{Current: 20, UpToDate: 17}, want: true},
		{name: "plain check passes", skipNotReadyNodes: false, status: &v2alpha1.DaemonSetStatus{Current: 40, UpToDate: 40}, want: true},
		{name: "no status", skipNotReadyNodes: true, status: nil, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := testOptions(t, objects...)
			o.skipNotReadyNodes = tt.skipNotReadyNodes
			if tt.maxSkippedPct != 0 {
				o.maxSkippedPct = tt.maxSkippedPct
			}
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

	t.Setenv("AGENT_SKIP_NOT_READY_NODES", "not-a-bool")
	assert.False(t, NewOptions(genericclioptions.IOStreams{}).skipNotReadyNodes)
}

func TestNewOptionsMaxSkippedPct(t *testing.T) {
	assert.InDelta(t, 10, NewOptions(genericclioptions.IOStreams{}).maxSkippedPct, 1e-9)

	t.Setenv("AGENT_SKIP_NOT_READY_MAX_PCT", "15")
	assert.InDelta(t, 15, NewOptions(genericclioptions.IOStreams{}).maxSkippedPct, 1e-9)

	for _, invalid := range []string{"not-a-number", "-1", "101"} {
		t.Setenv("AGENT_SKIP_NOT_READY_MAX_PCT", invalid)
		assert.InDelta(t, 10, NewOptions(genericclioptions.IOStreams{}).maxSkippedPct, 1e-9, invalid)
	}
}

func TestNewOptionsMaxNodeLookups(t *testing.T) {
	assert.Equal(t, 300, NewOptions(genericclioptions.IOStreams{}).maxNodeLookups)

	t.Setenv("AGENT_SKIP_NOT_READY_MAX_NODE_LOOKUPS", "500")
	assert.Equal(t, 500, NewOptions(genericclioptions.IOStreams{}).maxNodeLookups)

	for _, invalid := range []string{"not-a-number", "0", "-1"} {
		t.Setenv("AGENT_SKIP_NOT_READY_MAX_NODE_LOOKUPS", invalid)
		assert.Equal(t, 300, NewOptions(genericclioptions.IOStreams{}).maxNodeLookups, invalid)
	}
}
