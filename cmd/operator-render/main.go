// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/DataDog/datadog-operator/internal/controller/testutils/renderer"
)

func main() {
	var (
		ddaFile        string
		outputFile     string
		format         string
		supportCilium  bool
		profileEnabled bool
	)

	var dapFiles []string

	flag.StringVar(&ddaFile, "dda", "", "Path to DatadogAgent YAML file (required)")
	flag.StringVar(&outputFile, "output", "", "Output file path (default: stdout)")
	flag.StringVar(&format, "format", "yaml", "Output format: yaml or json")
	flag.BoolVar(&supportCilium, "support-cilium", false, "Generate CiliumNetworkPolicy resources")
	flag.BoolVar(&profileEnabled, "profiles-enabled", false, "Enable DatadogAgentProfile reconciliation (independent of --dap inputs)")
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

	resources, scheme, err := renderer.Render(renderer.Options{
		DDA:            dda,
		DAPs:           daps,
		ProfileEnabled: profileEnabled,
		SupportCilium:  supportCilium,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	out, err := renderer.Serialize(resources, scheme, format)
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
