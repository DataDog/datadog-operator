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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func TestLoggerWithSpan(t *testing.T) {
	var logged string
	logger := funcr.New(func(_, args string) { logged = args }, funcr.Options{})

	LoggerWithSpan(context.Background(), logger).Info("no span")
	assert.NotContains(t, logged, ext.LogKeyTraceID)

	startMockTracer(t)
	span, ctx := StartSpan(context.Background())
	defer span.Finish()

	LoggerWithSpan(ctx, logger).Info("with span")
	assert.Contains(t, logged, `"`+ext.LogKeyTraceID+`"="`+span.Context().TraceID()+`"`)
	assert.Contains(t, logged, `"`+ext.LogKeySpanID+`"=`)
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
}
