// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package upgrade

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	controllercommon "github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	plugincommon "github.com/DataDog/datadog-operator/pkg/plugin/common"
)

func TestGetDDAIs(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v1alpha1.AddToScheme(scheme))

	matchingDDAI := &v1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{
		Name:      "agent",
		Namespace: "test-namespace",
		Labels:    map[string]string{apicommon.DatadogAgentNameLabelKey: "agent"},
	}}
	matchingProfileDDAI := &v1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{
		Name:      "agent-profile-a",
		Namespace: "test-namespace",
		Labels:    map[string]string{apicommon.DatadogAgentNameLabelKey: "agent"},
	}}
	otherDDAI := &v1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{
		Name:      "other-agent",
		Namespace: "test-namespace",
		Labels:    map[string]string{apicommon.DatadogAgentNameLabelKey: "other"},
	}}
	otherNamespaceDDAI := &v1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{
		Name:      "agent",
		Namespace: "other-namespace",
		Labels:    map[string]string{apicommon.DatadogAgentNameLabelKey: "agent"},
	}}

	o := &Options{
		Options: plugincommon.Options{
			Client: fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(matchingDDAI, matchingProfileDDAI, otherDDAI, otherNamespaceDDAI).
				Build(),
			UserNamespace: "test-namespace",
		},
		datadogAgentName: "agent",
	}

	ddais, err := o.getDDAIs(context.Background())
	require.NoError(t, err)
	require.Len(t, ddais, 2)
	assert.ElementsMatch(t, []string{matchingDDAI.Name, matchingProfileDDAI.Name}, []string{ddais[0].Name, ddais[1].Name})
}

func TestIsReconcileError(t *testing.T) {
	tests := []struct {
		name       string
		conditions []metav1.Condition
		wantError  string
	}{
		{
			name: "DDAI error propagated to DDA",
			conditions: []metav1.Condition{{
				Type:    controllercommon.DatadogAgentInternalReconcileErrorConditionType,
				Status:  metav1.ConditionTrue,
				Message: "agent: forbidden",
			}},
			wantError: "datadogAgentInternal reconciliation error message: agent: forbidden",
		},
		{
			name: "DDAI direct reconcile error",
			conditions: []metav1.Condition{{
				Type:    controllercommon.DatadogAgentReconcileErrorConditionType,
				Status:  metav1.ConditionTrue,
				Message: "forbidden",
			}},
			wantError: "datadogAgent reconciliation error message: forbidden",
		},
		{
			name: "resolved DDAI reconcile error",
			conditions: []metav1.Condition{{
				Type:    controllercommon.DatadogAgentReconcileErrorConditionType,
				Status:  metav1.ConditionFalse,
				Message: "reconcile ok",
			}},
		},
		{
			name: "agent component error",
			conditions: []metav1.Condition{{
				Type:    controllercommon.AgentReconcileConditionType,
				Status:  metav1.ConditionFalse,
				Message: "daemonset failed",
			}},
			wantError: "agent reconciliation error message: daemonset failed",
		},
		{
			name: "cluster agent component error",
			conditions: []metav1.Condition{{
				Type:    controllercommon.ClusterAgentReconcileConditionType,
				Status:  metav1.ConditionFalse,
				Message: "deployment failed",
			}},
			wantError: "cluster Agent reconciliation error message: deployment failed",
		},
		{
			name: "cluster checks runner component error",
			conditions: []metav1.Condition{{
				Type:    controllercommon.ClusterChecksRunnerReconcileConditionType,
				Status:  metav1.ConditionFalse,
				Message: "deployment failed",
			}},
			wantError: "cluster Check Runner reconciliation error message: deployment failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := isReconcileError(tt.conditions)
			if tt.wantError == "" {
				assert.NoError(t, err)
				return
			}
			require.EqualError(t, err, tt.wantError)
		})
	}
}

func TestStartupFlagsDoNotRequireManualRolloutConfiguration(t *testing.T) {
	cmd := NewCmdUpgrade(genericclioptions.IOStreams{Out: io.Discard, ErrOut: io.Discard})
	assert.False(t, NewOptions(genericclioptions.IOStreams{}).collectStartupTimings)
	assert.Nil(t, cmd.Flags().Lookup("startup-created-after"))
	assert.Nil(t, cmd.Flags().Lookup("startup-observation-timeout"))
}

type upgradeErrorClient struct {
	client.Client
	getError  error
	listError error
}

func (c *upgradeErrorClient) Get(ctx context.Context, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
	if _, ok := obj.(*v2alpha1.DatadogAgent); ok && c.getError != nil {
		return c.getError
	}
	return c.Client.Get(ctx, key, obj, opts...)
}
func (c *upgradeErrorClient) List(ctx context.Context, obj client.ObjectList, opts ...client.ListOption) error {
	if _, ok := obj.(*v1alpha1.DatadogAgentInternalList); ok && c.listError != nil {
		return c.listError
	}
	return c.Client.List(ctx, obj, opts...)
}

func TestRunPreservesUpgradeResultWithTelemetry(t *testing.T) {
	sentinel := errors.New("API unavailable")
	for _, telemetry := range []bool{false, true} {
		for _, scenario := range []string{"threshold success", "DDA absent", "still upgrading", "reconcile error", "get error", "list error"} {
			t.Run(fmt.Sprintf("telemetry=%t/%s", telemetry, scenario), func(t *testing.T) {
				dda := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "test"}, Status: v2alpha1.DatadogAgentStatus{Agent: &v2alpha1.DaemonSetStatus{Current: 37, UpToDate: 27}}}
				if scenario == "still upgrading" {
					dda.Status.Agent.UpToDate = 0
				}
				if scenario == "reconcile error" {
					dda.Status.Conditions = []metav1.Condition{{Type: controllercommon.AgentReconcileConditionType, Status: metav1.ConditionFalse, Message: "failed"}}
				}
				objects := []client.Object{dda}
				if scenario == "DDA absent" {
					objects = nil
				}
				c, _ := testCollector(t, objects...)
				o := c.options
				o.Out, o.ErrOut = io.Discard, io.Discard
				o.collectStartupTimings = telemetry
				o.checkPeriod = time.Millisecond
				o.checkTimeout = 40 * time.Millisecond
				if !telemetry && scenario == "threshold success" {
					o.checkTimeout = time.Second
				}
				o.agentCompletionPct = .95
				o.agentCompletionMin = 10
				o.dcaMinUpToDate = 1
				o.clcMinUpToDate = 2
				switch scenario {
				case "get error":
					o.Client = &upgradeErrorClient{Client: o.Client, getError: sentinel}
				case "list error":
					o.Client = &upgradeErrorClient{Client: o.Client, listError: sentinel}
				}
				started := time.Now()
				err := o.Run()
				switch scenario {
				case "get error", "list error":
					require.ErrorIs(t, err, sentinel)
				case "still upgrading", "reconcile error":
					require.ErrorIs(t, err, context.DeadlineExceeded)
				default:
					require.NoError(t, err)
				}
				if !telemetry && scenario == "threshold success" {
					assert.Less(t, time.Since(started), 500*time.Millisecond)
				}
			})
		}
	}
}

type failedStartupWriter struct{}

func (failedStartupWriter) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }
func TestRunTelemetryWriteFailurePreservesSuccess(t *testing.T) {
	c, _ := testCollector(t)
	o := c.options
	o.IOStreams = genericclioptions.IOStreams{Out: io.Discard, ErrOut: failedStartupWriter{}}
	o.collectStartupTimings = true
	o.checkPeriod = time.Millisecond
	o.checkTimeout = 10 * time.Millisecond
	require.NoError(t, o.Run())
}
