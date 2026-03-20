// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"

	"github.com/DataDog/datadog-operator/pkg/trace"
)

func startDDASpan(ctx context.Context, extraTags ...tracer.StartSpanOption) (*tracer.Span, context.Context) {
	return trace.StartControllerSpan(ctx, trace.CallerFuncName(1), extraTags...)
}
