// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusterchecksrunner

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8svalidation "k8s.io/apimachinery/pkg/util/validation"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/utils"
)

// Group is an experimental runner group: a dedicated Cluster Checks Runner Deployment that runs only the checks it claims.
type Group struct {
	Name     string                                  `json:"name"`
	Checks   []string                                `json:"checks"`
	Override *v2alpha1.DatadogAgentComponentOverride `json:"override,omitempty"`
}

// CoreGroupName is the built-in core group's name; a user group with this name replaces it.
const CoreGroupName = "core"

const defaultCoreGroupReplicas int32 = 2

// groupsMinVersion is the first Agent release supporting runner groups, for both the Cluster Agent and the runners.
const groupsMinVersion = "7.86.0-0"

func coreGroup() Group {
	return Group{
		Name:     CoreGroupName,
		Checks:   []string{constants.KubeStateMetricsCoreCheckName, constants.OrchestratorCheckName, constants.HelmCheckName},
		Override: &v2alpha1.DatadogAgentComponentOverride{Replicas: new(defaultCoreGroupReplicas)},
	}
}

// Groups returns the runner groups to deploy. There are none when cluster checks, the Cluster Agent or the Cluster
// Checks Runner component are disabled, or when the Cluster Agent is older than 7.86, and a group whose runner image is
// older than 7.86 is left out. A version the Operator cannot parse is assumed recent enough. The error reports what
// was left out.
func Groups(obj metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec) ([]Group, error) {
	if !constants.IsClusterChecksEnabled(ddaSpec) ||
		isDisabled(ddaSpec, v2alpha1.ClusterAgentComponentName) ||
		isDisabled(ddaSpec, v2alpha1.ClusterChecksRunnerComponentName) {
		return nil, nil
	}

	groups, err := declaredGroups(obj)
	if len(groups) == 0 {
		return nil, err
	}

	if version := clusterAgentVersion(ddaSpec); !utils.IsAboveMinVersion(version, groupsMinVersion, nil) {
		return nil, fmt.Errorf("cluster checks runner groups require Cluster Agent 7.86 or later, got %s", version)
	}

	var supported []Group
	errs := []error{err}
	for _, group := range groups {
		if version := groupVersion(ddaSpec, group); !utils.IsAboveMinVersion(version, groupsMinVersion, nil) {
			errs = append(errs, fmt.Errorf("cluster checks runner group %q requires a runner image 7.86 or later, got %s", group.Name, version))
			continue
		}
		supported = append(supported, group)
	}
	return supported, errors.Join(errs...)
}

// RunnersRequired returns whether the DDA needs the default Cluster Checks Runners or runner groups.
func RunnersRequired(obj metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec) bool {
	if !constants.IsClusterChecksEnabled(ddaSpec) {
		return false
	}
	groups, _ := Groups(obj, ddaSpec)
	return constants.IsCCREnabled(ddaSpec) || len(groups) > 0
}

// RunsOnRunners returns whether a cluster check runs on the default Cluster Checks Runners or on its runner group.
func RunsOnRunners(obj metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, checkName string) bool {
	if !constants.IsClusterChecksEnabled(ddaSpec) {
		return false
	}
	if constants.IsCCREnabled(ddaSpec) {
		return true
	}
	_, claimed := claimingGroup(obj, ddaSpec, checkName)
	return claimed
}

// RunnerVersion returns the Agent version of the runners that run a check: its runner group's image when a group
// claims it, else the default Cluster Checks Runners' image.
func RunnerVersion(obj metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, checkName string) string {
	if group, claimed := claimingGroup(obj, ddaSpec, checkName); claimed {
		return groupVersion(ddaSpec, group)
	}
	return runnersVersion(ddaSpec)
}

func claimingGroup(obj metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, checkName string) (Group, bool) {
	groups, _ := Groups(obj, ddaSpec)
	for _, group := range groups {
		if slices.Contains(group.Checks, checkName) {
			return group, true
		}
	}
	return Group{}, false
}

// declaredGroups returns the annotation groups plus the built-in core group when enabled. Any mistake in the groups
// annotation drops all of its groups, but never the built-in core group.
func declaredGroups(obj metav1.Object) ([]Group, error) {
	custom, err := customGroups(obj)

	if enabled, _ := strconv.ParseBool(obj.GetAnnotations()[v2alpha1.AnnotationExperimentalCoreChecksRunnerDefault]); enabled &&
		!slices.ContainsFunc(custom, func(g Group) bool { return g.Name == CoreGroupName }) {
		core := coreGroup()
		if overlapErr := validateGroups(append([]Group{core}, custom...)); overlapErr != nil {
			err = errors.Join(err, fmt.Errorf("ignoring the %s annotation: %w", v2alpha1.AnnotationExperimentalClusterChecksRunnerGroups, overlapErr))
			custom = nil
		}
		custom = append([]Group{core}, custom...)
	}

	valid, nameErr := filterInvalidNames(obj, custom)
	return valid, errors.Join(err, nameErr)
}

func customGroups(obj metav1.Object) ([]Group, error) {
	raw := obj.GetAnnotations()[v2alpha1.AnnotationExperimentalClusterChecksRunnerGroups]
	if raw == "" {
		return nil, nil
	}
	var groups []Group
	if err := json.Unmarshal([]byte(raw), &groups); err != nil {
		return nil, fmt.Errorf("ignoring the %s annotation: %w", v2alpha1.AnnotationExperimentalClusterChecksRunnerGroups, err)
	}
	if err := validateGroups(groups); err != nil {
		return nil, fmt.Errorf("ignoring the %s annotation: %w", v2alpha1.AnnotationExperimentalClusterChecksRunnerGroups, err)
	}
	return groups, nil
}

func validateGroups(groups []Group) error {
	names := make(map[string]struct{}, len(groups))
	claimedBy := make(map[string]string)
	for _, group := range groups {
		if _, dup := names[group.Name]; dup {
			return fmt.Errorf("duplicate group name %q", group.Name)
		}
		names[group.Name] = struct{}{}

		if len(group.Checks) == 0 {
			return fmt.Errorf("group %q must declare checks", group.Name)
		}
		for _, check := range group.Checks {
			if owner, claimed := claimedBy[check]; claimed && owner != group.Name {
				return fmt.Errorf("check %q is claimed by both %q and %q", check, owner, group.Name)
			}
			claimedBy[check] = group.Name
		}
	}
	return nil
}

// filterInvalidNames drops the groups whose name can't name a Deployment, and keeps the others.
func filterInvalidNames(obj metav1.Object, groups []Group) ([]Group, error) {
	var valid []Group
	var errs []error
	for _, group := range groups {
		if msgs := k8svalidation.IsDNS1123Label(group.Name); len(msgs) > 0 {
			errs = append(errs, fmt.Errorf("skipping cluster checks runner group %q: %s", group.Name, strings.Join(msgs, "; ")))
			continue
		}
		// The Deployment name is also used as a label value, which can't exceed 63 characters.
		if name := GetClusterChecksRunnerGroupName(obj, group.Name); len(name) > k8svalidation.LabelValueMaxLength {
			errs = append(errs, fmt.Errorf("skipping cluster checks runner group %q: Deployment name %q is longer than %d characters", group.Name, name, k8svalidation.LabelValueMaxLength))
			continue
		}
		valid = append(valid, group)
	}
	return valid, errors.Join(errs...)
}

func isDisabled(ddaSpec *v2alpha1.DatadogAgentSpec, component v2alpha1.ComponentName) bool {
	override := ddaSpec.Override[component]
	return override != nil && apiutils.BoolValue(override.Disabled)
}

func clusterAgentVersion(ddaSpec *v2alpha1.DatadogAgentSpec) string {
	return common.GetComponentVersionFromSpec(ddaSpec, v2alpha1.ClusterAgentComponentName)
}

func runnersVersion(ddaSpec *v2alpha1.DatadogAgentSpec) string {
	return common.GetComponentVersionFromSpec(ddaSpec, v2alpha1.ClusterChecksRunnerComponentName)
}

// groupVersion applies the group's image override on top of the runners' image, like the pod template overrides do.
func groupVersion(ddaSpec *v2alpha1.DatadogAgentSpec, group Group) string {
	image := common.ResolveComponentImage(ddaSpec, v2alpha1.ClusterChecksRunnerComponentName)
	if group.Override != nil {
		image = images.OverrideAgentImage(image, group.Override.Image)
	}
	return images.FromString(image).Version()
}
