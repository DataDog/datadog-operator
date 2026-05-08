// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apm

import (
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/utils/ptr"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

func applyAPMProfileSharedConfigOverlay(dst, _ *v2alpha1.DatadogAgentSpec, profileSpec *v2alpha1.DatadogAgentSpec) error {
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
	hasNamedTargets := hasNamedSSITarget(profileSSI.Targets)
	// Empty target names are ignored by the Cluster Agent. If a profile only
	// provided unnamed targets, leave the default DDAI unchanged.
	if len(profileSSI.Targets) > 0 && !hasNamedTargets && !hasNonTargetSSIConfig(profileSSI) {
		return nil
	}
	if hasNamedTargets {
		if err := validateSSITargetsSupported(dst); err != nil {
			return err
		}
	}

	// Reset disabled or absent base SSI before merging so synthetic defaults on
	// disabled SSI do not conflict with the profile's enabled SSI config.
	dstSSI := ensureDestinationSSI(dst)
	if err := mergeSSI(dstSSI, profileSSI); err != nil {
		return err
	}
	dstSSI.Enabled = ptr.To(true)
	defaultLanguageDetection(dstSSI)

	return nil
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

// ensureDestinationSSI prepares the destination default-DDAI SSI config for an
// overlay. Disabled SSI is intentionally replaced because disabled base config
// should not participate in the enabled profile merge.
func ensureDestinationSSI(dst *v2alpha1.DatadogAgentSpec) *v2alpha1.SingleStepInstrumentation {
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

func mergeBoolPtr(dst **bool, src *bool, field string) error {
	srcValue := ptr.Deref(src, false)
	if *dst != nil && ptr.Deref(*dst, false) != srcValue {
		return fmt.Errorf("%s has conflicting values", field)
	}
	*dst = ptr.To(srcValue)
	return nil
}

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

func mergeSSITargets(dst *[]v2alpha1.SSITarget, src []v2alpha1.SSITarget) error {
	for _, target := range src {
		// The Cluster Agent cannot address unnamed targets, so ignore them.
		if target.Name == "" {
			continue
		}
		idx := slices.IndexFunc(*dst, func(existing v2alpha1.SSITarget) bool {
			return existing.Name == target.Name
		})
		if idx == -1 {
			*dst = append(*dst, *target.DeepCopy())
			continue
		}
		if err := mergeSSITarget(&(*dst)[idx], &target); err != nil {
			return err
		}
	}
	return nil
}

func mergeSSITarget(dst, src *v2alpha1.SSITarget) error {
	// Target names define identity. Selectors must match for the same target so
	// two profiles cannot silently redefine where an existing target applies.
	if !apiequality.Semantic.DeepEqual(dst.PodSelector, src.PodSelector) {
		return fmt.Errorf("features.apm.instrumentation.targets[%q].podSelector has conflicting values", src.Name)
	}
	if !apiequality.Semantic.DeepEqual(dst.NamespaceSelector, src.NamespaceSelector) {
		return fmt.Errorf("features.apm.instrumentation.targets[%q].namespaceSelector has conflicting values", src.Name)
	}
	if err := mergeStringMap(&dst.TracerVersions, src.TracerVersions, fmt.Sprintf("features.apm.instrumentation.targets[%q].ddTraceVersions", src.Name)); err != nil {
		return err
	}
	return mergeEnvVarsByName(&dst.TracerConfigs, src.TracerConfigs, fmt.Sprintf("features.apm.instrumentation.targets[%q].ddTraceConfigs", src.Name))
}

func mergeEnvVarsByName(dst *[]corev1.EnvVar, src []corev1.EnvVar, field string) error {
	for _, env := range src {
		// Env var name defines identity. Reusing a name with different content is
		// ambiguous, especially when ValueFrom is involved, so reject it.
		idx := slices.IndexFunc(*dst, func(existing corev1.EnvVar) bool {
			return existing.Name == env.Name
		})
		if idx == -1 {
			*dst = append(*dst, *env.DeepCopy())
			continue
		}
		if !apiequality.Semantic.DeepEqual((*dst)[idx], env) {
			return fmt.Errorf("%s[%q] has conflicting values", field, env.Name)
		}
	}
	return nil
}

func hasNamedSSITarget(targets []v2alpha1.SSITarget) bool {
	for _, target := range targets {
		if target.Name != "" {
			return true
		}
	}
	return false
}

func hasNonTargetSSIConfig(ssi *v2alpha1.SingleStepInstrumentation) bool {
	// Used to distinguish "only unnamed targets" from a real overlay that also
	// happens to include unnamed targets.
	return len(ssi.EnabledNamespaces) > 0 ||
		len(ssi.DisabledNamespaces) > 0 ||
		len(ssi.LibVersions) > 0 ||
		ssi.LanguageDetection != nil ||
		(ssi.Injector != nil && ssi.Injector.ImageTag != "") ||
		ssi.InjectionMode != ""
}
