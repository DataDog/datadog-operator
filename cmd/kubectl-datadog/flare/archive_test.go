// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArchiveDir(t *testing.T) {
	src := filepath.Join(t.TempDir(), "datadog-operator")
	require.NoError(t, os.MkdirAll(filepath.Join(src, "sub"), 0755))

	files := map[string]string{
		"datadog-custom-resources.yaml": "apiVersion: datadoghq.com/v2alpha1\n",
		"pod-abc.json":                  `{"msg":"hello"}`,
		"sub/nested.txt":                "nested\n",
	}
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(src, filepath.FromSlash(name)), []byte(content), 0644))
	}
	require.NoError(t, os.Symlink(filepath.Join(src, "pod-abc.json"), filepath.Join(src, "link.json")))

	destination := filepath.Join(t.TempDir(), "flare.zip")
	require.NoError(t, archiveDir(src, destination))

	reader, err := zip.OpenReader(destination)
	require.NoError(t, err)
	defer reader.Close()

	got := map[string]string{}
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			got[entry.Name] = ""
			continue
		}

		content, err := entry.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(content)
		require.NoError(t, err)
		require.NoError(t, content.Close())
		got[entry.Name] = string(data)
	}

	assert.Equal(t, map[string]string{
		"datadog-operator/": "",
		"datadog-operator/datadog-custom-resources.yaml": "apiVersion: datadoghq.com/v2alpha1\n",
		"datadog-operator/pod-abc.json":                  `{"msg":"hello"}`,
		"datadog-operator/sub/":                          "",
		"datadog-operator/sub/nested.txt":                "nested\n",
	}, got)
}

func TestArchiveDirKeepsExistingArchive(t *testing.T) {
	src := filepath.Join(t.TempDir(), "datadog-operator")
	require.NoError(t, os.MkdirAll(src, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "pod-abc.json"), []byte(`{"msg":"hello"}`), 0644))

	destination := filepath.Join(t.TempDir(), "flare.zip")
	existing := []byte("archive from a concurrent flare")
	require.NoError(t, os.WriteFile(destination, existing, 0644))

	assert.Error(t, archiveDir(src, destination))

	kept, err := os.ReadFile(destination)
	require.NoError(t, err)
	assert.Equal(t, existing, kept)
}

func TestArchiveDirMissingSource(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "flare.zip")

	assert.Error(t, archiveDir(filepath.Join(t.TempDir(), "absent"), destination))

	_, err := os.Stat(destination)
	assert.True(t, os.IsNotExist(err), "a failed archive must not be left behind")
}

func TestArchiveDirUnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads files regardless of their permission bits")
	}

	src := filepath.Join(t.TempDir(), "datadog-operator")
	require.NoError(t, os.MkdirAll(src, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "unreadable.txt"), []byte("collected"), 0000))

	destination := filepath.Join(t.TempDir(), "flare.zip")

	assert.Error(t, archiveDir(src, destination))

	_, err := os.Stat(destination)
	assert.True(t, os.IsNotExist(err), "a failed archive must not be left behind")
}
