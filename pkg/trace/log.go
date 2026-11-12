// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"os"
	"slices"
	"strconv"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/ext"
	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	"github.com/go-logr/logr"
)

// spanSink adds a span's trace and span IDs to every log entry.
type spanSink struct {
	logr.LogSink
	ids []any
}

var _ logr.CallDepthLogSink = spanSink{}

// loggerWithSpan returns logger tagged with span's IDs, replacing any IDs from a parent span.
func loggerWithSpan(logger logr.Logger, span *tracer.Span) logr.Logger {
	sink := logger.GetSink()
	if sink == nil {
		return logger
	}
	if s, ok := sink.(spanSink); ok {
		sink = s.LogSink
	} else if cd, ok := sink.(logr.CallDepthLogSink); ok {
		// Skip the spanSink frame when reporting the caller.
		sink = cd.WithCallDepth(1)
	}
	sc := span.Context()
	return logger.WithSink(spanSink{
		LogSink: sink,
		ids: []any{
			ext.LogKeyTraceID, logTraceID(sc),
			ext.LogKeySpanID, strconv.FormatUint(sc.SpanID(), 10),
		},
	})
}

// Init is a no-op: the wrapped sink is already initialized.
func (s spanSink) Init(logr.RuntimeInfo) {}

func (s spanSink) Info(level int, msg string, keysAndValues ...any) {
	s.LogSink.Info(level, msg, slices.Concat(s.ids, keysAndValues)...)
}

func (s spanSink) Error(err error, msg string, keysAndValues ...any) {
	s.LogSink.Error(err, msg, slices.Concat(s.ids, keysAndValues)...)
}

func (s spanSink) WithValues(keysAndValues ...any) logr.LogSink {
	return spanSink{LogSink: s.LogSink.WithValues(keysAndValues...), ids: s.ids}
}

func (s spanSink) WithName(name string) logr.LogSink {
	return spanSink{LogSink: s.LogSink.WithName(name), ids: s.ids}
}

func (s spanSink) WithCallDepth(depth int) logr.LogSink {
	if cd, ok := s.LogSink.(logr.CallDepthLogSink); ok {
		return spanSink{LogSink: cd.WithCallDepth(depth), ids: s.ids}
	}
	return s
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
