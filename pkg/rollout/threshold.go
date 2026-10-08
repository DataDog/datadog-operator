// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rollout

import (
	"fmt"
	"strconv"
	"strings"
)

// MaxUnavailable is a runtime availability threshold: a count of pods, or a
// percentage of the desired pods. The zero value tolerates no unavailable
// pod. It is unrelated to the workload's rollingUpdate.maxUnavailable,
// which is a rollout budget.
type MaxUnavailable struct {
	Value int32
	// Percent is true when Value is a percentage, in [0, 100].
	Percent bool
}

// ParseMaxUnavailable parses a non-negative integer ("2") or a percentage
// from 0 to 100 ("5%").
func ParseMaxUnavailable(s string) (MaxUnavailable, error) {
	s = strings.TrimSpace(s)
	num, percent := strings.CutSuffix(s, "%")
	n, err := strconv.ParseInt(num, 10, 32)
	if err != nil || n < 0 || (percent && n > 100) {
		return MaxUnavailable{}, fmt.Errorf("%q is not a non-negative integer or a percentage from 0%% to 100%%", s)
	}
	return MaxUnavailable{Value: int32(n), Percent: percent}, nil
}

// String formats m the way ParseMaxUnavailable reads it.
func (m MaxUnavailable) String() string {
	if m.Percent {
		return strconv.Itoa(int(m.Value)) + "%"
	}
	return strconv.Itoa(int(m.Value))
}

// Resolve returns the number of unavailable pods tolerated out of desired:
// a percentage is rounded down.
func (m MaxUnavailable) Resolve(desired int32) int32 {
	if !m.Percent {
		return max(m.Value, 0)
	}
	return int32(int64(max(desired, 0)) * int64(min(max(m.Value, 0), 100)) / 100)
}
