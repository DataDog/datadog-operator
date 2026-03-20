// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"net/http"
	"strconv"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/ext"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
)

// tracingTransport is an http.RoundTripper that creates a Datadog APM span
// for every outbound HTTP request, matching the format produced by Orchestrion:
//
//	operation: "http.request"
//	resource:  "METHOD /path"
//	tags:      http.method, http.url, http.status_code, span.type=http, span.kind=client
type tracingTransport struct {
	wrapped http.RoundTripper
}

// WrapTransport returns an http.RoundTripper that traces every HTTP request.
// Assign to rest.Config.WrapTransport to trace all Kubernetes API server calls:
//
//	restConfig.WrapTransport = trace.WrapTransport
func WrapTransport(rt http.RoundTripper) http.RoundTripper {
	return &tracingTransport{wrapped: rt}
}

func (t *tracingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resource := req.Method + " " + req.URL.Path
	span, ctx := tracer.StartSpanFromContext(req.Context(), "http.request",
		tracer.ResourceName(resource),
		tracer.Tag(ext.HTTPMethod, req.Method),
		tracer.Tag(ext.HTTPURL, req.URL.String()),
		tracer.Tag(ext.SpanType, ext.SpanTypeHTTP),
		tracer.Tag(ext.SpanKind, ext.SpanKindClient),
	)
	req = req.WithContext(ctx)

	resp, err := t.wrapped.RoundTrip(req)
	if resp != nil {
		span.SetTag(ext.HTTPCode, strconv.Itoa(resp.StatusCode))
	}
	span.Finish(tracer.WithError(err))
	return resp, err
}
