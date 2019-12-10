// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadog

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	test "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1/test"
	"github.com/stretchr/testify/mock"
	api "github.com/zorkian/go-datadog-api"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func (c *fakeMetricsForwarder) delegatedValidateCreds(apiKey, appKey string) (*api.Client, error) {
	c.Called(apiKey, appKey)
	if strings.Contains(apiKey, "invalid") || strings.Contains(appKey, "invalid") {
		return nil, errors.New("invalid creds")
	}
	return &api.Client{}, nil
}

func TestMetricsForwarder_SendStatusMetrics(t *testing.T) {
	tests := []struct {
		name     string
		loadFunc func() (*MetricsForwarder, *fakeMetricsForwarder)
		status   *datadoghqv1alpha1.DatadogAgentDeploymentStatus
		wantErr  bool
		wantFunc func(*fakeMetricsForwarder) error
	}{
		{
			name: "empty status",
			loadFunc: func() (*MetricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				return &MetricsForwarder{
					delegator: f,
				}, f
			},
			status:  &datadoghqv1alpha1.DatadogAgentDeploymentStatus{},
			wantErr: false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertNumberOfCalls(t, "delegatedSendDeploymentMetric", 0) {
					return errors.New("Wrong number of calls")
				}
				return nil
			},
		},
		{
			name: "nil status",
			loadFunc: func() (*MetricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				return &MetricsForwarder{
					delegator: f,
				}, f
			},
			status:  nil,
			wantErr: true,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertNumberOfCalls(t, "delegatedSendDeploymentMetric", 0) {
					return errors.New("Wrong number of calls")
				}
				return nil
			},
		},
		{
			name: "agent only, available",
			loadFunc: func() (*MetricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedSendDeploymentMetric", 1.0, "agent", []string{"state:Running"})
				return &MetricsForwarder{
					delegator: f,
				}, f
			},
			status: &datadoghqv1alpha1.DatadogAgentDeploymentStatus{
				Agent: &datadoghqv1alpha1.DatadogAgentDeploymentAgentStatus{
					Desired:   int32(1337),
					Available: int32(1337),
					State:     datadoghqv1alpha1.DatadogAgentDeploymentAgentStateRunning,
				},
			},
			wantErr: false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 1.0, "agent", []string{"state:Running"}) {
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
			loadFunc: func() (*MetricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedSendDeploymentMetric", 1.0, "agent", []string{"cluster_name:testcluster", "state:Running"})
				return &MetricsForwarder{
					delegator: f,
					tags:      []string{"cluster_name:testcluster"},
				}, f
			},
			status: &datadoghqv1alpha1.DatadogAgentDeploymentStatus{
				Agent: &datadoghqv1alpha1.DatadogAgentDeploymentAgentStatus{
					Desired:   int32(1337),
					Available: int32(1337),
					State:     datadoghqv1alpha1.DatadogAgentDeploymentAgentStateRunning,
				},
			},
			wantErr: false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 1.0, "agent", []string{"cluster_name:testcluster", "state:Running"}) {
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
			loadFunc: func() (*MetricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedSendDeploymentMetric", 0.0, "agent", []string{"state:Failed"})
				return &MetricsForwarder{
					delegator: f,
				}, f
			},
			status: &datadoghqv1alpha1.DatadogAgentDeploymentStatus{
				Agent: &datadoghqv1alpha1.DatadogAgentDeploymentAgentStatus{
					Desired:   int32(1337),
					Available: int32(1336),
					State:     datadoghqv1alpha1.DatadogAgentDeploymentAgentStateFailed,
				},
			},
			wantErr: false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 0.0, "agent", []string{"state:Failed"}) {
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
			loadFunc: func() (*MetricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedSendDeploymentMetric", 1.0, "agent", []string{"state:Running"})
				f.On("delegatedSendDeploymentMetric", 1.0, "clusteragent", []string{"state:Running"})
				f.On("delegatedSendDeploymentMetric", 1.0, "clustercheckrunner", []string{"state:Running"})
				return &MetricsForwarder{
					delegator: f,
				}, f
			},
			status: &datadoghqv1alpha1.DatadogAgentDeploymentStatus{
				Agent: &datadoghqv1alpha1.DatadogAgentDeploymentAgentStatus{
					Desired:   int32(1337),
					Available: int32(1337),
					State:     datadoghqv1alpha1.DatadogAgentDeploymentAgentStateRunning,
				},
				ClusterAgent: &datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStatus{
					Replicas:          int32(2),
					AvailableReplicas: int32(2),
					State:             datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStateRunning,
				},
				ClusterChecksRunner: &datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStatus{
					Replicas:          int32(3),
					AvailableReplicas: int32(3),
					State:             datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStateRunning,
				},
			},
			wantErr: false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 1.0, "agent", []string{"state:Running"}) {
					return errors.New("Function not called")
				}
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 1.0, "clusteragent", []string{"state:Running"}) {
					return errors.New("Function not called")
				}
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 1.0, "clustercheckrunner", []string{"state:Running"}) {
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
			loadFunc: func() (*MetricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedSendDeploymentMetric", 1.0, "agent", []string{"state:Running"})
				f.On("delegatedSendDeploymentMetric", 0.0, "clusteragent", []string{"state:Started"})
				return &MetricsForwarder{
					delegator: f,
				}, f
			},
			status: &datadoghqv1alpha1.DatadogAgentDeploymentStatus{
				Agent: &datadoghqv1alpha1.DatadogAgentDeploymentAgentStatus{
					Desired:   int32(1337),
					Available: int32(1337),
					State:     datadoghqv1alpha1.DatadogAgentDeploymentAgentStateRunning,
				},
				ClusterAgent: &datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStatus{
					Replicas:          int32(2),
					AvailableReplicas: int32(0),
					State:             datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStateStarted,
				},
			},
			wantErr: false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 1.0, "agent", []string{"state:Running"}) {
					return errors.New("Function not called")
				}
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 0.0, "clusteragent", []string{"state:Started"}) {
					return errors.New("Function not called")
				}
				if !f.AssertNumberOfCalls(t, "delegatedSendDeploymentMetric", 2) {
					return errors.New("Wrong number of calls")
				}
				return nil
			},
		},
		{
			name: "all components, clustercheckrunner not available",
			loadFunc: func() (*MetricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedSendDeploymentMetric", 1.0, "agent", []string{"state:Running"})
				f.On("delegatedSendDeploymentMetric", 1.0, "clusteragent", []string{"state:Running"})
				f.On("delegatedSendDeploymentMetric", 0.0, "clustercheckrunner", []string{"state:Running"})
				return &MetricsForwarder{
					delegator: f,
				}, f
			},
			status: &datadoghqv1alpha1.DatadogAgentDeploymentStatus{
				Agent: &datadoghqv1alpha1.DatadogAgentDeploymentAgentStatus{
					Desired:   int32(1337),
					Available: int32(1337),
					State:     datadoghqv1alpha1.DatadogAgentDeploymentAgentStateRunning,
				},
				ClusterAgent: &datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStatus{
					Replicas:          int32(2),
					AvailableReplicas: int32(2),
					State:             datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStateRunning,
				},
				ClusterChecksRunner: &datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStatus{
					Replicas:          int32(3),
					AvailableReplicas: int32(1),
					State:             datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStateRunning,
				},
			},
			wantErr: false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 1.0, "agent", []string{"state:Running"}) {
					return errors.New("Function not called")
				}
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 1.0, "clusteragent", []string{"state:Running"}) {
					return errors.New("Function not called")
				}
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 0.0, "clustercheckrunner", []string{"state:Running"}) {
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
			if err := dd.sendStatusMetrics(tt.status); (err != nil) != tt.wantErr {
				t.Errorf("MetricsForwarder.sendStatusMetrics() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err := tt.wantFunc(f); err != nil {
				t.Errorf("MetricsForwarder.sendStatusMetrics() wantFunc validation error: %v", err)
			}
		})
	}
}

func TestMetricsForwarder_updateCredsIfNeeded(t *testing.T) {
	tests := []struct {
		name     string
		loadFunc func() (*MetricsForwarder, *fakeMetricsForwarder)
		apiKey   string
		appKey   string
		wantErr  bool
		wantFunc func(*MetricsForwarder, *fakeMetricsForwarder) error
	}{
		{
			name: "same creds, no update",
			loadFunc: func() (*MetricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				return &MetricsForwarder{
					delegator: f,
					keysHash:  hashKeys("sameApiKey", "sameAppKey"),
				}, f
			},
			apiKey:  "sameApiKey",
			appKey:  "sameAppKey",
			wantErr: false,
			wantFunc: func(m *MetricsForwarder, f *fakeMetricsForwarder) error {
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
			loadFunc: func() (*MetricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedValidateCreds", "newApiKey", "sameAppKey")
				return &MetricsForwarder{
					delegator: f,
					keysHash:  hashKeys("oldApiKey", "sameAppKey"),
				}, f
			},
			apiKey:  "newApiKey",
			appKey:  "sameAppKey",
			wantErr: false,
			wantFunc: func(m *MetricsForwarder, f *fakeMetricsForwarder) error {
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
			loadFunc: func() (*MetricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedValidateCreds", "sameApiKey", "newAppKey")
				return &MetricsForwarder{
					delegator: f,
					keysHash:  hashKeys("sameApiKey", "oldAppKey"),
				}, f
			},
			apiKey:  "sameApiKey",
			appKey:  "newAppKey",
			wantErr: false,
			wantFunc: func(m *MetricsForwarder, f *fakeMetricsForwarder) error {
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
			loadFunc: func() (*MetricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedValidateCreds", "invalidApiKey", "invalidAppKey")
				return &MetricsForwarder{
					delegator: f,
					keysHash:  hashKeys("oldApiKey", "oldAppKey"),
				}, f
			},
			apiKey:  "invalidApiKey",
			appKey:  "invalidAppKey",
			wantErr: true,
			wantFunc: func(m *MetricsForwarder, f *fakeMetricsForwarder) error {
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
				t.Errorf("MetricsForwarder.updateCredsIfNeeded() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err := tt.wantFunc(dd, f); err != nil {
				t.Errorf("MetricsForwarder.updateCredsIfNeeded() wantFunc validation error: %v", err)
			}
		})
	}
}

func TestReconcileDatadogAgentDeployment_getCredentials(t *testing.T) {
	type fields struct {
		client client.Client
	}
	type args struct {
		dad      *datadoghqv1alpha1.DatadogAgentDeployment
		loadFunc func(c client.Client)
	}
	tests := []struct {
		name       string
		fields     fields
		args       args
		wantAPIKey string
		wantAPPKey string
		wantErr    bool
	}{
		{
			name: "creds found in CR",
			fields: fields{
				client: fake.NewFakeClient(),
			},
			args: args{
				dad: test.NewDefaultedDatadogAgentDeployment("foo", "bar",
					&test.NewDatadogAgentDeploymentOptions{
						Creds: &datadoghqv1alpha1.AgentCredentials{
							APIKey: "foundApiKey",
							AppKey: "foundAppKey",
						}}),
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
				dad: test.NewDefaultedDatadogAgentDeployment(
					"foo",
					"bar",
					&test.NewDatadogAgentDeploymentOptions{
						Creds: &datadoghqv1alpha1.AgentCredentials{
							APIKey: "foundApiKey",
						}}),
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
				dad: test.NewDefaultedDatadogAgentDeployment("foo", "bar",
					&test.NewDatadogAgentDeploymentOptions{
						Creds: &datadoghqv1alpha1.AgentCredentials{
							APIKeyExistingSecret: "datadog-creds",
							AppKeyExistingSecret: "datadog-creds",
						}}),
				loadFunc: func(c client.Client) {
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
					_ = c.Create(context.TODO(), secret)
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
				dad: test.NewDefaultedDatadogAgentDeployment("foo", "bar",
					&test.NewDatadogAgentDeploymentOptions{
						Creds: &datadoghqv1alpha1.AgentCredentials{
							APIKey:               "foundApiKey",
							AppKeyExistingSecret: "datadog-creds",
						}}),
				loadFunc: func(c client.Client) {
					secret := &corev1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "datadog-creds",
							Namespace: "foo",
						},
						Data: map[string][]byte{
							"app_key": []byte("foundAppKey"),
						},
					}
					_ = c.Create(context.TODO(), secret)
				},
			},
			wantAPIKey: "foundApiKey",
			wantAPPKey: "foundAppKey",
			wantErr:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dd := &MetricsForwarder{
				k8sClient: tt.fields.client,
			}
			if tt.args.loadFunc != nil {
				tt.args.loadFunc(dd.k8sClient)
			}
			apiKey, appKey, err := dd.getCredentials(tt.args.dad)
			if (err != nil) != tt.wantErr {
				t.Errorf("MetricsForwarder.getCredentials() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if apiKey != tt.wantAPIKey {
				t.Errorf("MetricsForwarder.getCredentials() apiKey = %v, want %v", apiKey, tt.wantAPIKey)
			}
			if appKey != tt.wantAPPKey {
				t.Errorf("MetricsForwarder.getCredentials() appKey = %v, want %v", appKey, tt.wantAPPKey)
			}
		})
	}
}

func TestMetricsForwarder_setTags(t *testing.T) {
	tests := []struct {
		name string
		dad  *datadoghqv1alpha1.DatadogAgentDeployment
		want []string
	}{
		{
			name: "nil dad",
			dad:  nil,
			want: []string{},
		},
		{
			name: "empty labels",
			dad: test.NewDefaultedDatadogAgentDeployment("foo", "bar",
				&test.NewDatadogAgentDeploymentOptions{}),
			want: []string{},
		},
		{
			name: "with labels",
			dad: test.NewDefaultedDatadogAgentDeployment("foo", "bar",
				&test.NewDatadogAgentDeploymentOptions{
					Labels: map[string]string{
						"firstKey":  "firstValue",
						"secondKey": "secondValue",
					},
				}),
			want: []string{
				"firstKey:firstValue",
				"secondKey:secondValue",
			},
		},
		{
			name: "with clustername",
			dad: test.NewDefaultedDatadogAgentDeployment("foo", "bar",
				&test.NewDatadogAgentDeploymentOptions{
					ClusterName: datadoghqv1alpha1.NewStringPointer("testcluster"),
				}),
			want: []string{
				"cluster_name:testcluster",
			},
		},
		{
			name: "with clustername and labels",
			dad: test.NewDefaultedDatadogAgentDeployment("foo", "bar",
				&test.NewDatadogAgentDeploymentOptions{
					ClusterName: datadoghqv1alpha1.NewStringPointer("testcluster"),
					Labels: map[string]string{
						"firstKey":  "firstValue",
						"secondKey": "secondValue",
					},
				}),
			want: []string{
				"cluster_name:testcluster",
				"firstKey:firstValue",
				"secondKey:secondValue",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dd := &MetricsForwarder{}
			dd.setTags(tt.dad)

			sort.Strings(dd.tags)
			sort.Strings(tt.want)
			if !reflect.DeepEqual(dd.tags, tt.want) {
				t.Errorf("MetricsForwarder.setTags() dd.tags = %v, want %v", dd.tags, tt.want)
			}
		})
	}
}
