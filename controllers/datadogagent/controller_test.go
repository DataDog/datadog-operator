// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagent

import (
	"context"
	"encoding/base64"
	"reflect"
	"time"

	"fmt"
	"testing"

	"github.com/pkg/errors"
	assert "github.com/stretchr/testify/require"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
	test "github.com/DataDog/datadog-operator/api/v1alpha1/test"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var defaultAgentHash string

func init() {
	const resourcesName = "foo"
	const resourcesNamespace = "bar"

	// init default hashes global variables for a bar/foo datadog agent deployment default config
	ad := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, &test.NewDatadogAgentOptions{UseEDS: true, ClusterAgentEnabled: true, Labels: map[string]string{"label-foo-key": "label-bar-value"}})
	defaultAgentHash, _ = comparison.GenerateMD5ForSpec(ad.Spec)
}

func TestReconcileDatadogAgent_createNewExtendedDaemonSet(t *testing.T) {
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconcileDatadogAgent_createNewExtendedDaemonSet"})
	forwarders := dummyManager{}

	logf.SetLogger(logf.ZapLogger(true))
	localLog := logf.Log.WithName("TestReconcileDatadogAgent_createNewExtendedDaemonSet")

	const resourcesName = "foo"
	const resourcesNamespace = "bar"

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogAgent{})
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &edsdatadoghqv1alpha1.ExtendedDaemonSet{})
	s.AddKnownTypes(appsv1.SchemeGroupVersion, &appsv1.DaemonSet{})

	type fields struct {
		client   client.Client
		scheme   *runtime.Scheme
		recorder record.EventRecorder
	}
	type args struct {
		logger          logr.Logger
		agentdeployment *datadoghqv1alpha1.DatadogAgent
		newStatus       *datadoghqv1alpha1.DatadogAgentStatus
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
				logger:          localLog,
				agentdeployment: test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, nil),
				newStatus:       &datadoghqv1alpha1.DatadogAgentStatus{},
			},
			want:    reconcile.Result{},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{
				client:     tt.fields.client,
				scheme:     tt.fields.scheme,
				recorder:   recorder,
				forwarders: forwarders,
				options: ReconcilerOptions{
					SupportExtendedDaemonset: true,
				},
			}
			got, err := r.createNewExtendedDaemonSet(tt.args.logger, tt.args.agentdeployment, tt.args.newStatus)
			if tt.wantErr {
				assert.Error(t, err, "ReconcileDatadogAgent.createNewExtendedDaemonSet() expected an error")
			} else {
				assert.NoError(t, err, "ReconcileDatadogAgent.createNewExtendedDaemonSet() unexpected error: %v", err)
			}
			assert.Equal(t, tt.want, got, "ReconcileDatadogAgent.createNewExtendedDaemonSet() unexpected result")
		})
	}
}

func TestReconcileDatadogAgent_Reconcile(t *testing.T) {
	const resourcesName = "foo"
	const resourcesNamespace = "bar"
	const dsName = "foo-agent"
	const rbacResourcesName = "foo-agent"
	const rbacResourcesNameClusterAgent = "foo-cluster-agent"
	const rbacResourcesNameClusterChecksRunner = "foo-cluster-checks-runner"

	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconcileDatadogAgent_Reconcile"})
	forwarders := dummyManager{}

	logf.SetLogger(logf.ZapLogger(true))

	// Register operator types with the runtime scheme.
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogAgent{})
	s.AddKnownTypes(edsdatadoghqv1alpha1.GroupVersion, &edsdatadoghqv1alpha1.ExtendedDaemonSet{})
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
	s.AddKnownTypes(apiregistrationv1.SchemeGroupVersion, &apiregistrationv1.APIService{})
	s.AddKnownTypes(networkingv1.SchemeGroupVersion, &networkingv1.NetworkPolicy{})

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
			name: "DatadogAgent not found",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
			},
			want:    reconcile.Result{},
			wantErr: false,
		},
		{
			name: "DatadogAgent found, add finalizer",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					_ = c.Create(context.TODO(), &datadoghqv1alpha1.DatadogAgent{
						TypeMeta: metav1.TypeMeta{
							Kind:       "DatadogAgent",
							APIVersion: fmt.Sprintf("%s/%s", datadoghqv1alpha1.GroupVersion.Group, datadoghqv1alpha1.GroupVersion.Version),
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace:   resourcesNamespace,
							Name:        resourcesName,
							Labels:      map[string]string{"label-foo-key": "label-bar-value"},
							Annotations: map[string]string{"annotations-foo-key": "annotations-bar-value"},
						},
						Spec: datadoghqv1alpha1.DatadogAgentSpec{
							Credentials:  datadoghqv1alpha1.AgentCredentials{Token: "token-foo"},
							Agent:        &datadoghqv1alpha1.DatadogAgentSpecAgentSpec{},
							ClusterAgent: &datadoghqv1alpha1.DatadogAgentSpecClusterAgentSpec{},
						},
					})
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				dda := &datadoghqv1alpha1.DatadogAgent{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, dda); err != nil {
					return err
				}
				assert.Contains(t, dda.GetFinalizers(), "finalizer.agent.datadoghq.com")
				return nil
			},
		},
		{
			name: "DatadogAgent found, but not defaulted",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					_ = c.Create(context.TODO(), &datadoghqv1alpha1.DatadogAgent{
						TypeMeta: metav1.TypeMeta{
							Kind:       "DatadogAgent",
							APIVersion: fmt.Sprintf("%s/%s", datadoghqv1alpha1.GroupVersion.Group, datadoghqv1alpha1.GroupVersion.Version),
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace:   resourcesNamespace,
							Name:        resourcesName,
							Labels:      map[string]string{"label-foo-key": "label-bar-value"},
							Annotations: map[string]string{"annotations-foo-key": "annotations-bar-value"},
						},
						Spec: datadoghqv1alpha1.DatadogAgentSpec{
							Credentials:  datadoghqv1alpha1.AgentCredentials{Token: "token-foo"},
							Agent:        &datadoghqv1alpha1.DatadogAgentSpecAgentSpec{},
							ClusterAgent: &datadoghqv1alpha1.DatadogAgentSpecClusterAgentSpec{},
						},
					})
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
		},
		{
			name: "DatadogAgent found and defaulted, create the Agent's ClusterRole",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, &test.NewDatadogAgentOptions{UseEDS: true, Labels: map[string]string{"label-foo-key": "label-bar-value"}})
					_ = c.Create(context.TODO(), dda)
					labels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultAgentResourceSuffix, getAgentVersion(dda))
					_ = c.Create(context.TODO(), test.NewSecret(resourcesNamespace, "foo", &test.NewSecretOptions{Labels: labels, Data: map[string][]byte{
						"api-key": []byte(base64.StdEncoding.EncodeToString([]byte("api-foo"))),
						"app-key": []byte(base64.StdEncoding.EncodeToString([]byte("app-foo"))),
						"token":   []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))

					installinfoCM, _ := buildInstallInfoConfigMap(dda)
					_ = c.Create(context.TODO(), installinfoCM)
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
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
			name: "DatadogAgent found and defaulted, create the Agent's ClusterRoleBinding",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, &test.NewDatadogAgentOptions{UseEDS: true, Labels: map[string]string{"label-foo-key": "label-bar-value"}})
					_ = c.Create(context.TODO(), dda)
					_ = c.Create(context.TODO(), buildAgentClusterRole(dda, getAgentRbacResourcesName(dda), getAgentVersion(dda)))
					_ = c.Create(context.TODO(), buildServiceAccount(dda, getAgentRbacResourcesName(dda), getAgentVersion(dda)))
					labels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultAgentResourceSuffix, getAgentVersion(dda))
					_ = c.Create(context.TODO(), test.NewSecret(resourcesNamespace, "foo", &test.NewSecretOptions{Labels: labels, Data: map[string][]byte{
						"api-key": []byte(base64.StdEncoding.EncodeToString([]byte("api-foo"))),
						"app-key": []byte(base64.StdEncoding.EncodeToString([]byte("app-foo"))),
						"token":   []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))

					installinfoCM, _ := buildInstallInfoConfigMap(dda)
					_ = c.Create(context.TODO(), installinfoCM)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
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
			name: "DatadogAgent found and defaulted, create the Agent's ServiceAccount",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, &test.NewDatadogAgentOptions{UseEDS: true, Labels: map[string]string{"label-foo-key": "label-bar-value"}})
					_ = c.Create(context.TODO(), dda)
					resourceName := getAgentRbacResourcesName(dda)
					version := getAgentVersion(dda)
					_ = c.Create(context.TODO(), buildAgentClusterRole(dda, resourceName, version))
					_ = c.Create(context.TODO(), buildClusterRoleBinding(dda, roleBindingInfo{
						name:               resourceName,
						roleName:           resourceName,
						serviceAccountName: resourceName,
					}, version))
					labels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultAgentResourceSuffix, getAgentVersion(dda))
					_ = c.Create(context.TODO(), test.NewSecret(resourcesNamespace, "foo", &test.NewSecretOptions{Labels: labels, Data: map[string][]byte{
						"api-key": []byte(base64.StdEncoding.EncodeToString([]byte("api-foo"))),
						"app-key": []byte(base64.StdEncoding.EncodeToString([]byte("app-foo"))),
						"token":   []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))

					installinfoCM, _ := buildInstallInfoConfigMap(dda)
					_ = c.Create(context.TODO(), installinfoCM)
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				rbacResourcesName := "foo-agent"
				serviceAccount := &corev1.ServiceAccount{}
				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: rbacResourcesName}, serviceAccount); err != nil {
					return err
				}
				if !ownedByDatadogOperator(serviceAccount.OwnerReferences) {
					return fmt.Errorf("bad serviceAccount, should be owned by the datadog operator, current owners: %v", serviceAccount.OwnerReferences)
				}
				return nil
			},
		},
		{
			name: "DatadogAgent found and defaulted, create the ExtendedDaemonSet",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, &test.NewDatadogAgentOptions{UseEDS: true, Labels: map[string]string{"label-foo-key": "label-bar-value"}})
					_ = c.Create(context.TODO(), dda)

					createAgentDependencies(c, dda)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				eds := &edsdatadoghqv1alpha1.ExtendedDaemonSet{}
				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: dsName}, eds); err != nil {
					return err
				}
				if eds.Name != dsName {
					return fmt.Errorf("eds bad name, should be: 'foo', current: %s", eds.Name)
				}
				if eds.OwnerReferences == nil || len(eds.OwnerReferences) != 1 {
					return fmt.Errorf("eds bad owner references, should be: '[Kind DatadogAgent - Name foo]', current: %v", eds.OwnerReferences)
				}
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
				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: rbacResourcesName}, &corev1.ServiceAccount{}); err != nil {
					return err
				}

				return nil
			},
		},
		{
			name: "DatadogAgent found and defaulted, block daemonsetName change",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, &test.NewDatadogAgentOptions{
						UseEDS: true,
						Labels: map[string]string{"label-foo-key": "label-bar-value"},
						Status: &datadoghqv1alpha1.DatadogAgentStatus{
							Agent: &datadoghqv1alpha1.DaemonSetStatus{
								DaemonsetName: "datadog-agent-daemonset-before",
							},
						},
						AgentDaemonsetName: "datadog-agent-daemonset",
					})
					_ = c.Create(context.TODO(), dda)

					createAgentDependencies(c, dda)
				},
			},
			want:    reconcile.Result{},
			wantErr: true,
			wantFunc: func(c client.Client) error {
				eds := &edsdatadoghqv1alpha1.ExtendedDaemonSet{}
				err := c.Get(context.TODO(), newRequest(resourcesNamespace, dsName).NamespacedName, eds)
				if apierrors.IsNotFound(err) {
					// Daemonset must NOT be created
					return nil
				}
				return err
			},
		},
		{
			name: "DatadogAgent found and defaulted, create the ExtendedDaemonSet with non default config",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, &test.NewDatadogAgentOptions{
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
					_ = c.Create(context.TODO(), dda)

					createAgentDependencies(c, dda)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				eds := &edsdatadoghqv1alpha1.ExtendedDaemonSet{}
				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: dsName}, eds); err != nil {
					return err
				}
				if eds.Name != dsName {
					return fmt.Errorf("eds bad name, should be: 'foo', current: %s", eds.Name)
				}
				if eds.OwnerReferences == nil || len(eds.OwnerReferences) != 1 {
					return fmt.Errorf("eds bad owner references, should be: '[Kind DatadogAgent - Name foo]', current: %v", eds.OwnerReferences)
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
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					_ = c.Create(context.TODO(), test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, &test.NewDatadogAgentOptions{ClusterAgentEnabled: true, Labels: map[string]string{"label-foo-key": "label-bar-value"}}))
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				secret := &corev1.Secret{}
				if err := c.Get(context.TODO(), newRequest(resourcesNamespace, "foo").NamespacedName, secret); err != nil {
					return err
				}
				if secret.OwnerReferences == nil || len(secret.OwnerReferences) != 1 {
					return fmt.Errorf("ds bad owner references, should be: '[Kind DatadogAgent - Name foo]', current: %v", secret.OwnerReferences)
				}

				return nil
			},
		},
		{
			name: "DatadogAgent found and defaulted, create the DaemonSet",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := test.NewDefaultedDatadogAgent(
						resourcesNamespace,
						resourcesName,
						&test.NewDatadogAgentOptions{
							ClusterAgentEnabled: false,
							UseEDS:              false,
							Labels:              map[string]string{"label-foo-key": "label-bar-value"},
							NodeAgentConfig: datadoghqv1alpha1.DefaultDatadogAgentSpecAgentConfig(&datadoghqv1alpha1.NodeAgentConfig{
								SecurityContext: &corev1.PodSecurityContext{
									RunAsUser: datadoghqv1alpha1.NewInt64Pointer(100),
								}}),
						})
					_ = c.Create(context.TODO(), dda)
					createAgentDependencies(c, dda)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				ds := &appsv1.DaemonSet{}
				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: dsName}, ds); err != nil {
					return err
				}
				if ds.Spec.Template.Spec.SecurityContext == nil || ds.Spec.Template.Spec.SecurityContext.RunAsUser == nil || *ds.Spec.Template.Spec.SecurityContext.RunAsUser != 100 {
					return fmt.Errorf("securityContext not applied")
				}
				if ds.Name != dsName {
					return fmt.Errorf("ds bad name, should be: 'foo', current: %s", ds.Name)
				}
				if ds.OwnerReferences == nil || len(ds.OwnerReferences) != 1 {
					return fmt.Errorf("ds bad owner references, should be: '[Kind DatadogAgent - Name foo]', current: %v", ds.OwnerReferences)
				}

				return nil
			},
		},
		{
			name: "DatadogAgent with APM agent found and defaulted, create Daemonset",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, &test.NewDatadogAgentOptions{APMEnabled: true, ClusterAgentEnabled: false, UseEDS: false, Labels: map[string]string{"label-foo-key": "label-bar-value"}})
					_ = c.Create(context.TODO(), dda)
					createAgentDependencies(c, dda)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				ds := &appsv1.DaemonSet{}
				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: dsName}, ds); err != nil {
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
			name: "DatadogAgent with Process agent found and defaulted, create Daemonset",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, &test.NewDatadogAgentOptions{ProcessEnabled: true, ClusterAgentEnabled: false, UseEDS: false, Labels: map[string]string{"label-foo-key": "label-bar-value"}})
					_ = c.Create(context.TODO(), dda)
					createAgentDependencies(c, dda)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				ds := &appsv1.DaemonSet{}
				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: dsName}, ds); err != nil {
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
			name: "DatadogAgent with Process agent found and defaulted, create system-probe-config configmap",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, &test.NewDatadogAgentOptions{ProcessEnabled: true, SystemProbeEnabled: true, ClusterAgentEnabled: false, UseEDS: false, Labels: map[string]string{"label-foo-key": "label-bar-value"}})
					_ = c.Create(context.TODO(), dda)
					createAgentDependencies(c, dda)
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				configmap := &corev1.ConfigMap{}
				if err := c.Get(context.TODO(), newRequest(resourcesNamespace, getSystemProbeConfigConfigMapName(resourcesName)).NamespacedName, configmap); err != nil {
					return err
				}

				return nil
			},
		},
		{
			name: "DatadogAgent with Process agent found and defaulted, create datadog-agent-security configmap",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, &test.NewDatadogAgentOptions{ProcessEnabled: true, SystemProbeEnabled: true, ClusterAgentEnabled: false, UseEDS: false, Labels: map[string]string{"label-foo-key": "label-bar-value"}})
					_ = c.Create(context.TODO(), dda)
					createAgentDependencies(c, dda)
					configCM, _ := buildSystemProbeConfigConfiMap(dda)
					_ = c.Create(context.TODO(), configCM)
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				configmap := &corev1.ConfigMap{}
				if err := c.Get(context.TODO(), newRequest(resourcesNamespace, getSecCompConfigMapName(resourcesName)).NamespacedName, configmap); err != nil {
					return err
				}

				return nil
			},
		},
		{
			name: "DatadogAgent with Process agent and system-probe found and defaulted, create Daemonset",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					options := &test.NewDatadogAgentOptions{
						ProcessEnabled:                 true,
						SystemProbeEnabled:             true,
						SystemProbeAppArmorProfileName: "AppArmorFoo",
						SystemProbeSeccompProfileName:  "runtime/default",
						ClusterAgentEnabled:            false,
						UseEDS:                         false,
						Labels:                         map[string]string{"label-foo-key": "label-bar-value"},
					}
					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, options)
					_ = c.Create(context.TODO(), dda)
					createAgentDependencies(c, dda)
					createSystemProbeDependencies(c, dda)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				ds := &appsv1.DaemonSet{}
				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: dsName}, ds); err != nil {
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

				if val, ok := ds.Spec.Template.Annotations[datadoghqv1alpha1.SysteProbeAppArmorAnnotationKey]; !ok && val != "AppArmorFoo" {
					return fmt.Errorf("AppArmor annotation is wrong, got: %s, want: AppArmorFoo", val)
				}

				if val, ok := ds.Spec.Template.Annotations[datadoghqv1alpha1.SysteProbeSeccompAnnotationKey]; !ok && val != "runtime/default" {
					return fmt.Errorf("Seccomp annotation is wrong, got: %s, want: runtime/default", val)
				}

				return nil
			},
		},
		{
			name: "DatadogAgent found and defaulted, ExtendedDaemonSet already exists",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					adOptions := &test.NewDatadogAgentOptions{
						UseEDS: true,
						Labels: map[string]string{"label-foo-key": "label-bar-value"},
						Status: &datadoghqv1alpha1.DatadogAgentStatus{},
					}
					ad := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, adOptions)
					adHash, _ := comparison.GenerateMD5ForSpec(ad.Spec)
					createAgentDependencies(c, ad)
					edsOptions := &test.NewExtendedDaemonSetOptions{
						Labels:      map[string]string{"label-foo-key": "label-bar-value"},
						Annotations: map[string]string{string(datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey): adHash},
					}
					eds := test.NewExtendedDaemonSet(resourcesNamespace, resourcesName, edsOptions)

					_ = c.Create(context.TODO(), ad)
					_ = c.Create(context.TODO(), eds)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				eds := &edsdatadoghqv1alpha1.ExtendedDaemonSet{}
				if err := c.Get(context.TODO(), newRequest(resourcesNamespace, resourcesName).NamespacedName, eds); err != nil {
					return err
				}
				if eds.Name != resourcesName {
					return fmt.Errorf("eds bad name, should be: 'foo', current: %s", eds.Name)
				}

				return nil
			},
		},
		{
			name: "DatadogAgent found and defaulted, ExtendedDaemonSet already exists but not up-to-date",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					adOptions := &test.NewDatadogAgentOptions{
						UseEDS: true,
						Labels: map[string]string{"label-foo-key": "label-bar-value"},
						Status: &datadoghqv1alpha1.DatadogAgentStatus{},
					}
					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, adOptions)

					createAgentDependencies(c, dda)

					edsOptions := &test.NewExtendedDaemonSetOptions{
						Labels:      map[string]string{"label-foo-key": "label-bar-value"},
						Annotations: map[string]string{datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey: "outdated-hash"},
					}
					eds := test.NewExtendedDaemonSet(resourcesNamespace, resourcesName, edsOptions)

					_ = c.Create(context.TODO(), dda)
					_ = c.Create(context.TODO(), eds)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeuePeriod},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				eds := &edsdatadoghqv1alpha1.ExtendedDaemonSet{}
				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: dsName}, eds); err != nil {
					return err
				}
				if eds.Name != dsName {
					return fmt.Errorf("eds bad name, should be: 'foo', current: %s", eds.Name)
				}
				if eds.OwnerReferences == nil || len(eds.OwnerReferences) != 1 {
					return fmt.Errorf("eds bad owner references, should be: '[Kind DatadogAgent - Name foo]', current: %v", eds.OwnerReferences)
				}
				if hash := eds.Annotations[datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey]; hash == "outdated-hash" {
					return errors.New("eds hash not updated")
				}

				return nil
			},
		},
		{
			name: "DatadogAgent found and defaulted, Cluster Agent enabled, create the Cluster Agent Service",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, &test.NewDatadogAgentOptions{Labels: map[string]string{"label-foo-key": "label-bar-value"}, ClusterAgentEnabled: true})
					_ = c.Create(context.TODO(), dda)
					commonDCAlabels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
					_ = c.Create(context.TODO(), test.NewSecret(resourcesNamespace, "foo", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				dcaService := &corev1.Service{}
				if err := c.Get(context.TODO(), newRequest(resourcesNamespace, "foo-cluster-agent").NamespacedName, dcaService); err != nil {
					return err
				}

				return nil
			},
		},
		{
			name: "DatadogAgent found and defaulted, Cluster Agent enabled, create the Metrics Server Service",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, &test.NewDatadogAgentOptions{Labels: map[string]string{"label-foo-key": "label-bar-value"}, ClusterAgentEnabled: true, MetricsServerEnabled: true})
					_ = c.Create(context.TODO(), dda)
					commonDCAlabels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
					_ = c.Create(context.TODO(), test.NewSecret(resourcesNamespace, "foo", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))
					dcaService := test.NewService(resourcesNamespace, "foo-cluster-agent", &test.NewServiceOptions{Spec: &corev1.ServiceSpec{
						Type: corev1.ServiceTypeClusterIP,
						Selector: map[string]string{
							datadoghqv1alpha1.AgentDeploymentNameLabelKey:      resourcesName,
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
				if err := c.Get(context.TODO(), newRequest(resourcesNamespace, "foo-cluster-agent-metrics-server").NamespacedName, dcaService); err != nil {
					return err
				}

				return nil
			},
		},
		{
			name: "DatadogAgent found and defaulted, Cluster Agent enabled, create the Admission Controller Service",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, &test.NewDatadogAgentOptions{Labels: map[string]string{"label-foo-key": "label-bar-value"}, ClusterAgentEnabled: true, AdmissionControllerEnabled: true})
					_ = c.Create(context.TODO(), dda)
					commonDCAlabels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
					_ = c.Create(context.TODO(), test.NewSecret(resourcesNamespace, "foo", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))
					dcaService := test.NewService(resourcesNamespace, "foo-cluster-agent", &test.NewServiceOptions{Spec: &corev1.ServiceSpec{
						Type: corev1.ServiceTypeClusterIP,
						Selector: map[string]string{
							datadoghqv1alpha1.AgentDeploymentNameLabelKey:      resourcesName,
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
				if err := c.Get(context.TODO(), newRequest(resourcesNamespace, "datadog-admission-controller").NamespacedName, dcaService); err != nil {
					return err
				}

				return nil
			},
		},
		{
			name: "DatadogAgent found and defaulted, Cluster Agent enabled, create the Cluster Agent Deployment",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, &test.NewDatadogAgentOptions{Labels: map[string]string{"label-foo-key": "label-bar-value"}, ClusterAgentEnabled: true})
					_ = c.Create(context.TODO(), dda)
					commonDCAlabels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
					_ = c.Create(context.TODO(), test.NewSecret(resourcesNamespace, "foo", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))

					createClusterAgentDependencies(c, dda)
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				dca := &appsv1.Deployment{}
				if err := c.Get(context.TODO(), newRequest(resourcesNamespace, "foo-cluster-agent").NamespacedName, dca); err != nil {
					return err
				}
				if dca.OwnerReferences == nil || len(dca.OwnerReferences) != 1 {
					return fmt.Errorf("dca bad owner references, should be: '[Kind DatadogAgent - Name foo]', current: %v", dca.OwnerReferences)
				}
				return nil
			},
		},
		{
			name: "DatadogAgent found and defaulted, Cluster Agent enabled, create the Cluster Agent PDB",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, &test.NewDatadogAgentOptions{Labels: map[string]string{"label-foo-key": "label-bar-value"}, ClusterAgentEnabled: true})
					_ = c.Create(context.TODO(), dda)
					commonDCAlabels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
					_ = c.Create(context.TODO(), test.NewSecret(resourcesNamespace, "foo", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))
					dcaService := test.NewService(resourcesNamespace, "foo-cluster-agent", &test.NewServiceOptions{Spec: &corev1.ServiceSpec{
						Type: corev1.ServiceTypeClusterIP,
						Selector: map[string]string{
							datadoghqv1alpha1.AgentDeploymentNameLabelKey:      resourcesName,
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
				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: "foo-cluster-agent"}, pdb); err != nil {
					return err
				}
				if !ownedByDatadogOperator(pdb.OwnerReferences) {
					return fmt.Errorf("bad PDB, should be owned by the datadog operator, current owners: %v", pdb.OwnerReferences)
				}

				return nil
			},
		},
		{
			name: "DatadogAgent found and defaulted, Cluster Agent enabled, create the Cluster Agent ClusterRole",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, &test.NewDatadogAgentOptions{Labels: map[string]string{"label-foo-key": "label-bar-value"}, ClusterAgentEnabled: true})
					_ = c.Create(context.TODO(), dda)
					commonDCAlabels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
					_ = c.Create(context.TODO(), test.NewSecret(resourcesNamespace, "foo", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))
					_ = c.Create(context.TODO(), buildServiceAccount(dda, "foo-cluster-agent", getClusterAgentVersion(dda)))
					dcaService := test.NewService(resourcesNamespace, "foo-cluster-agent", &test.NewServiceOptions{Spec: &corev1.ServiceSpec{
						Type: corev1.ServiceTypeClusterIP,
						Selector: map[string]string{
							datadoghqv1alpha1.AgentDeploymentNameLabelKey:      resourcesName,
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
					_ = c.Create(context.TODO(), buildClusterAgentPDB(dda))
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
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
			name: "DatadogAgent found and defaulted, Cluster Agent enabled, WPA Controller enabled, create the Cluster Agent ClusterRole",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, &test.NewDatadogAgentOptions{Labels: map[string]string{"label-foo-key": "label-bar-value"}, ClusterAgentEnabled: true, MetricsServerEnabled: true, MetricsServerWPAController: true})
					_ = c.Create(context.TODO(), dda)
					commonDCAlabels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
					_ = c.Create(context.TODO(), test.NewSecret(resourcesNamespace, "foo", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))

					createClusterAgentDependencies(c, dda)

					dcaExternalMetricsService := test.NewService(resourcesNamespace, "foo-cluster-agent-metrics-server", &test.NewServiceOptions{Spec: &corev1.ServiceSpec{
						Type: corev1.ServiceTypeClusterIP,
						Selector: map[string]string{
							datadoghqv1alpha1.AgentDeploymentNameLabelKey:      resourcesName,
							datadoghqv1alpha1.AgentDeploymentComponentLabelKey: "cluster-agent",
						},
						Ports: []corev1.ServicePort{
							{
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(datadoghqv1alpha1.DefaultMetricsServerTargetPort),
								Port:       datadoghqv1alpha1.DefaultMetricsServerServicePort,
							},
						},
						SessionAffinity: corev1.ServiceAffinityNone,
					},
					})
					_, _ = comparison.SetMD5GenerationAnnotation(&dcaExternalMetricsService.ObjectMeta, dcaExternalMetricsService.Spec)
					dcaExternalMetricsService.Labels = commonDCAlabels
					_ = c.Create(context.TODO(), dcaExternalMetricsService)
					_ = c.Create(context.TODO(), buildClusterAgentPDB(dda))
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				metricsService := &corev1.Service{}
				if err := c.Get(context.TODO(), newRequest(resourcesNamespace, "foo-cluster-agent-metrics-server").NamespacedName, metricsService); err != nil {
					return err
				}
				clusterRole := &rbacv1.ClusterRole{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesNameClusterAgent}, clusterRole); err != nil {
					return err
				}
				if !hasAllClusterLevelRbacResources(clusterRole.Rules) {
					return fmt.Errorf("bad cluster role, should contain all cluster level rbac resources, current: %v", clusterRole.Rules)
				}
				if !hasWpaRbacs(clusterRole.Rules) {
					return fmt.Errorf("bad cluster role, should contain wpa cluster level rbac resources, current: %v", clusterRole.Rules)
				}
				return nil
			},
		},
		{
			name: "DatadogAgent found and defaulted, Cluster Agent enabled, Admission Controller enabled, create the Cluster Agent ClusterRole",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, &test.NewDatadogAgentOptions{Labels: map[string]string{"label-foo-key": "label-bar-value"}, ClusterAgentEnabled: true, AdmissionControllerEnabled: true})
					_ = c.Create(context.TODO(), dda)
					commonDCAlabels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
					_ = c.Create(context.TODO(), test.NewSecret(resourcesNamespace, "foo", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))
					_ = c.Create(context.TODO(), buildServiceAccount(dda, "foo-cluster-agent", getClusterAgentVersion(dda)))
					dcaService := test.NewService(resourcesNamespace, "foo-cluster-agent", &test.NewServiceOptions{Spec: &corev1.ServiceSpec{
						Type: corev1.ServiceTypeClusterIP,
						Selector: map[string]string{
							datadoghqv1alpha1.AgentDeploymentNameLabelKey:      resourcesName,
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
					admissionService := test.NewService(resourcesNamespace, "datadog-admission-controller", &test.NewServiceOptions{Spec: &corev1.ServiceSpec{
						Type: corev1.ServiceTypeClusterIP,
						Selector: map[string]string{
							datadoghqv1alpha1.AgentDeploymentNameLabelKey:      resourcesName,
							datadoghqv1alpha1.AgentDeploymentComponentLabelKey: "cluster-agent",
						},
						Ports: []corev1.ServicePort{
							{
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(8000),
								Port:       443,
							},
						},
						SessionAffinity: corev1.ServiceAffinityNone,
					},
					})
					_, _ = comparison.SetMD5GenerationAnnotation(&dcaService.ObjectMeta, dcaService.Spec)
					dcaService.Labels = commonDCAlabels
					_ = c.Create(context.TODO(), dcaService)

					_, _ = comparison.SetMD5GenerationAnnotation(&admissionService.ObjectMeta, admissionService.Spec)
					admissionService.Labels = commonDCAlabels
					_ = c.Create(context.TODO(), admissionService)

					_ = c.Create(context.TODO(), buildClusterAgentPDB(dda))
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				clusterRole := &rbacv1.ClusterRole{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesNameClusterAgent}, clusterRole); err != nil {
					return err
				}
				if !hasAllClusterLevelRbacResources(clusterRole.Rules) {
					return fmt.Errorf("bad cluster role, should contain all cluster level rbac resources, current: %v", clusterRole.Rules)
				}
				if !hasAdmissionRbacResources(clusterRole.Rules) {
					return fmt.Errorf("bad cluster role, should contain cluster level rbac resources needed by the admission controller, current: %v", clusterRole.Rules)
				}
				if !ownedByDatadogOperator(clusterRole.OwnerReferences) {
					return fmt.Errorf("bad clusterRole, should be owned by the datadog operator, current owners: %v", clusterRole.OwnerReferences)
				}

				return nil
			},
		},
		{
			name: "DatadogAgent found and defaulted, Cluster Agent enabled, create the Cluster Agent ClusterRoleBinding",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, &test.NewDatadogAgentOptions{Labels: map[string]string{"label-foo-key": "label-bar-value"}, ClusterAgentEnabled: true})
					_ = c.Create(context.TODO(), dda)
					commonDCAlabels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
					_ = c.Create(context.TODO(), test.NewSecret(resourcesNamespace, "foo", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))
					dcaService := test.NewService(resourcesNamespace, "foo-cluster-agent", &test.NewServiceOptions{Spec: &corev1.ServiceSpec{
						Type: corev1.ServiceTypeClusterIP,
						Selector: map[string]string{
							datadoghqv1alpha1.AgentDeploymentNameLabelKey:      resourcesName,
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
					_ = c.Create(context.TODO(), buildServiceAccount(dda, "foo-cluster-agent", getClusterAgentVersion(dda)))
					_ = c.Create(context.TODO(), buildClusterAgentClusterRole(dda, "foo-cluster-agent", getClusterAgentVersion(dda)))
					_ = c.Create(context.TODO(), buildClusterAgentPDB(dda))
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
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
			name: "DatadogAgent found and defaulted, Cluster Agent enabled, create the Cluster Agent HPA ClusterRoleBinding",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, &test.NewDatadogAgentOptions{Labels: map[string]string{"label-foo-key": "label-bar-value"}, ClusterAgentEnabled: true, MetricsServerEnabled: true})
					_ = c.Create(context.TODO(), dda)
					commonDCAlabels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
					_ = c.Create(context.TODO(), test.NewSecret(resourcesNamespace, "foo", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))

					createClusterAgentDependencies(c, dda)

					dcaExternalMetricsService := test.NewService(resourcesNamespace, "foo-cluster-agent-metrics-server", &test.NewServiceOptions{Spec: &corev1.ServiceSpec{
						Type: corev1.ServiceTypeClusterIP,
						Selector: map[string]string{
							datadoghqv1alpha1.AgentDeploymentNameLabelKey:      resourcesName,
							datadoghqv1alpha1.AgentDeploymentComponentLabelKey: "cluster-agent",
						},
						Ports: []corev1.ServicePort{
							{
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(datadoghqv1alpha1.DefaultMetricsServerTargetPort),
								Port:       datadoghqv1alpha1.DefaultMetricsServerServicePort,
							},
						},
						SessionAffinity: corev1.ServiceAffinityNone,
					},
					})
					_, _ = comparison.SetMD5GenerationAnnotation(&dcaExternalMetricsService.ObjectMeta, dcaExternalMetricsService.Spec)
					dcaExternalMetricsService.Labels = commonDCAlabels
					_ = c.Create(context.TODO(), dcaExternalMetricsService)

					port := int32(datadoghqv1alpha1.DefaultMetricsServerServicePort)
					dcaExternalMetricsAPIService := test.NewAPIService("", "v1beta1.external.metrics.k8s.io", &test.NewAPIServiceOptions{
						Spec: &apiregistrationv1.APIServiceSpec{
							Service: &apiregistrationv1.ServiceReference{
								Namespace: resourcesNamespace,
								Name:      "foo-cluster-agent-metrics-server",
								Port:      &port,
							},
							Version:               "v1beta1",
							InsecureSkipTLSVerify: true,
							Group:                 "external.metrics.k8s.io",
							GroupPriorityMinimum:  100,
							VersionPriority:       100,
						},
					})
					_, _ = comparison.SetMD5GenerationAnnotation(&dcaExternalMetricsAPIService.ObjectMeta, dcaExternalMetricsAPIService.Spec)
					dcaExternalMetricsAPIService.Labels = commonDCAlabels
					_ = c.Create(context.TODO(), dcaExternalMetricsAPIService)

					_ = c.Create(context.TODO(), buildClusterAgentPDB(dda))
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
			name: "DatadogAgent found and defaulted, Cluster Agent enabled, create the Cluster Agent ServiceAccount",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, &test.NewDatadogAgentOptions{Labels: map[string]string{"label-foo-key": "label-bar-value"}, ClusterAgentEnabled: true, MetricsServerEnabled: false})
					_ = c.Create(context.TODO(), dda)
					commonDCAlabels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
					_ = c.Create(context.TODO(), test.NewSecret(resourcesNamespace, "foo", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))
					dcaService := test.NewService(resourcesNamespace, "foo-cluster-agent", &test.NewServiceOptions{Spec: &corev1.ServiceSpec{
						Type: corev1.ServiceTypeClusterIP,
						Selector: map[string]string{
							datadoghqv1alpha1.AgentDeploymentNameLabelKey:      resourcesName,
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
					dcaExternalMetricsService := test.NewService(resourcesNamespace, "foo-cluster-agent-metrics-server", &test.NewServiceOptions{Spec: &corev1.ServiceSpec{
						Type: corev1.ServiceTypeClusterIP,
						Selector: map[string]string{
							datadoghqv1alpha1.AgentDeploymentNameLabelKey:      resourcesName,
							datadoghqv1alpha1.AgentDeploymentComponentLabelKey: "cluster-agent",
						},
						Ports: []corev1.ServicePort{
							{
								Protocol:   corev1.ProtocolTCP,
								TargetPort: intstr.FromInt(datadoghqv1alpha1.DefaultMetricsServerTargetPort),
								Port:       datadoghqv1alpha1.DefaultMetricsServerServicePort,
							},
						},
						SessionAffinity: corev1.ServiceAffinityNone,
					},
					})
					_, _ = comparison.SetMD5GenerationAnnotation(&dcaExternalMetricsService.ObjectMeta, dcaExternalMetricsService.Spec)
					dcaExternalMetricsService.Labels = commonDCAlabels
					_ = c.Create(context.TODO(), dcaExternalMetricsService)
					version := getClusterAgentVersion(dda)
					_ = c.Create(context.TODO(), buildClusterAgentClusterRole(dda, "foo-cluster-agent", version))
					_ = c.Create(context.TODO(), buildClusterRoleBinding(dda, roleBindingInfo{
						name:               "foo-cluster-agent",
						roleName:           "foo-cluster-agent",
						serviceAccountName: "foo-cluster-agent",
					}, version))
					_ = c.Create(context.TODO(), buildMetricsServerClusterRoleBinding(dda, "foo-cluster-agent-system-auth-delegator", version))
					_ = c.Create(context.TODO(), buildClusterAgentPDB(dda))
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				// Make sure Cluster Agent ServiceAccount is created properly
				rbacResourcesNameClusterAgent := "foo-cluster-agent"
				serviceAccount := &corev1.ServiceAccount{}
				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: rbacResourcesNameClusterAgent}, serviceAccount); err != nil {
					return err
				}
				if !ownedByDatadogOperator(serviceAccount.OwnerReferences) {
					return fmt.Errorf("bad serviceAccount, should be owned by the datadog operator, current owners: %v", serviceAccount.OwnerReferences)
				}

				return nil
			},
		},
		{
			name: "DatadogAgent found and defaulted, Cluster Agent Deployment already exists, create Daemonset",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dadOptions := &test.NewDatadogAgentOptions{
						Labels:              map[string]string{"label-foo-key": "label-bar-value"},
						Status:              &datadoghqv1alpha1.DatadogAgentStatus{},
						ClusterAgentEnabled: true,
					}

					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, dadOptions)
					_ = c.Create(context.TODO(), dda)
					commonDCAlabels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
					_ = c.Create(context.TODO(), test.NewSecret(resourcesNamespace, "foo", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))

					createClusterAgentDependencies(c, dda)

					dcaOptions := &test.NewDeploymentOptions{
						Labels:                 map[string]string{"label-foo-key": "label-bar-value"},
						ForceAvailableReplicas: datadoghqv1alpha1.NewInt32Pointer(1),
					}
					dca := test.NewClusterAgentDeployment(resourcesNamespace, resourcesName, dcaOptions)

					_ = c.Create(context.TODO(), dda)
					_ = c.Create(context.TODO(), dca)

					createAgentDependencies(c, dda)
					resourceName := getAgentRbacResourcesName(dda)
					version := getAgentVersion(dda)
					_ = c.Create(context.TODO(), buildClusterRoleBinding(dda, roleBindingInfo{
						name:               getClusterChecksRunnerRbacResourcesName(dda),
						roleName:           resourceName,
						serviceAccountName: getClusterChecksRunnerServiceAccount(dda),
					}, version))
					_ = c.Create(context.TODO(), buildServiceAccount(dda, getClusterChecksRunnerServiceAccount(dda), version))

				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				ds := &appsv1.DaemonSet{}
				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: dsName}, ds); err != nil {
					return err
				}

				return nil
			},
		},
		{
			name: "DatadogAgent found and defaulted, Cluster Agent Deployment already exists, block DeploymentName change",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dadOptions := &test.NewDatadogAgentOptions{
						Labels: map[string]string{"label-foo-key": "label-bar-value"},
						Status: &datadoghqv1alpha1.DatadogAgentStatus{
							ClusterAgent: &datadoghqv1alpha1.DeploymentStatus{
								DeploymentName: "cluster-agent-deployment-before",
							},
						},
						ClusterAgentEnabled:        true,
						ClusterAgentDeploymentName: "cluster-agent-depoyment",
					}

					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, dadOptions)
					_ = c.Create(context.TODO(), dda)
					commonDCAlabels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
					_ = c.Create(context.TODO(), test.NewSecret(resourcesNamespace, "foo", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))

					createClusterAgentDependencies(c, dda)

					dcaOptions := &test.NewDeploymentOptions{
						Labels:                 map[string]string{"label-foo-key": "label-bar-value"},
						ForceAvailableReplicas: datadoghqv1alpha1.NewInt32Pointer(1),
					}
					dca := test.NewClusterAgentDeployment(resourcesNamespace, resourcesName, dcaOptions)

					_ = c.Create(context.TODO(), dda)
					_ = c.Create(context.TODO(), dca)

					createAgentDependencies(c, dda)
					resourceName := getAgentRbacResourcesName(dda)
					version := getAgentVersion(dda)
					_ = c.Create(context.TODO(), buildClusterRoleBinding(dda, roleBindingInfo{
						name:               getClusterChecksRunnerRbacResourcesName(dda),
						roleName:           resourceName,
						serviceAccountName: getClusterChecksRunnerServiceAccount(dda),
					}, version))
					_ = c.Create(context.TODO(), buildServiceAccount(dda, getClusterChecksRunnerServiceAccount(dda), version))

				},
			},
			want:    reconcile.Result{},
			wantErr: true,
			wantFunc: func(c client.Client) error {
				ds := &appsv1.DaemonSet{}
				err := c.Get(context.TODO(), newRequest(resourcesNamespace, resourcesName).NamespacedName, ds)
				if apierrors.IsNotFound(err) {
					// Daemonset must NOT be created
					return nil
				}
				return err
			},
		},
		/*
			{
				name: "DatadogAgent found and defaulted, Cluster Agent enabled, block DeploymentName change",
				fields: fields{
					client:   fake.NewFakeClient(),
					scheme:   s,
					recorder: recorder,
				},
				args: args{
					request: newRequest(resourcesNamespace, resourcesName),
					loadFunc: func(c client.Client) {
						dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, &test.NewDatadogAgentOptions{Labels: map[string]string{"label-foo-key": "label-bar-value"}, ClusterAgentEnabled: true})
						dda.Status.ClusterAgent = &datadoghqv1alpha1.DeploymentStatus{
							DeploymentName: "cluster-agent-prev-name",
						}
						_ = c.Create(context.TODO(), dda)
						// commonDCAlabels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
						// _ = c.Create(context.TODO(), test.NewSecret(resourcesNamespace, "foo", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						// 	"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
						// }}))
					},
				},
				want:    reconcile.Result{},
				wantErr: true,
				wantFunc: func(c client.Client) error {
					dcaService := &corev1.Service{}
					err := c.Get(context.TODO(), newRequest(resourcesNamespace, "foo-cluster-agent").NamespacedName, dcaService)
					if apierrors.IsNotFound(err) {
						// Daemonset must NOT be created
						return nil
					}
					return err
				},
			},

		*/
		{
			name: "DatadogAgent found and defaulted, Cluster Agent Deployment already exists but with 0 pods ready, do not create Daemonset",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dadOptions := &test.NewDatadogAgentOptions{
						Labels:              map[string]string{"label-foo-key": "label-bar-value"},
						Status:              &datadoghqv1alpha1.DatadogAgentStatus{},
						ClusterAgentEnabled: true,
					}

					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, dadOptions)
					_ = c.Create(context.TODO(), dda)
					commonDCAlabels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
					_ = c.Create(context.TODO(), test.NewSecret(resourcesNamespace, "foo", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))

					createClusterAgentDependencies(c, dda)

					dcaOptions := &test.NewDeploymentOptions{
						Labels:                 map[string]string{"label-foo-key": "label-bar-value"},
						ForceAvailableReplicas: datadoghqv1alpha1.NewInt32Pointer(0),
					}
					dca := test.NewClusterAgentDeployment(resourcesNamespace, resourcesName, dcaOptions)

					_ = c.Create(context.TODO(), dda)
					_ = c.Create(context.TODO(), dca)

					createAgentDependencies(c, dda)
					resourceName := getAgentRbacResourcesName(dda)
					version := getAgentVersion(dda)
					_ = c.Create(context.TODO(), buildClusterRoleBinding(dda, roleBindingInfo{
						name:               getClusterChecksRunnerRbacResourcesName(dda),
						roleName:           resourceName,
						serviceAccountName: getClusterChecksRunnerServiceAccount(dda),
					}, version))
					_ = c.Create(context.TODO(), buildServiceAccount(dda, getClusterChecksRunnerServiceAccount(dda), version))

				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: true,
			wantFunc: func(c client.Client) error {
				ds := &appsv1.DaemonSet{}
				err := c.Get(context.TODO(), newRequest(resourcesNamespace, resourcesName).NamespacedName, ds)
				if apierrors.IsNotFound(err) {
					// The Cluster Agent exists but not available yet
					// Daemonset must NOT be created
					return nil
				}
				return err
			},
		},
		{
			name: "DatadogAgent found and defaulted, Cluster Checks Runner PDB Creation",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dadOptions := &test.NewDatadogAgentOptions{
						Labels:                     map[string]string{"label-foo-key": "label-bar-value"},
						Status:                     &datadoghqv1alpha1.DatadogAgentStatus{},
						ClusterAgentEnabled:        true,
						ClusterChecksEnabled:       true,
						ClusterChecksRunnerEnabled: true,
					}
					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, dadOptions)
					_ = c.Create(context.TODO(), dda)

					commonDCAlabels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
					dcaLabels := map[string]string{"label-foo-key": "label-bar-value"}
					for k, v := range commonDCAlabels {
						dcaLabels[k] = v
					}

					dcaOptions := &test.NewDeploymentOptions{
						Labels:                 dcaLabels,
						ForceAvailableReplicas: datadoghqv1alpha1.NewInt32Pointer(1),
					}
					dca := test.NewClusterAgentDeployment(resourcesNamespace, resourcesName, dcaOptions)

					_ = c.Create(context.TODO(), dda)
					_ = c.Create(context.TODO(), dca)
					_ = c.Create(context.TODO(), test.NewSecret(resourcesNamespace, "foo", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))

					createClusterAgentDependencies(c, dda)
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				pdb := &policyv1.PodDisruptionBudget{}
				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: rbacResourcesNameClusterChecksRunner}, pdb); err != nil {
					return err
				}
				if !ownedByDatadogOperator(pdb.OwnerReferences) {
					return fmt.Errorf("bad PDB, should be owned by the datadog operator, current owners: %v", pdb.OwnerReferences)
				}

				return nil
			},
		},
		{
			name: "DatadogAgent found and defaulted, Cluster Checks Runner PDB Update",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dadOptions := &test.NewDatadogAgentOptions{
						Labels:                     map[string]string{"label-foo-key": "label-bar-value"},
						Status:                     &datadoghqv1alpha1.DatadogAgentStatus{},
						ClusterAgentEnabled:        true,
						ClusterChecksEnabled:       true,
						ClusterChecksRunnerEnabled: true,
					}
					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, dadOptions)
					_ = c.Create(context.TODO(), dda)

					commonDCAlabels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
					dcaLabels := map[string]string{"label-foo-key": "label-bar-value"}
					for k, v := range commonDCAlabels {
						dcaLabels[k] = v
					}

					dcaOptions := &test.NewDeploymentOptions{
						Labels:                 dcaLabels,
						ForceAvailableReplicas: datadoghqv1alpha1.NewInt32Pointer(1),
					}
					dca := test.NewClusterAgentDeployment(resourcesNamespace, resourcesName, dcaOptions)

					_ = c.Create(context.TODO(), dda)
					_ = c.Create(context.TODO(), dca)
					_ = c.Create(context.TODO(), test.NewSecret(resourcesNamespace, "foo", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))

					createClusterAgentDependencies(c, dda)

					// Create wrong value PDB
					pdb := buildClusterChecksRunnerPDB(dda)
					wrongMinAvailable := intstr.FromInt(10)
					pdb.Spec.MinAvailable = &wrongMinAvailable
					_ = controllerutil.SetControllerReference(dda, pdb, s)
					_ = c.Create(context.TODO(), pdb)
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				pdb := &policyv1.PodDisruptionBudget{}
				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: rbacResourcesNameClusterChecksRunner}, pdb); err != nil {
					return err
				}
				if pdb.Spec.MinAvailable.IntValue() != pdbMinAvailableInstances {
					return fmt.Errorf("MinAvailable incorrect, expected %d, got %d", pdbMinAvailableInstances, pdb.Spec.MinAvailable.IntValue())
				}

				return nil
			},
		},
		{
			name: "DatadogAgent found and defaulted, Cluster Checks Runner ClusterRoleBidning creation",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dadOptions := &test.NewDatadogAgentOptions{
						Labels:                     map[string]string{"label-foo-key": "label-bar-value"},
						Status:                     &datadoghqv1alpha1.DatadogAgentStatus{},
						ClusterAgentEnabled:        true,
						ClusterChecksEnabled:       true,
						ClusterChecksRunnerEnabled: true,
					}
					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, dadOptions)
					_ = c.Create(context.TODO(), dda)

					commonDCAlabels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
					dcaLabels := map[string]string{"label-foo-key": "label-bar-value"}
					for k, v := range commonDCAlabels {
						dcaLabels[k] = v
					}

					dcaOptions := &test.NewDeploymentOptions{
						Labels:                 dcaLabels,
						ForceAvailableReplicas: datadoghqv1alpha1.NewInt32Pointer(1),
					}
					dca := test.NewClusterAgentDeployment(resourcesNamespace, resourcesName, dcaOptions)

					_ = c.Create(context.TODO(), dda)
					_ = c.Create(context.TODO(), dca)
					_ = c.Create(context.TODO(), test.NewSecret(resourcesNamespace, "foo", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))

					createClusterAgentDependencies(c, dda)
					createClusterChecksRunnerDependencies(c, dda)
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
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
			name: "DatadogAgent found and defaulted, Cluster Checks Runner Service Account creation",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dadOptions := &test.NewDatadogAgentOptions{
						Labels:                     map[string]string{"label-foo-key": "label-bar-value"},
						Status:                     &datadoghqv1alpha1.DatadogAgentStatus{},
						ClusterAgentEnabled:        true,
						ClusterChecksEnabled:       true,
						ClusterChecksRunnerEnabled: true,
					}
					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, dadOptions)
					_ = c.Create(context.TODO(), dda)

					commonDCAlabels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
					dcaLabels := map[string]string{"label-foo-key": "label-bar-value"}
					for k, v := range commonDCAlabels {
						dcaLabels[k] = v
					}

					dcaOptions := &test.NewDeploymentOptions{
						Labels:                 dcaLabels,
						ForceAvailableReplicas: datadoghqv1alpha1.NewInt32Pointer(1),
					}
					dca := test.NewClusterAgentDeployment(resourcesNamespace, resourcesName, dcaOptions)

					_ = c.Create(context.TODO(), dda)
					_ = c.Create(context.TODO(), dca)
					_ = c.Create(context.TODO(), test.NewSecret(resourcesNamespace, "foo", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))

					createClusterAgentDependencies(c, dda)
					createClusterChecksRunnerDependencies(c, dda)

					version := getClusterChecksRunnerVersion(dda)
					_ = c.Create(context.TODO(), buildClusterRoleBinding(dda, roleBindingInfo{
						name:               rbacResourcesNameClusterChecksRunner,
						roleName:           "foo-agent",
						serviceAccountName: rbacResourcesNameClusterChecksRunner,
					}, version))
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				rbacResourcesNameClusterChecksRunner := rbacResourcesNameClusterChecksRunner
				serviceAccount := &corev1.ServiceAccount{}
				if err := c.Get(context.TODO(), types.NamespacedName{Namespace: resourcesNamespace, Name: rbacResourcesNameClusterChecksRunner}, serviceAccount); err != nil {
					return err
				}
				if !ownedByDatadogOperator(serviceAccount.OwnerReferences) {
					return fmt.Errorf("bad serviceAccount, should be owned by the datadog operator, current owners: %v", serviceAccount.OwnerReferences)
				}

				return nil
			},
		},
		{
			name: "DatadogAgent found and defaulted, Cluster Checks Runner Deployment creation",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dadOptions := &test.NewDatadogAgentOptions{
						Labels:                     map[string]string{"label-foo-key": "label-bar-value"},
						Status:                     &datadoghqv1alpha1.DatadogAgentStatus{},
						ClusterAgentEnabled:        true,
						ClusterChecksEnabled:       true,
						ClusterChecksRunnerEnabled: true,
					}
					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, dadOptions)
					_ = c.Create(context.TODO(), dda)

					commonDCAlabels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
					dcaLabels := map[string]string{"label-foo-key": "label-bar-value"}
					for k, v := range commonDCAlabels {
						dcaLabels[k] = v
					}

					dcaOptions := &test.NewDeploymentOptions{
						Labels:                 dcaLabels,
						ForceAvailableReplicas: datadoghqv1alpha1.NewInt32Pointer(1),
					}
					dca := test.NewClusterAgentDeployment(resourcesNamespace, resourcesName, dcaOptions)

					_ = c.Create(context.TODO(), dda)
					_ = c.Create(context.TODO(), dca)
					_ = c.Create(context.TODO(), test.NewSecret(resourcesNamespace, "foo", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))

					createClusterAgentDependencies(c, dda)
					createAgentDependencies(c, dda)
					createClusterChecksRunnerDependencies(c, dda)

					resourceName := getAgentRbacResourcesName(dda)
					version := getAgentVersion(dda)
					_ = c.Create(context.TODO(), buildClusterRoleBinding(dda, roleBindingInfo{
						name:               getClusterChecksRunnerRbacResourcesName(dda),
						roleName:           resourceName,
						serviceAccountName: getClusterChecksRunnerServiceAccount(dda),
					}, version))
					_ = c.Create(context.TODO(), buildServiceAccount(dda, getClusterChecksRunnerServiceAccount(dda), version))

				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				dca := &appsv1.Deployment{}
				if err := c.Get(context.TODO(), newRequest(resourcesNamespace, "foo-cluster-agent").NamespacedName, dca); err != nil {
					return err
				}
				if dca.Name != "foo-cluster-agent" {
					return fmt.Errorf("dca bad name, should be: 'foo', current: %s", dca.Name)
				}

				dcaw := &appsv1.Deployment{}
				if err := c.Get(context.TODO(), newRequest(resourcesNamespace, rbacResourcesNameClusterChecksRunner).NamespacedName, dcaw); err != nil {
					return err
				}
				if dcaw.Name != rbacResourcesNameClusterChecksRunner {
					return fmt.Errorf("dcaw bad name, should be: 'foo', current: %s", dcaw.Name)
				}

				return nil
			},
		},
		{
			name: "DatadogAgent found and defaulted, Cluster Agent Deployment already exists but not up-to-date",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dadOptions := &test.NewDatadogAgentOptions{
						Labels:              map[string]string{"label-foo-key": "label-bar-value"},
						Status:              &datadoghqv1alpha1.DatadogAgentStatus{},
						ClusterAgentEnabled: true,
					}
					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, dadOptions)
					_ = c.Create(context.TODO(), dda)

					commonDCAlabels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
					dcaLabels := map[string]string{"label-foo-key": "label-bar-value"}
					for k, v := range commonDCAlabels {
						dcaLabels[k] = v
					}
					dcaOptions := &test.NewDeploymentOptions{
						Labels:      dcaLabels,
						Annotations: map[string]string{datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey: "outdated-hash"},
					}
					dca := test.NewClusterAgentDeployment(resourcesNamespace, "foo-cluster-agent", dcaOptions)

					_ = c.Create(context.TODO(), dda)
					_ = c.Create(context.TODO(), dca)
					_ = c.Create(context.TODO(), test.NewSecret(resourcesNamespace, "foo", &test.NewSecretOptions{Labels: commonDCAlabels, Data: map[string][]byte{
						"token": []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
					}}))

					createClusterAgentDependencies(c, dda)
					createClusterChecksRunnerDependencies(c, dda)
				},
			},
			want:    reconcile.Result{Requeue: true},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				dca := &appsv1.Deployment{}
				if err := c.Get(context.TODO(), newRequest(resourcesNamespace, "foo-cluster-agent").NamespacedName, dca); err != nil {
					return err
				}
				if dca.Annotations[datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey] == "outdated-hash" || dca.Annotations[datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey] == "" {
					return fmt.Errorf("dca bad hash, should be updated, current: %s", dca.Annotations[datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey])
				}
				if dca.OwnerReferences == nil || len(dca.OwnerReferences) != 1 {
					return fmt.Errorf("dca bad owner references, should be: '[Kind DatadogAgent - Name foo]', current: %v", dca.OwnerReferences)
				}

				return nil
			},
		},
		{
			name: "DatadogAgent found and defaulted, Agent network policies are created",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dadOptions := &test.NewDatadogAgentOptions{
						CreateNetworkPolicy: true,
					}

					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, dadOptions)
					_ = c.Create(context.TODO(), dda)

					createAgentDependencies(c, dda)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				ds := &appsv1.DaemonSet{}
				err := c.Get(context.TODO(), newRequest(resourcesNamespace, dsName).NamespacedName, ds)
				if err != nil {
					return err
				}

				policy := &networkingv1.NetworkPolicy{}
				err = c.Get(context.TODO(), newRequest(resourcesNamespace, dsName).NamespacedName, policy)
				if err != nil {
					return err
				}

				dsLabels := labels.Set(ds.Spec.Template.Labels)
				policySelector := labels.Set(policy.Spec.PodSelector.MatchLabels).AsSelector()
				if !policySelector.Matches(dsLabels) {
					return fmt.Errorf("network policy's selector %s does not match pods defined in the daemonset", policySelector)
				}

				return nil
			},
		},
		{
			name: "DatadogAgent found and defaulted, Cluster Agent network policies are created",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dadOptions := &test.NewDatadogAgentOptions{
						ClusterAgentEnabled: true,
						CreateNetworkPolicy: true,
					}

					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, dadOptions)
					_ = c.Create(context.TODO(), dda)

					createAgentDependencies(c, dda)
					createClusterAgentDependencies(c, dda)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				dca := &appsv1.Deployment{}
				err := c.Get(context.TODO(), newRequest(resourcesNamespace, rbacResourcesNameClusterAgent).NamespacedName, dca)
				if err != nil {
					return err
				}

				policy := &networkingv1.NetworkPolicy{}
				err = c.Get(context.TODO(), newRequest(resourcesNamespace, rbacResourcesNameClusterAgent).NamespacedName, policy)
				if err != nil {
					return err
				}

				dcaLabels := labels.Set(dca.Spec.Template.Labels)
				policySelector := labels.Set(policy.Spec.PodSelector.MatchLabels).AsSelector()
				if !policySelector.Matches(dcaLabels) {
					return fmt.Errorf("network policy's selector %s does not match pods defined in the daemonset", policySelector)
				}

				return nil
			},
		},
		{
			name: "DatadogAgent found and defaulted, Cluster Checks Runner network policies are created",
			fields: fields{
				client:   fake.NewFakeClient(),
				scheme:   s,
				recorder: recorder,
			},
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				loadFunc: func(c client.Client) {
					dadOptions := &test.NewDatadogAgentOptions{
						ClusterAgentEnabled:        true,
						ClusterChecksEnabled:       true,
						ClusterChecksRunnerEnabled: true,
						CreateNetworkPolicy:        true,
					}

					dda := test.NewDefaultedDatadogAgent(resourcesNamespace, resourcesName, dadOptions)
					_ = c.Create(context.TODO(), dda)

					dcaOptions := &test.NewDeploymentOptions{
						Labels:                 map[string]string{"label-foo-key": "label-bar-value"},
						ForceAvailableReplicas: datadoghqv1alpha1.NewInt32Pointer(1),
					}
					dca := test.NewClusterAgentDeployment(resourcesNamespace, resourcesName, dcaOptions)
					_ = c.Create(context.TODO(), dca)

					createAgentDependencies(c, dda)
					createClusterAgentDependencies(c, dda)
					createClusterChecksRunnerDependencies(c, dda)
				},
			},
			want:    reconcile.Result{RequeueAfter: defaultRequeueDuration},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				dca := &appsv1.Deployment{}
				err := c.Get(context.TODO(), newRequest(resourcesNamespace, rbacResourcesNameClusterChecksRunner).NamespacedName, dca)
				if err != nil {
					return err
				}

				policy := &networkingv1.NetworkPolicy{}
				err = c.Get(context.TODO(), newRequest(resourcesNamespace, rbacResourcesNameClusterChecksRunner).NamespacedName, policy)
				if err != nil {
					return err
				}

				dcaLabels := labels.Set(dca.Spec.Template.Labels)
				policySelector := labels.Set(policy.Spec.PodSelector.MatchLabels).AsSelector()
				if !policySelector.Matches(dcaLabels) {
					return fmt.Errorf("network policy's selector %s does not match pods defined in the daemonset", policySelector)
				}

				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{
				client:     tt.fields.client,
				scheme:     tt.fields.scheme,
				recorder:   recorder,
				log:        logf.Log.WithName(tt.name),
				forwarders: forwarders,
				options: ReconcilerOptions{
					SupportExtendedDaemonset: true,
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

func hasWpaRbacs(policyRules []rbacv1.PolicyRule) bool {
	requiredVerbs := []string{
		datadoghqv1alpha1.ListVerb,
		datadoghqv1alpha1.WatchVerb,
		datadoghqv1alpha1.GetVerb,
	}

	for _, policyRule := range policyRules {
		resourceFound := false
		groupFound := false
		verbsFound := false

		for _, resource := range policyRule.Resources {
			if resource == "watermarkpodautoscalers" {
				resourceFound = true
				break
			}
		}
		for _, group := range policyRule.APIGroups {
			if group == "datadoghq.com" {
				groupFound = true
				break
			}
		}
		if reflect.DeepEqual(policyRule.Verbs, requiredVerbs) {
			verbsFound = true
		}
		if resourceFound && groupFound && verbsFound {
			return true
		}
	}

	return false
}

func hasAdmissionRbacResources(policyRules []rbacv1.PolicyRule) bool {
	clusterLevelResources := map[string]bool{
		"secrets":                       true,
		"mutatingwebhookconfigurations": true,
		"replicasets":                   true,
		"deployments":                   true,
		"statefulsets":                  true,
		"cronjobs":                      true,
		"jobs":                          true,
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

func createSystemProbeDependencies(c client.Client, dda *datadoghqv1alpha1.DatadogAgent) {
	configCM, _ := buildSystemProbeConfigConfiMap(dda)
	securityCM, _ := buildSystemProbeSecCompConfigMap(dda)
	_ = c.Create(context.TODO(), configCM)
	_ = c.Create(context.TODO(), securityCM)
}

func createAgentDependencies(c client.Client, dda *datadoghqv1alpha1.DatadogAgent) {
	resourceName := getAgentRbacResourcesName(dda)
	version := getAgentVersion(dda)
	_ = c.Create(context.TODO(), buildAgentClusterRole(dda, resourceName, version))
	_ = c.Create(context.TODO(), buildClusterRoleBinding(dda, roleBindingInfo{
		name:               resourceName,
		roleName:           resourceName,
		serviceAccountName: getAgentServiceAccount(dda),
	}, version))
	_ = c.Create(context.TODO(), buildServiceAccount(dda, getAgentServiceAccount(dda), version))

	labels := getDefaultLabels(dda, datadoghqv1alpha1.DefaultAgentResourceSuffix, getAgentVersion(dda))
	_ = c.Create(context.TODO(), test.NewSecret(dda.ObjectMeta.Namespace, "foo", &test.NewSecretOptions{Labels: labels, Data: map[string][]byte{
		"api-key": []byte(base64.StdEncoding.EncodeToString([]byte("api-foo"))),
		"app-key": []byte(base64.StdEncoding.EncodeToString([]byte("app-foo"))),
		"token":   []byte(base64.StdEncoding.EncodeToString([]byte("token-foo"))),
	}}))

	installinfoCM, _ := buildInstallInfoConfigMap(dda)
	_ = c.Create(context.TODO(), installinfoCM)
}

func createClusterAgentDependencies(c client.Client, dda *datadoghqv1alpha1.DatadogAgent) {
	const resourcesName = "foo"
	const resourcesNamespace = "bar"

	version := getAgentVersion(dda)
	clusterAgentSAName := getClusterAgentServiceAccount(dda)
	_ = c.Create(context.TODO(), buildClusterAgentClusterRole(dda, "foo-cluster-agent", version))
	_ = c.Create(context.TODO(), buildClusterAgentRole(dda, "foo-cluster-agent", version))
	_ = c.Create(context.TODO(), buildServiceAccount(dda, clusterAgentSAName, version))
	info := roleBindingInfo{
		name:               "foo-cluster-agent",
		roleName:           "foo-cluster-agent",
		serviceAccountName: getClusterAgentServiceAccount(dda),
	}
	_ = c.Create(context.TODO(), buildClusterRoleBinding(dda, info, version))
	_ = c.Create(context.TODO(), buildRoleBinding(dda, info, version))
	_ = c.Create(context.TODO(), buildClusterAgentPDB(dda))

	dcaService := test.NewService(resourcesNamespace, "foo-cluster-agent", &test.NewServiceOptions{Spec: &corev1.ServiceSpec{
		Type: corev1.ServiceTypeClusterIP,
		Selector: map[string]string{
			datadoghqv1alpha1.AgentDeploymentNameLabelKey:      resourcesName,
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
	dcaService.Labels = getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
	_ = c.Create(context.TODO(), dcaService)

	installinfoCM, _ := buildInstallInfoConfigMap(dda)
	_ = c.Create(context.TODO(), installinfoCM)
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

func (dummyManager) MetricsForwarderStatusForObj(obj datadog.MonitoredObject) *datadoghqv1alpha1.DatadogAgentCondition {
	return nil
}

func createClusterChecksRunnerDependencies(c client.Client, dda *datadoghqv1alpha1.DatadogAgent) {
	_ = c.Create(context.TODO(), buildClusterChecksRunnerPDB(dda))

	installinfoCM, _ := buildInstallInfoConfigMap(dda)
	_ = c.Create(context.TODO(), installinfoCM)
}
