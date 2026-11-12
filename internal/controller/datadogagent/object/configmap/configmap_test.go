// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package configmap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
)

func TestCustomConfigYAMLValidation(t *testing.T) {
	for _, tt := range []struct {
		name   string
		config string
		valid  bool
	}{
		{name: "nested mapping", config: "instances:\n  - tags: [env:test]\n    enabled: true\n", valid: true},
		{name: "malformed YAML", config: "instances: ["},
		{name: "duplicate keys", config: "enabled: true\nenabled: false\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cm, err := BuildConfigMapConfigData("ns", &tt.config, "config", "check.yaml")
			if tt.valid {
				require.NoError(t, err)
				assert.Equal(t, tt.config, cm.Data["check.yaml"])
			} else {
				require.Error(t, err)
				assert.Nil(t, cm)
			}

			configs := map[string]string{"check.yaml": tt.config}
			multi, err := BuildConfigMapMulti("ns", configs, "config", true)
			vol := volume.GetVolumeFromMultiCustomConfig(&v2alpha1.MultiCustomConfig{ConfigDataMap: configs}, "checks", "config")
			if tt.valid {
				require.NoError(t, err)
				assert.Equal(t, configs, multi.Data)
				require.Len(t, vol.ConfigMap.Items, 1)
				assert.Equal(t, "check.yaml", vol.ConfigMap.Items[0].Key)
			} else {
				require.Error(t, err)
				assert.Empty(t, multi.Data)
				assert.Empty(t, vol.ConfigMap.Items)
			}

			unvalidated, err := BuildConfigMapMulti("ns", configs, "config", false)
			require.NoError(t, err)
			assert.Equal(t, configs, unvalidated.Data)
		})
	}
}
