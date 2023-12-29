// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentprofile

import (
	"fmt"
	"sort"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apimachinery/pkg/types"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/go-logr/logr"
)

const (
	ProfileLabelKey     = "agent.datadoghq.com/profile"
	defaultProfileName  = "default"
	daemonSetNamePrefix = "datadog-agent-with-profile-"
)

// ProfilesToApply given a list of profiles, returns the ones that should be
// applied in the cluster.
// - If there are no profiles, it returns the default profile.
// - If there are no conflicting profiles, it returns all the profiles plus the default one.
// - If there are conflicting profiles, it returns a subset that does not
// conflict plus the default one. When there are conflicting profiles, the
// oldest one is the one that takes precedence. When two profiles share an
// identical creation timestamp, the profile whose name is alphabetically first
// is considered to have priority.
// This function also returns a map that maps each node name to the profile that
// should be applied to it.
func ProfilesToApply(profiles []datadoghqv1alpha1.DatadogAgentProfile, nodes []v1.Node, logger logr.Logger) ([]datadoghqv1alpha1.DatadogAgentProfile, map[string]types.NamespacedName, error) {
	var profilesToApply []datadoghqv1alpha1.DatadogAgentProfile
	profileAppliedPerNode := make(map[string]types.NamespacedName, len(nodes))

	sortedProfiles := sortProfiles(profiles)

	for _, profile := range sortedProfiles {
		conflicts := false
		nodesThatMatchProfile := map[string]bool{}

		if err := datadoghqv1alpha1.ValidateDatadogAgentProfileSpec(&profile.Spec); err != nil {
			logger.Error(err, "profile spec is invalid, skipping", "name", profile.Name, "namespace", profile.Namespace)
			continue
		}

		for _, node := range nodes {
			matchesNode, err := profileMatchesNode(&profile, &node)
			if err != nil {
				return nil, nil, err
			}

			if matchesNode {
				if _, found := profileAppliedPerNode[node.Name]; found {
					// Conflict. This profile should not be applied.
					conflicts = true
					break
				} else {
					nodesThatMatchProfile[node.Name] = true
				}
			}
		}

		if conflicts {
			continue
		}

		for node := range nodesThatMatchProfile {
			profileAppliedPerNode[node] = types.NamespacedName{
				Namespace: profile.Namespace,
				Name:      profile.Name,
			}
		}

		profilesToApply = append(profilesToApply, profile)
	}

	profilesToApply = append(profilesToApply, defaultProfile())

	// Apply the default profile to all nodes that don't have a profile applied
	for _, node := range nodes {
		if _, found := profileAppliedPerNode[node.Name]; !found {
			profileAppliedPerNode[node.Name] = types.NamespacedName{
				Name: defaultProfileName,
			}
		}
	}

	return profilesToApply, profileAppliedPerNode, nil
}

// ComponentOverrideFromProfile returns the component override that should be
// applied according to the given profile.
func ComponentOverrideFromProfile(profile *datadoghqv1alpha1.DatadogAgentProfile) v2alpha1.DatadogAgentComponentOverride {
	overrideDSName := DaemonSetName(types.NamespacedName{
		Namespace: profile.Namespace,
		Name:      profile.Name,
	})

	return v2alpha1.DatadogAgentComponentOverride{
		Name:       &overrideDSName,
		Affinity:   affinityOverride(profile),
		Containers: containersOverride(profile),
		Labels:     labelsOverride(profile),
	}
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
					MatchExpressions: profile.Spec.ProfileAffinity.ProfileNodeAffinity,
				},
			},
		},
	}

	return affinity
}

// affinityOverrideForDefaultProfile returns the affinity override that should
// be applied to the default profile. The default profile should be applied to
// all nodes that don't have the agent.datadoghq.com/profile label.
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
			}
		}
	}

	return res
}

func labelsOverride(profile *datadoghqv1alpha1.DatadogAgentProfile) map[string]string {
	if IsDefaultProfile(profile.Namespace, profile.Name) {
		return nil
	}

	return map[string]string{
		// Can't use the namespaced name because it includes "/" which is not
		// accepted in labels.
		ProfileLabelKey: fmt.Sprintf("%s-%s", profile.Namespace, profile.Name),
	}
}

// sortProfiles sorts the profiles by creation timestamp. If two profiles have
// the same creation timestamp, it sorts them by name.
func sortProfiles(profiles []datadoghqv1alpha1.DatadogAgentProfile) []datadoghqv1alpha1.DatadogAgentProfile {
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

func profileMatchesNode(profile *datadoghqv1alpha1.DatadogAgentProfile, node *v1.Node) (bool, error) {
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

		if !selector.Matches(labels.Set(node.Labels)) {
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
