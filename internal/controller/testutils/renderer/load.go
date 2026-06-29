// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package renderer

import (
	"fmt"
	"os"

	"sigs.k8s.io/yaml"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// LoadDDA reads a DatadogAgent YAML manifest from disk.
func LoadDDA(path string) (*datadoghqv2alpha1.DatadogAgent, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	dda := &datadoghqv2alpha1.DatadogAgent{}
	if err := yaml.Unmarshal(data, dda); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if dda.Name == "" {
		return nil, fmt.Errorf("%s: DatadogAgent has no name", path)
	}
	return dda, nil
}

// LoadDAPs reads DatadogAgentProfile YAML manifests from disk.
func LoadDAPs(paths []string) ([]*datadoghqv1alpha1.DatadogAgentProfile, error) {
	daps := make([]*datadoghqv1alpha1.DatadogAgentProfile, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		dap := &datadoghqv1alpha1.DatadogAgentProfile{}
		if err := yaml.Unmarshal(data, dap); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
		if dap.Name == "" {
			return nil, fmt.Errorf("%s: DatadogAgentProfile has no name", path)
		}
		daps = append(daps, dap)
	}
	return daps, nil
}
