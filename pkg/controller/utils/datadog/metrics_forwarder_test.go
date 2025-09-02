// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadog

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/secrets"
	"github.com/DataDog/datadog-operator/pkg/testutils"

	"github.com/stretchr/testify/mock"
	assert "github.com/stretchr/testify/require"
	api "github.com/zorkian/go-datadog-api"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type fakeMetricsForwarder struct {
	mock.Mock
}

func (c *fakeMetricsForwarder) delegatedSendDeploymentMetric(metricValue float64, component string, tags []string) error {
	c.Called(metricValue, component, tags)
	return nil
}

func (c *fakeMetricsForwarder) delegatedSendReconcileMetric(metricValue float64, tags []string) error {
	c.Called(metricValue, tags)
	return nil
}

func (c *fakeMetricsForwarder) delegatedSendEvent(eventTitle string, eventType EventType) error {
	c.Called(eventTitle, eventType)
	return nil
}

func (c *fakeMetricsForwarder) delegatedSendFeatureMetric(feature string) error {
	c.Called(feature)
	return nil
}

func (c *fakeMetricsForwarder) delegatedValidateCreds(apiKey string) (*api.Client, error) {
	c.Called(apiKey)
	if strings.Contains(apiKey, "invalid") {
		return nil, errors.New("invalid creds")
	}
	return &api.Client{}, nil
}

func TestMetricsForwarder_updateCredsIfNeeded(t *testing.T) {
	tests := []struct {
		name     string
		loadFunc func() (*metricsForwarder, *fakeMetricsForwarder)
		apiKey   string
		wantErr  bool
		wantFunc func(*metricsForwarder, *fakeMetricsForwarder) error
	}{
		{
			name: "same creds, no update",
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				return &metricsForwarder{
					delegator: f,
					keysHash:  hashKeys("sameApiKey"),
				}, f
			},
			apiKey:  "sameApiKey",
			wantErr: false,
			wantFunc: func(m *metricsForwarder, f *fakeMetricsForwarder) error {
				if m.keysHash != hashKeys("sameApiKey") {
					return errors.New("Wrong hash update")
				}
				if !f.AssertNumberOfCalls(t, "delegatedValidateCreds", 0) {
					return errors.New("Wrong number of calls")
				}
				return nil
			},
		},
		{
			name: "new apiKey, update",
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedValidateCreds", "newApiKey")
				return &metricsForwarder{
					delegator: f,
					keysHash:  hashKeys("oldApiKey"),
				}, f
			},
			apiKey:  "newApiKey",
			wantErr: false,
			wantFunc: func(m *metricsForwarder, f *fakeMetricsForwarder) error {
				if m.keysHash != hashKeys("newApiKey") {
					return errors.New("Wrong hash update")
				}
				if !f.AssertNumberOfCalls(t, "delegatedValidateCreds", 1) {
					return errors.New("Wrong number of calls")
				}
				return nil
			},
		},
		{
			name: "invalid creds, no update",
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedValidateCreds", "invalidApiKey")
				return &metricsForwarder{
					delegator: f,
					keysHash:  hashKeys("oldApiKey"),
				}, f
			},
			apiKey:  "invalidApiKey",
			wantErr: true,
			wantFunc: func(m *metricsForwarder, f *fakeMetricsForwarder) error {
				if m.keysHash != hashKeys("oldApiKey") {
					return errors.New("Wrong hash update")
				}
				if !f.AssertNumberOfCalls(t, "delegatedValidateCreds", 1) {
					return errors.New("Wrong number of calls")
				}
				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dd, f := tt.loadFunc()
			if err := dd.updateCredsIfNeeded(tt.apiKey); (err != nil) != tt.wantErr {
				t.Errorf("metricsForwarder.updateCredsIfNeeded() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err := tt.wantFunc(dd, f); err != nil {
				t.Errorf("metricsForwarder.updateCredsIfNeeded() wantFunc validation error: %v", err)
			}
		})
	}
}

func Test_setupFromOperator(t *testing.T) {
	type fields struct {
		client client.Client
	}

	tests := []struct {
		name        string
		loadFunc    func(*metricsForwarder, *secrets.DummyDecryptor)
		wantAPIKey  string
		wantBaseURL string
	}{
		{
			name: "basic creds with default URL",
			loadFunc: func(m *metricsForwarder, d *secrets.DummyDecryptor) {
				os.Setenv(constants.DDAPIKey, "test123")
				os.Setenv(constants.DDAppKey, "testabc")
			},
			wantBaseURL: defaultbaseURL,
			wantAPIKey:  "test123",
		},
		{
			name: "basic creds with default DD_URL",
			loadFunc: func(m *metricsForwarder, d *secrets.DummyDecryptor) {
				os.Setenv(constants.DDAppKey, "testabc")
				os.Setenv(constants.DDAppKey, "testabc")
				os.Setenv(constants.DDddURL, "https://api.dd_url.com")
			},
			wantBaseURL: "https://api.dd_url.com",
			wantAPIKey:  "test123",
		},
		{
			name: "basic creds with default DD_DD_URL",
			loadFunc: func(m *metricsForwarder, d *secrets.DummyDecryptor) {
				os.Setenv(constants.DDAPIKey, "test123")
				os.Setenv(constants.DDAppKey, "testabc")
				os.Setenv(constants.DDddURL, "https://api.dd_dd_url.com")
			},
			wantBaseURL: "https://api.dd_dd_url.com",
			wantAPIKey:  "test123",
		},
		{
			name: "basic creds with default DD_SITE",
			loadFunc: func(m *metricsForwarder, d *secrets.DummyDecryptor) {
				os.Setenv(constants.DDAPIKey, "test123")
				os.Setenv(constants.DDAppKey, "testabc")
				os.Setenv(constants.DDSite, "dd_site.com")
			},
			wantBaseURL: "https://api.dd_site.com",
			wantAPIKey:  "test123",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &secrets.DummyDecryptor{}
			mf := &metricsForwarder{
				k8sClient:    fake.NewFakeClient(),
				decryptor:    d,
				creds:        sync.Map{},
				credsManager: config.NewCredentialManager(),
			}
			if tt.loadFunc != nil {
				tt.loadFunc(mf, d)
			}
			_ = mf.setupFromOperator()

			if mf.apiKey != tt.wantAPIKey {
				t.Errorf("metricsForwarder.setupFromOperator() apiKey = %v, want %v", mf.apiKey, tt.wantAPIKey)
			}
		})
	}
}

func Test_setupFromDDA(t *testing.T) {
	apiVersion := fmt.Sprintf("%s/%s", v2alpha1.GroupVersion.Group, v2alpha1.GroupVersion.Version)
	apiKey := "foundAPIKey"
	labels := map[string]string{
		"labelKey1": "labelValue1",
	}

	type args struct {
		dda                  *v2alpha1.DatadogAgent
		credsSetFromOperator bool
	}
	tests := []struct {
		name            string
		args            args
		wantLabels      map[string]string
		wantClusterName string
		wantBaseURL     string
		wantAPIKey      string
		wantDsStatus    []*v2alpha1.DaemonSetStatus
		wantDcaStatus   *v2alpha1.DeploymentStatus
		wantCcrStatus   *v2alpha1.DeploymentStatus
		wantErr         bool
	}{
		{
			name: "base URL, cluster name and API key",
			args: args{
				dda: &v2alpha1.DatadogAgent{
					TypeMeta: metav1.TypeMeta{
						Kind:       "DatadogAgent",
						APIVersion: apiVersion,
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "foo",
						Name:       "bar",
						Labels:     labels,
						Finalizers: []string{"finalizer.agent.datadoghq.com"},
					},
					Spec: v2alpha1.DatadogAgentSpec{
						Global: &v2alpha1.GlobalConfig{
							ClusterName: apiutils.NewStringPointer("test-cluster"),
							Credentials: &v2alpha1.DatadogCredentials{
								APIKey: apiutils.NewStringPointer(apiKey),
							},
						},
					},
				},
				credsSetFromOperator: false,
			},
			wantLabels:      labels,
			wantClusterName: "test-cluster",
			wantBaseURL:     defaultbaseURL,
			wantAPIKey:      "foundAPIKey",
			wantErr:         false,
		},
		{
			name: "creds set from operator, status and labels copied from DDA",
			args: args{
				dda: &v2alpha1.DatadogAgent{
					TypeMeta: metav1.TypeMeta{
						Kind:       "DatadogAgent",
						APIVersion: apiVersion,
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "foo",
						Name:       "bar",
						Labels:     labels,
						Finalizers: []string{"finalizer.agent.datadoghq.com"},
					},
					Spec: v2alpha1.DatadogAgentSpec{
						Global: &v2alpha1.GlobalConfig{
							ClusterName: apiutils.NewStringPointer("test-cluster"),
							Credentials: &v2alpha1.DatadogCredentials{
								APIKey: apiutils.NewStringPointer(apiKey),
							},
						},
					},
					Status: v2alpha1.DatadogAgentStatus{
						AgentList: []*v2alpha1.DaemonSetStatus{
							{
								DaemonsetName: "datadog-agent",
								State:         "Running",
								Desired:       3,
								Available:     3,
							},
						},
						ClusterAgent: &v2alpha1.DeploymentStatus{
							State:             "Running",
							Replicas:          1,
							AvailableReplicas: 1,
						},
						ClusterChecksRunner: &v2alpha1.DeploymentStatus{
							State:             "Running",
							Replicas:          2,
							AvailableReplicas: 2,
						},
					},
				},
				credsSetFromOperator: true,
			},
			wantLabels:      labels,
			wantClusterName: "test-cluster",
			wantBaseURL:     "", // Should not be set when credsSetFromOperator is true
			wantAPIKey:      "", // Should not be set when credsSetFromOperator is true
			wantDsStatus: []*v2alpha1.DaemonSetStatus{
				{
					DaemonsetName: "datadog-agent",
					State:         "Running",
					Desired:       3,
					Available:     3,
				},
			},
			wantDcaStatus: &v2alpha1.DeploymentStatus{
				State:             "Running",
				Replicas:          1,
				AvailableReplicas: 1,
			},
			wantCcrStatus: &v2alpha1.DeploymentStatus{
				State:             "Running",
				Replicas:          2,
				AvailableReplicas: 2,
			},
			wantErr: false,
		},
		{
			name: "creds not set from operator, status and labels copied from DDA",
			args: args{
				dda: &v2alpha1.DatadogAgent{
					TypeMeta: metav1.TypeMeta{
						Kind:       "DatadogAgent",
						APIVersion: apiVersion,
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace:  "foo",
						Name:       "bar",
						Labels:     labels,
						Finalizers: []string{"finalizer.agent.datadoghq.com"},
					},
					Spec: v2alpha1.DatadogAgentSpec{
						Global: &v2alpha1.GlobalConfig{
							ClusterName: apiutils.NewStringPointer("test-cluster"),
							Credentials: &v2alpha1.DatadogCredentials{
								APIKey: apiutils.NewStringPointer(apiKey),
							},
						},
					},
					Status: v2alpha1.DatadogAgentStatus{
						AgentList: []*v2alpha1.DaemonSetStatus{
							{
								DaemonsetName: "datadog-agent",
								State:         "Pending",
								Desired:       2,
								Available:     1,
							},
						},
						ClusterAgent: &v2alpha1.DeploymentStatus{
							State:             "Pending",
							Replicas:          1,
							AvailableReplicas: 0,
						},
						ClusterChecksRunner: &v2alpha1.DeploymentStatus{
							State:             "Running",
							Replicas:          1,
							AvailableReplicas: 1,
						},
					},
				},
				credsSetFromOperator: false,
			},
			wantLabels:      labels,
			wantClusterName: "test-cluster",
			wantBaseURL:     defaultbaseURL,
			wantAPIKey:      "foundAPIKey",
			wantDsStatus: []*v2alpha1.DaemonSetStatus{
				{
					DaemonsetName: "datadog-agent",
					State:         "Pending",
					Desired:       2,
					Available:     1,
				},
			},
			wantDcaStatus: &v2alpha1.DeploymentStatus{
				State:             "Pending",
				Replicas:          1,
				AvailableReplicas: 0,
			},
			wantCcrStatus: &v2alpha1.DeploymentStatus{
				State:             "Running",
				Replicas:          1,
				AvailableReplicas: 1,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &secrets.DummyDecryptor{}
			mf := &metricsForwarder{
				k8sClient:    fake.NewFakeClient(),
				decryptor:    d,
				creds:        sync.Map{},
				credsManager: config.NewCredentialManager(),
			}

			err := mf.setupFromDDA(tt.args.dda, tt.args.credsSetFromOperator)
			if (err != nil) != tt.wantErr {
				t.Errorf("metricsForwarder.setupFromDDA() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if mf.apiKey != tt.wantAPIKey {
				t.Errorf("metricsForwarder.setupFromDDA() apiKey = %v, want %v", mf.apiKey, tt.wantAPIKey)
			}
			if mf.baseURL != tt.wantBaseURL {
				t.Errorf("metricsForwarder.setupFromDDA() baseURL = %v, want %v", mf.baseURL, tt.wantBaseURL)
			}
			if mf.clusterName != tt.wantClusterName {
				t.Errorf("metricsForwarder.setupFromDDA() clusterName = %v, want %v", mf.clusterName, tt.wantClusterName)
			}
			for k, v := range mf.labels {
				if tt.wantLabels[k] != v {
					t.Errorf("metricsForwarder.setupFromDDA() label value = %v, want %v", v, tt.wantLabels[k])
				}
			}

			// Check status fields are copied over
			if !reflect.DeepEqual(mf.dsStatus, tt.wantDsStatus) {
				t.Errorf("metricsForwarder.setupFromDDA() dsStatus = %v, want %v", mf.dsStatus, tt.wantDsStatus)
			}
			if !reflect.DeepEqual(mf.dcaStatus, tt.wantDcaStatus) {
				t.Errorf("metricsForwarder.setupFromDDA() dcaStatus = %v, want %v", mf.dcaStatus, tt.wantDcaStatus)
			}
			if !reflect.DeepEqual(mf.ccrStatus, tt.wantCcrStatus) {
				t.Errorf("metricsForwarder.setupFromDDA() ccrStatus = %v, want %v", mf.ccrStatus, tt.wantCcrStatus)
			}
		})
	}
}

func Test_getCredentialsFromDDA(t *testing.T) {
	apiKey := "foundAPIKey"

	encAPIKey := "ENC[APIKey]"

	type fields struct {
		client client.Client
	}
	type args struct {
		dda      *v2alpha1.DatadogAgent
		loadFunc func(*metricsForwarder, *secrets.DummyDecryptor)
	}
	tests := []struct {
		name       string
		fields     fields
		args       args
		wantAPIKey string
		wantErr    bool
		wantFunc   func(*metricsForwarder, *secrets.DummyDecryptor) error
	}{
		{
			name: "creds found in CR",
			fields: fields{
				client: fake.NewFakeClient(),
			},
			args: args{
				dda: testutils.NewDatadogAgent("foo", "bar", &v2alpha1.GlobalConfig{
					Credentials: &v2alpha1.DatadogCredentials{
						APIKey: apiutils.NewStringPointer(apiKey),
					},
				}),
			},
			wantAPIKey: "foundAPIKey",
			wantErr:    false,
		},
		{
			name: "creds found in secrets",
			fields: fields{
				client: fake.NewFakeClient(),
			},
			args: args{
				dda: testutils.NewDatadogAgent("foo", "bar", &v2alpha1.GlobalConfig{
					Credentials: &v2alpha1.DatadogCredentials{
						APISecret: &v2alpha1.SecretConfig{
							SecretName: "datadog-creds-api",
							KeyName:    "datadog_api_key",
						},
					},
				}),
				loadFunc: func(m *metricsForwarder, d *secrets.DummyDecryptor) {
					secret := &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "datadog-creds-api",
							Namespace: "foo",
						},
						Data: map[string][]byte{
							"datadog_api_key": []byte(apiKey),
						},
					}
					_ = m.k8sClient.Create(context.TODO(), secret)
				},
			},
			wantAPIKey: "foundAPIKey",
			wantErr:    false,
		},
		{
			name: "enc creds found in cache",
			fields: fields{
				client: fake.NewFakeClient(),
			},
			args: args{
				dda: testutils.NewDatadogAgent("foo", "bar", &v2alpha1.GlobalConfig{
					Credentials: &v2alpha1.DatadogCredentials{
						APIKey: apiutils.NewStringPointer(encAPIKey),
					},
				}),
				loadFunc: func(m *metricsForwarder, d *secrets.DummyDecryptor) {
					m.cleanSecretsCache()
					m.creds.Store(encAPIKey, "cachedAPIKey")
				},
			},
			wantAPIKey: "cachedAPIKey",
			wantErr:    false,
			wantFunc: func(m *metricsForwarder, d *secrets.DummyDecryptor) error {
				if !d.AssertNumberOfCalls(t, "Decrypt", 0) {
					return errors.New("Wrong number of calls")
				}
				d.AssertExpectations(t)
				return nil
			},
		},
		{
			name: "enc creds not found in cache, call secret backend",
			fields: fields{
				client: fake.NewFakeClient(),
			},
			args: args{
				dda: testutils.NewDatadogAgent("foo", "bar", &v2alpha1.GlobalConfig{
					Credentials: &v2alpha1.DatadogCredentials{
						APIKey: apiutils.NewStringPointer(encAPIKey),
					},
				}),
				loadFunc: func(m *metricsForwarder, d *secrets.DummyDecryptor) {
					m.cleanSecretsCache()
					d.On("Decrypt", []string{encAPIKey}).Once()
				},
			},
			wantAPIKey: "DEC[ENC[APIKey]]",
			wantErr:    false,
			wantFunc: func(m *metricsForwarder, d *secrets.DummyDecryptor) error {
				v, found := m.creds.Load(encAPIKey)
				assert.True(t, found)
				assert.Equal(t, "DEC[ENC[APIKey]]", v)

				d.AssertExpectations(t)
				return nil
			},
		},
		{
			name: "nil credentials doesn't cause segmentation fault",
			fields: fields{
				client: fake.NewFakeClient(),
			},
			args: args{
				dda: testutils.NewDatadogAgent("foo", "bar", &v2alpha1.GlobalConfig{}),
				loadFunc: func(m *metricsForwarder, d *secrets.DummyDecryptor) {
				},
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &secrets.DummyDecryptor{}
			mf := &metricsForwarder{
				k8sClient:    tt.fields.client,
				decryptor:    d,
				creds:        sync.Map{},
				credsManager: config.NewCredentialManager(),
			}
			if tt.args.loadFunc != nil {
				tt.args.loadFunc(mf, d)
			}
			apiKey, err := mf.getCredentialsFromDDA(tt.args.dda)
			if (err != nil) != tt.wantErr {
				t.Errorf("metricsForwarder.getCredentialsFromDDA() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if apiKey != tt.wantAPIKey {
				t.Errorf("metricsForwarder.getCredentialsFromDDA() apiKey = %v, want %v", apiKey, tt.wantAPIKey)
			}
			if tt.wantFunc != nil {
				if err := tt.wantFunc(mf, d); err != nil {
					t.Errorf("metricsForwarder.getCredentialsFromDDA() wantFunc validation error: %v", err)
				}
			}
		})
	}
}

func Test_metricsForwarder_processReconcileError(t *testing.T) {
	platformInfo := kubernetes.NewPlatformInfoFromVersionMaps(
		nil,
		map[string]string{},
		map[string]string{},
	)

	nsn := types.NamespacedName{
		Namespace: "foo",
		Name:      "bar",
	}
	mf := &metricsForwarder{
		namespacedName:      nsn,
		monitoredObjectKind: "DatadogAgent",
		platformInfo:        &platformInfo,
	}
	mf.initGlobalTags()

	tests := []struct {
		name     string
		loadFunc func() (*metricsForwarder, *fakeMetricsForwarder)
		err      error
		wantErr  bool
		wantFunc func(*fakeMetricsForwarder) error
	}{
		{
			name: "last error init value, new unknown error => send unsuccess metric",
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedSendReconcileMetric", 0.0, []string{"kube_namespace:foo", "resource_name:bar", "reconcile_err:err_msg", "cr_preferred_version:null"}).Once()
				mf.delegator = f
				mf.lastReconcileErr = errInitValue
				return mf, f
			},
			err:     errors.New("err_msg"),
			wantErr: false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				f.AssertExpectations(t)
				return nil
			},
		},
		{
			name: "last error init value, new auth error => send unsuccess metric",
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedSendReconcileMetric", 0.0, []string{"kube_namespace:foo", "resource_name:bar", "reconcile_err:Unauthorized", "cr_preferred_version:null"}).Once()
				mf.delegator = f
				mf.lastReconcileErr = errInitValue
				return mf, f
			},
			err:     apierrors.NewUnauthorized("Auth error"),
			wantErr: false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				f.AssertExpectations(t)
				return nil
			},
		},
		{
			name: "last error init value, new error is nil => send success metric",
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedSendReconcileMetric", 1.0, []string{"kube_namespace:foo", "resource_name:bar", "reconcile_err:null", "cr_preferred_version:null"}).Once()
				mf.delegator = f
				mf.lastReconcileErr = errInitValue
				return mf, f
			},
			err:     nil,
			wantErr: false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				f.AssertExpectations(t)
				return nil
			},
		},
		{
			name: "last error nil, new error is nil => don't send metric",
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				mf.delegator = f
				mf.lastReconcileErr = nil
				return mf, f
			},
			err:     nil,
			wantErr: false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertNumberOfCalls(t, "delegatedSendReconcileMetric", 0) {
					return errors.New("Wrong number of calls")
				}
				f.AssertExpectations(t)
				return nil
			},
		},
		{
			name: "last error not nil and not init value, new error equals last error => don't send metric",
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				mf.delegator = f
				mf.lastReconcileErr = apierrors.NewUnauthorized("Auth error")
				return mf, f
			},
			err:     apierrors.NewUnauthorized("Auth error"),
			wantErr: false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertNumberOfCalls(t, "delegatedSendReconcileMetric", 0) {
					return errors.New("Wrong number of calls")
				}
				f.AssertExpectations(t)
				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dd, f := tt.loadFunc()
			if err := dd.processReconcileError(tt.err); (err != nil) != tt.wantErr {
				t.Errorf("metricsForwarder.processReconcileError() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err := tt.wantFunc(f); err != nil {
				t.Errorf("metricsForwarder.processReconcileError() wantFunc validation error: %v", err)
			}
		})
	}
}

func Test_metricsForwarder_prepareReconcileMetric(t *testing.T) {
	defaultGlobalTags := []string{"gtagkey:gtagvalue"}
	defaultTags := []string{"tagkey:tagvalue"}
	tests := []struct {
		name         string
		reconcileErr error
		want         float64
		want1        []string
		wantErr      bool
	}{
		{
			name:         "lastReconcileErr init value",
			reconcileErr: errInitValue,
			want:         0.0,
			want1:        nil,
			wantErr:      true,
		},
		{
			name:         "lastReconcileErr nil",
			reconcileErr: nil,
			want:         1.0,
			want1:        []string{"gtagkey:gtagvalue", "tagkey:tagvalue", "reconcile_err:null", "cr_preferred_version:v1", "cr_other_version:v1alpha1"},
			wantErr:      false,
		},
		{
			name:         "lastReconcileErr updated and not nil",
			reconcileErr: apierrors.NewUnauthorized("Auth error"),
			want:         0.0,
			want1:        []string{"gtagkey:gtagvalue", "tagkey:tagvalue", "reconcile_err:Unauthorized", "cr_preferred_version:v1", "cr_other_version:v1alpha1"},
			wantErr:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mf := &metricsForwarder{
				globalTags:          defaultGlobalTags,
				tags:                defaultTags,
				monitoredObjectKind: "DatadogAgent",
				platformInfo:        createPlatformInfo(),
			}
			got, got1, err := mf.prepareReconcileMetric(tt.reconcileErr)
			if (err != nil) != tt.wantErr {
				t.Errorf("metricsForwarder.prepareReconcileMetric() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("metricsForwarder.prepareReconcileMetric() got = %v, want %v", got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("metricsForwarder.prepareReconcileMetric() got1 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func Test_metricsForwarder_cleanSecretsCache(t *testing.T) {
	mf := &metricsForwarder{
		creds: sync.Map{},
	}

	mf.creds.Store("k", "v")

	mf.cleanSecretsCache()

	_, found := mf.creds.Load("k")
	assert.False(t, found)

	mf.creds.Range(func(k, v interface{}) bool {
		t.Error("creds cache not empty")
		return false
	})
}

func Test_metricsForwarder_resetSecretsCache(t *testing.T) {
	mf := &metricsForwarder{
		creds: sync.Map{},
	}

	mf.resetSecretsCache(map[string]string{
		"k": "v",
	})

	v, found := mf.creds.Load("k")
	assert.True(t, found)
	assert.Equal(t, "v", v)

	mf.resetSecretsCache(map[string]string{
		"kk":  "vv",
		"kkk": "vvv",
	})

	_, found = mf.creds.Load("k")
	assert.False(t, found)

	v, found = mf.creds.Load("kk")
	assert.True(t, found)
	assert.Equal(t, "vv", v)

	v, found = mf.creds.Load("kkk")
	assert.True(t, found)
	assert.Equal(t, "vvv", v)
}

func Test_metricsForwarder_getSecretsFromCache(t *testing.T) {
	type args struct {
		encAPIKey string
	}
	tests := []struct {
		name   string
		cached map[string]string
		args   args
		want   string
		want1  bool
	}{
		{
			name: "cache hit",
			cached: map[string]string{
				"apiKey": "decApiKey",
			},
			args: args{
				encAPIKey: "apiKey",
			},
			want:  "decApiKey",
			want1: true,
		},
		{
			name: "apiKey cache miss",
			cached: map[string]string{
				"foo": "bar",
			},
			args: args{
				encAPIKey: "apiKey",
			},
			want:  "",
			want1: false,
		},
		{
			name:   "total cache miss",
			cached: map[string]string{},
			args: args{
				encAPIKey: "apiKey",
			},
			want:  "",
			want1: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mf := &metricsForwarder{
				creds: sync.Map{},
			}
			for k, v := range tt.cached {
				mf.creds.Store(k, v)
			}
			got, got1 := mf.getSecretsFromCache(tt.args.encAPIKey)
			if got != tt.want {
				t.Errorf("metricsForwarder.getSecretsFromCache() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("metricsForwarder.getSecretsFromCache() got2 = %v, want %v", got1, tt.want1)
			}
		})
	}
}

func Test_getbaseURL(t *testing.T) {
	euSite := "datadoghq.eu"

	type args struct {
		dda *v2alpha1.DatadogAgent
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Get default baseURL",
			args: args{
				dda: testutils.NewDatadogAgent("foo", "bar", nil),
			},
			want: "https://api.datadoghq.com",
		},
		{
			name: "Compute baseURL from site when passing Site",
			args: args{
				dda: testutils.NewDatadogAgent("foo", "bar", &v2alpha1.GlobalConfig{
					Site: &euSite,
				}),
			},
			want: "https://api.datadoghq.eu",
		},
		{
			name: "Compute baseURL from endpoint.URL when Site is not defined",
			args: args{
				dda: testutils.NewDatadogAgent("foo", "bar", &v2alpha1.GlobalConfig{
					Endpoint: &v2alpha1.Endpoint{
						URL: apiutils.NewStringPointer("https://test.url.com"),
					},
				}),
			},
			want: "https://test.url.com",
		},
		{
			name: "Test that DDUrl takes precedence over Site",
			args: args{
				dda: testutils.NewDatadogAgent("foo", "bar", &v2alpha1.GlobalConfig{
					Site: &euSite,
					Endpoint: &v2alpha1.Endpoint{
						URL: apiutils.NewStringPointer("https://test.url.com"),
					},
				}),
			},
			want: "https://test.url.com",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getbaseURL(&tt.args.dda.Spec); got != tt.want {
				t.Errorf("getbaseURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func createPlatformInfo() *kubernetes.PlatformInfo {
	platformInfo := kubernetes.NewPlatformInfoFromVersionMaps(
		nil,
		map[string]string{
			"DatadogAgent": "v1",
		},
		map[string]string{
			"DatadogAgent": "v1alpha1",
		},
	)
	return &platformInfo
}

func TestMetricsForwarder_sendFeatureMetric(t *testing.T) {
	fmf := &fakeMetricsForwarder{}
	nsn := types.NamespacedName{
		Namespace: "foo",
		Name:      "bar",
	}
	mf := &metricsForwarder{
		namespacedName:      nsn,
		delegator:           fmf,
		monitoredObjectKind: "DatadogAgent",
	}
	mf.initGlobalTags()

	tests := []struct {
		name     string
		loadFunc func() (*metricsForwarder, *fakeMetricsForwarder)
		feature  string
		tags     []string
		wantErr  bool
		wantFunc func(*fakeMetricsForwarder) error
	}{
		{
			name: "send feature metric",
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedSendFeatureMetric", "test_feature")
				mf.delegator = f
				return mf, f
			},
			feature: "test_feature",
			wantErr: false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertCalled(t, "delegatedSendFeatureMetric", "test_feature") {
					return errors.New("Function not called")
				}
				if !f.AssertNumberOfCalls(t, "delegatedSendFeatureMetric", 1) {
					return errors.New("Wrong number of calls")
				}
				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dd, f := tt.loadFunc()
			if err := dd.sendFeatureMetric(tt.feature); (err != nil) != tt.wantErr {
				t.Errorf("metricsForwarder.sendFeatureMetric() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err := tt.wantFunc(f); err != nil {
				t.Errorf("metricsForwarder.sendFeatureMetric() wantFunc validation error: %v", err)
			}
		})
	}
}

func Test_setEnabledFeatures(t *testing.T) {
	tests := []struct {
		name            string
		enabledFeatures []string
		expected        map[string][]string
	}{
		{
			name:            "empty features",
			enabledFeatures: []string{},
			expected:        map[string][]string{"foo": {}},
		},
		{
			name:            "one feature",
			enabledFeatures: []string{"feature1"},
			expected:        map[string][]string{"foo": {"feature1"}},
		},
		{
			name:            "multiple features",
			enabledFeatures: []string{"feature1", "feature2"},
			expected:        map[string][]string{"foo": {"feature1", "feature2"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mf := &metricsForwarder{
				id: "foo",
			}

			mf.setEnabledFeatures(tt.enabledFeatures)
			assert.Equal(t, tt.expected, mf.EnabledFeatures)
		})
	}
}
