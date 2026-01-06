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
		mapConditionalServiceAccountName,
		mapHealthPortWithProbes,
		mapTraceAgentLivenessProbe,
	} {
		registry[p.name] = p.runFunc
	}
	return registry
}

// mapSecretKeyName adds the secret `keyName` field for mapping k8s secrets.
// args:
//   - keyName: the key name in the secret (e.g., "api-key", "token")
//   - keyNamePath: the DDA path for the key name
//   - skipEmpty: (optional) if true, skip mapping when pathVal is an empty string (for optional secrets)
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

		// Check for optional skipEmpty flag
		skipEmpty, _ := utils.GetPathBool(args[0], "skipEmpty")

		// If skipEmpty is true, validate that pathVal is a non-empty string
		if skipEmpty {
			secretName, isString := pathVal.(string)
			if !isString {
				log.Printf("Warning: mapSecretKeyName with skipEmpty expected string value, got %T", pathVal)
				return
			}
			if secretName == "" {
				// Skip mapping - let operator handle default behavior
				return
			}
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

// mapConditionalServiceAccountName maps serviceAccountName when rbac.create is false
//
// args:
//   - rbacCreatePath: path to the rbac.create value (e.g., "spec.override.clusterAgent.createRbac")
var mapConditionalServiceAccountName = MappingProcessor{
	name: "mapConditionalServiceAccountName",
	runFunc: func(interim map[string]interface{}, newPath string, pathVal interface{}, args []interface{}, _ chartutil.Values) {
		if len(args) != 1 {
			log.Printf("Warning: mapConditionalServiceAccountName requires exactly 1 argument (rbacCreatePath)")
			return
		}

		rbacCreatePath, ok := utils.GetPathString(args[0], "rbacCreatePath")
		if !ok || rbacCreatePath == "" {
			log.Printf("Warning: mapConditionalServiceAccountName missing 'rbacCreatePath' argument")
			return
		}

		// Only map serviceAccountName if rbac.create is explicitly false.
		// If not set or true, the operator creates its own ServiceAccount.
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

// mapHealthPortWithProbes maps healthPort and probe ports if probe httpGet.port is explicitly
// defined in the Helm source values. This matches Helm chart behavior where probes use healthPort.
//
// args:
//   - sourcePrefix: the Helm source path prefix (e.g., "clusterAgent" or "agents.containers.agent")
//   - containerPath: the DDA container path for setting probe ports (e.g., "spec.override.clusterAgent.containers.cluster-agent")
var mapHealthPortWithProbes = MappingProcessor{
	name: "mapHealthPortWithProbes",
	runFunc: func(interim map[string]interface{}, newPath string, pathVal interface{}, args []interface{}, sourceValues chartutil.Values) {
		if len(args) != 2 {
			log.Printf("Warning: mapHealthPortWithProbes requires exactly 2 arguments (sourcePrefix and containerPath)")
			return
		}

		sourcePrefix, ok := utils.GetPathString(args[0], "sourcePrefix")
		if !ok || sourcePrefix == "" {
			log.Printf("Warning: mapHealthPortWithProbes missing 'sourcePrefix' argument")
			return
		}

		containerPath, ok := utils.GetPathString(args[1], "containerPath")
		if !ok || containerPath == "" {
			log.Printf("Warning: mapHealthPortWithProbes missing 'containerPath' argument")
			return
		}

		// Get the port value as int
		portValue, ok := normalizeToInt(pathVal)
		if !ok {
			log.Printf("Warning: mapHealthPortWithProbes expected numeric value, got %T", pathVal)
			return
		}

		// Check which probe ports are defined in the Helm source values and match healthPort
		probeTypes := []string{"livenessProbe", "readinessProbe", "startupProbe"}
		var definedProbes []string

		for _, probeType := range probeTypes {
			helmPath := sourcePrefix + "." + probeType + ".httpGet.port"
			if val, _ := sourceValues.PathValue(helmPath); val != nil {
				// Normalize the probe port value to int for comparison
				probePortValue, ok := normalizeToInt(val)
				if !ok {
					continue // Skip if not a numeric type
				}
				// Only include if probe port matches healthPort
				if probePortValue == portValue {
					definedProbes = append(definedProbes, probeType)
				} else {
					// Log warning for mismatched probe port (similar to Helm chart's NOTES.txt error)
					log.Printf("Warning: %s.httpGet.port (%d) is different from healthPort (%d). This is a misconfiguration - probe port will not be mapped.", helmPath, probePortValue, portValue)
				}
			}
		}

		// Only map if at least one probe port is explicitly defined in Helm values
		if len(definedProbes) == 0 {
			return
		}

		// Set healthPort
		utils.MergeOrSet(interim, newPath, portValue)

		// Set probe ports for each defined probe
		for _, probeType := range definedProbes {
			ddaPath := containerPath + "." + probeType + ".httpGet.port"
			utils.MergeOrSet(interim, ddaPath, portValue)
		}
	},
}

// mapTraceAgentLivenessProbe maps the trace agent liveness probe.
// In the Helm chart, the trace agent uses a TCP probe with datadog.apm.port when APM is enabled.
// The probe is only set when datadog.apm.portEnabled or datadog.apm.socketEnabled is true.
// If the user has set a custom probe type (httpGet, tcpSocket, exec), the probe is mapped as-is.
//
// args:
//   - apmPortPath: path to the apm port value in source values (e.g., "datadog.apm.port")
var mapTraceAgentLivenessProbe = MappingProcessor{
	name: "mapTraceAgentLivenessProbe",
	runFunc: func(interim map[string]interface{}, newPath string, pathVal interface{}, args []interface{}, sourceValues chartutil.Values) {
		if len(args) != 1 {
			log.Printf("Warning: mapTraceAgentLivenessProbe requires exactly 1 argument (apmPortPath)")
			return
		}

		apmPortPath, ok := utils.GetPathString(args[0], "apmPortPath")
		if !ok || apmPortPath == "" {
			log.Printf("Warning: mapTraceAgentLivenessProbe missing 'apmPortPath' argument")
			return
		}

		// Check if APM is enabled (portEnabled or socketEnabled)
		portEnabled, _ := sourceValues.PathValue("datadog.apm.portEnabled")
		socketEnabled, _ := sourceValues.PathValue("datadog.apm.socketEnabled")

		apmEnabled := false
		if pe, peOk := portEnabled.(bool); peOk && pe {
			apmEnabled = true
		}
		if se, seOk := socketEnabled.(bool); seOk && se {
			apmEnabled = true
		}

		// APM is not enabled, skip mapping
		if !apmEnabled {
			return
		}

		// Check if custom probe type (httpGet, tcpSocket, exec)
		// pathVal can be either map[string]interface{} or chartutil.Values
		var probeSettings map[string]interface{}
		switch v := pathVal.(type) {
		case map[string]interface{}:
			probeSettings = v
		case chartutil.Values:
			probeSettings = map[string]interface{}(v)
		default:
			return
		}

		// If explicitly set httpGet, tcpSocket, or exec, map probe as-is
		if _, hasHttpGet := probeSettings["httpGet"]; hasHttpGet {
			utils.MergeOrSet(interim, newPath, pathVal)
			return
		}
		if _, hasTcpSocket := probeSettings["tcpSocket"]; hasTcpSocket {
			utils.MergeOrSet(interim, newPath, pathVal)
			return
		}
		if _, hasExec := probeSettings["exec"]; hasExec {
			utils.MergeOrSet(interim, newPath, pathVal)
			return
		}

		// Get APM port from source values
		apmPort, _ := sourceValues.PathValue(apmPortPath)
		if apmPort == nil {
			return
		}

		// Normalize port value to int
		portValue, ok := normalizeToInt(apmPort)
		if !ok {
			log.Printf("Warning: mapTraceAgentLivenessProbe expected numeric apm port, got %T", apmPort)
			return
		}

		// Merge the probe settings with tcpSocket.port
		// First, map any existing probe settings (initialDelaySeconds, periodSeconds, etc.)
		for key, val := range probeSettings {
			utils.MergeOrSet(interim, newPath+"."+key, val)
		}
		// Then set the tcpSocket.port
		utils.MergeOrSet(interim, newPath+".tcpSocket.port", portValue)
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

