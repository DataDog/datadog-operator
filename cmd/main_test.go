// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"flag"
	"io"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestOptionsParse_EnvOverridesDefaults(t *testing.T) {
	resetCommandLine(t)
	t.Setenv("DD_METRICS_ADDR", ":9090")
	t.Setenv("DD_METRICS_SECURE", "true")
	t.Setenv("DD_PROFILING_ENABLED", "true")
	t.Setenv("DD_PPROF_ENABLED", "true")
	t.Setenv("DD_LEADER_ELECTION_ENABLED", "false")
	t.Setenv("DD_LEADER_ELECTION_LEASE_DURATION", "90s")
	t.Setenv("DD_SUPPORT_CILIUM", "true")
	t.Setenv("DD_MONITOR_CONTROLLER_ENABLED", "true")
	t.Setenv("DD_MAXIMUM_GOROUTINES", "123")
	t.Setenv("DD_UNTAINT_CONTROLLER_WAIT_FOR_CSI_DRIVER", "true")
	t.Setenv("DD_CREATE_CONTROLLER_REVISIONS", "true")

	var opts options
	opts.Parse()

	require.Equal(t, ":9090", opts.metricsAddr)
	require.True(t, opts.secureMetrics)
	require.True(t, opts.profilingEnabled)
	require.True(t, opts.pprofActive)
	require.False(t, opts.enableLeaderElection)
	require.Equal(t, 90*time.Second, opts.leaderElectionLeaseDuration)
	require.True(t, opts.supportCilium)
	require.True(t, opts.datadogMonitorEnabled)
	require.Equal(t, 123, opts.maximumGoroutines)
	require.True(t, opts.untaintControllerWaitForCSIDriver)
	require.True(t, opts.createControllerRevisions)
}

func TestOptionsParse_CLIOverridesEnv(t *testing.T) {
	resetCommandLine(t,
		"-metrics-addr=:7070",
		"-metrics-secure=false",
		"-datadogMonitorEnabled=false",
		"-maximumGoroutines=456",
		"-leader-election-lease-duration=2m",
		"-untaintControllerWaitForCSIDriver=false",
	)
	t.Setenv("DD_METRICS_ADDR", ":9090")
	t.Setenv("DD_METRICS_SECURE", "true")
	t.Setenv("DD_MONITOR_CONTROLLER_ENABLED", "true")
	t.Setenv("DD_MAXIMUM_GOROUTINES", "123")
	t.Setenv("DD_LEADER_ELECTION_LEASE_DURATION", "90s")
	t.Setenv("DD_UNTAINT_CONTROLLER_WAIT_FOR_CSI_DRIVER", "true")

	var opts options
	opts.Parse()

	require.Equal(t, ":7070", opts.metricsAddr)
	require.False(t, opts.secureMetrics)
	require.False(t, opts.datadogMonitorEnabled)
	require.Equal(t, 456, opts.maximumGoroutines)
	require.Equal(t, 2*time.Minute, opts.leaderElectionLeaseDuration)
	require.False(t, opts.untaintControllerWaitForCSIDriver)
}

func TestOptionsParse_InvalidEnvLeavesDefault(t *testing.T) {
	resetCommandLine(t)
	t.Setenv("DD_MAXIMUM_GOROUTINES", "not-an-int")
	t.Setenv("DD_LEADER_ELECTION_LEASE_DURATION", "not-a-duration")
	t.Setenv("DD_MONITOR_CONTROLLER_ENABLED", "not-a-boolean-meaning-string")

	var opts options
	opts.Parse()

	require.Equal(t, defaultMaximumGoroutines, opts.maximumGoroutines)
	require.Equal(t, 60*time.Second, opts.leaderElectionLeaseDuration)
	require.False(t, opts.datadogMonitorEnabled)
}

func resetCommandLine(t *testing.T, args ...string) {
	t.Helper()

	previousCommandLine := flag.CommandLine
	previousArgs := os.Args
	flag.CommandLine = flag.NewFlagSet(t.Name(), flag.ContinueOnError)
	flag.CommandLine.SetOutput(io.Discard)
	os.Args = append([]string{t.Name()}, args...)

	t.Cleanup(func() {
		flag.CommandLine = previousCommandLine
		os.Args = previousArgs
	})
}

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
	for range callers {
		go func() {
			defer wg.Done()
			_ = check(nil)
		}()
	}
	wg.Wait()

	require.Len(t, logs.FilterMessage("healthz check entering failing state").All(), 1)
}
