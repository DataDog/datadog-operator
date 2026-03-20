// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package trace provides shared APM tracing utilities for Datadog Operator controllers.
package trace

import (
	"context"
	"runtime"
	"strings"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
)

type contextKey string

const (
	contextKeyName          contextKey = "name"
	contextKeyNamespace     contextKey = "namespace"
	contextKeyReconcileID   contextKey = "reconcileID"
	contextKeyKind          contextKey = "kind"
	contextKeyOperationName contextKey = "operationName"
)

// WithControllerContext stores all fixed controller identity fields in ctx so all child
// spans pick them up without threading values through every function signature.
func WithControllerContext(ctx context.Context, name, namespace, reconcileID, kind, operationName string) context.Context {
	ctx = context.WithValue(ctx, contextKeyName, name)
	ctx = context.WithValue(ctx, contextKeyNamespace, namespace)
	ctx = context.WithValue(ctx, contextKeyReconcileID, reconcileID)
	ctx = context.WithValue(ctx, contextKeyKind, kind)
	ctx = context.WithValue(ctx, contextKeyOperationName, operationName)
	return ctx
}

// CallerFuncName returns the name of the function at the given depth in the call stack,
// relative to the caller of CallerFuncName. depth=1 returns the caller of CallerFuncName,
// depth=2 returns the caller's caller, etc.
func CallerFuncName(depth int) string {
	if pc, _, _, ok := runtime.Caller(depth + 1); ok {
		if fn := runtime.FuncForPC(pc); fn != nil {
			name := fn.Name()
			if idx := strings.LastIndex(name, "."); idx >= 0 {
				return name[idx+1:]
			}
		}
	}
	return "unknown"
}

// StartControllerSpan starts a span for a controller reconcile loop.
// operationName, kind, name, namespace, and reconcileID are all read from ctx
// if set via WithControllerContext.
// resourceName is typically the calling method name, derived via CallerFuncName.
func StartControllerSpan(ctx context.Context, resourceName string, extraTags ...tracer.StartSpanOption) (*tracer.Span, context.Context) {
	operationName, _ := ctx.Value(contextKeyOperationName).(string)
	if operationName == "" {
		operationName = "controller.reconcile"
	}

	opts := []tracer.StartSpanOption{
		tracer.ResourceName(resourceName),
		tracer.Measured(),
	}
	if kind, ok := ctx.Value(contextKeyKind).(string); ok {
		opts = append(opts, tracer.Tag("kind", kind))
	}
	if name, ok := ctx.Value(contextKeyName).(string); ok {
		opts = append(opts, tracer.Tag("name", name))
	}
	if namespace, ok := ctx.Value(contextKeyNamespace).(string); ok {
		opts = append(opts, tracer.Tag("namespace", namespace))
	}
	if reconcileID, ok := ctx.Value(contextKeyReconcileID).(string); ok {
		opts = append(opts, tracer.Tag("reconcileID", reconcileID))
	}
	opts = append(opts, extraTags...)
	return tracer.StartSpanFromContext(ctx, operationName, opts...)
}
