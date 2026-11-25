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

type parsedSemver struct {
	major         uint64
	minor         uint64
	patch         uint64
	hasPrerelease bool
	prerelease    string
}

func hasASCIIDigit(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			return true
		}
	}
	return false
}

// parseUint scans a non-empty run of ASCII digits without leading zeros (except "0").
func parseUint(s string, pos int) (val uint64, next int, ok bool) {
	start := pos
	if pos >= len(s) || s[pos] < '0' || s[pos] > '9' {
		return 0, pos, false
	}
	if s[pos] == '0' && pos+1 < len(s) && s[pos+1] >= '0' && s[pos+1] <= '9' {
		// Reject leading zeros on the fast path so semver fallback decides.
		return 0, pos, false
	}
	for pos < len(s) && s[pos] >= '0' && s[pos] <= '9' {
		d := uint64(s[pos] - '0')
		if val > (1<<63-1-d)/10 {
			return 0, start, false
		}
		val = val*10 + d
		pos++
	}
	return val, pos, pos > start
}

// isValidSemverMetadata checks dot-separated SemVer prerelease or build identifiers.
func isValidSemverIdentList(s string, disallowNumericLeadingZero bool) bool {
	if len(s) == 0 {
		return false
	}
	i := 0
	for i < len(s) {
		start := i
		allDigits := true
		for i < len(s) && s[i] != '.' {
			c := s[i]
			isDigit := c >= '0' && c <= '9'
			isAlpha := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
			if !isDigit && !isAlpha && c != '-' {
				return false
			}
			if !isDigit {
				allDigits = false
			}
			i++
		}
		segLen := i - start
		if segLen == 0 {
			return false
		}
		if disallowNumericLeadingZero && allDigits && segLen > 1 && s[start] == '0' {
			return false
		}
		if i < len(s) {
			i++ // skip '.'
			if i == len(s) {
				return false
			}
		}
	}
	return true
}

// parseFastVersion parses common version tags (e.g. "7.78.0", "7.78.0-jmx", "7-78-0",
// "v1.31.2-gke.100") without regexes or heap allocations. Returns ok=false for unusual
// inputs so the caller falls back to formatVersionTag + Masterminds/semver.
func parseFastVersion(s string) (parsedSemver, bool) {
	pos := 0
	if len(s) > 0 && (s[0] == 'v' || s[0] == 'V') {
		pos = 1
	}
	major, pos, ok := parseUint(s, pos)
	if !ok || pos >= len(s) {
		return parsedSemver{}, false
	}
	sep := s[pos]
	if sep != '.' && sep != '-' {
		return parsedSemver{}, false
	}
	pos++
	minor, pos, ok := parseUint(s, pos)
	if !ok || pos >= len(s) || s[pos] != sep {
		return parsedSemver{}, false
	}
	pos++
	patch, pos, ok := parseUint(s, pos)
	if !ok {
		return parsedSemver{}, false
	}

	var pre string
	hasPre := false
	if pos < len(s) {
		if s[pos] == '-' {
			pos++
			preStart := pos
			for pos < len(s) && s[pos] != '+' {
				pos++
			}
			pre = s[preStart:pos]
			// If separator was '.' and prerelease contains a dash-separated triple (d-d-d),
			// fall back to formatVersionTag to match versionWithDashesRx exactly.
			if sep == '.' && containsDashTriple(pre) {
				return parsedSemver{}, false
			}
			if !isValidSemverIdentList(pre, true) {
				return parsedSemver{}, false
			}
			hasPre = true
		}
		if pos < len(s) {
			if s[pos] != '+' {
				return parsedSemver{}, false
			}
			if !isValidSemverIdentList(s[pos+1:], false) {
				return parsedSemver{}, false
			}
		}
	}

	return parsedSemver{
		major:         major,
		minor:         minor,
		patch:         patch,
		hasPrerelease: hasPre,
		prerelease:    pre,
	}, true
}

func containsDashTriple(s string) bool {
	// Quick check: need at least 2 dashes for d-d-d.
	_, rest, ok := strings.Cut(s, "-")
	return ok && strings.IndexByte(rest, '-') >= 0
}

// parseFastMinVersion parses standard minimum version bounds (e.g. "7.76.0-0",
// "7.26.0-rc.5", "7.50.0", "1.22-0") without compiling a semver.Constraints regex.
func parseFastMinVersion(s string) (parsedSemver, bool) {
	pos := 0
	if len(s) > 0 && (s[0] == 'v' || s[0] == 'V') {
		pos = 1
	}
	major, pos, ok := parseUint(s, pos)
	if !ok || pos >= len(s) || s[pos] != '.' {
		return parsedSemver{}, false
	}
	pos++
	minor, pos, ok := parseUint(s, pos)
	if !ok {
		return parsedSemver{}, false
	}
	var patch uint64
	if pos < len(s) && s[pos] == '.' {
		pos++
		patch, pos, ok = parseUint(s, pos)
		if !ok {
			return parsedSemver{}, false
		}
	}

	var pre string
	hasPre := false
	if pos < len(s) {
		if s[pos] == '-' {
			pos++
			preStart := pos
			for pos < len(s) && s[pos] != '+' {
				pos++
			}
			pre = s[preStart:pos]
			if !isValidSemverIdentList(pre, true) {
				return parsedSemver{}, false
			}
			hasPre = true
		}
		if pos < len(s) {
			if s[pos] != '+' {
				return parsedSemver{}, false
			}
			if !isValidSemverIdentList(s[pos+1:], false) {
				return parsedSemver{}, false
			}
		}
	}

	return parsedSemver{
		major:         major,
		minor:         minor,
		patch:         patch,
		hasPrerelease: hasPre,
		prerelease:    pre,
	}, true
}

// IsAboveMinVersion uses semver to check if `version` is >= minVersion.
// The defaultValue parameter controls the behavior when version parsing fails:
//   - nil: uses the default behavior (returns true for unparseable versions)
//   - true: returns true for unparseable versions
//   - false: returns false for unparseable versions
func IsAboveMinVersion(version, minVersion string, defaultValue *bool) bool {
	// Determine the fallback value
	fallback := true
	if defaultValue != nil {
		fallback = *defaultValue
	}

	// Fast reject tags with no digits (e.g. "latest", "main", "foobar").
	if !hasASCIIDigit(version) {
		return fallback
	}

	// Stateless fast path avoiding regexes and semver.NewConstraint allocations.
	if v, ok := parseFastVersion(version); ok {
		if m, ok := parseFastMinVersion(minVersion); ok {
			// In Masterminds/semver, constraints without a prerelease never match prereleases.
			if v.hasPrerelease && !m.hasPrerelease {
				return false
			}
			if v.major != m.major {
				return v.major > m.major
			}
			if v.minor != m.minor {
				return v.minor > m.minor
			}
			if v.patch != m.patch {
				return v.patch > m.patch
			}
			if !v.hasPrerelease {
				return true
			}
			if m.prerelease == "0" || v.prerelease == m.prerelease {
				return true
			}
		}
	}

	version = formatVersionTag(version)
	v, err := semver.NewVersion(version)
	if err != nil {
		// If the version tag is not a valid semver, return fallback value.
		return fallback
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
