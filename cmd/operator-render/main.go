// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	var (
		ddaFile       string
		outputFile    string
		format        string
		supportCilium bool
	)

	var dapFiles []string

	flag.StringVar(&ddaFile, "dda", "", "Path to DatadogAgent YAML file (required)")
	flag.StringVar(&outputFile, "output", "", "Output file path (default: stdout)")
	flag.StringVar(&format, "format", "yaml", "Output format: yaml or json")
	flag.BoolVar(&supportCilium, "support-cilium", false, "Generate CiliumNetworkPolicy resources")
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

	opts := renderOptions{
		DDAFile:        ddaFile,
		DAPFiles:       dapFiles,
		SupportCilium:  supportCilium,
		ProfileEnabled: len(dapFiles) > 0,
	}

	resources, scheme, err := render(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	out, err := serialize(resources, scheme, format)
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
