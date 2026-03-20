// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package experiment

import "time"

const (
	// DefaultExperimentTimeout is the default duration before an unacknowledged experiment auto-rolls back.
	DefaultExperimentTimeout = 30 * time.Minute

	// RevisionHashLength is the number of hex characters used from the MD5 hash for revision naming.
	RevisionHashLength = 10
)
