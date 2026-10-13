// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"fmt"
	"net/http"
	"os"

	kubetrace "github.com/DataDog/dd-trace-go/contrib/k8s.io/client-go/v2/kubernetes"
	httptrace "github.com/DataDog/dd-trace-go/contrib/net/http/v2"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/ext"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
)

const clientErrorStatusesEnvVar = "DD_TRACE_HTTP_CLIENT_ERROR_STATUSES"

// kubernetesComponentName matches the dd-trace-go client-go contrib.
const kubernetesComponentName = "k8s.io/client-go/kubernetes"

// auditIDHeader is the header the apiserver uses as the request's audit ID,
// generating a UUID of its own only when it is absent. The apiserver truncates
// the ID to 64 characters before putting it on its own trace spans, so the
// value below stays well under that.
const auditIDHeader = "Audit-Id"

// WrapTransport traces Kubernetes API requests made within an active span.
func WrapTransport(rt http.RoundTripper) http.RoundTripper {
	opts := []httptrace.RoundTripperOption{
		httptrace.WithIgnoreRequest(hasNoParentSpan),
		httptrace.WithBefore(tagKubernetesRequest),
	}
	if os.Getenv(clientErrorStatusesEnvVar) == "" {
		opts = append(opts, httptrace.WithStatusCheck(isServerError))
	}
	return httptrace.WrapRoundTripper(rt, opts...)
}

// tagKubernetesRequest tags span like the dd-trace-go client-go contrib, except
// for the audit ID. The contrib sends the trace ID, which every API call in a
// reconcile shares; the apiserver takes that header as is, so a whole reconcile
// would collapse onto one audit ID and its request and response events could no
// longer be paired up. Sending the span ID too keeps the ID unique per request
// while still pointing back at the reconcile that made it.
func tagKubernetesRequest(req *http.Request, span *tracer.Span) {
	span.SetTag(ext.ResourceName, kubetrace.RequestToResource(req.Method, req.URL.Path))
	span.SetTag(ext.Component, kubernetesComponentName)
	span.SetTag(ext.SpanKind, ext.SpanKindClient)

	sc := span.Context()
	if sc.TraceID() == tracer.TraceIDZero {
		// Tracer isn't running.
		return
	}
	auditID := fmt.Sprintf("%s-%016x", sc.TraceID(), sc.SpanID())
	req.Header.Set(auditIDHeader, auditID)
	span.SetTag("kubernetes.audit_id", auditID)
}

func hasNoParentSpan(req *http.Request) bool {
	_, ok := tracer.SpanFromContext(req.Context())
	return !ok
}

func isServerError(statusCode int) bool {
	return statusCode >= 500 && statusCode < 600
}
