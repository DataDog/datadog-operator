// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package version

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	var jsonOut, textOut bytes.Buffer
	PrintVersionWriter(&jsonOut, "json")
	PrintVersionWriter(&textOut, "text")
	for name, out := range map[string][]byte{"json": jsonOut.Bytes(), "text": textOut.Bytes()} {
		t.Run(name, func(t *testing.T) {
			info, err := Parse(out)
			require.NoError(t, err)
			assert.Equal(t, Version, info.Version)
			assert.NotEmpty(t, info.Go)
			assert.NotEmpty(t, info.Os)
		})
	}

	info, err := Parse([]byte("Version:\n- Version: v1.22.0_abc\n- Build time: 2026-11-13/20:22:32\n- Git Commit: 163b93d0\n- Go Version: go1.26\n- Go OS/Arch: linux/arm64\n"))
	require.NoError(t, err)
	assert.Equal(t, JSON{Version: "v1.22.0_abc", BuildTime: "2026-11-13/20:22:32", Commit: "163b93d0", Go: "go1.26", Os: "linux/arm64"}, info)

	info, err = Parse([]byte(`{"level":"info","msg":"starting"}` + "\n" + `{"version":"1.23.0","commit":"abc"}`))
	require.NoError(t, err)
	assert.Equal(t, "1.23.0", info.Version)

	for name, out := range map[string]string{
		"empty":        "  \n",
		"error":        `{"error":"cannot get version"}`,
		"invalid json": `{"version":`,
		"unknown":      "Unknown format: yaml",
	} {
		_, err := Parse([]byte(out))
		assert.Error(t, err, name)
	}
}

func TestNormalize(t *testing.T) {
	assert.Equal(t, "1.22.0", Normalize("v1.22.0_abcdef"))
	assert.Equal(t, "1.22.0-rc.1", Normalize(" 1.22.0-rc.1 "))
	assert.Equal(t, "163b93d0", Normalize("163b93d0"))
}
