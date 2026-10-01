// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"context"
	"strconv"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/ext"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"github.com/go-logr/logr"
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
		ext.LogKeyTraceID, sc.TraceID(),
		ext.LogKeySpanID, strconv.FormatUint(sc.SpanID(), 10),
	)
}
