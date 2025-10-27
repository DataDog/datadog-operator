// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package utils

import (
	"log"

	"helm.sh/helm/v3/pkg/chartutil"
)

// DepAction Action to perform on deprecated keys
type DepAction int

const (
	DepBoolOr  DepAction = iota // boolean OR operation
	DepBoolNeg                  // boolean ! operation
)

// depRuleRegistry is a mapping of standard keys and their deprecation rules.
func depRuleRegistry() map[string]DepRule {
	r := map[string]DepRule{}
	for _, rule := range []DepRule{
		apmPortEnabledDepRule,
		apmSocketEnabledDepRule,
		disableDefaultOSReleasePathsDepRule,
		remoteConfigEnabledDepRule,
		useHostPIDDepRule,
		securityAgentHostBenchmarksEnabledDepRule,
		networkPolicyCreateDepRule,
		clusterAgentPDBCreateDepRule,
		clusterChecksRunnerPDBCreateDepRule,
	} {
		r[rule.Standard] = rule
	}
	return r
}

// DepRule describes how to map deprecated keys into its standard key.
type DepRule struct {
	Deprecated []string
	Action     DepAction
	Standard   string
}

var apmPortEnabledDepRule = DepRule{
	Deprecated: []string{"datadog.apm.enabled"},
	Action:     DepBoolOr,
	Standard:   "datadog.apm.portEnabled",
}

var apmSocketEnabledDepRule = DepRule{
	Deprecated: []string{"datadog.apm.useSocketVolume"},
	Action:     DepBoolOr,
	Standard:   "datadog.apm.socketEnabled",
}

var disableDefaultOSReleasePathsDepRule = DepRule{
	Deprecated: []string{"datadog.systemProbe.enableDefaultOsReleasePaths"},
	Action:     DepBoolNeg,
	Standard:   "datadog.disableDefaultOsReleasePaths",
}

var remoteConfigEnabledDepRule = DepRule{
	Deprecated: []string{"datadog.remoteConfiguration.enabled"},
	Action:     DepBoolOr,
	Standard:   "remoteConfiguration.enabled",
}

var useHostPIDDepRule = DepRule{
	Deprecated: []string{"datadog.dogstatsd.useHostPID"},
	Action:     DepBoolOr,
	Standard:   "datadog.useHostPID",
}

var securityAgentHostBenchmarksEnabledDepRule = DepRule{
	Deprecated: []string{"datadog.securityAgent.compliance.xccdf"},
	Action:     DepBoolOr,
	Standard:   "datadog.securityAgent.compliance.host_benchmarks.enabled",
}

var networkPolicyCreateDepRule = DepRule{
	Deprecated: []string{
		"agents.networkPolicy.create",
		"clusterAgent.networkPolicy.create",
		"clusterChecksRunner.networkPolicy.create",
	},
	Action:   DepBoolOr,
	Standard: "datadog.networkPolicy.create",
}

var clusterAgentPDBCreateDepRule = DepRule{
	Deprecated: []string{"clusterAgent.createPodDisruptionBudget"},
	Action:     DepBoolOr,
	Standard:   "clusterAgent.pdb.create"}

var clusterChecksRunnerPDBCreateDepRule = DepRule{
	Deprecated: []string{"clusterChecksRunner.createPodDisruptionBudget"},
	Action:     DepBoolOr,
	Standard:   "clusterChecksRunner.pdb.create",
}

// ApplyDeprecationRules maps “standard” key values by looking at their
// deprecated aliases according to depRules. It writes the effective
// value to sourceValues under the standard key.
func ApplyDeprecationRules(sourceValues chartutil.Values) chartutil.Values {
	// chartutil.Values is a map[string]interface{}
	root := map[string]interface{}(sourceValues)
	depRules := depRuleRegistry()

	for stdKey, depRule := range depRules {
		candidates := depRule.Deprecated
		// If the standard key is present in the source values, add it to the candidates
		if stdVal, err := sourceValues.PathValue(stdKey); stdVal != nil && err == nil {
			candidates = append(candidates, stdKey)
		}

		if len(candidates) == 0 {
			continue // nothing to do
		}

		val := false
		seen := false
		for _, c := range candidates {
			cVal, err := sourceValues.PathValue(c)
			if err != nil {
				continue
			}

			switch depRule.Action {
			case DepBoolOr:
				val = val || cVal.(bool)

			case DepBoolNeg:
				stdVal, err := sourceValues.PathValue(stdKey)
				if err != nil {
					val = !cVal.(bool)
				} else {
					val = stdVal.(bool)
				}
			default:
				continue
			}

			if c != stdKey {
				removeAtPath(root, c)
				log.Printf("Mapped deprecated helm key '%v' to '%v'", c, stdKey)
			}
			seen = true
		}
		if seen {
			root = InsertAtPath(stdKey, val, root)
		}
	}
	return root
}
