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

// TagAgentComponent is the span tag for the Agent component being reconciled.
// The "component" tag is reserved by dd-trace-go for the instrumentation library.
const TagAgentComponent = "agent.component"

// defaultOperationName is used when a span is started outside a reconcile.
const defaultOperationName = "controller.reconcile"

type controllerContextKey struct{}

type controllerContext struct {
	operationName string
	tags          [][2]string
}

// StartReconcileSpan starts the root span of a reconcile for obj, stores the controller
// identity in ctx for child spans, and annotates the ctx logger with the trace IDs.
func StartReconcileSpan(ctx context.Context, kind, operationName string, obj metav1.Object) (*tracer.Span, context.Context) {
	if !Enabled() {
		return nil, ctx
	}
	ctx = context.WithValue(ctx, controllerContextKey{}, controllerContext{
		operationName: operationName,
		tags: [][2]string{
			{"kind", kind},
			{"name", obj.GetName()},
			{"namespace", obj.GetNamespace()},
			{"reconcileID", string(controller.ReconcileIDFromContext(ctx))},
		},
	})
	span, ctx := startSpan(ctx, "Reconcile")
	ctx = log.IntoContext(ctx, LoggerWithSpan(ctx, log.FromContext(ctx)))
	return span, ctx
}

// StartSpan starts a child span of the current reconcile, named after the calling
// function. Returns a nil span and ctx unchanged when tracing is disabled;
// *tracer.Span methods are nil-safe.
func StartSpan(ctx context.Context, extraTags ...tracer.StartSpanOption) (*tracer.Span, context.Context) {
	if !Enabled() {
		return nil, ctx
	}
	return startSpan(ctx, callerFuncName(1), extraTags...)
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

func startSpan(ctx context.Context, resourceName string, extraTags ...tracer.StartSpanOption) (*tracer.Span, context.Context) {
	cc, _ := ctx.Value(controllerContextKey{}).(controllerContext)
	operationName := cc.operationName
	if operationName == "" {
		operationName = defaultOperationName
	}

	opts := []tracer.StartSpanOption{tracer.ResourceName(resourceName), tracer.Measured()}
	for _, tag := range cc.tags {
		if tag[1] != "" {
			opts = append(opts, tracer.Tag(tag[0], tag[1]))
		}
	}
	opts = append(opts, extraTags...)
	return tracer.StartSpanFromContext(ctx, operationName, opts...)
}

// callerFuncName returns the unqualified name of the function depth frames above
// its caller: depth=0 is the caller of callerFuncName, depth=1 its caller, etc.
func callerFuncName(depth int) string {
	if pc, _, _, ok := runtime.Caller(depth + 1); ok {
		if fn := runtime.FuncForPC(pc); fn != nil {
			name := fn.Name()
			return name[strings.LastIndex(name, ".")+1:]
		}
	}
	return "unknown"
}
