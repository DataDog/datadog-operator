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
)

// ClusterChecksRunnerGroup is a dedicated Cluster Checks Runner Deployment that runs only the checks it claims.
type ClusterChecksRunnerGroup struct {
	Name          string   `json:"name"`
	ChecksInclude []string `json:"checksInclude"`
	// +optional
	Override *DatadogAgentComponentOverride `json:"override,omitempty"`
}

// KubeChecksRunnerGroupName is the built-in kube group's name; a user group with this name replaces it.
const KubeChecksRunnerGroupName = "kube"

// KubeChecksRunnerGroupChecksInclude lists the checks the built-in kube group claims.
var KubeChecksRunnerGroupChecksInclude = []string{
	"kubernetes_state_core",
	"orchestrator",
}

const defaultKubeChecksRunnerGroupReplicas int32 = 2

// IsExperimentalKubeChecksRunnerDefaultEnabled reports whether the built-in kube group is enabled.
func IsExperimentalKubeChecksRunnerDefaultEnabled(obj metav1.Object) bool {
	enabled, _ := strconv.ParseBool(obj.GetAnnotations()[AnnotationExperimentalKubeChecksRunnerDefault])
	return enabled
}

// GetEffectiveClusterChecksRunnerGroups returns the validated annotation groups, plus the built-in kube group when enabled.
func GetEffectiveClusterChecksRunnerGroups(obj metav1.Object) ([]ClusterChecksRunnerGroup, error) {
	var groups []ClusterChecksRunnerGroup
	if raw := obj.GetAnnotations()[AnnotationExperimentalClusterChecksRunnerGroups]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &groups); err != nil {
			return nil, fmt.Errorf("invalid %s annotation: %w", AnnotationExperimentalClusterChecksRunnerGroups, err)
		}
	}

	userKubeGroup := slices.ContainsFunc(groups, func(g ClusterChecksRunnerGroup) bool { return g.Name == KubeChecksRunnerGroupName })
	if IsExperimentalKubeChecksRunnerDefaultEnabled(obj) && !userKubeGroup {
		kubeGroup := ClusterChecksRunnerGroup{
			Name:          KubeChecksRunnerGroupName,
			ChecksInclude: slices.Clone(KubeChecksRunnerGroupChecksInclude),
			Override:      &DatadogAgentComponentOverride{Replicas: new(defaultKubeChecksRunnerGroupReplicas)},
		}
		groups = append([]ClusterChecksRunnerGroup{kubeGroup}, groups...)
	}

	if err := validateClusterChecksRunnerGroups(groups); err != nil {
		return nil, fmt.Errorf("invalid cluster checks runner groups: %w", err)
	}
	return groups, nil
}

func validateClusterChecksRunnerGroups(groups []ClusterChecksRunnerGroup) error {
	names := make(map[string]struct{}, len(groups))
	claimedBy := make(map[string]string)
	for _, group := range groups {
		if _, dup := names[group.Name]; dup {
			return fmt.Errorf("duplicate group name %q", group.Name)
		}
		names[group.Name] = struct{}{}

		if len(group.ChecksInclude) == 0 {
			return fmt.Errorf("group %q must declare checksInclude", group.Name)
		}
		for _, check := range group.ChecksInclude {
			if owner, claimed := claimedBy[check]; claimed && owner != group.Name {
				return fmt.Errorf("check %q is claimed by both %q and %q", check, owner, group.Name)
			}
			claimedBy[check] = group.Name
		}
	}
	return nil
}
