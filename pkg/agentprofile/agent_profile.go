// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentprofile

import (
	"fmt"
	"os"
	"sort"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/metrics"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
)

const (
	// OldProfileLabelKey was deprecated in operator v1.8.0
	OldProfileLabelKey  = "agent.datadoghq.com/profile"
	defaultProfileName  = "default"
	daemonSetNamePrefix = "datadog-agent-with-profile-"
	labelValueMaxLength = 63
)

// ApplyProfile validates a profile spec and returns a map that maps each
// node name to the profile that should be applied to it.
// When create strategy is enabled, the profile is mapped to:
// - existing nodes with the correct label
// - nodes that need a new or corrected label up to maxUnavailable # of nodes
func ApplyProfile(logger logr.Logger, profile *v1alpha1.DatadogAgentProfile, nodes []v1.Node, profileAppliedByNode map[string]types.NamespacedName,
	now metav1.Time, maxUnavailable int) (map[string]types.NamespacedName, error) {
	matchingNodes := map[string]bool{}
	profileStatus := v1alpha1.DatadogAgentProfileStatus{}

	if hash, err := comparison.GenerateMD5ForSpec(profile.Spec); err != nil {
		logger.Error(err, "couldn't generate hash for profile", "datadogagentprofile", profile.Name, "datadogagentprofile_namespace", profile.Namespace)
	} else {
		profileStatus.CurrentHash = hash
	}

	if err := validateProfileName(profile.Name); err != nil {
		logger.Error(err, "profile name is invalid, skipping", "datadogagentprofile", profile.Name, "datadogagentprofile_namespace", profile.Namespace)
		profileStatus.Conditions = SetDatadogAgentProfileCondition(profileStatus.Conditions, NewDatadogAgentProfileCondition(ValidConditionType, metav1.ConditionFalse, now, InvalidConditionReason, err.Error()))
		profileStatus.Valid = metav1.ConditionFalse
		UpdateProfileStatus(logger, profile, profileStatus, now)
		return profileAppliedByNode, err
	}

	if err := v1alpha1.ValidateDatadogAgentProfileSpec(&profile.Spec); err != nil {
		logger.Error(err, "profile spec is invalid, skipping", "datadogagentprofile", profile.Name, "datadogagentprofile_namespace", profile.Namespace)
		metrics.DAPValid.With(prometheus.Labels{"datadogagentprofile": profile.Name}).Set(metrics.FalseValue)
		profileStatus.Conditions = SetDatadogAgentProfileCondition(profileStatus.Conditions, NewDatadogAgentProfileCondition(ValidConditionType, metav1.ConditionFalse, now, InvalidConditionReason, err.Error()))
		profileStatus.Valid = metav1.ConditionFalse
		UpdateProfileStatus(logger, profile, profileStatus, now)
		return profileAppliedByNode, err
	}

	toLabelNodeCount := 0

	for _, node := range nodes {
		matchesNode, err := profileMatchesNode(profile, node.Labels)
		if err != nil {
			logger.Error(err, "profile selector is invalid, skipping", "datadogagentprofile", profile.Name, "datadogagentprofile_namespace", profile.Namespace)
			metrics.DAPValid.With(prometheus.Labels{"datadogagentprofile": profile.Name}).Set(metrics.FalseValue)
			profileStatus.Conditions = SetDatadogAgentProfileCondition(profileStatus.Conditions, NewDatadogAgentProfileCondition(ValidConditionType, metav1.ConditionFalse, now, InvalidConditionReason, err.Error()))
			profileStatus.Valid = metav1.ConditionFalse
			UpdateProfileStatus(logger, profile, profileStatus, now)
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
				UpdateProfileStatus(logger, profile, profileStatus, now)
				return profileAppliedByNode, fmt.Errorf("conflict with existing profile")
			} else {
				profileLabelValue, labelExists := node.Labels[constants.ProfileLabelKey]
				if labelExists && profileLabelValue == profile.Name {
					matchingNodes[node.Name] = true
				} else {
					matchingNodes[node.Name] = false
					toLabelNodeCount++
				}
				profileStatus.Conditions = SetDatadogAgentProfileCondition(profileStatus.Conditions, NewDatadogAgentProfileCondition(AppliedConditionType, metav1.ConditionTrue, now, AppliedConditionReason, "Profile applied"))
				profileStatus.Applied = metav1.ConditionTrue
			}
		}
	}

	numNodesToLabel := 0
	if CreateStrategyEnabled() {
		profileStatus.CreateStrategy = &v1alpha1.CreateStrategy{}
		if profile.Status.CreateStrategy != nil {
			profileStatus.CreateStrategy.PodsReady = profile.Status.CreateStrategy.PodsReady
			profileStatus.CreateStrategy.LastTransition = profile.Status.CreateStrategy.LastTransition
		}
		profileStatus.CreateStrategy.Status = getCreateStrategyStatus(profile.Status.CreateStrategy, toLabelNodeCount)
		profileStatus.CreateStrategy.MaxUnavailable = int32(maxUnavailable)

		if canLabel(logger, profileStatus.CreateStrategy) {
			numNodesToLabel = getNumNodesToLabel(profile.Status.CreateStrategy, maxUnavailable, toLabelNodeCount)
		}
	}

	for node, hasCorrectProfileLabel := range matchingNodes {
		if CreateStrategyEnabled() {
			if hasCorrectProfileLabel {
				profileStatus.CreateStrategy.NodesLabeled++
			} else {
				if numNodesToLabel <= 0 {
					continue
				}
				numNodesToLabel--
				profileStatus.CreateStrategy.NodesLabeled++
			}
		}

		profileAppliedByNode[node] = types.NamespacedName{
			Namespace: profile.Namespace,
			Name:      profile.Name,
		}
	}

	UpdateProfileStatus(logger, profile, profileStatus, now)
	return profileAppliedByNode, nil
}

func ApplyDefaultProfile(profilesToApply []v1alpha1.DatadogAgentProfile, profileAppliedByNode map[string]types.NamespacedName, nodes []v1.Node) []v1alpha1.DatadogAgentProfile {
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
func OverrideFromProfile(profile *v1alpha1.DatadogAgentProfile, useV3Metadata bool) v2alpha1.DatadogAgentComponentOverride {
	if profile.Name == "" && profile.Namespace == "" {
		return v2alpha1.DatadogAgentComponentOverride{}
	}
	overrideDSName := DaemonSetName(types.NamespacedName{
		Namespace: profile.Namespace,
		Name:      profile.Name,
	}, useV3Metadata)

	profileComponentOverride := v2alpha1.DatadogAgentComponentOverride{
		Name:     &overrideDSName,
		Affinity: AffinityOverride(profile),
		Labels:   labelsOverride(profile),
	}

	if !IsDefaultProfile(profile.Namespace, profile.Name) && profile.Spec.Config != nil {
		// We only support overrides for the node agent
		if nodeAgentOverride, ok := profile.Spec.Config.Override[v2alpha1.NodeAgentComponentName]; ok {
			profileComponentOverride.Containers = containersOverride(nodeAgentOverride)
			profileComponentOverride.PriorityClassName = nodeAgentOverride.PriorityClassName
			profileComponentOverride.RuntimeClassName = nodeAgentOverride.RuntimeClassName
			profileComponentOverride.UpdateStrategy = nodeAgentOverride.UpdateStrategy
		}
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
func DaemonSetName(profileNamespacedName types.NamespacedName, useV3Metadata bool) string {
	if IsDefaultProfile(profileNamespacedName.Namespace, profileNamespacedName.Name) {
		return "" // Return empty so it does not override the default DaemonSet name
	}

	if useV3Metadata {
		return fmt.Sprintf("%s-%s", profileNamespacedName.Name, constants.DefaultAgentResourceSuffix)
	}

	return daemonSetNamePrefix + profileNamespacedName.Namespace + "-" + profileNamespacedName.Name
}

// defaultProfile returns the default profile, we just need a name to identify
// it.
func defaultProfile() v1alpha1.DatadogAgentProfile {
	return v1alpha1.DatadogAgentProfile{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "",
			Name:      defaultProfileName,
		},
	}
}

func AffinityOverride(profile *v1alpha1.DatadogAgentProfile) *v1.Affinity {
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
		Key:      constants.ProfileLabelKey,
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
								Key:      constants.ProfileLabelKey,
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

func containersOverride(nodeAgentOverride *v2alpha1.DatadogAgentComponentOverride) map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer {
	if len(nodeAgentOverride.Containers) == 0 {
		return nil
	}

	containersInNodeAgent := []apicommon.AgentContainerName{
		apicommon.CoreAgentContainerName,
		apicommon.TraceAgentContainerName,
		apicommon.ProcessAgentContainerName,
		apicommon.SecurityAgentContainerName,
		apicommon.SystemProbeContainerName,
		apicommon.OtelAgent,
		apicommon.AgentDataPlaneContainerName,
	}

	res := map[apicommon.AgentContainerName]*v2alpha1.DatadogAgentGenericContainer{}

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

func labelsOverride(profile *v1alpha1.DatadogAgentProfile) map[string]string {
	if IsDefaultProfile(profile.Namespace, profile.Name) {
		return nil
	}

	labels := map[string]string{}

	if profile.Spec.Config != nil {
		if nodeAgentOverride, ok := profile.Spec.Config.Override[v2alpha1.NodeAgentComponentName]; ok {
			for labelName, labelVal := range nodeAgentOverride.Labels {
				labels[labelName] = labelVal
			}
		}
	}

	labels[constants.ProfileLabelKey] = profile.Name

	return labels
}

// SortProfiles sorts the profiles by creation timestamp. If two profiles have
// the same creation timestamp, it sorts them by name.
func SortProfiles(profiles []v1alpha1.DatadogAgentProfile) []v1alpha1.DatadogAgentProfile {
	sortedProfiles := make([]v1alpha1.DatadogAgentProfile, len(profiles))
	copy(sortedProfiles, profiles)

	sort.Slice(sortedProfiles, func(i, j int) bool {
		if !sortedProfiles[i].CreationTimestamp.Equal(&sortedProfiles[j].CreationTimestamp) {
			return sortedProfiles[i].CreationTimestamp.Before(&sortedProfiles[j].CreationTimestamp)
		}

		return sortedProfiles[i].Name < sortedProfiles[j].Name
	})

	return sortedProfiles
}

func profileMatchesNode(profile *v1alpha1.DatadogAgentProfile, nodeLabels map[string]string) (bool, error) {
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

func canLabel(logger logr.Logger, createStrategy *v1alpha1.CreateStrategy) bool {
	if createStrategy == nil {
		return false
	}

	switch createStrategy.Status {
	case v1alpha1.CompletedStatus:
		return true
	case v1alpha1.InProgressStatus:
		return true
	case v1alpha1.WaitingStatus:
		return false
	default:
		logger.Error(fmt.Errorf("received unexpected create strategy status condition"), string(createStrategy.Status))
		return false
	}
}

func getNumNodesToLabel(createStrategyStatus *v1alpha1.CreateStrategy, maxUnavailable, toLabelNodeCount int) int {
	if createStrategyStatus == nil {
		return 0
	}

	// once create strategy status is completed, label all necessary nodes
	if createStrategyStatus.Status == v1alpha1.CompletedStatus {
		return toLabelNodeCount
	}

	return maxUnavailable - (int(createStrategyStatus.NodesLabeled - createStrategyStatus.PodsReady))
}

func getCreateStrategyStatus(status *v1alpha1.CreateStrategy, toLabelNodeCount int) v1alpha1.CreateStrategyStatus {
	// new profiles start in waiting to ensure profile daemonsets are created prior to node labeling
	if status == nil {
		return v1alpha1.WaitingStatus
	}

	// all necessary nodes have been labeled
	if toLabelNodeCount == 0 {
		return v1alpha1.CompletedStatus
	}

	return status.Status
}

// CreateStrategyEnabled returns true if the create strategy enabled env var is set to true
func CreateStrategyEnabled() bool {
	return os.Getenv(apicommon.CreateStrategyEnabled) == "true"
}

// GetMaxUnavailable gets the maxUnavailable value as in int.
// Priority is DAP > DDA > Kubernetes default value
func GetMaxUnavailable(logger logr.Logger, ddaSpec *v2alpha1.DatadogAgentSpec, profile *v1alpha1.DatadogAgentProfile, numNodes int, edsOptions *agent.ExtendedDaemonsetOptions) int {
	// Kubernetes default for DaemonSet MaxUnavailable is 1
	// https://github.com/kubernetes/kubernetes/blob/4aca09bc0c45acc69cfdb425d1eea8818eee04d9/pkg/apis/apps/v1/defaults.go#L87
	defaultMaxUnavailable := 1

	// maxUnavailable from profile
	if profile.Spec.Config != nil {
		if nodeAgentOverride, ok := profile.Spec.Config.Override[v2alpha1.NodeAgentComponentName]; ok {
			if nodeAgentOverride.UpdateStrategy != nil && nodeAgentOverride.UpdateStrategy.RollingUpdate != nil {
				numToScale, err := intstr.GetScaledValueFromIntOrPercent(nodeAgentOverride.UpdateStrategy.RollingUpdate.MaxUnavailable, numNodes, true)
				if err != nil {
					logger.Error(err, "unable to get max unavailable pods from DatadogAgentProfile, defaulting to 1")
					return defaultMaxUnavailable
				}
				return numToScale
			}
		}
	}

	// maxUnavilable from DDA
	if nodeAgentOverride, ok := ddaSpec.Override[v2alpha1.NodeAgentComponentName]; ok {
		if nodeAgentOverride.UpdateStrategy != nil && nodeAgentOverride.UpdateStrategy.RollingUpdate != nil {
			numToScale, err := intstr.GetScaledValueFromIntOrPercent(nodeAgentOverride.UpdateStrategy.RollingUpdate.MaxUnavailable, numNodes, true)
			if err != nil {
				logger.Error(err, "unable to get max unavailable pods from DatadogAgent, defaulting to 1")
				return defaultMaxUnavailable
			}
			return numToScale
		}
	}

	// maxUnavailable from EDS options
	if edsOptions != nil && edsOptions.MaxPodUnavailable != "" {
		numToScale, err := intstr.GetScaledValueFromIntOrPercent(apiutils.NewIntOrStringPointer(edsOptions.MaxPodUnavailable), numNodes, true)
		if err != nil {
			logger.Error(err, "unable to get max unavailable pods from EDS options, defaulting to 1")
			return defaultMaxUnavailable
		}
		return numToScale
	}

	// k8s default
	return defaultMaxUnavailable
}
