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
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8svalidation "k8s.io/apimachinery/pkg/util/validation"
)

// ClusterChecksRunnerGroup declares an additional, dedicated Cluster Checks
// Runner Deployment restricted to a subset of checks. Experimental, not part
// of the CRD schema: configured out-of-band via the
// AnnotationExperimentalClusterChecksRunnerGroups annotation.
type ClusterChecksRunnerGroup struct {
	// Name uniquely identifies this runner group and is used to derive its Deployment name.
	Name string `json:"name"`

	// ChecksInclude is the exclusive, non-empty list of check names this group runs.
	// A check can be claimed by at most one group; node agents and the default
	// runners are configured to exclude every claimed check.
	ChecksInclude []string `json:"checksInclude"`

	// Override allows customization of this runner group's Deployment.
	// +optional
	Override *DatadogAgentComponentOverride `json:"override,omitempty"`
}

// KubeChecksRunnerGroupName is the reserved name of the built-in kube
// runner group. A user-declared group with this name replaces the built-in.
const KubeChecksRunnerGroupName = "kube"

// KubeChecksRunnerGroupChecksInclude is the default list of checks the
// built-in group claims: KSM core and the orchestrator check. Single source
// of truth; do not mutate. Control-plane monitoring checks stay in the
// general pool unless added via checksInclude.
var KubeChecksRunnerGroupChecksInclude = []string{
	"kubernetes_state_core",
	"orchestrator",
}

// defaultKubeChecksRunnerGroupReplicas is the built-in group's default
// replica count: 2 is the HA floor (failover capacity, not throughput).
const defaultKubeChecksRunnerGroupReplicas int32 = 2

// IsExperimentalKubeChecksRunnerDefaultEnabled reports whether the
// AnnotationExperimentalKubeChecksRunnerDefault annotation is a strict
// boolean "true". Absent, empty or unparseable means disabled.
func IsExperimentalKubeChecksRunnerDefaultEnabled(obj metav1.Object) bool {
	enabled, _ := strconv.ParseBool(obj.GetAnnotations()[AnnotationExperimentalKubeChecksRunnerDefault])
	return enabled
}

// GetEffectiveClusterChecksRunnerGroups returns the validated groups to
// materialize: the AnnotationExperimentalClusterChecksRunnerGroups groups plus
// the built-in kube group when the knob is on (skipped when the user declared
// their own "kube" group). Returns (nil, nil) when there are none.
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

// validateClusterChecksRunnerGroups enforces unique DNS-1123 names, a
// non-empty ChecksInclude, and disjoint claims: a check belongs to one group.
func validateClusterChecksRunnerGroups(groups []ClusterChecksRunnerGroup) error {
	names := make(map[string]struct{}, len(groups))
	claimedBy := make(map[string]string)
	for _, group := range groups {
		if errs := k8svalidation.IsDNS1123Label(group.Name); len(errs) > 0 {
			return fmt.Errorf("group name %q: %s", group.Name, strings.Join(errs, ", "))
		}
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
