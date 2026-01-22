// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"
	"testing"

	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/go-logr/zapr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestCheckRequiredCredentials(t *testing.T) {
	credErr := errors.New("empty API key and/or App key")
	expectedErr := fmt.Errorf("Unable to retrieve Datadog API credentials required by one or more enabled controllers, err:%w", credErr)

	tests := []struct {
		name                          string
		datadogMonitorEnabled         bool
		datadogDashboardEnabled       bool
		datadogSLOEnabled             bool
		datadogGenericResourceEnabled bool
		setupEnv                      func()
		cleanupEnv                    func()
		wantErr                       error
	}{
		{
			name:                          "no controllers enabled, no credentials required",
			datadogMonitorEnabled:         false,
			datadogDashboardEnabled:       false,
			datadogSLOEnabled:             false,
			datadogGenericResourceEnabled: false,
			setupEnv: func() {
				os.Unsetenv("DD_API_KEY")
				os.Unsetenv("DD_APP_KEY")
			},
			cleanupEnv: func() {},
			wantErr:    nil,
		},
		{
			name:                          "monitor controller enabled, credentials present",
			datadogMonitorEnabled:         true,
			datadogDashboardEnabled:       false,
			datadogSLOEnabled:             false,
			datadogGenericResourceEnabled: false,
			setupEnv: func() {
				os.Setenv("DD_API_KEY", "test-api-key")
				os.Setenv("DD_APP_KEY", "test-app-key")
			},
			cleanupEnv: func() {
				os.Unsetenv("DD_API_KEY")
				os.Unsetenv("DD_APP_KEY")
			},
			wantErr: nil,
		},
		{
			name:                          "monitor controller enabled, credentials missing",
			datadogMonitorEnabled:         true,
			datadogDashboardEnabled:       false,
			datadogSLOEnabled:             false,
			datadogGenericResourceEnabled: false,
			setupEnv: func() {
				os.Unsetenv("DD_API_KEY")
				os.Unsetenv("DD_APP_KEY")
			},
			cleanupEnv: func() {},
			wantErr:    expectedErr,
		},
		{
			name:                          "dashboard controller enabled, credentials missing",
			datadogMonitorEnabled:         false,
			datadogDashboardEnabled:       true,
			datadogSLOEnabled:             false,
			datadogGenericResourceEnabled: false,
			setupEnv: func() {
				os.Unsetenv("DD_API_KEY")
				os.Unsetenv("DD_APP_KEY")
			},
			cleanupEnv: func() {},
			wantErr:    expectedErr,
		},
		{
			name:                          "SLO controller enabled, credentials missing",
			datadogMonitorEnabled:         false,
			datadogDashboardEnabled:       false,
			datadogSLOEnabled:             true,
			datadogGenericResourceEnabled: false,
			setupEnv: func() {
				os.Unsetenv("DD_API_KEY")
				os.Unsetenv("DD_APP_KEY")
			},
			cleanupEnv: func() {},
			wantErr:    expectedErr,
		},
		{
			name:                          "generic resource controller enabled, credentials missing",
			datadogMonitorEnabled:         false,
			datadogDashboardEnabled:       false,
			datadogSLOEnabled:             false,
			datadogGenericResourceEnabled: true,
			setupEnv: func() {
				os.Unsetenv("DD_API_KEY")
				os.Unsetenv("DD_APP_KEY")
			},
			cleanupEnv: func() {},
			wantErr:    expectedErr,
		},
		{
			name:                          "multiple controllers enabled, credentials present",
			datadogMonitorEnabled:         true,
			datadogDashboardEnabled:       true,
			datadogSLOEnabled:             true,
			datadogGenericResourceEnabled: true,
			setupEnv: func() {
				os.Setenv("DD_API_KEY", "test-api-key")
				os.Setenv("DD_APP_KEY", "test-app-key")
			},
			cleanupEnv: func() {
				os.Unsetenv("DD_API_KEY")
				os.Unsetenv("DD_APP_KEY")
			},
			wantErr: nil,
		},
		{
			name:                          "multiple controllers enabled, credentials missing",
			datadogMonitorEnabled:         true,
			datadogDashboardEnabled:       false,
			datadogSLOEnabled:             true,
			datadogGenericResourceEnabled: false,
			setupEnv: func() {
				os.Unsetenv("DD_API_KEY")
				os.Unsetenv("DD_APP_KEY")
			},
			cleanupEnv: func() {},
			wantErr:    expectedErr,
		},
		{
			name:                          "only API key present, controller enabled - should fail",
			datadogMonitorEnabled:         true,
			datadogDashboardEnabled:       false,
			datadogSLOEnabled:             false,
			datadogGenericResourceEnabled: false,
			setupEnv: func() {
				os.Setenv("DD_API_KEY", "test-api-key")
				os.Unsetenv("DD_APP_KEY")
			},
			cleanupEnv: func() {
				os.Unsetenv("DD_API_KEY")
			},
			wantErr: expectedErr,
		},
		{
			name:                          "only APP key present, controller enabled - should fail",
			datadogMonitorEnabled:         false,
			datadogDashboardEnabled:       true,
			datadogSLOEnabled:             false,
			datadogGenericResourceEnabled: false,
			setupEnv: func() {
				os.Unsetenv("DD_API_KEY")
				os.Setenv("DD_APP_KEY", "test-app-key")
			},
			cleanupEnv: func() {
				os.Unsetenv("DD_APP_KEY")
			},
			wantErr: expectedErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupEnv()
			defer tt.cleanupEnv()

			opts := &options{
				datadogMonitorEnabled:         tt.datadogMonitorEnabled,
				datadogDashboardEnabled:       tt.datadogDashboardEnabled,
				datadogSLOEnabled:             tt.datadogSLOEnabled,
				datadogGenericResourceEnabled: tt.datadogGenericResourceEnabled,
			}

			credsManager := config.NewCredentialManager(nil)
			_, err := credsManager.GetCredentials()
			err = checkRequiredCredentials(opts, err)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.EqualError(t, err, tt.wantErr.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
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
	for i := 0; i < callers; i++ {
		go func() {
			defer wg.Done()
			_ = check(nil)
		}()
	}
	wg.Wait()

	require.Len(t, logs.FilterMessage("healthz check entering failing state").All(), 1)
}
