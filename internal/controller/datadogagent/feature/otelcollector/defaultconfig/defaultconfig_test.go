// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestWithoutCoreAgent(t *testing.T) {
	for name, config := range map[string]string{
		"default": DefaultOtelCollectorConfig,
		"gateway": DefaultOtelCollectorConfigInGateway("datadog"),
	} {
		t.Run(name, func(t *testing.T) {
			got := WithoutCoreAgent(config)
			assert.NotContains(t, got, "infraattributes")

			var parsed struct {
				Processors map[string]any `yaml:"processors"`
				Service    struct {
					Pipelines map[string]struct {
						Processors []string `yaml:"processors"`
					} `yaml:"pipelines"`
				} `yaml:"service"`
			}
			require.NoError(t, yaml.Unmarshal([]byte(got), &parsed))
			assert.Contains(t, parsed.Processors, "k8s_attributes")
			for pipeline, p := range parsed.Service.Pipelines {
				assert.Contains(t, p.Processors, "k8s_attributes", "pipeline %s", pipeline)
			}
		})
	}
}
