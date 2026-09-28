// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"context"
	"strconv"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"github.com/go-logr/logr"
)

// Log keys used by Datadog for log/trace correlation.
const (
	LogKeyTraceID = "dd.trace_id"
	LogKeySpanID  = "dd.span_id"
)

// LoggerWithSpan returns logger annotated with the trace and span IDs of the
// active span in ctx, or logger unchanged when there is no active span.
func LoggerWithSpan(ctx context.Context, logger logr.Logger) logr.Logger {
	span, ok := tracer.SpanFromContext(ctx)
	if !ok || span == nil {
		return logger
	}
	sc := span.Context()
	return logger.WithValues(
		LogKeyTraceID, sc.TraceID(),
		LogKeySpanID, strconv.FormatUint(sc.SpanID(), 10),
	)
}
