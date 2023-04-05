// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadog

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	test "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1/test"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	testV2 "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1/test"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/secrets"

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

func (c *fakeMetricsForwarder) delegatedValidateCreds(apiKey, appKey string) (*api.Client, error) {
	c.Called(apiKey, appKey)
	if strings.Contains(apiKey, "invalid") || strings.Contains(appKey, "invalid") {
		return nil, errors.New("invalid creds")
	}
	return &api.Client{}, nil
}

func TestMetricsForwarder_sendStatusMetrics(t *testing.T) {
	fmf := &fakeMetricsForwarder{}
	nsn := types.NamespacedName{
		Namespace: "foo",
		Name:      "bar",
	}
	mf := &metricsForwarder{
		namespacedName:      nsn,
		delegator:           fmf,
		monitoredObjectKind: "DatadogAgent",
		platformInfo:        createPlatformInfo(),
	}
	mf.initGlobalTags()

	tests := []struct {
		name      string
		loadFunc  func() (*metricsForwarder, *fakeMetricsForwarder)
		dsStatus  *commonv1.DaemonSetStatus
		dcaStatus *commonv1.DeploymentStatus
		ccrStatus *commonv1.DeploymentStatus
		wantErr   bool
		wantFunc  func(*fakeMetricsForwarder) error
	}{
		{
			name: "nil statuses",
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				return mf, fmf
			},
			dsStatus:  nil,
			dcaStatus: nil,
			ccrStatus: nil,
			wantErr:   false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertNumberOfCalls(t, "delegatedSendDeploymentMetric", 0) {
					return errors.New("Wrong number of calls")
				}
				return nil
			},
		},
		{
			name: "agent only, available",
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedSendDeploymentMetric", 1.0, "agent", []string{"cr_namespace:foo", "cr_name:bar", "state:Running", "cr_preferred_version:v1", "cr_other_version:v1alpha1"})
				mf.delegator = f
				return mf, f
			},
			dsStatus: &commonv1.DaemonSetStatus{
				Desired:   int32(1337),
				Available: int32(1337),
				State:     string(datadoghqv1alpha1.DatadogAgentStateRunning),
			},
			dcaStatus: nil,
			ccrStatus: nil,
			wantErr:   false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 1.0, "agent", []string{"cr_namespace:foo", "cr_name:bar", "state:Running", "cr_preferred_version:v1", "cr_other_version:v1alpha1"}) {
					return errors.New("Function not called")
				}
				if !f.AssertNumberOfCalls(t, "delegatedSendDeploymentMetric", 1) {
					return errors.New("Wrong number of calls")
				}
				return nil
			},
		},
		{
			name: "agent only, available + tags not empty",
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedSendDeploymentMetric", 1.0, "agent", []string{"cr_namespace:foo", "cr_name:bar", "cluster_name:testcluster", "state:Running", "cr_preferred_version:v1", "cr_other_version:v1alpha1"})
				mf.delegator = f
				mf.tags = []string{"cluster_name:testcluster"}
				return mf, f
			},
			dsStatus: &commonv1.DaemonSetStatus{
				Desired:   int32(1337),
				Available: int32(1337),
				State:     string(datadoghqv1alpha1.DatadogAgentStateRunning),
			},
			dcaStatus: nil,
			ccrStatus: nil,
			wantErr:   false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 1.0, "agent", []string{"cr_namespace:foo", "cr_name:bar", "cluster_name:testcluster", "state:Running", "cr_preferred_version:v1", "cr_other_version:v1alpha1"}) {
					return errors.New("Function not called")
				}
				if !f.AssertNumberOfCalls(t, "delegatedSendDeploymentMetric", 1) {
					return errors.New("Wrong number of calls")
				}
				return nil
			},
		},
		{
			name: "agent only, not available",
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedSendDeploymentMetric", 0.0, "agent", []string{"cr_namespace:foo", "cr_name:bar", "state:Failed", "cr_preferred_version:v1", "cr_other_version:v1alpha1"})
				mf.delegator = f
				mf.tags = []string{}
				return mf, f
			},
			dsStatus: &commonv1.DaemonSetStatus{
				Desired:   int32(1337),
				Available: int32(1336),
				State:     string(datadoghqv1alpha1.DatadogAgentStateFailed),
			},
			dcaStatus: nil,
			ccrStatus: nil,
			wantErr:   false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 0.0, "agent", []string{"cr_namespace:foo", "cr_name:bar", "state:Failed", "cr_preferred_version:v1", "cr_other_version:v1alpha1"}) {
					return errors.New("Function not called")
				}
				if !f.AssertNumberOfCalls(t, "delegatedSendDeploymentMetric", 1) {
					return errors.New("Wrong number of calls")
				}
				return nil
			},
		},
		{
			name: "all components, all available",
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedSendDeploymentMetric", 1.0, "agent", []string{"cr_namespace:foo", "cr_name:bar", "state:Running", "cr_preferred_version:v1", "cr_other_version:v1alpha1"})
				f.On("delegatedSendDeploymentMetric", 1.0, "clusteragent", []string{"cr_namespace:foo", "cr_name:bar", "state:Running", "cr_preferred_version:v1", "cr_other_version:v1alpha1"})
				f.On("delegatedSendDeploymentMetric", 1.0, "clusterchecksrunner", []string{"cr_namespace:foo", "cr_name:bar", "state:Running", "cr_preferred_version:v1", "cr_other_version:v1alpha1"})
				mf.delegator = f
				return mf, f
			},
			dsStatus: &commonv1.DaemonSetStatus{
				Desired:   int32(1337),
				Available: int32(1337),
				State:     string(datadoghqv1alpha1.DatadogAgentStateRunning),
			},
			dcaStatus: &commonv1.DeploymentStatus{
				Replicas:          int32(2),
				AvailableReplicas: int32(2),
				State:             string(datadoghqv1alpha1.DatadogAgentStateRunning),
			},
			ccrStatus: &commonv1.DeploymentStatus{
				Replicas:          int32(3),
				AvailableReplicas: int32(3),
				State:             string(datadoghqv1alpha1.DatadogAgentStateRunning),
			},
			wantErr: false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 1.0, "agent", []string{"cr_namespace:foo", "cr_name:bar", "state:Running", "cr_preferred_version:v1", "cr_other_version:v1alpha1"}) {
					return errors.New("Function not called")
				}
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 1.0, "clusteragent", []string{"cr_namespace:foo", "cr_name:bar", "state:Running", "cr_preferred_version:v1", "cr_other_version:v1alpha1"}) {
					return errors.New("Function not called")
				}
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 1.0, "clusterchecksrunner", []string{"cr_namespace:foo", "cr_name:bar", "state:Running", "cr_preferred_version:v1", "cr_other_version:v1alpha1"}) {
					return errors.New("Function not called")
				}
				if !f.AssertNumberOfCalls(t, "delegatedSendDeploymentMetric", 3) {
					return errors.New("Wrong number of calls")
				}
				return nil
			},
		},
		{
			name: "agent and clusteragent, clusteragent not available",
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedSendDeploymentMetric", 1.0, "agent", []string{"cr_namespace:foo", "cr_name:bar", "state:Running", "cr_preferred_version:v1", "cr_other_version:v1alpha1"})
				f.On("delegatedSendDeploymentMetric", 0.0, "clusteragent", []string{"cr_namespace:foo", "cr_name:bar", "state:Progressing", "cr_preferred_version:v1", "cr_other_version:v1alpha1"})
				mf.delegator = f
				return mf, f
			},
			dsStatus: &commonv1.DaemonSetStatus{
				Desired:   int32(1337),
				Available: int32(1337),
				State:     string(datadoghqv1alpha1.DatadogAgentStateRunning),
			},
			dcaStatus: &commonv1.DeploymentStatus{
				Replicas:          int32(2),
				AvailableReplicas: int32(0),
				State:             string(datadoghqv1alpha1.DatadogAgentStateProgressing),
			},
			ccrStatus: nil,
			wantErr:   false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 1.0, "agent", []string{"cr_namespace:foo", "cr_name:bar", "state:Running", "cr_preferred_version:v1", "cr_other_version:v1alpha1"}) {
					return errors.New("Function not called")
				}
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 0.0, "clusteragent", []string{"cr_namespace:foo", "cr_name:bar", "state:Progressing", "cr_preferred_version:v1", "cr_other_version:v1alpha1"}) {
					return errors.New("Function not called")
				}
				if !f.AssertNumberOfCalls(t, "delegatedSendDeploymentMetric", 2) {
					return errors.New("Wrong number of calls")
				}
				return nil
			},
		},
		{
			name: "all components, clusterchecksrunner not available",
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedSendDeploymentMetric", 1.0, "agent", []string{"cr_namespace:foo", "cr_name:bar", "state:Running", "cr_preferred_version:v1", "cr_other_version:v1alpha1"})
				f.On("delegatedSendDeploymentMetric", 1.0, "clusteragent", []string{"cr_namespace:foo", "cr_name:bar", "state:Running", "cr_preferred_version:v1", "cr_other_version:v1alpha1"})
				f.On("delegatedSendDeploymentMetric", 0.0, "clusterchecksrunner", []string{"cr_namespace:foo", "cr_name:bar", "state:Running", "cr_preferred_version:v1", "cr_other_version:v1alpha1"})
				mf.delegator = f
				return mf, f
			},
			dsStatus: &commonv1.DaemonSetStatus{
				Desired:   int32(1337),
				Available: int32(1337),
				State:     string(datadoghqv1alpha1.DatadogAgentStateRunning),
			},
			dcaStatus: &commonv1.DeploymentStatus{
				Replicas:          int32(2),
				AvailableReplicas: int32(2),
				State:             string(datadoghqv1alpha1.DatadogAgentStateRunning),
			},
			ccrStatus: &commonv1.DeploymentStatus{
				Replicas:          int32(3),
				AvailableReplicas: int32(1),
				State:             string(datadoghqv1alpha1.DatadogAgentStateRunning),
			},
			wantErr: false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 1.0, "agent", []string{"cr_namespace:foo", "cr_name:bar", "state:Running", "cr_preferred_version:v1", "cr_other_version:v1alpha1"}) {
					return errors.New("Function not called")
				}
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 1.0, "clusteragent", []string{"cr_namespace:foo", "cr_name:bar", "state:Running", "cr_preferred_version:v1", "cr_other_version:v1alpha1"}) {
					return errors.New("Function not called")
				}
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 0.0, "clusterchecksrunner", []string{"cr_namespace:foo", "cr_name:bar", "state:Running", "cr_preferred_version:v1", "cr_other_version:v1alpha1"}) {
					return errors.New("Function not called")
				}
				if !f.AssertNumberOfCalls(t, "delegatedSendDeploymentMetric", 3) {
					return errors.New("Wrong number of calls")
				}
				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dd, f := tt.loadFunc()
			if err := dd.sendStatusMetrics(tt.dsStatus, tt.dcaStatus, tt.ccrStatus); (err != nil) != tt.wantErr {
				t.Errorf("metricsForwarder.sendStatusMetrics() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err := tt.wantFunc(f); err != nil {
				t.Errorf("metricsForwarder.sendStatusMetrics() wantFunc validation error: %v", err)
			}
		})
	}
}

func TestMetricsForwarder_updateCredsIfNeeded(t *testing.T) {
	tests := []struct {
		name     string
		loadFunc func() (*metricsForwarder, *fakeMetricsForwarder)
		apiKey   string
		appKey   string
		wantErr  bool
		wantFunc func(*metricsForwarder, *fakeMetricsForwarder) error
	}{
		{
			name: "same creds, no update",
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				return &metricsForwarder{
					delegator: f,
					keysHash:  hashKeys("sameApiKey", "sameAppKey"),
				}, f
			},
			apiKey:  "sameApiKey",
			appKey:  "sameAppKey",
			wantErr: false,
			wantFunc: func(m *metricsForwarder, f *fakeMetricsForwarder) error {
				if m.keysHash != hashKeys("sameApiKey", "sameAppKey") {
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
				f.On("delegatedValidateCreds", "newApiKey", "sameAppKey")
				return &metricsForwarder{
					delegator: f,
					keysHash:  hashKeys("oldApiKey", "sameAppKey"),
				}, f
			},
			apiKey:  "newApiKey",
			appKey:  "sameAppKey",
			wantErr: false,
			wantFunc: func(m *metricsForwarder, f *fakeMetricsForwarder) error {
				if m.keysHash != hashKeys("newApiKey", "sameAppKey") {
					return errors.New("Wrong hash update")
				}
				if !f.AssertNumberOfCalls(t, "delegatedValidateCreds", 1) {
					return errors.New("Wrong number of calls")
				}
				return nil
			},
		},
		{
			name: "new appKey, update",
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedValidateCreds", "sameApiKey", "newAppKey")
				return &metricsForwarder{
					delegator: f,
					keysHash:  hashKeys("sameApiKey", "oldAppKey"),
				}, f
			},
			apiKey:  "sameApiKey",
			appKey:  "newAppKey",
			wantErr: false,
			wantFunc: func(m *metricsForwarder, f *fakeMetricsForwarder) error {
				if m.keysHash != hashKeys("sameApiKey", "newAppKey") {
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
				f.On("delegatedValidateCreds", "invalidApiKey", "invalidAppKey")
				return &metricsForwarder{
					delegator: f,
					keysHash:  hashKeys("oldApiKey", "oldAppKey"),
				}, f
			},
			apiKey:  "invalidApiKey",
			appKey:  "invalidAppKey",
			wantErr: true,
			wantFunc: func(m *metricsForwarder, f *fakeMetricsForwarder) error {
				if m.keysHash != hashKeys("oldApiKey", "oldAppKey") {
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
			if err := dd.updateCredsIfNeeded(tt.apiKey, tt.appKey); (err != nil) != tt.wantErr {
				t.Errorf("metricsForwarder.updateCredsIfNeeded() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err := tt.wantFunc(dd, f); err != nil {
				t.Errorf("metricsForwarder.updateCredsIfNeeded() wantFunc validation error: %v", err)
			}
		})
	}
}

func TestReconcileDatadogAgent_getCredentialsV2(t *testing.T) {
	apiKey := "foundAPIKey"
	appKey := "foundAppKey"

	encAPIKey := "ENC[APIKey]"
	encAppKey := "ENC[AppKey]"

	type fields struct {
		client client.Client
	}
	type args struct {
		dda      *datadoghqv2alpha1.DatadogAgent
		loadFunc func(*metricsForwarder, *secrets.DummyDecryptor)
	}
	tests := []struct {
		name       string
		fields     fields
		args       args
		wantAPIKey string
		wantAppKey string
		wantErr    bool
		wantFunc   func(*metricsForwarder, *secrets.DummyDecryptor) error
	}{
		{
			name: "creds found in CR",
			fields: fields{
				client: fake.NewFakeClient(),
			},
			args: args{
				dda: testV2.NewDatadogAgent("foo", "bar", &datadoghqv2alpha1.GlobalConfig{
					Credentials: &datadoghqv2alpha1.DatadogCredentials{
						APIKey: apiutils.NewStringPointer(apiKey),
						AppKey: apiutils.NewStringPointer(appKey),
					},
				}),
			},
			wantAPIKey: "foundAPIKey",
			wantAppKey: "foundAppKey",
			wantErr:    false,
		},
		{
			name: "appKey missing",
			fields: fields{
				client: fake.NewFakeClient(),
			},
			args: args{
				dda: testV2.NewDatadogAgent("foo", "bar", &datadoghqv2alpha1.GlobalConfig{
					Credentials: &datadoghqv2alpha1.DatadogCredentials{
						APIKey: apiutils.NewStringPointer(apiKey),
					},
				}),
			},
			wantAPIKey: "",
			wantAppKey: "",
			wantErr:    true,
		},
		{
			name: "creds found in secrets",
			fields: fields{
				client: fake.NewFakeClient(),
			},
			args: args{
				dda: testV2.NewDatadogAgent("foo", "bar", &datadoghqv2alpha1.GlobalConfig{
					Credentials: &datadoghqv2alpha1.DatadogCredentials{
						APISecret: &commonv1.SecretConfig{
							SecretName: "datadog-creds-api",
							KeyName:    "datadog_api_key",
						},
						AppSecret: &commonv1.SecretConfig{
							SecretName: "datadog-creds-app",
							KeyName:    "application_key",
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

					secret = &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "datadog-creds-app",
							Namespace: "foo",
						},
						Data: map[string][]byte{
							"application_key": []byte(appKey),
						},
					}
					_ = m.k8sClient.Create(context.TODO(), secret)
				},
			},
			wantAPIKey: "foundAPIKey",
			wantAppKey: "foundAppKey",
			wantErr:    false,
		},
		{
			name: "apiKey found in CR, appKey found in secret",
			fields: fields{
				client: fake.NewFakeClient(),
			},
			args: args{
				dda: testV2.NewDatadogAgent("foo", "bar", &datadoghqv2alpha1.GlobalConfig{
					Credentials: &datadoghqv2alpha1.DatadogCredentials{
						APIKey: &apiKey,
						AppSecret: &commonv1.SecretConfig{
							SecretName: "datadog-creds",
						},
					},
				}),
				loadFunc: func(m *metricsForwarder, d *secrets.DummyDecryptor) {
					secret := &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "datadog-creds",
							Namespace: "foo",
						},
						Data: map[string][]byte{
							"app_key": []byte(appKey),
						},
					}
					_ = m.k8sClient.Create(context.TODO(), secret)
				},
			},
			wantAPIKey: "foundAPIKey",
			wantAppKey: "foundAppKey",
			wantErr:    false,
		},
		{
			name: "enc creds found in cache",
			fields: fields{
				client: fake.NewFakeClient(),
			},
			args: args{
				dda: testV2.NewDatadogAgent("foo", "bar", &datadoghqv2alpha1.GlobalConfig{
					Credentials: &datadoghqv2alpha1.DatadogCredentials{
						APIKey: apiutils.NewStringPointer(encAPIKey),
						AppKey: apiutils.NewStringPointer(encAppKey),
					},
				}),
				loadFunc: func(m *metricsForwarder, d *secrets.DummyDecryptor) {
					m.cleanSecretsCache()
					m.creds.Store(encAPIKey, "cachedAPIKey")
					m.creds.Store(encAppKey, "cachedAppKey")
				},
			},
			wantAPIKey: "cachedAPIKey",
			wantAppKey: "cachedAppKey",
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
				dda: testV2.NewDatadogAgent("foo", "bar", &datadoghqv2alpha1.GlobalConfig{
					Credentials: &datadoghqv2alpha1.DatadogCredentials{
						APIKey: apiutils.NewStringPointer(encAPIKey),
						AppKey: apiutils.NewStringPointer(encAppKey),
					},
				}),
				loadFunc: func(m *metricsForwarder, d *secrets.DummyDecryptor) {
					m.cleanSecretsCache()
					d.On("Decrypt", []string{encAPIKey, encAppKey}).Once()
				},
			},
			wantAPIKey: "DEC[ENC[APIKey]]",
			wantAppKey: "DEC[ENC[AppKey]]",
			wantErr:    false,
			wantFunc: func(m *metricsForwarder, d *secrets.DummyDecryptor) error {
				v, found := m.creds.Load(encAPIKey)
				assert.True(t, found)
				assert.Equal(t, "DEC[ENC[APIKey]]", v)

				v, found = m.creds.Load(encAppKey)
				assert.True(t, found)
				assert.Equal(t, "DEC[ENC[AppKey]]", v)

				d.AssertExpectations(t)
				return nil
			},
		},
		{
			name: "nil credentials not causes segmentation fault",
			fields: fields{
				client: fake.NewFakeClient(),
			},
			args: args{
				dda: testV2.NewDatadogAgent("foo", "bar", &datadoghqv2alpha1.GlobalConfig{}),
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &secrets.DummyDecryptor{}
			mf := &metricsForwarder{
				k8sClient: tt.fields.client,
				decryptor: d,
				creds:     sync.Map{},
			}
			if tt.args.loadFunc != nil {
				tt.args.loadFunc(mf, d)
			}
			apiKey, appKey, err := mf.getCredentialsV2(tt.args.dda)
			if (err != nil) != tt.wantErr {
				t.Errorf("metricsForwarder.getCredentialsV2() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if apiKey != tt.wantAPIKey {
				t.Errorf("metricsForwarder.getCredentialsV2() apiKey = %v, want %v", apiKey, tt.wantAPIKey)
			}
			if appKey != tt.wantAppKey {
				t.Errorf("metricsForwarder.getCredentialsV2() appKey = %v, want %v", appKey, tt.wantAppKey)
			}
			if tt.wantFunc != nil {
				if err := tt.wantFunc(mf, d); err != nil {
					t.Errorf("metricsForwarder.getCredentialsV2() wantFunc validation error: %v", err)
				}
			}
		})
	}
}

func TestReconcileDatadogAgent_getCredsFromDatadogAgent(t *testing.T) {
	type fields struct {
		client client.Client
	}
	type args struct {
		dda      *datadoghqv1alpha1.DatadogAgent
		loadFunc func(*metricsForwarder, *secrets.DummyDecryptor)
	}
	tests := []struct {
		name       string
		fields     fields
		args       args
		wantAPIKey string
		wantAPPKey string
		wantErr    bool
		wantFunc   func(*metricsForwarder, *secrets.DummyDecryptor) error
	}{
		{
			name: "creds found in CR",
			fields: fields{
				client: fake.NewFakeClient(),
			},
			args: args{
				dda: test.NewDefaultedDatadogAgent("foo", "bar",
					&test.NewDatadogAgentOptions{
						Creds: &datadoghqv1alpha1.AgentCredentials{
							DatadogCredentials: datadoghqv1alpha1.DatadogCredentials{
								APIKey: "foundApiKey",
								AppKey: "foundAppKey",
							},
						},
					}),
			},
			wantAPIKey: "foundApiKey",
			wantAPPKey: "foundAppKey",
			wantErr:    false,
		},
		{
			name: "appKey missing",
			fields: fields{
				client: fake.NewFakeClient(),
			},
			args: args{
				dda: test.NewDefaultedDatadogAgent(
					"foo",
					"bar",
					&test.NewDatadogAgentOptions{
						Creds: &datadoghqv1alpha1.AgentCredentials{
							DatadogCredentials: datadoghqv1alpha1.DatadogCredentials{
								APIKey: "foundApiKey",
							},
						},
					}),
			},
			wantAPIKey: "",
			wantAPPKey: "",
			wantErr:    true,
		},
		{
			name: "creds found in secrets",
			fields: fields{
				client: fake.NewFakeClient(),
			},
			args: args{
				dda: test.NewDefaultedDatadogAgent("foo", "bar",
					&test.NewDatadogAgentOptions{
						Creds: &datadoghqv1alpha1.AgentCredentials{
							DatadogCredentials: datadoghqv1alpha1.DatadogCredentials{
								APISecret: &commonv1.SecretConfig{
									SecretName: "datadog-creds-api",
									KeyName:    "datadog_api_key",
								},
								APPSecret: &commonv1.SecretConfig{
									SecretName: "datadog-creds-app",
									KeyName:    "application_key",
								},
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
							"datadog_api_key": []byte("foundApiKey"),
						},
					}
					_ = m.k8sClient.Create(context.TODO(), secret)

					secret = &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "datadog-creds-app",
							Namespace: "foo",
						},
						Data: map[string][]byte{
							"application_key": []byte("foundAppKey"),
						},
					}
					_ = m.k8sClient.Create(context.TODO(), secret)
				},
			},
			wantAPIKey: "foundApiKey",
			wantAPPKey: "foundAppKey",
			wantErr:    false,
		},
		{
			name: "creds found in deprecated secrets",
			fields: fields{
				client: fake.NewFakeClient(),
			},
			args: args{
				dda: test.NewDefaultedDatadogAgent("foo", "bar",
					&test.NewDatadogAgentOptions{
						Creds: &datadoghqv1alpha1.AgentCredentials{
							DatadogCredentials: datadoghqv1alpha1.DatadogCredentials{
								APIKeyExistingSecret: "datadog-creds",
								AppKeyExistingSecret: "datadog-creds",
							},
						},
					}),
				loadFunc: func(m *metricsForwarder, d *secrets.DummyDecryptor) {
					secret := &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "datadog-creds",
							Namespace: "foo",
						},
						Data: map[string][]byte{
							"api_key": []byte("foundApiKey"),
							"app_key": []byte("foundAppKey"),
						},
					}
					_ = m.k8sClient.Create(context.TODO(), secret)
				},
			},
			wantAPIKey: "foundApiKey",
			wantAPPKey: "foundAppKey",
			wantErr:    false,
		},
		{
			name: "apiKey found in CR, appKey found in secret",
			fields: fields{
				client: fake.NewFakeClient(),
			},
			args: args{
				dda: test.NewDefaultedDatadogAgent("foo", "bar",
					&test.NewDatadogAgentOptions{
						Creds: &datadoghqv1alpha1.AgentCredentials{
							DatadogCredentials: datadoghqv1alpha1.DatadogCredentials{
								APIKey: "foundApiKey",
								APPSecret: &commonv1.SecretConfig{
									SecretName: "datadog-creds",
								},
							},
						},
					}),
				loadFunc: func(m *metricsForwarder, d *secrets.DummyDecryptor) {
					secret := &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "datadog-creds",
							Namespace: "foo",
						},
						Data: map[string][]byte{
							"app_key": []byte("foundAppKey"),
						},
					}
					_ = m.k8sClient.Create(context.TODO(), secret)
				},
			},
			wantAPIKey: "foundApiKey",
			wantAPPKey: "foundAppKey",
			wantErr:    false,
		},
		{
			name: "apiKey found in CR, appKey found in deprecated secret",
			fields: fields{
				client: fake.NewFakeClient(),
			},
			args: args{
				dda: test.NewDefaultedDatadogAgent("foo", "bar",
					&test.NewDatadogAgentOptions{
						Creds: &datadoghqv1alpha1.AgentCredentials{
							DatadogCredentials: datadoghqv1alpha1.DatadogCredentials{
								APIKey:               "foundApiKey",
								AppKeyExistingSecret: "datadog-creds",
							},
						},
					}),
				loadFunc: func(m *metricsForwarder, d *secrets.DummyDecryptor) {
					secret := &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "datadog-creds",
							Namespace: "foo",
						},
						Data: map[string][]byte{
							"app_key": []byte("foundAppKey"),
						},
					}
					_ = m.k8sClient.Create(context.TODO(), secret)
				},
			},
			wantAPIKey: "foundApiKey",
			wantAPPKey: "foundAppKey",
			wantErr:    false,
		},
		{
			name: "enc creds found in cache",
			fields: fields{
				client: fake.NewFakeClient(),
			},
			args: args{
				dda: test.NewDefaultedDatadogAgent("foo", "bar",
					&test.NewDatadogAgentOptions{
						Creds: &datadoghqv1alpha1.AgentCredentials{
							DatadogCredentials: datadoghqv1alpha1.DatadogCredentials{
								APIKey: "ENC[ApiKey]",
								AppKey: "ENC[AppKey]",
							},
						},
					}),
				loadFunc: func(m *metricsForwarder, d *secrets.DummyDecryptor) {
					m.cleanSecretsCache()
					m.creds.Store("ENC[ApiKey]", "cachedApiKey")
					m.creds.Store("ENC[AppKey]", "cachedAppKey")
				},
			},
			wantAPIKey: "cachedApiKey",
			wantAPPKey: "cachedAppKey",
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
				dda: test.NewDefaultedDatadogAgent("foo", "bar",
					&test.NewDatadogAgentOptions{
						Creds: &datadoghqv1alpha1.AgentCredentials{
							DatadogCredentials: datadoghqv1alpha1.DatadogCredentials{
								APIKey: "ENC[ApiKey]",
								AppKey: "ENC[AppKey]",
							},
						},
					}),
				loadFunc: func(m *metricsForwarder, d *secrets.DummyDecryptor) {
					m.cleanSecretsCache()
					d.On("Decrypt", []string{"ENC[ApiKey]", "ENC[AppKey]"}).Once()
				},
			},
			wantAPIKey: "DEC[ENC[ApiKey]]",
			wantAPPKey: "DEC[ENC[AppKey]]",
			wantErr:    false,
			wantFunc: func(m *metricsForwarder, d *secrets.DummyDecryptor) error {
				v, found := m.creds.Load("ENC[ApiKey]")
				assert.True(t, found)
				assert.Equal(t, "DEC[ENC[ApiKey]]", v)

				v, found = m.creds.Load("ENC[AppKey]")
				assert.True(t, found)
				assert.Equal(t, "DEC[ENC[AppKey]]", v)

				d.AssertExpectations(t)
				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &secrets.DummyDecryptor{}
			mf := &metricsForwarder{
				k8sClient: tt.fields.client,
				decryptor: d,
				creds:     sync.Map{},
			}
			if tt.args.loadFunc != nil {
				tt.args.loadFunc(mf, d)
			}
			apiKey, appKey, err := mf.getCredsFromDatadogAgent(tt.args.dda)
			if (err != nil) != tt.wantErr {
				t.Errorf("metricsForwarder.getCredsFromDatadogAgent() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if apiKey != tt.wantAPIKey {
				t.Errorf("metricsForwarder.getCredsFromDatadogAgent() apiKey = %v, want %v", apiKey, tt.wantAPIKey)
			}
			if appKey != tt.wantAPPKey {
				t.Errorf("metricsForwarder.getCredsFromDatadogAgent() appKey = %v, want %v", appKey, tt.wantAPPKey)
			}
			if tt.wantFunc != nil {
				if err := tt.wantFunc(mf, d); err != nil {
					t.Errorf("metricsForwarder.getCredsFromDatadogAgent() wantFunc validation error: %v", err)
				}
			}
		})
	}
}

func TestMetricsForwarder_setTags(t *testing.T) {
	tests := []struct {
		name        string
		clusterName string
		labels      map[string]string
		want        []string
	}{
		{
			name:   "empty labels",
			labels: map[string]string{},
			want:   []string{},
		},
		{
			name: "with labels",
			labels: map[string]string{
				"firstKey":  "firstValue",
				"secondKey": "secondValue",
			},
			want: []string{
				"firstKey:firstValue",
				"secondKey:secondValue",
			},
		},
		{
			name:        "with clustername",
			clusterName: "testcluster",
			want: []string{
				"cluster_name:testcluster",
			},
		},
		{
			name:        "with clustername and labels",
			clusterName: "testcluster",
			labels: map[string]string{
				"firstKey":  "firstValue",
				"secondKey": "secondValue",
			},
			want: []string{
				"cluster_name:testcluster",
				"firstKey:firstValue",
				"secondKey:secondValue",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dd := &metricsForwarder{}
			dd.updateTags(tt.clusterName, tt.labels)

			sort.Strings(dd.tags)
			sort.Strings(tt.want)
			if !reflect.DeepEqual(dd.tags, tt.want) {
				t.Errorf("metricsForwarder.setTags() dd.tags = %v, want %v", dd.tags, tt.want)
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
			name: "last error init value, new unknown error => send unsucess metric",
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedSendReconcileMetric", 0.0, []string{"cr_namespace:foo", "cr_name:bar", "reconcile_err:err_msg", "cr_preferred_version:null"}).Once()
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
			name: "last error init value, new auth error => send unsucess metric",
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedSendReconcileMetric", 0.0, []string{"cr_namespace:foo", "cr_name:bar", "reconcile_err:Unauthorized", "cr_preferred_version:null"}).Once()
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
				f.On("delegatedSendReconcileMetric", 1.0, []string{"cr_namespace:foo", "cr_name:bar", "reconcile_err:null", "cr_preferred_version:null"}).Once()
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
	mf.creds.Store("kk", "vv")

	mf.cleanSecretsCache()

	_, found := mf.creds.Load("k")
	assert.False(t, found)

	_, found = mf.creds.Load("kk")
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
		encAPPKey string
	}
	tests := []struct {
		name   string
		cached map[string]string
		args   args
		want   string
		want1  string
		want2  bool
	}{
		{
			name: "cache hit",
			cached: map[string]string{
				"apiKey": "decApiKey",
				"appKey": "decAppKey",
			},
			args: args{
				encAPIKey: "apiKey",
				encAPPKey: "appKey",
			},
			want:  "decApiKey",
			want1: "decAppKey",
			want2: true,
		},
		{
			name: "apiKey cache miss",
			cached: map[string]string{
				"appKey": "decAppKey",
			},
			args: args{
				encAPIKey: "apiKey",
				encAPPKey: "appKey",
			},
			want:  "",
			want1: "",
			want2: false,
		},
		{
			name: "appKey cache miss",
			cached: map[string]string{
				"apiKey": "decApiKey",
			},
			args: args{
				encAPIKey: "apiKey",
				encAPPKey: "appKey",
			},
			want:  "",
			want1: "",
			want2: false,
		},
		{
			name:   "total cache miss",
			cached: map[string]string{},
			args: args{
				encAPIKey: "apiKey",
				encAPPKey: "appKey",
			},
			want:  "",
			want1: "",
			want2: false,
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
			got, got1, got2 := mf.getSecretsFromCache(tt.args.encAPIKey, tt.args.encAPPKey)
			if got != tt.want {
				t.Errorf("metricsForwarder.getSecretsFromCache() got = %v, want %v", got, tt.want)
			}
			if got1 != tt.want1 {
				t.Errorf("metricsForwarder.getSecretsFromCache() got1 = %v, want %v", got1, tt.want1)
			}
			if got2 != tt.want2 {
				t.Errorf("metricsForwarder.getSecretsFromCache() got2 = %v, want %v", got2, tt.want2)
			}
		})
	}
}

func Test_getbaseURL(t *testing.T) {
	type args struct {
		dda *datadoghqv1alpha1.DatadogAgent
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Get default baseURL",
			args: args{
				dda: test.NewDefaultedDatadogAgent("foo", "bar", &test.NewDatadogAgentOptions{}),
			},
			want: "https://api.datadoghq.com",
		},
		{
			name: "Compute baseURL from site when passing Site",
			args: args{
				dda: test.NewDefaultedDatadogAgent("foo", "bar", &test.NewDatadogAgentOptions{
					Site: "datadoghq.eu",
				}),
			},
			want: "https://api.datadoghq.eu",
		},
		{
			name: "Compute baseURL from ddUrl when Site is not defined",
			args: args{
				dda: test.NewDefaultedDatadogAgent("foo", "bar", &test.NewDatadogAgentOptions{
					NodeAgentConfig: &datadoghqv1alpha1.NodeAgentConfig{
						DDUrl: apiutils.NewStringPointer("https://test.url.com"),
					},
				}),
			},
			want: "https://test.url.com",
		},
		{
			name: "Test that DDUrl takes precedence over Site",
			args: args{
				dda: test.NewDefaultedDatadogAgent("foo", "bar", &test.NewDatadogAgentOptions{
					Site: "datadoghq.eu",
					NodeAgentConfig: &datadoghqv1alpha1.NodeAgentConfig{
						DDUrl: apiutils.NewStringPointer("https://test.url.com"),
					},
				}),
			},
			want: "https://test.url.com",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getbaseURL(tt.args.dda); got != tt.want {
				t.Errorf("getbaseURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getbaseURLV2(t *testing.T) {
	euSite := "datadoghq.eu"

	type args struct {
		dda *datadoghqv2alpha1.DatadogAgent
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "Get default baseURL",
			args: args{
				dda: testV2.NewDatadogAgent("foo", "bar", nil),
			},
			want: "https://api.datadoghq.com",
		},
		{
			name: "Compute baseURL from site when passing Site",
			args: args{
				dda: testV2.NewDatadogAgent("foo", "bar", &datadoghqv2alpha1.GlobalConfig{
					Site: &euSite,
				}),
			},
			want: "https://api.datadoghq.eu",
		},
		{
			name: "Compute baseURL from endpoint.URL when Site is not defined",
			args: args{
				dda: testV2.NewDatadogAgent("foo", "bar", &datadoghqv2alpha1.GlobalConfig{
					Endpoint: &datadoghqv2alpha1.Endpoint{
						URL: apiutils.NewStringPointer("https://test.url.com"),
					},
				}),
			},
			want: "https://test.url.com",
		},
		{
			name: "Test that DDUrl takes precedence over Site",
			args: args{
				dda: testV2.NewDatadogAgent("foo", "bar", &datadoghqv2alpha1.GlobalConfig{
					Site: &euSite,
					Endpoint: &datadoghqv2alpha1.Endpoint{
						URL: apiutils.NewStringPointer("https://test.url.com"),
					},
				}),
			},
			want: "https://test.url.com",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getbaseURLV2(tt.args.dda); got != tt.want {
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
