// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	common "github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/experimental"
	agenttestutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/testutils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/testutils"

	assert "github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type testCase struct {
	name                 string
	clientBuilder        *fake.ClientBuilder
	loadFunc             func(c client.Client) *v2alpha1.DatadogAgent
	dda                  *v2alpha1.DatadogAgent
	nodes                []client.Object
	want                 reconcile.Result
	wantErr              bool
	wantFunc             func(t *testing.T, c client.Client)
	profile              *v1alpha1.DatadogAgentProfile // For DDAI tests
	profilesEnabled      bool                          // For DDAI tests
	ddaiEnabled          bool                          // For DDAI tests
	introspectionEnabled bool                          // For introspection tests
	focus                bool                          // For debugging: run only focused tests if any are focused
}

// runTestCases runs test cases, respecting the focus field for debugging.
// If any test has focus=true, only focused tests run. Otherwise all tests run.
func runTestCases(t *testing.T, tests []testCase, testFunc func(t *testing.T, tt testCase, opts ReconcilerOptions)) {
	// Check if any test is focused
	hasFocused := false
	for _, tt := range tests {
		if tt.focus {
			hasFocused = true
			break
		}
	}

	// Run tests
	for _, tt := range tests {
		// Skip non-focused tests if any test is focused
		if hasFocused && !tt.focus {
			continue
		}

		t.Run(tt.name, func(t *testing.T) {
			// Create a copy of opts for this test
			testOpts := ReconcilerOptions{
				DatadogAgentInternalEnabled: tt.ddaiEnabled,
				DatadogAgentProfileEnabled:  tt.profilesEnabled,
				IntrospectionEnabled:        tt.introspectionEnabled,
			}

			testFunc(t, tt, testOpts)
		})
	}
}

// runDDAReconcilerTest runs test case using only the DDA reconciler
func runDDAReconcilerTest(t *testing.T, tt testCase, opts ReconcilerOptions) {
	t.Helper()

	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := agenttestutils.TestScheme()

	// Create test event recorder and forwarders
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(s, corev1.EventSource{Component: "test"})
	forwarders := dummyManager{}

	c := buildClient(t, tt, s, false)

	// Create reconciler
	r := &Reconciler{
		client:       c,
		scheme:       s,
		platformInfo: kubernetes.PlatformInfo{},
		recorder:     recorder,
		log:          logf.Log.WithName(tt.name),
		forwarders:   forwarders,
		options:      opts,
	}
	r.initializeComponentRegistry()

	// Load or create DatadogAgent
	var dda *v2alpha1.DatadogAgent
	if tt.loadFunc != nil {
		dda = tt.loadFunc(r.client)
	} else if tt.dda != nil {
		_ = r.client.Create(context.TODO(), tt.dda)
		dda = tt.dda
	}

	// Run reconciliation
	got, err := r.Reconcile(context.TODO(), dda)

	// Assert on error expectation
	if tt.wantErr {
		assert.Error(t, err, "ReconcileDatadogAgent.Reconcile() expected an error")
	} else {
		assert.NoError(t, err, "ReconcileDatadogAgent.Reconcile() unexpected error: %v", err)
	}

	// Assert on reconciliation result
	assert.Equal(t, tt.want, got, "ReconcileDatadogAgent.Reconcile() unexpected result")

	// Run custom validation if provided
	if tt.wantFunc != nil {
		tt.wantFunc(t, r.client)
	}
}

// runFullReconcilerTest runs test case using both DDA and DDAI reconcilers
func runFullReconcilerTest(t *testing.T, tt testCase, opts ReconcilerOptions) {
	t.Helper()

	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := agenttestutils.TestScheme()

	// Create test event recorder and forwarders
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(s, corev1.EventSource{Component: "test"})
	forwarders := dummyManager{}

	opts.DatadogAgentInternalEnabled = true
	c := buildClient(t, tt, s, true)

	// Create reconciler
	r := &Reconciler{
		client:       c,
		scheme:       s,
		platformInfo: kubernetes.PlatformInfo{},
		recorder:     recorder,
		log:          logf.Log.WithName(tt.name),
		forwarders:   forwarders,
		options:      opts,
	}
	r.initializeComponentRegistry()

	ri, err := datadogagentinternal.NewReconciler(
		datadogagentinternal.ReconcilerOptions{},
		c,
		kubernetes.PlatformInfo{},
		s,
		logf.Log.WithName(tt.name),
		recorder,
		forwarders)
	assert.NoError(t, err, "Failed to create datadogagentinternal reconciler")

	// Load or create DatadogAgent
	var dda *v2alpha1.DatadogAgent
	if tt.loadFunc != nil {
		dda = tt.loadFunc(r.client)
	} else if tt.dda != nil {
		_ = r.client.Create(context.TODO(), tt.dda)
		dda = tt.dda
	}

	// Run reconciliation
	got, err := r.Reconcile(context.TODO(), dda)

	ddais := &v1alpha1.DatadogAgentInternalList{}
	err = c.List(context.TODO(), ddais)
	assert.NoError(t, err, "Failed to list datadogagentinternal")
	assert.NotEmpty(t, ddais.Items, "Expected at least 1 ddai")
		for i := range ddais.Items {
			ddai := &ddais.Items[i]
			_, err := ri.Reconcile(context.TODO(), ddai)
			assert.NoError(t, err, "Failed to reconcile datadogagentinternal")
		}

	// Assert on error expectation
	if tt.wantErr {
		assert.Error(t, err, "ReconcileDatadogAgent.Reconcile() expected an error")
	} else {
		assert.NoError(t, err, "ReconcileDatadogAgent.Reconcile() unexpected error: %v", err)
	}

	// Assert on reconciliation result
	assert.Equal(t, tt.want, got, "ReconcileDatadogAgent.Reconcile() unexpected result")

	// Run custom validation if provided
	if tt.wantFunc != nil {
		tt.wantFunc(t, r.client)
	}
}

func buildClient(t *testing.T, tt testCase, s *runtime.Scheme, ddaiEnabled bool) client.Client {
	var builder *fake.ClientBuilder
	if tt.clientBuilder != nil {
		// Deep copy primarily to avoid adding CRD twice when running both DDA and full reconciler tests
		copy := *tt.clientBuilder
		builder = &copy
	} else {
		builder = fake.NewClientBuilder().
			WithStatusSubresource(&appsv1.DaemonSet{}, &corev1.Node{}, &v2alpha1.DatadogAgent{})
	}

	if tt.nodes != nil {
		builder = builder.WithObjects(tt.nodes...)
	}

	// Add DDAI CRD from file if DDAI is enabled
	if tt.ddaiEnabled || ddaiEnabled {
		crd, err := getDDAICRDFromConfig(s)
		assert.NoError(t, err)
		builder = builder.WithObjects(crd).WithStatusSubresource(&v1alpha1.DatadogAgentInternal{})
	}

	return builder.Build()
}

func TestReconcileDatadogAgentV2_Reconcile(t *testing.T) {
	const resourcesName = "foo"
	const resourcesNamespace = "bar"
	const dsName = "foo-agent"

	defaultRequeueDuration := 15 * time.Second

	tests := []testCase{
		{
			name: "DatadogAgent default, create Daemonset with core and trace agents",
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
					Build()
				_ = c.Create(context.TODO(), dda)
				return dda
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(t *testing.T, c client.Client) {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
				}

				verifyDaemonsetContainers(t, c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "DatadogAgent singleProcessContainer, create Daemonset with core and agents",
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
					WithSingleContainerStrategy(false).
					Build()
				_ = c.Create(context.TODO(), dda)
				return dda
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(t *testing.T, c client.Client) {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
				}

				verifyDaemonsetContainers(t, c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "[single container] DatadogAgent default, create Daemonset with a single container",
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
					WithSingleContainerStrategy(true).
					Build()
				_ = c.Create(context.TODO(), dda)
				return dda
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(t *testing.T, c client.Client) {
				expectedContainers := []string{
					string(apicommon.UnprivilegedSingleAgentContainerName),
				}

				verifyDaemonsetContainers(t, c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "DatadogAgent with APM enabled, create Daemonset with core and process agents",
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
					WithAPMEnabled(true).
					WithSingleContainerStrategy(false).
					Build()
				_ = c.Create(context.TODO(), dda)
				return dda
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(t *testing.T, c client.Client) {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
				}

				verifyDaemonsetContainers(t, c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "[single container] DatadogAgent with APM enabled, create Daemonset with a single container",
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
					WithAPMEnabled(true).
					WithSingleContainerStrategy(true).
					Build()
				_ = c.Create(context.TODO(), dda)
				return dda
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(t *testing.T, c client.Client) {
				expectedContainers := []string{
					string(apicommon.UnprivilegedSingleAgentContainerName),
				}

				verifyDaemonsetContainers(t, c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "DatadogAgent with APM and CWS enables, create Daemonset with four agents",
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
					WithAPMEnabled(true).
					WithCWSEnabled(true).
					WithSingleContainerStrategy(false).
					Build()
				_ = c.Create(context.TODO(), dda)
				return dda
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(t *testing.T, c client.Client) {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
					string(apicommon.SystemProbeContainerName),
					string(apicommon.SecurityAgentContainerName),
				}

				verifyDaemonsetContainers(t, c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "[single container] DatadogAgent with APM and CWS enables, create Daemonset with four agents",
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
					WithAPMEnabled(true).
					WithCWSEnabled(true).
					WithSingleContainerStrategy(true).
					Build()

				_ = c.Create(context.TODO(), dda)
				return dda
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(t *testing.T, c client.Client) {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
					string(apicommon.SystemProbeContainerName),
					string(apicommon.SecurityAgentContainerName),
				}

				verifyDaemonsetContainers(t, c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "DatadogAgent with APM and OOMKill enabled, create Daemonset with core, trace, and system-probe",
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
					WithAPMEnabled(true).
					WithOOMKillEnabled(true).
					WithSingleContainerStrategy(false).
					Build()
				_ = c.Create(context.TODO(), dda)
				return dda
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(t *testing.T, c client.Client) {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
					string(apicommon.SystemProbeContainerName),
				}

				verifyDaemonsetContainers(t, c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "[single container] DatadogAgent with APM and OOMKill enabled, create Daemonset with core, trace, and system-probe",
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
					WithAPMEnabled(true).
					WithOOMKillEnabled(true).
					WithSingleContainerStrategy(true).
					Build()
				_ = c.Create(context.TODO(), dda)
				return dda
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(t *testing.T, c client.Client) {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
					string(apicommon.SystemProbeContainerName),
				}

				verifyDaemonsetContainers(t, c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "DatadogAgent with FIPS enabled",
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				fipsConfig := v2alpha1.FIPSConfig{
					Enabled: apiutils.NewBoolPointer(true),
				}
				dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
					WithFIPS(fipsConfig).
					Build()
				_ = c.Create(context.TODO(), dda)
				return dda
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(t *testing.T, c client.Client) {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
					string(apicommon.FIPSProxyContainerName),
				}

				verifyDaemonsetContainers(t, c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name:          "DatadogAgent with PDB enabled",
			clientBuilder: fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v2alpha1.DatadogAgent{}, &policyv1.PodDisruptionBudget{}),
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
					WithComponentOverride(v2alpha1.ClusterAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
						CreatePodDisruptionBudget: apiutils.NewBoolPointer(true),
					}).
					WithClusterChecksUseCLCEnabled(true).
					WithComponentOverride(v2alpha1.ClusterChecksRunnerComponentName, v2alpha1.DatadogAgentComponentOverride{
						CreatePodDisruptionBudget: apiutils.NewBoolPointer(true),
					}).
					Build()
				_ = c.Create(context.TODO(), dda)
				return dda
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(t *testing.T, c client.Client) {
				verifyPDB(t, c)
			},
		},
		{
			name: "DatadogAgent with override.nodeAgent.disabled true",
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
					WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
						Disabled: apiutils.NewBoolPointer(true),
					}).
					Build()
				_ = c.Create(context.TODO(), dda)
				return dda
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(t *testing.T, c client.Client) {
				ds := &appsv1.DaemonSet{}
				err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: dsName}, ds)
				assert.NoError(t, client.IgnoreNotFound(err), "Unexpected error getting resource")
			},
		},
		{
			name: "DCA status and condition set",
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
					WithStatus(v2alpha1.DatadogAgentStatus{
						ClusterAgent: &v2alpha1.DeploymentStatus{
							GeneratedToken: "token",
						},
					}).
					Build()
				_ = c.Create(context.TODO(), dda)
				return dda
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(t *testing.T, c client.Client) {
				dda := &v2alpha1.DatadogAgent{}
				err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: resourcesName}, dda)
				assert.NoError(t, client.IgnoreNotFound(err), "Unexpected error getting resource")
				assert.NotNil(t, dda.Status.ClusterAgent, "DCA status should be set")
				assert.Equal(t, "token", dda.Status.ClusterAgent.GeneratedToken)
				dcaCondition := condition.GetCondition(&dda.Status, common.ClusterAgentReconcileConditionType)
				// Condition may be set in DDAI if full reconciler is used.
				if dcaCondition == nil {
					ddai := &v1alpha1.DatadogAgentInternal{}
					err = c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: resourcesName}, ddai)
					assert.NoError(t, client.IgnoreNotFound(err), "Unexpected error getting resource")
					dcaCondition = condition.GetDDAICondition(&ddai.Status, common.ClusterAgentReconcileConditionType)
				}
				assert.True(t, dcaCondition.Status == metav1.ConditionTrue && dcaCondition.Reason == "reconcile_succeed", "DCA status condition should be set")
			},
		},
		{
			name: "DCA status condition should be deleted when disabled",
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
					WithComponentOverride(v2alpha1.ClusterAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
						Disabled: apiutils.NewBoolPointer(true),
					}).
					WithStatus(v2alpha1.DatadogAgentStatus{
						ClusterAgent: &v2alpha1.DeploymentStatus{
							GeneratedToken: "token",
						},
					}).
					Build()
				_ = c.Create(context.TODO(), dda)
				return dda
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(t *testing.T, c client.Client) {
				dda := &v2alpha1.DatadogAgent{}
				err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: resourcesName}, dda)
				assert.NoError(t, client.IgnoreNotFound(err), "Unexpected error getting resource")
				// assert.Equal(t, "token", dda.Status.ClusterAgent.GeneratedToken)
				conflictCondition := condition.GetCondition(&dda.Status, common.OverrideReconcileConflictConditionType)
				if conflictCondition == nil {
					// Condition will be set in DDAI if full reconciler is used.
					ddai := &v1alpha1.DatadogAgentInternal{}
					err = c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: resourcesName}, ddai)
					assert.NoError(t, client.IgnoreNotFound(err), "Unexpected error getting resource")
					assert.Nil(t, ddai.Status.ClusterAgent, "DCA status should be nil when cleaned up")
					assert.Nil(t, condition.GetDDAICondition(&ddai.Status, common.ClusterAgentReconcileConditionType), "DCA status condition should be nil when cleaned up")
					conflictCondition = condition.GetDDAICondition(&ddai.Status, common.OverrideReconcileConflictConditionType)
				} else {
					// Condition will be set via DDA reconciler.
					assert.Nil(t, dda.Status.ClusterAgent, "DCA status should be nil when cleaned up")
					assert.Nil(t, condition.GetCondition(&dda.Status, common.ClusterAgentReconcileConditionType), "DCA status condition should be nil when cleaned up")
					assert.True(t, conflictCondition.Status == metav1.ConditionTrue, "OverrideReconcileConflictCondition should be true")
				}
			},
		},
		{
			name: "CLC status and condition set",
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
					WithClusterChecksEnabled(true).
					WithClusterChecksUseCLCEnabled(true).
					Build()
				_ = c.Create(context.TODO(), dda)
				return dda
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(t *testing.T, c client.Client) {
				dda := &v2alpha1.DatadogAgent{}
				err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: resourcesName}, dda)
				assert.NoError(t, client.IgnoreNotFound(err), "Unexpected error getting resource")
				clcStatus := dda.Status.ClusterChecksRunner
				var clcCondition *metav1.Condition
				if clcStatus == nil {
					// Condition will be set in DDAI if full reconciler is used.
					ddai := &v1alpha1.DatadogAgentInternal{}
					err = c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: resourcesName}, ddai)
					assert.NoError(t, client.IgnoreNotFound(err), "Unexpected error getting resource")
					clcStatus = ddai.Status.ClusterChecksRunner
					clcCondition = condition.GetDDAICondition(&ddai.Status, common.ClusterChecksRunnerReconcileConditionType)
					assert.NotNil(t, clcCondition, "CLC status condition should be set")
					assert.True(t, clcCondition.Status == metav1.ConditionTrue && clcCondition.Reason == "reconcile_succeed", "CLC status condition should be set")
				} else {
					// Condition will be set via DDA reconciler.
					clcCondition = condition.GetCondition(&dda.Status, common.ClusterChecksRunnerReconcileConditionType)
					assert.True(t, clcCondition.Status == metav1.ConditionTrue && clcCondition.Reason == "reconcile_succeed", "CLC status condition should be set")
				}
			},
		},
		{
			name: "CLC status condition should be set to conflict when disable via override",
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
					WithComponentOverride(v2alpha1.ClusterChecksRunnerComponentName, v2alpha1.DatadogAgentComponentOverride{
						Disabled: apiutils.NewBoolPointer(true),
					}).
					WithClusterChecksEnabled(true).
					WithClusterChecksUseCLCEnabled(true).
					Build()
				_ = c.Create(context.TODO(), dda)
				return dda
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(t *testing.T, c client.Client) {
				dda := &v2alpha1.DatadogAgent{}
				err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: resourcesName}, dda)
				assert.NoError(t, client.IgnoreNotFound(err), "Unexpected error getting resource")
				assert.Nil(t, dda.Status.ClusterChecksRunner, "CLC status should be nil when cleaned up")
				assert.Nil(t, condition.GetCondition(&dda.Status, common.ClusterChecksRunnerReconcileConditionType), "CLC status condition should be nil when cleaned up")
				conflictCondition := condition.GetCondition(&dda.Status, common.OverrideReconcileConflictConditionType)
				if conflictCondition == nil {
					ddai := &v1alpha1.DatadogAgentInternal{}
					err = c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: resourcesName}, ddai)
					assert.NoError(t, client.IgnoreNotFound(err), "Unexpected error getting resource")
					conflictCondition = condition.GetDDAICondition(&ddai.Status, common.OverrideReconcileConflictConditionType)
				}
				assert.True(t, conflictCondition.Status == metav1.ConditionTrue, "OverrideReconcileConflictCondition should be true")
			},
		},
	}

	runTestCases(t, tests, runDDAReconcilerTest)
	runTestCases(t, tests, runFullReconcilerTest)
}

func Test_Introspection(t *testing.T) {
	const resourcesName = "foo"
	const resourcesNamespace = "bar"

	defaultRequeueDuration := 15 * time.Second

	tests := []testCase{
		{
			name:                 "[introspection] Daemonset names with affinity override",
			introspectionEnabled: true,
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
					WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
						Affinity: &corev1.Affinity{
							PodAntiAffinity: &corev1.PodAntiAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
									{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"foo": "bar",
											},
										},
										TopologyKey: "baz",
									},
								},
							},
						},
					}).
					Build()
				_ = c.Create(context.TODO(), dda)
				return dda
			},
			nodes: []client.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "default-node",
						Labels: map[string]string{
							"foo": "bar",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gke-cos-node",
						Labels: map[string]string{
							kubernetes.GKEProviderLabel: kubernetes.GKECosType,
						},
					},
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(t *testing.T, c client.Client) {
				expectedDaemonsets := []string{
					string("foo-agent-default"),
					string("foo-agent-gke-cos"),
				}

				verifyDaemonsetNames(t, c, resourcesNamespace, expectedDaemonsets)
			},
		},
	}

	// introspection is supported only with the DDA reconciler
	runTestCases(t, tests, runDDAReconcilerTest)
}

func Test_otelImageTags(t *testing.T) {
	const resourcesName = "foo"
	const resourcesNamespace = "bar"
	const dsName = "foo-agent"

	defaultRequeueDuration := 15 * time.Second

	tests := []testCase{
		{
			name: "otelEnabled true, no override",
			dda: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithOTelCollectorEnabled(true).
				Build(),
			want: reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantFunc: func(t *testing.T, c client.Client) {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
					string(apicommon.OtelAgent),
				}
				verifyDaemonsetContainers(t, c, resourcesNamespace, dsName, expectedContainers)
				agentContainer := getDsContainers(c, resourcesNamespace, dsName)

				assert.Equal(t, fmt.Sprintf("gcr.io/datadoghq/agent:%s", images.AgentLatestVersion), agentContainer[apicommon.CoreAgentContainerName].Image)
				assert.Equal(t, fmt.Sprintf("gcr.io/datadoghq/agent:%s", images.AgentLatestVersion), agentContainer[apicommon.TraceAgentContainerName].Image)
				assert.Equal(t, fmt.Sprintf("gcr.io/datadoghq/ddot-collector:%s", images.AgentLatestVersion), agentContainer[apicommon.OtelAgent].Image)

			},
		},
		{
			name: "otelEnabled true, override Tag to compatible version",
			dda: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithOTelCollectorEnabled(true).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
					Image: &v2alpha1.AgentImageConfig{
						Tag: "7.71.0",
					},
				}).
				Build(),
			want: reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantFunc: func(t *testing.T, c client.Client) {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
					string(apicommon.OtelAgent),
				}
				verifyDaemonsetContainers(t, c, resourcesNamespace, dsName, expectedContainers)
				agentContainer := getDsContainers(c, resourcesNamespace, dsName)

				assert.Equal(t, "gcr.io/datadoghq/agent:7.71.0", agentContainer[apicommon.CoreAgentContainerName].Image)
				assert.Equal(t, "gcr.io/datadoghq/agent:7.71.0", agentContainer[apicommon.TraceAgentContainerName].Image)
				assert.Equal(t, "gcr.io/datadoghq/ddot-collector:7.71.0", agentContainer[apicommon.OtelAgent].Image)

			},
		},
		{
			name: "otelEnabled true, override Tag to incompatible version",
			dda: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithOTelCollectorEnabled(true).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
					Image: &v2alpha1.AgentImageConfig{
						Tag: "7.66.0",
					},
				}).
				Build(),
			want: reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantFunc: func(t *testing.T, c client.Client) {
				// With incompatible OTel agent image, OTel container should not be present
				verifyDaemonsetContainers(t, c, resourcesNamespace, dsName, []string{string(apicommon.CoreAgentContainerName), string(apicommon.TraceAgentContainerName)})
			},
		},
		{
			name: "otelEnabled true, override Tag to incompatible full version",
			dda: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithOTelCollectorEnabled(true).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
					Image: &v2alpha1.AgentImageConfig{
						Tag: "7.72.1-full",
					},
				}).
				Build(),
			want: reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantFunc: func(t *testing.T, c client.Client) {
				// With incompatible OTel agent image, OTel container should not be present
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
				}
				verifyDaemonsetContainers(t, c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "otelEnabled true, override Name, Tag - override Name, Tag on all agents",
			dda: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithOTelCollectorEnabled(true).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
					Image: &v2alpha1.AgentImageConfig{
						Name: "testagent",
						Tag:  "7.65.0-full",
					},
				}).Build(),
			want: reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantFunc: func(t *testing.T, c client.Client) {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
					string(apicommon.OtelAgent),
				}
				verifyDaemonsetContainers(t, c, resourcesNamespace, dsName, expectedContainers)
				agentContainer := getDsContainers(c, resourcesNamespace, dsName)

				assert.Equal(t, "gcr.io/datadoghq/testagent:7.65.0-full", agentContainer[apicommon.CoreAgentContainerName].Image)
				assert.Equal(t, "gcr.io/datadoghq/testagent:7.65.0-full", agentContainer[apicommon.TraceAgentContainerName].Image)
				assert.Equal(t, "gcr.io/datadoghq/testagent:7.65.0-full", agentContainer[apicommon.OtelAgent].Image)

			},
		},
		{
			name: "otelEnabled true, override Name including tag - override Name including tag on all agents",
			dda: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithOTelCollectorEnabled(true).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
					Image: &v2alpha1.AgentImageConfig{
						Name: "testagent:7.65.0-full",
					},
				}).Build(),
			want: reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantFunc: func(t *testing.T, c client.Client) {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
					string(apicommon.OtelAgent),
				}
				verifyDaemonsetContainers(t, c, resourcesNamespace, dsName, expectedContainers)
				agentContainer := getDsContainers(c, resourcesNamespace, dsName)

				assert.Equal(t, "gcr.io/datadoghq/testagent:7.65.0-full", agentContainer[apicommon.CoreAgentContainerName].Image)
				assert.Equal(t, "gcr.io/datadoghq/testagent:7.65.0-full", agentContainer[apicommon.TraceAgentContainerName].Image)
				assert.Equal(t, "gcr.io/datadoghq/testagent:7.65.0-full", agentContainer[apicommon.OtelAgent].Image)

			},
		},
		{
			name: "otelEnabled true, override Tag and Name with full name - all agents with full name, ignoring tag",
			dda: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithOTelCollectorEnabled(true).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
					Image: &v2alpha1.AgentImageConfig{
						Name: "gcr.io/datacat/testagent:latest",
						Tag:  "7.66.0",
					},
				}).Build(),
			want: reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantFunc: func(t *testing.T, c client.Client) {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
					string(apicommon.OtelAgent),
				}
				verifyDaemonsetContainers(t, c, resourcesNamespace, dsName, expectedContainers)
				agentContainer := getDsContainers(c, resourcesNamespace, dsName)

				assert.Equal(t, "gcr.io/datacat/testagent:latest", agentContainer[apicommon.CoreAgentContainerName].Image)
				assert.Equal(t, "gcr.io/datacat/testagent:latest", agentContainer[apicommon.TraceAgentContainerName].Image)
				assert.Equal(t, "gcr.io/datacat/testagent:latest", agentContainer[apicommon.OtelAgent].Image)

			},
		},
	}

	runTestCases(t, tests, runDDAReconcilerTest)
	runTestCases(t, tests, runFullReconcilerTest)
}

func getDsContainers(c client.Client, resourcesNamespace, dsName string) map[apicommon.AgentContainerName]corev1.Container {
	ds := &appsv1.DaemonSet{}
	if err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: dsName}, ds); err != nil {
		return nil
	}

	dsContainers := map[apicommon.AgentContainerName]corev1.Container{}
	for _, container := range ds.Spec.Template.Spec.Containers {
		dsContainers[apicommon.AgentContainerName(container.Name)] = container
	}

	return dsContainers
}

func Test_AutopilotOverrides(t *testing.T) {
	const resourcesName, resourcesNamespace, dsName = "foo", "bar", "foo-agent"

	defaultRequeueDuration := 15 * time.Second

	tests := []testCase{
		{
			name: "autopilot enabled with core-agent only",
			want: reconcile.Result{RequeueAfter: defaultRequeueDuration},
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				autopilotKey := experimental.ExperimentalAnnotationPrefix + "/" + experimental.ExperimentalAutopilotSubkey
				dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
					WithAPMEnabled(false).
					WithClusterChecksEnabled(false).
					WithAdmissionControllerEnabled(false).
					WithOrchestratorExplorerEnabled(false).
					WithKSMEnabled(false).
					WithDogstatsdUnixDomainSocketConfigEnabled(false).
					WithAnnotations(map[string]string{
						autopilotKey: "true",
					}).
					Build()

				_ = c.Create(context.TODO(), dda)
				return dda
			},
			wantFunc: func(t *testing.T, c client.Client) {
				expectedContainers := []string{string(apicommon.CoreAgentContainerName)}
				verifyDaemonsetContainers(t, c, resourcesNamespace, dsName, expectedContainers)

				ds := &appsv1.DaemonSet{}
				c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: dsName}, ds)

				forbiddenVolumes := map[string]struct{}{
					common.AuthVolumeName:            {},
					common.CriSocketVolumeName:       {},
					common.DogstatsdSocketVolumeName: {},
					common.APMSocketVolumeName:       {},
				}
				for _, v := range ds.Spec.Template.Spec.Volumes {
					if _, found := forbiddenVolumes[v.Name]; found {
						assert.Fail(t, "forbidden volume %s is not allowed in GKE Autopilot", v.Name)
					}
				}

				initVolumePatchFound := false
				for _, ic := range ds.Spec.Template.Spec.InitContainers {
					if ic.Name == "init-volume" {
						if len(ic.Args) != 1 || ic.Args[0] != "cp -r /etc/datadog-agent /opt" {
							assert.Fail(t, "init-volume args not patched correctly, got: %v", ic.Args)
						}
						initVolumePatchFound = true
					}

					forbiddenMounts := map[string]struct{}{
						common.AuthVolumeName:      {},
						common.CriSocketVolumeName: {},
					}
					for _, m := range ic.VolumeMounts {
						if _, found := forbiddenMounts[m.Name]; found {
							assert.Fail(t, "forbidden mount %s in init container %s is not allowed in GKE Autopilot", m.Name, ic.Name)
						}
					}
				}
				if !initVolumePatchFound {
					assert.Fail(t, "init-volume container not found or not patched")
				}

				for _, ctn := range ds.Spec.Template.Spec.Containers {
					if ctn.Name == string(apicommon.CoreAgentContainerName) {
						forbiddenMounts := map[string]struct{}{
							common.AuthVolumeName:            {},
							common.DogstatsdSocketVolumeName: {},
							common.CriSocketVolumeName:       {},
						}
						for _, m := range ctn.VolumeMounts {
							if _, found := forbiddenMounts[m.Name]; found {
								assert.Fail(t, "forbidden mount %s found in core agent is not allowed in GKE Autopilot", m.Name)
							}
						}
					}
				}

			},
		},
		{
			name: "autopilot enabled with core-agent and trace-agent",
			want: reconcile.Result{RequeueAfter: defaultRequeueDuration},
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				autopilotKey := experimental.ExperimentalAnnotationPrefix + "/" + experimental.ExperimentalAutopilotSubkey
				dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
					WithAPMEnabled(true).
					WithClusterChecksEnabled(false).
					WithAdmissionControllerEnabled(false).
					WithOrchestratorExplorerEnabled(false).
					WithKSMEnabled(false).
					WithDogstatsdUnixDomainSocketConfigEnabled(false).
					WithAnnotations(map[string]string{
						autopilotKey: "true",
					}).
					Build()

				_ = c.Create(context.TODO(), dda)
				return dda
			},
			wantFunc: func(t *testing.T, c client.Client) {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
				}
				verifyDaemonsetContainers(t, c, resourcesNamespace, dsName, expectedContainers)

				ds := &appsv1.DaemonSet{}
				c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: dsName}, ds)

				traceAgentFound := false
				for _, ctn := range ds.Spec.Template.Spec.Containers {
					if ctn.Name == string(apicommon.TraceAgentContainerName) {
						expectedCommand := []string{
							"trace-agent",
							"-config=/etc/datadog-agent/datadog.yaml",
						}
						if !reflect.DeepEqual(ctn.Command, expectedCommand) {
							assert.Fail(t, "trace-agent command incorrect, expected: %v, got: %v", expectedCommand, ctn.Command)
						}

						forbiddenMounts := map[string]struct{}{
							common.AuthVolumeName:            {},
							common.CriSocketVolumeName:       {},
							common.ProcdirVolumeName:         {},
							common.CgroupsVolumeName:         {},
							common.APMSocketVolumeName:       {},
							common.DogstatsdSocketVolumeName: {},
						}
						for _, m := range ctn.VolumeMounts {
							if _, found := forbiddenMounts[m.Name]; found {
								assert.Fail(t, "forbidden mount %s should be removed from trace-agent", m.Name)
							}
						}
						traceAgentFound = true
					}
				}
				if !traceAgentFound {
					assert.Fail(t, "trace-agent container not found")
				}

			},
		},
	}

	runTestCases(t, tests, runDDAReconcilerTest)
	runTestCases(t, tests, runFullReconcilerTest)
}

// Helper function for creating DatadogAgent with cluster checks enabled
func createDatadogAgentWithClusterChecks(c client.Client, namespace, name string) *v2alpha1.DatadogAgent {
	dda := testutils.NewInitializedDatadogAgentBuilder(namespace, name).
		WithClusterChecksEnabled(true).
		WithClusterChecksUseCLCEnabled(true).
		Build()
	_ = c.Create(context.TODO(), dda)
	return dda
}

func Test_Control_Plane_Monitoring(t *testing.T) {
	const resourcesName = "foo"
	const resourcesNamespace = "bar"
	const dcaName = "foo-cluster-agent"
	const dsName = "foo-agent-default"

	defaultRequeueDuration := 15 * time.Second

	tests := []testCase{
		{
			name:                 "[introspection] Control Plane Monitoring for Openshift",
			introspectionEnabled: true,
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				return createDatadogAgentWithClusterChecks(c, resourcesNamespace, resourcesName)
			},
			nodes: []client.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "openshift-node-1",
						Labels: map[string]string{
							kubernetes.OpenShiftProviderLabel: "rhel",
						},
					},
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(t *testing.T, c client.Client) {
				verifyDCADeployment(t, c, resourcesName, resourcesNamespace, dcaName, "openshift")
				expectedDaemonsets := []string{
					dsName,
				}
				verifyDaemonsetNames(t, c, resourcesNamespace, expectedDaemonsets)
				verifyEtcdMountsOpenshift(t, c, resourcesNamespace, dsName, "openshift")
			},
		},
		{
			name:                 "[introspection] Control Plane Monitoring with EKS",
			introspectionEnabled: true,
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				return createDatadogAgentWithClusterChecks(c, resourcesNamespace, resourcesName)
			},
			nodes: []client.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "eks-node-1",
						Labels: map[string]string{
							kubernetes.EKSProviderLabel: "amazon-eks-node-1.29-v20240627",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "default-node-2",
						Labels: map[string]string{
							kubernetes.DefaultProvider: "",
						},
					},
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(t *testing.T, c client.Client) {
				verifyDCADeployment(t, c, resourcesName, resourcesNamespace, dcaName, "eks")
				expectedDaemonsets := []string{
					dsName,
				}
				verifyDaemonsetNames(t, c, resourcesNamespace, expectedDaemonsets)
			},
		},
		{
			name:                 "[introspection] Control Plane Monitoring with multiple providers",
			introspectionEnabled: true,
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				return createDatadogAgentWithClusterChecks(c, resourcesNamespace, resourcesName)
			},
			nodes: []client.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "eks-node-1",
						Labels: map[string]string{
							kubernetes.EKSProviderLabel: "amazon-eks-node-1.29-v20240627",
						},
					},
				},
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "openshift-node-2",
						Labels: map[string]string{
							kubernetes.OpenShiftProviderLabel: "rhcos",
						},
					},
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(t *testing.T, c client.Client) {
				verifyDCADeployment(t, c, resourcesName, resourcesNamespace, dcaName, "default")
				expectedDaemonsets := []string{
					dsName,
				}
				verifyDaemonsetNames(t, c, resourcesNamespace, expectedDaemonsets)
			},
		},
		{
			// This test verifies that when a node has a GKE provider label with an unsupported OS value,
			// the system falls back to the "default" provider for control plane monitoring
			name:                 "[introspection] Control Plane Monitoring with unsupported provider",
			introspectionEnabled: true,
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				return createDatadogAgentWithClusterChecks(c, resourcesNamespace, resourcesName)
			},
			nodes: []client.Object{
				&corev1.Node{
					ObjectMeta: metav1.ObjectMeta{
						Name: "gke-node-1",
						Labels: map[string]string{
							// Use unsupported OS value to trigger fallback to "default" provider
							kubernetes.GKEProviderLabel: "unsupported-os",
						},
					},
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(t *testing.T, c client.Client) {
				verifyDCADeployment(t, c, resourcesName, resourcesNamespace, dcaName, "default")
				expectedDaemonsets := []string{
					dsName,
				}
				verifyDaemonsetNames(t, c, resourcesNamespace, expectedDaemonsets)
			},
		},
	}

	// introspection is supported only with the DDA reconciler
	runTestCases(t, tests, runDDAReconcilerTest)
}

func verifyDCADeployment(t *testing.T, c client.Client, ddaName, resourcesNamespace, expectedName string, provider string) {
	deploymentList := appsv1.DeploymentList{}
	err := c.List(context.TODO(), &deploymentList, client.HasLabels{constants.MD5AgentDeploymentProviderLabelKey})
	assert.NoError(t, err, "Failed to list deployments")
	assert.Equal(t, 1, len(deploymentList.Items), "Expected 1 deployment")
	assert.Equal(t, expectedName, deploymentList.Items[0].ObjectMeta.Name, "Deployment name mismatch")

	cms := corev1.ConfigMapList{}
	err = c.List(context.TODO(), &cms, client.InNamespace(resourcesNamespace))
	assert.NoError(t, err, "Failed to list ConfigMaps")

	dcaDeployment := deploymentList.Items[0]
	if provider == kubernetes.DefaultProvider {
		for _, cm := range cms.Items {
			assert.NotEqual(t, fmt.Sprintf("datadog-controlplane-monitoring-%s", provider), cm.ObjectMeta.Name,
				"Default provider should not create control plane monitoring ConfigMap")
		}
		for _, volume := range dcaDeployment.Spec.Template.Spec.Volumes {
			assert.NotEqual(t, "kube-apiserver-metrics-config", volume.Name,
				"Default provider should not have control plane volumes")
		}
	} else if provider == kubernetes.OpenshiftProvider || provider == kubernetes.EKSCloudProvider {
		cpCm := corev1.ConfigMap{}
		err := c.Get(context.TODO(), types.NamespacedName{
			Name:      fmt.Sprintf("datadog-controlplane-monitoring-%s", provider),
			Namespace: resourcesNamespace,
		}, &cpCm)
		assert.NoError(t, err, "Control plane monitoring ConfigMap should exist for provider %s", provider)

		verifyCheckMounts(t, dcaDeployment, provider, "kube-apiserver-metrics")
		verifyCheckMounts(t, dcaDeployment, provider, "kube-controller-manager")
		verifyCheckMounts(t, dcaDeployment, provider, "kube-scheduler")
	}
	if provider == kubernetes.OpenshiftProvider {
		verifyCheckMounts(t, dcaDeployment, provider, "etcd")
	}
}

func verifyCheckMounts(t *testing.T, dcaDeployment appsv1.Deployment, provider string, checkName string) {
	volumeToKeyMap := map[string]string{
		"kube-apiserver-metrics":  "kube_apiserver_metrics",
		"kube-controller-manager": "kube_controller_manager",
		"kube-scheduler":          "kube_scheduler",
		"etcd":                    "etcd",
	}
	configMapKey := volumeToKeyMap[checkName]

	assert.Contains(t, dcaDeployment.Spec.Template.Spec.Volumes, corev1.Volume{
		Name: fmt.Sprintf("%s-config", checkName),
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: fmt.Sprintf("datadog-controlplane-monitoring-%s", provider),
				},
				Items: []corev1.KeyToPath{
					{
						Key:  fmt.Sprintf("%s.yaml", configMapKey),
						Path: fmt.Sprintf("%s.yaml", configMapKey),
					},
				},
			},
		},
	})

	dcaContainer := dcaDeployment.Spec.Template.Spec.Containers[0]
	assert.Contains(t, dcaContainer.VolumeMounts, corev1.VolumeMount{
		Name:      fmt.Sprintf("%s-config", checkName),
		MountPath: fmt.Sprintf("/etc/datadog-agent/conf.d/%s.d", configMapKey),
		ReadOnly:  true,
	})
}

func verifyDaemonsetContainers(t *testing.T, c client.Client, resourcesNamespace, dsName string, expectedContainers []string) {
	ds := &appsv1.DaemonSet{}
	err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: dsName}, ds)
	assert.NoError(t, err, "Failed to get DaemonSet %s/%s", resourcesNamespace, dsName)

	dsContainers := []string{}
	for _, container := range ds.Spec.Template.Spec.Containers {
		dsContainers = append(dsContainers, container.Name)
	}

	sort.Strings(dsContainers)
	sort.Strings(expectedContainers)
	assert.Equal(t, expectedContainers, dsContainers, "Container names don't match")
}

func verifyDaemonsetNames(t *testing.T, c client.Client, resourcesNamespace string, expectedDSNames []string) {
	daemonSetList := appsv1.DaemonSetList{}
	err := c.List(context.TODO(), &daemonSetList, client.HasLabels{constants.MD5AgentDeploymentProviderLabelKey})
	assert.NoError(t, err, "Failed to list DaemonSets")

	actualDSNames := []string{}
	for _, ds := range daemonSetList.Items {
		actualDSNames = append(actualDSNames, ds.Name)
	}
	sort.Strings(actualDSNames)
	sort.Strings(expectedDSNames)
	assert.Equal(t, expectedDSNames, actualDSNames, "DaemonSet names don't match")
}

func verifyEtcdMountsOpenshift(t *testing.T, c client.Client, resourcesNamespace, dsName string, provider string) {
	expectedMounts := []corev1.VolumeMount{
		{
			Name:      "etcd-client-certs",
			MountPath: "/etc/etcd-certs",
			ReadOnly:  true,
		},
		{
			Name:      "disable-etcd-autoconf",
			MountPath: "/etc/datadog-agent/conf.d/etcd.d",
			ReadOnly:  false,
		},
	}

	// Node Agent
	ds := &appsv1.DaemonSet{}
	err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: dsName}, ds)
	assert.NoError(t, err, "Failed to get DaemonSet %s/%s", resourcesNamespace, dsName)

	var coreAgentContainer *corev1.Container
	for _, container := range ds.Spec.Template.Spec.Containers {
		if container.Name == string(apicommon.CoreAgentContainerName) {
			coreAgentContainer = &container
			break
		}
	}

	assert.NotNil(t, coreAgentContainer, "core agent container not found in DaemonSet %s", dsName)

	for _, expectedMount := range expectedMounts {
		found := false
		for _, mount := range coreAgentContainer.VolumeMounts {
			if mount.Name == expectedMount.Name {
				found = true
				assert.Equal(t, expectedMount.MountPath, mount.MountPath, "Mount path mismatch for %s in core agent", expectedMount.Name)
				assert.Equal(t, expectedMount.ReadOnly, mount.ReadOnly, "ReadOnly mismatch for %s in core agent", expectedMount.Name)
				break
			}
		}
		assert.True(t, found, "Expected volume mount %s not found in core agent container", expectedMount.Name)
	}

	// Cluster Checks Runner
	deploymentList := appsv1.DeploymentList{}
	err = c.List(context.TODO(), &deploymentList, client.InNamespace(resourcesNamespace))
	assert.NoError(t, err, "Failed to list deployments")

	var ccrDeployment *appsv1.Deployment
	for _, deployment := range deploymentList.Items {
		if deployment.Name == "foo-cluster-checks-runner" {
			ccrDeployment = &deployment
			break
		}
	}

	assert.NotNil(t, ccrDeployment, "cluster-checks-runner deployment not found")

	var ccrContainer *corev1.Container
	for _, container := range ccrDeployment.Spec.Template.Spec.Containers {
		if container.Name == string(apicommon.ClusterChecksRunnersContainerName) {
			ccrContainer = &container
			break
		}
	}

	assert.NotNil(t, ccrContainer, "cluster-checks-runner container not found in deployment")

	for _, expectedMount := range expectedMounts {
		found := false
		for _, mount := range ccrContainer.VolumeMounts {
			if mount.Name == expectedMount.Name {
				found = true
				assert.Equal(t, expectedMount.MountPath, mount.MountPath, "Mount path mismatch for %s in CCR", expectedMount.Name)
				assert.Equal(t, expectedMount.ReadOnly, mount.ReadOnly, "ReadOnly mismatch for %s in CCR", expectedMount.Name)
				break
			}
		}
		assert.True(t, found, "Expected volume mount %s not found in cluster-checks-runner container", expectedMount.Name)
	}
}

func verifyPDB(t *testing.T, c client.Client) {
	pdbList := policyv1.PodDisruptionBudgetList{}
	err := c.List(context.TODO(), &pdbList)
	assert.NoError(t, err, "Failed to list PodDisruptionBudgets")
	assert.True(t, len(pdbList.Items) == 2, "Expected 2 PDBs, got %d", len(pdbList.Items))

	dcaPDB := pdbList.Items[0]
	assert.Equal(t, "foo-cluster-agent-pdb", dcaPDB.Name)
	assert.Equal(t, intstr.FromInt(1), *dcaPDB.Spec.MinAvailable)
	assert.Nil(t, dcaPDB.Spec.MaxUnavailable)

	ccrPDB := pdbList.Items[1]
	assert.Equal(t, "foo-cluster-checks-runner-pdb", ccrPDB.Name)
	assert.Equal(t, intstr.FromInt(1), *ccrPDB.Spec.MaxUnavailable)
	assert.Nil(t, ccrPDB.Spec.MinAvailable)
}

func Test_DDAI_ReconcileV3(t *testing.T) {
	const resourcesName = "foo"
	const resourcesNamespace = "bar"

	defaultRequeueDuration := 15 * time.Second

	dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).BuildWithDefaults()

	// Define profile for the test that needs it
	fooProfile := &v1alpha1.DatadogAgentProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo-profile",
			Namespace: resourcesNamespace,
		},
		Spec: v1alpha1.DatadogAgentProfileSpec{
			ProfileAffinity: &v1alpha1.ProfileAffinity{
				ProfileNodeAffinity: []corev1.NodeSelectorRequirement{
					{
						Key:      "foo",
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{"foo-profile"},
					},
				},
			},
			Config: &v2alpha1.DatadogAgentSpec{
				Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.NodeAgentComponentName: {
						Labels: map[string]string{
							"foo": "bar",
						},
					},
				},
			},
		},
	}

	tests := []testCase{
		{
			name:        "[ddai] Create DDAI from minimal DDA",
			ddaiEnabled: true,
			clientBuilder: fake.NewClientBuilder().
				WithStatusSubresource(&v2alpha1.DatadogAgent{}, &v1alpha1.DatadogAgentProfile{}, &v1alpha1.DatadogAgentInternal{}),
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				_ = c.Create(context.TODO(), dda)
				return dda
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(t *testing.T, c client.Client) {
				expectedDDAI := getBaseDDAI(dda)
				expectedDDAI.Annotations = map[string]string{
					constants.MD5DDAIDeploymentAnnotationKey: "c7280f85b8590dcaa3668ea3b789053e",
				}

				verifyDDAI(t, c, []v1alpha1.DatadogAgentInternal{expectedDDAI})
			},
		},
		{
			name:        "[ddai] Create DDAI from customized DDA",
			ddaiEnabled: true,
			clientBuilder: fake.NewClientBuilder().
				WithStatusSubresource(&v2alpha1.DatadogAgent{}, &v1alpha1.DatadogAgentProfile{}, &v1alpha1.DatadogAgentInternal{}),
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				ddaCustom := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
					WithDCAToken("abcdefghijklmnopqrstuvwxyz").
					WithCredentialsFromSecret("custom-secret", "api", "custom-secret2", "app").
					WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
						Labels: map[string]string{
							"custom-label": "custom-value",
						},
					}).
					WithClusterChecksEnabled(true).
					WithClusterChecksUseCLCEnabled(true).
					BuildWithDefaults()
				_ = c.Create(context.TODO(), ddaCustom)
				return ddaCustom
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(t *testing.T, c client.Client) {
				baseDDAI := getBaseDDAI(dda)
				expectedDDAI := baseDDAI.DeepCopy()
				expectedDDAI.Annotations = map[string]string{
					constants.MD5DDAIDeploymentAnnotationKey: "5c83da6fcf791a4865951949af039537",
				}
				expectedDDAI.Spec.Features.ClusterChecks.UseClusterChecksRunners = apiutils.NewBoolPointer(true)
				expectedDDAI.Spec.Global.Credentials = &v2alpha1.DatadogCredentials{
					APISecret: &v2alpha1.SecretConfig{
						SecretName: "custom-secret",
						KeyName:    "api",
					},
					AppSecret: &v2alpha1.SecretConfig{
						SecretName: "custom-secret2",
						KeyName:    "app",
					},
				}
				expectedDDAI.Spec.Global.ClusterAgentTokenSecret = &v2alpha1.SecretConfig{
					SecretName: "foo-token",
					KeyName:    "token",
				}
				expectedDDAI.Spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.NodeAgentComponentName: {
						Labels: map[string]string{
							"custom-label": "custom-value",
							constants.MD5AgentDeploymentProviderLabelKey: "",
						},
						Annotations: map[string]string{
							"checksum/dca-token-custom-config": "0c85492446fadac292912bb6d5fc3efd",
						},
					},
					v2alpha1.ClusterAgentComponentName: {
						Annotations: map[string]string{
							"checksum/dca-token-custom-config": "0c85492446fadac292912bb6d5fc3efd",
						},
					},
					v2alpha1.ClusterChecksRunnerComponentName: {
						Annotations: map[string]string{
							"checksum/dca-token-custom-config": "0c85492446fadac292912bb6d5fc3efd",
						},
					},
				}

				verifyDDAI(t, c, []v1alpha1.DatadogAgentInternal{*expectedDDAI})
			},
		},
		{
			name:        "[ddai] Create DDAI from minimal DDA and default profile",
			ddaiEnabled: true,
			clientBuilder: fake.NewClientBuilder().
				WithStatusSubresource(&v2alpha1.DatadogAgent{}, &v1alpha1.DatadogAgentProfile{}, &v1alpha1.DatadogAgentInternal{}),
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				_ = c.Create(context.TODO(), dda)
				return dda
			},
			profilesEnabled: true,
			want:            reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr:         false,
			wantFunc: func(t *testing.T, c client.Client) {
				verifyDDAI(t, c, []v1alpha1.DatadogAgentInternal{getDefaultDDAI(dda)})
			},
		},
		{
			name:        "[ddai] Create DDAI from minimal DDA and user created profile",
			ddaiEnabled: true,
			clientBuilder: fake.NewClientBuilder().
				WithStatusSubresource(&v2alpha1.DatadogAgent{}, &v1alpha1.DatadogAgentProfile{}, &v1alpha1.DatadogAgentInternal{}).
				WithObjects(fooProfile),
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				_ = c.Create(context.TODO(), dda)
				return dda
			},
			profilesEnabled: true,
			profile:         fooProfile,
			want:            reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr:         false,
			wantFunc: func(t *testing.T, c client.Client) {
				profileDDAI := getBaseDDAI(dda)
				profileDDAI.Name = "foo-profile"
				profileDDAI.Annotations = map[string]string{
					constants.MD5DDAIDeploymentAnnotationKey: "74ddba33da89fb703cbe43718cb78e1e",
				}
				profileDDAI.Labels[constants.ProfileLabelKey] = "foo-profile"
				profileDDAI.Spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.ClusterAgentComponentName: {
						Disabled: apiutils.NewBoolPointer(true),
					},
					v2alpha1.ClusterChecksRunnerComponentName: {
						Disabled: apiutils.NewBoolPointer(true),
					},
					v2alpha1.OtelAgentGatewayComponentName: {
						Disabled: apiutils.NewBoolPointer(true),
					},
					v2alpha1.NodeAgentComponentName: {
						Name: apiutils.NewStringPointer("foo-profile-agent"),
						Labels: map[string]string{
							constants.MD5AgentDeploymentProviderLabelKey: "",
							"foo":                     "bar",
							constants.ProfileLabelKey: "foo-profile",
						},
						Affinity: &corev1.Affinity{
							NodeAffinity: &corev1.NodeAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
									NodeSelectorTerms: []corev1.NodeSelectorTerm{
										{
											MatchExpressions: []corev1.NodeSelectorRequirement{
												{
													Key:      "foo",
													Operator: corev1.NodeSelectorOpIn,
													Values:   []string{"foo-profile"},
												},
												{
													Key:      constants.ProfileLabelKey,
													Operator: corev1.NodeSelectorOpIn,
													Values:   []string{"foo-profile"},
												},
											},
										},
									},
								},
							},
							PodAntiAffinity: &corev1.PodAntiAffinity{
								RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
									{
										LabelSelector: &metav1.LabelSelector{
											MatchExpressions: []metav1.LabelSelectorRequirement{
												{
													Key:      apicommon.AgentDeploymentComponentLabelKey,
													Operator: metav1.LabelSelectorOpIn,
													Values:   []string{string(apicommon.CoreAgentContainerName)},
												},
											},
										},
										TopologyKey: "kubernetes.io/hostname",
									},
								},
							},
						},
					},
				}

				verifyDDAI(t, c, []v1alpha1.DatadogAgentInternal{getDefaultDDAI(dda), profileDDAI})
			},
		},
	}

	runTestCases(t, tests, runDDAReconcilerTest)
	runTestCases(t, tests, runFullReconcilerTest)
}

func verifyDDAI(t *testing.T, c client.Client, expectedDDAI []v1alpha1.DatadogAgentInternal) {
	ddaiList := v1alpha1.DatadogAgentInternalList{}
	err := c.List(context.TODO(), &ddaiList)
	assert.NoError(t, err, "Failed to list DatadogAgentInternal resources")
	assert.Equal(t, len(expectedDDAI), len(ddaiList.Items), "DDAI count mismatch")
	for i := range ddaiList.Items {
		// clear managed fields
		ddaiList.Items[i].ObjectMeta.ManagedFields = nil
		// type meta is only added when merging ddais
		ddaiList.Items[i].TypeMeta = metav1.TypeMeta{}
		// clear status since full reconciler is setting it
		ddaiList.Items[i].Status = v1alpha1.DatadogAgentInternalStatus{}
		// reset resource version to 1 since full reconciler is incrementing it
		ddaiList.Items[i].ObjectMeta.ResourceVersion = "1"

	}
	assert.ElementsMatch(t, expectedDDAI, ddaiList.Items, "DDAI resources don't match")
}

func getBaseDDAI(dda *v2alpha1.DatadogAgent) v1alpha1.DatadogAgentInternal {
	expectedDDAI := v1alpha1.DatadogAgentInternal{
		ObjectMeta: metav1.ObjectMeta{
			Name:            dda.Name,
			Namespace:       dda.Namespace,
			ResourceVersion: "1",
			Labels: map[string]string{
				apicommon.DatadogAgentNameLabelKey: dda.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "datadoghq.com/v2alpha1",
					Kind:               "DatadogAgent",
					Name:               dda.Name,
					Controller:         apiutils.NewBoolPointer(true),
					BlockOwnerDeletion: apiutils.NewBoolPointer(true),
				},
			},
			Finalizers: []string{constants.DatadogAgentInternalFinalizer},
		},
		Spec: v2alpha1.DatadogAgentSpec{
			Features: dda.Spec.Features,
			Global:   dda.Spec.Global,
			Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
				v2alpha1.NodeAgentComponentName: {
					Labels: map[string]string{
						constants.MD5AgentDeploymentProviderLabelKey: "",
					},
				},
			},
		},
	}

	expectedDDAI.Spec.Global.Credentials = &v2alpha1.DatadogCredentials{
		APISecret: &v2alpha1.SecretConfig{
			SecretName: "foo-secret",
			KeyName:    "api_key",
		},
		AppSecret: &v2alpha1.SecretConfig{
			SecretName: "foo-secret",
			KeyName:    "app_key",
		},
	}

	expectedDDAI.Spec.Global.ClusterAgentTokenSecret = &v2alpha1.SecretConfig{
		SecretName: "foo-token",
		KeyName:    "token",
	}

	return expectedDDAI
}

func getDefaultDDAI(dda *v2alpha1.DatadogAgent) v1alpha1.DatadogAgentInternal {
	expectedDDAI := getBaseDDAI(dda)
	expectedDDAI.Annotations = map[string]string{
		constants.MD5DDAIDeploymentAnnotationKey: "a79dfe841c72f0e71dea9cb26f3eb2a7",
	}
	expectedDDAI.Spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
		v2alpha1.NodeAgentComponentName: {
			Labels: map[string]string{
				constants.MD5AgentDeploymentProviderLabelKey: "",
			},
			Affinity: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
						NodeSelectorTerms: []corev1.NodeSelectorTerm{
							{
								MatchExpressions: []corev1.NodeSelectorRequirement{
									{
										Key:      constants.ProfileLabelKey,
										Operator: corev1.NodeSelectorOpDoesNotExist,
									},
								},
							},
						},
					},
				},
				PodAntiAffinity: &corev1.PodAntiAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchExpressions: []metav1.LabelSelectorRequirement{
									{
										Key:      apicommon.AgentDeploymentComponentLabelKey,
										Operator: metav1.LabelSelectorOpIn,
										Values:   []string{string(apicommon.CoreAgentContainerName)},
									},
								},
							},
							TopologyKey: "kubernetes.io/hostname",
						},
					},
				},
			},
		},
	}
	return expectedDDAI
}
