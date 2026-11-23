// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"archive/zip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArchiveDir(t *testing.T) {
	src := filepath.Join(t.TempDir(), "datadog-operator")
	require.NoError(t, os.MkdirAll(filepath.Join(src, "sub"), 0755))

	files := map[string]string{
		"already-compressed.zip":        "existing ZIP bytes",
		"datadog-custom-resources.yaml": "apiVersion: datadoghq.com/v2alpha1\n",
		"pod-abc.json":                  `{"msg":"hello"}`,
		"sub/nested.txt":                "nested\n",
	}
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(src, filepath.FromSlash(name)), []byte(content), 0644))
	}

	destination := filepath.Join(t.TempDir(), "flare.zip")
	require.NoError(t, archiveDir(t.Context(), src, destination))

	reader, err := zip.OpenReader(destination)
	require.NoError(t, err)
	defer reader.Close()

	expectedNames := []string{
		"",
		"already-compressed.zip",
		"datadog-custom-resources.yaml",
		"pod-abc.json",
		"sub/",
		"sub/nested.txt",
	}
	require.Len(t, reader.File, len(expectedNames))
	for i, entry := range reader.File {
		name := expectedNames[i]
		assert.Equal(t, "datadog-operator/"+name, entry.Name)
		want, isFile := files[name]
		assert.Equal(t, !isFile, entry.FileInfo().IsDir(), entry.Name)
		if !isFile {
			assert.Equal(t, uint16(zip.Store), entry.Method, entry.Name)
			continue
		}

		method := uint16(zip.Deflate)
		if name == "already-compressed.zip" {
			method = zip.Store
		}
		assert.Equal(t, method, entry.Method, entry.Name)
		content, err := entry.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(content)
		require.NoError(t, err)
		require.NoError(t, content.Close())
		assert.Equal(t, want, string(data), entry.Name)
	}
}

func TestArchiveDirSkipsNonRegularFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks needs elevation or Developer Mode on Windows")
	}

	src := filepath.Join(t.TempDir(), "datadog-operator")
	require.NoError(t, os.MkdirAll(src, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "pod-abc.json"), []byte(`{"msg":"hello"}`), 0644))
	require.NoError(t, os.Symlink(filepath.Join(src, "pod-abc.json"), filepath.Join(src, "link.json")))

	destination := filepath.Join(t.TempDir(), "flare.zip")
	require.NoError(t, archiveDir(t.Context(), src, destination))

	reader, err := zip.OpenReader(destination)
	require.NoError(t, err)
	defer reader.Close()

	names := []string{}
	for _, entry := range reader.File {
		names = append(names, entry.Name)
	}

	assert.Equal(t, []string{"datadog-operator/", "datadog-operator/pod-abc.json"}, names)
}

func TestArchiveDirKeepsExistingArchive(t *testing.T) {
	src := filepath.Join(t.TempDir(), "datadog-operator")
	require.NoError(t, os.MkdirAll(src, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "pod-abc.json"), []byte(`{"msg":"hello"}`), 0644))

	destination := filepath.Join(t.TempDir(), "flare.zip")
	existing := []byte("archive from a concurrent flare")
	require.NoError(t, os.WriteFile(destination, existing, 0644))

	assert.ErrorIs(t, archiveDir(t.Context(), src, destination), os.ErrExist)

	kept, err := os.ReadFile(destination)
	require.NoError(t, err)
	assert.Equal(t, existing, kept)
}

func TestArchiveDirMissingSource(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "flare.zip")

	assert.ErrorIs(t, archiveDir(t.Context(), filepath.Join(t.TempDir(), "absent"), destination), os.ErrNotExist)

	_, err := os.Stat(destination)
	assert.True(t, os.IsNotExist(err), "a failed archive must not be left behind")
}

func TestArchiveDirUnreadableFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		// Geteuid also reports -1 there, so the check below would not catch it.
		t.Skip("permission bits do not make a file unreadable on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root reads files regardless of their permission bits")
	}

	src := filepath.Join(t.TempDir(), "datadog-operator")
	require.NoError(t, os.MkdirAll(src, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "unreadable.txt"), []byte("collected"), 0000))

	destination := filepath.Join(t.TempDir(), "flare.zip")

	assert.Error(t, archiveDir(t.Context(), src, destination))

	_, err := os.Stat(destination)
	assert.True(t, os.IsNotExist(err), "a failed archive must not be left behind")
}

func TestWriteFlareZIPReportsFinalizationFailure(t *testing.T) {
	writeErr := errors.New("cannot write ZIP central directory")
	assert.ErrorIs(t, writeFlareZIP(t.Context(), failingZIPWriter{writeErr}, nil), writeErr)
}

type failingZIPWriter struct {
	err error
}

func (w failingZIPWriter) Write([]byte) (int, error) {
	return 0, w.err
}
