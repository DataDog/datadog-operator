// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apm

import (
	"fmt"
	"slices"

	"k8s.io/utils/ptr"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

// applyAPMProfileSharedConfigOverlay merges explicit shared profile contributions.
// validationSpec contains accepted profile values only, never DDA defaults.
func applyAPMProfileSharedConfigOverlay(validationSpec, defaultDDAISpec, originalDefaultDDAISpec, profileSpec *v2alpha1.DatadogAgentSpec) error {
	if profileSpec == nil || profileSpec.Features == nil || profileSpec.Features.APM == nil {
		return nil
	}
	profileAPM := profileSpec.Features.APM
	if err := mergeProfileLocalAgentServicePort(validationSpec, profileAPM); err != nil {
		return err
	}
	profileSSI := profileAPM.SingleStepInstrumentation
	if profileSSI == nil {
		return nil
	}
	if profileSSI.Enabled != nil && *profileSSI.Enabled {
		if err := validateSSISharedComponentPrerequisites(originalDefaultDDAISpec); err != nil {
			return err
		}
	}
	if detection := profileSSI.LanguageDetection; detection != nil && ptr.Deref(detection.Enabled, false) && originalDefaultDDAISpec != nil {
		if ca := originalDefaultDDAISpec.Override[v2alpha1.ClusterAgentComponentName]; ca != nil && ptr.Deref(ca.Disabled, false) {
			return fmt.Errorf("clusterAgent cannot be disabled on the base DatadogAgent when language detection is enabled")
		}
	}
	if len(profileSSI.Targets) > 0 && !supportsInstrumentationTargets(originalDefaultDDAISpec) {
		return fmt.Errorf("features.apm.instrumentation.targets requires Cluster Agent version >= %s", minInstrumentationTargetsVersion)
	}
	return mergeSSI(baseSSIForProfileOverlay(validationSpec), baseSSIForProfileOverlay(defaultDDAISpec), profileSSI)
}

func sharedAPMConfig(spec *v2alpha1.DatadogAgentSpec) *v2alpha1.APMFeatureConfig {
	if spec.Features == nil {
		spec.Features = &v2alpha1.DatadogFeatures{}
	}
	if spec.Features.APM == nil {
		spec.Features.APM = &v2alpha1.APMFeatureConfig{}
	}
	return spec.Features.APM
}

// Host-port enablement controls the node. Only an explicit port contributes a
// shared Service-port choice, including when the contributing node disables APM.
func mergeProfileLocalAgentServicePort(validationSpec *v2alpha1.DatadogAgentSpec, profileAPM *v2alpha1.APMFeatureConfig) error {
	if profileAPM.HostPortConfig == nil || profileAPM.HostPortConfig.Port == nil {
		return nil
	}
	port := *profileAPM.HostPortConfig.Port
	raw := sharedAPMConfig(validationSpec)
	if raw.HostPortConfig != nil && raw.HostPortConfig.Port != nil && *raw.HostPortConfig.Port != port {
		return fmt.Errorf("local Agent Service port %q conflicts with existing port", constants.DefaultApmPortName)
	}
	raw.HostPortConfig = &v2alpha1.HostPortConfig{Port: new(port)}
	return nil
}

func validateSSISharedComponentPrerequisites(spec *v2alpha1.DatadogAgentSpec) error {
	// SSI requires the Cluster Agent and admission controller.
	if spec == nil || spec.Features == nil || spec.Features.AdmissionController == nil || !ptr.Deref(spec.Features.AdmissionController.Enabled, false) {
		return fmt.Errorf("features.admissionController.enabled must be true on the base DatadogAgent when APM instrumentation is configured")
	}
	if clusterAgent := spec.Override[v2alpha1.ClusterAgentComponentName]; clusterAgent != nil && ptr.Deref(clusterAgent.Disabled, false) {
		return fmt.Errorf("clusterAgent cannot be disabled on the base DatadogAgent when APM instrumentation is configured")
	}
	return nil
}

func baseSSIForProfileOverlay(dst *v2alpha1.DatadogAgentSpec) *v2alpha1.SingleStepInstrumentation {
	apm := sharedAPMConfig(dst)
	if apm.SingleStepInstrumentation == nil {
		apm.SingleStepInstrumentation = &v2alpha1.SingleStepInstrumentation{}
	}
	return apm.SingleStepInstrumentation
}

// mergeSSI checks raw values and writes accepted fields to both SSI configs.
func mergeSSI(raw, dst, src *v2alpha1.SingleStepInstrumentation) error {
	if src.Enabled != nil {
		if raw.Enabled != nil && *raw.Enabled != *src.Enabled {
			return fmt.Errorf("features.apm.instrumentation.enabled has conflicting values")
		}
		raw.Enabled = new(*src.Enabled)
		dst.Enabled = new(*src.Enabled)
	}
	if err := mergeOnDemand(raw, dst, src); err != nil {
		return err
	}
	if src.EnabledNamespaces != nil || src.DisabledNamespaces != nil {
		if raw.EnabledNamespaces == nil && raw.DisabledNamespaces == nil {
			dst.EnabledNamespaces = nil
			dst.DisabledNamespaces = nil
		}
		raw.EnabledNamespaces = appendDeduplicateStrings(raw.EnabledNamespaces, src.EnabledNamespaces)
		raw.DisabledNamespaces = appendDeduplicateStrings(raw.DisabledNamespaces, src.DisabledNamespaces)
		if len(raw.EnabledNamespaces) > 0 && len(raw.DisabledNamespaces) > 0 {
			return fmt.Errorf("features.apm.instrumentation.enabledNamespaces and features.apm.instrumentation.disabledNamespaces cannot both be set")
		}
		dst.EnabledNamespaces = slices.Clone(raw.EnabledNamespaces)
		dst.DisabledNamespaces = slices.Clone(raw.DisabledNamespaces)
	}
	if err := mergeStringMap(&raw.LibVersions, &dst.LibVersions, src.LibVersions, "features.apm.instrumentation.libVersions"); err != nil {
		return err
	}
	if err := mergeLanguageDetection(raw, dst, src); err != nil {
		return err
	}
	if err := mergeInjector(raw, dst, src); err != nil {
		return err
	}
	if err := mergeStringLikeField(&raw.InjectionMode, &dst.InjectionMode, src.InjectionMode, "features.apm.instrumentation.injectionMode"); err != nil {
		return err
	}
	// The first profile target list overrides the base; subsequent profiles append.
	if src.Targets != nil && raw.Targets == nil {
		dst.Targets = nil
	}
	// Preserve target order because the first matching target wins.
	for _, target := range src.Targets {
		raw.Targets = append(raw.Targets, *target.DeepCopy())
		dst.Targets = append(dst.Targets, *target.DeepCopy())
	}
	return nil
}

func appendDeduplicateStrings(dst []string, src []string) []string {
	if len(src) == 0 {
		return dst
	}
	out := make([]string, 0, len(dst)+len(src))
	seen := make(map[string]bool, len(dst)+len(src))
	for _, values := range [][]string{dst, src} {
		for _, value := range values {
			if !seen[value] {
				seen[value] = true
				out = append(out, value)
			}
		}
	}
	return out
}

// mergeStringMap checks raw keys, then copies each value to both maps.
func mergeStringMap(raw, dst *map[string]string, src map[string]string, field string) error {
	if len(src) == 0 {
		return nil
	}
	if *raw == nil {
		*raw = map[string]string{}
	}
	if *dst == nil {
		*dst = map[string]string{}
	}
	for key, value := range src {
		if existing, ok := (*raw)[key]; ok && existing != value {
			return fmt.Errorf("%s[%q] has conflicting values %q and %q", field, key, existing, value)
		}
		(*raw)[key] = value
		(*dst)[key] = value
	}
	return nil
}

// mergeLanguageDetection applies explicit profile values without defaulting omissions.
func mergeLanguageDetection(raw, dst, src *v2alpha1.SingleStepInstrumentation) error {
	config := src.LanguageDetection
	if config == nil || config.Enabled == nil {
		return nil
	}
	if existing := raw.LanguageDetection; existing != nil && existing.Enabled != nil && *existing.Enabled != *config.Enabled {
		return fmt.Errorf("features.apm.instrumentation.languageDetection.enabled has conflicting values")
	}
	raw.LanguageDetection = config.DeepCopy()
	dst.LanguageDetection = config.DeepCopy()
	return nil
}

func mergeInjector(raw, dst, src *v2alpha1.SingleStepInstrumentation) error {
	if src.Injector == nil {
		return nil
	}
	if raw.Injector == nil {
		raw.Injector = &v2alpha1.InjectorConfig{}
	}
	if dst.Injector == nil {
		dst.Injector = &v2alpha1.InjectorConfig{}
	}
	return mergeStringLikeField(&raw.Injector.ImageTag, &dst.Injector.ImageTag, src.Injector.ImageTag, "features.apm.instrumentation.injector.imageTag")
}

// mergeOnDemand checks explicit values before updating both configs.
func mergeOnDemand(raw, dst, src *v2alpha1.SingleStepInstrumentation) error {
	if src.OnDemand == nil {
		return nil
	}
	if raw.OnDemand != nil && *raw.OnDemand != *src.OnDemand {
		return fmt.Errorf("features.apm.instrumentation.onDemand has conflicting values")
	}
	raw.OnDemand = new(*src.OnDemand)
	dst.OnDemand = new(*src.OnDemand)
	return nil
}

// mergeStringLikeField checks a non-empty value and copies it to both configs.
func mergeStringLikeField[T ~string](raw, dst *T, src T, field string) error {
	var zero T
	if src == zero {
		return nil
	}
	if *raw != zero && *raw != src {
		return fmt.Errorf("%s has conflicting values %q and %q", field, *raw, src)
	}
	*raw = src
	*dst = src
	return nil
}
