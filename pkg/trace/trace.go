// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package trace provides shared APM tracing utilities for Datadog Operator controllers.
package trace

import (
	"sync/atomic"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
)

var enabled atomic.Bool

// Start starts the global Datadog tracer and enables controller spans.
func Start(opts ...tracer.StartOption) error {
	if err := tracer.Start(opts...); err != nil {
		return err
	}
	enabled.Store(true)
	return nil
}

// Stop flushes and stops the global Datadog tracer.
func Stop() {
	enabled.Store(false)
	tracer.Stop()
}

// Enabled reports whether the tracer was started with Start.
func Enabled() bool {
	return enabled.Load()
}

// SetEnabled toggles controller spans without starting the tracer.
// Intended for tests that install a mocktracer.
func SetEnabled(v bool) {
	enabled.Store(v)
}
