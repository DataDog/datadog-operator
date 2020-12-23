// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package datadogmonitor

func getMax(val1, val2 int64) int64 {
	if val1 >= val2 {
		return val1
	}
	return val2
}
