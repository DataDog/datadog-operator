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

	"github.com/DataDog/datadog-operator/cmd/yaml-mapper/utils"
	"sigs.k8s.io/yaml"
)

type MappingRunFunc = func(values map[string]interface{}, newPath string, pathVal interface{}, args []interface{})
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
	} {
		registry[p.name] = p.runFunc
	}
	return registry
}

// mapSecretKeyName adds the secret `keyName` field for mapping k8s secrets
var mapSecretKeyName = MappingProcessor{
	name: "mapSecretKeyName",
	runFunc: func(interim map[string]interface{}, newPath string, pathVal interface{}, args []interface{}) {
		if len(args) != 1 {
			return
		}
		keyName, ok := utils.GetPathString(args[0], "keyName")
		if !ok {
			return
		}
		keyNamePath, ok := utils.GetPathString(args[0], "keyNamePath")
		if !ok {
			return
		}
		utils.MergeOrSet(interim, newPath, pathVal)
		utils.MergeOrSet(interim, keyNamePath, keyName)
	},
}

// mapSeccompProfile Maps the seccompProfile
var mapSeccompProfile = MappingProcessor{
	name: "mapSeccompProfile",
	runFunc: func(interim map[string]interface{}, newPath string, pathVal interface{}, args []interface{}) {
		seccompValue, err := pathVal.(string)
		if !err {
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
	runFunc: func(interim map[string]interface{}, newPath string, pathVal interface{}, args []interface{}) {
		appArmorValue, err := pathVal.(string)
		if !err || appArmorValue == "" {
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
	runFunc: func(interim map[string]interface{}, newPath string, pathVal interface{}, args []interface{}) {
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
	runFunc: func(interim map[string]interface{}, newPath string, pathVal interface{}, args []interface{}) {
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
	runFunc: func(interim map[string]interface{}, newPath string, pathVal interface{}, args []interface{}) {
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
	runFunc: func(interim map[string]interface{}, newPath string, pathVal interface{}, args []interface{}) {
		{
			if len(args) != 1 {
				return
			}
			newType, ok := utils.GetPathString(args[0], "newType")

			// if values type is different from new type, convert it
			pathValType := reflect.TypeOf(pathVal).Kind().String()
			var newPathVal []byte
			var err error
			if ok && pathValType != newType {
				switch {
				case newType == "string" && pathValType == "slice":
					newPathVal, err = yaml.Marshal(pathVal)
					if err != nil {
						log.Println(err)
					}
					utils.MergeOrSet(interim, newPath, string(newPathVal))

				case newType == "int":
					switch {
					case pathValType == "string":
						convertedInt, convErr := strconv.Atoi(pathVal.(string))
						if convErr != nil {
							log.Println(convErr)
						} else {
							utils.MergeOrSet(interim, newPath, convertedInt)
						}
					}
				}
			}
		}
	},
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
