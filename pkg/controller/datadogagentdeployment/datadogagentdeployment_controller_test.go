// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagentdeployment

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"testing"
	"time"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	test "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1/test"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/google/go-cmp/cmp"
	assert "github.com/stretchr/testify/require"

	edsdatadoghqv1alpha1 "github.com/datadog/extendeddaemonset/pkg/apis/datadoghq/v1alpha1"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var defaultAgentHash, defaultClusterAgentHash string

func TestReconcileDatadogAgentDeployment_createNewExtendedDaemonSet(t *testing.T) {
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconcileDatadogAgentDeployment_createNewExtendedDaemonSet"})
	forwarders := dummyManager{}

	logf.SetLogger(logf.ZapLogger(true))
	log := logf.Log.WithName("TestReconcileDatadogAgentDeployment_createNewExtendedDaemonSet")

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.SchemeGroupVersion, &datadoghqv1alpha1.DatadogAgentDeployment{})
	s.AddKnownTypes(datadoghqv1alpha1.SchemeGroupVersion, &edsdatadoghqv1alpha1.ExtendedDaemonSet{})
	s.AddKnownTypes(appsv1.SchemeGroupVersion, &appsv1.DaemonSet{})

	type fields struct {
		client   client.Client
		scheme   *runtime.Scheme
		recorder record.EventRecorder
	}
	type args struct {
		logger          logr.Logger
		agentdeployment *datadoghqv1alpha1.DatadogAgentDeployment
		newStatus       *datadoghqv1alpha1.DatadogAgentDeploymentStatus
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    reconcile.Result
		wantErr bool
	}{
		{
			name: "create new EDS",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				logger:          log,
				agentdeployment: test.NewDefaultedDatadogAgentDeployment("bar", "foo", nil),
				newStatus:       &datadoghqv1alpha1.DatadogAgentDeploymentStatus{},
			},
			want:    reconcile.Result{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReconcileDatadogAgentDeployment{
				client:     tt.fields.client,
				scheme:     tt.fields.scheme,
				recorder:   recorder,
				forwarders: forwarders,
			}
			got, err := r.createNewExtendedDaemonSet(tt.args.logger, tt.args.agentdeployment, tt.args.newStatus)
			if tt.wantErr {
				assert.Error(t, err, "ReconcileDatadogAgentDeployment.createNewExtendedDaemonSet() expected an error")
			} else {
				assert.NoError(t, err, "ReconcileDatadogAgentDeployment.createNewExtendedDaemonSet() unexpected error: %v", err)
			}
			assert.Equal(t, tt.want, got, "ReconcileDatadogAgentDeployment.createNewExtendedDaemonSet() unexpected result")
		})
	}
}

func TestReconcileDatadogAgentDeployment_Reconcile(t *testing.T) {
	supportExtendedDaemonset = true
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconcileDatadogAgentDeployment_Reconcile"})
	forwarders := dummyManager{}

	logf.SetLogger(logf.ZapLogger(true))

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.SchemeGroupVersion, &datadoghqv1alpha1.DatadogAgentDeployment{})
	s.AddKnownTypes(datadoghqv1alpha1.SchemeGroupVersion, &edsdatadoghqv1alpha1.ExtendedDaemonSet{})
	s.AddKnownTypes(appsv1.SchemeGroupVersion, &appsv1.DaemonSet{})
	s.AddKnownTypes(appsv1.SchemeGroupVersion, &appsv1.Deployment{})
	s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Secret{})
	s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.ServiceAccount{})
	s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.ConfigMap{})
	s.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.ClusterRoleBinding{})
	s.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.ClusterRole{})
	s.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.Role{})
	s.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.RoleBinding{})
	s.AddKnownTypes(policyv1.SchemeGroupVersion, &policyv1.PodDisruptionBudget{})

	defaultRequeueDuration := 15 * time.Second

	type fields struct {
		client   client.Client
		scheme   *runtime.Scheme
		recorder record.EventRecorder
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
			name: "DatadogAgentDeployment not found",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
			},
			want:    reconcile.Result{},
			wantErr: false,
		},
		{
			name: "DatadogAgentDeployment found, add finalizer",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					_ = c.Create(context.TODO(), &datadoghqv1alpha1.DatadogAgentDeployment{
						TypeMeta: metav1.TypeMeta{
							Kind:       "DatadogAgentDeployment",
							APIVersion: fmt.Sprintf("%s/%s", datadoghqv1alpha1.SchemeGroupVersion.Group, datadoghqv1alpha1.SchemeGroupVersion.Version),
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace:   "bar",
							Name:        "foo",
							Labels:      map[string]string{"label-foo-key": "label-bar-value"},
							Annotations: map[string]string{"annotations-foo-key": "annotations-bar-value"},
						},
						Spec: datadoghqv1alpha1.DatadogAgentDeploymentSpec{
							Credentials:  datadoghqv1alpha1.AgentCredentials{Token: "token-foo"},
							Agent:        &datadoghqv1alpha1.DatadogAgentDeploymentSpecAgentSpec{},
							ClusterAgent: &datadoghqv1alpha1.DatadogAgentDeploymentSpecClusterAgentSpec{},
						},
					})
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				dad := &datadoghqv1alpha1.DatadogAgentDeployment{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: "foo", Namespace: "bar"}, dad); err != nil {
					return err
				}
				assert.Contains(t, dad.GetFinalizers(), "finalizer.agentdeployment.datadoghq.com")
				return nil
			},
		},
		{
			name: "DatadogAgentDeployment found, but not defaulted",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					_ = c.Create(context.TODO(), &datadoghqv1alpha1.DatadogAgentDeployment{
						TypeMeta: metav1.TypeMeta{
							Kind:       "DatadogAgentDeployment",
							APIVersion: fmt.Sprintf("%s/%s", datadoghqv1alpha1.SchemeGroupVersion.Group, datadoghqv1alpha1.SchemeGroupVersion.Version),
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace:   "bar",
							Name:        "foo",
							Labels:      map[string]string{"label-foo-key": "label-bar-value"},
							Annotations: map[string]string{"annotations-foo-key": "annotations-bar-value"},
						},
						Spec: datadoghqv1alpha1.DatadogAgentDeploymentSpec{
							Credentials:  datadoghqv1alpha1.AgentCredentials{Token: "token-foo"},
							Agent:        &datadoghqv1alpha1.DatadogAgentDeploymentSpecAgentSpec{},
							ClusterAgent: &datadoghqv1alpha1.DatadogAgentDeploymentSpecClusterAgentSpec{},
						},
					})
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
		},
		{
			name: "DatadogAgentDeployment found and defaulted, create the Agent's ClusterRole",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					_ = c.Create(context.TODO(), test.NewDefaultedDatadogAgentDeployment("bar", "foo", &test.NewDatadogAgentDeploymentOptions{UseEDS: true, Labels: map[string]string{"label-foo-key": "label-bar-value"}}))
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				rbacResourcesName := "foo-agent"
				clusterRole := &rbacv1.ClusterRole{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesName}, clusterRole); err != nil {
					return err
				}
				if !hasAllClusterLevelRbacResources(clusterRole.Rules) {
					return fmt.Errorf("bad cluster role, should contain all cluster level rbac resources, current: %v", clusterRole.Rules)
				}
				if !hasAllNodeLevelRbacResources(clusterRole.Rules) {
					return fmt.Errorf("bad cluster role, should contain all node level rbac resources, current: %v", clusterRole.Rules)
				}
				if !ownedByDatadogOperator(clusterRole.OwnerReferences) {
					return fmt.Errorf("bad cluster role, should be owned by the datadog operator, current owners: %v", clusterRole.OwnerReferences)
				}

				return nil
			},
		},
		{
			name: "DatadogAgentDeployment found and defaulted, create the Agent's ClusterRoleBinding",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", &test.NewDatadogAgentDeploymentOptions{UseEDS: true, Labels: map[string]string{"label-foo-key": "label-bar-value"}})
					_ = c.Create(context.TODO(), dad)
					_ = c.Create(context.TODO(), buildAgentClusterRole(dad, getAgentRbacResourcesName(dad), getAgentVersion(dad)))
					_ = c.Create(context.TODO(), buildServiceAccount(dad, getAgentRbacResourcesName(dad), getAgentVersion(dad)))
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				rbacResourcesName := "foo-agent"
				clusterRoleBinding := &rbacv1.ClusterRoleBinding{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesName}, clusterRoleBinding); err != nil {
					return err
				}
				if !ownedByDatadogOperator(clusterRoleBinding.OwnerReferences) {
					return fmt.Errorf("bad clusterRoleBinding, should be owned by the datadog operator, current owners: %v", clusterRoleBinding.OwnerReferences)
				}
				return nil
			},
		},
		{
			name: "DatadogAgentDeployment found and defaulted, create the Agent's ServiceAccount",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", &test.NewDatadogAgentDeploymentOptions{UseEDS: true, Labels: map[string]string{"label-foo-key": "label-bar-value"}})
					_ = c.Create(context.TODO(), dad)
					resourceName := getAgentRbacResourcesName(dad)
					version := getAgentVersion(dad)
					_ = c.Create(context.TODO(), buildAgentClusterRole(dad, resourceName, version))
					_ = c.Create(context.TODO(), buildClusterRoleBinding(dad, roleBindingInfo{
						name:               resourceName,
						roleName:           resourceName,
						serviceAccountName: resourceName,
					}, version))
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				rbacResourcesName := "foo-agent"
				serviceAccount := &corev1.ServiceAccount{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesName}, serviceAccount); err != nil {
					return err
				}
				if !ownedByDatadogOperator(serviceAccount.OwnerReferences) {
					return fmt.Errorf("bad serviceAccount, should be owned by the datadog operator, current owners: %v", serviceAccount.OwnerReferences)
				}
				return nil
			},
		},
		{
			name: "DatadogAgentDeployment found and defaulted, create the ExtendedDaemonSet",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", &test.NewDatadogAgentDeploymentOptions{UseEDS: true, Labels: map[string]string{"label-foo-key": "label-bar-value"}})
					_ = c.Create(context.TODO(), dad)

					createAgentDependencies(c, dad)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				eds := &edsdatadoghqv1alpha1.ExtendedDaemonSet{}
				if err := c.Get(context.TODO(), newRequest("bar", "foo").NamespacedName, eds); err != nil {
					return err
				}
				if eds.Name != "foo" {
					return fmt.Errorf("eds bad name, should be: 'foo', current: %s", eds.Name)
				}
				if eds.OwnerReferences == nil || len(eds.OwnerReferences) != 1 {
					return fmt.Errorf("eds bad owner references, should be: '[Kind DatadogAgentDeployment - Name foo]', current: %v", eds.OwnerReferences)
				}
				rbacResourcesName := fmt.Sprintf("%s-agent", eds.Name)
				clusterRole := &rbacv1.ClusterRole{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesName}, clusterRole); err != nil {
					return err
				}
				if !hasAllClusterLevelRbacResources(clusterRole.Rules) {
					return fmt.Errorf("bad cluster role, should contain all cluster level rbac resources, current: %v", clusterRole.Rules)
				}
				if !hasAllNodeLevelRbacResources(clusterRole.Rules) {
					return fmt.Errorf("bad cluster role, should contain all node level rbac resources, current: %v", clusterRole.Rules)
				}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesName}, &rbacv1.ClusterRoleBinding{}); err != nil {
					return err
				}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesName}, &corev1.ServiceAccount{}); err != nil {
					return err
				}

				return nil
			},
		},
		{
			name: "DatadogAgentDeployment found and defaulted, block daemonsetName change",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", &test.NewDatadogAgentDeploymentOptions{
						UseEDS: true,
						Labels: map[string]string{"label-foo-key": "label-bar-value"},
						Status: &datadoghqv1alpha1.DatadogAgentDeploymentStatus{
							Agent: &datadoghqv1alpha1.DatadogAgentDeploymentAgentStatus{
								DaemonsetName: "datadog-agent-daemonset-before",
							},
						},
						AgentDaemonsetName: "datadog-agent-daemonset",
					})
					_ = c.Create(context.TODO(), dad)

					createAgentDependencies(c, dad)
				},
			},
			want:    reconcile.Result{},
			wantErr: true,
			wantFunc: func(c client.Client) error {
				eds := &edsdatadoghqv1alpha1.ExtendedDaemonSet{}
				err := c.Get(context.TODO(), newRequest("bar", "foo").NamespacedName, eds)
				if apierrors.IsNotFound(err) {
					// Daemonset must NOT be created
					return nil
				}
				return err
			},
		},
		{
			name: "DatadogAgentDeployment found and defaulted, create the ExtendedDaemonSet with non default config",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", &test.NewDatadogAgentDeploymentOptions{
						UseEDS: true,
						Labels: map[string]string{"label-foo-key": "label-bar-value"},
						NodeAgentConfig: &datadoghqv1alpha1.NodeAgentConfig{
							DDUrl:    datadoghqv1alpha1.NewStringPointer("https://test.url.com"),
							LogLevel: datadoghqv1alpha1.NewStringPointer("TRACE"),
							Tags:     []string{"tag:test"},
							Env: []corev1.EnvVar{
								{
									Name:  "env",
									Value: "test",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "volumeMount",
									MountPath: "my/test/path",
								},
							},
							PodLabelsAsTags: map[string]string{
								"label": "test",
							},
							PodAnnotationsAsTags: map[string]string{
								"annotation": "test",
							},
							CollectEvents:  datadoghqv1alpha1.NewBoolPointer(true),
							LeaderElection: datadoghqv1alpha1.NewBoolPointer(true),
						},
					})
					_ = c.Create(context.TODO(), dad)

					createAgentDependencies(c, dad)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				eds := &edsdatadoghqv1alpha1.ExtendedDaemonSet{}
				if err := c.Get(context.TODO(), newRequest("bar", "foo").NamespacedName, eds); err != nil {
					return err
				}
				if eds.Name != "foo" {
					return fmt.Errorf("eds bad name, should be: 'foo', current: %s", eds.Name)
				}
				if eds.OwnerReferences == nil || len(eds.OwnerReferences) != 1 {
					return fmt.Errorf("eds bad owner references, should be: '[Kind DatadogAgentDeployment - Name foo]', current: %v", eds.OwnerReferences)
				}

				agentContainer := eds.Spec.Template.Spec.Containers[0]
				if !containsEnv(agentContainer.Env, "DD_DD_URL", "https://test.url.com") {
					return errors.New("eds pod template is missing a custom env var")
				}
				if !containsEnv(agentContainer.Env, "env", "test") {
					return errors.New("eds pod template is missing a custom env var")
				}
				if !containsEnv(agentContainer.Env, "DD_LOG_LEVEL", "TRACE") {
					return errors.New("DD_LOG_LEVEL hasn't been set correctly")
				}
				if !containsEnv(agentContainer.Env, "DD_TAGS", "[\"tag:test\"]") {
					return errors.New("DD_TAGS hasn't been set correctly")
				}
				if !containsEnv(agentContainer.Env, "DD_KUBERNETES_POD_LABELS_AS_TAGS", "{\"label\":\"test\"}") {
					return errors.New("DD_KUBERNETES_POD_LABELS_AS_TAGS hasn't been set correctly")
				}
				if !containsEnv(agentContainer.Env, "DD_KUBERNETES_POD_ANNOTATIONS_AS_TAGS", "{\"annotation\":\"test\"}") {
					return errors.New("DD_KUBERNETES_POD_ANNOTATIONS_AS_TAGS hasn't been set correctly")
				}
				if !containsEnv(agentContainer.Env, "DD_COLLECT_KUBERNETES_EVENTS", "true") {
					return errors.New("DD_COLLECT_KUBERNETES_EVENTS hasn't been set correctly")
				}
				if !containsEnv(agentContainer.Env, "DD_LEADER_ELECTION", "true") {
					return errors.New("DD_LEADER_ELECTION hasn't been set correctly")
				}
				if !containsVolumeMounts(agentContainer.VolumeMounts, "volumeMount", "my/test/path") {
					return errors.New("volumeMount hasn't been set correctly")
				}

				return nil
			},
		},

		{
			name: "Cluster Agent enabled, create the cluster agent secret",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					_ = c.Create(context.TODO(), test.NewDefaultedDatadogAgentDeployment("bar", "foo", &test.NewDatadogAgentDeploymentOptions{ClusterAgentEnabled: true, Labels: map[string]string{"label-foo-key": "label-bar-value"}}))
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				secret := &corev1.Secret{}
				if err := c.Get(context.TODO(), newRequest("bar", "foo-cluster-agent").NamespacedName, secret); err != nil {
					return err
				}
				if secret.OwnerReferences == nil || len(secret.OwnerReferences) != 1 {
					return fmt.Errorf("ds bad owner references, should be: '[Kind DatadogAgentDeployment - Name foo]', current: %v", secret.OwnerReferences)
				}

				return nil
			},
		},

		{
			name: "DatadogAgentDeployment found and defaulted, create the DaemonSet",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", &test.NewDatadogAgentDeploymentOptions{ClusterAgentEnabled: false, UseEDS: false, Labels: map[string]string{"label-foo-key": "label-bar-value"}})
					_ = c.Create(context.TODO(), dad)
					createAgentDependencies(c, dad)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				ds := &appsv1.DaemonSet{}
				if err := c.Get(context.TODO(), newRequest("bar", "foo").NamespacedName, ds); err != nil {
					return err
				}
				if ds.Name != "foo" {
					return fmt.Errorf("ds bad name, should be: 'foo', current: %s", ds.Name)
				}
				if ds.OwnerReferences == nil || len(ds.OwnerReferences) != 1 {
					return fmt.Errorf("ds bad owner references, should be: '[Kind DatadogAgentDeployment - Name foo]', current: %v", ds.OwnerReferences)
				}

				return nil
			},
		},
		{
			name: "DatadogAgentDeployment with APM agent found and defaulted, create Daemonset",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", &test.NewDatadogAgentDeploymentOptions{APMEnabled: true, ClusterAgentEnabled: false, UseEDS: false, Labels: map[string]string{"label-foo-key": "label-bar-value"}})
					_ = c.Create(context.TODO(), dad)
					createAgentDependencies(c, dad)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				ds := &appsv1.DaemonSet{}
				if err := c.Get(context.TODO(), newRequest("bar", "foo").NamespacedName, ds); err != nil {
					return err
				}

				for _, container := range ds.Spec.Template.Spec.Containers {
					if container.Name == "trace-agent" {
						return nil
					}
				}

				return fmt.Errorf("APM container not found")
			},
		},
		{
			name: "DatadogAgentDeployment with Process agent found and defaulted, create Daemonset",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", &test.NewDatadogAgentDeploymentOptions{ProcessEnabled: true, ClusterAgentEnabled: false, UseEDS: false, Labels: map[string]string{"label-foo-key": "label-bar-value"}})
					_ = c.Create(context.TODO(), dad)
					createAgentDependencies(c, dad)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				ds := &appsv1.DaemonSet{}
				if err := c.Get(context.TODO(), newRequest("bar", "foo").NamespacedName, ds); err != nil {
					return err
				}

				for _, container := range ds.Spec.Template.Spec.Containers {
					if container.Name == "process-agent" {
						return nil
					}
				}

				return fmt.Errorf("process container not found")
			},
		},
		{
			name: "DatadogAgentDeployment with Process agent found and defaulted, create system-probe-config configmap",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", &test.NewDatadogAgentDeploymentOptions{ProcessEnabled: true, SystemProbeEnabled: true, ClusterAgentEnabled: false, UseEDS: false, Labels: map[string]string{"label-foo-key": "label-bar-value"}})
					_ = c.Create(context.TODO(), dad)
					createAgentDependencies(c, dad)
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				configmap := &corev1.ConfigMap{}
				if err := c.Get(context.TODO(), newRequest("bar", getSystemProbeConfiConfigMapName("foo")).NamespacedName, configmap); err != nil {
					return err
				}

				return nil
			},
		},
		{
			name: "DatadogAgentDeployment with Process agent found and defaulted, create datadog-agent-security configmap",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", &test.NewDatadogAgentDeploymentOptions{ProcessEnabled: true, SystemProbeEnabled: true, ClusterAgentEnabled: false, UseEDS: false, Labels: map[string]string{"label-foo-key": "label-bar-value"}})
					_ = c.Create(context.TODO(), dad)
					createAgentDependencies(c, dad)
					configCM, _ := buildSystemProbeConfigConfiMap(dad)
					_ = c.Create(context.TODO(), configCM)
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				configmap := &corev1.ConfigMap{}
				if err := c.Get(context.TODO(), newRequest("bar", getSecCompConfigMapName("foo")).NamespacedName, configmap); err != nil {
					return err
				}

				return nil
			},
		},
		{
			name: "DatadogAgentDeployment with Process agent and system-probe found and defaulted, create Daemonset",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", &test.NewDatadogAgentDeploymentOptions{ProcessEnabled: true, SystemProbeEnabled: true, ClusterAgentEnabled: false, UseEDS: false, Labels: map[string]string{"label-foo-key": "label-bar-value"}})
					_ = c.Create(context.TODO(), dad)
					createAgentDependencies(c, dad)
					createSystemProbeDependencies(c, dad)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				ds := &appsv1.DaemonSet{}
				if err := c.Get(context.TODO(), newRequest("bar", "foo").NamespacedName, ds); err != nil {
					return err
				}
				var process, systemprobe bool
				for _, container := range ds.Spec.Template.Spec.Containers {
					if container.Name == "process-agent" {
						process = true
					}
					if container.Name == "system-probe" {
						systemprobe = true
					}
				}
				if !process {
					return fmt.Errorf("process container not found")
				}

				if !systemprobe {
					return fmt.Errorf("system-probe container not found")
				}

				return nil
			},
		},
		{
			name: "DatadogAgentDeployment found and defaulted, ExtendedDaemonSet already exists",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					adOptions := &test.NewDatadogAgentDeploymentOptions{
						UseEDS: true,
						Labels: map[string]string{"label-foo-key": "label-bar-value"},
						Status: &datadoghqv1alpha1.DatadogAgentDeploymentStatus{},
					}
					ad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", adOptions)
					adHash, _ := comparison.GenerateMD5ForSpec(ad.Spec)
					createAgentDependencies(c, ad)
					edsOptions := &test.NewExtendedDaemonSetOptions{
						Labels:      map[string]string{"label-foo-key": "label-bar-value"},
						Annotations: map[string]string{string(datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey): adHash},
					}
					eds := test.NewExtendedDaemonSet("bar", "foo", edsOptions)

					_ = c.Create(context.TODO(), ad)
					_ = c.Create(context.TODO(), eds)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				eds := &edsdatadoghqv1alpha1.ExtendedDaemonSet{}
				if err := c.Get(context.TODO(), newRequest("bar", "foo").NamespacedName, eds); err != nil {
					return err
				}
				if eds.Name != "foo" {
					return fmt.Errorf("eds bad name, should be: 'foo', current: %s", eds.Name)
				}

				return nil
			},
		},
		{
			name: "DatadogAgentDeployment found and defaulted, ExtendedDaemonSet already exists but not up-to-date",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					adOptions := &test.NewDatadogAgentDeploymentOptions{
						UseEDS: true,
						Labels: map[string]string{"label-foo-key": "label-bar-value"},
						Status: &datadoghqv1alpha1.DatadogAgentDeploymentStatus{},
					}
					dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", adOptions)

					createAgentDependencies(c, dad)

					edsOptions := &test.NewExtendedDaemonSetOptions{
						Labels:      map[string]string{"label-foo-key": "label-bar-value"},
						Annotations: map[string]string{datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey: "outdated-hash"},
					}
					eds := test.NewExtendedDaemonSet("bar", "foo", edsOptions)

					_ = c.Create(context.TODO(), dad)
					_ = c.Create(context.TODO(), eds)
				},
			},
			want:    reconcile.Result{RequeueAfter: 5 * time.Second},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				eds := &edsdatadoghqv1alpha1.ExtendedDaemonSet{}
				if err := c.Get(context.TODO(), newRequest("bar", "foo").NamespacedName, eds); err != nil {
					return err
				}
				if eds.Name != "foo" {
					return fmt.Errorf("eds bad name, should be: 'foo', current: %s", eds.Name)
				}
				if eds.OwnerReferences == nil || len(eds.OwnerReferences) != 1 {
					return fmt.Errorf("eds bad owner references, should be: '[Kind DatadogAgentDeployment - Name foo]', current: %v", eds.OwnerReferences)
				}
				if hash := eds.Annotations[datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey]; hash == "outdated-hash" {
					return errors.New("eds hash not updated")
				}

				return nil
			},
		},
		{
			name: "DatadogAgentDeployment found and defaulted, Cluster Agent enabled, create the Cluster Agent Service",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", &test.NewDatadogAgentDeploymentOptions{Labels: map[string]string{"label-foo-key": "label-bar-value"}, ClusterAgentEnabled: true})
					_ = c.Create(context.TODO(), dad)
					commonDCAlabels := getDefaultLabels(dad, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dad))
					_ = c.Create(context.TODO(), test.NewSecret("bar", "foo-cluster-agent", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				dcaService := &corev1.Service{}
				if err := c.Get(context.TODO(), newRequest("bar", "foo-cluster-agent").NamespacedName, dcaService); err != nil {
					return err
				}

				return nil
			},
		},
		{
			name: "DatadogAgentDeployment found and defaulted, Cluster Agent enabled, create the Metrics Server Service",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", &test.NewDatadogAgentDeploymentOptions{Labels: map[string]string{"label-foo-key": "label-bar-value"}, ClusterAgentEnabled: true, MetricsServerEnabled: true})
					_ = c.Create(context.TODO(), dad)
					commonDCAlabels := getDefaultLabels(dad, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dad))
					_ = c.Create(context.TODO(), test.NewSecret("bar", "foo-cluster-agent", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))
					dcaService := test.NewService("bar", "foo-cluster-agent", &test.NewServiceOptions{Spec: &corev1.ServiceSpec{
						Type: corev1.ServiceTypeClusterIP,
						Selector: map[string]string{
							datadoghqv1alpha1.AgentDeploymentNameLabelKey:      "foo",
							datadoghqv1alpha1.AgentDeploymentComponentLabelKey: "cluster-agent",
						},
						Ports: []corev1.ServicePort{
							{
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(datadoghqv1alpha1.DefaultClusterAgentServicePort),
								Port:       datadoghqv1alpha1.DefaultClusterAgentServicePort,
							},
						},
						SessionAffinity: corev1.ServiceAffinityNone,
					},
					})
					_, _ = comparison.SetMD5GenerationAnnotation(&dcaService.ObjectMeta, dcaService.Spec)
					dcaService.Labels = commonDCAlabels
					_ = c.Create(context.TODO(), dcaService)
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				dcaService := &corev1.Service{}
				if err := c.Get(context.TODO(), newRequest("bar", "foo-cluster-agent-metrics-server").NamespacedName, dcaService); err != nil {
					return err
				}

				return nil
			},
		},
		{
			name: "DatadogAgentDeployment found and defaulted, Cluster Agent enabled, create the Cluster Agent Deployment",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", &test.NewDatadogAgentDeploymentOptions{Labels: map[string]string{"label-foo-key": "label-bar-value"}, ClusterAgentEnabled: true})
					_ = c.Create(context.TODO(), dad)
					commonDCAlabels := getDefaultLabels(dad, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dad))
					_ = c.Create(context.TODO(), test.NewSecret("bar", "foo-cluster-agent", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))

					createClusterAgentDependencies(c, dad)
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				dca := &appsv1.Deployment{}
				if err := c.Get(context.TODO(), newRequest("bar", "foo-cluster-agent").NamespacedName, dca); err != nil {
					return err
				}
				if dca.OwnerReferences == nil || len(dca.OwnerReferences) != 1 {
					return fmt.Errorf("dca bad owner references, should be: '[Kind DatadogAgentDeployment - Name foo]', current: %v", dca.OwnerReferences)
				}
				return nil
			},
		},
		{
			name: "DatadogAgentDeployment found and defaulted, Cluster Agent enabled, create the Cluster Agent PDB",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", &test.NewDatadogAgentDeploymentOptions{Labels: map[string]string{"label-foo-key": "label-bar-value"}, ClusterAgentEnabled: true})
					_ = c.Create(context.TODO(), dad)
					commonDCAlabels := getDefaultLabels(dad, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dad))
					_ = c.Create(context.TODO(), test.NewSecret("bar", "foo-cluster-agent", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))
					dcaService := test.NewService("bar", "foo-cluster-agent", &test.NewServiceOptions{Spec: &corev1.ServiceSpec{
						Type: corev1.ServiceTypeClusterIP,
						Selector: map[string]string{
							datadoghqv1alpha1.AgentDeploymentNameLabelKey:      "foo",
							datadoghqv1alpha1.AgentDeploymentComponentLabelKey: "cluster-agent",
						},
						Ports: []corev1.ServicePort{
							{
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(datadoghqv1alpha1.DefaultClusterAgentServicePort),
								Port:       datadoghqv1alpha1.DefaultClusterAgentServicePort,
							},
						},
						SessionAffinity: corev1.ServiceAffinityNone,
					},
					})
					_, _ = comparison.SetMD5GenerationAnnotation(&dcaService.ObjectMeta, dcaService.Spec)
					dcaService.Labels = commonDCAlabels
					_ = c.Create(context.TODO(), dcaService)
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				pdb := &policyv1.PodDisruptionBudget{}
				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: "bar", Name: "foo-cluster-agent"}, pdb); err != nil {
					return err
				}
				if !ownedByDatadogOperator(pdb.OwnerReferences) {
					return fmt.Errorf("bad PDB, should be owned by the datadog operator, current owners: %v", pdb.OwnerReferences)
				}

				return nil
			},
		},
		{
			name: "DatadogAgentDeployment found and defaulted, Cluster Agent enabled, create the Cluster Agent ClusterRole",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", &test.NewDatadogAgentDeploymentOptions{Labels: map[string]string{"label-foo-key": "label-bar-value"}, ClusterAgentEnabled: true})
					_ = c.Create(context.TODO(), dad)
					commonDCAlabels := getDefaultLabels(dad, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dad))
					_ = c.Create(context.TODO(), test.NewSecret("bar", "foo-cluster-agent", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))
					dcaService := test.NewService("bar", "foo-cluster-agent", &test.NewServiceOptions{Spec: &corev1.ServiceSpec{
						Type: corev1.ServiceTypeClusterIP,
						Selector: map[string]string{
							datadoghqv1alpha1.AgentDeploymentNameLabelKey:      "foo",
							datadoghqv1alpha1.AgentDeploymentComponentLabelKey: "cluster-agent",
						},
						Ports: []corev1.ServicePort{
							{
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(datadoghqv1alpha1.DefaultClusterAgentServicePort),
								Port:       datadoghqv1alpha1.DefaultClusterAgentServicePort,
							},
						},
						SessionAffinity: corev1.ServiceAffinityNone,
					},
					})
					_, _ = comparison.SetMD5GenerationAnnotation(&dcaService.ObjectMeta, dcaService.Spec)
					dcaService.Labels = commonDCAlabels
					_ = c.Create(context.TODO(), dcaService)
					_ = c.Create(context.TODO(), buildClusterAgentPDB(dad))
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				rbacResourcesNameClusterAgent := "foo-cluster-agent"
				clusterRole := &rbacv1.ClusterRole{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesNameClusterAgent}, clusterRole); err != nil {
					return err
				}
				if !hasAllClusterLevelRbacResources(clusterRole.Rules) {
					return fmt.Errorf("bad cluster role, should contain all cluster level rbac resources, current: %v", clusterRole.Rules)
				}
				if !ownedByDatadogOperator(clusterRole.OwnerReferences) {
					return fmt.Errorf("bad clusterRole, should be owned by the datadog operator, current owners: %v", clusterRole.OwnerReferences)
				}

				return nil
			},
		},
		{
			name: "DatadogAgentDeployment found and defaulted, Cluster Agent enabled, create the Cluster Agent ClusterRoleBinding",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", &test.NewDatadogAgentDeploymentOptions{Labels: map[string]string{"label-foo-key": "label-bar-value"}, ClusterAgentEnabled: true})
					_ = c.Create(context.TODO(), dad)
					commonDCAlabels := getDefaultLabels(dad, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dad))
					_ = c.Create(context.TODO(), test.NewSecret("bar", "foo-cluster-agent", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))
					dcaService := test.NewService("bar", "foo-cluster-agent", &test.NewServiceOptions{Spec: &corev1.ServiceSpec{
						Type: corev1.ServiceTypeClusterIP,
						Selector: map[string]string{
							datadoghqv1alpha1.AgentDeploymentNameLabelKey:      "foo",
							datadoghqv1alpha1.AgentDeploymentComponentLabelKey: "cluster-agent",
						},
						Ports: []corev1.ServicePort{
							{
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(datadoghqv1alpha1.DefaultClusterAgentServicePort),
								Port:       datadoghqv1alpha1.DefaultClusterAgentServicePort,
							},
						},
						SessionAffinity: corev1.ServiceAffinityNone,
					},
					})
					_, _ = comparison.SetMD5GenerationAnnotation(&dcaService.ObjectMeta, dcaService.Spec)
					dcaService.Labels = commonDCAlabels
					_ = c.Create(context.TODO(), dcaService)
					_ = c.Create(context.TODO(), buildClusterAgentClusterRole(dad, "foo-cluster-agent", getClusterAgentVersion(dad)))
					_ = c.Create(context.TODO(), buildClusterAgentPDB(dad))
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				rbacResourcesNameClusterAgent := "foo-cluster-agent"
				clusterRoleBinding := &rbacv1.ClusterRoleBinding{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesNameClusterAgent}, clusterRoleBinding); err != nil {
					return err
				}
				if !ownedByDatadogOperator(clusterRoleBinding.OwnerReferences) {
					return fmt.Errorf("bad clusterRoleBinding, should be owned by the datadog operator, current owners: %v", clusterRoleBinding.OwnerReferences)
				}

				return nil
			},
		},
		{
			name: "DatadogAgentDeployment found and defaulted, Cluster Agent enabled, create the Cluster Agent HPA ClusterRoleBinding",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", &test.NewDatadogAgentDeploymentOptions{Labels: map[string]string{"label-foo-key": "label-bar-value"}, ClusterAgentEnabled: true, MetricsServerEnabled: true})
					_ = c.Create(context.TODO(), dad)
					commonDCAlabels := getDefaultLabels(dad, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dad))
					_ = c.Create(context.TODO(), test.NewSecret("bar", "foo-cluster-agent", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))

					createClusterAgentDependencies(c, dad)

					dcaExternalMetricsService := test.NewService("bar", "foo-cluster-agent-metrics-server", &test.NewServiceOptions{Spec: &corev1.ServiceSpec{
						Type: corev1.ServiceTypeClusterIP,
						Selector: map[string]string{
							datadoghqv1alpha1.AgentDeploymentNameLabelKey:      "foo",
							datadoghqv1alpha1.AgentDeploymentComponentLabelKey: "cluster-agent",
						},
						Ports: []corev1.ServicePort{
							{
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(datadoghqv1alpha1.DefaultMetricsServerServicePort),
								Port:       datadoghqv1alpha1.DefaultMetricsServerServicePort,
							},
						},
						SessionAffinity: corev1.ServiceAffinityNone,
					},
					})
					_, _ = comparison.SetMD5GenerationAnnotation(&dcaExternalMetricsService.ObjectMeta, dcaExternalMetricsService.Spec)
					dcaExternalMetricsService.Labels = commonDCAlabels
					_ = c.Create(context.TODO(), dcaExternalMetricsService)
					_ = c.Create(context.TODO(), buildClusterAgentPDB(dad))

				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				// Make sure Cluster Agent HPA ClusterRoleBinding is created properly
				clusterRoleBinding := &rbacv1.ClusterRoleBinding{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: "foo-cluster-agent-auth-delegator"}, clusterRoleBinding); err != nil {
					return err
				}
				if !ownedByDatadogOperator(clusterRoleBinding.OwnerReferences) {
					return fmt.Errorf("bad clusterRoleBinding, should be owned by the datadog operator, current owners: %v", clusterRoleBinding.OwnerReferences)
				}

				return nil
			},
		},
		{
			name: "DatadogAgentDeployment found and defaulted, Cluster Agent enabled, create the Cluster Agent ServiceAccount",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", &test.NewDatadogAgentDeploymentOptions{Labels: map[string]string{"label-foo-key": "label-bar-value"}, ClusterAgentEnabled: true, MetricsServerEnabled: false})
					_ = c.Create(context.TODO(), dad)
					commonDCAlabels := getDefaultLabels(dad, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dad))
					_ = c.Create(context.TODO(), test.NewSecret("bar", "foo-cluster-agent", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))
					dcaService := test.NewService("bar", "foo-cluster-agent", &test.NewServiceOptions{Spec: &corev1.ServiceSpec{
						Type: corev1.ServiceTypeClusterIP,
						Selector: map[string]string{
							datadoghqv1alpha1.AgentDeploymentNameLabelKey:      "foo",
							datadoghqv1alpha1.AgentDeploymentComponentLabelKey: "cluster-agent",
						},
						Ports: []corev1.ServicePort{
							{
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(datadoghqv1alpha1.DefaultClusterAgentServicePort),
								Port:       datadoghqv1alpha1.DefaultClusterAgentServicePort,
							},
						},
						SessionAffinity: corev1.ServiceAffinityNone,
					},
					})
					_, _ = comparison.SetMD5GenerationAnnotation(&dcaService.ObjectMeta, dcaService.Spec)
					dcaService.Labels = commonDCAlabels
					_ = c.Create(context.TODO(), dcaService)
					dcaExternalMetricsService := test.NewService("bar", "foo-cluster-agent-metrics-server", &test.NewServiceOptions{Spec: &corev1.ServiceSpec{
						Type: corev1.ServiceTypeClusterIP,
						Selector: map[string]string{
							datadoghqv1alpha1.AgentDeploymentNameLabelKey:      "foo",
							datadoghqv1alpha1.AgentDeploymentComponentLabelKey: "cluster-agent",
						},
						Ports: []corev1.ServicePort{
							{
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(datadoghqv1alpha1.DefaultMetricsServerServicePort),
								Port:       datadoghqv1alpha1.DefaultMetricsServerServicePort,
							},
						},
						SessionAffinity: corev1.ServiceAffinityNone,
					},
					})
					_, _ = comparison.SetMD5GenerationAnnotation(&dcaExternalMetricsService.ObjectMeta, dcaExternalMetricsService.Spec)
					dcaExternalMetricsService.Labels = commonDCAlabels
					_ = c.Create(context.TODO(), dcaExternalMetricsService)
					version := getClusterAgentVersion(dad)
					_ = c.Create(context.TODO(), buildClusterAgentClusterRole(dad, "foo-cluster-agent", version))
					_ = c.Create(context.TODO(), buildClusterRoleBinding(dad, roleBindingInfo{
						name:               "foo-cluster-agent",
						roleName:           "foo-cluster-agent",
						serviceAccountName: "foo-cluster-agent",
					}, version))
					_ = c.Create(context.TODO(), buildMetricsServerClusterRoleBinding(dad, "foo-cluster-agent-system-auth-delegator", version))
					_ = c.Create(context.TODO(), buildClusterAgentPDB(dad))
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				// Make sure Cluster Agent ServiceAccount is created properly
				rbacResourcesNameClusterAgent := "foo-cluster-agent"
				serviceAccount := &corev1.ServiceAccount{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesNameClusterAgent}, serviceAccount); err != nil {
					return err
				}
				if !ownedByDatadogOperator(serviceAccount.OwnerReferences) {
					return fmt.Errorf("bad serviceAccount, should be owned by the datadog operator, current owners: %v", serviceAccount.OwnerReferences)
				}

				return nil
			},
		},
		{
			name: "DatadogAgentDeployment found and defaulted, Cluster Agent Deployment already exists, create Daemonset",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dadOptions := &test.NewDatadogAgentDeploymentOptions{
						Labels:              map[string]string{"label-foo-key": "label-bar-value"},
						Status:              &datadoghqv1alpha1.DatadogAgentDeploymentStatus{},
						ClusterAgentEnabled: true,
					}

					dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", dadOptions)
					_ = c.Create(context.TODO(), dad)
					commonDCAlabels := getDefaultLabels(dad, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dad))
					_ = c.Create(context.TODO(), test.NewSecret("bar", "foo-cluster-agent", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))

					createClusterAgentDependencies(c, dad)

					dcaOptions := &test.NewDeploymentOptions{
						Labels:                 map[string]string{"label-foo-key": "label-bar-value"},
						Annotations:            map[string]string{string(datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey): defaultClusterAgentHash},
						ForceAvailableReplicas: datadoghqv1alpha1.NewInt32Pointer(1),
					}
					dca := test.NewClusterAgentDeployment("bar", "foo", dcaOptions)

					_ = c.Create(context.TODO(), dad)
					_ = c.Create(context.TODO(), dca)

					createAgentDependencies(c, dad)
					resourceName := getAgentRbacResourcesName(dad)
					version := getAgentVersion(dad)
					_ = c.Create(context.TODO(), buildClusterRoleBinding(dad, roleBindingInfo{
						name:               getClusterChecksRunnerRbacResourcesName(dad),
						roleName:           resourceName,
						serviceAccountName: getClusterChecksRunnerServiceAccount(dad),
					}, version))
					_ = c.Create(context.TODO(), buildServiceAccount(dad, getClusterChecksRunnerServiceAccount(dad), version))

				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				ds := &appsv1.DaemonSet{}
				if err := c.Get(context.TODO(), newRequest("bar", "foo").NamespacedName, ds); err != nil {
					return err
				}

				return nil
			},
		},
		{
			name: "DatadogAgentDeployment found and defaulted, Cluster Agent Deployment already exists, block DeploymentName change",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dadOptions := &test.NewDatadogAgentDeploymentOptions{
						Labels: map[string]string{"label-foo-key": "label-bar-value"},
						Status: &datadoghqv1alpha1.DatadogAgentDeploymentStatus{
							ClusterAgent: &datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStatus{
								DeploymentName: "cluster-agent-deployment-before",
							},
						},
						ClusterAgentEnabled:        true,
						ClusterAgentDeploymentName: "cluster-agent-depoyment",
					}

					dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", dadOptions)
					_ = c.Create(context.TODO(), dad)
					commonDCAlabels := getDefaultLabels(dad, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dad))
					_ = c.Create(context.TODO(), test.NewSecret("bar", "foo-cluster-agent", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))

					createClusterAgentDependencies(c, dad)

					dcaOptions := &test.NewDeploymentOptions{
						Labels:                 map[string]string{"label-foo-key": "label-bar-value"},
						Annotations:            map[string]string{string(datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey): defaultClusterAgentHash},
						ForceAvailableReplicas: datadoghqv1alpha1.NewInt32Pointer(1),
					}
					dca := test.NewClusterAgentDeployment("bar", "foo", dcaOptions)

					_ = c.Create(context.TODO(), dad)
					_ = c.Create(context.TODO(), dca)

					createAgentDependencies(c, dad)
					resourceName := getAgentRbacResourcesName(dad)
					version := getAgentVersion(dad)
					_ = c.Create(context.TODO(), buildClusterRoleBinding(dad, roleBindingInfo{
						name:               getClusterChecksRunnerRbacResourcesName(dad),
						roleName:           resourceName,
						serviceAccountName: getClusterChecksRunnerServiceAccount(dad),
					}, version))
					_ = c.Create(context.TODO(), buildServiceAccount(dad, getClusterChecksRunnerServiceAccount(dad), version))

				},
			},
			want:    reconcile.Result{},
			wantErr: true,
			wantFunc: func(c client.Client) error {
				ds := &appsv1.DaemonSet{}
				err := c.Get(context.TODO(), newRequest("bar", "foo").NamespacedName, ds)
				if apierrors.IsNotFound(err) {
					// Daemonset must NOT be created
					return nil
				}
				return err
			},
		},
		/*
			{
				name: "DatadogAgentDeployment found and defaulted, Cluster Agent enabled, block DeploymentName change",
				fields: fields{
					client:   fake.NewFakeClient(),
					scheme:   s,
					recorder: recorder,
				},
				args: args{
					request: newRequest("bar", "foo"),
					loadFunc: func(c client.Client) {
						dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", &test.NewDatadogAgentDeploymentOptions{Labels: map[string]string{"label-foo-key": "label-bar-value"}, ClusterAgentEnabled: true})
						dad.Status.ClusterAgent = &datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStatus{
							DeploymentName: "cluster-agent-prev-name",
						}
						_ = c.Create(context.TODO(), dad)
						// commonDCAlabels := getDefaultLabels(dad, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dad))
						// _ = c.Create(context.TODO(), test.NewSecret("bar", "foo-cluster-agent", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						// 	"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
						// }}))
					},
				},
				want:    reconcile.Result{},
				wantErr: true,
				wantFunc: func(c client.Client) error {
					dcaService := &corev1.Service{}
					err := c.Get(context.TODO(), newRequest("bar", "foo-cluster-agent").NamespacedName, dcaService)
					if apierrors.IsNotFound(err) {
						// Daemonset must NOT be created
						return nil
					}
					return err
				},
			},

		*/
		{
			name: "DatadogAgentDeployment found and defaulted, Cluster Agent Deployment already exists but with 0 pods ready, do not create Daemonset",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dadOptions := &test.NewDatadogAgentDeploymentOptions{
						Labels:              map[string]string{"label-foo-key": "label-bar-value"},
						Status:              &datadoghqv1alpha1.DatadogAgentDeploymentStatus{},
						ClusterAgentEnabled: true,
					}

					dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", dadOptions)
					_ = c.Create(context.TODO(), dad)
					commonDCAlabels := getDefaultLabels(dad, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dad))
					_ = c.Create(context.TODO(), test.NewSecret("bar", "foo-cluster-agent", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))

					createClusterAgentDependencies(c, dad)

					dcaOptions := &test.NewDeploymentOptions{
						Labels:                 map[string]string{"label-foo-key": "label-bar-value"},
						Annotations:            map[string]string{string(datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey): defaultClusterAgentHash},
						ForceAvailableReplicas: datadoghqv1alpha1.NewInt32Pointer(0),
					}
					dca := test.NewClusterAgentDeployment("bar", "foo", dcaOptions)

					_ = c.Create(context.TODO(), dad)
					_ = c.Create(context.TODO(), dca)

					createAgentDependencies(c, dad)
					resourceName := getAgentRbacResourcesName(dad)
					version := getAgentVersion(dad)
					_ = c.Create(context.TODO(), buildClusterRoleBinding(dad, roleBindingInfo{
						name:               getClusterChecksRunnerRbacResourcesName(dad),
						roleName:           resourceName,
						serviceAccountName: getClusterChecksRunnerServiceAccount(dad),
					}, version))
					_ = c.Create(context.TODO(), buildServiceAccount(dad, getClusterChecksRunnerServiceAccount(dad), version))

				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeuPeriod},
			wantErr: true,
			wantFunc: func(c client.Client) error {
				ds := &appsv1.DaemonSet{}
				err := c.Get(context.TODO(), newRequest("bar", "foo").NamespacedName, ds)
				if apierrors.IsNotFound(err) {
					// The Cluster Agent exists but not available yet
					// Daemonset must NOT be created
					return nil
				}
				return err
			},
		},
		{
			name: "DatadogAgentDeployment found and defaulted, Cluster Checks Runner PDB Creation",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dadOptions := &test.NewDatadogAgentDeploymentOptions{
						Labels:                     map[string]string{"label-foo-key": "label-bar-value"},
						Status:                     &datadoghqv1alpha1.DatadogAgentDeploymentStatus{},
						ClusterAgentEnabled:        true,
						ClusterChecksRunnerEnabled: true,
					}
					dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", dadOptions)
					_ = c.Create(context.TODO(), dad)

					commonDCAlabels := getDefaultLabels(dad, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dad))
					dcaLabels := map[string]string{"label-foo-key": "label-bar-value"}
					for k, v := range commonDCAlabels {
						dcaLabels[k] = v
					}

					dcaOptions := &test.NewDeploymentOptions{
						Labels:                 dcaLabels,
						Annotations:            map[string]string{string(datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey): defaultClusterAgentHash},
						ForceAvailableReplicas: datadoghqv1alpha1.NewInt32Pointer(1),
					}
					dca := test.NewClusterAgentDeployment("bar", "foo", dcaOptions)

					_ = c.Create(context.TODO(), dad)
					_ = c.Create(context.TODO(), dca)
					_ = c.Create(context.TODO(), test.NewSecret("bar", "foo-cluster-agent", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))

					createClusterAgentDependencies(c, dad)
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				pdb := &policyv1.PodDisruptionBudget{}
				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: "bar", Name: "foo-cluster-checks-runner"}, pdb); err != nil {
					return err
				}
				if !ownedByDatadogOperator(pdb.OwnerReferences) {
					return fmt.Errorf("bad PDB, should be owned by the datadog operator, current owners: %v", pdb.OwnerReferences)
				}

				return nil
			},
		},
		{
			name: "DatadogAgentDeployment found and defaulted, Cluster Checks Runner PDB Update",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dadOptions := &test.NewDatadogAgentDeploymentOptions{
						Labels:                     map[string]string{"label-foo-key": "label-bar-value"},
						Status:                     &datadoghqv1alpha1.DatadogAgentDeploymentStatus{},
						ClusterAgentEnabled:        true,
						ClusterChecksRunnerEnabled: true,
					}
					dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", dadOptions)
					_ = c.Create(context.TODO(), dad)

					commonDCAlabels := getDefaultLabels(dad, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dad))
					dcaLabels := map[string]string{"label-foo-key": "label-bar-value"}
					for k, v := range commonDCAlabels {
						dcaLabels[k] = v
					}

					dcaOptions := &test.NewDeploymentOptions{
						Labels:                 dcaLabels,
						Annotations:            map[string]string{string(datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey): defaultClusterAgentHash},
						ForceAvailableReplicas: datadoghqv1alpha1.NewInt32Pointer(1),
					}
					dca := test.NewClusterAgentDeployment("bar", "foo", dcaOptions)

					_ = c.Create(context.TODO(), dad)
					_ = c.Create(context.TODO(), dca)
					_ = c.Create(context.TODO(), test.NewSecret("bar", "foo-cluster-agent", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))

					createClusterAgentDependencies(c, dad)

					// Create wrong value PDB
					pdb := buildClusterChecksRunnerPDB(dad)
					wrongMinAvailable := intstr.FromInt(10)
					pdb.Spec.MinAvailable = &wrongMinAvailable
					_ = controllerutil.SetControllerReference(dad, pdb, s)
					_ = c.Create(context.TODO(), pdb)
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				pdb := &policyv1.PodDisruptionBudget{}
				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: "bar", Name: "foo-cluster-checks-runner"}, pdb); err != nil {
					return err
				}
				if pdb.Spec.MinAvailable.IntValue() != pdbMinAvailableInstances {
					return fmt.Errorf("MinAvailable incorrect, expected %d, got %d", pdbMinAvailableInstances, pdb.Spec.MinAvailable.IntValue())
				}

				return nil
			},
		},
		{
			name: "DatadogAgentDeployment found and defaulted, Cluster Checks Runner ClusterRoleBidning creation",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dadOptions := &test.NewDatadogAgentDeploymentOptions{
						Labels:                     map[string]string{"label-foo-key": "label-bar-value"},
						Status:                     &datadoghqv1alpha1.DatadogAgentDeploymentStatus{},
						ClusterAgentEnabled:        true,
						ClusterChecksRunnerEnabled: true,
					}
					dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", dadOptions)
					_ = c.Create(context.TODO(), dad)

					commonDCAlabels := getDefaultLabels(dad, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dad))
					dcaLabels := map[string]string{"label-foo-key": "label-bar-value"}
					for k, v := range commonDCAlabels {
						dcaLabels[k] = v
					}

					dcaOptions := &test.NewDeploymentOptions{
						Labels:                 dcaLabels,
						Annotations:            map[string]string{string(datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey): defaultClusterAgentHash},
						ForceAvailableReplicas: datadoghqv1alpha1.NewInt32Pointer(1),
					}
					dca := test.NewClusterAgentDeployment("bar", "foo", dcaOptions)

					_ = c.Create(context.TODO(), dad)
					_ = c.Create(context.TODO(), dca)
					_ = c.Create(context.TODO(), test.NewSecret("bar", "foo-cluster-agent", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))

					createClusterAgentDependencies(c, dad)
					createClusterChecksRunnerDependencies(c, dad)
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				rbacResourcesNameClusterChecksRunner := "foo-cluster-checks-runner"
				clusterRoleBinding := &rbacv1.ClusterRoleBinding{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesNameClusterChecksRunner}, clusterRoleBinding); err != nil {
					return err
				}
				if !ownedByDatadogOperator(clusterRoleBinding.OwnerReferences) {
					return fmt.Errorf("bad clusterRoleBinding, should be owned by the datadog operator, current owners: %v", clusterRoleBinding.OwnerReferences)
				}

				return nil
			},
		},
		{
			name: "DatadogAgentDeployment found and defaulted, Cluster Checks Runner Service Account creation",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dadOptions := &test.NewDatadogAgentDeploymentOptions{
						Labels:                     map[string]string{"label-foo-key": "label-bar-value"},
						Status:                     &datadoghqv1alpha1.DatadogAgentDeploymentStatus{},
						ClusterAgentEnabled:        true,
						ClusterChecksRunnerEnabled: true,
					}
					dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", dadOptions)
					_ = c.Create(context.TODO(), dad)

					commonDCAlabels := getDefaultLabels(dad, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dad))
					dcaLabels := map[string]string{"label-foo-key": "label-bar-value"}
					for k, v := range commonDCAlabels {
						dcaLabels[k] = v
					}

					dcaOptions := &test.NewDeploymentOptions{
						Labels:                 dcaLabels,
						Annotations:            map[string]string{string(datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey): defaultClusterAgentHash},
						ForceAvailableReplicas: datadoghqv1alpha1.NewInt32Pointer(1),
					}
					dca := test.NewClusterAgentDeployment("bar", "foo", dcaOptions)

					_ = c.Create(context.TODO(), dad)
					_ = c.Create(context.TODO(), dca)
					_ = c.Create(context.TODO(), test.NewSecret("bar", "foo-cluster-agent", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))

					createClusterAgentDependencies(c, dad)
					createClusterChecksRunnerDependencies(c, dad)

					version := getClusterChecksRunnerVersion(dad)
					_ = c.Create(context.TODO(), buildClusterRoleBinding(dad, roleBindingInfo{
						name:               "foo-cluster-checks-runner",
						roleName:           "foo-agent",
						serviceAccountName: "foo-cluster-checks-runner",
					}, version))
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				rbacResourcesNameClusterChecksRunner := "foo-cluster-checks-runner"
				serviceAccount := &corev1.ServiceAccount{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesNameClusterChecksRunner}, serviceAccount); err != nil {
					return err
				}
				if !ownedByDatadogOperator(serviceAccount.OwnerReferences) {
					return fmt.Errorf("bad serviceAccount, should be owned by the datadog operator, current owners: %v", serviceAccount.OwnerReferences)
				}

				return nil
			},
		},
		{
			name: "DatadogAgentDeployment found and defaulted, Cluster Checks Runner Deployment creation",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dadOptions := &test.NewDatadogAgentDeploymentOptions{
						Labels:                     map[string]string{"label-foo-key": "label-bar-value"},
						Status:                     &datadoghqv1alpha1.DatadogAgentDeploymentStatus{},
						ClusterAgentEnabled:        true,
						ClusterChecksRunnerEnabled: true,
					}
					dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", dadOptions)
					_ = c.Create(context.TODO(), dad)

					commonDCAlabels := getDefaultLabels(dad, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dad))
					dcaLabels := map[string]string{"label-foo-key": "label-bar-value"}
					for k, v := range commonDCAlabels {
						dcaLabels[k] = v
					}

					dcaOptions := &test.NewDeploymentOptions{
						Labels:                 dcaLabels,
						Annotations:            map[string]string{string(datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey): defaultClusterAgentHash},
						ForceAvailableReplicas: datadoghqv1alpha1.NewInt32Pointer(1),
					}
					dca := test.NewClusterAgentDeployment("bar", "foo", dcaOptions)

					_ = c.Create(context.TODO(), dad)
					_ = c.Create(context.TODO(), dca)
					_ = c.Create(context.TODO(), test.NewSecret("bar", "foo-cluster-agent", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))

					createClusterAgentDependencies(c, dad)
					createAgentDependencies(c, dad)
					createClusterChecksRunnerDependencies(c, dad)

					resourceName := getAgentRbacResourcesName(dad)
					version := getAgentVersion(dad)
					_ = c.Create(context.TODO(), buildClusterRoleBinding(dad, roleBindingInfo{
						name:               getClusterChecksRunnerRbacResourcesName(dad),
						roleName:           resourceName,
						serviceAccountName: getClusterChecksRunnerServiceAccount(dad),
					}, version))
					_ = c.Create(context.TODO(), buildServiceAccount(dad, getClusterChecksRunnerServiceAccount(dad), version))

				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				dca := &appsv1.Deployment{}
				if err := c.Get(context.TODO(), newRequest("bar", "foo-cluster-agent").NamespacedName, dca); err != nil {
					return err
				}
				if dca.Name != "foo-cluster-agent" {
					return fmt.Errorf("dca bad name, should be: 'foo', current: %s", dca.Name)
				}

				dcaw := &appsv1.Deployment{}
				if err := c.Get(context.TODO(), newRequest("bar", "foo-cluster-checks-runner").NamespacedName, dcaw); err != nil {
					return err
				}
				if dcaw.Name != "foo-cluster-checks-runner" {
					return fmt.Errorf("dcaw bad name, should be: 'foo', current: %s", dcaw.Name)
				}

				return nil
			},
		},
		{
			name: "DatadogAgentDeployment found and defaulted, Cluster Agent Deployment already exists but not up-to-date",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest("bar", "foo"),
				loadFunc: func(c client.Client) {
					dadOptions := &test.NewDatadogAgentDeploymentOptions{
						Labels:              map[string]string{"label-foo-key": "label-bar-value"},
						Status:              &datadoghqv1alpha1.DatadogAgentDeploymentStatus{},
						ClusterAgentEnabled: true,
					}
					dad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", dadOptions)
					_ = c.Create(context.TODO(), dad)

					commonDCAlabels := getDefaultLabels(dad, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dad))
					dcaLabels := map[string]string{"label-foo-key": "label-bar-value"}
					for k, v := range commonDCAlabels {
						dcaLabels[k] = v
					}
					dcaOptions := &test.NewDeploymentOptions{
						Labels:      dcaLabels,
						Annotations: map[string]string{datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey: "outdated-hash"},
					}
					dca := test.NewClusterAgentDeployment("bar", "foo-cluster-agent", dcaOptions)

					_ = c.Create(context.TODO(), dad)
					_ = c.Create(context.TODO(), dca)
					_ = c.Create(context.TODO(), test.NewSecret("bar", "foo-cluster-agent", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))

					createClusterAgentDependencies(c, dad)
					createClusterChecksRunnerDependencies(c, dad)
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				dca := &appsv1.Deployment{}
				if err := c.Get(context.TODO(), newRequest("bar", "foo-cluster-agent").NamespacedName, dca); err != nil {
					return err
				}
				if dca.Annotations[datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey] == "outdated-hash" || dca.Annotations[datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey] == "" {
					return fmt.Errorf("dca bad hash, should be updated, current: %s", dca.Annotations[datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey])
				}
				if dca.OwnerReferences == nil || len(dca.OwnerReferences) != 1 {
					return fmt.Errorf("dca bad owner references, should be: '[Kind DatadogAgentDeployment - Name foo]', current: %v", dca.OwnerReferences)
				}

				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log = logf.Log.WithName(tt.name)
			r := &ReconcileDatadogAgentDeployment{
				client:     tt.fields.client,
				scheme:     tt.fields.scheme,
				recorder:   recorder,
				forwarders: forwarders,
			}
			if tt.args.loadFunc != nil {
				tt.args.loadFunc(r.client)
			}
			got, err := r.Reconcile(tt.args.request)
			if tt.wantErr {
				assert.Error(t, err, "ReconcileDatadogAgentDeployment.Reconcile() expected an error")
			} else {
				assert.NoError(t, err, "ReconcileDatadogAgentDeployment.Reconcile() unexpected error: %v", err)
			}

			assert.Equal(t, tt.want, got, "ReconcileDatadogAgentDeployment.Reconcile() unexpected result")

			if tt.wantFunc != nil {
				err := tt.wantFunc(r.client)
				assert.NoError(t, err, "ReconcileDatadogAgentDeployment.Reconcile() wantFunc validation error: %v", err)
			}
		})
	}
}

func Test_newClusterAgentDeploymentFromInstance(t *testing.T) {
	logf.SetLogger(logf.ZapLogger(true))
	authTokenValue := &corev1.EnvVarSource{
		SecretKeyRef: &corev1.SecretKeySelector{},
	}
	dadName := "foo"
	authTokenValue.SecretKeyRef.Name = fmt.Sprintf("%s-%s", dadName, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix)
	authTokenValue.SecretKeyRef.Key = "token"
	replicas := int32(1)
	defaultPodSpec := corev1.PodSpec{
		Affinity:           getPodAffinity(nil, "foo-cluster-agent"),
		ServiceAccountName: "foo-cluster-agent",
		Containers: []corev1.Container{
			{
				Name:            "cluster-agent",
				Image:           "datadog/cluster-agent:latest",
				ImagePullPolicy: corev1.PullIfNotPresent,
				Resources:       corev1.ResourceRequirements{},
				Ports: []corev1.ContainerPort{
					{
						ContainerPort: 5005,
						Name:          "agentport",
						Protocol:      "TCP",
					},
					{
						ContainerPort: 443,
						Name:          "metricsapi",
						Protocol:      "TCP",
					},
				},
				Env: []corev1.EnvVar{
					{
						Name:  "DD_CLUSTER_NAME",
						Value: "",
					},
					{
						Name:  "DD_SITE",
						Value: "",
					},
					{
						Name:  "DD_DD_URL",
						Value: "https://app.datadoghq.com",
					},
					{
						Name:  "DD_CLUSTER_CHECKS_ENABLED",
						Value: "false",
					},
					{
						Name:  "DD_CLUSTER_AGENT_KUBERNETES_SERVICE_NAME",
						Value: fmt.Sprintf("%s-%s", dadName, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix),
					},
					{
						Name:      "DD_CLUSTER_AGENT_AUTH_TOKEN",
						ValueFrom: authTokenValue,
					},
					{
						Name:  "DD_LEADER_ELECTION",
						Value: "true",
					},
					{
						Name:  "DD_API_KEY",
						Value: "",
					},
				},
			},
		},
	}

	userVolumes := []corev1.Volume{
		{
			Name: "tmp",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: "/tmp",
				},
			},
		},
	}
	userVolumeMounts := []corev1.VolumeMount{
		{
			Name:      "tmp",
			MountPath: "/some/path",
			ReadOnly:  true,
		},
	}
	userMountsPodSpec := defaultPodSpec.DeepCopy()
	userMountsPodSpec.Volumes = append(userMountsPodSpec.Volumes, userVolumes...)
	userMountsPodSpec.Containers[0].VolumeMounts = append(userMountsPodSpec.Containers[0].VolumeMounts, userVolumeMounts...)

	userMountsAgentDeployment := test.NewDefaultedDatadogAgentDeployment(
		"bar",
		"foo",
		&test.NewDatadogAgentDeploymentOptions{
			ClusterAgentEnabled:      true,
			ClusterAgentVolumes:      userVolumes,
			ClusterAgentVolumeMounts: userVolumeMounts,
		},
	)
	userMountsClusterAgentHash, _ := comparison.GenerateMD5ForSpec(userMountsAgentDeployment.Spec.ClusterAgent)

	customDeploymentName := "custom-cluster-agent-deployment"
	deploymentNamePodSpec := defaultPodSpec.DeepCopy()
	deploymentNamePodSpec.Affinity = getPodAffinity(nil, customDeploymentName)

	deploymentNameAgentDeployment := test.NewDefaultedDatadogAgentDeployment("bar", "foo",
		&test.NewDatadogAgentDeploymentOptions{
			UseEDS:                     true,
			ClusterAgentEnabled:        true,
			ClusterAgentDeploymentName: customDeploymentName,
		})

	deploymentNameClusterAgentHash, _ := comparison.GenerateMD5ForSpec(deploymentNameAgentDeployment.Spec.ClusterAgent)

	tests := []struct {
		name            string
		agentdeployment *datadoghqv1alpha1.DatadogAgentDeployment
		selector        *metav1.LabelSelector
		newStatus       *datadoghqv1alpha1.DatadogAgentDeploymentStatus
		want            *appsv1.Deployment
		wantErr         bool
	}{
		{
			name:            "defaulted case",
			agentdeployment: test.NewDefaultedDatadogAgentDeployment("bar", "foo", &test.NewDatadogAgentDeploymentOptions{ClusterAgentEnabled: true}),
			newStatus:       &datadoghqv1alpha1.DatadogAgentDeploymentStatus{},
			wantErr:         false,
			want: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "bar",
					Name:      "foo-cluster-agent",
					Labels: map[string]string{"agentdeployment.datadoghq.com/name": "foo",
						"agentdeployment.datadoghq.com/component": "cluster-agent",
						"app.kubernetes.io/instance":              "cluster-agent",
						"app.kubernetes.io/managed-by":            "datadog-operator",
						"app.kubernetes.io/name":                  "datadog-agent-deployment",
						"app.kubernetes.io/part-of":               "foo",
						"app.kubernetes.io/version":               "",
					},
					Annotations: map[string]string{"agentdeployment.datadoghq.com/agentspechash": defaultClusterAgentHash},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"agentdeployment.datadoghq.com/name":      "foo",
								"agentdeployment.datadoghq.com/component": "cluster-agent",
								"app.kubernetes.io/instance":              "cluster-agent",
								"app.kubernetes.io/managed-by":            "datadog-operator",
								"app.kubernetes.io/name":                  "datadog-agent-deployment",
								"app.kubernetes.io/part-of":               "foo",
								"app.kubernetes.io/version":               "",
							},
						},
						Spec: defaultPodSpec,
					},
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"agentdeployment.datadoghq.com/name":      "foo",
							"agentdeployment.datadoghq.com/component": "cluster-agent",
						},
					},
				},
			},
		},
		{
			name:            "with labels and annotations",
			agentdeployment: test.NewDefaultedDatadogAgentDeployment("bar", "foo", &test.NewDatadogAgentDeploymentOptions{ClusterAgentEnabled: true, Labels: map[string]string{"label-foo-key": "label-bar-value"}, Annotations: map[string]string{"annotations-foo-key": "annotations-bar-value"}}),
			newStatus:       &datadoghqv1alpha1.DatadogAgentDeploymentStatus{},
			wantErr:         false,
			want: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "bar",
					Name:      "foo-cluster-agent",
					Labels: map[string]string{
						"agentdeployment.datadoghq.com/name":      "foo",
						"agentdeployment.datadoghq.com/component": "cluster-agent",
						"label-foo-key":                "label-bar-value",
						"app.kubernetes.io/instance":   "cluster-agent",
						"app.kubernetes.io/managed-by": "datadog-operator",
						"app.kubernetes.io/name":       "datadog-agent-deployment",
						"app.kubernetes.io/part-of":    "foo",
						"app.kubernetes.io/version":    "",
					},
					Annotations: map[string]string{"agentdeployment.datadoghq.com/agentspechash": defaultClusterAgentHash, "annotations-foo-key": "annotations-bar-value"},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"agentdeployment.datadoghq.com/name":      "foo",
								"agentdeployment.datadoghq.com/component": "cluster-agent",
								"label-foo-key":                "label-bar-value",
								"app.kubernetes.io/instance":   "cluster-agent",
								"app.kubernetes.io/managed-by": "datadog-operator",
								"app.kubernetes.io/name":       "datadog-agent-deployment",
								"app.kubernetes.io/part-of":    "foo",
								"app.kubernetes.io/version":    "",
							},
						},
						Spec: defaultPodSpec,
					},
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"agentdeployment.datadoghq.com/name":      "foo",
							"agentdeployment.datadoghq.com/component": "cluster-agent",
						},
					},
				},
			},
		},
		{
			name:            "with user volumes and mounts",
			agentdeployment: userMountsAgentDeployment,
			newStatus:       &datadoghqv1alpha1.DatadogAgentDeploymentStatus{},
			wantErr:         false,
			want: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "bar",
					Name:      "foo-cluster-agent",
					Labels: map[string]string{"agentdeployment.datadoghq.com/name": "foo",
						"agentdeployment.datadoghq.com/component": "cluster-agent",
						"app.kubernetes.io/instance":              "cluster-agent",
						"app.kubernetes.io/managed-by":            "datadog-operator",
						"app.kubernetes.io/name":                  "datadog-agent-deployment",
						"app.kubernetes.io/part-of":               "foo",
						"app.kubernetes.io/version":               "",
					},
					Annotations: map[string]string{"agentdeployment.datadoghq.com/agentspechash": userMountsClusterAgentHash},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"agentdeployment.datadoghq.com/name":      "foo",
								"agentdeployment.datadoghq.com/component": "cluster-agent",
								"app.kubernetes.io/instance":              "cluster-agent",
								"app.kubernetes.io/managed-by":            "datadog-operator",
								"app.kubernetes.io/name":                  "datadog-agent-deployment",
								"app.kubernetes.io/part-of":               "foo",
								"app.kubernetes.io/version":               "",
							},
						},
						Spec: *userMountsPodSpec,
					},
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"agentdeployment.datadoghq.com/name":      "foo",
							"agentdeployment.datadoghq.com/component": "cluster-agent",
						},
					},
				},
			},
		},
		{
			name:            "with custom deployment name and selector",
			agentdeployment: deploymentNameAgentDeployment,
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "datadog-monitoring",
				},
			},
			newStatus: &datadoghqv1alpha1.DatadogAgentDeploymentStatus{},
			wantErr:   false,
			want: &appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "bar",
					Name:      customDeploymentName,
					Labels: map[string]string{"agentdeployment.datadoghq.com/name": "foo",
						"agentdeployment.datadoghq.com/component": "cluster-agent",
						"app.kubernetes.io/instance":              "cluster-agent",
						"app.kubernetes.io/managed-by":            "datadog-operator",
						"app.kubernetes.io/name":                  "datadog-agent-deployment",
						"app.kubernetes.io/part-of":               "foo",
						"app.kubernetes.io/version":               "",
						"app":                                     "datadog-monitoring",
					},
					Annotations: map[string]string{"agentdeployment.datadoghq.com/agentspechash": deploymentNameClusterAgentHash},
				},
				Spec: appsv1.DeploymentSpec{
					Template: corev1.PodTemplateSpec{
						ObjectMeta: metav1.ObjectMeta{
							Labels: map[string]string{
								"agentdeployment.datadoghq.com/name":      "foo",
								"agentdeployment.datadoghq.com/component": "cluster-agent",
								"app.kubernetes.io/instance":              "cluster-agent",
								"app.kubernetes.io/managed-by":            "datadog-operator",
								"app.kubernetes.io/name":                  "datadog-agent-deployment",
								"app.kubernetes.io/part-of":               "foo",
								"app.kubernetes.io/version":               "",
								"app":                                     "datadog-monitoring",
							},
						},
						Spec: *deploymentNamePodSpec,
					},
					Replicas: &replicas,
					Selector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"app": "datadog-monitoring",
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reqLogger := log.WithValues("test:", tt.name)
			got, _, err := newClusterAgentDeploymentFromInstance(reqLogger, tt.agentdeployment, tt.newStatus, tt.selector)
			if tt.wantErr {
				assert.Error(t, err, "newClusterAgentDeploymentFromInstance() expected an error")
			} else {
				assert.NoError(t, err, "newClusterAgentDeploymentFromInstance() unexpected error: %v", err)
			}
			assert.True(t, apiequality.Semantic.DeepEqual(got, tt.want), "newClusterAgentDeploymentFromInstance() = %#v, want %#v\ndiff = %s", got, tt.want,
				cmp.Diff(got, tt.want))
		})
	}
}

func TestReconcileDatadogAgentDeployment_createNewClusterAgentDeployment(t *testing.T) {
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconcileDatadogAgentDeployment_createNewClusterAgentDeployment"})
	forwarders := dummyManager{}

	logf.SetLogger(logf.ZapLogger(true))
	log := logf.Log.WithName("TestReconcileDatadogAgentDeployment_createNewClusterAgentDeployment")

	s := scheme.Scheme

	type fields struct {
		client   client.Client
		scheme   *runtime.Scheme
		recorder record.EventRecorder
	}
	type args struct {
		logger          logr.Logger
		agentdeployment *datadoghqv1alpha1.DatadogAgentDeployment
		newStatus       *datadoghqv1alpha1.DatadogAgentDeploymentStatus
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    reconcile.Result
		wantErr bool
	}{
		{
			name: "create new DCA",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				logger:          log,
				agentdeployment: test.NewDefaultedDatadogAgentDeployment("bar", "foo", &test.NewDatadogAgentDeploymentOptions{ClusterAgentEnabled: true}),
				newStatus:       &datadoghqv1alpha1.DatadogAgentDeploymentStatus{},
			},
			want:    reconcile.Result{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &ReconcileDatadogAgentDeployment{
				client:     tt.fields.client,
				scheme:     tt.fields.scheme,
				recorder:   recorder,
				forwarders: forwarders,
			}
			got, err := r.createNewClusterAgentDeployment(tt.args.logger, tt.args.agentdeployment, tt.args.newStatus)
			if tt.wantErr {
				assert.Error(t, err, "ReconcileDatadogAgentDeployment.createNewClusterAgentDeployment() should return an error")
			} else {
				assert.NoError(t, err, "ReconcileDatadogAgentDeployment.createNewClusterAgentDeployment() unexpected error: %v", err)
			}
			assert.Equal(t, tt.want, got, "ReconcileDatadogAgentDeployment.createNewClusterAgentDeployment() unexpected result")
		})
	}
}

func newRequest(ns, name string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: ns,
			Name:      name,
		},
	}
}

func containsEnv(slice []corev1.EnvVar, name, value string) bool {
	for _, element := range slice {
		if element.Name == name && element.Value == value {
			return true
		}
	}
	return false
}

func containsVolumeMounts(slice []corev1.VolumeMount, name, path string) bool {
	for _, element := range slice {
		if element.Name == name && element.MountPath == path {
			return true
		}
	}
	return false
}

func hasAllClusterLevelRbacResources(policyRules []rbacv1.PolicyRule) bool {
	clusterLevelResources := map[string]bool{
		"services":              true,
		"events":                true,
		"pods":                  true,
		"nodes":                 true,
		"componentstatuses":     true,
		"clusterresourcequotas": true,
	}
	for _, policyRule := range policyRules {
		for _, resource := range policyRule.Resources {
			if _, found := clusterLevelResources[resource]; found {
				delete(clusterLevelResources, resource)
			}
		}
	}
	return len(clusterLevelResources) == 0
}

func hasAllNodeLevelRbacResources(policyRules []rbacv1.PolicyRule) bool {
	nodeLevelResources := map[string]bool{
		"endpoints":     true,
		"nodes/metrics": true,
		"nodes/spec":    true,
		"nodes/proxy":   true,
	}
	for _, policyRule := range policyRules {
		for _, resource := range policyRule.Resources {
			if _, found := nodeLevelResources[resource]; found {
				delete(nodeLevelResources, resource)
			}
		}
	}
	return len(nodeLevelResources) == 0
}

func createSystemProbeDependencies(c client.Client, dad *datadoghqv1alpha1.DatadogAgentDeployment) {
	configCM, _ := buildSystemProbeConfigConfiMap(dad)
	securityCM, _ := buildSystemProbeSecCompConfigMap(dad)
	_ = c.Create(context.TODO(), configCM)
	_ = c.Create(context.TODO(), securityCM)
}

func createAgentDependencies(c client.Client, dad *datadoghqv1alpha1.DatadogAgentDeployment) {
	resourceName := getAgentRbacResourcesName(dad)
	version := getAgentVersion(dad)
	_ = c.Create(context.TODO(), buildAgentClusterRole(dad, resourceName, version))
	_ = c.Create(context.TODO(), buildClusterRoleBinding(dad, roleBindingInfo{
		name:               resourceName,
		roleName:           resourceName,
		serviceAccountName: getAgentServiceAccount(dad),
	}, version))
	_ = c.Create(context.TODO(), buildServiceAccount(dad, getAgentServiceAccount(dad), version))
}

func createClusterAgentDependencies(c client.Client, dad *datadoghqv1alpha1.DatadogAgentDeployment) {
	version := getAgentVersion(dad)
	clusterAgentSAName := getClusterAgentServiceAccount(dad)
	_ = c.Create(context.TODO(), buildClusterAgentClusterRole(dad, "foo-cluster-agent", version))
	_ = c.Create(context.TODO(), buildClusterAgentRole(dad, "foo-cluster-agent", version))
	_ = c.Create(context.TODO(), buildServiceAccount(dad, clusterAgentSAName, version))
	_ = c.Create(context.TODO(), buildClusterRoleBinding(dad, roleBindingInfo{
		name:               "foo-cluster-agent",
		roleName:           "foo-cluster-agent",
		serviceAccountName: clusterAgentSAName,
	}, version))
	_ = c.Create(context.TODO(), buildClusterAgentPDB(dad))

	dcaService := test.NewService("bar", "foo-cluster-agent", &test.NewServiceOptions{Spec: &corev1.ServiceSpec{
		Type: corev1.ServiceTypeClusterIP,
		Selector: map[string]string{
			datadoghqv1alpha1.AgentDeploymentNameLabelKey:      "foo",
			datadoghqv1alpha1.AgentDeploymentComponentLabelKey: "cluster-agent",
		},
		Ports: []corev1.ServicePort{
			{
				Protocol:   corev1.ProtocolTCP,
				TargetPort: intstr.FromInt(datadoghqv1alpha1.DefaultClusterAgentServicePort),
				Port:       datadoghqv1alpha1.DefaultClusterAgentServicePort,
			},
		},
		SessionAffinity: corev1.ServiceAffinityNone,
	},
	})
	_, _ = comparison.SetMD5GenerationAnnotation(&dcaService.ObjectMeta, dcaService.Spec)
	dcaService.Labels = getDefaultLabels(dad, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dad))
	_ = c.Create(context.TODO(), dcaService)
}

// dummyManager mocks the metric forwarder by implementing the metricForwardersManager interface
// the metricForwardersManager logic is tested in the util/datadog package
type dummyManager struct {
}

func (dummyManager) Register(datadog.MonitoredObject) {
}

func (dummyManager) Unregister(datadog.MonitoredObject) {
}

func (dummyManager) ProcessError(datadog.MonitoredObject, error) {
}

func (dummyManager) ProcessEvent(datadog.MonitoredObject, datadog.Event) {
}

func createClusterChecksRunnerDependencies(c client.Client, dad *datadoghqv1alpha1.DatadogAgentDeployment) {
	_ = c.Create(context.TODO(), buildClusterChecksRunnerPDB(dad))
}

func init() {
	// init default hashes global variables for a bar/foo datadog agent deployment default config
	ad := test.NewDefaultedDatadogAgentDeployment("bar", "foo", &test.NewDatadogAgentDeploymentOptions{UseEDS: true, ClusterAgentEnabled: true, Labels: map[string]string{"label-foo-key": "label-bar-value"}})
	defaultAgentHash, _ = comparison.GenerateMD5ForSpec(ad.Spec)
	defaultClusterAgentHash, _ = comparison.GenerateMD5ForSpec(ad.Spec.ClusterAgent)
}
