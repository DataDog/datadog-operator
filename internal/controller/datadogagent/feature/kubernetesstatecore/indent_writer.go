// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetesstatecore

import (
	"bytes"
	"io"
)

// indentWriter wraps an io.Writer and adds indentation to each line.
type indentWriter struct {
	w          io.Writer
	indent     []byte
	needIndent bool
}

// newIndentWriter creates an indentWriter with the specified number of spaces.
func newIndentWriter(w io.Writer, spaces int) *indentWriter {
	return &indentWriter{
		w:          w,
		indent:     bytes.Repeat([]byte(" "), spaces),
		needIndent: true,
	}
}

// Write implements io.Writer, adding indentation at the start of each line.
func (iw *indentWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	for _, b := range p {
		if iw.needIndent && b != '\n' {
			if _, err := iw.w.Write(iw.indent); err != nil {
				return 0, err
			}
			iw.needIndent = false
		}

		if _, err := iw.w.Write([]byte{b}); err != nil {
			return 0, err
		}

		if b == '\n' {
			iw.needIndent = true
		}
	}

	return len(p), nil
}
