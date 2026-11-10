// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"sigs.k8s.io/yaml"
)

const (
	// DefaultMaxBytes is the default size limit of a flare: the sum of the
	// scrubbed, uncompressed files. The compressed archive is smaller.
	DefaultMaxBytes int64 = 50 * 1024 * 1024
	// manifestReserve is kept free for the manifest file.
	manifestReserve int64 = 512 * 1024
	// ManifestFile is the name of the manifest file at the root of the archive.
	ManifestFile    = "manifest.json"
	truncatedMarker = "\n[truncated by the Datadog Operator flare: the flare size limit was reached]\n"
)

var (
	// ErrSizeLimit is returned when a file was truncated or dropped because
	// the flare size limit was reached.
	ErrSizeLimit = errors.New("flare size limit reached")
	// ErrArchiveClosed is returned when a collector writes after the archive
	// was closed, for example after its time limit.
	ErrArchiveClosed = errors.New("flare archive already closed")
)

// Archive builds a flare zip archive in memory. Every file is scrubbed before
// it is written, and the sum of the written files is capped. Archive is safe
// for concurrent use.
type Archive struct {
	mu       sync.Mutex
	buf      bytes.Buffer
	zw       *zip.Writer
	root     string
	scrubber *Scrubber
	maxBytes int64
	written  int64
	files    []FileInfo
	paths    map[string]struct{}
	closed   bool
	now      func() time.Time
}

// FileInfo describes a file of the archive in the manifest.
type FileInfo struct {
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	Truncated bool   `json:"truncated,omitempty"`
}

// ArchiveOptions configures an Archive.
type ArchiveOptions struct {
	// Root is the directory of the archive every file is written into.
	Root string
	// MaxBytes caps the sum of the scrubbed files. DefaultMaxBytes if 0.
	MaxBytes int64
	// Scrubber scrubs every file. NewScrubber() if nil.
	Scrubber *Scrubber
	// Now returns the file modification time. time.Now if nil.
	Now func() time.Time
}

// NewArchive returns an empty Archive.
func NewArchive(opts ArchiveOptions) *Archive {
	a := &Archive{
		root:     strings.Trim(opts.Root, "/"),
		scrubber: opts.Scrubber,
		maxBytes: opts.MaxBytes,
		paths:    map[string]struct{}{},
		now:      opts.Now,
	}
	if a.scrubber == nil {
		a.scrubber = NewScrubber()
	}
	if a.maxBytes <= 0 {
		a.maxBytes = DefaultMaxBytes
	}
	if a.now == nil {
		a.now = time.Now
	}
	a.zw = zip.NewWriter(&a.buf)
	return a
}

// Scrubber returns the scrubber of the archive.
func (a *Archive) Scrubber() *Scrubber {
	return a.scrubber
}

// AddText scrubs data and writes it to name. If the size limit is reached,
// the file is truncated and ErrSizeLimit is returned.
func (a *Archive) AddText(name string, data []byte) error {
	return a.write(name, a.scrubber.ScrubBytes(data), manifestReserve)
}

// AddYAML redacts and scrubs obj and writes it as YAML to name.
func (a *Archive) AddYAML(name string, obj any) error {
	redactedObj, err := RedactObject(obj)
	if err != nil {
		return fmt.Errorf("unable to serialize %s: %w", name, err)
	}
	data, err := yaml.Marshal(redactedObj)
	if err != nil {
		return fmt.Errorf("unable to serialize %s: %w", name, err)
	}
	return a.AddText(name, data)
}

// AddJSON redacts and scrubs obj and writes it as indented JSON to name.
func (a *Archive) AddJSON(name string, obj any) error {
	redactedObj, err := RedactObject(obj)
	if err != nil {
		return fmt.Errorf("unable to serialize %s: %w", name, err)
	}
	data, err := json.MarshalIndent(redactedObj, "", "  ")
	if err != nil {
		return fmt.Errorf("unable to serialize %s: %w", name, err)
	}
	return a.AddText(name, data)
}

// Remaining returns how many bytes collectors can still write.
func (a *Archive) Remaining() int64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return max(a.maxBytes-manifestReserve-a.written, 0)
}

// Files returns the files written so far.
func (a *Archive) Files() []FileInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]FileInfo(nil), a.files...)
}

func (a *Archive) write(name string, data []byte, reserve int64) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return ErrArchiveClosed
	}

	name = a.uniquePath(name)
	remaining := a.maxBytes - reserve - a.written
	var sizeErr error
	truncated := false
	if int64(len(data)) > remaining {
		sizeErr = fmt.Errorf("%w: %s was truncated", ErrSizeLimit, name)
		if remaining <= int64(len(truncatedMarker)) {
			return fmt.Errorf("%w: %s was not written", ErrSizeLimit, name)
		}
		data = append(data[:remaining-int64(len(truncatedMarker))], truncatedMarker...)
		truncated = true
	}

	w, err := a.zw.CreateHeader(&zip.FileHeader{
		Name:     path.Join(a.root, name),
		Method:   zip.Deflate,
		Modified: a.now(),
	})
	if err != nil {
		return err
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	a.written += int64(len(data))
	a.files = append(a.files, FileInfo{Path: name, Size: int64(len(data)), Truncated: truncated})
	return sizeErr
}

// uniquePath returns a clean, relative path that is not used yet.
func (a *Archive) uniquePath(name string) string {
	name = strings.TrimLeft(path.Clean("/"+name), "/")
	candidate := name
	for i := 1; ; i++ {
		if _, used := a.paths[candidate]; !used {
			a.paths[candidate] = struct{}{}
			return candidate
		}
		ext := path.Ext(name)
		candidate = fmt.Sprintf("%s-%d%s", strings.TrimSuffix(name, ext), i, ext)
	}
}

// Close writes the manifest and returns the zip archive. Writes after Close
// fail with ErrArchiveClosed.
func (a *Archive) Close(manifest *Manifest) ([]byte, error) {
	if manifest != nil {
		a.mu.Lock()
		manifest.Files = append([]FileInfo(nil), a.files...)
		sort.Slice(manifest.Files, func(i, j int) bool { return manifest.Files[i].Path < manifest.Files[j].Path })
		manifest.TotalBytes = a.written
		manifest.MaxBytes = a.maxBytes
		a.mu.Unlock()

		data, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return nil, err
		}
		// The manifest has its own reserved space.
		if err := a.write(ManifestFile, a.scrubber.ScrubBytes(data), 0); err != nil {
			return nil, err
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return nil, ErrArchiveClosed
	}
	a.closed = true
	if err := a.zw.Close(); err != nil {
		return nil, err
	}
	return a.buf.Bytes(), nil
}
