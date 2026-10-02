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
const TagAgentComponent = "agent.component"

// defaultOperationName is used when a span is started outside a reconcile.
const defaultOperationName = "controller.reconcile"

type controllerContextKey struct{}

type controllerContext struct {
	operationName string
	tags          [][2]string
}

// StartReconcileSpan starts the root span of a reconcile.
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
	return startSpan(ctx, "Reconcile")
}

// StartSpan starts a span whose resource name is the calling function, and
// tags the context logger with its IDs.
func StartSpan(ctx context.Context, extraTags ...tracer.StartSpanOption) (*tracer.Span, context.Context) {
	if !Enabled() {
		return nil, ctx
	}
	return startSpan(ctx, callerFuncName(1), extraTags...)
}

// FinishSpan finishes span, recording *errp as the span error.
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
	span, ctx := tracer.StartSpanFromContext(ctx, operationName, opts...)
	if span == nil {
		// Tracer isn't running (e.g. DD_TRACE_ENABLED=false).
		return nil, ctx
	}
	return span, log.IntoContext(ctx, loggerWithSpan(log.FromContext(ctx), span))
}

// callerFuncName returns the name of the function depth frames above its caller.
func callerFuncName(depth int) string {
	if pc, _, _, ok := runtime.Caller(depth + 1); ok {
		if fn := runtime.FuncForPC(pc); fn != nil {
			name := fn.Name()
			return name[strings.LastIndex(name, ".")+1:]
		}
	}
	return "unknown"
}
