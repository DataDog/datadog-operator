// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package debug

import (
	"net/http"
	"net/http/pprof"
)

// GetExtraMetricHandlers creates debug endpoints if enabled.
func GetExtraMetricHandlers(enabled bool) map[string]http.Handler {
	handlers := make(map[string]http.Handler)

	if !enabled {
		return handlers
	}

	handlers["/debug/pprof/"] = http.HandlerFunc(pprof.Index)
	handlers["/debug/pprof/cmdline"] = http.HandlerFunc(pprof.Cmdline)
	handlers["/debug/pprof/profile"] = http.HandlerFunc(pprof.Profile)
	handlers["/debug/pprof/symbol"] = http.HandlerFunc(pprof.Symbol)
	handlers["/debug/pprof/trace"] = http.HandlerFunc(pprof.Trace)
	handlers["/debug/pprof/heap"] = pprof.Handler("heap")
	handlers["/debug/pprof/goroutine"] = pprof.Handler("goroutine")
	handlers["/debug/pprof/threadcreate"] = pprof.Handler("threadcreate")
	handlers["/debug/pprof/block"] = pprof.Handler("block")
	handlers["/debug/pprof/mutex"] = pprof.Handler("mutex")
	handlers["/debug/pprof/allocs"] = pprof.Handler("allocs")

	return handlers
}
