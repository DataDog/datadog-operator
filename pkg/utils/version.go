// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package utils

import "github.com/Masterminds/semver/v3"

// IsAboveMinVersion uses semver to check if `version` is >= minVersion
func IsAboveMinVersion(version, minVersion string) bool {
	v, err := semver.NewVersion(version)
	if err != nil {
		return false
	}

	c, err := semver.NewConstraint("^" + minVersion)
	if err != nil {
		return false
	}

	return c.Check(v)
}
