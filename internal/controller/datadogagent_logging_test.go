// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build integration

package controller

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/ext"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/mocktracer"
	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/testutils"
	"github.com/DataDog/datadog-operator/pkg/trace"
)

// Run the registered controller through its queue, rather than manually seeding
// a context logger. This covers WithLogConstructor, the public Reconcile entry,
// and the root/child span loggers on the active DDA -> DDAI path together.
func TestDatadogAgentReconcileLogging(t *testing.T) {
	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases", "v1")},
		ErrorIfCRDPathMissing: true,
	}
	cfg, err := testEnv.Start()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, testEnv.Stop()) })

	mt := mocktracer.Start()
	previous := trace.Enabled()
	trace.SetEnabled(true)
	t.Cleanup(func() { trace.SetEnabled(previous); mt.Stop() })
	// Make the expected trace ID representation deterministic.
	t.Setenv("DD_TRACE_128_BIT_TRACEID_LOGGING_ENABLED", "false")

	scheme := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(scheme))
	require.NoError(t, apiextensionsv1.AddToScheme(scheme))
	require.NoError(t, apiregistrationv1.AddToScheme(scheme))
	require.NoError(t, v1alpha1.AddToScheme(scheme))
	require.NoError(t, v2alpha1.AddToScheme(scheme))
	core, logs := observer.New(zapcore.DebugLevel)
	logger := zapr.NewLogger(zap.New(core))
	ctrl.SetLogger(logger)
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 scheme,
		Logger:                 logger,
		Metrics:                metricsserver.Options{BindAddress: "0"},
		HealthProbeBindAddress: "0",
		Controller:             config.Controller{SkipNameValidation: ptr.To(true)},
	})
	require.NoError(t, err)
	r := &DatadogAgentReconciler{
		Client: mgr.GetClient(), Scheme: scheme,
		Recorder: mgr.GetEventRecorderFor("datadogagent-logging-test"),
	}
	require.NoError(t, r.SetupWithManager(mgr, nil))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var managerErr error
	go func() { defer close(done); managerErr = mgr.Start(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
			require.NoError(t, managerErr)
		case <-time.After(10 * time.Second):
			t.Error("manager did not stop")
		}
	})
	cli, err := client.New(cfg, client.Options{Scheme: scheme})
	require.NoError(t, err)
	dda := testutils.NewInitializedDatadogAgentBuilder("default", "logging-test").Build()
	require.NoError(t, cli.Create(ctx, dda))

	reconciled := assert.Eventually(t, func() bool {
		entries := logs.FilterMessage("creating DatadogAgentInternal").All()
		if len(entries) == 0 {
			return false
		}
		reconcileID := fmt.Sprint(entries[0].ContextMap()["reconcileID"])
		for _, span := range mt.FinishedSpans() {
			if span.ParentID() == 0 && span.Tag("reconcileID") == reconcileID {
				return true
			}
		}
		return false
	}, 30*time.Second, 100*time.Millisecond)
	require.True(t, reconciled, "real reconcile did not finish DDAI creation; errors: %v", logs.FilterLevelExact(zapcore.ErrorLevel).All())
	require.NoError(t, cli.Get(ctx, client.ObjectKeyFromObject(dda), &v1alpha1.DatadogAgentInternal{}))
	cancel()
	select {
	case <-done:
		require.NoError(t, managerErr)
	case <-time.After(10 * time.Second):
		t.Fatal("manager did not stop")
	}

	spans := make(map[string]*mocktracer.Span)
	for _, span := range mt.FinishedSpans() {
		spans[strconv.FormatUint(span.SpanID(), 10)] = span
	}
	childEntry := logs.FilterMessage("creating DatadogAgentInternal").All()[0]
	childFields := childEntry.ContextMap()
	var rootEntry observer.LoggedEntry
	found := false
	for _, entry := range logs.FilterMessage("Reconciling DatadogAgent").All() {
		if entry.ContextMap()["reconcileID"] == childFields["reconcileID"] {
			rootEntry, found = entry, true
			break
		}
	}
	require.True(t, found, "missing root log for DDAI creation reconcile")
	for _, entry := range []observer.LoggedEntry{rootEntry, childEntry} {
		fields := entry.ContextMap()
		assert.Equal(t, "controllers.DatadogAgent", entry.LoggerName)
		assert.Equal(t, "datadogagent", fields["controller"])
		assert.Equal(t, "datadoghq.com", fields["controllerGroup"])
		assert.Equal(t, "DatadogAgent", fields["controllerKind"])
		assert.Equal(t, dda.Namespace, fields["namespace"])
		assert.Equal(t, dda.Name, fields["name"])
		for _, key := range []string{"controller", "controllerGroup", "controllerKind", "namespace", "name"} {
			count := 0
			for _, field := range entry.Context {
				if field.Key == key {
					count++
				}
			}
			assert.Equal(t, 1, count, "%s must appear exactly once", key)
		}
		assert.NotContains(t, fields, "DatadogAgent")
		assert.NotContains(t, fields, "datadogagent")
		require.NotEmpty(t, fields["reconcileID"])
		id, ok := fields["dd.span_id"].(string)
		require.True(t, ok, "missing span ID: %v", fields)
		span := spans[id]
		require.NotNil(t, span, "log references an unknown span")
		assert.Equal(t, strconv.FormatUint(span.TraceID(), 10), fields["dd.trace_id"])
		assert.Equal(t, fmt.Sprint(fields["reconcileID"]), span.Tag("reconcileID"))
	}
	assert.Equal(t, "DatadogAgentInternal", childFields["object.kind"])
	assert.Equal(t, dda.Namespace, childFields["object.namespace"])
	assert.Equal(t, dda.Name, childFields["object.name"])
	rootSpan := spans[rootEntry.ContextMap()["dd.span_id"].(string)]
	childSpan := spans[childFields["dd.span_id"].(string)]
	assert.Nil(t, rootSpan.Tag(ext.ErrorMsg))
	assert.Nil(t, childSpan.Tag(ext.ErrorMsg))
	assert.Zero(t, rootSpan.ParentID())
	assert.Equal(t, "Reconcile", rootSpan.Tag(ext.ResourceName))
	assert.Equal(t, "createOrUpdateDDAI", childSpan.Tag(ext.ResourceName))
	assert.Equal(t, rootSpan.SpanID(), childSpan.ParentID())
	assert.NotEqual(t, rootSpan.SpanID(), childSpan.SpanID())
}
