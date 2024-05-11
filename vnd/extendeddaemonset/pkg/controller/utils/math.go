// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package utils

// MaxInt returns the larger of x or y.
func MaxInt(val0 int, vals ...int) int {
	max := val0
	for _, val := range vals {
		if val > max {
			max = val
		}
	}

	return max
}

// MinInt returns the larger of x or y.
func MinInt(val0 int, vals ...int) int {
	min := val0
	for _, val := range vals {
		if val < min {
			min = val
		}
	}

	return min
}
