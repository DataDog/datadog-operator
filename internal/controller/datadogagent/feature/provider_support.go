// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// This file holds the centralized provider feature-support matrix: for each
// provider, how the operator should respond to each feature.
//
// The level encodes the operator's RESPONSE, not merely a support fact, because
// the response differs by provider. GKE Autopilot is a whole-cluster provider, so
// enabling an unsupported feature is an unambiguous misconfiguration that Helm
// hard-fails — the operator rejects it (Rejected) or, for best-effort features
// Helm only warns about, surfaces a warning (Degraded). Windows shares one spec
// across node types (via profiles), so an unsupported feature must be dropped for
// the Windows subset while still running on Linux (Excluded).

package feature

import (
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// SupportLevel is the operator's response to a feature on a given provider.
type SupportLevel int

const (
	// Supported runs the feature normally. It is the zero value, so any provider
	// or feature absent from the matrix is Supported by default.
	Supported SupportLevel = iota
	// Degraded runs the feature but surfaces a warning: the provider does not
	// fully support it (mirrors a Helm warn-only notice).
	Degraded
	// Rejected blocks reconciliation: the feature cannot run on the provider
	// (mirrors a Helm hard-fail).
	Rejected
	// Excluded builds the agent WITHOUT the feature (its ManageNodeAgent hook is
	// skipped) while reconciliation continues. Used by providers that run a reduced
	// feature set on a subset of nodes, e.g. Windows (a shared spec still runs the
	// full set on Linux).
	Excluded
)

// providerSupportPolicy is one provider's row in the matrix: a default level for
// unlisted features plus per-feature overrides.
type providerSupportPolicy struct {
	defaultLevel SupportLevel
	features     map[IDType]SupportLevel
}

// providerSupport is the centralized feature-support matrix, keyed by provider.
// Providers absent here impose no restrictions (everything Supported).
//
// GKE Autopilot: most features work (default Supported); only the WorkloadAllowlist
// hard-incompatible ones are Rejected, and the best-effort/unsupported-but-rendering
// ones are Degraded. This mirrors charts/datadog/templates/NOTES.txt.
//
// System-probe features (NPM, USM, TCP queue length, OOM kill, ...) are supported on
// GKE Autopilot >= v1.32.1-gke.1729000 (WorkloadAllowlists) and are intentionally left
// Supported here; the operator does not gate on GKE version.
var providerSupport = map[string]providerSupportPolicy{
	kubernetes.GKEAutopilotProvider: {
		defaultLevel: Supported,
		features: map[IDType]SupportLevel{
			// Helm hard-fails these on Autopilot.
			CWSIDType:                 Rejected, // securityAgent.runtime
			CSPMIDType:                Rejected, // securityAgent.compliance
			HostProfilerIDType:        Rejected, // hostProfiler
			PrivateActionRunnerIDType: Rejected, // privateActionRunner
			// Helm only warns (feature renders but is unsupported on Autopilot).
			SBOMIDType: Degraded, // sbom.containerImage / sbom.host
			GPUIDType:  Degraded, // gpuMonitoring
		},
	},

	// Windows: default Excluded (allowlist semantics — new Linux-only features do not
	// silently leak onto Windows); only the listed features run their ManageNodeAgent
	// hook against a Windows DaemonSet.
	//
	// NOTE when marking a Windows feature Supported: the Windows strip removes env vars
	// by NAME (Unix-socket names, known Linux-only globals — see
	// stripWindowsIncompatibleEnvVars) and mounts by Linux path prefix, but does NOT
	// inspect env-var VALUES. Verify the feature does not hand the agent a Linux
	// path/transport VALUE under an innocuous (non-SOCKET) env-var name; if it does,
	// extend the strip accordingly before marking it Supported.
	kubernetes.WindowsProvider: {
		defaultLevel: Excluded,
		features: map[IDType]SupportLevel{
			DefaultIDType:              Supported, // base agent config (required)
			APMIDType:                  Supported, // trace-agent runs on Windows (TCP; see non-local-traffic)
			LogCollectionIDType:        Supported, // log collection — Windows host-log mounts added post-strip
			LiveContainerIDType:        Supported, // container collection
			LiveProcessIDType:          Supported, // process collection (no eBPF on Windows)
			ProcessDiscoveryIDType:     Supported,
			DogstatsdIDType:            Supported, // dogstatsd (UDP non-local; named pipe future)
			OrchestratorExplorerIDType: Supported, // node-side config is env-only
			RemoteConfigurationIDType:  Supported,
			PrometheusScrapeIDType:     Supported,
			EventCollectionIDType:      Supported,
		},
	},
}

// FeatureSupportLevel returns the operator's response to feature id on the given
// provider: a per-feature override if present, otherwise the provider's default,
// otherwise Supported for providers with no policy.
func FeatureSupportLevel(provider string, id IDType) SupportLevel {
	policy, ok := providerSupport[provider]
	if !ok {
		return Supported
	}
	if level, ok := policy.features[id]; ok {
		return level
	}
	return policy.defaultLevel
}

// ProviderSupportResult is one feature's non-default support level on the active
// provider, carrying the feature ID for diagnostics.
type ProviderSupportResult struct {
	ID    IDType
	Level SupportLevel
}

// EvaluateProviderSupport returns one result per feature whose support level on
// the given provider is not Supported. Features that are Supported (or a nil/empty
// provider) yield no results.
func EvaluateProviderSupport(features []Feature, provider string) []ProviderSupportResult {
	if provider == "" {
		return nil
	}
	var results []ProviderSupportResult
	for _, feat := range features {
		if level := FeatureSupportLevel(provider, feat.ID()); level != Supported {
			results = append(results, ProviderSupportResult{ID: feat.ID(), Level: level})
		}
	}
	return results
}
