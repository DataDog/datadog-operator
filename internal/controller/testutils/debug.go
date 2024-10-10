// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutils

import (
	"encoding/json"
	"fmt"
	"io"
)

// DebugPrint prints a JSON representation of a compatible obj to given writer
func DebugPrint(writer io.Writer, obj interface{}) {
	marshaled, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		fmt.Fprint(writer, err)
	}
	fmt.Fprintln(writer, string(marshaled))
}
