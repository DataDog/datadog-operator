// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package providers declares what each provider changes about the agent
// configuration, so config a provider cannot use is never added instead of
// added and then removed.
package providers

import (
	"fmt"
	"maps"

	corev1 "k8s.io/api/core/v1"
)

// Name is a provider name, used in the node-provider annotation.
type Name string

// Verb is what a provider does to one piece of config.
type Verb string

const (
	// Deny keeps the config from being added.
	Deny Verb = "deny"
	// Replace swaps in the provider's value.
	Replace Verb = "replace"
	// Add turns a feature on.
	// Add Verb = "add"
)

// Rule is a provider's change to one named piece of config. Deny needs no
// Value.
type Rule[T any] struct {
	Verb  Verb
	Value *T
}

// Provider is what one provider changes. An empty Provider changes nothing.
type Provider struct {
	// Rules keyed by config name.
	envVars      map[string]Rule[corev1.EnvVar]
	volumes      map[string]Rule[corev1.Volume]
	volumeMounts map[string]Rule[corev1.VolumeMount]
	// Use string instead of feature.IDType to prevent circular dependency
	features map[string]Verb
}

// registry holds every provider, filled in by each provider file's init.
var registry = map[Name]Provider{}

// register adds a provider under name.
func register(name Name, p Provider) error {
	if _, found := registry[name]; found {
		return fmt.Errorf("the provider %s is registered already", name)
	}
	registry[name] = p
	return nil
}

func Default() Provider {
	return Provider{}
}

func Get(name Name) Provider {
	return registry[name]
}

// All returns every provider, keyed by name.
func All() map[Name]Provider {
	return maps.Clone(registry)
}

// ResolveEnvVar applies the provider's rule for the env var the caller is adding.
func (p Provider) ResolveEnvVar(current *corev1.EnvVar) (resolved *corev1.EnvVar, allow bool) {
	return resolve(p.envVars, current.Name, current)
}

// ResolveVolume applies the provider's rule for the volume the caller is adding.
func (p Provider) ResolveVolume(current *corev1.Volume) (resolved *corev1.Volume, allow bool) {
	return resolve(p.volumes, current.Name, current)
}

// ResolveVolumeMount applies the provider's rule for the mount the caller is adding.
func (p Provider) ResolveVolumeMount(current *corev1.VolumeMount) (resolved *corev1.VolumeMount, allow bool) {
	return resolve(p.volumeMounts, current.Name, current)
}

// resolve applies the rule for name
func resolve[T any](rules map[string]Rule[T], name string, current *T) (resolved *T, allow bool) {
	rule, found := rules[name]
	// allow
	if !found {
		return current, true
	}
	// deny
	if rule.Verb == Deny {
		return nil, false
	}
	// no replace value provided, keep current value
	if rule.Value == nil {
		return current, true
	}
	// replace
	return rule.Value, true
}

// ApplyEnvVars applies the provider's env var rules to baseline.
func (p Provider) ApplyEnvVars(baseline []corev1.EnvVar) []corev1.EnvVar {
	return apply(p.envVars, baseline, func(e corev1.EnvVar) string { return e.Name })
}

// ApplyVolumes applies the provider's volume rules to baseline.
func (p Provider) ApplyVolumes(baseline []corev1.Volume) []corev1.Volume {
	return apply(p.volumes, baseline, func(v corev1.Volume) string { return v.Name })
}

// ApplyVolumeMounts applies the provider's mount rules to baseline.
func (p Provider) ApplyVolumeMounts(baseline []corev1.VolumeMount) []corev1.VolumeMount {
	return apply(p.volumeMounts, baseline, func(m corev1.VolumeMount) string { return m.Name })
}

// apply runs every entry in baseline through resolve, dropping denied entries.
func apply[T any](rules map[string]Rule[T], baseline []T, name func(T) string) []T {
	if len(rules) == 0 {
		return baseline
	}
	resolved := make([]T, 0, len(baseline))
	for _, entry := range baseline {
		if value, allow := resolve(rules, name(entry), &entry); allow {
			resolved = append(resolved, *value)
		}
	}
	return resolved
}

// Features returns the provider's feature rules, keyed by feature ID.
func (p Provider) Features() map[string]Verb {
	return p.features
}
