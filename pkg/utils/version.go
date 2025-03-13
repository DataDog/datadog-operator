// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
	// Special case: version containing "main" or "latest" is always considered above any version
	if strings.Contains(version, "main") || strings.Contains(version, "latest") {
		return true
	}

	version = formatVersionTag(version)
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

// formatVersionTag checks if the version tag uses dashes in lieu of periods, and replaces the first two dashes if so.
func formatVersionTag(versionTag string) string {
	if versionWithDashesRx.FindString(versionTag) != "" {
		versionTag = strings.Replace(versionTag, "-", ".", 2)
	}

	// Return versionTag if it matches with versionRx regex, or "".
	return versionRx.FindString(versionTag)
}
