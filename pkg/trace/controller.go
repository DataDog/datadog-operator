// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"context"
	"runtime"
	"strings"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// DefaultOperationName is used when no operation name is set in the controller context.
const DefaultOperationName = "controller.reconcile"

type controllerContextKey struct{}

type controllerContext struct {
	name          string
	namespace     string
	reconcileID   string
	kind          string
	operationName string
}

// WithControllerContext stores the controller identity fields in ctx so all child
// spans pick them up without threading values through every function signature.
func WithControllerContext(ctx context.Context, name, namespace, reconcileID, kind, operationName string) context.Context {
	return context.WithValue(ctx, controllerContextKey{}, controllerContext{
		name:          name,
		namespace:     namespace,
		reconcileID:   reconcileID,
		kind:          kind,
		operationName: operationName,
	})
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
// The operation name and kind/name/namespace/reconcileID tags are read from ctx
// if set via WithControllerContext. resourceName is typically the calling method
// name, derived via CallerFuncName. Returns a nil span and ctx unchanged when
// tracing is disabled; *tracer.Span methods are nil-safe.
func StartControllerSpan(ctx context.Context, resourceName string, extraTags ...tracer.StartSpanOption) (*tracer.Span, context.Context) {
	if !Enabled() {
		return nil, ctx
	}

	cc, _ := ctx.Value(controllerContextKey{}).(controllerContext)
	operationName := cc.operationName
	if operationName == "" {
		operationName = DefaultOperationName
	}

	opts := make([]tracer.StartSpanOption, 0, 6+len(extraTags))
	opts = append(opts,
		tracer.ResourceName(resourceName),
		tracer.Measured(),
	)
	for _, tag := range [][2]string{
		{"kind", cc.kind},
		{"name", cc.name},
		{"namespace", cc.namespace},
		{"reconcileID", cc.reconcileID},
	} {
		if tag[1] != "" {
			opts = append(opts, tracer.Tag(tag[0], tag[1]))
		}
	}
	opts = append(opts, extraTags...)
	return tracer.StartSpanFromContext(ctx, operationName, opts...)
}

// StartReconcileSpan starts the root span of a reconcile for obj, stores the controller
// context in ctx for child spans, and annotates the ctx logger with the trace IDs.
func StartReconcileSpan(ctx context.Context, kind, operationName string, obj metav1.Object) (*tracer.Span, context.Context) {
	if !Enabled() {
		return nil, ctx
	}
	reconcileID := string(controller.ReconcileIDFromContext(ctx))
	ctx = WithControllerContext(ctx, obj.GetName(), obj.GetNamespace(), reconcileID, kind, operationName)
	span, ctx := StartControllerSpan(ctx, "Reconcile")
	ctx = log.IntoContext(ctx, LoggerWithSpan(ctx, log.FromContext(ctx)))
	return span, ctx
}

// FinishSpan finishes span, recording *errp as the span error.
// Intended to be deferred with a named error return: defer trace.FinishSpan(span, &err).
func FinishSpan(span *tracer.Span, errp *error) {
	var err error
	if errp != nil {
		err = *errp
	}
	span.Finish(tracer.WithError(err))
}
