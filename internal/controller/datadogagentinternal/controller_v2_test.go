// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

// TODO: re-enable after investigation

// import (
// 	"context"
// 	"fmt"
// 	"reflect"
// 	"sort"
// 	"testing"
// 	"time"

// 	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
// 	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
// 	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
// 	apiutils "github.com/DataDog/datadog-operator/api/utils"
// 	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
// 	agenttestutils "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/testutils"
// 	"github.com/DataDog/datadog-operator/pkg/kubernetes"
// 	"github.com/DataDog/datadog-operator/pkg/testutils"

// 	assert "github.com/stretchr/testify/require"
// 	appsv1 "k8s.io/api/apps/v1"
// 	corev1 "k8s.io/api/core/v1"
// 	policyv1 "k8s.io/api/policy/v1"
// 	"k8s.io/apimachinery/pkg/runtime"
// 	"k8s.io/apimachinery/pkg/types"
// 	"k8s.io/apimachinery/pkg/util/intstr"
// 	"k8s.io/client-go/kubernetes/scheme"
// 	"k8s.io/client-go/tools/record"
// 	"sigs.k8s.io/controller-runtime/pkg/client"
// 	"sigs.k8s.io/controller-runtime/pkg/client/fake"
// 	logf "sigs.k8s.io/controller-runtime/pkg/log"
// 	"sigs.k8s.io/controller-runtime/pkg/log/zap"
// 	"sigs.k8s.io/controller-runtime/pkg/reconcile"
// )

// type fields struct {
// 	client       client.Client
// 	scheme       *runtime.Scheme
// 	platformInfo kubernetes.PlatformInfo
// 	recorder     record.EventRecorder
// }

// func TestReconcileDatadogAgentV2_Reconcile(t *testing.T) {
// 	const resourcesName = "foo"
// 	const resourcesNamespace = "bar"
// 	const dsName = "foo-agent"

// 	eventBroadcaster := record.NewBroadcaster()
// 	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconcileDatadogAgent_Reconcile"})
// 	forwarders := dummyManager{}

// 	logf.SetLogger(zap.New(zap.UseDevMode(true)))

// 	// Register operator types with the runtime scheme.
// 	s := agenttestutils.TestScheme()

// 	defaultRequeueDuration := 15 * time.Second

// 	tests := []struct {
// 		name     string
// 		fields   fields
// 		loadFunc func(c client.Client) *v1alpha1.DatadogAgentInternal
// 		want     reconcile.Result
// 		wantErr  bool
// 		wantFunc func(c client.Client) error
// 	}{
// 		{
// 			name: "DatadogAgent default, create Daemonset with core and trace agents",
// 			fields: fields{
// 				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v1alpha1.DatadogAgentInternal{}).Build(),
// 				scheme:   s,
// 				recorder: recorder,
// 			},
// 			loadFunc: func(c client.Client) *v1alpha1.DatadogAgentInternal {
// 				ddai := testutils.NewInitializedDatadogAgentInternalBuilder(resourcesNamespace, resourcesName).
// 					Build()
// 				_ = c.Create(context.TODO(), ddai)
// 				return ddai
// 			},
// 			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
// 			wantErr: false,
// 			wantFunc: func(c client.Client) error {
// 				expectedContainers := []string{
// 					string(apicommon.CoreAgentContainerName),
// 					string(apicommon.TraceAgentContainerName),
// 				}

// 				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
// 			},
// 		},
// 		{
// 			name: "DatadogAgent singleProcessContainer, create Daemonset with core and agents",
// 			fields: fields{
// 				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v1alpha1.DatadogAgentInternal{}).Build(),
// 				scheme:   s,
// 				recorder: recorder,
// 			},
// 			loadFunc: func(c client.Client) *v1alpha1.DatadogAgentInternal {
// 				ddai := testutils.NewInitializedDatadogAgentInternalBuilder(resourcesNamespace, resourcesName).
// 					WithSingleContainerStrategy(false).
// 					Build()
// 				_ = c.Create(context.TODO(), ddai)
// 				return ddai
// 			},
// 			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
// 			wantErr: false,
// 			wantFunc: func(c client.Client) error {
// 				expectedContainers := []string{
// 					string(apicommon.CoreAgentContainerName),
// 					string(apicommon.TraceAgentContainerName),
// 				}

// 				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
// 			},
// 		},
// 		{
// 			name: "[single container] DatadogAgent default, create Daemonset with a single container",
// 			fields: fields{
// 				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v1alpha1.DatadogAgentInternal{}).Build(),
// 				scheme:   s,
// 				recorder: recorder,
// 			},
// 			loadFunc: func(c client.Client) *v1alpha1.DatadogAgentInternal {
// 				ddai := testutils.NewInitializedDatadogAgentInternalBuilder(resourcesNamespace, resourcesName).
// 					WithSingleContainerStrategy(true).
// 					Build()
// 				_ = c.Create(context.TODO(), ddai)
// 				return ddai
// 			},
// 			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
// 			wantErr: false,
// 			wantFunc: func(c client.Client) error {
// 				expectedContainers := []string{
// 					string(apicommon.UnprivilegedSingleAgentContainerName),
// 				}

// 				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
// 			},
// 		},
// 		{
// 			name: "DatadogAgent with APM enabled, create Daemonset with core and process agents",
// 			fields: fields{
// 				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v1alpha1.DatadogAgentInternal{}).Build(),
// 				scheme:   s,
// 				recorder: recorder,
// 			},
// 			loadFunc: func(c client.Client) *v1alpha1.DatadogAgentInternal {
// 				ddai := testutils.NewInitializedDatadogAgentInternalBuilder(resourcesNamespace, resourcesName).
// 					WithAPMEnabled(true).
// 					WithSingleContainerStrategy(false).
// 					Build()
// 				_ = c.Create(context.TODO(), ddai)
// 				return ddai
// 			},
// 			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
// 			wantErr: false,
// 			wantFunc: func(c client.Client) error {
// 				expectedContainers := []string{
// 					string(apicommon.CoreAgentContainerName),
// 					string(apicommon.TraceAgentContainerName),
// 				}

// 				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
// 			},
// 		},
// 		{
// 			name: "[single container] DatadogAgent with APM enabled, create Daemonset with a single container",
// 			fields: fields{
// 				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v1alpha1.DatadogAgentInternal{}).Build(),
// 				scheme:   s,
// 				recorder: recorder,
// 			},
// 			loadFunc: func(c client.Client) *v1alpha1.DatadogAgentInternal {
// 				ddai := testutils.NewInitializedDatadogAgentInternalBuilder(resourcesNamespace, resourcesName).
// 					WithAPMEnabled(true).
// 					WithSingleContainerStrategy(true).
// 					Build()
// 				_ = c.Create(context.TODO(), ddai)
// 				return ddai
// 			},
// 			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
// 			wantErr: false,
// 			wantFunc: func(c client.Client) error {
// 				expectedContainers := []string{
// 					string(apicommon.UnprivilegedSingleAgentContainerName),
// 				}

// 				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
// 			},
// 		},
// 		{
// 			name: "DatadogAgent with APM and CWS enables, create Daemonset with four agents",
// 			fields: fields{
// 				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v1alpha1.DatadogAgentInternal{}).Build(),
// 				scheme:   s,
// 				recorder: recorder,
// 			},
// 			loadFunc: func(c client.Client) *v1alpha1.DatadogAgentInternal {
// 				ddai := testutils.NewInitializedDatadogAgentInternalBuilder(resourcesNamespace, resourcesName).
// 					WithAPMEnabled(true).
// 					WithCWSEnabled(true).
// 					WithSingleContainerStrategy(false).
// 					Build()
// 				_ = c.Create(context.TODO(), ddai)
// 				return ddai
// 			},
// 			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
// 			wantErr: false,
// 			wantFunc: func(c client.Client) error {
// 				expectedContainers := []string{
// 					string(apicommon.CoreAgentContainerName),
// 					string(apicommon.TraceAgentContainerName),
// 					string(apicommon.SystemProbeContainerName),
// 					string(apicommon.SecurityAgentContainerName),
// 				}

// 				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
// 			},
// 		},
// 		{
// 			name: "[single container] DatadogAgent with APM and CWS enables, create Daemonset with four agents",
// 			fields: fields{
// 				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v1alpha1.DatadogAgentInternal{}).Build(),
// 				scheme:   s,
// 				recorder: recorder,
// 			},
// 			loadFunc: func(c client.Client) *v1alpha1.DatadogAgentInternal {
// 				ddai := testutils.NewInitializedDatadogAgentInternalBuilder(resourcesNamespace, resourcesName).
// 					WithAPMEnabled(true).
// 					WithCWSEnabled(true).
// 					WithSingleContainerStrategy(true).
// 					Build()

// 				_ = c.Create(context.TODO(), ddai)
// 				return ddai
// 			},
// 			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
// 			wantErr: false,
// 			wantFunc: func(c client.Client) error {
// 				expectedContainers := []string{
// 					string(apicommon.CoreAgentContainerName),
// 					string(apicommon.TraceAgentContainerName),
// 					string(apicommon.SystemProbeContainerName),
// 					string(apicommon.SecurityAgentContainerName),
// 				}

// 				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
// 			},
// 		},
// 		{
// 			name: "DatadogAgent with APM and OOMKill enabled, create Daemonset with core, trace, and system-probe",
// 			fields: fields{
// 				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v1alpha1.DatadogAgentInternal{}).Build(),
// 				scheme:   s,
// 				recorder: recorder,
// 			},
// 			loadFunc: func(c client.Client) *v1alpha1.DatadogAgentInternal {
// 				ddai := testutils.NewInitializedDatadogAgentInternalBuilder(resourcesNamespace, resourcesName).
// 					WithAPMEnabled(true).
// 					WithOOMKillEnabled(true).
// 					WithSingleContainerStrategy(false).
// 					Build()
// 				_ = c.Create(context.TODO(), ddai)
// 				return ddai
// 			},
// 			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
// 			wantErr: false,
// 			wantFunc: func(c client.Client) error {
// 				expectedContainers := []string{
// 					string(apicommon.CoreAgentContainerName),
// 					string(apicommon.TraceAgentContainerName),
// 					string(apicommon.SystemProbeContainerName),
// 				}

// 				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
// 			},
// 		},
// 		{
// 			name: "[single container] DatadogAgent with APM and OOMKill enabled, create Daemonset with core, trace, and system-probe",
// 			fields: fields{
// 				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v1alpha1.DatadogAgentInternal{}).Build(),
// 				scheme:   s,
// 				recorder: recorder,
// 			},
// 			loadFunc: func(c client.Client) *v1alpha1.DatadogAgentInternal {
// 				ddai := testutils.NewInitializedDatadogAgentInternalBuilder(resourcesNamespace, resourcesName).
// 					WithAPMEnabled(true).
// 					WithOOMKillEnabled(true).
// 					WithSingleContainerStrategy(true).
// 					Build()
// 				_ = c.Create(context.TODO(), ddai)
// 				return ddai
// 			},
// 			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
// 			wantErr: false,
// 			wantFunc: func(c client.Client) error {
// 				expectedContainers := []string{
// 					string(apicommon.CoreAgentContainerName),
// 					string(apicommon.TraceAgentContainerName),
// 					string(apicommon.SystemProbeContainerName),
// 				}

// 				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
// 			},
// 		},
// 		{
// 			name: "DatadogAgent with FIPS enabled",
// 			fields: fields{
// 				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v1alpha1.DatadogAgentInternal{}).Build(),
// 				scheme:   s,
// 				recorder: recorder,
// 			},
// 			loadFunc: func(c client.Client) *v1alpha1.DatadogAgentInternal {
// 				fipsConfig := v2alpha1.FIPSConfig{
// 					Enabled: apiutils.NewBoolPointer(true),
// 				}
// 				ddai := testutils.NewInitializedDatadogAgentInternalBuilder(resourcesNamespace, resourcesName).
// 					WithFIPS(fipsConfig).
// 					Build()
// 				_ = c.Create(context.TODO(), ddai)
// 				return ddai
// 			},
// 			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
// 			wantErr: false,
// 			wantFunc: func(c client.Client) error {
// 				expectedContainers := []string{
// 					string(apicommon.CoreAgentContainerName),
// 					string(apicommon.TraceAgentContainerName),
// 					string(apicommon.FIPSProxyContainerName),
// 				}

// 				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
// 			},
// 		},
// 		{
// 			name: "DatadogAgent with PDB enabled",
// 			fields: fields{
// 				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v1alpha1.DatadogAgentInternal{}, &policyv1.PodDisruptionBudget{}).Build(),
// 				scheme:   s,
// 				recorder: recorder,
// 			},
// 			loadFunc: func(c client.Client) *v1alpha1.DatadogAgentInternal {
// 				ddai := testutils.NewInitializedDatadogAgentInternalBuilder(resourcesNamespace, resourcesName).
// 					WithComponentOverride(v2alpha1.ClusterAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
// 						CreatePodDisruptionBudget: apiutils.NewBoolPointer(true),
// 					}).
// 					WithClusterChecksUseCLCEnabled(true).
// 					WithComponentOverride(v2alpha1.ClusterChecksRunnerComponentName, v2alpha1.DatadogAgentComponentOverride{
// 						CreatePodDisruptionBudget: apiutils.NewBoolPointer(true),
// 					}).
// 					Build()
// 				_ = c.Create(context.TODO(), ddai)
// 				return ddai
// 			},
// 			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
// 			wantErr: false,
// 			wantFunc: func(c client.Client) error {
// 				return verifyPDB(t, c)
// 			},
// 		},
// 		{
// 			name: "DatadogAgent with container monitoring in process agent",
// 			fields: fields{
// 				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v1alpha1.DatadogAgentInternal{}).Build(),
// 				scheme:   s,
// 				recorder: recorder,
// 			},
// 			loadFunc: func(c client.Client) *v1alpha1.DatadogAgentInternal {
// 				ddai := testutils.NewInitializedDatadogAgentInternalBuilder(resourcesNamespace, resourcesName).
// 					WithProcessChecksInCoreAgent(false).
// 					Build()
// 				_ = c.Create(context.TODO(), ddai)
// 				return ddai
// 			},
// 			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
// 			wantErr: false,
// 			wantFunc: func(c client.Client) error {
// 				expectedContainers := []string{
// 					string(apicommon.CoreAgentContainerName),
// 					string(apicommon.ProcessAgentContainerName),
// 					string(apicommon.TraceAgentContainerName),
// 				}

// 				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
// 			},
// 		},
// 		{
// 			name: "DatadogAgent with override.nodeAgent.disabled true",
// 			fields: fields{
// 				client:   fake.NewClientBuilder().WithStatusSubresource(&appsv1.DaemonSet{}, &v1alpha1.DatadogAgentInternal{}).Build(),
// 				scheme:   s,
// 				recorder: recorder,
// 			},
// 			loadFunc: func(c client.Client) *v1alpha1.DatadogAgentInternal {
// 				ddai := testutils.NewInitializedDatadogAgentInternalBuilder(resourcesNamespace, resourcesName).
// 					WithComponentOverride(v2alpha1.NodeAgentComponentName, v2alpha1.DatadogAgentComponentOverride{
// 						Disabled: apiutils.NewBoolPointer(true),
// 					}).
// 					Build()
// 				_ = c.Create(context.TODO(), ddai)
// 				return ddai
// 			},
// 			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
// 			wantErr: false,
// 			wantFunc: func(c client.Client) error {
// 				ds := &appsv1.DaemonSet{}
// 				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: dsName}, ds); client.IgnoreNotFound(err) != nil {
// 					return err
// 				}
// 				return nil
// 			},
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			r := &Reconciler{
// 				client:       tt.fields.client,
// 				scheme:       tt.fields.scheme,
// 				platformInfo: tt.fields.platformInfo,
// 				recorder:     recorder,
// 				log:          logf.Log.WithName(tt.name),
// 				forwarders:   forwarders,
// 				options: ReconcilerOptions{
// 					ExtendedDaemonsetOptions: componentagent.ExtendedDaemonsetOptions{
// 						Enabled: false,
// 					},
// 					SupportCilium: false,
// 				},
// 			}

// 			var dda *v1alpha1.DatadogAgentInternal
// 			if tt.loadFunc != nil {
// 				dda = tt.loadFunc(r.client)
// 			}
// 			got, err := r.Reconcile(context.TODO(), dda)
// 			if tt.wantErr {
// 				assert.Error(t, err, "ReconcileDatadogAgent.Reconcile() expected an error")
// 			} else {
// 				assert.NoError(t, err, "ReconcileDatadogAgent.Reconcile() unexpected error: %v", err)
// 			}

// 			assert.Equal(t, tt.want, got, "ReconcileDatadogAgent.Reconcile() unexpected result")

// 			if tt.wantFunc != nil {
// 				err := tt.wantFunc(r.client)
// 				assert.NoError(t, err, "ReconcileDatadogAgent.Reconcile() wantFunc validation error: %v", err)
// 			}
// 		})
// 	}
// }

// func verifyDaemonsetContainers(c client.Client, resourcesNamespace, dsName string, expectedContainers []string) error {
// 	ds := &appsv1.DaemonSet{}
// 	if err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: dsName}, ds); err != nil {
// 		return err
// 	}
// 	dsContainers := []string{}
// 	for _, container := range ds.Spec.Template.Spec.Containers {
// 		dsContainers = append(dsContainers, container.Name)
// 	}

// 	sort.Strings(dsContainers)
// 	sort.Strings(expectedContainers)
// 	if reflect.DeepEqual(expectedContainers, dsContainers) {
// 		return nil
// 	} else {
// 		return fmt.Errorf("Container don't match, expected %s, actual %s", expectedContainers, dsContainers)
// 	}
// }

// func verifyPDB(t *testing.T, c client.Client) error {
// 	pdbList := policyv1.PodDisruptionBudgetList{}
// 	if err := c.List(context.TODO(), &pdbList); err != nil {
// 		return err
// 	}
// 	assert.True(t, len(pdbList.Items) == 2)

// 	dcaPDB := pdbList.Items[0]
// 	assert.Equal(t, "foo-cluster-agent-pdb", dcaPDB.Name)
// 	assert.Equal(t, intstr.FromInt(1), *dcaPDB.Spec.MinAvailable)
// 	assert.Nil(t, dcaPDB.Spec.MaxUnavailable)

// 	ccrPDB := pdbList.Items[1]
// 	assert.Equal(t, "foo-cluster-checks-runner-pdb", ccrPDB.Name)
// 	assert.Equal(t, intstr.FromInt(1), *ccrPDB.Spec.MaxUnavailable)
// 	assert.Nil(t, ccrPDB.Spec.MinAvailable)
// 	return nil
// }
