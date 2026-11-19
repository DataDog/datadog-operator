// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build integration

package controller

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlconfig "sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/testutils"
)

// Stop DDA/DDAI at the finalizer write so this test exercises their real
// reconcile entry points and controller-runtime's error logger without creating
// Agent workloads. API-backed controllers stop at missing credentials instead.
type loggingUpdateErrorClient struct {
	client.Client
}

func (c *loggingUpdateErrorClient) Update(context.Context, client.Object, ...client.UpdateOption) error {
	return errors.New("injected finalizer update failure")
}

func TestControllerReconcileLogFields(t *testing.T) {
	t.Setenv(constants.DDAPIKey, "")
	t.Setenv(constants.DDAppKey, "")
	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases", "v1")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := testEnv.Start()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, testEnv.Stop()) })

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, v2alpha1.AddToScheme(scheme))
	// Include controller-runtime's per-request debug logs, including successful
	// reconciles for the Profile controller, which currently only logs and exits.
	core, logs := observer.New(zapcore.Level(-5))
	logger := zapr.NewLogger(zap.New(core))
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 scheme,
		Logger:                 logger,
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
		Controller:             ctrlconfig.Controller{SkipNameValidation: ptr.To(true)},
	})
	require.NoError(t, err)
	cli := &loggingUpdateErrorClient{Client: mgr.GetClient()}
	creds := config.NewCredentialManager(cli)
	recorder := mgr.GetEventRecorderFor("logging-test")
	ddaReconciler := &DatadogAgentReconciler{Client: cli, Scheme: scheme, Recorder: recorder}
	ddaiReconciler := &DatadogAgentInternalReconciler{Client: cli, Scheme: scheme, Recorder: recorder}
	monitorReconciler := &DatadogMonitorReconciler{Client: cli, Scheme: scheme, Recorder: recorder, CredsManager: creds}
	dashboardReconciler := &DatadogDashboardReconciler{Client: cli, Scheme: scheme, Recorder: recorder, CredsManager: creds}
	sloReconciler := &DatadogSLOReconciler{Client: cli, Scheme: scheme, Recorder: recorder, CredsManager: creds}
	profileReconciler := &DatadogAgentProfileReconciler{Client: cli, Scheme: scheme, Recorder: recorder}
	genericReconciler := &DatadogGenericResourceReconciler{Client: cli, Scheme: scheme, Recorder: recorder, CredsManager: creds}
	require.NoError(t, ddaReconciler.SetupWithManager(mgr, nil))
	require.NoError(t, ddaiReconciler.SetupWithManager(mgr, nil))
	require.NoError(t, monitorReconciler.SetupWithManager(mgr, nil))
	require.NoError(t, dashboardReconciler.SetupWithManager(mgr))
	require.NoError(t, sloReconciler.SetupWithManager(mgr))
	require.NoError(t, profileReconciler.SetupWithManager(mgr))
	require.NoError(t, genericReconciler.SetupWithManager(mgr))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- mgr.Start(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(10 * time.Second):
			t.Error("manager did not stop")
		}
	})
	apiClient, err := client.New(cfg, client.Options{Scheme: scheme})
	require.NoError(t, err)
	dda := testutils.NewInitializedDatadogAgentBuilder("default", "logging-dda").Build()
	dda.Finalizers = nil
	ddai := testutils.NewInitializedDatadogAgentInternalBuilder("default", "logging-ddai").Build()
	ddai.Finalizers = nil
	newResource := func(kind string, spec map[string]any) client.Object {
		return &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": v1alpha1.GroupVersion.String(), "kind": kind,
			"metadata": map[string]any{"namespace": "default", "name": "logging-" + strings.ToLower(kind)},
			"spec":     spec,
		}}
	}
	tests := []struct {
		kind    string
		object  client.Object
		message string
	}{
		{agentControllerName, dda, "Reconciling DatadogAgent"},
		{agentInternalControllerName, ddai, "Reconciling DatadogAgentInternal"},
		{monitorControllerName, newResource(monitorControllerName, map[string]any{"name": "logging", "message": "logging test", "type": "query alert", "query": "avg(last_5m):avg:system.cpu.user{*} > 90"}), "Reconciling DatadogMonitor"},
		{dashboardControllerName, newResource(dashboardControllerName, map[string]any{"title": "logging", "layoutType": "ordered"}), "Reconciling Datadog Dashboard"},
		{sloControllerName, newResource(sloControllerName, map[string]any{"name": "logging", "type": "monitor", "timeframe": "7d", "targetThreshold": "99"}), "Reconciling Datadog SLO"},
		{profileControllerName, &v1alpha1.DatadogAgentProfile{ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "logging-profile"}}, "Reconciling DatadogAgentProfile"},
		{genericResourceControllerName, newResource(genericResourceControllerName, map[string]any{"type": "monitor", "jsonSpec": "{}"}), "Reconciling DatadogGenericResource"},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			require.NoError(t, apiClient.Create(ctx, tt.object))
			runtimeMessage := "Reconciler error"
			if tt.kind == profileControllerName {
				runtimeMessage = "Reconcile successful"
			}
			require.Eventually(t, func() bool {
				return logs.FilterMessage(tt.message).Len() > 0 &&
					logs.FilterMessage(runtimeMessage).FilterField(zap.String("controllerKind", tt.kind)).Len() > 0
			}, 30*time.Second, 100*time.Millisecond, "missing reconcile logs: %v", logs.FilterLevelExact(zapcore.ErrorLevel).All())

			entry := logs.FilterMessage(tt.message).All()[0]
			runtimeEntries := logs.FilterMessage(runtimeMessage).FilterField(zap.String("controllerKind", tt.kind)).All()
			assert.Equal(t, entry.ContextMap()["reconcileID"], runtimeEntries[0].ContextMap()["reconcileID"])
			for _, entry := range []observer.LoggedEntry{entry, runtimeEntries[0]} {
				fields := entry.ContextMap()
				assert.Equal(t, "controllers."+tt.kind, entry.LoggerName)
				assert.Equal(t, "default", fields["namespace"])
				assert.Equal(t, tt.object.GetName(), fields["name"])
				assert.Equal(t, strings.ToLower(tt.kind), fields["controller"])
				assert.Equal(t, "datadoghq.com", fields["controllerGroup"])
				assert.Equal(t, tt.kind, fields["controllerKind"])
				require.NotEmpty(t, fields["reconcileID"])
				for _, key := range []string{"namespace", "name", "controller", "controllerGroup", "controllerKind", "reconcileID"} {
					count := 0
					for _, field := range entry.Context {
						if field.Key == key {
							count++
						}
					}
					assert.Equal(t, 1, count, "%s must appear exactly once", key)
				}
				assert.NotContains(t, fields, tt.kind)
				assert.NotContains(t, fields, strings.ToLower(tt.kind))
				assert.NotContains(t, fields, "Monitor Namespace")
				assert.NotContains(t, fields, "Monitor Name")
			}
			// Startup logs use the same name without pretending to identify a resource.
			startup := logs.FilterMessage("Starting Controller").FilterField(zap.String("controllerKind", tt.kind)).All()
			require.NotEmpty(t, startup)
			assert.Equal(t, "controllers."+tt.kind, startup[0].LoggerName)
			assert.NotContains(t, startup[0].ContextMap(), "namespace")
			assert.NotContains(t, startup[0].ContextMap(), "name")
		})
	}
}
