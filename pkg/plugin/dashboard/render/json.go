// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package render

import (
	"encoding/json"
	"io"

	"github.com/DataDog/datadog-operator/pkg/plugin/dashboard"
)

// JSON writes the View as indented JSON followed by a newline.
// Messages are never truncated.
func JSON(w io.Writer, v dashboard.View) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
