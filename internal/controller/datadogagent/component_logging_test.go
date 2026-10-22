// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
)

// The cleanup path logs through context, letting the registry test check that
// each component inherits its own fields without leaking them to its siblings.
type loggingCleanupComponent struct {
	ComponentReconciler
	name v2alpha1.ComponentName
}

func (c loggingCleanupComponent) Name() v2alpha1.ComponentName { return c.name }
func (c loggingCleanupComponent) IsEnabled(feature.RequiredComponents, map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride) (bool, bool) {
	return false, false
}
func (c loggingCleanupComponent) GetNewDeploymentFunc() func(metav1.Object, *v2alpha1.DatadogAgentSpec) *appsv1.Deployment {
	return func(dda metav1.Object, _ *v2alpha1.DatadogAgentSpec) *appsv1.Deployment {
		return &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: string(c.name), Namespace: dda.GetNamespace()}}
	}
}
func (c loggingCleanupComponent) CleanupDependencies(ctx context.Context, _ *v2alpha1.DatadogAgent, _ feature.ResourceManagers) (reconcile.Result, error) {
	ctrl.LoggerFrom(ctx).Info("Cleaning component dependencies")
	return reconcile.Result{}, nil
}
func (c loggingCleanupComponent) GetConditionType() string                          { return string(c.name) }
func (c loggingCleanupComponent) DeleteStatus(*v2alpha1.DatadogAgentStatus, string) {}

func TestComponentLoggerContextIsolation(t *testing.T) {
	ctx, checkLogs := tracedLogContext(t)
	scheme := testScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	r := &Reconciler{client: fake.NewClientBuilder().WithScheme(scheme).Build()}
	registry := NewComponentRegistry(r)
	registry.Register(loggingCleanupComponent{name: v2alpha1.ClusterAgentComponentName})
	registry.Register(loggingCleanupComponent{name: v2alpha1.ClusterChecksRunnerComponentName})
	_, err := registry.ReconcileComponents(ctx, &ReconcileComponentParams{
		DDA:    &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: "dda", Namespace: "ns"}},
		Status: &v2alpha1.DatadogAgentStatus{},
	})
	require.NoError(t, err)
	ctrl.LoggerFrom(ctx).Info("After components")
	logs := checkLogs()
	require.Len(t, logs, 3)
	assert.Contains(t, logs[0], `"component"="clusterAgent"`)
	assert.Contains(t, logs[1], `"component"="clusterChecksRunner"`)
	assert.Equal(t, 1, strings.Count(logs[1], `"component"=`))
	assert.NotContains(t, logs[2], `"component"=`)
}

func TestDeploymentLoggerPropagatesToLookup(t *testing.T) {
	ctx, checkLogs := tracedLogContext(t)
	scheme := testScheme()
	require.NoError(t, appsv1.AddToScheme(scheme))
	r := &Reconciler{scheme: scheme, client: fake.NewClientBuilder().WithScheme(scheme).Build(), recorder: record.NewFakeRecorder(10)}
	dda := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: "dda", Namespace: "ns"}}
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "deployment", Namespace: "ns"}}
	_, err := r.createOrUpdateDeployment(ctx, dda, deployment, &v2alpha1.DatadogAgentStatus{}, func(*appsv1.Deployment, *v2alpha1.DatadogAgentStatus, metav1.Time, metav1.ConditionStatus, string, string) {
	})
	require.NoError(t, err)
	logs := checkLogs()
	require.NotEmpty(t, logs)
	// The first log comes from the nested getCurrentDeployment helper.
	assert.Contains(t, logs[0], `"msg"="deployment is not found"`)
	assert.Contains(t, logs[0], `"deployment.Namespace"="ns"`)
	assert.Contains(t, logs[0], `"deployment.Name"="deployment"`)
}
