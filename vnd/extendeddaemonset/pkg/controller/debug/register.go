// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// Package debug contains debuging helpers.
package debug

import (
	"net/http"
	"net/http/pprof"
)

// Options used to provide configuration options.
type Options struct {
	CmdLine bool
	Profile bool
	Symbol  bool
	Trace   bool
}

// DefaultOptions returns default options configuration.
func DefaultOptions() *Options {
	return &Options{
		CmdLine: true,
		Profile: true,
		Symbol:  true,
		Trace:   true,
	}
}

// RegisterEndpoint used to register the different debug endpoints.
func RegisterEndpoint(register func(string, http.Handler) error, options *Options) error {
	err := register("/debug/pprof", http.HandlerFunc(pprof.Index))
	if err != nil {
		return err
	}

	if options == nil {
		options = DefaultOptions()
	}
	if options.CmdLine {
		err := register("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
		if err != nil {
			return err
		}
	}
	if options.Profile {
		err := register("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
		if err != nil {
			return err
		}
	}
	if options.Symbol {
		err := register("/debug/pprof/symobol", http.HandlerFunc(pprof.Symbol))
		if err != nil {
			return err
		}
	}
	if options.Trace {
		err := register("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
		if err != nil {
			return err
		}
	}

	return nil
}
