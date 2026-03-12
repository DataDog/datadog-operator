// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"

	"github.com/DataDog/datadog-operator/pkg/trace"
)

// startDDAISpan starts a span for the DatadogAgentInternal controller.
// The resource name is derived automatically from the calling function name.
// operationName, kind, name, namespace, and reconcileID are read from ctx.
func startDDAISpan(ctx context.Context, extraTags ...tracer.StartSpanOption) (*tracer.Span, context.Context) {
	return trace.StartControllerSpan(ctx, trace.CallerFuncName(1), extraTags...)
}
