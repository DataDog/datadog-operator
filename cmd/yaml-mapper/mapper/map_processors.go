// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package mapper

import (
	"log"
	"reflect"
	"strconv"
	"strings"

	"helm.sh/helm/v3/pkg/chartutil"
	"sigs.k8s.io/yaml"

	"github.com/DataDog/datadog-operator/cmd/yaml-mapper/utils"
)

// MappingRunFunc is the function signature for mapping processors
// - interim: the DDA spec being built
// - newPath: the DDA target path
// - pathVal: the value from Helm at the current source path
// - args: custom arguments from the mapping config
// - sourceValues: the original Helm values
type MappingRunFunc = func(interim map[string]interface{}, newPath string, pathVal interface{}, args []interface{}, sourceValues chartutil.Values)
type MappingProcessor struct {
	name    string
	runFunc MappingRunFunc
}

// mapFuncRegistry builds an indexable map object of the mapping functions.
func mapFuncRegistry() map[string]MappingRunFunc {
	registry := map[string]MappingRunFunc{}
	for _, p := range []MappingProcessor{
		mapSecretKeyName,
		mapSeccompProfile,
		mapSystemProbeAppArmor,
		mapLocalServiceName,
		mapAppendEnvVar,
		mapMergeEnvs,
		mapOverrideType,
		mapServiceAccountName,
		mapHealthPortWithProbes,
		mapTraceAgentLivenessProbe,
		mapApmPortToContainerPort,
	} {
		registry[p.name] = p.runFunc
	}
	return registry
}

// mapSecretKeyName adds the secret `keyName` field for mapping k8s secrets.
// Skips mapping when pathVal is an empty string (let operator handle defaults).
// args:
//   - keyName: the key name in the secret (e.g., "api-key", "token")
//   - keyNamePath: the DDA path for the key name
var mapSecretKeyName = MappingProcessor{
	name: "mapSecretKeyName",
	runFunc: func(interim map[string]interface{}, newPath string, pathVal interface{}, args []interface{}, _ chartutil.Values) {
		if len(args) != 1 {
			log.Printf("Warning: mapSecretKeyName requires exactly 1 argument")
			return
		}

		keyName, ok := utils.GetPathString(args[0], "keyName")
		if !ok {
			log.Printf("Warning: mapSecretKeyName missing 'keyName' argument")
			return
		}
		keyNamePath, ok := utils.GetPathString(args[0], "keyNamePath")
		if !ok {
			log.Printf("Warning: mapSecretKeyName missing 'keyNamePath' argument")
			return
		}

		// Skip empty secret names - let operator handle defaults
		secretName, isString := pathVal.(string)
		if !isString {
			log.Printf("Warning: mapSecretKeyName expected string value, got %T", pathVal)
			return
		}
		if secretName == "" {
			return
		}

		utils.MergeOrSet(interim, newPath, pathVal)
		utils.MergeOrSet(interim, keyNamePath, keyName)
	},
}

// mapSeccompProfile Maps the seccompProfile
var mapSeccompProfile = MappingProcessor{
	name: "mapSeccompProfile",
	runFunc: func(interim map[string]interface{}, newPath string, pathVal interface{}, args []interface{}, _ chartutil.Values) {
		seccompValue, ok := pathVal.(string)
		if !ok {
			return
		}

		switch {
		case strings.HasPrefix(seccompValue, "localhost/"):
			profileName := strings.TrimPrefix(seccompValue, "localhost/")
			utils.MergeOrSet(interim, newPath+".type", "Localhost")
			utils.MergeOrSet(interim, newPath+".localhostProfile", profileName)
		case seccompValue == "runtime/default":
			utils.MergeOrSet(interim, newPath+".type", "RuntimeDefault")
		case seccompValue == "unconfined":
			utils.MergeOrSet(interim, newPath+".type", "Unconfined")
		}
	},
}

// mapSystemProbeAppArmor Maps the systemProbe appArmor profile name
var mapSystemProbeAppArmor = MappingProcessor{
	name: "mapSystemProbeAppArmor",
	runFunc: func(interim map[string]interface{}, newPath string, pathVal interface{}, args []interface{}, _ chartutil.Values) {
		appArmorValue, ok := pathVal.(string)
		if !ok || appArmorValue == "" {
			// must be set to non-empty string
			return
		}

		systemProbeFeatures := []string{
			"spec.features.cws.enabled",            // datadog.securityAgent.runtime.enabled
			"spec.features.npm.enabled",            // datadog.networkMonitoring.enabled
			"spec.features.tcpQueueLength.enabled", // datadog.systemProbe.enableTCPQueueLength
			"spec.features.oomKill.enabled",        // datadog.systemProbe.enableOOMKill
			"spec.features.usm.enabled",            // datadog.serviceMonitoring.enabled
		}

		hasSystemProbeFeature := false
		for _, feature := range systemProbeFeatures {
			if val, enabled := utils.GetPathBool(interim, feature); enabled && val {
				hasSystemProbeFeature = true
				break
			}
		}
		// Check if GPU is enabled
		if !hasSystemProbeFeature {
			gpuEnabled, gpuExists := utils.GetPathBool(interim, "spec.features.gpu.enabled")
			gpuPrivileged, privExists := utils.GetPathBool(interim, "spec.features.gpu.privilegedMode")
			if gpuExists && privExists && gpuEnabled && gpuPrivileged {
				hasSystemProbeFeature = true
			}
		}

		if hasSystemProbeFeature {
			// must be set to non-empty string
			utils.MergeOrSet(interim, newPath, appArmorValue)
		}
	},
}

// mapLocalServiceName maps the localService name
var mapLocalServiceName = MappingProcessor{
	name: "mapLocalServiceName",
	runFunc: func(interim map[string]interface{}, newPath string, pathVal interface{}, args []interface{}, _ chartutil.Values) {
		nameOverride, ok := pathVal.(string)
		if !ok || nameOverride == "" {
			return
		}
		utils.MergeOrSet(interim, newPath, nameOverride)
	},
}

// mapAppendEnvVar appends a new environment variable to the specified key path.
// The new environment variable name must be specified using `args`. For example:
// args:
//   - name: DD_ENV_VAR
var mapAppendEnvVar = MappingProcessor{
	name: "mapAppendEnvVar",
	runFunc: func(interim map[string]interface{}, newPath string, pathVal interface{}, args []interface{}, _ chartutil.Values) {
		if len(args) != 1 {
			return
		}

		envMap, ok := utils.GetPathMap(args[0])
		if !ok {
			log.Printf("expected map[string]interface{} for env var map definition, got %T", args[0])
			return
		}

		// Build new env var
		newEnvName, ok := utils.GetPathString(envMap, "name")
		if !ok || newEnvName == "" {
			log.Printf("expected 'name' in env var map, got: %v", envMap)
			return
		}
		// Base env var
		newEnvVar := map[string]interface{}{
			"name":  newEnvName,
			"value": pathVal,
		}

		// Handle valueFrom
		// a) valFrom is a Map
		if valFrom, vOk := utils.GetPathVal(pathVal, "valueFrom"); vOk && valFrom != nil {
			newEnvVar = map[string]interface{}{"name": newEnvName, "valueFrom": valFrom}
		} else {
			// b) valFrom is YAML string
			if s, isStr := pathVal.(string); isStr && strings.Contains(s, "valueFrom") {
				var data map[string]interface{}
				if err := yaml.Unmarshal([]byte(s), &data); err == nil {
					if v, vStrOk := data["valueFrom"]; vStrOk {
						newEnvVar = map[string]interface{}{"name": newEnvName, "valueFrom": v}
					}
				}
			}
		}

		// Create the interim[newPath] if it doesn't exist yet
		if _, exists := interim[newPath]; !exists {
			utils.MergeOrSet(interim, newPath, []interface{}{newEnvVar})
			return
		}

		existingEnvs, ok := interim[newPath].([]interface{})
		if !ok {
			log.Printf("Error: expected []interface{} at path %s, got %T", newPath, interim[newPath])
			return
		}

		if !hasDuplicateEnv(existingEnvs, newEnvName) {
			utils.MergeOrSet(interim, newPath, append(existingEnvs, newEnvVar))
		}
	},
}

// mapMergeEnvs merges lists of environment variables at the specified key path.
// It takes a slice of environment variable maps and merges them with any existing
// environment variables at the target path.
var mapMergeEnvs = MappingProcessor{
	name: "mapMergeEnvs",
	runFunc: func(interim map[string]interface{}, newPath string, pathVal interface{}, args []interface{}, _ chartutil.Values) {
		newEnvs, ok := pathVal.([]interface{})
		if !ok {
			log.Printf("Warning: expected []interface{} for pathVal, got %T", pathVal)
			return
		}

		// Initialize mergedEnvs with existing environments or an empty slice
		var mergedEnvs []interface{}
		if existingEnvs, exists := interim[newPath]; exists {
			if existingEnvsSlice, ok := existingEnvs.([]interface{}); ok {
				mergedEnvs = make([]interface{}, 0, len(existingEnvsSlice))
				mergedEnvs = append(mergedEnvs, existingEnvsSlice...)
			}
		}

		// Add new envs that don't already exist
		for _, newEnv := range newEnvs {
			newEnvMap, ok := utils.GetPathMap(newEnv)
			if !ok {
				log.Printf("Warning: expected map[string]interface{} in newEnvs, got %T", newEnv)
				continue
			}

			newName, ok := utils.GetPathString(newEnvMap, "name")
			if !ok || newName == "" {
				log.Printf("Warning: missing or invalid 'name' field in environment variable: %v", newEnvMap)
				continue
			}

			if !hasDuplicateEnv(mergedEnvs, newName) {
				mergedEnvs = append(mergedEnvs, newEnv)
			} else {
				// Override existing env with new env
				for i, existingEnv := range mergedEnvs {
					if existingMap, ok := utils.GetPathMap(existingEnv); ok {
						if existingName, ok := utils.GetPathString(existingMap, "name"); ok && existingName == newName {
							mergedEnvs[i] = newEnv
						}
					}
				}
			}
		}

		if len(mergedEnvs) > 0 {
			utils.MergeOrSet(interim, newPath, mergedEnvs)
		}
	},
}

// mapOverrideType overrides the source value type to a new type specified in `args`:
// args:
//   - newType: string
//
// Supports mapping slice -> string and string -> int.
var mapOverrideType = MappingProcessor{
	name: "mapOverrideType",
	runFunc: func(interim map[string]interface{}, newPath string, pathVal interface{}, args []interface{}, _ chartutil.Values) {
		if len(args) != 1 {
			return
		}
		newType, ok := utils.GetPathString(args[0], "newType")
		if !ok {
			return
		}

		// if values type is different from new type, convert it
		pathValType := reflect.TypeOf(pathVal).Kind().String()
		if pathValType == newType {
			return
		}

		switch {
		case newType == "string" && pathValType == "slice":
			newPathVal, err := yaml.Marshal(pathVal)
			if err != nil {
				log.Println(err)
				return
			}
			utils.MergeOrSet(interim, newPath, string(newPathVal))

		case newType == "int" && pathValType == "string":
			convertedInt, err := strconv.Atoi(pathVal.(string))
			if err != nil {
				log.Println(err)
				return
			}
			utils.MergeOrSet(interim, newPath, convertedInt)
		}
	},
}

// mapServiceAccountName maps serviceAccountName when rbac.create is false
//
// args:
//   - rbacCreatePath: path to the rbac.create value (e.g., "spec.override.clusterAgent.createRbac")
var mapServiceAccountName = MappingProcessor{
	name: "mapServiceAccountName",
	runFunc: func(interim map[string]interface{}, newPath string, pathVal interface{}, args []interface{}, _ chartutil.Values) {
		if len(args) != 1 {
			log.Printf("Warning: mapServiceAccountName requires exactly 1 argument (rbacCreatePath)")
			return
		}

		rbacCreatePath, ok := utils.GetPathString(args[0], "rbacCreatePath")
		if !ok || rbacCreatePath == "" {
			log.Printf("Warning: mapServiceAccountName missing 'rbacCreatePath' argument")
			return
		}

		// Only map serviceAccountName if rbac.create is explicitly false
		rbacCreate, exists := utils.GetPathBool(interim, rbacCreatePath)
		if !exists || rbacCreate {
			return
		}

		saName, ok := pathVal.(string)
		if ok && saName != "" {
			utils.MergeOrSet(interim, newPath, saName)
		}
	},
}

// mapHealthPortWithProbes maps healthPort and validates that probe httpGet.ports match.
// In the Helm chart, users MUST set livenessProbe, startupProbe, and readinessProbe ports
// if they are setting healthPort, and all ports must match. Otherwise, don't map anything (let operator handle defaults).
//
// args:
//   - sourcePrefix: the Helm values prefix for checking probe ports (e.g., "agents.containers.agent", "clusterAgent")
//   - containerPath: the DDA container path for setting probe ports (e.g., "spec.override.clusterAgent.containers.cluster-agent")
var mapHealthPortWithProbes = MappingProcessor{
	name: "mapHealthPortWithProbes",
	runFunc: func(interim map[string]interface{}, newPath string, pathVal interface{}, args []interface{}, sourceValues chartutil.Values) {
		if len(args) != 1 {
			log.Printf("Warning: mapHealthPortWithProbes requires exactly 1 argument")
			return
		}

		sourcePrefix, ok := utils.GetPathString(args[0], "sourcePrefix")
		if !ok || sourcePrefix == "" {
			log.Printf("Warning: mapHealthPortWithProbes missing 'sourcePrefix' argument")
			return
		}

		containerPath, ok := utils.GetPathString(args[0], "containerPath")
		if !ok || containerPath == "" {
			log.Printf("Warning: mapHealthPortWithProbes missing 'containerPath' argument")
			return
		}

		// Get the healthPort value as int
		healthPort, ok := normalizeToInt(pathVal)
		if !ok {
			log.Printf("Warning: mapHealthPortWithProbes expected numeric value, got %T", pathVal)
			return
		}

		// Check if ALL probe ports are explicitly set in sourceValues and match healthPort
		probeTypes := []string{"livenessProbe", "readinessProbe", "startupProbe"}
		allProbesMatch := true
		probesCount := 0

		for _, probeType := range probeTypes {
			helmPath := sourcePrefix + "." + probeType + ".httpGet.port"
			probePortVal, err := sourceValues.PathValue(helmPath)
			if err != nil || probePortVal == nil {
				// Probe port not set by user - can't validate, don't map
				allProbesMatch = false
				break
			}

			probePort, ok := normalizeToInt(probePortVal)
			if !ok {
				log.Printf("Warning: mapHealthPortWithProbes probe port at %s is not numeric: %T", helmPath, probePortVal)
				allProbesMatch = false
				break
			}

			if probePort != healthPort {
				log.Printf("Warning: mapHealthPortWithProbes probe port mismatch at %s: expected %d, got %d", helmPath, healthPort, probePort)
				allProbesMatch = false
				break
			}
			probesCount++
		}

		// Only map if all probes are set and match
		if !allProbesMatch || probesCount < 3 {
			return
		}

		// Set healthPort
		utils.MergeOrSet(interim, newPath, healthPort)

		// Set all probe ports (they've been validated to match)
		for _, probeType := range probeTypes {
			ddaPath := containerPath + "." + probeType + ".httpGet.port"
			utils.MergeOrSet(interim, ddaPath, healthPort)
		}
	},
}

// mapTraceAgentLivenessProbe maps the trace agent liveness probe with tcpSocket.port.
// Deduces port from datadog.apm.port or default 8126 if not set.
// Validates that explicit tcpSocket.port matches apm.port or default; skips mapping on mismatch.
// args:
//   - newPath: the DDA path for the trace agent liveness probe
var mapTraceAgentLivenessProbe = MappingProcessor{
	name: "mapTraceAgentLivenessProbe",
	runFunc: func(interim map[string]interface{}, newPath string, pathVal interface{}, args []interface{}, sourceValues chartutil.Values) {
		if pathVal == nil {
			return
		}

		var probeSettings map[string]interface{}
		switch v := pathVal.(type) {
		case map[string]interface{}:
			probeSettings = v
		case chartutil.Values:
			probeSettings = map[string]interface{}(v)
		default:
			return
		}

		const defaultPort = 8126

		// Get user's explicit apm.port if set
		userApmPort, hasUserApmPort := utils.GetPathInt(sourceValues, "datadog", "apm", "port")

		// Check if tcpSocket.port is set
		tcpSocketPort, hasTcpSocketPort := utils.GetPathInt(probeSettings, "tcpSocket", "port")

		if !hasTcpSocketPort {
			// tcpSocket.port not set - deduce from apm.port or use default
			port := defaultPort
			if hasUserApmPort {
				port = userApmPort
			}
			// Map all existing probe settings plus tcpSocket.port
			for key, val := range probeSettings {
				utils.MergeOrSet(interim, newPath+"."+key, val)
			}
			utils.MergeOrSet(interim, newPath+".tcpSocket.port", port)
			return
		}

		// tcpSocket.port is set - validate it matches apm.port or default
		if tcpSocketPort == defaultPort || (hasUserApmPort && tcpSocketPort == userApmPort) {
			utils.MergeOrSet(interim, newPath, pathVal)
		} else {
			log.Printf("Warning: mapTraceAgentLivenessProbe tcpSocket.port %d doesn't match apm.port or default 8126", tcpSocketPort)
		}
	},
}

// normalizeToInt converts various numeric types to int.
// Returns (0, false) if the value is not a recognized numeric type.
func normalizeToInt(val interface{}) (int, bool) {
	switch v := val.(type) {
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}

// hasDuplicateEnv checks if a given env var name is already present in the given list of env vars.
func hasDuplicateEnv(existingEnvs []interface{}, newEnvName string) bool {
	for _, existingEnv := range existingEnvs {
		if existingMap, ok := utils.GetPathMap(existingEnv); ok {
			if existingName, ok := utils.GetPathString(existingMap, "name"); ok && existingName == newEnvName {
				return true
			}
		}
	}
	return false
}

// mapApmPortToContainerPort maps datadog.apm.port to both hostPort and containerPort for trace-agent.
// The Helm chart always sets containerPort = hostPort = datadog.apm.port, but the operator only
// syncs them when hostNetwork is enabled on the pod. This processor preserves Helm behavior.
var mapApmPortToContainerPort = MappingProcessor{
	name: "mapApmPortToContainerPort",
	runFunc: func(interim map[string]interface{}, newPath string, pathVal interface{}, args []interface{}, sourceValues chartutil.Values) {
		port, ok := normalizeToInt(pathVal)
		if !ok {
			log.Printf("Warning: mapApmPortToContainerPort expected numeric value, got %T", pathVal)
			return
		}

		// Default APM port is 8126 - only map if user has set a different value
		if port == 8126 {
			return
		}

		// Set hostPort
		utils.MergeOrSet(interim, newPath, port)

		// Set matching containerPort
		containerPortPath := "spec.override.nodeAgent.containers.trace-agent.ports"
		portEntry := []interface{}{
			map[string]interface{}{
				"name":          "traceport",
				"containerPort": port,
				"hostPort":      port,
				"protocol":      "TCP",
			},
		}
		utils.MergeOrSet(interim, containerPortPath, portEntry)
	},
}
