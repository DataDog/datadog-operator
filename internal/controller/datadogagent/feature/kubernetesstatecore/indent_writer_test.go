// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetesstatecore

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIndentWriter(t *testing.T) {
	var buf bytes.Buffer
	w := newIndentWriter(&buf, 4)

	n, err := w.Write([]byte("first\nsecond\n"))
	require.NoError(t, err)
	assert.Equal(t, len("first\nsecond\n"), n)
	assert.Equal(t, "    first\n    second\n", buf.String())
}

func TestIndentWriterMultipleWrites(t *testing.T) {
	var buf bytes.Buffer
	w := newIndentWriter(&buf, 2)

	for _, value := range []string{"first", " line\n", "second"} {
		_, err := w.Write([]byte(value))
		require.NoError(t, err)
	}

	assert.Equal(t, "  first line\n  second", buf.String())
}
