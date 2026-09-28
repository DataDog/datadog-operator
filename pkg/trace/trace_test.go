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
	"testing"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/ext"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/mocktracer"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"github.com/go-logr/logr/funcr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestStartControllerSpan_Disabled(t *testing.T) {
	ctx := WithControllerContext(context.Background(), "dda", "ns", "id", "DatadogAgent", "")
	span, spanCtx := StartControllerSpan(ctx, "Reconcile")

	assert.Nil(t, span)
	assert.Equal(t, ctx, spanCtx)
	// nil spans must be safe to finish
	var err error
	FinishSpan(span, &err)
}

func TestStartControllerSpan_Tags(t *testing.T) {
	mt := startMockTracer(t)

	ctx := WithControllerContext(context.Background(), "dda", "ns", "rid", "DatadogAgent", "datadogagent.reconcile")
	parent, ctx := StartControllerSpan(ctx, "Reconcile")
	child, _ := StartControllerSpan(ctx, "child", tracer.Tag("component", "clusterAgent"))
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
	assert.Equal(t, "rid", p.Tag("reconcileID"))
	assert.Nil(t, p.Tag(ext.ErrorMsg))

	assert.Equal(t, p.SpanID(), c.ParentID())
	assert.Equal(t, "child", c.Tag(ext.ResourceName))
	assert.Equal(t, "clusterAgent", c.Tag("component"))
	assert.Equal(t, "boom", c.Tag(ext.ErrorMsg))
}

func TestStartControllerSpan_DefaultOperationName(t *testing.T) {
	mt := startMockTracer(t)

	span, _ := StartControllerSpan(context.Background(), "Reconcile")
	span.Finish()

	spans := mt.FinishedSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, DefaultOperationName, spans[0].OperationName())
	assert.Nil(t, spans[0].Tag("kind"))
}

func TestCallerFuncName(t *testing.T) {
	assert.Equal(t, "TestCallerFuncName", CallerFuncName(0))
	assert.Equal(t, "TestCallerFuncName", func() string { return CallerFuncName(1) }())
}

func TestLoggerWithSpan(t *testing.T) {
	var logged string
	logger := funcr.New(func(_, args string) { logged = args }, funcr.Options{})

	LoggerWithSpan(context.Background(), logger).Info("no span")
	assert.NotContains(t, logged, LogKeyTraceID)

	startMockTracer(t)
	span, ctx := StartControllerSpan(context.Background(), "Reconcile")
	defer span.Finish()

	LoggerWithSpan(ctx, logger).Info("with span")
	assert.Contains(t, logged, `"`+LogKeyTraceID+`"="`+span.Context().TraceID()+`"`)
	assert.Contains(t, logged, `"`+LogKeySpanID+`"=`)
}

func TestWrapTransport(t *testing.T) {
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

	parent, ctx := StartControllerSpan(context.Background(), "Reconcile")
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
}
