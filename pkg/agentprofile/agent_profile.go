// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentprofile

import (
	"fmt"
	"sort"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/controllers/metrics"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
)

const (
	ProfileLabelKey = "agent.datadoghq.com/datadogagentprofile"
	// OldProfileLabelKey was deprecated in operator v1.8.0
	OldProfileLabelKey  = "agent.datadoghq.com/profile"
	defaultProfileName  = "default"
	daemonSetNamePrefix = "datadog-agent-with-profile-"
	labelValueMaxLength = 63
)

// ProfileToApply validates a profile spec and returns a map that maps each
// node name to the profile that should be applied to it.
func ProfileToApply(logger logr.Logger, profile *datadoghqv1alpha1.DatadogAgentProfile, nodes []v1.Node, profileAppliedByNode map[string]types.NamespacedName,
	now metav1.Time) (map[string]types.NamespacedName, error) {
	nodesThatMatchProfile := map[string]bool{}
	profileStatus := datadoghqv1alpha1.DatadogAgentProfileStatus{}

	if hash, err := comparison.GenerateMD5ForSpec(profile.Spec); err != nil {
		logger.Error(err, "couldn't generate hash for profile", "datadogagentprofile", profile.Name, "datadogagentprofile_namespace", profile.Namespace)
	} else {
		profileStatus.CurrentHash = hash
	}

	if err := validateProfileName(profile.Name); err != nil {
		logger.Error(err, "profile name is invalid, skipping", "datadogagentprofile", profile.Name, "datadogagentprofile_namespace", profile.Namespace)
		profileStatus.Conditions = SetDatadogAgentProfileCondition(profileStatus.Conditions, NewDatadogAgentProfileCondition(ValidConditionType, metav1.ConditionFalse, now, InvalidConditionReason, err.Error()))
		profileStatus.Valid = metav1.ConditionFalse
		UpdateProfileStatus(profile, profileStatus, now)
		return profileAppliedByNode, err
	}

	if err := datadoghqv1alpha1.ValidateDatadogAgentProfileSpec(&profile.Spec); err != nil {
		logger.Error(err, "profile spec is invalid, skipping", "datadogagentprofile", profile.Name, "datadogagentprofile_namespace", profile.Namespace)
		metrics.DAPValid.With(prometheus.Labels{"datadogagentprofile": profile.Name}).Set(metrics.FalseValue)
		profileStatus.Conditions = SetDatadogAgentProfileCondition(profileStatus.Conditions, NewDatadogAgentProfileCondition(ValidConditionType, metav1.ConditionFalse, now, InvalidConditionReason, err.Error()))
		profileStatus.Valid = metav1.ConditionFalse
		UpdateProfileStatus(profile, profileStatus, now)
		return profileAppliedByNode, err
	}

	for _, node := range nodes {
		matchesNode, err := profileMatchesNode(profile, node.Labels)
		if err != nil {
			logger.Error(err, "profile selector is invalid, skipping", "datadogagentprofile", profile.Name, "datadogagentprofile_namespace", profile.Namespace)
			metrics.DAPValid.With(prometheus.Labels{"datadogagentprofile": profile.Name}).Set(metrics.FalseValue)
			profileStatus.Conditions = SetDatadogAgentProfileCondition(profileStatus.Conditions, NewDatadogAgentProfileCondition(ValidConditionType, metav1.ConditionFalse, now, InvalidConditionReason, err.Error()))
			profileStatus.Valid = metav1.ConditionFalse
			UpdateProfileStatus(profile, profileStatus, now)
			return profileAppliedByNode, err
		}
		metrics.DAPValid.With(prometheus.Labels{"datadogagentprofile": profile.Name}).Set(metrics.TrueValue)
		profileStatus.Valid = metav1.ConditionTrue
		profileStatus.Conditions = SetDatadogAgentProfileCondition(profileStatus.Conditions, NewDatadogAgentProfileCondition(ValidConditionType, metav1.ConditionTrue, now, ValidConditionReason, "Valid manifest"))

		if matchesNode {
			if existingProfile, found := profileAppliedByNode[node.Name]; found {
				// Conflict. This profile should not be applied.
				logger.Info("conflict with existing profile, skipping", "conflicting profile", profile.Namespace+"/"+profile.Name, "existing profile", existingProfile.String())
				profileStatus.Conditions = SetDatadogAgentProfileCondition(profileStatus.Conditions, NewDatadogAgentProfileCondition(AppliedConditionType, metav1.ConditionFalse, now, ConflictConditionReason, "Conflict with existing profile"))
				profileStatus.Applied = metav1.ConditionFalse
				UpdateProfileStatus(profile, profileStatus, now)
				return profileAppliedByNode, fmt.Errorf("conflict with existing profile")
			} else {
				nodesThatMatchProfile[node.Name] = true
				profileStatus.Conditions = SetDatadogAgentProfileCondition(profileStatus.Conditions, NewDatadogAgentProfileCondition(AppliedConditionType, metav1.ConditionTrue, now, AppliedConditionReason, "Profile applied"))
				profileStatus.Applied = metav1.ConditionTrue
			}
		}
	}

	for node := range nodesThatMatchProfile {
		profileAppliedByNode[node] = types.NamespacedName{
			Namespace: profile.Namespace,
			Name:      profile.Name,
		}
	}

	UpdateProfileStatus(profile, profileStatus, now)

	return profileAppliedByNode, nil
}

func ApplyDefaultProfile(profilesToApply []datadoghqv1alpha1.DatadogAgentProfile, profileAppliedByNode map[string]types.NamespacedName, nodes []v1.Node) []datadoghqv1alpha1.DatadogAgentProfile {
	profilesToApply = append(profilesToApply, defaultProfile())

	// Apply the default profile to all nodes that don't have a profile applied
	for _, node := range nodes {
		if _, found := profileAppliedByNode[node.Name]; !found {
			profileAppliedByNode[node.Name] = types.NamespacedName{
				Name: defaultProfileName,
			}
		}
	}

	return profilesToApply
}

// OverrideFromProfile returns the component override that should be
// applied according to the given profile.
func OverrideFromProfile(profile *datadoghqv1alpha1.DatadogAgentProfile) v2alpha1.DatadogAgentComponentOverride {
	if profile.Name == "" && profile.Namespace == "" {
		return v2alpha1.DatadogAgentComponentOverride{}
	}
	overrideDSName := DaemonSetName(types.NamespacedName{
		Namespace: profile.Namespace,
		Name:      profile.Name,
	})

	profileComponentOverride := v2alpha1.DatadogAgentComponentOverride{
		Name:       &overrideDSName,
		Affinity:   affinityOverride(profile),
		Containers: containersOverride(profile),
		Labels:     labelsOverride(profile),
	}

	if priorityClassName := priorityClassNameOverride(profile); priorityClassName != nil {
		profileComponentOverride.PriorityClassName = priorityClassName
	}

	return profileComponentOverride
}

// IsDefaultProfile returns true if the given profile namespace and name
// correspond to the default profile.
func IsDefaultProfile(profileNamespace string, profileName string) bool {
	return profileNamespace == "" && profileName == defaultProfileName
}

// DaemonSetName returns the name that the DaemonSet should have according to
// the name of the profile associated with it.
func DaemonSetName(profileNamespacedName types.NamespacedName) string {
	if IsDefaultProfile(profileNamespacedName.Namespace, profileNamespacedName.Name) {
		return "" // Return empty so it does not override the default DaemonSet name
	}

	return daemonSetNamePrefix + profileNamespacedName.Namespace + "-" + profileNamespacedName.Name
}

// defaultProfile returns the default profile, we just need a name to identify
// it.
func defaultProfile() datadoghqv1alpha1.DatadogAgentProfile {
	return datadoghqv1alpha1.DatadogAgentProfile{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "",
			Name:      defaultProfileName,
		},
	}
}

func affinityOverride(profile *datadoghqv1alpha1.DatadogAgentProfile) *v1.Affinity {
	if IsDefaultProfile(profile.Namespace, profile.Name) {
		return affinityOverrideForDefaultProfile()
	}

	affinity := &v1.Affinity{
		PodAntiAffinity: podAntiAffinityOverride(),
	}

	if profile.Spec.ProfileAffinity == nil || len(profile.Spec.ProfileAffinity.ProfileNodeAffinity) == 0 {
		return affinity
	}

	affinity.NodeAffinity = &v1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
			NodeSelectorTerms: []v1.NodeSelectorTerm{
				{
					MatchExpressions: append(profile.Spec.ProfileAffinity.ProfileNodeAffinity, profileLabelKeyNSR(profile.Name)),
				},
			},
		},
	}

	return affinity
}

// profileLabelKeyNSR returns the NodeSelectorRequirement for a profile to be
// applied to nodes with the following label:
// agent.datadoghq.com/datadogagentprofile:<profile-name>
func profileLabelKeyNSR(profileName string) v1.NodeSelectorRequirement {
	return v1.NodeSelectorRequirement{
		Key:      ProfileLabelKey,
		Operator: v1.NodeSelectorOpIn,
		Values:   []string{profileName},
	}
}

// affinityOverrideForDefaultProfile returns the affinity override that should
// be applied to the default profile. The default profile should be applied to
// all nodes that don't have the agent.datadoghq.com/datadogagentprofile label.
func affinityOverrideForDefaultProfile() *v1.Affinity {
	return &v1.Affinity{
		NodeAffinity: &v1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
				NodeSelectorTerms: []v1.NodeSelectorTerm{
					{
						MatchExpressions: []v1.NodeSelectorRequirement{
							{
								Key:      ProfileLabelKey,
								Operator: v1.NodeSelectorOpDoesNotExist,
							},
						},
					},
				},
			},
		},
		PodAntiAffinity: podAntiAffinityOverride(),
	}
}

// podAntiAffinityOverride returns the pod anti-affinity used to avoid
// scheduling multiple agent pods of different profiles on the same node during
// rollouts.
func podAntiAffinityOverride() *v1.PodAntiAffinity {
	return &v1.PodAntiAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: []v1.PodAffinityTerm{
			{
				LabelSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{
						{
							Key:      apicommon.AgentDeploymentComponentLabelKey,
							Operator: metav1.LabelSelectorOpIn,
							Values:   []string{"agent"},
						},
					},
				},
				TopologyKey: v1.LabelHostname, // Applies to all nodes
			},
		},
	}
}

func containersOverride(profile *datadoghqv1alpha1.DatadogAgentProfile) map[common.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer {
	if profile.Spec.Config == nil {
		return nil
	}

	nodeAgentOverride, ok := profile.Spec.Config.Override[datadoghqv1alpha1.NodeAgentComponentName]
	if !ok { // We only support overrides for the node agent, if there is no override for it, there's nothing to do
		return nil
	}

	if len(nodeAgentOverride.Containers) == 0 {
		return nil
	}

	containersInNodeAgent := []common.AgentContainerName{
		common.CoreAgentContainerName,
		common.TraceAgentContainerName,
		common.ProcessAgentContainerName,
		common.SecurityAgentContainerName,
		common.SystemProbeContainerName,
	}

	res := map[common.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{}

	for _, containerName := range containersInNodeAgent {
		if overrideForContainer, overrideIsDefined := nodeAgentOverride.Containers[containerName]; overrideIsDefined {
			res[containerName] = &v2alpha1.DatadogAgentGenericContainer{
				Resources: overrideForContainer.Resources,
				Env:       overrideForContainer.Env,
			}
		}
	}

	return res
}

func labelsOverride(profile *datadoghqv1alpha1.DatadogAgentProfile) map[string]string {
	if IsDefaultProfile(profile.Namespace, profile.Name) {
		return nil
	}

	labels := map[string]string{}

	if profile.Spec.Config != nil {
		if nodeAgentOverride, ok := profile.Spec.Config.Override[datadoghqv1alpha1.NodeAgentComponentName]; ok {
			for labelName, labelVal := range nodeAgentOverride.Labels {
				labels[labelName] = labelVal
			}
		}
	}

	labels[ProfileLabelKey] = profile.Name

	return labels
}

func priorityClassNameOverride(profile *datadoghqv1alpha1.DatadogAgentProfile) *string {
	if IsDefaultProfile(profile.Namespace, profile.Name) {
		return nil
	}

	if profile.Spec.Config == nil {
		return nil
	}

	nodeAgentOverride, ok := profile.Spec.Config.Override[datadoghqv1alpha1.NodeAgentComponentName]
	if !ok { // We only support overrides for the node agent, if there is no override for it, there's nothing to do
		return nil
	}

	return nodeAgentOverride.PriorityClassName
}

// SortProfiles sorts the profiles by creation timestamp. If two profiles have
// the same creation timestamp, it sorts them by name.
func SortProfiles(profiles []datadoghqv1alpha1.DatadogAgentProfile) []datadoghqv1alpha1.DatadogAgentProfile {
	sortedProfiles := make([]datadoghqv1alpha1.DatadogAgentProfile, len(profiles))
	copy(sortedProfiles, profiles)

	sort.Slice(sortedProfiles, func(i, j int) bool {
		if !sortedProfiles[i].CreationTimestamp.Equal(&sortedProfiles[j].CreationTimestamp) {
			return sortedProfiles[i].CreationTimestamp.Before(&sortedProfiles[j].CreationTimestamp)
		}

		return sortedProfiles[i].Name < sortedProfiles[j].Name
	})

	return sortedProfiles
}

func profileMatchesNode(profile *datadoghqv1alpha1.DatadogAgentProfile, nodeLabels map[string]string) (bool, error) {
	if profile.Spec.ProfileAffinity == nil {
		return true, nil
	}

	for _, requirement := range profile.Spec.ProfileAffinity.ProfileNodeAffinity {
		selector, err := labels.NewRequirement(
			requirement.Key,
			nodeSelectorOperatorToSelectionOperator(requirement.Operator),
			requirement.Values,
		)
		if err != nil {
			return false, err
		}

		if !selector.Matches(labels.Set(nodeLabels)) {
			return false, nil
		}
	}

	return true, nil
}

func nodeSelectorOperatorToSelectionOperator(op v1.NodeSelectorOperator) selection.Operator {
	switch op {
	case v1.NodeSelectorOpIn:
		return selection.In
	case v1.NodeSelectorOpNotIn:
		return selection.NotIn
	case v1.NodeSelectorOpExists:
		return selection.Exists
	case v1.NodeSelectorOpDoesNotExist:
		return selection.DoesNotExist
	case v1.NodeSelectorOpGt:
		return selection.GreaterThan
	case v1.NodeSelectorOpLt:
		return selection.LessThan
	default:
		return ""
	}
}

func validateProfileName(profileName string) error {
	// Label values can be empty but a profile's name should not be empty
	if profileName == "" {
		return fmt.Errorf("Profile name cannot be empty")
	}
	// We add the profile name as a label value, which can be 63 characters max
	if len(profileName) > labelValueMaxLength {
		return fmt.Errorf("Profile name must be no more than 63 characters")
	}

	return nil
}
