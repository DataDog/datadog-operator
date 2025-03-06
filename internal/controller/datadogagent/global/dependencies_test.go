// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package global

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
)

type InstallInfoData struct {
	InstallMethod InstallMethod `yaml:"install_method"`
}

type InstallMethod struct {
	Tool             string `yaml:"tool"`
	ToolVersion      string `yaml:"tool_version"`
	InstallerVersion string `yaml:"installer_version"`
}

func Test_getInstallInfoValue(t *testing.T) {
	tests := []struct {
		name                   string
		toolVersionEnvVarValue string
		expectedToolVersion    string
	}{
		{
			name:                   "Env var empty/unset (os.Getenv returns unset env var as empty string)",
			toolVersionEnvVarValue: "",
			expectedToolVersion:    "unknown",
		},
		{
			name:                   "Env var set",
			toolVersionEnvVarValue: "foo",
			expectedToolVersion:    "foo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(DDInstallInfoToolVersion, tt.toolVersionEnvVarValue)
			installInfo := InstallInfoData{}

			test := getInstallInfoValue()

			err := yaml.Unmarshal([]byte(test), &installInfo)
			assert.NoError(t, err)

			assert.Equal(t, "datadog-operator", installInfo.InstallMethod.Tool)
			assert.Equal(t, tt.expectedToolVersion, installInfo.InstallMethod.ToolVersion)
			assert.Equal(t, "0.0.0", installInfo.InstallMethod.InstallerVersion)
		})
	}
}
