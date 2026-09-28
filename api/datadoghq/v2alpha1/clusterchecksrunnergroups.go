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

// KubeChecksRunnerGroupName is the reserved name of the built-in kube runner
// group materialized when the AnnotationExperimentalKubeChecksRunnerDefault
// annotation is set to "true". A user-declared group with this name in the
// AnnotationExperimentalClusterChecksRunnerGroups annotation replaces the
// built-in, so the kube family and its Deployment can be fully customized
// (replicas, resources, checks) like any other group.
const KubeChecksRunnerGroupName = "kube"

// KubeChecksRunnerGroupChecksInclude is the curated list of kube-family
// checks the built-in kube runner group claims: KSM core, the orchestrator
// check, and the control-plane monitoring checks. It is the single source of
// truth for the built-in group; do not mutate it.
var KubeChecksRunnerGroupChecksInclude = []string{
	"kubernetes_state_core",
	"orchestrator",
	"kube_apiserver_metrics",
	"kube_controller_manager",
	"kube_scheduler",
}

// defaultKubeChecksRunnerGroupReplicas is the replica count applied to the
// built-in kube runner group's Deployment by default. Two is the minimum
// high-availability floor: kube checks are single-owner-per-digest, so a
// second replica is failover capacity for the whole kube family rather than
// throughput, and one replica would make every runner restart briefly move
// the kube checks onto node agents.
const defaultKubeChecksRunnerGroupReplicas int32 = 2

// IsExperimentalKubeChecksRunnerDefaultEnabled returns whether the
// AnnotationExperimentalKubeChecksRunnerDefault annotation opts this object
// in to the built-in kube runner group. Only strict boolean values (as
// parsed by strconv.ParseBool) count; an absent, empty or unparseable value
// means disabled.
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

// GetEffectiveClusterChecksRunnerGroups returns the runner groups that must
// be materialized as dedicated Cluster Checks Runner Deployments: the groups
// declared in the AnnotationExperimentalClusterChecksRunnerGroups annotation,
// plus the built-in kube group (KubeChecksRunnerGroupName, restricted to
// KubeChecksRunnerGroupChecksInclude, 2 replicas by default) prepended when
// the AnnotationExperimentalKubeChecksRunnerDefault annotation is enabled —
// unless the user declared their own group named "kube", which replaces the
// built-in entirely.
//
// Both the clusterchecks feature (to derive the default runners' exclude
// list) and the DDAI reconciler (to materialize the group Deployments) call
// this, so the two can never disagree on which groups exist.
func GetEffectiveClusterChecksRunnerGroups(obj metav1.Object) ([]ClusterChecksRunnerGroup, error) {
	groups, err := GetExperimentalClusterChecksRunnerGroups(obj)
	if err != nil {
		return nil, err
	}

	if !IsExperimentalKubeChecksRunnerDefaultEnabled(obj) {
		return groups, nil
	}

	// The "kube" name is reserved: a user-declared group with that name takes
	// over the built-in so it can be fully customized.
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
