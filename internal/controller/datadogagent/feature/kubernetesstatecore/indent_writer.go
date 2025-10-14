// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetesstatecore

import (
	"bytes"
	"io"
)

// indentWriter wraps an io.Writer and adds indentation to each line
type indentWriter struct {
	w          io.Writer
	indent     []byte
	needIndent bool
}

// newIndentWriter creates a new indentWriter with the specified number of spaces for indentation
func newIndentWriter(w io.Writer, spaces int) *indentWriter {
	return &indentWriter{
		w:          w,
		indent:     bytes.Repeat([]byte(" "), spaces),
		needIndent: true,
	}
}

// Write implements io.Writer interface, adding indentation at the start of each line
func (iw *indentWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	for _, b := range p {
		if iw.needIndent && b != '\n' {
			_, err := iw.w.Write(iw.indent)
			if err != nil {
				return 0, err
			}
			iw.needIndent = false
		}

		_, err := iw.w.Write([]byte{b})
		if err != nil {
			return 0, err
		}

		if b == '\n' {
			iw.needIndent = true
		}
	}

	// Return the original byte count to satisfy the io.Writer contract
	return len(p), nil
}
