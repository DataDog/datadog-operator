// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// ValidateDatadogAgentProfileSpec is used to check if a DatadogAgentProfileSpec is valid
func ValidateDatadogAgentProfileSpec(spec *DatadogAgentProfileSpec, datadogAgentInternalEnabled bool) error {
	if err := validateProfileAffinity(spec.ProfileAffinity); err != nil {
		return err
	}
	if err := validateConfig(spec.Config, datadogAgentInternalEnabled); err != nil {
		return err
	}

	return nil
}

func validateProfileAffinity(profileAffinity *ProfileAffinity) error {
	if profileAffinity == nil {
		return undefinedError("profileAffinity")
	}
	if profileAffinity.ProfileNodeAffinity == nil {
		return undefinedError("profileNodeAffinity")
	}
	if len(profileAffinity.ProfileNodeAffinity) < 1 {
		return fmt.Errorf("profileNodeAffinity must have at least 1 requirement")
	}

	return nil
}

func validateConfig(spec *v2alpha1.DatadogAgentSpec, datadogAgentInternalEnabled bool) error {
	if spec == nil {
		return undefinedError("config")
	}
	if err := validateFeatures(spec.Features, datadogAgentInternalEnabled); err != nil {
		return err
	}
	// global is not supported
	if spec.Global != nil {
		return unsupportedError("global")
	}
	if !datadogAgentInternalEnabled && spec.Override == nil {
		return undefinedError("config override")
	}
	for component, override := range spec.Override {
		if err := validateOverride(component, override); err != nil {
			return err
		}
	}

	return nil
}

func validateFeatures(features *v2alpha1.DatadogFeatures, datadogAgentInternalEnabled bool) error {
	if features == nil {
		return nil
	}
	if !datadogAgentInternalEnabled {
		return fmt.Errorf("the 'features' field is only supported when DatadogAgentInternal is enabled")
	}

	validFeatures := map[string]struct{}{
		"gpu": {},
	}

	v := reflect.ValueOf(features).Elem()
	t := v.Type()

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		fieldValue := v.Field(i)

		if jsonTag, ok := field.Tag.Lookup("json"); ok {
			// Use feature name from json tag
			featureName := jsonTag
			if idx := strings.Index(jsonTag, ","); idx != -1 {
				featureName = jsonTag[:idx]
			}

			// Skip valid features
			if _, valid := validFeatures[featureName]; valid {
				continue
			}

			if !fieldValue.IsNil() {
				return unsupportedError(featureName)
			}
		}
	}

	return nil
}

func validateOverride(component v2alpha1.ComponentName, override *v2alpha1.DatadogAgentComponentOverride) error {
	if component != v2alpha1.NodeAgentComponentName {
		return fmt.Errorf("only node agent componentoverrides are supported")
	}

	if override.Name != nil {
		return unsupportedError("component name")
	}
	if override.Replicas != nil {
		return unsupportedError("component replicas")
	}
	if override.CreatePodDisruptionBudget != nil {
		return unsupportedError("component create pod disruption budget")
	}
	if override.CreateRbac != nil {
		return unsupportedError("component create rbac")
	}
	if override.ServiceAccountName != nil {
		return unsupportedError("component service account name")
	}
	if override.ServiceAccountAnnotations != nil {
		return unsupportedError("component service account annotations")
	}
	if override.Image != nil {
		return unsupportedError("component image")
	}
	if override.Env != nil {
		return unsupportedError("component env")
	}
	if override.EnvFrom != nil {
		return unsupportedError("component env from")
	}
	if override.CustomConfigurations != nil {
		return unsupportedError("component custom configurations")
	}
	if override.ExtraConfd != nil {
		return unsupportedError("component extra confd")
	}
	if override.ExtraChecksd != nil {
		return unsupportedError("component extra checksd")
	}
	for name, override := range override.Containers {
		if err := validateContainerOverride(name, override); err != nil {
			return err
		}
	}
	if override.Volumes != nil {
		return unsupportedError("component volumes")
	}
	if override.SecurityContext != nil {
		return unsupportedError("component security context")
	}
	if override.Affinity != nil {
		return unsupportedError("component affinity")
	}
	if override.DNSPolicy != nil {
		return unsupportedError("component dns policy")
	}
	if override.DNSConfig != nil {
		return unsupportedError("component dns config")
	}
	if override.NodeSelector != nil {
		return unsupportedError("component node selector")
	}
	if override.Tolerations != nil {
		return unsupportedError("component tolerations")
	}
	if override.Annotations != nil {
		return unsupportedError("component annotations")
	}
	if override.HostNetwork != nil {
		return unsupportedError("component host network")
	}
	if override.HostPID != nil {
		return unsupportedError("component host pid")
	}
	if override.Disabled != nil {
		return unsupportedError("component disabled")
	}

	return nil
}

func validateContainerOverride(name common.AgentContainerName, override *v2alpha1.DatadogAgentGenericContainer) error {
	supportedContainers := map[common.AgentContainerName]struct{}{
		common.CoreAgentContainerName:      {},
		common.TraceAgentContainerName:     {},
		common.ProcessAgentContainerName:   {},
		common.SecurityAgentContainerName:  {},
		common.SystemProbeContainerName:    {},
		common.OtelAgent:                   {},
		common.AgentDataPlaneContainerName: {},
	}
	if _, ok := supportedContainers[name]; !ok {
		return unsupportedError(fmt.Sprintf("container %s", name))
	}

	if override.Name != nil {
		return unsupportedError("container name")
	}
	if override.LogLevel != nil {
		return unsupportedError("container log level")
	}
	if override.VolumeMounts != nil {
		return unsupportedError("container volume mounts")
	}
	if override.Command != nil {
		return unsupportedError("container command")
	}
	if override.Args != nil {
		return unsupportedError("container args")
	}
	if override.HealthPort != nil {
		return unsupportedError("container health port")
	}
	if override.ReadinessProbe != nil {
		return unsupportedError("container readiness probe")
	}
	if override.LivenessProbe != nil {
		return unsupportedError("container liveness probe")
	}
	if override.StartupProbe != nil {
		return unsupportedError("container startup probe")
	}
	if override.SecurityContext != nil {
		return unsupportedError("container security context")
	}
	if override.SeccompConfig != nil {
		return unsupportedError("container seccomp config")
	}
	if override.AppArmorProfileName != nil {
		return unsupportedError("container app armor profile name")
	}

	return nil
}

func unsupportedError(config string) error {
	return fmt.Errorf("%s override is not supported", config)
}

func undefinedError(config string) error {
	return fmt.Errorf("%s must be defined", config)
}
