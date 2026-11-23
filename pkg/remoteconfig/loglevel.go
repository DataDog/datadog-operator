// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	"fmt"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// WithLogLevel lets the log_level of AGENT_CONFIG change level at runtime.
// Removing the config restores the level set at startup.
func WithLogLevel(level zap.AtomicLevel) RemoteConfigUpdaterOption {
	return func(updater *RemoteConfigUpdater) {
		updater.logLevel = &level
		updater.startupLogLevel = level.Level()
	}
}

// parseAgentLogLevel maps a Datadog Agent log level name, as sent in the
// log_level of AGENT_CONFIG, to a zap level. controller-runtime logs
// logr V(n) messages at zap level -n, so "trace" enables V(2) messages and
// "debug" enables V(1) messages.
func parseAgentLogLevel(level string) (zapcore.Level, error) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "trace":
		return zapcore.DebugLevel - 1, nil
	case "debug":
		return zapcore.DebugLevel, nil
	case "info":
		return zapcore.InfoLevel, nil
	case "warn", "warning":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	case "critical":
		return zapcore.DPanicLevel, nil
	case "off":
		return zapcore.FatalLevel + 1, nil
	default:
		return zapcore.InfoLevel, fmt.Errorf("unsupported log level %q", level)
	}
}
