// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadog

import (
	"errors"
	"reflect"
	"testing"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

func (c *fakeMetricsForwarder) delegatedSendMonitorMetric(metricValue float64, component string, tags []string) error {
	c.Called(metricValue, component, tags)
	return nil
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
		status   *datadoghqv1alpha1.DatadogAgentStatus
		wantErr  bool
		wantFunc func(*fakeMetricsForwarder) error
	}{
		{
			name: "empty status",
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				return mf, fmf
			},
			status:  &datadoghqv1alpha1.DatadogAgentStatus{},
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
			status: &datadoghqv1alpha1.DatadogAgentStatus{
				Agent: &datadoghqv1alpha1.DaemonSetStatus{
					Desired:   int32(1337),
					Available: int32(1337),
					State:     string(datadoghqv1alpha1.DatadogAgentStateRunning),
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
			status: &datadoghqv1alpha1.DatadogAgentStatus{
				Agent: &datadoghqv1alpha1.DaemonSetStatus{
					Desired:   int32(1337),
					Available: int32(1337),
					State:     string(datadoghqv1alpha1.DatadogAgentStateRunning),
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
			status: &datadoghqv1alpha1.DatadogAgentStatus{
				Agent: &datadoghqv1alpha1.DaemonSetStatus{
					Desired:   int32(1337),
					Available: int32(1336),
					State:     string(datadoghqv1alpha1.DatadogAgentStateFailed),
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
			status: &datadoghqv1alpha1.DatadogAgentStatus{
				Agent: &datadoghqv1alpha1.DaemonSetStatus{
					Desired:   int32(1337),
					Available: int32(1337),
					State:     string(datadoghqv1alpha1.DatadogAgentStateRunning),
				},
				ClusterAgent: &datadoghqv1alpha1.DeploymentStatus{
					Replicas:          int32(2),
					AvailableReplicas: int32(2),
					State:             string(datadoghqv1alpha1.DatadogAgentStateRunning),
				},
				ClusterChecksRunner: &datadoghqv1alpha1.DeploymentStatus{
					Replicas:          int32(3),
					AvailableReplicas: int32(3),
					State:             string(datadoghqv1alpha1.DatadogAgentStateRunning),
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
			status: &datadoghqv1alpha1.DatadogAgentStatus{
				Agent: &datadoghqv1alpha1.DaemonSetStatus{
					Desired:   int32(1337),
					Available: int32(1337),
					State:     string(datadoghqv1alpha1.DatadogAgentStateRunning),
				},
				ClusterAgent: &datadoghqv1alpha1.DeploymentStatus{
					Replicas:          int32(2),
					AvailableReplicas: int32(0),
					State:             string(datadoghqv1alpha1.DatadogAgentStateProgressing),
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
			status: &datadoghqv1alpha1.DatadogAgentStatus{
				Agent: &datadoghqv1alpha1.DaemonSetStatus{
					Desired:   int32(1337),
					Available: int32(1337),
					State:     string(datadoghqv1alpha1.DatadogAgentStateRunning),
				},
				ClusterAgent: &datadoghqv1alpha1.DeploymentStatus{
					Replicas:          int32(2),
					AvailableReplicas: int32(2),
					State:             string(datadoghqv1alpha1.DatadogAgentStateRunning),
				},
				ClusterChecksRunner: &datadoghqv1alpha1.DeploymentStatus{
					Replicas:          int32(3),
					AvailableReplicas: int32(1),
					State:             string(datadoghqv1alpha1.DatadogAgentStateRunning),
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

func TestMetricsForwarder_sendMonitorStatus(t *testing.T) {
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
		status   *datadoghqv1alpha1.DatadogMonitorStatus
		wantErr  bool
		wantFunc func(*fakeMetricsForwarder) error
	}{
		{
			name: "empty status",
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				return mf, fmf
			},
			status:  &datadoghqv1alpha1.DatadogMonitorStatus{},
			wantErr: false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertNumberOfCalls(t, "delegatedSendMonitorMetric", 0) {
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
				if !f.AssertNumberOfCalls(t, "delegatedSendMonitorMetric", 0) {
					return errors.New("Wrong number of calls")
				}
				return nil
			},
		},
		{
			name: "only active, available ok status",
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedSendMonitorMetric", 1.0, "active", []string{"monitor_id:12345", "monitor_state:OK", "monitor_sync_status:OK"})
				mf.delegator = f
				return mf, f
			},
			status: &datadoghqv1alpha1.DatadogMonitorStatus{
				ID:           12345,
				MonitorState: datadoghqv1alpha1.DatadogMonitorStateOK,
				SyncStatus:   datadoghqv1alpha1.SyncStatusOK,
				Conditions: []datadoghqv1alpha1.DatadogMonitorCondition{
					{
						Type:   datadoghqv1alpha1.DatadogMonitorConditionTypeActive,
						Status: corev1.ConditionTrue,
					},
				},
			},
			wantErr: false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertCalled(t, "delegatedSendMonitorMetric", 1.0, "active", []string{"monitor_id:12345", "monitor_state:OK", "monitor_sync_status:OK"}) {
					return errors.New("Function not called")
				}
				if !f.AssertNumberOfCalls(t, "delegatedSendMonitorMetric", 1) {
					return errors.New("Wrong number of calls")
				}
				return nil
			},
		},
		{
			name: "only error, error status",
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedSendMonitorMetric", 1.0, "error", []string{"monitor_sync_status:error validating monitor"})
				mf.delegator = f
				return mf, f
			},
			status: &datadoghqv1alpha1.DatadogMonitorStatus{
				SyncStatus: datadoghqv1alpha1.SyncStatusValidateError,
				Conditions: []datadoghqv1alpha1.DatadogMonitorCondition{
					{
						Type:   datadoghqv1alpha1.DatadogMonitorConditionTypeError,
						Status: corev1.ConditionTrue,
					},
				},
			},
			wantErr: false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertCalled(t, "delegatedSendMonitorMetric", 1.0, "error", []string{"monitor_sync_status:error validating monitor"}) {
					return errors.New("Function not called")
				}
				if !f.AssertNumberOfCalls(t, "delegatedSendMonitorMetric", 1) {
					return errors.New("Wrong number of calls")
				}
				return nil
			},
		},
		{
			name: "active, create, error conditions available",
			loadFunc: func() (*metricsForwarder, *fakeMetricsForwarder) {
				f := &fakeMetricsForwarder{}
				f.On("delegatedSendMonitorMetric", 1.0, "active", []string{"monitor_id:12345", "monitor_state:OK", "monitor_sync_status:OK"})
				f.On("delegatedSendMonitorMetric", 1.0, "created", []string{"monitor_id:12345", "monitor_state:OK", "monitor_sync_status:OK"})
				f.On("delegatedSendMonitorMetric", 0.0, "error", []string{"monitor_id:12345", "monitor_state:OK", "monitor_sync_status:OK"})
				mf.delegator = f
				return mf, f
			},
			status: &datadoghqv1alpha1.DatadogMonitorStatus{
				ID:           12345,
				MonitorState: datadoghqv1alpha1.DatadogMonitorStateOK,
				SyncStatus:   datadoghqv1alpha1.SyncStatusOK,
				Conditions: []datadoghqv1alpha1.DatadogMonitorCondition{
					{
						Type:   datadoghqv1alpha1.DatadogMonitorConditionTypeActive,
						Status: corev1.ConditionTrue,
					},
					{
						Type:   datadoghqv1alpha1.DatadogMonitorConditionTypeCreated,
						Status: corev1.ConditionTrue,
					},
					{
						Type:   datadoghqv1alpha1.DatadogMonitorConditionTypeError,
						Status: corev1.ConditionFalse,
					},
				},
			},
			wantErr: false,
			wantFunc: func(f *fakeMetricsForwarder) error {
				if !f.AssertCalled(t, "delegatedSendMonitorMetric", 1.0, "active", []string{"monitor_id:12345", "monitor_state:OK", "monitor_sync_status:OK"}) {
					return errors.New("Function not called")
				}
				if !f.AssertCalled(t, "delegatedSendMonitorMetric", 1.0, "created", []string{"monitor_id:12345", "monitor_state:OK", "monitor_sync_status:OK"}) {
					return errors.New("Function not called")
				}
				if !f.AssertCalled(t, "delegatedSendMonitorMetric", 0.0, "error", []string{"monitor_id:12345", "monitor_state:OK", "monitor_sync_status:OK"}) {
					return errors.New("Function not called")
				}
				if !f.AssertNumberOfCalls(t, "delegatedSendMonitorMetric", 3) {
					return errors.New("Wrong number of calls")
				}
				return nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mf, fakeForwarder := tt.loadFunc()
			if err := mf.sendMonitorStatus(tt.status); (err != nil) != tt.wantErr {
				t.Errorf("metricsForwarder.sendMonitorStatus() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err := tt.wantFunc(fakeForwarder); err != nil {
				t.Errorf("metricsForwarder.sendMonitorStatus() wantFunc validation error: %v", err)
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
				f.On("delegatedSendReconcileMetric", 0.0, []string{"cr_namespace:foo", "cr_name:bar", "reconcile_err:err_msg"}).Once()
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
				f.On("delegatedSendReconcileMetric", 0.0, []string{"cr_namespace:foo", "cr_name:bar", "reconcile_err:Unauthorized"}).Once()
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
				f.On("delegatedSendReconcileMetric", 1.0, []string{"cr_namespace:foo", "cr_name:bar", "reconcile_err:null"}).Once()
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
