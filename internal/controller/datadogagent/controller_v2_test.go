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
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	common "github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/experimental"
	agenttestutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/testutils"
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
			name: "DatadogAgent with container monitoring in process agent",
			fields: fields{
				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v2alpha1.DatadogAgent{}).Build(),
				scheme:   s,
				recorder: recorder,
			},
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
					WithProcessChecksInCoreAgent(false).
					Build()
				_ = c.Create(context.TODO(), dda)
				return dda
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.ProcessAgentContainerName),
					string(apicommon.TraceAgentContainerName),
				}

				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
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

				return verifyDaemonsetNames(t, c, resourcesNamespace, dsName, expectedDaemonsets)
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
			name: "otelEnabled true, no override - full image all agents",
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

				assert.Equal(t, fmt.Sprintf("gcr.io/datadoghq/agent:%s-full", images.AgentLatestVersion), agentContainer[apicommon.CoreAgentContainerName].Image)
				assert.Equal(t, fmt.Sprintf("gcr.io/datadoghq/agent:%s-full", images.AgentLatestVersion), agentContainer[apicommon.TraceAgentContainerName].Image)
				assert.Equal(t, fmt.Sprintf("gcr.io/datadoghq/agent:%s-full", images.AgentLatestVersion), agentContainer[apicommon.OtelAgent].Image)

				return nil
			},
		},
		{
			name: "otelEnabled true, override Tag - override tag all agents",
			dda: testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
				WithOTelCollectorEnabled(true).
				WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
					Image: &v2alpha1.AgentImageConfig{
						Tag: "7.65.0-full",
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

				assert.Equal(t, "gcr.io/datadoghq/agent:7.65.0-full", agentContainer[apicommon.CoreAgentContainerName].Image)
				assert.Equal(t, "gcr.io/datadoghq/agent:7.65.0-full", agentContainer[apicommon.TraceAgentContainerName].Image)
				assert.Equal(t, "gcr.io/datadoghq/agent:7.65.0-full", agentContainer[apicommon.OtelAgent].Image)

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
		{
			name: "autopilot enabled with core-agent and process-agent",
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				autopilotKey := experimental.ExperimentalAnnotationPrefix + "/" + experimental.ExperimentalAutopilotSubkey
				dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
					WithAPMEnabled(false).
					WithLiveProcessEnabled(true).
					WithProcessChecksInCoreAgent(false).
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
					string(apicommon.ProcessAgentContainerName),
				}
				if err := verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers); err != nil {
					return err
				}

				ds := &appsv1.DaemonSet{}
				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: dsName}, ds); err != nil {
					return err
				}

				processAgentFound := false
				for _, ctn := range ds.Spec.Template.Spec.Containers {
					if ctn.Name == string(apicommon.ProcessAgentContainerName) {
						expectedCommand := []string{
							"process-agent",
							"-config=/etc/datadog-agent/datadog.yaml",
						}
						if !reflect.DeepEqual(ctn.Command, expectedCommand) {
							return fmt.Errorf("process-agent command incorrect, expected: %v, got: %v", expectedCommand, ctn.Command)
						}

						forbiddenMounts := map[string]struct{}{
							common.AuthVolumeName:            {},
							common.CriSocketVolumeName:       {},
							common.DogstatsdSocketVolumeName: {},
						}
						for _, m := range ctn.VolumeMounts {
							if _, found := forbiddenMounts[m.Name]; found {
								return fmt.Errorf("forbidden mount %s found in process-agent is not allowed in GKE Autopilot", m.Name)
							}
						}
						processAgentFound = true
					}
				}
				if !processAgentFound {
					return fmt.Errorf("process-agent container not found")
				}

				return nil
			},
		},
		{
			name: "autopilot enabled with all containers (core, trace, process)",
			loadFunc: func(c client.Client) *v2alpha1.DatadogAgent {
				autopilotKey := experimental.ExperimentalAnnotationPrefix + "/" + experimental.ExperimentalAutopilotSubkey
				dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
					WithAPMEnabled(true).
					WithLiveProcessEnabled(true).
					WithProcessChecksInCoreAgent(false).
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
					string(apicommon.ProcessAgentContainerName),
				}
				if err := verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers); err != nil {
					return err
				}

				ds := &appsv1.DaemonSet{}
				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: dsName}, ds); err != nil {
					return err
				}

				traceAgentFound := false
				processAgentFound := false
				for _, ctn := range ds.Spec.Template.Spec.Containers {
					switch ctn.Name {
					case string(apicommon.TraceAgentContainerName):
						expectedCommand := []string{
							"trace-agent",
							"-config=/etc/datadog-agent/datadog.yaml",
						}
						if !reflect.DeepEqual(ctn.Command, expectedCommand) {
							return fmt.Errorf("trace-agent command incorrect, expected: %v, got: %v", expectedCommand, ctn.Command)
						}
						traceAgentFound = true

					case string(apicommon.ProcessAgentContainerName):
						expectedCommand := []string{
							"process-agent",
							"-config=/etc/datadog-agent/datadog.yaml",
						}
						if !reflect.DeepEqual(ctn.Command, expectedCommand) {
							return fmt.Errorf("process-agent command incorrect, expected: %v, got: %v", expectedCommand, ctn.Command)
						}
						processAgentFound = true
					}
				}

				if !traceAgentFound {
					return fmt.Errorf("trace-agent container not found")
				}
				if !processAgentFound {
					return fmt.Errorf("process-agent container not found")
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

func verifyDaemonsetNames(t *testing.T, c client.Client, resourcesNamespace, dsName string, expectedDSNames []string) error {
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
