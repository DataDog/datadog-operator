// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"strings"

	utilvalidation "k8s.io/apimachinery/pkg/util/validation"

	v1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// managedAgentInstallationMaxAgentProfiles bounds the number of DatadogAgentProfile resources a single managed Agent installation intent can request.
const managedAgentInstallationMaxAgentProfiles = 8

// managedAgentInstallationAgentProfile is a DatadogAgentProfile requested by a managed Agent installation intent.
type managedAgentInstallationAgentProfile struct {
	Name            string                    `json:"name"`
	ProfileAffinity *v1alpha1.ProfileAffinity `json:"profileAffinity"`
	Config          json.RawMessage           `json:"config,omitempty"`
}

// decodedManagedAgentInstallationAgentProfile is the validated, decoded form used to build the DatadogAgentProfile CR.
type decodedManagedAgentInstallationAgentProfile struct {
	Name            string                     `json:"name"`
	ProfileAffinity *v1alpha1.ProfileAffinity  `json:"profileAffinity"`
	Config          *v2alpha1.DatadogAgentSpec `json:"config,omitempty"`
}

func decodeManagedAgentInstallationAgentProfiles(raw json.RawMessage) ([]decodedManagedAgentInstallationAgentProfile, error) {
	if len(raw) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil, nil
	}
	var profiles []managedAgentInstallationAgentProfile
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&profiles); err != nil {
		return nil, fmt.Errorf("decode managed Agent installation agent profiles: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("decode managed Agent installation agent profiles: trailing JSON content")
	}
	if len(profiles) == 0 {
		return nil, fmt.Errorf("managed Agent installation agent profiles must not be empty")
	}
	if len(profiles) > managedAgentInstallationMaxAgentProfiles {
		return nil, fmt.Errorf("managed Agent installation agent profiles exceed the maximum of %d", managedAgentInstallationMaxAgentProfiles)
	}

	decoded := make([]decodedManagedAgentInstallationAgentProfile, 0, len(profiles))
	seen := make(map[string]struct{}, len(profiles))
	for i, profile := range profiles {
		if errs := utilvalidation.IsDNS1123Subdomain(profile.Name); len(errs) != 0 {
			return nil, fmt.Errorf("managed Agent installation agent profile %d has an invalid name: %s", i, strings.Join(errs, "; "))
		}
		if profile.Name == managedAgentInstallationWindowsProfileName {
			return nil, fmt.Errorf("managed Agent installation agent profile name %q is reserved", managedAgentInstallationWindowsProfileName)
		}
		if _, duplicate := seen[profile.Name]; duplicate {
			return nil, fmt.Errorf("managed Agent installation agent profile name %q is duplicated", profile.Name)
		}
		seen[profile.Name] = struct{}{}

		var config *v2alpha1.DatadogAgentSpec
		if len(profile.Config) != 0 && string(bytes.TrimSpace(profile.Config)) != "null" {
			decodedConfig, err := decodeRemoteDatadogAgentConfig(profile.Config, false)
			if err != nil {
				return nil, fmt.Errorf("managed Agent installation agent profile %q has an invalid config: %w", profile.Name, err)
			}
			if decodedConfig.Spec == nil {
				return nil, fmt.Errorf("managed Agent installation agent profile %q config must contain spec", profile.Name)
			}
			config = decodedConfig.Spec
		}
		spec := v1alpha1.DatadogAgentProfileSpec{
			ProfileAffinity: profile.ProfileAffinity,
			Config:          config,
		}
		if err := v1alpha1.ValidateDatadogAgentProfileSpec(&spec); err != nil {
			return nil, fmt.Errorf("managed Agent installation agent profile %q is invalid: %w", profile.Name, err)
		}
		decoded = append(decoded, decodedManagedAgentInstallationAgentProfile{
			Name:            profile.Name,
			ProfileAffinity: profile.ProfileAffinity,
			Config:          config,
		})
	}
	return decoded, nil
}

// mergeManagedAgentInstallationAgentConfig merges the intent-provided DatadogAgentSpec fragment over the bootstrap spec, which stays authoritative for cluster name, site, and linux node selectors.
func mergeManagedAgentInstallationAgentConfig(bootstrap *v2alpha1.DatadogAgentSpec, agentConfig *datadogAgentManagedAgentInstallationConfig) (*v2alpha1.DatadogAgentSpec, error) {
	if agentConfig == nil || agentConfig.Spec == nil {
		return bootstrap, nil
	}

	merged := agentConfig.Spec.DeepCopy()
	if merged.Global == nil {
		merged.Global = &v2alpha1.GlobalConfig{}
	}
	merged.Global.ClusterName = bootstrap.Global.ClusterName
	merged.Global.Site = bootstrap.Global.Site

	if merged.Override == nil {
		merged.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{}
	}
	linuxSelector := maps.Clone(bootstrap.Override[v2alpha1.NodeAgentComponentName].NodeSelector)
	for _, component := range []v2alpha1.ComponentName{v2alpha1.NodeAgentComponentName, v2alpha1.ClusterAgentComponentName} {
		override, ok := merged.Override[component]
		if !ok || override == nil {
			override = &v2alpha1.DatadogAgentComponentOverride{}
			merged.Override[component] = override
		}
		if override.NodeSelector == nil {
			override.NodeSelector = map[string]string{}
		} else {
			// Copy so the caller's fragment map is never mutated.
			override.NodeSelector = maps.Clone(override.NodeSelector)
		}
		maps.Copy(override.NodeSelector, linuxSelector)
	}
	return merged, nil
}

func canonicalManagedAgentInstallationJSON(value any) (json.RawMessage, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode managed Agent installation config: %w", err)
	}
	return encoded, nil
}

// finalizeManagedAgentInstallationIntent decodes the agent settings, merges the agent config over the bootstrap spec, and returns the normalized config and digest.
func finalizeManagedAgentInstallationIntent(
	provider ManagedAgentInstallationProvider,
	version string,
	installationID string,
	targetID string,
	operationID string,
	acknowledgedOperationID string,
	desiredState managedAgentInstallationDesiredState,
	bootstrap managedAgentInstallationBootstrap,
	agentConfig json.RawMessage,
	agentProfiles json.RawMessage,
) (managedAgentInstallationIntent, json.RawMessage, string, error) {
	var decodedAgentConfig *datadogAgentManagedAgentInstallationConfig
	if len(agentConfig) != 0 && string(bytes.TrimSpace(agentConfig)) != "null" {
		decoded, err := decodeRemoteDatadogAgentConfig(agentConfig, false)
		if err != nil {
			return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("invalid managed Agent installation agent config: %w", err)
		}
		decodedAgentConfig = decoded
	}
	decodedProfiles, err := decodeManagedAgentInstallationAgentProfiles(agentProfiles)
	if err != nil {
		return managedAgentInstallationIntent{}, nil, "", err
	}

	var normalizedConfig json.RawMessage
	switch desiredState {
	case managedAgentInstallationDesiredStateInstalled:
		mergedSpec, mergeErr := mergeManagedAgentInstallationAgentConfig(managedAgentInstallationBootstrapSpec(bootstrap), decodedAgentConfig)
		if mergeErr != nil {
			return managedAgentInstallationIntent{}, nil, "", mergeErr
		}
		config, configErr := canonicalManagedAgentInstallationJSON(datadogAgentManagedAgentInstallationConfig{Spec: mergedSpec})
		if configErr != nil {
			return managedAgentInstallationIntent{}, nil, "", configErr
		}
		normalizedConfig = config
	case managedAgentInstallationDesiredStateAbsent:
		if decodedAgentConfig != nil || decodedProfiles != nil {
			return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("managed Agent installation uninstall must not carry agent config or agent profiles")
		}
		normalizedConfig = json.RawMessage(`{}`)
	default:
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("unsupported managed Agent installation desired state %q", desiredState)
	}

	intent := managedAgentInstallationIntent{
		Version:                 version,
		Provider:                provider,
		InstallationID:          installationID,
		TargetID:                targetID,
		OperationID:             operationID,
		DesiredState:            desiredState,
		AcknowledgedOperationID: acknowledgedOperationID,
		Bootstrap:               bootstrap,
		AgentConfig:             decodedAgentConfig,
		AgentProfiles:           decodedProfiles,
	}
	normalized := normalizedManagedAgentInstallationIntent{
		Version:        intent.Version,
		Provider:       intent.Provider,
		InstallationID: intent.InstallationID,
		TargetID:       intent.TargetID,
		OperationID:    intent.OperationID,
		DesiredState:   intent.DesiredState,
		Bootstrap:      intent.Bootstrap,
	}
	if decodedAgentConfig != nil {
		canonicalConfig, configErr := canonicalManagedAgentInstallationJSON(decodedAgentConfig)
		if configErr != nil {
			return managedAgentInstallationIntent{}, nil, "", configErr
		}
		normalized.AgentConfig = canonicalConfig
	}
	if decodedProfiles != nil {
		canonicalProfiles, profilesErr := canonicalManagedAgentInstallationJSON(decodedProfiles)
		if profilesErr != nil {
			return managedAgentInstallationIntent{}, nil, "", profilesErr
		}
		normalized.AgentProfiles = canonicalProfiles
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("encode normalized managed Agent installation intent: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return intent, normalizedConfig, hex.EncodeToString(digest[:]), nil
}
