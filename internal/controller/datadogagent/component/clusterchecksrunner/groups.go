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

// coreGroupChecks returns the checks the built-in core group claims: those whose feature is enabled.
func coreGroupChecks(ddaSpec *v2alpha1.DatadogAgentSpec) []string {
	features := ddaSpec.Features
	if features == nil {
		return nil
	}
	var checks []string
	if features.KubeStateMetricsCore != nil && apiutils.BoolValue(features.KubeStateMetricsCore.Enabled) {
		checks = append(checks, constants.KubeStateMetricsCoreCheckName)
	}
	if features.OrchestratorExplorer != nil && apiutils.BoolValue(features.OrchestratorExplorer.Enabled) {
		checks = append(checks, constants.OrchestratorCheckName)
	}
	if features.HelmCheck != nil && apiutils.BoolValue(features.HelmCheck.Enabled) {
		checks = append(checks, constants.HelmCheckName)
	}
	return checks
}

// Groups returns the runner groups to deploy. There are none when cluster checks, the Cluster Agent or the Cluster
// Checks Runner component are disabled, or when the Cluster Agent is older than 7.86. Each group is otherwise checked
// on its own (see declaredGroups). A version the Operator cannot parse is assumed recent enough. The error reports
// what was left out.
func Groups(obj metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec) ([]Group, error) {
	if !constants.IsClusterChecksEnabled(ddaSpec) ||
		isDisabled(ddaSpec, v2alpha1.ClusterAgentComponentName) ||
		isDisabled(ddaSpec, v2alpha1.ClusterChecksRunnerComponentName) {
		return nil, nil
	}

	groups, err := declaredGroups(obj, ddaSpec)
	if len(groups) == 0 {
		return nil, err
	}

	if version := clusterAgentVersion(ddaSpec); !utils.IsAboveMinVersion(version, groupsMinVersion, nil) {
		return nil, fmt.Errorf("cluster checks runner groups require Cluster Agent 7.86 or later, got %s", version)
	}
	return groups, err
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

// declaredGroups returns the groups from the groups annotation, then the built-in core group when enabled.
//
// User groups are taken in list order, and a group is skipped on its own when its name can't name a Deployment, it
// declares no checks, its name is taken, its runner image is older than 7.86, or it claims a check an earlier group
// already claims. The core group claims the checks of its enabled features that no user group claims, and isn't
// returned when none are left. A group named core in the annotation only sets the core group's override. A group whose
// override sets disabled is left out, and its checks run where they would without it.
func declaredGroups(obj metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec) ([]Group, error) {
	declared, err := annotationGroups(obj)
	errs := []error{err}
	skip := func(group Group, reason string) {
		errs = append(errs, fmt.Errorf("skipping cluster checks runner group %q: %s", group.Name, reason))
	}

	coreEnabled, _ := strconv.ParseBool(obj.GetAnnotations()[v2alpha1.AnnotationExperimentalCoreChecksRunnerDefault])
	core := Group{
		Name:     CoreGroupName,
		Override: &v2alpha1.DatadogAgentComponentOverride{Replicas: new(defaultCoreGroupReplicas)},
	}

	var groups []Group
	names := map[string]struct{}{}
	claimedBy := map[string]string{}
	for _, group := range declared {
		if group.Override != nil && apiutils.BoolValue(group.Override.Disabled) {
			if group.Name == CoreGroupName {
				coreEnabled = false
			}
			continue
		}
		if group.Name == CoreGroupName {
			if !coreEnabled {
				skip(group, "core is reserved for the built-in group, which is disabled")
				continue
			}
			if len(group.Checks) > 0 {
				errs = append(errs, errors.New("ignoring checks on the core group: its checks are fixed"))
			}
			if group.Override != nil {
				override := group.Override.DeepCopy()
				if override.Replicas == nil {
					override.Replicas = core.Override.Replicas
				}
				core.Override = override
			}
			continue
		}
		if reason := invalidName(obj, group.Name); reason != "" {
			skip(group, reason)
			continue
		}
		if len(group.Checks) == 0 {
			skip(group, "no checks declared")
			continue
		}
		if _, dup := names[group.Name]; dup {
			skip(group, "duplicate group name")
			continue
		}
		if version := groupVersion(ddaSpec, group); !utils.IsAboveMinVersion(version, groupsMinVersion, nil) {
			skip(group, fmt.Sprintf("requires a runner image 7.86 or later, got %s", version))
			continue
		}
		if i := slices.IndexFunc(group.Checks, func(check string) bool { _, claimed := claimedBy[check]; return claimed }); i >= 0 {
			skip(group, fmt.Sprintf("check %q is already claimed by %q", group.Checks[i], claimedBy[group.Checks[i]]))
			continue
		}
		names[group.Name] = struct{}{}
		for _, check := range group.Checks {
			claimedBy[check] = group.Name
		}
		groups = append(groups, group)
	}

	if !coreEnabled {
		return groups, errors.Join(errs...)
	}
	for _, check := range coreGroupChecks(ddaSpec) {
		if _, claimed := claimedBy[check]; !claimed {
			core.Checks = append(core.Checks, check)
		}
	}
	switch {
	case len(core.Checks) == 0:
	case invalidName(obj, core.Name) != "":
		skip(core, invalidName(obj, core.Name))
	case !utils.IsAboveMinVersion(groupVersion(ddaSpec, core), groupsMinVersion, nil):
		skip(core, fmt.Sprintf("requires a runner image 7.86 or later, got %s", groupVersion(ddaSpec, core)))
	default:
		groups = append([]Group{core}, groups...)
	}
	return groups, errors.Join(errs...)
}

func annotationGroups(obj metav1.Object) ([]Group, error) {
	raw := obj.GetAnnotations()[v2alpha1.AnnotationExperimentalClusterChecksRunnerGroups]
	if raw == "" {
		return nil, nil
	}
	var groups []Group
	if err := json.Unmarshal([]byte(raw), &groups); err != nil {
		return nil, fmt.Errorf("ignoring the %s annotation: %w", v2alpha1.AnnotationExperimentalClusterChecksRunnerGroups, err)
	}
	return groups, nil
}

// invalidName returns why a group name can't name its Deployment, or "" when it can.
func invalidName(obj metav1.Object, name string) string {
	if msgs := k8svalidation.IsDNS1123Label(name); len(msgs) > 0 {
		return strings.Join(msgs, "; ")
	}
	// The Deployment name is also used as a label value, which can't exceed 63 characters.
	if deployment := GetClusterChecksRunnerGroupName(obj, name); len(deployment) > k8svalidation.LabelValueMaxLength {
		return fmt.Sprintf("Deployment name %q is longer than %d characters", deployment, k8svalidation.LabelValueMaxLength)
	}
	return ""
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
