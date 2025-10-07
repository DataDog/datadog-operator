// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import "slices"

const requiredTag = "generated:kubernetes"

func GetRequiredTags() []string {
	return []string{requiredTag}
}

func GetTagsToAdd(tags []string) []string {
	tagsToAdd := []string{}
	var found bool
	for _, rT := range GetRequiredTags() {
		found = slices.Contains(tags, rT)
		if !found {
			tagsToAdd = append(tagsToAdd, rT)
		}
	}
	return tagsToAdd
}
