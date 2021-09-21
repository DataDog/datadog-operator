// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"reflect"
	"testing"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	test "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1/test"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestReconciler_manageClusterChecksRunnerRBACs(t *testing.T) {
	t.Helper()
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	logger := logf.Log.WithName(t.Name())
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconciler_manageClusterChecksRunnerRBACs"})
	forwarders := dummyManager{}

	s := scheme.Scheme
	if err := datadoghqv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("Unable to add DatadogAgent scheme: %v", err)
	}

	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogAgent{})

	ddaName := "foo"
	ddaNamespace := "bar"
	ddaDefaultOptions := &test.NewDatadogAgentOptions{
		ClusterAgentEnabled:        true,
		ClusterChecksEnabled:       true,
		ClusterChecksRunnerEnabled: true,
		KubeStateMetricsCore: &datadoghqv1alpha1.KubeStateMetricsCore{
			Enabled:      datadoghqv1alpha1.NewBoolPointer(true),
			ClusterCheck: datadoghqv1alpha1.NewBoolPointer(true),
		},
	}
	ddaDefault := test.NewDefaultedDatadogAgent(ddaNamespace, ddaName, ddaDefaultOptions)
	agentVersion := getAgentVersion(ddaDefault)
	serviceAccountName := getClusterChecksRunnerServiceAccount(ddaDefault)
	serviceAccount := buildServiceAccount(ddaDefault, serviceAccountName, agentVersion)
	roleName := getAgentRbacResourcesName(ddaDefault)
	rbacResourcesName := getClusterChecksRunnerRbacResourcesName(ddaDefault)
	clusterRoleBindingInfo := roleBindingInfo{
		name:               rbacResourcesName,
		roleName:           roleName,
		serviceAccountName: serviceAccountName,
	}
	clusterRoleBinding := buildClusterRoleBinding(ddaDefault, clusterRoleBindingInfo, agentVersion)

	agentClusterRoleRBAC := buildAgentClusterRole(ddaDefault, rbacResourcesName, agentVersion)

	clusterRoleRBAC := buildKubeStateMetricsCoreRBAC(ddaDefault, getKubeStateMetricsRBACResourceName(ddaDefault, checkRunnersSuffix), agentVersion)

	type fields struct {
		options     ReconcilerOptions
		client      client.Client
		versionInfo *version.Info
		scheme      *runtime.Scheme
		log         logr.Logger
		recorder    record.EventRecorder
		forwarders  datadog.MetricForwardersManager
	}
	type args struct {
		logger logr.Logger
		dda    *datadoghqv1alpha1.DatadogAgent
	}
	tests := []struct {
		name     string
		fields   fields
		args     args
		want     reconcile.Result
		wantErr  bool
		wantFunc func(t *testing.T, client client.Client)
	}{
		{
			name: "test KSM ClusterRole creation",
			args: args{
				logger: logger,
				dda:    ddaDefault,
			},
			fields: fields{
				client:     fake.NewFakeClientWithScheme(s, serviceAccount, agentClusterRoleRBAC, clusterRoleBinding),
				scheme:     s,
				recorder:   recorder,
				forwarders: forwarders,
			},
			want: reconcile.Result{
				Requeue: false,
			},
			wantErr: false,
			wantFunc: func(t *testing.T, client client.Client) {
				clusterRole := &rbacv1.ClusterRole{}
				kubeStateMetricsRBACName := getKubeStateMetricsRBACResourceName(ddaDefault, checkRunnersSuffix)
				if err := client.Get(context.TODO(), types.NamespacedName{Name: kubeStateMetricsRBACName}, clusterRole); errors.IsNotFound(err) {
					t.Errorf("ClusterRole %s sould be present", kubeStateMetricsRBACName)
				}
			},
		},
		{
			name: "test KSM ClusterRoleBindingRBAC creation",
			args: args{
				logger: logger,
				dda:    ddaDefault,
			},
			fields: fields{
				client:     fake.NewFakeClientWithScheme(s, serviceAccount, agentClusterRoleRBAC, clusterRoleBinding, clusterRoleRBAC),
				scheme:     s,
				recorder:   recorder,
				forwarders: forwarders,
			},
			want: reconcile.Result{
				Requeue: false,
			},
			wantErr: false,
			wantFunc: func(t *testing.T, client client.Client) {
				clusterRoleBinding := &rbacv1.ClusterRoleBinding{}
				kubeStateMetricsRBACName := getKubeStateMetricsRBACResourceName(ddaDefault, checkRunnersSuffix)
				if err := client.Get(context.TODO(), types.NamespacedName{Name: kubeStateMetricsRBACName}, clusterRoleBinding); errors.IsNotFound(err) {
					t.Errorf("ClusterRoleBinding %s sould be present", kubeStateMetricsRBACName)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{
				options:     tt.fields.options,
				client:      tt.fields.client,
				versionInfo: tt.fields.versionInfo,
				scheme:      tt.fields.scheme,
				log:         tt.fields.log,
				recorder:    tt.fields.recorder,
				forwarders:  tt.fields.forwarders,
			}
			got, err := r.manageClusterChecksRunnerRBACs(tt.args.logger, tt.args.dda)
			if (err != nil) != tt.wantErr {
				t.Errorf("Reconciler.manageClusterChecksRunnerRBACs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Reconciler.manageClusterChecksRunnerRBACs() = %v, want %v", got, tt.want)
			}
			if tt.wantFunc != nil {
				tt.wantFunc(t, r.client)
			}
		})
	}
}
