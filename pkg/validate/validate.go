// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package validate offers a reusable entry point for parsing a DatadogAgent
// manifest and running the operator's own semantic validation against it,
// without needing a running cluster or reconcile loop.
package validate

import (
	"fmt"
	"os"

	"sigs.k8s.io/yaml"

	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// ParseAndValidateBytes strictly decodes a DatadogAgent YAML/JSON document and
// runs the operator's semantic validation (the same ValidateDatadogAgent check
// the reconcile loop performs). Strict decoding rejects unknown/duplicate
// fields, so structural mistakes surface here rather than being silently
// dropped. The parsed DatadogAgent is returned so callers can inspect it.
func ParseAndValidateBytes(data []byte) (*datadoghqv2alpha1.DatadogAgent, error) {
	dda := &datadoghqv2alpha1.DatadogAgent{}
	if err := yaml.UnmarshalStrict(data, dda); err != nil {
		return nil, fmt.Errorf("parsing DatadogAgent: %w", err)
	}

	if dda.Name == "" {
		return nil, fmt.Errorf("DatadogAgent has no metadata.name")
	}

	if err := datadoghqv2alpha1.ValidateDatadogAgent(dda); err != nil {
		return dda, fmt.Errorf("invalid DatadogAgent %q: %w", dda.Name, err)
	}

	return dda, nil
}

// ParseAndValidateFile reads a DatadogAgent manifest from disk and delegates to
// ParseAndValidateBytes.
func ParseAndValidateFile(path string) (*datadoghqv2alpha1.DatadogAgent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return ParseAndValidateBytes(data)
}
