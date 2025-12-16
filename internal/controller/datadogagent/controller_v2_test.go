// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"errors"
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
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/experimental"
	agenttestutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/testutils"
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
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type fields struct {
	client       client.Client
	scheme       *runtime.Scheme
	platformInfo kubernetes.PlatformInfo
	recorder     record.EventRecorder
}

func TestReconcileDatadogAgentV2_Reconcile(t *testing.T) {
	const resourcesName = "foo"
	const resourcesNamespace = "bar"
	const dsName = "foo-agent"

	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconcileDatadogAgent_Reconcile"})
	forwarders := dummyManager{}

	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	// Register operator types with the runtime scheme.
	s := agenttestutils.TestScheme()

	defaultRequeueDuration := 15 * time.Second

	tests := []struct {
		name     string
		fields   fields
		loadFunc func(c client.Client) *v2alpha1.DatadogAgent
		want     reconcile.Result
		wantErr  bool
		wantFunc func(c client.Client) error
	}{
		{
			name: "DatadogAgent default, create Daemonset with core and trace agents",
			fields: fields{
				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v2alpha1.DatadogAgent{}).Build(),
				scheme:   s,
				recorder: recorder,
			},
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
					Build()
				_ = c.Create(context.TODO(), dda)
				return dda
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
				}

				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "DatadogAgent singleProcessContainer, create Daemonset with core and agents",
			fields: fields{
				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v2alpha1.DatadogAgent{}).Build(),
				scheme:   s,
				recorder: recorder,
			},
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
					WithSingleContainerStrategy(false).
					Build()
				_ = c.Create(context.TODO(), dda)
				return dda
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
				}

				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "[single container] DatadogAgent default, create Daemonset with a single container",
			fields: fields{
				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v2alpha1.DatadogAgent{}).Build(),
				scheme:   s,
				recorder: recorder,
			},
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
					WithSingleContainerStrategy(true).
					Build()
				_ = c.Create(context.TODO(), dda)
				return dda
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommon.UnprivilegedSingleAgentContainerName),
				}

				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "DatadogAgent with APM enabled, create Daemonset with core and process agents",
			fields: fields{
				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v2alpha1.DatadogAgent{}).Build(),
				scheme:   s,
				recorder: recorder,
			},
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
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
				}

				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "[single container] DatadogAgent with APM enabled, create Daemonset with a single container",
			fields: fields{
				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v2alpha1.DatadogAgent{}).Build(),
				scheme:   s,
				recorder: recorder,
			},
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
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommon.UnprivilegedSingleAgentContainerName),
				}

				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "DatadogAgent with APM and CWS enables, create Daemonset with four agents",
			fields: fields{
				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v2alpha1.DatadogAgent{}).Build(),
				scheme:   s,
				recorder: recorder,
			},
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
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
					string(apicommon.SystemProbeContainerName),
					string(apicommon.SecurityAgentContainerName),
				}

				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "[single container] DatadogAgent with APM and CWS enables, create Daemonset with four agents",
			fields: fields{
				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v2alpha1.DatadogAgent{}).Build(),
				scheme:   s,
				recorder: recorder,
			},
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
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
					string(apicommon.SystemProbeContainerName),
					string(apicommon.SecurityAgentContainerName),
				}

				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "DatadogAgent with APM and OOMKill enabled, create Daemonset with core, trace, and system-probe",
			fields: fields{
				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v2alpha1.DatadogAgent{}).Build(),
				scheme:   s,
				recorder: recorder,
			},
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
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
					string(apicommon.SystemProbeContainerName),
				}

				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "[single container] DatadogAgent with APM and OOMKill enabled, create Daemonset with core, trace, and system-probe",
			fields: fields{
				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v2alpha1.DatadogAgent{}).Build(),
				scheme:   s,
				recorder: recorder,
			},
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
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
					string(apicommon.SystemProbeContainerName),
				}

				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "DatadogAgent with FIPS enabled",
			fields: fields{
				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v2alpha1.DatadogAgent{}).Build(),
				scheme:   s,
				recorder: recorder,
			},
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
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
					string(apicommon.FIPSProxyContainerName),
				}

				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "DatadogAgent with PDB enabled",
			fields: fields{
				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v2alpha1.DatadogAgent{}, &policyv1.PodDisruptionBudget{}).Build(),
				scheme:   s,
				recorder: recorder,
			},
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
			wantFunc: func(c client.Client) error {
				return verifyPDB(t, c)
			},
		},
		{
			name: "DatadogAgent with override.nodeAgent.disabled true",
			fields: fields{
				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v2alpha1.DatadogAgent{}).Build(),
				scheme:   s,
				recorder: recorder,
			},
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
			wantFunc: func(c client.Client) error {
				ds := &appsv1.DaemonSet{}
				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: dsName}, ds); client.IgnoreNotFound(err) != nil {
					return err
				}
				return nil
			},
		},
		{
			name: "DCA status and condition set",
			fields: fields{
				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v2alpha1.DatadogAgent{}).Build(),
				scheme:   s,
				recorder: recorder,
			},
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
			wantFunc: func(c client.Client) error {
				dda := &v2alpha1.DatadogAgent{}
				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: resourcesName}, dda); client.IgnoreNotFound(err) != nil {
					return err
				}
				// assert.Equal(t, "token", dda.Status.ClusterAgent.GeneratedToken)
				assert.NotNil(t, dda.Status.ClusterAgent, "DCA status should be set")
				dcaCondition := condition.GetCondition(&dda.Status, common.ClusterAgentReconcileConditionType)
				assert.True(t, dcaCondition.Status == metav1.ConditionTrue && dcaCondition.Reason == "reconcile_succeed", "DCA status condition should be set")
				return nil
			},
		},
		{
			name: "DCA status condition should be deleted when disabled",
			fields: fields{
				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v2alpha1.DatadogAgent{}).Build(),
				scheme:   s,
				recorder: recorder,
			},
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
			wantFunc: func(c client.Client) error {
				dda := &v2alpha1.DatadogAgent{}
				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: resourcesName}, dda); client.IgnoreNotFound(err) != nil {
					return err
				}
				// assert.Equal(t, "token", dda.Status.ClusterAgent.GeneratedToken)
				assert.Nil(t, dda.Status.ClusterAgent, "DCA status should be nil when cleaned up")
				assert.Nil(t, condition.GetCondition(&dda.Status, common.ClusterAgentReconcileConditionType), "DCA status condition should be nil when cleaned up")
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{
				client:       tt.fields.client,
				scheme:       tt.fields.scheme,
				platformInfo: tt.fields.platformInfo,
				recorder:     recorder,
				log:          logf.Log.WithName(tt.name),
				forwarders:   forwarders,
				options: ReconcilerOptions{
					ExtendedDaemonsetOptions: componentagent.ExtendedDaemonsetOptions{
						Enabled: false,
					},
					SupportCilium: false,
				},
			}
			r.initializeComponentRegistry()

			var dda *v2alpha1.DatadogAgent
			if tt.loadFunc != nil {
				dda = tt.loadFunc(r.client)
			}
			got, err := r.Reconcile(context.TODO(), dda)
			if tt.wantErr {
				assert.Error(t, err, "ReconcileDatadogAgent.Reconcile() expected an error")
			} else {
				assert.NoError(t, err, "ReconcileDatadogAgent.Reconcile() unexpected error: %v", err)
			}

			assert.Equal(t, tt.want, got, "ReconcileDatadogAgent.Reconcile() unexpected result")

			if tt.wantFunc != nil {
				err := tt.wantFunc(r.client)
				assert.NoError(t, err, "ReconcileDatadogAgent.Reconcile() wantFunc validation error: %v", err)
			}
		})
	}
}

func Test_Introspection(t *testing.T) {
	const resourcesName = "foo"
	const resourcesNamespace = "bar"

	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconcileDatadogAgent_Reconcile"})
	forwarders := dummyManager{}

	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	// Register operator types with the runtime scheme.
	s := agenttestutils.TestScheme()

	defaultRequeueDuration := 15 * time.Second

	tests := []struct {
		name     string
		fields   fields
		loadFunc func(c client.Client) *v2alpha1.DatadogAgent
		nodes    []client.Object
		want     reconcile.Result
		wantErr  bool
		wantFunc func(t *testing.T, c client.Client) error
	}{
		{
			name: "[introspection] Daemonset names with affinity override",
			fields: fields{
				scheme:   s,
				recorder: recorder,
			},
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
			wantFunc: func(t *testing.T, c client.Client) error {
				expectedDaemonsets := []string{
					string("foo-agent-default"),
					string("foo-agent-gke-cos"),
				}

				return verifyDaemonsetNames(t, c, resourcesNamespace, expectedDaemonsets)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{
				client:       fake.NewClientBuilder().WithStatusSubresource(&corev1.Node{}, &v2alpha1.DatadogAgent{}).WithObjects(tt.nodes...).Build(),
				scheme:       tt.fields.scheme,
				platformInfo: tt.fields.platformInfo,
				recorder:     recorder,
				log:          logf.Log.WithName(tt.name),
				forwarders:   forwarders,
				options: ReconcilerOptions{
					ExtendedDaemonsetOptions: componentagent.ExtendedDaemonsetOptions{
						Enabled: false,
					},
					SupportCilium:        false,
					IntrospectionEnabled: true,
				},
			}
			r.initializeComponentRegistry()

			var dda *v2alpha1.DatadogAgent
			if tt.loadFunc != nil {
				dda = tt.loadFunc(r.client)
			}
			got, err := r.Reconcile(context.TODO(), dda)
			if tt.wantErr {
				assert.Error(t, err, "ReconcileDatadogAgent.Reconcile() expected an error")
			} else {
				assert.NoError(t, err, "ReconcileDatadogAgent.Reconcile() unexpected error: %v", err)
			}

			assert.Equal(t, tt.want, got, "ReconcileDatadogAgent.Reconcile() unexpected result")

			if tt.wantFunc != nil {
				err := tt.wantFunc(t, r.client)
				assert.NoError(t, err, "ReconcileDatadogAgent.Reconcile() wantFunc validation error: %v", err)
			}
		})
	}
}

func Test_otelImageTags(t *testing.T) {
	const resourcesName = "foo"
	const resourcesNamespace = "bar"
	const dsName = "foo-agent"

	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	defaultRequeueDuration := 15 * time.Second

	tests := []struct {
		name     string
		fields   fields
		dda      *v2alpha1.DatadogAgent
		wantFunc func(c client.Client) error
	}{
		{
			name: "otelEnabled true, no override",
			dda: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithOTelCollectorEnabled(true).
				Build(),
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
					string(apicommon.OtelAgent),
				}
				assert.NoError(t, verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers))
				agentContainer := getDsContainers(c, resourcesNamespace, dsName)

				assert.Equal(t, fmt.Sprintf("gcr.io/datadoghq/agent:%s", images.AgentLatestVersion), agentContainer[apicommon.CoreAgentContainerName].Image)
				assert.Equal(t, fmt.Sprintf("gcr.io/datadoghq/agent:%s", images.AgentLatestVersion), agentContainer[apicommon.TraceAgentContainerName].Image)
				assert.Equal(t, fmt.Sprintf("gcr.io/datadoghq/ddot-collector:%s", images.AgentLatestVersion), agentContainer[apicommon.OtelAgent].Image)

				return nil
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
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
					string(apicommon.OtelAgent),
				}
				assert.NoError(t, verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers))
				agentContainer := getDsContainers(c, resourcesNamespace, dsName)

				assert.Equal(t, "gcr.io/datadoghq/agent:7.71.0", agentContainer[apicommon.CoreAgentContainerName].Image)
				assert.Equal(t, "gcr.io/datadoghq/agent:7.71.0", agentContainer[apicommon.TraceAgentContainerName].Image)
				assert.Equal(t, "gcr.io/datadoghq/ddot-collector:7.71.0", agentContainer[apicommon.OtelAgent].Image)

				return nil
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
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
					string(apicommon.OtelAgent),
				}
				assert.Error(t, errors.New("Incompatible OTel Agent image"), verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers))
				return nil
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
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
					string(apicommon.OtelAgent),
				}
				assert.Error(t, errors.New("Incompatible OTel Agent image"), verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers))
				return nil
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
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
					string(apicommon.OtelAgent),
				}
				assert.NoError(t, verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers))
				agentContainer := getDsContainers(c, resourcesNamespace, dsName)

				assert.Equal(t, "gcr.io/datadoghq/testagent:7.65.0-full", agentContainer[apicommon.CoreAgentContainerName].Image)
				assert.Equal(t, "gcr.io/datadoghq/testagent:7.65.0-full", agentContainer[apicommon.TraceAgentContainerName].Image)
				assert.Equal(t, "gcr.io/datadoghq/testagent:7.65.0-full", agentContainer[apicommon.OtelAgent].Image)

				return nil
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
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
					string(apicommon.OtelAgent),
				}
				assert.NoError(t, verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers))
				agentContainer := getDsContainers(c, resourcesNamespace, dsName)

				assert.Equal(t, "gcr.io/datadoghq/testagent:7.65.0-full", agentContainer[apicommon.CoreAgentContainerName].Image)
				assert.Equal(t, "gcr.io/datadoghq/testagent:7.65.0-full", agentContainer[apicommon.TraceAgentContainerName].Image)
				assert.Equal(t, "gcr.io/datadoghq/testagent:7.65.0-full", agentContainer[apicommon.OtelAgent].Image)

				return nil
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
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
					string(apicommon.OtelAgent),
				}
				assert.NoError(t, verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers))
				agentContainer := getDsContainers(c, resourcesNamespace, dsName)

				assert.Equal(t, "gcr.io/datacat/testagent:latest", agentContainer[apicommon.CoreAgentContainerName].Image)
				assert.Equal(t, "gcr.io/datacat/testagent:latest", agentContainer[apicommon.TraceAgentContainerName].Image)
				assert.Equal(t, "gcr.io/datacat/testagent:latest", agentContainer[apicommon.OtelAgent].Image)

				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eventBroadcaster := record.NewBroadcaster()
			recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconcileDatadogAgent_Reconcile"})
			forwarders := dummyManager{}
			s := agenttestutils.TestScheme()

			client := fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v2alpha1.DatadogAgent{}).Build()
			// Register operator types with the runtime scheme.

			r := &Reconciler{
				client:       client,
				scheme:       s,
				platformInfo: kubernetes.PlatformInfo{},
				recorder:     recorder,
				log:          logf.Log.WithName(tt.name),
				forwarders:   forwarders,
				options: ReconcilerOptions{
					ExtendedDaemonsetOptions: componentagent.ExtendedDaemonsetOptions{
						Enabled: false,
					},
					SupportCilium: false,
				},
			}
			r.initializeComponentRegistry()

			client.Create(context.TODO(), tt.dda)

			got, err := r.Reconcile(context.TODO(), tt.dda)

			assert.NoError(t, err, "ReconcileDatadogAgent.Reconcile() unexpected error: %v", err)
			assert.Equal(t, reconcile.Result{RequeueAfter: defaultRequeueDuration}, got, "ReconcileDatadogAgent.Reconcile() unexpected result")

			if tt.wantFunc != nil {
				err := tt.wantFunc(r.client)
				assert.NoError(t, err, "ReconcileDatadogAgent.Reconcile() wantFunc validation error: %v", err)
			}
		})
	}
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

	tests := []struct {
		name     string
		loadFunc func(client.Client) *v2alpha1.DatadogAgent
		wantFunc func(t *testing.T, c client.Client) error
	}{
		{
			name: "autopilot enabled with core-agent only",
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
			wantFunc: func(t *testing.T, c client.Client) error {
				expectedContainers := []string{string(apicommon.CoreAgentContainerName)}
				if err := verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers); err != nil {
					return err
				}

				ds := &appsv1.DaemonSet{}
				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: dsName}, ds); err != nil {
					return err
				}

				forbiddenVolumes := map[string]struct{}{
					common.AuthVolumeName:            {},
					common.CriSocketVolumeName:       {},
					common.DogstatsdSocketVolumeName: {},
					common.APMSocketVolumeName:       {},
				}
				for _, v := range ds.Spec.Template.Spec.Volumes {
					if _, found := forbiddenVolumes[v.Name]; found {
						return fmt.Errorf("forbidden volume %s is not allowed in GKE Autopilot", v.Name)
					}
				}

				initVolumePatchFound := false
				for _, ic := range ds.Spec.Template.Spec.InitContainers {
					if ic.Name == "init-volume" {
						if len(ic.Args) != 1 || ic.Args[0] != "cp -r /etc/datadog-agent /opt" {
							return fmt.Errorf("init-volume args not patched correctly, got: %v", ic.Args)
						}
						initVolumePatchFound = true
					}

					forbiddenMounts := map[string]struct{}{
						common.AuthVolumeName:      {},
						common.CriSocketVolumeName: {},
					}
					for _, m := range ic.VolumeMounts {
						if _, found := forbiddenMounts[m.Name]; found {
							return fmt.Errorf("forbidden mount %s in init container %s is not allowed in GKE Autopilot", m.Name, ic.Name)
						}
					}
				}
				if !initVolumePatchFound {
					return fmt.Errorf("init-volume container not found or not patched")
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
								return fmt.Errorf("forbidden mount %s found in core agent is not allowed in GKE Autopilot", m.Name)
							}
						}
					}
				}

				return nil
			},
		},
		{
			name: "autopilot enabled with core-agent and trace-agent",
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
			wantFunc: func(t *testing.T, c client.Client) error {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.TraceAgentContainerName),
				}
				if err := verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers); err != nil {
					return err
				}

				ds := &appsv1.DaemonSet{}
				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: dsName}, ds); err != nil {
					return err
				}

				traceAgentFound := false
				for _, ctn := range ds.Spec.Template.Spec.Containers {
					if ctn.Name == string(apicommon.TraceAgentContainerName) {
						expectedCommand := []string{
							"trace-agent",
							"-config=/etc/datadog-agent/datadog.yaml",
						}
						if !reflect.DeepEqual(ctn.Command, expectedCommand) {
							return fmt.Errorf("trace-agent command incorrect, expected: %v, got: %v", expectedCommand, ctn.Command)
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
								return fmt.Errorf("forbidden mount %s should be removed from trace-agent", m.Name)
							}
						}
						traceAgentFound = true
					}
				}
				if !traceAgentFound {
					return fmt.Errorf("trace-agent container not found")
				}

				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := agenttestutils.TestScheme()
			broadcaster := record.NewBroadcaster()
			rec := broadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{})
			fakeClient := fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v2alpha1.DatadogAgent{}).Build()
			r := &Reconciler{client: fakeClient, scheme: s, recorder: rec}
			r.initializeComponentRegistry()

			var dda *v2alpha1.DatadogAgent
			if tt.loadFunc != nil {
				dda = tt.loadFunc(fakeClient)
			}

			res, err := r.Reconcile(context.TODO(), dda)
			assert.NoError(t, err)
			assert.Equal(t, 15*time.Second, res.RequeueAfter)

			if tt.wantFunc != nil {
				err := tt.wantFunc(t, fakeClient)
				assert.NoError(t, err, "Test validation failed: %v", err)
			}
		})
	}
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

	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconcileDatadogAgent_Reconcile"})
	forwarders := dummyManager{}

	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	// Register operator types with the runtime scheme.
	s := agenttestutils.TestScheme()

	defaultRequeueDuration := 15 * time.Second

	tests := []struct {
		name     string
		fields   fields
		loadFunc func(c client.Client) *v2alpha1.DatadogAgent
		nodes    []client.Object
		want     reconcile.Result
		wantErr  bool
		wantFunc func(t *testing.T, c client.Client) error
	}{
		{
			name: "[introspection] Control Plane Monitoring for Openshift",
			fields: fields{
				scheme:   s,
				recorder: recorder,
			},
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
			wantFunc: func(t *testing.T, c client.Client) error {
				if err := verifyDCADeployment(t, c, resourcesName, resourcesNamespace, dcaName, "openshift"); err != nil {
					return err
				}
				expectedDaemonsets := []string{
					dsName,
				}
				if err := verifyDaemonsetNames(t, c, resourcesNamespace, expectedDaemonsets); err != nil {
					return err
				}
				return verifyEtcdMountsOpenshift(t, c, resourcesNamespace, dsName, "openshift")
			},
		},
		{
			name: "[introspection] Control Plane Monitoring with EKS",
			fields: fields{
				scheme:   s,
				recorder: recorder,
			},
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
			wantFunc: func(t *testing.T, c client.Client) error {
				if err := verifyDCADeployment(t, c, resourcesName, resourcesNamespace, dcaName, "eks"); err != nil {
					return err
				}
				expectedDaemonsets := []string{
					dsName,
				}
				return verifyDaemonsetNames(t, c, resourcesNamespace, expectedDaemonsets)
			},
		},
		{
			name: "[introspection] Control Plane Monitoring with multiple providers",
			fields: fields{
				scheme:   s,
				recorder: recorder,
			},
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
			wantFunc: func(t *testing.T, c client.Client) error {
				if err := verifyDCADeployment(t, c, resourcesName, resourcesNamespace, dcaName, "default"); err != nil {
					return err
				}
				expectedDaemonsets := []string{
					dsName,
				}
				return verifyDaemonsetNames(t, c, resourcesNamespace, expectedDaemonsets)
			},
		},
		{
			// This test verifies that when a node has a GKE provider label with an unsupported OS value,
			// the system falls back to the "default" provider for control plane monitoring
			name: "[introspection] Control Plane Monitoring with unsupported provider",
			fields: fields{
				scheme:   s,
				recorder: recorder,
			},
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
			wantFunc: func(t *testing.T, c client.Client) error {
				if err := verifyDCADeployment(t, c, resourcesName, resourcesNamespace, dcaName, "default"); err != nil {
					return err
				}
				expectedDaemonsets := []string{
					dsName,
				}
				return verifyDaemonsetNames(t, c, resourcesNamespace, expectedDaemonsets)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{
				client:       fake.NewClientBuilder().WithStatusSubresource(&corev1.Node{}, &v2alpha1.DatadogAgent{}).WithObjects(tt.nodes...).Build(),
				scheme:       tt.fields.scheme,
				platformInfo: tt.fields.platformInfo,
				recorder:     recorder,
				log:          logf.Log.WithName(tt.name),
				forwarders:   forwarders,
				options: ReconcilerOptions{
					ExtendedDaemonsetOptions: componentagent.ExtendedDaemonsetOptions{
						Enabled: false,
					},
					SupportCilium:        false,
					IntrospectionEnabled: true,
				},
			}
			r.initializeComponentRegistry()

			var dda *v2alpha1.DatadogAgent
			if tt.loadFunc != nil {
				dda = tt.loadFunc(r.client)
			}
			got, err := r.Reconcile(context.TODO(), dda)
			if tt.wantErr {
				assert.Error(t, err, "ReconcileDatadogAgent.Reconcile() expected an error")
			} else {
				assert.NoError(t, err, "ReconcileDatadogAgent.Reconcile() unexpected error: %v", err)
			}

			assert.Equal(t, tt.want, got, "ReconcileDatadogAgent.Reconcile() unexpected result")

			if tt.wantFunc != nil {
				err := tt.wantFunc(t, r.client)
				assert.NoError(t, err, "ReconcileDatadogAgent.Reconcile() wantFunc validation error: %v", err)
			}
		})
	}
}

func verifyDCADeployment(t *testing.T, c client.Client, ddaName, resourcesNamespace, expectedName string, provider string) error {
	deploymentList := appsv1.DeploymentList{}
	if err := c.List(context.TODO(), &deploymentList, client.HasLabels{constants.MD5AgentDeploymentProviderLabelKey}); err != nil {
		return err
	}
	assert.Equal(t, 1, len(deploymentList.Items))
	assert.Equal(t, expectedName, deploymentList.Items[0].ObjectMeta.Name)

	cms := corev1.ConfigMapList{}
	if err := c.List(context.TODO(), &cms, client.InNamespace(resourcesNamespace)); err != nil {
		return err
	}

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

		if err := verifyCheckMounts(t, dcaDeployment, provider, "kube-apiserver-metrics"); err != nil {
			return err
		}
		if err := verifyCheckMounts(t, dcaDeployment, provider, "kube-controller-manager"); err != nil {
			return err
		}
		if err := verifyCheckMounts(t, dcaDeployment, provider, "kube-scheduler"); err != nil {
			return err
		}
	}
	if provider == kubernetes.OpenshiftProvider {
		if err := verifyCheckMounts(t, dcaDeployment, provider, "etcd"); err != nil {
			return err
		}
	}
	return nil
}

func verifyCheckMounts(t *testing.T, dcaDeployment appsv1.Deployment, provider string, checkName string) error {
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
	return nil
}

func verifyDaemonsetContainers(c client.Client, resourcesNamespace, dsName string, expectedContainers []string) error {
	ds := &appsv1.DaemonSet{}
	if err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: dsName}, ds); err != nil {
		return err
	}
	dsContainers := []string{}
	for _, container := range ds.Spec.Template.Spec.Containers {
		dsContainers = append(dsContainers, container.Name)
	}

	sort.Strings(dsContainers)
	sort.Strings(expectedContainers)
	if reflect.DeepEqual(expectedContainers, dsContainers) {
		return nil
	} else {
		return fmt.Errorf("Container don't match, expected %s, actual %s", expectedContainers, dsContainers)
	}
}

func verifyDaemonsetNames(t *testing.T, c client.Client, resourcesNamespace string, expectedDSNames []string) error {
	daemonSetList := appsv1.DaemonSetList{}
	if err := c.List(context.TODO(), &daemonSetList, client.HasLabels{constants.MD5AgentDeploymentProviderLabelKey}); err != nil {
		return err
	}

	actualDSNames := []string{}
	for _, ds := range daemonSetList.Items {
		actualDSNames = append(actualDSNames, ds.Name)
	}
	sort.Strings(actualDSNames)
	sort.Strings(expectedDSNames)
	assert.Equal(t, expectedDSNames, actualDSNames)
	return nil
}

func verifyEtcdMountsOpenshift(t *testing.T, c client.Client, resourcesNamespace, dsName string, provider string) error {
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
	if err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: dsName}, ds); err != nil {
		return err
	}

	var coreAgentContainer *corev1.Container
	for _, container := range ds.Spec.Template.Spec.Containers {
		if container.Name == string(apicommon.CoreAgentContainerName) {
			coreAgentContainer = &container
			break
		}
	}

	if coreAgentContainer == nil {
		return fmt.Errorf("core agent container not found in DaemonSet %s", dsName)
	}

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
	if err := c.List(context.TODO(), &deploymentList, client.InNamespace(resourcesNamespace)); err != nil {
		return err
	}

	var ccrDeployment *appsv1.Deployment
	for _, deployment := range deploymentList.Items {
		if deployment.Name == "foo-cluster-checks-runner" {
			ccrDeployment = &deployment
			break
		}
	}

	if ccrDeployment == nil {
		return fmt.Errorf("cluster-checks-runner deployment not found")
	}

	var ccrContainer *corev1.Container
	for _, container := range ccrDeployment.Spec.Template.Spec.Containers {
		if container.Name == string(apicommon.ClusterChecksRunnersContainerName) {
			ccrContainer = &container
			break
		}
	}

	if ccrContainer == nil {
		return fmt.Errorf("cluster-checks-runner container not found in deployment")
	}

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

	return nil
}

func verifyPDB(t *testing.T, c client.Client) error {
	pdbList := policyv1.PodDisruptionBudgetList{}
	if err := c.List(context.TODO(), &pdbList); err != nil {
		return err
	}
	assert.True(t, len(pdbList.Items) == 2)

	dcaPDB := pdbList.Items[0]
	assert.Equal(t, "foo-cluster-agent-pdb", dcaPDB.Name)
	assert.Equal(t, intstr.FromInt(1), *dcaPDB.Spec.MinAvailable)
	assert.Nil(t, dcaPDB.Spec.MaxUnavailable)

	ccrPDB := pdbList.Items[1]
	assert.Equal(t, "foo-cluster-checks-runner-pdb", ccrPDB.Name)
	assert.Equal(t, intstr.FromInt(1), *ccrPDB.Spec.MaxUnavailable)
	assert.Nil(t, ccrPDB.Spec.MinAvailable)
	return nil
}

func Test_DDAI_ReconcileV3(t *testing.T) {
	const resourcesName = "foo"
	const resourcesNamespace = "bar"

	// Register operator types with the runtime scheme.
	s := agenttestutils.TestScheme()
	// Load CRD from config folder
	crd, err := getDDAICRDFromConfig(s)
	assert.NoError(t, err)
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(s, corev1.EventSource{Component: "Test_DDAI_ReconcileV3"})

	forwarders := dummyManager{}
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	defaultRequeueDuration := 15 * time.Second

	dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).BuildWithDefaults()

	tests := []struct {
		name            string
		profilesEnabled bool
		profile         *v1alpha1.DatadogAgentProfile
		loadFunc        func(c client.Client) *v2alpha1.DatadogAgent
		want            reconcile.Result
		wantErr         bool
		wantFunc        func(t *testing.T, c client.Client) error
	}{
		{
			name: "[ddai] Create DDAI from minimal DDA",
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				_ = c.Create(context.TODO(), dda)
				return dda
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(t *testing.T, c client.Client) error {
				expectedDDAI := getBaseDDAI(dda)
				expectedDDAI.Annotations = map[string]string{
					constants.MD5DDAIDeploymentAnnotationKey: "c7280f85b8590dcaa3668ea3b789053e",
				}

				return verifyDDAI(t, c, []v1alpha1.DatadogAgentInternal{expectedDDAI})
			},
		},
		{
			name: "[ddai] Create DDAI from customized DDA",
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
			wantFunc: func(t *testing.T, c client.Client) error {
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

				return verifyDDAI(t, c, []v1alpha1.DatadogAgentInternal{*expectedDDAI})
			},
		},
		{
			name: "[ddai] Create DDAI from minimal DDA and default profile",
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				_ = c.Create(context.TODO(), dda)
				return dda
			},
			profilesEnabled: true,
			want:            reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr:         false,
			wantFunc: func(t *testing.T, c client.Client) error {
				return verifyDDAI(t, c, []v1alpha1.DatadogAgentInternal{getDefaultDDAI(dda)})
			},
		},
		{
			name: "[ddai] Create DDAI from minimal DDA and user created profile",
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				_ = c.Create(context.TODO(), dda)
				return dda
			},
			profilesEnabled: true,
			profile: &v1alpha1.DatadogAgentProfile{
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
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(t *testing.T, c client.Client) error {
				profileDDAI := getBaseDDAI(dda)
				profileDDAI.Name = "foo-profile"
				profileDDAI.Annotations = map[string]string{
					constants.MD5DDAIDeploymentAnnotationKey: "cc45afac2d101aad1984d1e05c2fc592",
				}
				profileDDAI.Labels[constants.ProfileLabelKey] = "foo-profile"
				profileDDAI.Spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.ClusterAgentComponentName: {
						Disabled: apiutils.NewBoolPointer(true),
					},
					v2alpha1.ClusterChecksRunnerComponentName: {
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

				return verifyDDAI(t, c, []v1alpha1.DatadogAgentInternal{getDefaultDDAI(dda), profileDDAI})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs := []client.Object{crd}
			if tt.profile != nil {
				objs = append(objs, tt.profile)
			}
			r := &Reconciler{
				client:     fake.NewClientBuilder().WithStatusSubresource(&v2alpha1.DatadogAgent{}, &v1alpha1.DatadogAgentProfile{}, &v1alpha1.DatadogAgentInternal{}).WithObjects(objs...).Build(),
				scheme:     s,
				recorder:   recorder,
				log:        logf.Log.WithName(tt.name),
				forwarders: forwarders,
				options: ReconcilerOptions{
					DatadogAgentInternalEnabled: true,
					DatadogAgentProfileEnabled:  tt.profilesEnabled,
				},
			}
			r.initializeComponentRegistry()

			var dda *v2alpha1.DatadogAgent
			if tt.loadFunc != nil {
				dda = tt.loadFunc(r.client)
			}

			got, err := r.Reconcile(context.TODO(), dda)
			if tt.wantErr {
				assert.Error(t, err, "ReconcileDatadogAgent.Reconcile() expected an error")
			} else {
				assert.NoError(t, err, "ReconcileDatadogAgent.Reconcile() unexpected error: %v", err)
			}

			assert.Equal(t, tt.want, got, "ReconcileDatadogAgent.Reconcile() unexpected result")

			if tt.wantFunc != nil {
				err := tt.wantFunc(t, r.client)
				assert.NoError(t, err, "ReconcileDatadogAgent.Reconcile() wantFunc validation error: %v", err)
			}
		})
	}
}

func verifyDDAI(t *testing.T, c client.Client, expectedDDAI []v1alpha1.DatadogAgentInternal) error {
	ddaiList := v1alpha1.DatadogAgentInternalList{}
	if err := c.List(context.TODO(), &ddaiList); err != nil {
		return err
	}
	assert.Equal(t, len(expectedDDAI), len(ddaiList.Items))
	for i := range ddaiList.Items {
		// clear managed fields
		ddaiList.Items[i].ObjectMeta.ManagedFields = nil
		// type meta is only added when merging ddais
		ddaiList.Items[i].TypeMeta = metav1.TypeMeta{}
	}
	assert.ElementsMatch(t, expectedDDAI, ddaiList.Items)
	return nil
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
