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

// LogLevelSetter changes the Operator log level at runtime. Remote
// Configuration uses it to apply the log_level of AGENT_CONFIG configs, for
// example the temporary debug level that Fleet Automation sets before it
// requests a flare.
type LogLevelSetter interface {
	// SetLogLevel sets the log level from a Datadog Agent log level name
	// (trace, debug, info, warn, error, critical or off).
	SetLogLevel(level string) error
	// ResetLogLevel restores the log level the Operator started with.
	ResetLogLevel()
}

// AtomicLogLevel is a LogLevelSetter backed by the zap.AtomicLevel of the
// Operator logger.
type AtomicLogLevel struct {
	level   zap.AtomicLevel
	startup zapcore.Level
}

var _ LogLevelSetter = (*AtomicLogLevel)(nil)

// NewAtomicLogLevel returns a LogLevelSetter that changes level. The current
// level is the one ResetLogLevel restores.
func NewAtomicLogLevel(level zap.AtomicLevel) *AtomicLogLevel {
	return &AtomicLogLevel{level: level, startup: level.Level()}
}

// SetLogLevel implements LogLevelSetter.
func (a *AtomicLogLevel) SetLogLevel(level string) error {
	zapLevel, err := parseAgentLogLevel(level)
	if err != nil {
		return err
	}
	a.level.SetLevel(zapLevel)
	return nil
}

// ResetLogLevel implements LogLevelSetter.
func (a *AtomicLogLevel) ResetLogLevel() {
	a.level.SetLevel(a.startup)
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
