// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/ext"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/mocktracer"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"github.com/go-logr/logr"
	"github.com/go-logr/logr/funcr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func startMockTracer(t *testing.T) mocktracer.Tracer {
	t.Helper()
	mt := mocktracer.Start()
	enabled.Store(true)
	t.Cleanup(func() {
		enabled.Store(false)
		mt.Stop()
	})
	return mt
}

func testDDA() *metav1.ObjectMeta {
	return &metav1.ObjectMeta{Name: "dda", Namespace: "ns"}
}

func TestStartSpan_Disabled(t *testing.T) {
	ctx := context.Background()
	root, rootCtx := StartReconcileSpan(ctx, "DatadogAgent", "datadogagent.reconcile", testDDA())
	child, childCtx := StartSpan(ctx)

	assert.Nil(t, root)
	assert.Nil(t, child)
	assert.Equal(t, ctx, rootCtx)
	assert.Equal(t, ctx, childCtx)
	// nil spans must be safe to finish
	var err error
	FinishSpan(root, &err)
}

func TestStartSpan_Tags(t *testing.T) {
	mt := startMockTracer(t)

	parent, ctx := StartReconcileSpan(context.Background(), "DatadogAgent", "datadogagent.reconcile", testDDA())
	child, _ := StartSpan(ctx, tracer.Tag(TagAgentComponent, "clusterAgent"))
	err := errors.New("boom")
	FinishSpan(child, &err)
	FinishSpan(parent, nil)

	spans := mt.FinishedSpans()
	require.Len(t, spans, 2)
	c, p := spans[0], spans[1]

	assert.Equal(t, "datadogagent.reconcile", p.OperationName())
	assert.Equal(t, "Reconcile", p.Tag(ext.ResourceName))
	assert.Equal(t, "DatadogAgent", p.Tag("kind"))
	assert.Equal(t, "dda", p.Tag("name"))
	assert.Equal(t, "ns", p.Tag("namespace"))
	assert.Nil(t, p.Tag("reconcileID"), "empty tags are omitted")
	assert.Nil(t, p.Tag(ext.ErrorMsg))

	assert.Equal(t, p.SpanID(), c.ParentID())
	assert.Equal(t, "datadogagent.reconcile", c.OperationName())
	assert.Equal(t, "TestStartSpan_Tags", c.Tag(ext.ResourceName))
	assert.Equal(t, "dda", c.Tag("name"))
	assert.Equal(t, "clusterAgent", c.Tag(TagAgentComponent))
	assert.Equal(t, "boom", c.Tag(ext.ErrorMsg))
	assert.Nil(t, c.Tag("_dd.measured"), "child spans shouldn't add to the reconcile trace metrics")
}

func TestStartSpan_OutsideReconcile(t *testing.T) {
	mt := startMockTracer(t)

	span, _ := StartSpan(context.Background())
	span.Finish()

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, defaultOperationName, spans[0].OperationName())
	assert.Nil(t, spans[0].Tag("kind"))
}

func TestCallerFuncName(t *testing.T) {
	assert.Equal(t, "TestCallerFuncName", callerFuncName(0))
	assert.Equal(t, "TestCallerFuncName", func() string { return callerFuncName(1) }())
}

func TestStartSpan_Logger(t *testing.T) {
	var logged string
	logger := funcr.New(func(_, args string) { logged = args }, funcr.Options{})
	ctx := log.IntoContext(context.Background(), logger)

	log.FromContext(ctx).Info("no span")
	assert.NotContains(t, logged, ext.LogKeyTraceID)

	startMockTracer(t)
	root, ctx := StartReconcileSpan(ctx, "DatadogAgent", "datadogagent.reconcile", testDDA())
	defer root.Finish()
	ctx = log.IntoContext(ctx, log.FromContext(ctx).WithValues("key", "value"))
	child, ctx := StartSpan(ctx)
	defer child.Finish()

	log.FromContext(ctx).Info("with span")
	assert.Contains(t, logged, `"`+ext.LogKeyTraceID+`"="`+logTraceID(child.Context())+`"`)
	spanID := `"` + ext.LogKeySpanID + `"="` + strconv.FormatUint(child.Context().SpanID(), 10) + `"`
	assert.Contains(t, logged, spanID)
	assert.Equal(t, 1, strings.Count(logged, ext.LogKeySpanID), "parent span ID should be replaced")
	assert.Contains(t, logged, `"key"="value"`, "values added after the parent span are kept")
}

func TestStartSpan_TracerNotRunning(t *testing.T) {
	// Enabled but the global tracer is a no-op, e.g. DD_TRACE_ENABLED=false.
	enabled.Store(true)
	t.Cleanup(func() { enabled.Store(false) })

	var logged string
	logger := funcr.New(func(_, args string) { logged = args }, funcr.Options{})
	ctx := log.IntoContext(context.Background(), logger)

	span, ctx := StartSpan(ctx)
	assert.Nil(t, span)
	log.FromContext(ctx).Info("no tracer")
	assert.NotContains(t, logged, ext.LogKeyTraceID)
}

// plainSink hides the wrapped sink's logr.CallDepthLogSink implementation.
type plainSink struct{ logr.LogSink }

func TestLoggerWithSpan_Sink(t *testing.T) {
	startMockTracer(t)
	span, _ := StartSpan(context.Background())
	defer span.Finish()
	traceID := `"` + ext.LogKeyTraceID + `"="` + logTraceID(span.Context()) + `"`

	var prefix, logged string
	funcLogger := funcr.New(func(p, args string) { prefix, logged = p, args }, funcr.Options{})

	logger := loggerWithSpan(funcLogger, span).WithName("ctrl").WithCallDepth(1)
	logger.Error(errors.New("boom"), "failed")
	assert.Equal(t, "ctrl", prefix)
	assert.Contains(t, logged, traceID)
	assert.Contains(t, logged, `"error"="boom"`)

	plain := logr.New(plainSink{funcLogger.GetSink()})
	loggerWithSpan(plain, span).WithCallDepth(1).Info("plain")
	assert.Contains(t, logged, traceID)

	assert.Nil(t, loggerWithSpan(logr.Discard(), span).GetSink(), "nil sink is left as is")
}

func TestStartStop(t *testing.T) {
	t.Setenv("DD_TRACE_ENABLED", "false")
	require.NoError(t, Start())
	assert.True(t, Enabled())
	Stop()
	assert.False(t, Enabled())
}

func TestLogTraceID(t *testing.T) {
	extract := func(traceID string) *tracer.SpanContext {
		t.Helper()
		t.Setenv("DD_TRACE_PROPAGATION_STYLE", "tracecontext")
		sc, err := tracer.NewPropagator(nil).Extract(tracer.TextMapCarrier{
			"traceparent": "00-" + traceID + "-0000000000000001-01",
		})
		require.NoError(t, err)
		return sc
	}
	sc128 := extract("0123456789abcdef0000000000000002")
	sc64 := extract("00000000000000000000000000000002")

	assert.Equal(t, "0123456789abcdef0000000000000002", logTraceID(sc128))
	assert.Equal(t, "2", logTraceID(sc64))

	t.Setenv("DD_TRACE_128_BIT_TRACEID_LOGGING_ENABLED", "false")
	assert.Equal(t, "2", logTraceID(sc128))
}

func TestWrapTransport(t *testing.T) {
	// An empty error-statuses env var behaves like an unset one.
	for name, setEnv := range map[string]bool{"unset": false, "empty": true} {
		t.Run(name, func(t *testing.T) {
			if setEnv {
				t.Setenv(clientErrorStatusesEnvVar, "")
			}
			mt := startMockTracer(t)

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/v1/namespaces/ns/pods/missing":
					w.WriteHeader(http.StatusNotFound)
				case "/api/v1/namespaces/ns/pods/broken":
					w.WriteHeader(http.StatusInternalServerError)
				}
			}))
			defer srv.Close()
			client := &http.Client{Transport: WrapTransport(http.DefaultTransport)}

			get := func(ctx context.Context, path string) {
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+path, nil)
				require.NoError(t, err)
				resp, err := client.Do(req)
				require.NoError(t, err)
				resp.Body.Close()
			}

			// No parent span: not traced.
			get(context.Background(), "/api/v1/watch/namespaces/ns/pods")
			assert.Empty(t, mt.FinishedSpans())

			parent, ctx := StartSpan(context.Background())
			get(ctx, "/api/v1/namespaces/ns/pods/missing")
			get(ctx, "/api/v1/namespaces/ns/pods/broken")
			parent.Finish()

			spans := mt.FinishedSpans()
			require.Len(t, spans, 3)
			notFound, serverErr := spans[0], spans[1]

			assert.Equal(t, parent.Context().SpanID(), notFound.ParentID())
			assert.Equal(t, "GET namespaces/{namespace}/pods/{name}", notFound.Tag(ext.ResourceName))
			assert.Nil(t, notFound.Tag(ext.ErrorMsg), "404 should not be flagged as an error")
			assert.NotNil(t, serverErr.Tag(ext.ErrorMsg), "5xx should be flagged as an error")
		})
	}
}
