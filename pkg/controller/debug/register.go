// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package debug

import (
	"net/http"
	"net/http/pprof"
)

// Options used to provide configuration options
type Options struct {
	CmdLine bool
	Profile bool
	Symbol  bool
	Trace   bool
}

// DefaultOptions returns default options configuration
func DefaultOptions() *Options {
	return &Options{
		CmdLine: true,
		Profile: true,
		Symbol:  true,
		Trace:   true,
	}
}

// GetExtraMetricHandlers creates debug endpoints.
func GetExtraMetricHandlers() map[string]http.Handler {
	handlers := make(map[string]http.Handler)
	handlers["debug/pprof"] = http.HandlerFunc(pprof.Index)
	handlers["/debug/pprof/cmdline"] = http.HandlerFunc(pprof.Index)
	handlers["/debug/pprof/cmdline"] = http.HandlerFunc(pprof.Index)
	handlers["/debug/pprof/symobol"] = http.HandlerFunc(pprof.Index)
	handlers["/debug/pprof/trace"] = http.HandlerFunc(pprof.Index)
	return handlers
}
