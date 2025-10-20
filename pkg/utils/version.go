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
var majorMinorRx = regexp.MustCompile(`(\d+)\.(\d+)`)

// IsAboveMinVersion uses semver to check if `version` is >= minVersion.
// For versions not containing a semver, it will consider them above minVersion.
func IsAboveMinVersion(version, minVersion string) bool {

	version = formatVersionTag(version)
	v, err := semver.NewVersion(version)
	if err != nil {
		// If the version tag is not a valid semver, return true.
		return true
	}

	c, err := semver.NewConstraint(">= " + minVersion)
	if err != nil {
		return false
	}

	return c.Check(v)
}

// IsAboveMinVersionWithFallback uses semver to check if `version` is >= minVersion.
// If semver parsing fails, it falls back to parsing major.minor versions only.
// For versions that can't be parsed at all, it returns false (conservative approach).
func IsAboveMinVersionWithFallback(version, minVersion string) bool {
	version = formatVersionTag(version)

	// First, try standard semver parsing
	v, err := semver.NewVersion(version)
	if err == nil {
		c, constraintErr := semver.NewConstraint(">= " + minVersion)
		if constraintErr != nil {
			return false
		}
		return c.Check(v)
	}

	// Semver failed, try parsing just major.minor
	// Extract major.minor from version string
	matches := majorMinorRx.FindStringSubmatch(version)
	if len(matches) < 3 {
		// Can't parse version, return false (conservative)
		return false
	}

	versionMajorMinor := matches[1] + "." + matches[2] + ".0"

	// Parse minVersion to get its major.minor
	minMatches := majorMinorRx.FindStringSubmatch(minVersion)
	if len(minMatches) < 3 {
		// Can't parse minVersion, return false
		return false
	}

	minVersionMajorMinor := minMatches[1] + "." + minMatches[2] + ".0"

	// Now compare using semver with the constructed x.y.0 versions
	v, err = semver.NewVersion(versionMajorMinor)
	if err != nil {
		return false
	}

	c, constraintErr := semver.NewConstraint(">= " + minVersionMajorMinor)
	if constraintErr != nil {
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
