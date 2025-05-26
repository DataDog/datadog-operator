// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"errors"
	"fmt"
)

var errMergeAttempted = fmt.Errorf("merge attempted")

// IsMergeAttemptedError returns true if the err is a MergeAttemptedError type
func IsMergeAttemptedError(err error) bool {
	return errors.Is(err, errMergeAttempted)
}
