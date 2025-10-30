// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"maps"
	"slices"
	"strconv"
	"strings"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/utils"
)

const ProcessConfigRunInCoreAgentMinVersion = "7.60.0-0"
const EnableADPAnnotation = "agent.datadoghq.com/adp-enabled"
const EnableFineGrainedKubeletAuthz = "agent.datadoghq.com/fine-grained-kubelet-authorization-enabled"

func agentSupportsRunInCoreAgent(ddaSpec *v2alpha1.DatadogAgentSpec) bool {
	// Agent version must >= 7.60.0 to run feature in core agent
	if nodeAgent, ok := ddaSpec.Override[v2alpha1.NodeAgentComponentName]; ok {
		if nodeAgent.Image != nil {
			return utils.IsAboveMinVersion(common.GetAgentVersionFromImage(*nodeAgent.Image), ProcessConfigRunInCoreAgentMinVersion)
		}
	}
	return utils.IsAboveMinVersion(images.AgentLatestVersion, ProcessConfigRunInCoreAgentMinVersion)
}

// OverrideProcessConfigRunInCoreAgent determines whether to respect the currentVal based on
// environment variables and the agent version.
func OverrideProcessConfigRunInCoreAgent(ddaSpec *v2alpha1.DatadogAgentSpec, currentVal bool) bool {
	if nodeAgent, ok := ddaSpec.Override[v2alpha1.NodeAgentComponentName]; ok {
		for _, env := range nodeAgent.Env {
			if env.Name == common.DDProcessConfigRunInCoreAgent {
				val, err := strconv.ParseBool(env.Value)
				if err == nil {
					return val
				}
			}
		}
	}

	if !agentSupportsRunInCoreAgent(ddaSpec) {
		return false
	}

	return currentVal
}

func hasFeatureEnableAnnotation(dda metav1.Object, annotation string) bool {
	if value, ok := dda.GetAnnotations()[annotation]; ok {
		return value == "true"
	}
	return false
}

// HasAgentDataPlaneAnnotation returns true if the Agent Data Plane is enabled via the dedicated `agent.datadoghq.com/adp-enabled` annotation
func HasAgentDataPlaneAnnotation(dda metav1.Object) bool {
	return hasFeatureEnableAnnotation(dda, EnableADPAnnotation)
}

// HasFineGrainedKubeletAuthz returns true if the feature is enabled via the dedicated `agent.datadoghq.com/fine-grained-kubelet-authorization-enabled` annotation
func HasFineGrainedKubeletAuthz(dda metav1.Object) bool {
	return hasFeatureEnableAnnotation(dda, EnableFineGrainedKubeletAuthz)
}

// resourceSet represents a set of resources with their verbs
type resourceSet map[string][]string

// groupedResources maps API groups to their resource sets
type groupedResources map[string]resourceSet

// RBACBuilder provides a simple builder for creating RBAC policy rules for custom resources
type RBACBuilder struct {
	// groupedResources stores resources grouped by API group
	groupedResources groupedResources
	// verbs stores the verbs to apply to all rules
	verbs []string
}

// NewRBACBuilder creates a new RBACBuilder instance with the specified verbs
func NewRBACBuilder(verbs ...string) *RBACBuilder {
	return &RBACBuilder{
		groupedResources: make(groupedResources),
		verbs:            verbs,
	}
}

// AddGroupKind adds a custom resource by group and resource name with optional verbs
// If no verbs are provided, uses the default verbs from NewRBACBuilder
func (rb *RBACBuilder) AddGroupKind(group, resource string, verbs ...string) *RBACBuilder {
	if _, exists := rb.groupedResources[group]; !exists {
		rb.groupedResources[group] = make(resourceSet)
	}

	// Use provided verbs or fall back to default verbs
	resourceVerbs := verbs
	if len(verbs) == 0 {
		resourceVerbs = rb.verbs
	}

	existingVerbs := rb.groupedResources[group][resource]
	allVerbs := append(existingVerbs, resourceVerbs...)
	verbSet := make(map[string]struct{})
	var uniqueVerbs []string
	for _, verb := range allVerbs {
		if _, exists := verbSet[verb]; !exists {
			verbSet[verb] = struct{}{}
			uniqueVerbs = append(uniqueVerbs, verb)
		}
	}

	rb.groupedResources[group][resource] = uniqueVerbs
	return rb
}

// Build creates the final RBAC policy rules
func (rb *RBACBuilder) Build() []rbacv1.PolicyRule {
	if len(rb.groupedResources) == 0 {
		return nil
	}

	var rbacRules []rbacv1.PolicyRule

	// Sort API groups for deterministic output
	apiGroups := slices.Sorted(maps.Keys(rb.groupedResources))

	// Create RBAC rules for each API group
	for _, apiGroup := range apiGroups {
		resourceSet := rb.groupedResources[apiGroup]

		// Group resources by their verbs to minimize the number of rules
		verbToResources := make(map[string][]string)
		verbsKeyToActualVerbs := make(map[string][]string)

		for resource, verbs := range resourceSet {
			verbsKey := strings.Join(verbs, ",")
			verbToResources[verbsKey] = append(verbToResources[verbsKey], resource)
			verbsKeyToActualVerbs[verbsKey] = verbs
		}

		// Create one rule per verb combination
		// Sort verbsKeys for deterministic output
		verbsKeys := slices.Sorted(maps.Keys(verbToResources))
		for _, verbsKey := range verbsKeys {
			resources := verbToResources[verbsKey]
			slices.Sort(resources)

			rule := rbacv1.PolicyRule{
				APIGroups: []string{apiGroup},
				Resources: resources,
				Verbs:     verbsKeyToActualVerbs[verbsKey],
			}

			rbacRules = append(rbacRules, rule)
		}
	}

	return rbacRules
}
