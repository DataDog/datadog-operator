// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"context"
	"os"
	"strconv"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/ext"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"github.com/go-logr/logr"
)

// LoggerWithSpan adds the active span's trace and span IDs to logger.
func LoggerWithSpan(ctx context.Context, logger logr.Logger) logr.Logger {
	span, ok := tracer.SpanFromContext(ctx)
	if !ok || span == nil {
		return logger
	}
	sc := span.Context()
	return logger.WithValues(
		ext.LogKeyTraceID, logTraceID(sc),
		ext.LogKeySpanID, strconv.FormatUint(sc.SpanID(), 10),
	)
}

// logTraceID matches dd-trace-go's log trace ID format.
func logTraceID(sc *tracer.SpanContext) string {
	if sc.TraceIDUpper() != 0 && log128BitTraceID() {
		return sc.TraceID()
	}
	return strconv.FormatUint(sc.TraceIDLower(), 10)
}

func log128BitTraceID() bool {
	v, err := strconv.ParseBool(os.Getenv("DD_TRACE_128_BIT_TRACEID_LOGGING_ENABLED"))
	return err != nil || v
}
