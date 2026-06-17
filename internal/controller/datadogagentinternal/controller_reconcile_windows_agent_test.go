// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
)

func newWindowsTestReconciler(objs ...runtime.Object) *Reconciler {
	sch := runtime.NewScheme()
	_ = scheme.AddToScheme(sch)
	_ = datadoghqv1alpha1.AddToScheme(sch)
	clientObjs := make([]client.Object, 0, len(objs))
	for _, o := range objs {
		if co, ok := o.(client.Object); ok {
			clientObjs = append(clientObjs, co)
		}
	}
	fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(clientObjs...).Build()
	recorder := record.NewBroadcaster().NewRecorder(sch, corev1.EventSource{Component: "windows-test"})
	return &Reconciler{
		client:   fakeClient,
		scheme:   sch,
		recorder: recorder,
	}
}

func windowsTestDDAI(override map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride, global *datadoghqv2alpha1.GlobalConfig) *datadoghqv1alpha1.DatadogAgentInternal {
	return &datadoghqv1alpha1.DatadogAgentInternal{
		ObjectMeta: metav1.ObjectMeta{Name: "dd", Namespace: "datadog"},
		Spec: datadoghqv2alpha1.DatadogAgentSpec{
			Global:   global,
			Override: override,
		},
	}
}

// No windowsNodeAgent key → no-op, no DaemonSet created.
func TestReconcileV2WindowsAgent_NoOpWhenKeyAbsent(t *testing.T) {
	r := newWindowsTestReconciler()
	ddai := windowsTestDDAI(map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{}, nil)
	newStatus := &datadoghqv1alpha1.DatadogAgentInternalStatus{}

	_, err := r.reconcileV2WindowsAgent(context.Background(), feature.RequiredComponents{}, nil, ddai, nil, newStatus)
	require.NoError(t, err)

	// No Windows DaemonSet created.
	dsList := &appsv1.DaemonSetList{}
	require.NoError(t, r.client.List(context.Background(), dsList))
	assert.Empty(t, dsList.Items)
	assert.Nil(t, newStatus.AgentWindows)
}

// FIPS + Windows → no DaemonSet, condition surfaced, Linux Agent status untouched.
func TestReconcileV2WindowsAgent_FIPSGuard(t *testing.T) {
	r := newWindowsTestReconciler()
	ddai := windowsTestDDAI(
		map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{
			datadoghqv2alpha1.WindowsNodeAgentComponentName: {},
		},
		&datadoghqv2alpha1.GlobalConfig{UseFIPSAgent: ptr.To(true)},
	)
	newStatus := &datadoghqv1alpha1.DatadogAgentInternalStatus{}

	_, err := r.reconcileV2WindowsAgent(context.Background(), feature.RequiredComponents{}, nil, ddai, nil, newStatus)
	require.NoError(t, err)

	// No Windows DaemonSet created under FIPS.
	dsList := &appsv1.DaemonSetList{}
	require.NoError(t, r.client.List(context.Background(), dsList))
	assert.Empty(t, dsList.Items)

	// A WindowsAgentReconcile condition with the FIPS reason is set to False.
	found := false
	for _, c := range newStatus.Conditions {
		if c.Type == "WindowsAgentReconcile" {
			found = true
			assert.Equal(t, metav1.ConditionFalse, c.Status)
			assert.Equal(t, "FIPSWindowsUnsupported", c.Reason)
		}
	}
	assert.True(t, found, "expected WindowsAgentReconcile condition")
}

// Disabled flag → delete the Windows DS and clear ONLY AgentWindows, never the Linux Agent status.
func TestReconcileV2WindowsAgent_DisabledClearsOnlyWindowsStatus(t *testing.T) {
	existingWinDS := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "dd-agent-windows", Namespace: "datadog", Labels: map[string]string{"agent.datadoghq.com/component": "agent-windows", "app.kubernetes.io/part-of": "datadog-dd"}},
	}
	r := newWindowsTestReconciler(existingWinDS)

	ddai := windowsTestDDAI(
		map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{
			datadoghqv2alpha1.WindowsNodeAgentComponentName: {Disabled: ptr.To(true)},
		},
		nil,
	)
	// Pre-populate both Linux and Windows status to verify only Windows is cleared.
	linuxStatus := &datadoghqv2alpha1.DaemonSetStatus{DaemonsetName: "dd-agent", Desired: 3, Ready: 3}
	newStatus := &datadoghqv1alpha1.DatadogAgentInternalStatus{
		Agent:        linuxStatus,
		AgentWindows: &datadoghqv2alpha1.DaemonSetStatus{DaemonsetName: "dd-agent-windows", Desired: 1, Ready: 1},
	}

	_, err := r.reconcileV2WindowsAgent(context.Background(), feature.RequiredComponents{}, nil, ddai, nil, newStatus)
	require.NoError(t, err)

	// Windows DS deleted.
	ds := &appsv1.DaemonSet{}
	err = r.client.Get(context.Background(), types.NamespacedName{Name: "dd-agent-windows", Namespace: "datadog"}, ds)
	assert.True(t, err != nil, "Windows DaemonSet should have been deleted")

	// AgentWindows cleared, Linux Agent status untouched.
	assert.Nil(t, newStatus.AgentWindows, "AgentWindows must be cleared")
	require.NotNil(t, newStatus.Agent, "Linux Agent status must NOT be cleared")
	assert.Equal(t, "dd-agent", newStatus.Agent.DaemonsetName)
	assert.Equal(t, int32(3), newStatus.Agent.Desired)
}

// GetWindowsAgentName naming sanity (cross-check with reconciler delete target).
func TestReconcileV2WindowsAgent_DeleteTargetsCorrectName(t *testing.T) {
	ddai := windowsTestDDAI(nil, nil)
	assert.Equal(t, "dd-agent-windows", componentagent.GetWindowsAgentName(ddai))
}

// Removing the windowsNodeAgent override (key absent) must delete a previously-created
// Windows DaemonSet and clear AgentWindows, without touching the Linux Agent status.
func TestReconcileV2WindowsAgent_KeyAbsentCleansUpStaleDS(t *testing.T) {
	existingWinDS := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "dd-agent-windows", Namespace: "datadog", Labels: map[string]string{"agent.datadoghq.com/component": "agent-windows", "app.kubernetes.io/part-of": "datadog-dd"}},
	}
	r := newWindowsTestReconciler(existingWinDS)
	ddai := windowsTestDDAI(map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{}, nil)
	newStatus := &datadoghqv1alpha1.DatadogAgentInternalStatus{
		Agent:        &datadoghqv2alpha1.DaemonSetStatus{DaemonsetName: "dd-agent", Desired: 2, Ready: 2},
		AgentWindows: &datadoghqv2alpha1.DaemonSetStatus{DaemonsetName: "dd-agent-windows"},
	}

	_, err := r.reconcileV2WindowsAgent(context.Background(), feature.RequiredComponents{}, nil, ddai, nil, newStatus)
	require.NoError(t, err)

	ds := &appsv1.DaemonSet{}
	err = r.client.Get(context.Background(), types.NamespacedName{Name: "dd-agent-windows", Namespace: "datadog"}, ds)
	assert.True(t, err != nil, "stale Windows DaemonSet should be deleted when key is removed")
	assert.Nil(t, newStatus.AgentWindows, "AgentWindows must be cleared")
	require.NotNil(t, newStatus.Agent, "Linux Agent status must survive")
	assert.Equal(t, "dd-agent", newStatus.Agent.DaemonsetName)
}

// Enabling FIPS while windowsNodeAgent is set must delete the Windows DS (not leave it
// running misconfigured) and surface the FIPS condition.
func TestReconcileV2WindowsAgent_FIPSCleansUpStaleDS(t *testing.T) {
	existingWinDS := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "dd-agent-windows", Namespace: "datadog", Labels: map[string]string{"agent.datadoghq.com/component": "agent-windows", "app.kubernetes.io/part-of": "datadog-dd"}},
	}
	r := newWindowsTestReconciler(existingWinDS)
	ddai := windowsTestDDAI(
		map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{
			datadoghqv2alpha1.WindowsNodeAgentComponentName: {},
		},
		&datadoghqv2alpha1.GlobalConfig{UseFIPSAgent: ptr.To(true)},
	)
	newStatus := &datadoghqv1alpha1.DatadogAgentInternalStatus{}

	_, err := r.reconcileV2WindowsAgent(context.Background(), feature.RequiredComponents{}, nil, ddai, nil, newStatus)
	require.NoError(t, err)

	ds := &appsv1.DaemonSet{}
	err = r.client.Get(context.Background(), types.NamespacedName{Name: "dd-agent-windows", Namespace: "datadog"}, ds)
	assert.True(t, err != nil, "Windows DaemonSet should be deleted under FIPS")
	found := false
	for _, c := range newStatus.Conditions {
		if c.Type == "WindowsAgentReconcile" && c.Reason == "FIPSWindowsUnsupported" {
			found = true
		}
	}
	assert.True(t, found, "FIPS condition should be surfaced")
}

// Cleanup must be owner-scoped: removing windowsNodeAgent on one DDAI must NOT delete
// another DDAI's Windows DaemonSet in the same namespace (regression guard).
func TestReconcileV2WindowsAgent_CleanupOwnerScoped(t *testing.T) {
	// Our DDAI's Windows DS.
	ownDS := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "dd-agent-windows", Namespace: "datadog",
			Labels: map[string]string{"agent.datadoghq.com/component": "agent-windows", "app.kubernetes.io/part-of": "datadog-dd"},
		},
	}
	// A different DDAI's Windows DS in the same namespace (different part-of).
	otherDS := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "other-agent-windows", Namespace: "datadog",
			Labels: map[string]string{"agent.datadoghq.com/component": "agent-windows", "app.kubernetes.io/part-of": "datadog-other"},
		},
	}
	r := newWindowsTestReconciler(ownDS, otherDS)
	ddai := windowsTestDDAI(map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{}, nil)
	newStatus := &datadoghqv1alpha1.DatadogAgentInternalStatus{}

	_, err := r.reconcileV2WindowsAgent(context.Background(), feature.RequiredComponents{}, nil, ddai, nil, newStatus)
	require.NoError(t, err)

	// Our DS deleted.
	err = r.client.Get(context.Background(), types.NamespacedName{Name: "dd-agent-windows", Namespace: "datadog"}, &appsv1.DaemonSet{})
	assert.True(t, err != nil, "own Windows DaemonSet should be deleted")
	// Other DDAI's DS must survive.
	err = r.client.Get(context.Background(), types.NamespacedName{Name: "other-agent-windows", Namespace: "datadog"}, &appsv1.DaemonSet{})
	assert.NoError(t, err, "another DDAI's Windows DaemonSet must NOT be deleted")
}

// stubFeature is a minimal feature.Feature whose Configure returns a fixed set of
// node-agent required containers (enough to test windowsContainersFromFeatures).
type stubFeature struct {
	id             feature.IDType
	nodeContainers []apicommon.AgentContainerName
}

func (f stubFeature) ID() feature.IDType { return f.id }
func (f stubFeature) Configure(metav1.Object, *datadoghqv2alpha1.DatadogAgentSpec, *datadoghqv2alpha1.RemoteConfigConfiguration) feature.RequiredComponents {
	return feature.RequiredComponents{
		Agent: feature.RequiredComponent{Containers: f.nodeContainers},
	}
}
func (f stubFeature) ManageDependencies(feature.ResourceManagers) error                { return nil }
func (f stubFeature) ManageClusterAgent(feature.PodTemplateManagers) error             { return nil }
func (f stubFeature) ManageNodeAgent(feature.PodTemplateManagers) error                { return nil }
func (f stubFeature) ManageSingleContainerNodeAgent(feature.PodTemplateManagers) error { return nil }
func (f stubFeature) ManageClusterChecksRunner(feature.PodTemplateManagers) error      { return nil }
func (f stubFeature) ManageOtelAgentGateway(feature.PodTemplateManagers) error         { return nil }

// windowsContainersFromFeatures rebuilds the Windows container set from each feature's
// node-agent RequiredComponents (Configure), filtered to the Windows-supported sidecars.
func TestWindowsContainersFromFeatures(t *testing.T) {
	core := apicommon.CoreAgentContainerName
	trace := apicommon.TraceAgentContainerName
	process := apicommon.ProcessAgentContainerName
	ddai := windowsTestDDAI(nil, nil)

	cases := []struct {
		name  string
		feats []feature.Feature
		want  []apicommon.AgentContainerName
	}{
		{
			name:  "core only (feature needs no node sidecar)",
			feats: []feature.Feature{stubFeature{id: feature.LogCollectionIDType}},
			want:  []apicommon.AgentContainerName{core},
		},
		{
			name:  "node APM requires trace-agent",
			feats: []feature.Feature{stubFeature{id: feature.APMIDType, nodeContainers: []apicommon.AgentContainerName{core, trace}}},
			want:  []apicommon.AgentContainerName{core, trace},
		},
		{
			// APM enabled cluster-side only (SSI): node APM off → Configure returns no trace.
			// Must NOT add an unconfigured trace-agent.
			name:  "cluster-only APM does not add trace-agent",
			feats: []feature.Feature{stubFeature{id: feature.APMIDType, nodeContainers: nil}},
			want:  []apicommon.AgentContainerName{core},
		},
		{
			name: "process feature adds process (deduped across features)",
			feats: []feature.Feature{
				stubFeature{id: feature.LiveProcessIDType, nodeContainers: []apicommon.AgentContainerName{core, process}},
				stubFeature{id: feature.ProcessDiscoveryIDType, nodeContainers: []apicommon.AgentContainerName{core, process}},
			},
			want: []apicommon.AgentContainerName{core, process},
		},
		{
			name: "apm + process",
			feats: []feature.Feature{
				stubFeature{id: feature.APMIDType, nodeContainers: []apicommon.AgentContainerName{core, trace}},
				stubFeature{id: feature.LiveContainerIDType, nodeContainers: []apicommon.AgentContainerName{core, process}},
			},
			want: []apicommon.AgentContainerName{core, trace, process},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.ElementsMatch(t, c.want, windowsContainersFromFeatures(c.feats, ddai))
		})
	}
}
