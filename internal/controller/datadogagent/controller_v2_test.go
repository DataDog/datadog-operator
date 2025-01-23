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
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	agenttestutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/testutils"
	"github.com/DataDog/datadog-operator/pkg/constants"
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
type args struct {
	request  reconcile.Request
	loadFunc func(c client.Client)
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
		args     args
		want     reconcile.Result
		wantErr  bool
		wantFunc func(c client.Client) error
	}{
		{
			name: "DatadogAgent default, create Daemonset with core, trace and process agents",
			fields: fields{
				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v2alpha1.DatadogAgent{}).Build(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
						Build()
					_ = c.Create(context.TODO(), dda)
				},
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
			name: "DatadogAgent singleProcessContainer, create Daemonset with core, trace and process agents",
			fields: fields{
				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v2alpha1.DatadogAgent{}).Build(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
						WithSingleContainerStrategy(false).
						Build()
					_ = c.Create(context.TODO(), dda)
				},
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
			name: "[single container] DatadogAgent default, create Daemonset with a single container",
			fields: fields{
				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v2alpha1.DatadogAgent{}).Build(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
						WithSingleContainerStrategy(true).
						Build()
					_ = c.Create(context.TODO(), dda)
				},
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
			name: "DatadogAgent with APM enabled, create Daemonset with core, trace and process agents",
			fields: fields{
				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v2alpha1.DatadogAgent{}).Build(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
						WithAPMEnabled(true).
						WithSingleContainerStrategy(false).
						Build()
					_ = c.Create(context.TODO(), dda)
				},
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
			name: "[single container] DatadogAgent with APM enabled, create Daemonset with a single container",
			fields: fields{
				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v2alpha1.DatadogAgent{}).Build(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
						WithAPMEnabled(true).
						WithSingleContainerStrategy(true).
						Build()
					_ = c.Create(context.TODO(), dda)
				},
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
			name: "DatadogAgent with APM and CWS enables, create Daemonset with all five agents",
			fields: fields{
				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v2alpha1.DatadogAgent{}).Build(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
						WithAPMEnabled(true).
						WithCWSEnabled(true).
						WithSingleContainerStrategy(false).
						Build()
					_ = c.Create(context.TODO(), dda)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.ProcessAgentContainerName),
					string(apicommon.TraceAgentContainerName),
					string(apicommon.SystemProbeContainerName),
					string(apicommon.SecurityAgentContainerName),
				}

				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "[single container] DatadogAgent with APM and CWS enables, create Daemonset with all five agents",
			fields: fields{
				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v2alpha1.DatadogAgent{}).Build(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
						WithAPMEnabled(true).
						WithCWSEnabled(true).
						WithSingleContainerStrategy(true).
						Build()

					_ = c.Create(context.TODO(), dda)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.ProcessAgentContainerName),
					string(apicommon.TraceAgentContainerName),
					string(apicommon.SystemProbeContainerName),
					string(apicommon.SecurityAgentContainerName),
				}

				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "DatadogAgent with APM and OOMKill enabled, create Daemonset with core, trace, process and system-probe",
			fields: fields{
				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v2alpha1.DatadogAgent{}).Build(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
						WithAPMEnabled(true).
						WithOOMKillEnabled(true).
						WithSingleContainerStrategy(false).
						Build()
					_ = c.Create(context.TODO(), dda)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.ProcessAgentContainerName),
					string(apicommon.TraceAgentContainerName),
					string(apicommon.SystemProbeContainerName),
				}

				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "[single container] DatadogAgent with APM and OOMKill enabled, create Daemonset with core, trace, process and system-probe",
			fields: fields{
				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v2alpha1.DatadogAgent{}).Build(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
						WithAPMEnabled(true).
						WithOOMKillEnabled(true).
						WithSingleContainerStrategy(true).
						Build()
					_ = c.Create(context.TODO(), dda)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.ProcessAgentContainerName),
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
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					fipsConfig := v2alpha1.FIPSConfig{
						Enabled: apiutils.NewBoolPointer(true),
					}
					dda := testutils.NewInitializedDatadogAgentBuilder(resourcesNamespace, resourcesName).
						WithFIPS(fipsConfig).
						Build()
					_ = c.Create(context.TODO(), dda)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommon.CoreAgentContainerName),
					string(apicommon.ProcessAgentContainerName),
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
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
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
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				return verifyPDB(t, c)
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

			if tt.args.loadFunc != nil {
				tt.args.loadFunc(r.client)
			}
			got, err := r.Reconcile(context.TODO(), tt.args.request)
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
		args     args
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
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
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
				},
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

			if tt.args.loadFunc != nil {
				tt.args.loadFunc(r.client)
			}
			got, err := r.Reconcile(context.TODO(), tt.args.request)
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
