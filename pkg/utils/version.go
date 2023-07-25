// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package utils

import (
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
)

var versionRx = regexp.MustCompile(`(\d+\.\d+\.\d+)(\-[^\+]+)*(\+.+)*`)
var versionWithDashesRx = regexp.MustCompile(`(\d+\-\d+\-\d+)(\-[^\+]+)*(\+.+)*`)

// IsAboveMinVersion uses semver to check if `version` is >= minVersion
func IsAboveMinVersion(version, minVersion string) bool {
	v, err := semver.NewVersion(version)
	if err != nil {
		return false
	}

	c, err := semver.NewConstraint(">= " + minVersion)
	if err != nil {
		return false
	}

	return c.Check(v)
}

func FormatVersionTag(versionTag string) string {
	// Check if version tag uses dashes in lieu of periods. If so, then replace first two dashes with periods to convert to expected format.
	if versionWithDashesRx.FindString(versionTag) != "" {
		versionTag = strings.Replace(versionTag, "-", ".", 2)
	}

	// Return versionTag if it matches with versionRx regex, or "".
	return versionRx.FindString(versionTag)
}
