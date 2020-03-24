// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package flare

import (
	"io"
	"os"
)

// redactingWriter is a writer that will redact content before writing to target
type redactingWriter struct {
	target *os.File
	r      replacer
}

// newRedactingWriter instantiates a redactingWriter to target
func newRedactingWriter(target *os.File) *redactingWriter {
	return &redactingWriter{
		target: target,
		r:      replacer{},
	}
}

// Write writes the redacted byte stream, applying all replacers and credential cleanup to target
func (f *redactingWriter) Write(p []byte) (int, error) {
	cleaned, err := credentialsCleanerBytes(p)
	if err != nil {
		return 0, err
	}

	if f.r.regex != nil && f.r.replFunc != nil {
		cleaned = f.r.regex.ReplaceAllFunc(cleaned, f.r.replFunc)
	}

	n, err := f.target.Write(cleaned)
	if n != len(cleaned) {
		err = io.ErrShortWrite
	}

	return len(p), err
}
