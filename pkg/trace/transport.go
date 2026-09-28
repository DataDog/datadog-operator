// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"net/http"
	"os"

	kubetrace "github.com/DataDog/dd-trace-go/contrib/k8s.io/client-go/v2/kubernetes"
	httptrace "github.com/DataDog/dd-trace-go/contrib/net/http/v2"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
)

const clientErrorStatusesEnvVar = "DD_TRACE_HTTP_CLIENT_ERROR_STATUSES"

// WrapTransport returns an http.RoundTripper that traces Kubernetes API requests
// made within an active span. Requests without a parent span (informer list/watch,
// leader election) are not traced. Only 5xx responses are flagged as errors, since
// 404 and 409 are expected during reconciliation; DD_TRACE_HTTP_CLIENT_ERROR_STATUSES
// overrides this. Pass to rest.Config.Wrap:
//
//	restConfig.Wrap(trace.WrapTransport)
func WrapTransport(rt http.RoundTripper) http.RoundTripper {
	opts := []httptrace.RoundTripperOption{
		httptrace.WithIgnoreRequest(hasNoParentSpan),
	}
	if _, found := os.LookupEnv(clientErrorStatusesEnvVar); !found {
		opts = append(opts, httptrace.WithStatusCheck(isServerError))
	}
	return kubetrace.WrapRoundTripperFunc(opts...)(rt)
}

func hasNoParentSpan(req *http.Request) bool {
	_, ok := tracer.SpanFromContext(req.Context())
	return !ok
}

func isServerError(statusCode int) bool {
	return statusCode >= 500 && statusCode < 600
}
