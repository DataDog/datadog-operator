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

// applyAPMProfileSharedConfigOverlay checks explicit values in validationSpec
// and applies shared settings to defaultDDAISpec. Only the DDAI gets defaults.
func applyAPMProfileSharedConfigOverlay(validationSpec, defaultDDAISpec, originalDefaultDDAISpec, profileSpec *v2alpha1.DatadogAgentSpec) error {
	if profileSpec == nil || profileSpec.Features == nil || profileSpec.Features.APM == nil {
		return nil
	}
	profileAPM := profileSpec.Features.APM
	if err := mergeProfileLocalAgentServicePort(validationSpec, defaultDDAISpec, originalDefaultDDAISpec, profileAPM); err != nil {
		return err
	}
	profileSSI := profileAPM.SingleStepInstrumentation
	if profileSSI == nil || !ptr.Deref(profileSSI.Enabled, false) {
		return nil
	}
	if profileAPM.Enabled != nil && !ptr.Deref(profileAPM.Enabled, false) {
		return fmt.Errorf("features.apm.enabled must be true or unset when features.apm.instrumentation.enabled is true")
	}
	if err := validateSSISharedComponentPrerequisites(originalDefaultDDAISpec); err != nil {
		return err
	}
	if len(profileSSI.Targets) > 0 && !supportsInstrumentationTargets(originalDefaultDDAISpec) {
		return fmt.Errorf("features.apm.instrumentation.targets requires Cluster Agent version >= %s", minInstrumentationTargetsVersion)
	}

	rawSSI := baseSSIForProfileOverlay(validationSpec)
	ssi := baseSSIForProfileOverlay(defaultDDAISpec)
	if err := mergeSSI(rawSSI, ssi, profileSSI); err != nil {
		return err
	}
	rawSSI.Enabled = new(true)
	ssi.Enabled = new(true)
	return nil
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

// profileAPM is non-nil; the overlay entry point checks it once.
func mergeProfileLocalAgentServicePort(validationSpec, defaultDDAISpec, originalDefaultDDAISpec *v2alpha1.DatadogAgentSpec, profileAPM *v2alpha1.APMFeatureConfig) error {
	// Inherit APM enablement without changing the profile's explicit settings.
	var baseAPM *v2alpha1.APMFeatureConfig
	if originalDefaultDDAISpec != nil && originalDefaultDDAISpec.Features != nil {
		baseAPM = originalDefaultDDAISpec.Features.APM
	}
	effectiveAPM := *profileAPM
	// SSI and standalone error tracking can enable APM without an explicit flag.
	if effectiveAPM.Enabled == nil && !shouldEnableAPM(profileAPM) && baseAPM != nil {
		effectiveAPM.Enabled = baseAPM.Enabled
	}
	if !shouldEnableAPM(&effectiveAPM) {
		return nil
	}
	port := configuredServicePort(profileAPM)
	var existing *int32
	if validationSpec.Features != nil {
		existing = configuredServicePort(validationSpec.Features.APM)
	}
	basePort, baseHasService := profileLocalAgentServicePort(originalDefaultDDAISpec)
	if port != nil && existing != nil && *port != *existing {
		return fmt.Errorf("local Agent Service port %q conflicts with existing port", constants.DefaultApmPortName)
	}
	if port == nil && (existing != nil || baseHasService) {
		return nil
	}
	hostPort := &v2alpha1.HostPortConfig{Enabled: new(true)}
	if port != nil {
		// Preserve disabled host ports when the Service already uses this port.
		if baseHasService && basePort == *port && baseAPM.HostPortConfig != nil {
			hostPort.Enabled = new(ptr.Deref(baseAPM.HostPortConfig.Enabled, false))
		}
		hostPort.Port = new(*port)
	}
	sharedAPMConfig(validationSpec).HostPortConfig = hostPort
	// Resolve the Service port on the DDAI only; omitted ports in validationSpec stay unset.
	resolved := hostPort.DeepCopy()
	if resolved.Port == nil {
		resolved.Port = new(int32(constants.DefaultApmPort))
	}
	sharedAPMConfig(defaultDDAISpec).HostPortConfig = resolved
	return nil
}

// configuredServicePort reads a configured Service port, without adding defaults.
// APM may be disabled on the DDA while profiles use the shared Service.
func configuredServicePort(apm *v2alpha1.APMFeatureConfig) *int32 {
	if apm == nil || apm.HostPortConfig == nil {
		return nil
	}
	hostPort := apm.HostPortConfig
	// With host ports disabled, only 8126 is used by the Service.
	if !ptr.Deref(hostPort.Enabled, false) && (!ptr.Deref(apm.Enabled, true) || ptr.Deref(hostPort.Port, int32(0)) != constants.DefaultApmPort) {
		return nil
	}
	return hostPort.Port
}

func profileLocalAgentServicePort(spec *v2alpha1.DatadogAgentSpec) (int32, bool) {
	ports := apmLocalAgentServicePorts(nil, spec)
	if len(ports) == 0 {
		return 0, false
	}
	return ports[0].Port, true
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

// baseSSIForProfileOverlay clears inactive SSI settings before merging
// the first profile that enables SSI.
func baseSSIForProfileOverlay(dst *v2alpha1.DatadogAgentSpec) *v2alpha1.SingleStepInstrumentation {
	apm := sharedAPMConfig(dst)
	if apm.SingleStepInstrumentation == nil || !ptr.Deref(apm.SingleStepInstrumentation.Enabled, false) {
		// On-demand applies even when cluster-wide SSI is disabled.
		var onDemand *bool
		if apm.SingleStepInstrumentation != nil {
			onDemand = apm.SingleStepInstrumentation.OnDemand
		}
		apm.SingleStepInstrumentation = &v2alpha1.SingleStepInstrumentation{OnDemand: onDemand}
	}
	return apm.SingleStepInstrumentation
}

// mergeSSI checks raw values and writes accepted fields to both SSI configs.
func mergeSSI(raw, dst, src *v2alpha1.SingleStepInstrumentation) error {
	if err := mergeOnDemand(raw, dst, src); err != nil {
		return err
	}
	raw.EnabledNamespaces = appendDeduplicateStrings(raw.EnabledNamespaces, src.EnabledNamespaces)
	raw.DisabledNamespaces = appendDeduplicateStrings(raw.DisabledNamespaces, src.DisabledNamespaces)
	if len(raw.EnabledNamespaces) > 0 && len(raw.DisabledNamespaces) > 0 {
		return fmt.Errorf("features.apm.instrumentation.enabledNamespaces and features.apm.instrumentation.disabledNamespaces cannot both be set")
	}
	dst.EnabledNamespaces = slices.Clone(raw.EnabledNamespaces)
	dst.DisabledNamespaces = slices.Clone(raw.DisabledNamespaces)
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

// mergeLanguageDetection checks explicit values and defaults only the DDAI.
func mergeLanguageDetection(raw, dst, src *v2alpha1.SingleStepInstrumentation) error {
	if config := src.LanguageDetection; config != nil && config.Enabled != nil {
		if existing := raw.LanguageDetection; existing != nil && existing.Enabled != nil && *existing.Enabled != *config.Enabled {
			return fmt.Errorf("features.apm.instrumentation.languageDetection.enabled has conflicting values")
		}
		raw.LanguageDetection = config.DeepCopy()
		dst.LanguageDetection = config.DeepCopy()
	}
	if dst.LanguageDetection == nil {
		dst.LanguageDetection = &v2alpha1.LanguageDetectionConfig{}
	}
	if dst.LanguageDetection.Enabled == nil {
		dst.LanguageDetection.Enabled = new(true)
	}
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

// withSharedServicePort copies the DDAI spec with the chosen shared Service port.
// The original node Agent settings stay unchanged.
func withSharedServicePort(spec, shared *v2alpha1.DatadogAgentSpec) *v2alpha1.DatadogAgentSpec {
	if shared == nil || shared.Features == nil {
		return spec
	}
	port := configuredServicePort(shared.Features.APM)
	if port == nil {
		return spec
	}
	dependencySpec := spec.DeepCopy()
	sharedAPMConfig(dependencySpec).HostPortConfig = &v2alpha1.HostPortConfig{
		Enabled: new(true),
		Port:    new(*port),
	}
	return dependencySpec
}
