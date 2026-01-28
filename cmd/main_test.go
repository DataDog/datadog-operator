// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"net/http"
	"sync"
	"testing"

	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestGoroutinesNumberHealthzCheck_LogsOnceOnFailureAndRecovery(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	logger := zapr.NewLogger(zap.New(core))

	max := 0 // there is always at least 1 goroutine, so this forces a failure
	check := newGoroutinesNumberHealthzCheck(logger, &max)

	err := check(nil)
	require.Error(t, err)
	require.Len(t, logs.FilterMessage("healthz check entering failing state").All(), 1)

	// Second failing probe should not emit another error log.
	err = check(nil)
	require.Error(t, err)
	require.Len(t, logs.FilterMessage("healthz check entering failing state").All(), 1)

	// Recover and ensure we log recovery once.
	max = 400
	require.NoError(t, check(&http.Request{}))
	require.Len(t, logs.FilterMessage("healthz check recovered").All(), 1)

	// Second success should not emit another recovery log.
	require.NoError(t, check(nil))
	require.Len(t, logs.FilterMessage("healthz check recovered").All(), 1)
}

func TestGoroutinesNumberHealthzCheck_ConcurrentCallsLogSingleFailure(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	logger := zapr.NewLogger(zap.New(core))

	max := 0
	check := newGoroutinesNumberHealthzCheck(logger, &max)

	const callers = 25
	var wg sync.WaitGroup
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			_ = check(nil)
		}()
	}
	wg.Wait()

	require.Len(t, logs.FilterMessage("healthz check entering failing state").All(), 1)
}
