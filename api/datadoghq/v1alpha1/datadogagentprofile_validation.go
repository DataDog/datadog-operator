// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"fmt"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// ValidateDatadogAgentProfileSpec is used to check if a DatadogAgentProfileSpec is valid
func ValidateDatadogAgentProfileSpec(spec *DatadogAgentProfileSpec) error {
	if err := validateProfileAffinity(spec.ProfileAffinity); err != nil {
		return err
	}
	if err := validateConfig(spec.Config); err != nil {
		return err
	}

	return nil
}

func validateProfileAffinity(profileAffinity *ProfileAffinity) error {
	if profileAffinity == nil {
		return fmt.Errorf("profileAffinity must be defined")
	}
	if profileAffinity.ProfileNodeAffinity == nil {
		return fmt.Errorf("profileNodeAffinity must be defined")
	}
	if len(profileAffinity.ProfileNodeAffinity) < 1 {
		return fmt.Errorf("profileNodeAffinity must have at least 1 requirement")
	}

	return nil
}

func validateConfig(spec *v2alpha1.DatadogAgentSpec) error {
	if spec == nil {
		return fmt.Errorf("config must be defined")
	}
	// features are not supported
	if spec.Features != nil {
		return fmt.Errorf("feature overrides are not supported")
	}
	// global is not supported
	if spec.Global != nil {
		return fmt.Errorf("global overrides are not supported")
	}
	if spec.Override == nil {
		return fmt.Errorf("config override must be defined")
	}
	for component, override := range spec.Override {
		if err := validateOverride(component, override); err != nil {
			return err
		}
	}

	return nil
}

func validateOverride(component v2alpha1.ComponentName, override *v2alpha1.DatadogAgentComponentOverride) error {
	if component != v2alpha1.NodeAgentComponentName {
		return fmt.Errorf("only node agent componentoverrides are supported")
	}

	if override.Name != nil {
		return fmt.Errorf("component name override is not supported")
	}
	if override.Replicas != nil {
		return fmt.Errorf("component replicas override is not supported")
	}
	if override.CreatePodDisruptionBudget != nil {
		return fmt.Errorf("component create pod disruption budget override is not supported")
	}
	if override.CreateRbac != nil {
		return fmt.Errorf("component create rbac override is not supported")
	}
	if override.ServiceAccountName != nil {
		return fmt.Errorf("component service account name override is not supported")
	}
	if override.ServiceAccountAnnotations != nil {
		return fmt.Errorf("component service account annotations override is not supported")
	}
	if override.Image != nil {
		return fmt.Errorf("component image override is not supported")
	}
	if override.Env != nil {
		return fmt.Errorf("component env override is not supported")
	}
	if override.EnvFrom != nil {
		return fmt.Errorf("component env from override is not supported")
	}
	if override.CustomConfigurations != nil {
		return fmt.Errorf("component custom configurations override is not supported")
	}
	if override.ExtraConfd != nil {
		return fmt.Errorf("component extra confd override is not supported")
	}
	if override.ExtraChecksd != nil {
		return fmt.Errorf("component extra checksd override is not supported")
	}
	for name, override := range override.Containers {
		if err := validateContainerOverride(name, override); err != nil {
			return err
		}
	}
	if override.Volumes != nil {
		return fmt.Errorf("component volumes override is not supported")
	}
	if override.SecurityContext != nil {
		return fmt.Errorf("component security context override is not supported")
	}
	if override.Affinity != nil {
		return fmt.Errorf("component affinity override is not supported")
	}
	if override.DNSPolicy != nil {
		return fmt.Errorf("component dns policy override is not supported")
	}
	if override.DNSConfig != nil {
		return fmt.Errorf("component dns config override is not supported")
	}
	if override.NodeSelector != nil {
		return fmt.Errorf("component node selector override is not supported")
	}
	if override.Tolerations != nil {
		return fmt.Errorf("component tolerations override is not supported")
	}
	if override.Annotations != nil {
		return fmt.Errorf("component annotations override is not supported")
	}
	if override.HostNetwork != nil {
		return fmt.Errorf("component host network override is not supported")
	}
	if override.HostPID != nil {
		return fmt.Errorf("component host pid override is not supported")
	}
	if override.Disabled != nil {
		return fmt.Errorf("component disabled override is not supported")
	}

	return nil
}

func validateContainerOverride(name common.AgentContainerName, override *v2alpha1.DatadogAgentGenericContainer) error {
	supportedContainers := map[common.AgentContainerName]struct{}{
		common.CoreAgentContainerName:     {},
		common.TraceAgentContainerName:    {},
		common.ProcessAgentContainerName:  {},
		common.SecurityAgentContainerName: {},
		common.SystemProbeContainerName:   {},
	}
	if _, ok := supportedContainers[name]; !ok {
		return fmt.Errorf("container %s override is not supported", name)
	}

	if override.Name != nil {
		return fmt.Errorf("container name override is not supported")
	}
	if override.LogLevel != nil {
		return fmt.Errorf("container log level override is not supported")
	}
	if override.VolumeMounts != nil {
		return fmt.Errorf("container volume mounts override is not supported")
	}
	if override.Command != nil {
		return fmt.Errorf("container command override is not supported")
	}
	if override.Args != nil {
		return fmt.Errorf("container args override is not supported")
	}
	if override.HealthPort != nil {
		return fmt.Errorf("container health port override is not supported")
	}
	if override.ReadinessProbe != nil {
		return fmt.Errorf("container readiness probe override is not supported")
	}
	if override.LivenessProbe != nil {
		return fmt.Errorf("container liveness probe override is not supported")
	}
	if override.StartupProbe != nil {
		return fmt.Errorf("container startup probe override is not supported")
	}
	if override.SecurityContext != nil {
		return fmt.Errorf("container security context override is not supported")
	}
	if override.SeccompConfig != nil {
		return fmt.Errorf("container seccomp config override is not supported")
	}
	if override.AppArmorProfileName != nil {
		return fmt.Errorf("container app armor profile name override is not supported")
	}

	return nil
}
