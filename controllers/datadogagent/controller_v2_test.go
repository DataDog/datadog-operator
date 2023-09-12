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

	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	test "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1/test"
	"github.com/DataDog/datadog-operator/apis/utils"
	testutils "github.com/DataDog/datadog-operator/controllers/datadogagent/testutils"
	assert "github.com/stretchr/testify/require"

	componentagent "github.com/DataDog/datadog-operator/controllers/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/types"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconcileDatadogAgentV2_Reconcile(t *testing.T) {
	const resourcesName = "foo"
	const resourcesNamespace = "bar"
	const dsName = "foo-agent"

	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconcileDatadogAgent_Reconcile"})
	forwarders := dummyManager{}

	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	// Register operator types with the runtime scheme.
	s := testutils.TestScheme(true)

	defaultRequeueDuration := 15 * time.Second

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

	tests := []struct {
		name     string
		fields   fields
		args     args
		want     reconcile.Result
		wantErr  bool
		wantFunc func(c client.Client) error
	}{
		{
			name: "DatadogAgent default, create Daemonset with core and process agents",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := newBuilder(resourcesNamespace, resourcesName).build()
					_ = c.Create(context.TODO(), dda)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommonv1.CoreAgentContainerName),
					string(apicommonv1.ProcessAgentContainerName),
				}

				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "[mono-container] DatadogAgent default, create Daemonset with a mono-agent",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := newBuilder(resourcesNamespace, resourcesName).
						withUseMultiProcessContainer(true).build()
					_ = c.Create(context.TODO(), dda)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommonv1.NonPrivilegedMonoContainerName),
				}

				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "DatadogAgent with APM enabled, create Daemonset with core, trace and process agents",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := newBuilder(resourcesNamespace, resourcesName).
						withAPM(true).
						withUseMultiProcessContainer(false).build()
					_ = c.Create(context.TODO(), dda)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommonv1.CoreAgentContainerName),
					string(apicommonv1.ProcessAgentContainerName),
					string(apicommonv1.TraceAgentContainerName),
				}

				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "[mono-container] DatadogAgent with APM enabled, create Daemonset with a mono-agent",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := newBuilder(resourcesNamespace, resourcesName).
						withAPM(true).
						withUseMultiProcessContainer(true).build()
					_ = c.Create(context.TODO(), dda)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommonv1.NonPrivilegedMonoContainerName),
				}

				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "DatadogAgent with APM and CWS enables, create Daemonset with all five agents",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := newBuilder(resourcesNamespace, resourcesName).withAPM(true).withCWS(true).withUseMultiProcessContainer(false).build()
					_ = c.Create(context.TODO(), dda)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommonv1.CoreAgentContainerName),
					string(apicommonv1.ProcessAgentContainerName),
					string(apicommonv1.TraceAgentContainerName),
					string(apicommonv1.SystemProbeContainerName),
					string(apicommonv1.SecurityAgentContainerName),
				}

				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "[mono-container] DatadogAgent with APM and CWS enables, create Daemonset with all five agents",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := newBuilder(resourcesNamespace, resourcesName).
						withAPM(true).
						withCWS(true).
						withUseMultiProcessContainer(true).build()
					_ = c.Create(context.TODO(), dda)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommonv1.CoreAgentContainerName),
					string(apicommonv1.ProcessAgentContainerName),
					string(apicommonv1.TraceAgentContainerName),
					string(apicommonv1.SystemProbeContainerName),
					string(apicommonv1.SecurityAgentContainerName),
				}

				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "DatadogAgent with APM and OOMKill enabled, create Daemonset with core, trace, process and system-probe",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := newBuilder(resourcesNamespace, resourcesName).
						withAPM(true).
						withOOMKill(true).
						withUseMultiProcessContainer(false).build()
					_ = c.Create(context.TODO(), dda)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommonv1.CoreAgentContainerName),
					string(apicommonv1.ProcessAgentContainerName),
					string(apicommonv1.TraceAgentContainerName),
					string(apicommonv1.SystemProbeContainerName),
				}

				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
			},
		},
		{
			name: "[mono-container] DatadogAgent with APM and OOMKill enabled, create Daemonset with core, trace, process and system-probe",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := newBuilder(resourcesNamespace, resourcesName).
						withAPM(true).
						withOOMKill(true).
						withUseMultiProcessContainer(true).build()
					_ = c.Create(context.TODO(), dda)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				expectedContainers := []string{
					string(apicommonv1.CoreAgentContainerName),
					string(apicommonv1.ProcessAgentContainerName),
					string(apicommonv1.TraceAgentContainerName),
					string(apicommonv1.SystemProbeContainerName),
				}

				return verifyDaemonsetContainers(c, resourcesNamespace, dsName, expectedContainers)
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
					V2Enabled:     true,
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

type DatadogAgentBuilder struct {
	datadogAgent v2alpha1.DatadogAgent
}

func newBuilder(ns, name string) *DatadogAgentBuilder {
	dda := test.NewDatadogAgent(ns, name, &v2alpha1.GlobalConfig{
		Credentials: &v2alpha1.DatadogCredentials{
			APIKey: utils.NewStringPointer("apiKey"),
			AppKey: utils.NewStringPointer("appKey"),
		},
	})

	dda.Spec.Features = &v2alpha1.DatadogFeatures{}

	return &DatadogAgentBuilder{
		datadogAgent: *dda,
	}
}

func (builder *DatadogAgentBuilder) withAPM(enabled bool) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Features.APM = &v2alpha1.APMFeatureConfig{
		Enabled: utils.NewBoolPointer(enabled),
	}

	return builder
}

func (builder *DatadogAgentBuilder) withCWS(enabled bool) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Features.CWS = &v2alpha1.CWSFeatureConfig{
		Enabled: utils.NewBoolPointer(enabled),
	}

	return builder
}

func (builder *DatadogAgentBuilder) withOOMKill(enabled bool) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Features.OOMKill = &v2alpha1.OOMKillFeatureConfig{
		Enabled: utils.NewBoolPointer(enabled),
	}

	return builder
}

func (builder *DatadogAgentBuilder) withUseMultiProcessContainer(enabled bool) *DatadogAgentBuilder {
	builder.datadogAgent.Spec.Global.ContainerProcessModel = &v2alpha1.ContainerProcessModel{
		UseMultiProcessContainer: utils.NewBoolPointer(enabled),
	}

	return builder
}

func (builder *DatadogAgentBuilder) build() *v2alpha1.DatadogAgent {
	return &builder.datadogAgent
}
