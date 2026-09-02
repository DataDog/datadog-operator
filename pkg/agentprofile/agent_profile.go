// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentprofile

import (
	"cmp"
	"fmt"
	"os"
	"slices"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

const (
	// OldProfileLabelKey was deprecated in operator v1.8.0
	OldProfileLabelKey  = "agent.datadoghq.com/profile"
	defaultProfileName  = "default"
	daemonSetNamePrefix = "datadog-agent-with-profile-"
	labelValueMaxLength = 63
	// Kubernetes default for DaemonSet MaxUnavailable is 1
	// https://github.com/kubernetes/kubernetes/blob/4aca09bc0c45acc69cfdb425d1eea8818eee04d9/pkg/apis/apps/v1/defaults.go#L87
	defaultMaxUnavailable = 1
)

// CreateStrategyInfo holds create strategy data for a profile
type CreateStrategyInfo struct {
	nodesNeedingLabel   []string // list of nodes needing labels
	nodesAlreadyLabeled int32    // number of nodes with the correct label
}

// CheckProfileNodeConflicts checks whether a profile conflicts with already-assigned profiles.
// It is a pure read: it does not modify profilesByNode or csInfo.
func CheckProfileNodeConflicts(profile metav1.ObjectMeta, profileRequirements []*labels.Requirement, nodes []v1.Node, profilesByNode map[string]types.NamespacedName) error {
	for _, node := range nodes {
		if !profileMatchesNodeWithRequirements(profileRequirements, node.Labels) {
			continue
		}
		if existingProfile := profilesByNode[node.Name]; !IsDefaultProfile(existingProfile.Namespace, existingProfile.Name) {
			return fmt.Errorf("profile %s conflicts with existing profile: %s", profile.Name, existingProfile.String())
		}
	}
	return nil
}

// AssignNodesToProfile assigns matching nodes to the profile in profilesByNode and records
// create-strategy data in csInfo. It must only be called after CheckProfileNodeConflicts
// returns nil, so it never returns an error.
func AssignNodesToProfile(profile metav1.ObjectMeta, profileRequirements []*labels.Requirement, nodes []v1.Node, profilesByNode map[string]types.NamespacedName, csInfo map[types.NamespacedName]*CreateStrategyInfo) {
	profileNSName := types.NamespacedName{
		Namespace: profile.Namespace,
		Name:      profile.Name,
	}

	if CreateStrategyEnabled() && csInfo[profileNSName] == nil {
		csInfo[profileNSName] = &CreateStrategyInfo{}
	}

	for _, node := range nodes {
		if !profileMatchesNodeWithRequirements(profileRequirements, node.Labels) {
			continue
		}
		profilesByNode[node.Name] = profileNSName
		if CreateStrategyEnabled() {
			profileLabelValue, labelExists := node.Labels[constants.ProfileLabelKey]
			needsLabel := !labelExists || profileLabelValue != profile.Name
			if needsLabel {
				csInfo[profileNSName].nodesNeedingLabel = append(csInfo[profileNSName].nodesNeedingLabel, node.Name)
			} else {
				csInfo[profileNSName].nodesAlreadyLabeled++
			}
		}
	}
}

// ValidateProfileAndReturnRequirements validates a profile's name and spec and affinity requirements
func ValidateProfileAndReturnRequirements(profile *v1alpha1.DatadogAgentProfile) ([]*labels.Requirement, error) {
	if err := validateProfile(profile); err != nil {
		return nil, err
	}
	return parseProfileRequirements(profile)
}

// validateProfile validates a profile's name and spec
func validateProfile(profile *v1alpha1.DatadogAgentProfile) error {
	if err := validateProfileName(profile.Name); err != nil {
		return fmt.Errorf("profile name is invalid: %w", err)
	}
	if err := v1alpha1.ValidateDatadogAgentProfileSpec(&profile.Spec); err != nil {
		return fmt.Errorf("profile spec is invalid: %w", err)
	}
	return nil
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

// DefaultProfile returns the default profile, we just need a name to identify it
func DefaultProfile() v1alpha1.DatadogAgentProfile {
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
func SortProfiles(profiles []v1alpha1.DatadogAgentProfile) []v1alpha1.DatadogAgentProfile {
	sorted := append([]v1alpha1.DatadogAgentProfile{}, profiles...)

	if len(sorted) > 1 {
		slices.SortStableFunc(sorted, compareProfiles)
	}

	return sorted
}

// compareProfiles compares two profiles first by creation time, then by name.
func compareProfiles(a, b v1alpha1.DatadogAgentProfile) int {
	return cmp.Or(
		a.CreationTimestamp.Compare(b.CreationTimestamp.Time),
		cmp.Compare(a.Name, b.Name),
	)
}

// parseProfileRequirements creates requirements from a profile's node affinity
func parseProfileRequirements(profile *v1alpha1.DatadogAgentProfile) ([]*labels.Requirement, error) {
	if profile == nil || profile.Spec.ProfileAffinity == nil {
		return nil, nil
	}

	requirements := make([]*labels.Requirement, len(profile.Spec.ProfileAffinity.ProfileNodeAffinity))
	for i, requirement := range profile.Spec.ProfileAffinity.ProfileNodeAffinity {
		selector, err := labels.NewRequirement(
			requirement.Key,
			nodeSelectorOperatorToSelectionOperator(requirement.Operator),
			requirement.Values,
		)
		if err != nil {
			return nil, err
		}
		requirements[i] = selector
	}
	return requirements, nil
}

// profileMatchesNodeWithRequirements checks if a node matches the given requirements
func profileMatchesNodeWithRequirements(requirements []*labels.Requirement, nodeLabels map[string]string) bool {
	if len(requirements) == 0 {
		return true
	}

	nodeSet := labels.Set(nodeLabels)
	for _, requirement := range requirements {
		if !requirement.Matches(nodeSet) {
			return false
		}
	}
	return true
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

// CreateStrategyEnabled returns true if the create strategy enabled env var is set to true
func CreateStrategyEnabled() bool {
	return os.Getenv(apicommon.CreateStrategyEnabled) == "true"
}

// ApplyCreateStrategy applies the create strategy to the profiles
// - determines the max number of nodes that can be labeled based on max unavailable values
// - updates the profile status based on the create strategy
func ApplyCreateStrategy(logger logr.Logger, profilesByNode map[string]types.NamespacedName, csInfo *CreateStrategyInfo, profile *v1alpha1.DatadogAgentProfile, ddaMaxUnavailable intstr.IntOrString, numNodes int, dsStatus *appsv1.DaemonSetStatus) {
	// Preserve previous status fields to ensure stable transitions across reconciles.
	var previousStatus v1alpha1.CreateStrategyStatus
	var previousLastTransition *metav1.Time
	if profile.Status.CreateStrategy != nil {
		previousStatus = profile.Status.CreateStrategy.Status
		previousLastTransition = profile.Status.CreateStrategy.LastTransition
	}

	if profile.Status.CreateStrategy == nil {
		profile.Status.CreateStrategy = &v1alpha1.CreateStrategy{}
	}

	maxUnavailable := int32(getMaxNodesToLabel(logger, profile.Spec.Config, ddaMaxUnavailable, numNodes))

	// currentUnavailable: nodes labeled but pods not ready yet
	currentUnavailable := int32(0)
	if dsStatus != nil {
		currentUnavailable = csInfo.nodesAlreadyLabeled - dsStatus.NumberReady
	}

	numNodesToLabel := applyMaxNodesToLabel(profilesByNode, csInfo, maxUnavailable, currentUnavailable)

	// # of new unavailable pods after labeling new nodes
	newUnavailable := currentUnavailable + numNodesToLabel

	updateCreateStrategyStatus(profile, csInfo, numNodesToLabel, maxUnavailable, newUnavailable, dsStatus, previousStatus, previousLastTransition)
}

// applyMaxNodesToLabel trims the list of nodes based on max unavailable and returns the number of nodes to be labeled
func applyMaxNodesToLabel(profilesByNode map[string]types.NamespacedName, csInfo *CreateStrategyInfo, maxUnavailable int32, currentUnavailable int32) int32 {
	// sort to apply create strategy deterministically
	slices.Sort(csInfo.nodesNeedingLabel)

	// don't label if at capacity
	if currentUnavailable >= maxUnavailable {
		for _, nodeName := range csInfo.nodesNeedingLabel {
			delete(profilesByNode, nodeName)
		}
		return 0
	}

	capacity := maxUnavailable - currentUnavailable
	nodesToLabel := min(capacity, int32(len(csInfo.nodesNeedingLabel)))

	// delete nodes to be labeled if we are at capacity
	for i := nodesToLabel; i < int32(len(csInfo.nodesNeedingLabel)); i++ {
		delete(profilesByNode, csInfo.nodesNeedingLabel[i])
	}

	return nodesToLabel
}

// updateCreateStrategyStatus updates the profile's create strategy status fields
func updateCreateStrategyStatus(profile *v1alpha1.DatadogAgentProfile, info *CreateStrategyInfo, numNodesToLabel int32, maxUnavailable int32, newUnavailable int32, dsStatus *appsv1.DaemonSetStatus, previousStatus v1alpha1.CreateStrategyStatus, previousLastTransition *metav1.Time) {
	profile.Status.CreateStrategy.MaxUnavailable = maxUnavailable
	profile.Status.CreateStrategy.NodesLabeled = info.nodesAlreadyLabeled + numNodesToLabel
	if dsStatus != nil {
		profile.Status.CreateStrategy.PodsReady = dsStatus.NumberReady
	}

	totalNodes := info.nodesAlreadyLabeled + int32(len(info.nodesNeedingLabel))

	var newStatus v1alpha1.CreateStrategyStatus
	if totalNodes == 0 {
		// no nodes match this profile - completed
		newStatus = v1alpha1.CompletedStatus
	} else if info.nodesAlreadyLabeled == totalNodes {
		// all matching nodes are already labeled - completed
		newStatus = v1alpha1.CompletedStatus
	} else if numNodesToLabel > 0 || newUnavailable < maxUnavailable {
		// labeled nodes in this round or can label more nodes - in progress
		newStatus = v1alpha1.InProgressStatus
	} else {
		// can't label any nodes, waiting for pods to become ready
		newStatus = v1alpha1.WaitingStatus
	}

	// Only bump LastTransition when the Status field changes.
	if previousStatus != newStatus {
		now := metav1.Now()
		profile.Status.CreateStrategy.LastTransition = &now
	} else {
		profile.Status.CreateStrategy.LastTransition = previousLastTransition
	}
	profile.Status.CreateStrategy.Status = newStatus
}

// GetMaxUnavailableFromSpec gets the max unavailable value from a DatadogAgent spec and allows for a custom default value to be provided.
func GetMaxUnavailableFromSpec(spec *v2alpha1.DatadogAgentSpec, customDefault *intstr.IntOrString) intstr.IntOrString {
	// maxUnavailable from DDA spec
	if spec != nil {
		if nodeAgentOverride, ok := spec.Override[v2alpha1.NodeAgentComponentName]; ok {
			if nodeAgentOverride.UpdateStrategy != nil && nodeAgentOverride.UpdateStrategy.RollingUpdate != nil && nodeAgentOverride.UpdateStrategy.RollingUpdate.MaxUnavailable != nil {
				return *nodeAgentOverride.UpdateStrategy.RollingUpdate.MaxUnavailable
			}
		}
	}

	// if a default value is provided, return it over the k8s default
	if customDefault != nil {
		return *customDefault
	}

	// k8s default
	return intstr.FromInt(defaultMaxUnavailable)
}

func getMaxNodesToLabel(logger logr.Logger, spec *v2alpha1.DatadogAgentSpec, ddaMaxUnavailable intstr.IntOrString, numNodes int) int {
	// Get max unavailable from the profile with fallback to the DatadogAgent value.
	profileMaxUnavailable := GetMaxUnavailableFromSpec(spec, &ddaMaxUnavailable)

	// use max unavailable to calculate the max number of nodes to label
	numToScale, err := intstr.GetScaledValueFromIntOrPercent(&profileMaxUnavailable, numNodes, true)
	if err != nil {
		logger.Error(err, "unable to get max unavailable pods, defaulting to 1")
		return defaultMaxUnavailable
	}
	return numToScale
}

func CreateStrategyNeeded(profile *v1alpha1.DatadogAgentProfile, csInfo map[types.NamespacedName]*CreateStrategyInfo) bool {
	profileNSName := types.NamespacedName{
		Namespace: profile.Namespace,
		Name:      profile.Name,
	}

	if IsDefaultProfile(profile.Namespace, profile.Name) {
		return false
	}

	return csInfo[profileNSName] != nil
}
