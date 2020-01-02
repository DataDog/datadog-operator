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
		namespacedName: nsn,
		delegator:      fmf,
	}
	mf.initGlobalTags()

	tests := []struct {
		name     string
		loadFunc func() (*metricsForwarder, *fakeMetricsForwarder)
		status   *datadoghqv1alpha1.DatadogAgentDeploymentStatus
		wantErr  bool
		wantFunc func(*fakeMetricsForwarder) error
	}{
		{
			name: "empty status",
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				return mf, fmf
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
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				return mf, fmf
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
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedSendDeploymentMetric", 1.0, "agent", []string{"cr_namespace:foo", "cr_name:bar", "state:Running"})
				mf.delegator = f
				return mf, f
			},
			status: &datadoghqv1alpha1.DatadogAgentDeploymentStatus{
				Agent: &datadoghqv1alpha1.DatadogAgentDeploymentAgentStatus{
					Desired:   int32(1337),
					Available: int32(1337),
					State:     string(datadoghqv1alpha1.DatadogAgentDeploymentStateRunning),
				},
			},
			wantErr: false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 1.0, "agent", []string{"cr_namespace:foo", "cr_name:bar", "state:Running"}) {
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
				f.On("delegatedSendDeploymentMetric", 1.0, "agent", []string{"cr_namespace:foo", "cr_name:bar", "cluster_name:testcluster", "state:Running"})
				mf.delegator = f
				mf.tags = []string{"cluster_name:testcluster"}
				return mf, f
			},
			status: &datadoghqv1alpha1.DatadogAgentDeploymentStatus{
				Agent: &datadoghqv1alpha1.DatadogAgentDeploymentAgentStatus{
					Desired:   int32(1337),
					Available: int32(1337),
					State:     string(datadoghqv1alpha1.DatadogAgentDeploymentStateRunning),
				},
			},
			wantErr: false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 1.0, "agent", []string{"cr_namespace:foo", "cr_name:bar", "cluster_name:testcluster", "state:Running"}) {
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
				f.On("delegatedSendDeploymentMetric", 0.0, "agent", []string{"cr_namespace:foo", "cr_name:bar", "state:Failed"})
				mf.delegator = f
				mf.tags = []string{}
				return mf, f
			},
			status: &datadoghqv1alpha1.DatadogAgentDeploymentStatus{
				Agent: &datadoghqv1alpha1.DatadogAgentDeploymentAgentStatus{
					Desired:   int32(1337),
					Available: int32(1336),
					State:     string(datadoghqv1alpha1.DatadogAgentDeploymentStateFailed),
				},
			},
			wantErr: false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 0.0, "agent", []string{"cr_namespace:foo", "cr_name:bar", "state:Failed"}) {
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
				f.On("delegatedSendDeploymentMetric", 1.0, "agent", []string{"cr_namespace:foo", "cr_name:bar", "state:Running"})
				f.On("delegatedSendDeploymentMetric", 1.0, "clusteragent", []string{"cr_namespace:foo", "cr_name:bar", "state:Running"})
				f.On("delegatedSendDeploymentMetric", 1.0, "clustercheckrunner", []string{"cr_namespace:foo", "cr_name:bar", "state:Running"})
				mf.delegator = f
				return mf, f
			},
			status: &datadoghqv1alpha1.DatadogAgentDeploymentStatus{
				Agent: &datadoghqv1alpha1.DatadogAgentDeploymentAgentStatus{
					Desired:   int32(1337),
					Available: int32(1337),
					State:     string(datadoghqv1alpha1.DatadogAgentDeploymentStateRunning),
				},
				ClusterAgent: &datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStatus{
					Replicas:          int32(2),
					AvailableReplicas: int32(2),
					State:             string(datadoghqv1alpha1.DatadogAgentDeploymentStateRunning),
				},
				ClusterChecksRunner: &datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStatus{
					Replicas:          int32(3),
					AvailableReplicas: int32(3),
					State:             string(datadoghqv1alpha1.DatadogAgentDeploymentStateRunning),
				},
			},
			wantErr: false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 1.0, "agent", []string{"cr_namespace:foo", "cr_name:bar", "state:Running"}) {
					return errors.New("Function not called")
				}
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 1.0, "clusteragent", []string{"cr_namespace:foo", "cr_name:bar", "state:Running"}) {
					return errors.New("Function not called")
				}
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 1.0, "clustercheckrunner", []string{"cr_namespace:foo", "cr_name:bar", "state:Running"}) {
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
				f.On("delegatedSendDeploymentMetric", 1.0, "agent", []string{"cr_namespace:foo", "cr_name:bar", "state:Running"})
				f.On("delegatedSendDeploymentMetric", 0.0, "clusteragent", []string{"cr_namespace:foo", "cr_name:bar", "state:Progressing"})
				mf.delegator = f
				return mf, f
			},
			status: &datadoghqv1alpha1.DatadogAgentDeploymentStatus{
				Agent: &datadoghqv1alpha1.DatadogAgentDeploymentAgentStatus{
					Desired:   int32(1337),
					Available: int32(1337),
					State:     string(datadoghqv1alpha1.DatadogAgentDeploymentStateRunning),
				},
				ClusterAgent: &datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStatus{
					Replicas:          int32(2),
					AvailableReplicas: int32(0),
					State:             string(datadoghqv1alpha1.DatadogAgentDeploymentStateProgressing),
				},
			},
			wantErr: false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 1.0, "agent", []string{"cr_namespace:foo", "cr_name:bar", "state:Running"}) {
					return errors.New("Function not called")
				}
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 0.0, "clusteragent", []string{"cr_namespace:foo", "cr_name:bar", "state:Progressing"}) {
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
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedSendDeploymentMetric", 1.0, "agent", []string{"cr_namespace:foo", "cr_name:bar", "state:Running"})
				f.On("delegatedSendDeploymentMetric", 1.0, "clusteragent", []string{"cr_namespace:foo", "cr_name:bar", "state:Running"})
				f.On("delegatedSendDeploymentMetric", 0.0, "clustercheckrunner", []string{"cr_namespace:foo", "cr_name:bar", "state:Running"})
				mf.delegator = f
				return mf, f
			},
			status: &datadoghqv1alpha1.DatadogAgentDeploymentStatus{
				Agent: &datadoghqv1alpha1.DatadogAgentDeploymentAgentStatus{
					Desired:   int32(1337),
					Available: int32(1337),
					State:     string(datadoghqv1alpha1.DatadogAgentDeploymentStateRunning),
				},
				ClusterAgent: &datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStatus{
					Replicas:          int32(2),
					AvailableReplicas: int32(2),
					State:             string(datadoghqv1alpha1.DatadogAgentDeploymentStateRunning),
				},
				ClusterChecksRunner: &datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStatus{
					Replicas:          int32(3),
					AvailableReplicas: int32(1),
					State:             string(datadoghqv1alpha1.DatadogAgentDeploymentStateRunning),
				},
			},
			wantErr: false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 1.0, "agent", []string{"cr_namespace:foo", "cr_name:bar", "state:Running"}) {
					return errors.New("Function not called")
				}
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 1.0, "clusteragent", []string{"cr_namespace:foo", "cr_name:bar", "state:Running"}) {
					return errors.New("Function not called")
				}
				if !f.AssertCalled(t, "delegatedSendDeploymentMetric", 0.0, "clustercheckrunner", []string{"cr_namespace:foo", "cr_name:bar", "state:Running"}) {
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
			dd := &metricsForwarder{
				k8sClient: tt.fields.client,
			}
			if tt.args.loadFunc != nil {
				tt.args.loadFunc(dd.k8sClient)
			}
			apiKey, appKey, err := dd.getCredentials(tt.args.dad)
			if (err != nil) != tt.wantErr {
				t.Errorf("metricsForwarder.getCredentials() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if apiKey != tt.wantAPIKey {
				t.Errorf("metricsForwarder.getCredentials() apiKey = %v, want %v", apiKey, tt.wantAPIKey)
			}
			if appKey != tt.wantAPPKey {
				t.Errorf("metricsForwarder.getCredentials() appKey = %v, want %v", appKey, tt.wantAPPKey)
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
			dd := &metricsForwarder{}
			dd.updateTags(tt.dad)

			sort.Strings(dd.tags)
			sort.Strings(tt.want)
			if !reflect.DeepEqual(dd.tags, tt.want) {
				t.Errorf("metricsForwarder.setTags() dd.tags = %v, want %v", dd.tags, tt.want)
			}
		})
	}
}

func Test_metricsForwarder_processReconcileError(t *testing.T) {
	nsn := types.NamespacedName{
		Namespace: "foo",
		Name:      "bar",
	}
	mf := &metricsForwarder{
		namespacedName: nsn,
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
				f.On("delegatedSendReconcileMetric", 0.0, []string{"cr_namespace:foo", "cr_name:bar", "reconcile_err:err_msg"})
				mf.delegator = f
				mf.lastReconcileErr = errInitValue
				return mf, f
			},
			err:     errors.New("err_msg"),
			wantErr: false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertCalled(t, "delegatedSendReconcileMetric", 0.0, []string{"cr_namespace:foo", "cr_name:bar", "reconcile_err:err_msg"}) {
					return errors.New("Function not called")
				}
				if !f.AssertNumberOfCalls(t, "delegatedSendReconcileMetric", 1) {
					return errors.New("Wrong number of calls")
				}
				return nil
			},
		},
		{
			name: "last error init value, new auth error => send unsucess metric",
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedSendReconcileMetric", 0.0, []string{"cr_namespace:foo", "cr_name:bar", "reconcile_err:Unauthorized"})
				mf.delegator = f
				mf.lastReconcileErr = errInitValue
				return mf, f
			},
			err:     apierrors.NewUnauthorized("Auth error"),
			wantErr: false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertCalled(t, "delegatedSendReconcileMetric", 0.0, []string{"cr_namespace:foo", "cr_name:bar", "reconcile_err:Unauthorized"}) {
					return errors.New("Function not called")
				}
				if !f.AssertNumberOfCalls(t, "delegatedSendReconcileMetric", 1) {
					return errors.New("Wrong number of calls")
				}
				return nil
			},
		},
		{
			name: "last error init value, new error is nil => send success metric",
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedSendReconcileMetric", 1.0, []string{"cr_namespace:foo", "cr_name:bar", "reconcile_err:null"})
				mf.delegator = f
				mf.lastReconcileErr = errInitValue
				return mf, f
			},
			err:     nil,
			wantErr: false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertCalled(t, "delegatedSendReconcileMetric", 1.0, []string{"cr_namespace:foo", "cr_name:bar", "reconcile_err:null"}) {
					return errors.New("Function not called")
				}
				if !f.AssertNumberOfCalls(t, "delegatedSendReconcileMetric", 1) {
					return errors.New("Wrong number of calls")
				}
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
			want1:        []string{"gtagkey:gtagvalue", "tagkey:tagvalue", "reconcile_err:null"},
			wantErr:      false,
		},
		{
			name:         "lastReconcileErr updated and not nil",
			reconcileErr: apierrors.NewUnauthorized("Auth error"),
			want:         0.0,
			want1:        []string{"gtagkey:gtagvalue", "tagkey:tagvalue", "reconcile_err:Unauthorized"},
			wantErr:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mf := &metricsForwarder{
				globalTags: defaultGlobalTags,
				tags:       defaultTags,
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
