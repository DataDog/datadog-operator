// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentprofile

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
)

const (
	defaultProfileName  = "default"
	daemonSetNamePrefix = "datadog-agent-with-profile-"
)

// ProfilesToApply given a list of profiles, returns the ones that should be
// applied in the cluster.
// - If there are no profiles, it returns the default profile.
// - If there are no conflicting profiles, it returns all the profiles plus the default one.
// - If there are conflicting profiles, it returns a subset that does not
// conflict plus the default one. When there are conflicting profiles, the
// oldest one is the one that takes precedence.
func ProfilesToApply(profiles []datadoghqv1alpha1.DatadogAgentProfile) []datadoghqv1alpha1.DatadogAgentProfile {
	var res []datadoghqv1alpha1.DatadogAgentProfile

	// TODO: detect conflicts here and only add the ones that are not
	// conflicting (Give precedence to the oldest profile).
	// This function will need the list of hosts to check for conflicts.
	res = append(res, profiles...)

	return append(res, defaultProfile(res))
}

// ComponentOverrideFromProfile returns the component override that should be
// applied according to the given profile.
func ComponentOverrideFromProfile(profile *datadoghqv1alpha1.DatadogAgentProfile) v2alpha1.DatadogAgentComponentOverride {
	overrideDSName := DaemonSetName(profile.Name)

	return v2alpha1.DatadogAgentComponentOverride{
		Name:       &overrideDSName,
		Affinity:   affinityOverride(profile),
		Containers: containersOverride(profile),
	}
}

// DaemonSetName returns the name that the DaemonSet should have according to
// the name of the profile associated with it.
func DaemonSetName(profileName string) string {
	if profileName == defaultProfileName {
		return "" // Return empty so it does not override the default DaemonSet name
	}

	return daemonSetNamePrefix + profileName
}

// defaultProfile returns the default profile, which is the one to be applied in
// the nodes where none of the profiles received apply.
// Note: this function assumes that the profiles received do not conflict.
func defaultProfile(profiles []datadoghqv1alpha1.DatadogAgentProfile) datadoghqv1alpha1.DatadogAgentProfile {
	var nodeSelectorRequirements []v1.NodeSelectorRequirement

	// TODO: I think this strategy only works if there's only one node selector per profile.
	for _, profile := range profiles {
		if profile.Spec.ProfileAffinity != nil {
			for _, nodeSelectorRequirement := range profile.Spec.ProfileAffinity.ProfileNodeAffinity {
				nodeSelectorRequirements = append(
					nodeSelectorRequirements,
					v1.NodeSelectorRequirement{
						Key:      nodeSelectorRequirement.Key,
						Operator: oppositeOperator(nodeSelectorRequirement.Operator),
						Values:   nodeSelectorRequirement.Values,
					},
				)
			}
		}
	}

	profile := datadoghqv1alpha1.DatadogAgentProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name: defaultProfileName,
		},
	}

	if len(nodeSelectorRequirements) > 0 {
		profile.Spec.ProfileAffinity = &datadoghqv1alpha1.ProfileAffinity{
			ProfileNodeAffinity: nodeSelectorRequirements,
		}
	}

	return profile
}

func oppositeOperator(op v1.NodeSelectorOperator) v1.NodeSelectorOperator {
	switch op {
	case v1.NodeSelectorOpIn:
		return v1.NodeSelectorOpNotIn
	case v1.NodeSelectorOpNotIn:
		return v1.NodeSelectorOpIn
	case v1.NodeSelectorOpExists:
		return v1.NodeSelectorOpDoesNotExist
	case v1.NodeSelectorOpDoesNotExist:
		return v1.NodeSelectorOpExists
	case v1.NodeSelectorOpGt:
		return v1.NodeSelectorOpLt
	case v1.NodeSelectorOpLt:
		return v1.NodeSelectorOpGt
	default:
		return ""
	}
}

func affinityOverride(profile *datadoghqv1alpha1.DatadogAgentProfile) *v1.Affinity {
	if profile.Spec.ProfileAffinity == nil || len(profile.Spec.ProfileAffinity.ProfileNodeAffinity) == 0 {
		return nil
	}

	return &v1.Affinity{
		NodeAffinity: &v1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &v1.NodeSelector{
				NodeSelectorTerms: []v1.NodeSelectorTerm{
					{
						MatchExpressions: profile.Spec.ProfileAffinity.ProfileNodeAffinity,
					},
				},
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
