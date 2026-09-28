// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetExperimentalClusterChecksRunnerGroups reads and decodes the
// AnnotationExperimentalClusterChecksRunnerGroups annotation off obj. It
// returns (nil, nil) when the annotation is absent or empty, so callers don't
// need to special-case the common no-groups case.
func GetExperimentalClusterChecksRunnerGroups(obj metav1.Object) ([]ClusterChecksRunnerGroup, error) {
	raw, ok := obj.GetAnnotations()[AnnotationExperimentalClusterChecksRunnerGroups]
	if !ok || raw == "" {
		return nil, nil
	}

	var groups []ClusterChecksRunnerGroup
	if err := json.Unmarshal([]byte(raw), &groups); err != nil {
		return nil, fmt.Errorf("invalid %s annotation: %w", AnnotationExperimentalClusterChecksRunnerGroups, err)
	}

	return groups, nil
}
