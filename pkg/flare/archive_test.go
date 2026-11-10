// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArchiveSizeLimit(t *testing.T) {
	maxBytes := manifestReserve + 1000
	a := NewArchive(ArchiveOptions{Root: "flare", MaxBytes: maxBytes})

	require.NoError(t, a.AddText("small.txt", []byte(strings.Repeat("a", 400))))
	err := a.AddText("big.txt", []byte(strings.Repeat("b", 2000)))
	require.ErrorIs(t, err, ErrSizeLimit)
	assert.Equal(t, int64(0), a.Remaining())
	err = a.AddText("dropped.txt", []byte("c"))
	require.ErrorIs(t, err, ErrSizeLimit)

	manifest := &Manifest{}
	data, err := a.Close(manifest)
	require.NoError(t, err)
	files := readZip(t, data)

	assert.Len(t, files["flare/small.txt"], 400)
	assert.Len(t, files["flare/big.txt"], 600)
	assert.True(t, strings.HasSuffix(files["flare/big.txt"], truncatedMarker))
	assert.NotContains(t, files, "flare/dropped.txt")
	require.Contains(t, files, "flare/"+ManifestFile)

	var written Manifest
	require.NoError(t, json.Unmarshal([]byte(files["flare/"+ManifestFile]), &written))
	assert.Equal(t, int64(1000), written.TotalBytes)
	assert.Equal(t, maxBytes, written.MaxBytes)
	assert.Equal(t, []FileInfo{{Path: "big.txt", Size: 600, Truncated: true}, {Path: "small.txt", Size: 400}}, written.Files)
}

func TestArchiveScrubsAndDeduplicates(t *testing.T) {
	a := NewArchive(ArchiveOptions{})
	require.NoError(t, a.AddText("../escape.txt", []byte("api_key: "+testAPIKey)))
	require.NoError(t, a.AddText("escape.txt", []byte("second")))
	data, err := a.Close(nil)
	require.NoError(t, err)
	files := readZip(t, data)
	assert.Contains(t, files, "escape.txt")
	assert.Contains(t, files, "escape-1.txt")
	assertNoSecrets(t, files, testAPIKey)

	require.ErrorIs(t, a.AddText("late.txt", nil), ErrArchiveClosed)
	_, err = a.Close(nil)
	require.ErrorIs(t, err, ErrArchiveClosed)
}

func TestRunStatuses(t *testing.T) {
	collectors := []Collector{
		{Name: "ok", Collect: func(_ context.Context, c *Collection) error {
			return c.AddText("ok.txt", []byte("ok"))
		}},
		{Name: "partial", Collect: func(_ context.Context, c *Collection) error {
			c.Warn(errors.New("invalid API key " + testAPIKey + " for url https://user:" + testPass + "@proxy"))
			return c.AddText("partial.txt", []byte("partial"))
		}},
		{Name: "error", Collect: func(context.Context, *Collection) error {
			return errors.New("boom")
		}},
		{Name: "panic", Collect: func(context.Context, *Collection) error {
			panic("unexpected")
		}},
		{Name: "slow", Timeout: 50 * time.Millisecond, Collect: func(ctx context.Context, c *Collection) error {
			<-ctx.Done()
			return ctx.Err()
		}},
		{Name: "after-slow", Collect: func(_ context.Context, c *Collection) error {
			return c.AddText("after.txt", []byte("still collected"))
		}},
	}
	files, manifest := runCollectors(t, Env{}, collectors...)

	statuses := map[string]string{}
	for _, result := range manifest.Collectors {
		statuses[result.Name] = result.Status
	}
	assert.Equal(t, map[string]string{
		"ok":         StatusOK,
		"partial":    StatusPartial,
		"error":      StatusError,
		"panic":      StatusError,
		"slow":       StatusTimeout,
		"after-slow": StatusOK,
	}, statuses)
	assert.False(t, manifest.Complete)
	assert.Equal(t, []string{"partial", "error", "panic", "slow"}, manifest.FailedCollector)
	assert.Equal(t, "still collected", files["after.txt"])
	assert.Contains(t, files[ManifestFile], `"failed_collectors"`)
	// Errors recorded in the manifest are scrubbed too.
	assertNoSecrets(t, files, testAPIKey, testPass)
}

func TestRunTimeoutSkipsRemainingCollectors(t *testing.T) {
	started := make(chan struct{})
	collectors := []Collector{
		{Name: "hangs", Timeout: time.Hour, Collect: func(ctx context.Context, c *Collection) error {
			close(started)
			<-ctx.Done()
			// Writes after the time limit don't break the archive.
			time.Sleep(10 * time.Millisecond)
			return c.AddText("late.txt", []byte("late"))
		}},
		{Name: "never-started", Collect: func(_ context.Context, c *Collection) error {
			return c.AddText("never.txt", []byte("never"))
		}},
	}
	archive := NewArchive(ArchiveOptions{})
	start := time.Now()
	manifest := Run(context.Background(), Env{}, archive, collectors, RunOptions{Timeout: 100 * time.Millisecond})
	<-started
	assert.Less(t, time.Since(start), 5*time.Second)
	require.Len(t, manifest.Collectors, 2)
	assert.Equal(t, StatusTimeout, manifest.Collectors[0].Status)
	assert.Equal(t, StatusSkipped, manifest.Collectors[1].Status)
	assert.False(t, manifest.Complete)

	data, err := archive.Close(manifest)
	require.NoError(t, err)
	files := readZip(t, data)
	assert.NotContains(t, files, "never.txt")
	assert.Contains(t, files, ManifestFile)
}
