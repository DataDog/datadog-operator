// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v2alpha1

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

// GetExperimentalClusterChecksRunnerGroups decodes the
// AnnotationExperimentalClusterChecksRunnerGroups annotation.
// Returns (nil, nil) when absent or empty.
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

// KubeChecksRunnerGroupName is the reserved name of the built-in kube
// runner group. A user-declared group with this name replaces the built-in.
const KubeChecksRunnerGroupName = "kube"

// KubeChecksRunnerGroupChecksInclude is the list of kube-family checks the
// built-in group claims. Single source of truth; do not mutate.
var KubeChecksRunnerGroupChecksInclude = []string{
	"kubernetes_state_core",
	"orchestrator",
	"kube_apiserver_metrics",
	"kube_controller_manager",
	"kube_scheduler",
}

// defaultKubeChecksRunnerGroupReplicas is the built-in group's default
// replica count: 2 is the HA floor (failover capacity, not throughput).
const defaultKubeChecksRunnerGroupReplicas int32 = 2

// IsExperimentalKubeChecksRunnerDefaultEnabled reports whether the
// AnnotationExperimentalKubeChecksRunnerDefault annotation is a strict
// boolean "true". Absent, empty or unparseable means disabled.
func IsExperimentalKubeChecksRunnerDefaultEnabled(obj metav1.Object) bool {
	raw, ok := obj.GetAnnotations()[AnnotationExperimentalKubeChecksRunnerDefault]
	if !ok || raw == "" {
		return false
	}

	enabled, err := strconv.ParseBool(raw)
	if err != nil {
		return false
	}

	return enabled
}

// GetEffectiveClusterChecksRunnerGroups returns the groups to materialize:
// the annotation groups plus the built-in kube group when the knob is on
// (skipped when the user declared their own "kube" group). Shared by the
// clusterchecks feature and the DDAI reconciler so both agree.
func GetEffectiveClusterChecksRunnerGroups(obj metav1.Object) ([]ClusterChecksRunnerGroup, error) {
	groups, err := GetExperimentalClusterChecksRunnerGroups(obj)
	if err != nil {
		return nil, err
	}

	if !IsExperimentalKubeChecksRunnerDefaultEnabled(obj) {
		return groups, nil
	}

	// "kube" is reserved: a user-declared group takes over the built-in.
	for _, group := range groups {
		if group.Name == KubeChecksRunnerGroupName {
			return groups, nil
		}
	}

	kubeGroup := ClusterChecksRunnerGroup{
		Name:          KubeChecksRunnerGroupName,
		ChecksInclude: slices.Clone(KubeChecksRunnerGroupChecksInclude),
		Override: &DatadogAgentComponentOverride{
			Replicas: ptr.To(defaultKubeChecksRunnerGroupReplicas),
		},
	}

	return append([]ClusterChecksRunnerGroup{kubeGroup}, groups...), nil
}
