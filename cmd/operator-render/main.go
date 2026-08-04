// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"github.com/go-logr/zapr"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/DataDog/datadog-operator/internal/controller/testutils/renderer"
)

func main() {
	var (
		ddaFile        string
		outputFile     string
		format         string
		logLevel       string
		supportCilium  bool
		profileEnabled bool
		k8sVersion     string
	)

	var dapFiles []string

	flag.StringVar(&ddaFile, "dda", "", "Path to DatadogAgent YAML file (required)")
	flag.StringVar(&outputFile, "output", "", "Output file path (default: stdout)")
	flag.StringVar(&format, "format", "yaml", "Output format: yaml or json")
	flag.StringVar(&logLevel, "log-level", "none", "Log verbosity: none, info, or debug.\n\t\tnone  — no logging (default)\n\t\tinfo  — reconciler Info messages → stderr\n\t\tdebug — reconciler Debug+Info messages → stderr;\n\t\t        DDA/DDAI/DAP status fields included in output")
	flag.BoolVar(&supportCilium, "support-cilium", false, "Generate CiliumNetworkPolicy resources")
	flag.BoolVar(&profileEnabled, "profiles-enabled", false, "Enable DatadogAgentProfile reconciliation (independent of --dap inputs)")
	flag.StringVar(&k8sVersion, "kubernetes-version", renderer.DefaultKubernetesVersion, "Simulated Kubernetes server version (GitVersion). Affects version-gated resources such as the local agent service (created on k8s >= 1.22)")
	flag.Func("dap", "Path to DatadogAgentProfile YAML file (repeatable)", func(s string) error {
		dapFiles = append(dapFiles, s)
		return nil
	})
	flag.Parse()

	if ddaFile == "" {
		fmt.Fprintln(os.Stderr, "error: --dda is required")
		flag.Usage()
		os.Exit(1)
	}
	if format != "yaml" && format != "json" {
		fmt.Fprintf(os.Stderr, "error: --format must be yaml or json, got %q\n", format)
		os.Exit(1)
	}

	log, err := buildLogger(logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	dda, err := renderer.LoadDDA(ddaFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	daps, err := renderer.LoadDAPs(dapFiles)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	debug := logLevel == "debug"
	resources, scheme, err := renderer.Render(renderer.Options{
		DDA:                  dda,
		DAPs:                 daps,
		ProfileEnabled:       profileEnabled,
		SupportCilium:        supportCilium,
		Logger:               log,
		IncludeInputStatuses: debug,
		KubernetesVersion:    k8sVersion,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	out, err := renderer.Serialize(resources, scheme, format, debug)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error serializing output: %v\n", err)
		os.Exit(1)
	}

	if outputFile == "" {
		os.Stdout.Write(out)
	} else {
		if err := os.WriteFile(outputFile, out, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "error writing %s: %v\n", outputFile, err)
			os.Exit(1)
		}
	}
}

// buildLogger constructs a logr.Logger for the given level.
// "none" returns a discard logger (no output).
// "info" logs at Info level to stderr.
// "debug" logs at Debug level (V(1)+) to stderr.
func buildLogger(level string) (logr.Logger, error) {
	if level == "none" {
		return logr.Discard(), nil
	}

	zapLevel := zapcore.InfoLevel
	if level == "debug" {
		zapLevel = zapcore.DebugLevel
	} else if level != "info" {
		return logr.Discard(), fmt.Errorf("--log-level must be none, info, or debug; got %q", level)
	}

	cfg := zap.NewDevelopmentConfig()
	cfg.Level = zap.NewAtomicLevelAt(zapLevel)
	z, err := cfg.Build(zap.WithCaller(false))
	if err != nil {
		return logr.Discard(), fmt.Errorf("building logger: %w", err)
	}
	return zapr.NewLogger(z), nil
}
