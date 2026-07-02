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

func applyAPMProfileSharedConfigOverlay(dst, base *v2alpha1.DatadogAgentSpec, profileSpec *v2alpha1.DatadogAgentSpec) error {
	if err := applyProfileLocalAgentServicePort(dst, base, profileSpec); err != nil {
		return err
	}

	// Only profiles that explicitly enable SSI contribute shared Cluster Agent
	// config. SSI enabled=false is treated as "no shared overlay".
	profileSSI, ok := profileSSIOverlayConfig(profileSpec)
	if !ok {
		return nil
	}

	if profileSpec.Features.APM.Enabled != nil && !ptr.Deref(profileSpec.Features.APM.Enabled, false) {
		return fmt.Errorf("features.apm.enabled must be true or unset when features.apm.instrumentation.enabled is true")
	}
	if err := validateSSISharedComponentPrerequisites(dst); err != nil {
		return err
	}
	if len(profileSSI.Targets) > 0 && !supportsInstrumentationTargets(dst) {
		return fmt.Errorf("features.apm.instrumentation.targets requires Cluster Agent version >= %s", minInstrumentationTargetsVersion)
	}

	dstSSI := baseSSIForProfileOverlay(dst)
	if err := mergeSSI(dstSSI, profileSSI); err != nil {
		return err
	}
	dstSSI.Enabled = ptr.To(true)
	defaultLanguageDetection(dstSSI)

	return nil
}

func applyProfileLocalAgentServicePort(dst, base, profileSpec *v2alpha1.DatadogAgentSpec) error {
	profilePort, ok := profileLocalAgentServicePort(profileSpec)
	if !ok {
		return nil
	}

	existingPort, ok := profileLocalAgentServicePort(base)
	if !ok {
		if dst != nil && dst.Features != nil && dst.Features.APM != nil {
			hostPort := dst.Features.APM.HostPortConfig
			if hostPort != nil && ptr.Deref(hostPort.Enabled, false) && hostPort.Port != nil {
				existingPort = *hostPort.Port
				ok = true
			}
		}
	}
	if ok && existingPort != profilePort {
		return fmt.Errorf("local Agent Service port %q conflicts with existing port", constants.DefaultApmPortName)
	}

	if dst.Features == nil {
		dst.Features = &v2alpha1.DatadogFeatures{}
	}
	if dst.Features.APM == nil {
		dst.Features.APM = &v2alpha1.APMFeatureConfig{}
	}
	dst.Features.APM.HostPortConfig = &v2alpha1.HostPortConfig{
		Enabled: ptr.To(true),
		Port:    ptr.To(profilePort),
	}
	return nil
}

func profileLocalAgentServicePort(spec *v2alpha1.DatadogAgentSpec) (int32, bool) {
	ports := apmLocalAgentServicePorts(nil, spec)
	if len(ports) == 0 {
		return 0, false
	}
	return ports[0].Port, true
}

// profileSSIOverlayConfig extracts the only APM profile config that affects a
// shared component. Other APM fields stay on the profile DDAI.
func profileSSIOverlayConfig(profileSpec *v2alpha1.DatadogAgentSpec) (*v2alpha1.SingleStepInstrumentation, bool) {
	if profileSpec == nil ||
		profileSpec.Features == nil ||
		profileSpec.Features.APM == nil ||
		profileSpec.Features.APM.SingleStepInstrumentation == nil {
		return nil, false
	}

	ssi := profileSpec.Features.APM.SingleStepInstrumentation
	if !ptr.Deref(ssi.Enabled, false) {
		return nil, false
	}

	return ssi, true
}

func validateSSISharedComponentPrerequisites(spec *v2alpha1.DatadogAgentSpec) error {
	// SSI is rendered on the Cluster Agent admission controller path, so
	// profile-contributed SSI is only valid when that shared path exists.
	if !admissionControllerEnabled(spec) {
		return fmt.Errorf("features.admissionController.enabled must be true on the base DatadogAgent when APM instrumentation is configured")
	}
	if defaultClusterAgentDisabled(spec) {
		return fmt.Errorf("clusterAgent cannot be disabled on the base DatadogAgent when APM instrumentation is configured")
	}
	return nil
}

func admissionControllerEnabled(spec *v2alpha1.DatadogAgentSpec) bool {
	return spec != nil &&
		spec.Features != nil &&
		spec.Features.AdmissionController != nil &&
		ptr.Deref(spec.Features.AdmissionController.Enabled, false)
}

func defaultClusterAgentDisabled(spec *v2alpha1.DatadogAgentSpec) bool {
	if spec == nil || spec.Override == nil || spec.Override[v2alpha1.ClusterAgentComponentName] == nil {
		return false
	}
	return ptr.Deref(spec.Override[v2alpha1.ClusterAgentComponentName].Disabled, false)
}

// baseSSIForProfileOverlay returns the SSI config that profile overlays should
// merge into. When base instrumentation is absent or explicitly disabled, the
// profile's enabled instrumentation is the first active SSI config, so discard
// the inactive base block before merging.
func baseSSIForProfileOverlay(dst *v2alpha1.DatadogAgentSpec) *v2alpha1.SingleStepInstrumentation {
	if dst.Features == nil {
		dst.Features = &v2alpha1.DatadogFeatures{}
	}
	if dst.Features.APM == nil {
		dst.Features.APM = &v2alpha1.APMFeatureConfig{}
	}
	if dst.Features.APM.SingleStepInstrumentation == nil ||
		!ptr.Deref(dst.Features.APM.SingleStepInstrumentation.Enabled, false) {
		dst.Features.APM.SingleStepInstrumentation = &v2alpha1.SingleStepInstrumentation{}
	}
	return dst.Features.APM.SingleStepInstrumentation
}

// mergeSSI applies the profile SSI union rules used for shared Cluster Agent
// config. Fields that describe sets are unioned; singleton fields reject
// conflicting values.
func mergeSSI(dst, src *v2alpha1.SingleStepInstrumentation) error {
	dst.EnabledNamespaces = appendDeduplicateStrings(dst.EnabledNamespaces, src.EnabledNamespaces)
	dst.DisabledNamespaces = appendDeduplicateStrings(dst.DisabledNamespaces, src.DisabledNamespaces)
	if len(dst.EnabledNamespaces) > 0 && len(dst.DisabledNamespaces) > 0 {
		return fmt.Errorf("features.apm.instrumentation.enabledNamespaces and features.apm.instrumentation.disabledNamespaces cannot both be set")
	}

	if err := mergeStringMap(&dst.LibVersions, src.LibVersions, "features.apm.instrumentation.libVersions"); err != nil {
		return err
	}
	if err := mergeLanguageDetection(dst, src); err != nil {
		return err
	}
	if err := mergeInjector(dst, src); err != nil {
		return err
	}
	if err := mergeInjectionMode(dst, src); err != nil {
		return err
	}
	if err := mergeSSITargets(&dst.Targets, src.Targets); err != nil {
		return err
	}

	return nil
}

func appendDeduplicateStrings(dst []string, src []string) []string {
	if len(src) == 0 {
		return dst
	}
	out := make([]string, 0, len(dst)+len(src))
	for _, value := range dst {
		if !slices.Contains(out, value) {
			out = append(out, value)
		}
	}
	for _, value := range src {
		if !slices.Contains(out, value) {
			out = append(out, value)
		}
	}
	return out
}

// mergeStringMap copies profile keys into the shared config and rejects a key
// that is already set to a different value.
func mergeStringMap(dst *map[string]string, src map[string]string, field string) error {
	if len(src) == 0 {
		return nil
	}
	if *dst == nil {
		*dst = map[string]string{}
	}
	for key, value := range src {
		if existing, ok := (*dst)[key]; ok && existing != value {
			return fmt.Errorf("%s[%q] has conflicting values %q and %q", field, key, existing, value)
		}
		(*dst)[key] = value
	}
	return nil
}

func mergeLanguageDetection(dst, src *v2alpha1.SingleStepInstrumentation) error {
	if src.LanguageDetection == nil || src.LanguageDetection.Enabled == nil {
		return nil
	}
	if dst.LanguageDetection == nil {
		dst.LanguageDetection = &v2alpha1.LanguageDetectionConfig{}
	}
	return mergeBoolPtr(&dst.LanguageDetection.Enabled, src.LanguageDetection.Enabled, "features.apm.instrumentation.languageDetection.enabled")
}

func defaultLanguageDetection(dst *v2alpha1.SingleStepInstrumentation) {
	if dst.LanguageDetection == nil {
		dst.LanguageDetection = &v2alpha1.LanguageDetectionConfig{}
	}
	if dst.LanguageDetection.Enabled == nil {
		dst.LanguageDetection.Enabled = ptr.To(true)
	}
}

func mergeInjector(dst, src *v2alpha1.SingleStepInstrumentation) error {
	if src.Injector == nil {
		return nil
	}
	if dst.Injector == nil {
		dst.Injector = &v2alpha1.InjectorConfig{}
	}
	return mergeStringLikeField(&dst.Injector.ImageTag, src.Injector.ImageTag, "features.apm.instrumentation.injector.imageTag")
}

func mergeInjectionMode(dst, src *v2alpha1.SingleStepInstrumentation) error {
	return mergeStringLikeField(&dst.InjectionMode, src.InjectionMode, "features.apm.instrumentation.injectionMode")
}

// mergeBoolPtr copies an explicit profile bool into the shared config and
// rejects an already-set bool with the opposite value.
func mergeBoolPtr(dst **bool, src *bool, field string) error {
	srcValue := ptr.Deref(src, false)
	if *dst != nil && ptr.Deref(*dst, false) != srcValue {
		return fmt.Errorf("%s has conflicting values", field)
	}
	*dst = ptr.To(srcValue)
	return nil
}

// mergeStringLikeField copies a non-empty profile scalar into the shared config
// and rejects an already-set scalar with a different value.
func mergeStringLikeField[T ~string](dst *T, src T, field string) error {
	var zero T
	if src == zero {
		return nil
	}
	if *dst != zero && *dst != src {
		return fmt.Errorf("%s has conflicting values %q and %q", field, *dst, src)
	}
	*dst = src
	return nil
}

// mergeSSITargets appends profile targets in profile order. The Cluster Agent
// uses first-match semantics when evaluating the final target list.
func mergeSSITargets(dst *[]v2alpha1.SSITarget, src []v2alpha1.SSITarget) error {
	for _, target := range src {
		*dst = append(*dst, *target.DeepCopy())
	}
	return nil
}
