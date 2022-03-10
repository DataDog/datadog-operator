// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package secrets

import (
	"bytes"
	"fmt"
	"strings"
)

// extractHandles returns handles used to get secrets
// extractHandles returns an error if one element doesn't respect the format ENC[<handle>]
func extractHandles(encData []string) ([]string, error) {
	handles := []string{}
	for _, str := range encData {
		handle, err := extractHandle(str)
		if err != nil {
			return nil, err
		}
		handles = append(handles, handle)
	}

	return handles, nil
}

// IsEnc returns true if a string respects the ENC[<handle>] format
func IsEnc(str string) bool {
	return strings.HasPrefix(str, "ENC[") && strings.HasSuffix(str, "]")
}

// extractHandle returns the handle of a secret
func extractHandle(str string) (string, error) {
	if IsEnc(str) {
		return str[4 : len(str)-1], nil
	}

	return "", fmt.Errorf("wrong format, want ENC[<handle>], got: %s", str)
}

// encFormat puts a given secret handle in a ENC[<handle>] format
func encFormat(str string) string {
	return fmt.Sprintf("ENC[%s]", str)
}

type limitBuffer struct {
	max int
	buf *bytes.Buffer
}

func (b *limitBuffer) Write(p []byte) (n int, err error) {
	if len(p)+b.buf.Len() > b.max {
		return 0, fmt.Errorf("command output was too long: exceeded %d bytes", b.max)
	}

	return b.buf.Write(p)
}
