// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package utils

import "strings"

// GetMax returns the larger of two int64 values
func GetMax(val1, val2 int64) int64 {
	if val1 >= val2 {
		return val1
	}
	return val2
}

// GetTagFromImageName returns a tag from a full image name
// it should cover most cases
func GetTagFromImageName(imageName string) string {
	parts := strings.Split(imageName, ":")
	if len(parts) <= 1 {
		return "latest"
	}

	// / are not allowed in tags, we probably caught a :port/<> without tag
	tag := parts[len(parts)-1]
	if strings.Contains(tag, "/") {
		return "latest"
	}

	return tag
}
