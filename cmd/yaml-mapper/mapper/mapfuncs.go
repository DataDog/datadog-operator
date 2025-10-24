package mapper

import (
	"log"
	"reflect"
	"strconv"
	"strings"

	"sigs.k8s.io/yaml"
)

var mappingProcessors = []MappingProcessor{
	mapApiSecretKey,
	mapAppSecretKey,
	mapTokenSecretKey,
	mapSeccompProfile,
	mapSystemProbeAppArmor,
	mapLocalServiceName,
	mapAppendEnvVar,
	mapMergeEnvs,
	mapOverrideType,
}

func getMappingRunFunc(name string) MappingRunFunc {
	for _, processor := range mappingProcessors {
		if name == processor.name {
			return processor.runFunc
		}
	}
	return nil
}

type MappingRunFunc = func(values map[string]interface{}, newPath string, pathVal interface{}, args []interface{})
type MappingProcessor struct {
	name    string
	runFunc MappingRunFunc
}

var mapApiSecretKey = MappingProcessor{
	name: "mapApiSecretKey",
	runFunc: func(interim map[string]interface{}, newPath string, pathVal interface{}, args []interface{}) {
		//	if existing apikey secret, need to add key-name
		setInterim(interim, newPath, pathVal)
		setInterim(interim, "spec.global.credentials.apiSecret.keyName", "api-key")
	},
}

var mapAppSecretKey = MappingProcessor{
	name: "mapAppSecretKey",
	runFunc: func(interim map[string]interface{}, newPath string, pathVal interface{}, args []interface{}) {
		setInterim(interim, newPath, pathVal)
		setInterim(interim, "spec.global.credentials.appSecret.keyName", "app-key")
	}}

var mapTokenSecretKey = MappingProcessor{
	name: "mapTokenSecretKey",
	runFunc: func(interim map[string]interface{}, newPath string, pathVal interface{}, args []interface{}) {
		setInterim(interim, newPath, pathVal)
		setInterim(interim, "spec.global.clusterAgentTokenSecret.keyName", "token")
	},
}

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
			setInterim(interim, newPath+".type", "Localhost")
			setInterim(interim, newPath+".localhostProfile", profileName)
		case seccompValue == "runtime/default":
			setInterim(interim, newPath+".type", "RuntimeDefault")
		case seccompValue == "unconfined":
			setInterim(interim, newPath+".type", "Unconfined")
		}
	},
}

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
			if val, enabled := getBool(interim, feature); enabled && val {
				hasSystemProbeFeature = true
				break
			}
		}
		// Check if GPU is enabled
		if !hasSystemProbeFeature {
			gpuEnabled, gpuExists := getBool(interim, "spec.features.gpu.enabled")
			gpuPrivileged, privExists := getBool(interim, "spec.features.gpu.privilegedMode")
			if gpuExists && privExists && gpuEnabled && gpuPrivileged {
				hasSystemProbeFeature = true
			}
		}

		if hasSystemProbeFeature {
			// must be set to non-empty string
			setInterim(interim, newPath, appArmorValue)
		}
	},
}

var mapLocalServiceName = MappingProcessor{
	name: "mapLocalServiceName",
	runFunc: func(interim map[string]interface{}, newPath string, pathVal interface{}, args []interface{}) {
		nameOverride, ok := pathVal.(string)
		if !ok || nameOverride == "" {
			return
		}
		setInterim(interim, newPath, nameOverride)
	},
}

// mapAppendEnvVar appends environment variables to a specified path in the interim configuration.
// It takes a list of environment variable definitions in the format []map[string]interface{}{{"name": "VAR_NAME"}}
// and creates new environment variable entries with the provided pathVal as the value.
// The new variables are added to the interim map at the specified newPath.
// Example:
//   - mapFuncArgs: []interface{}{map[string]interface{}{"name": "DD_LOG_LEVEL"}}
//   - pathVal: "debug"
//   - Result: Appends {"name": "DD_LOG_LEVEL", "value": "debug"} to newPath in interim
var mapAppendEnvVar = MappingProcessor{
	name: "mapAppendEnvVar",
	runFunc: func(interim map[string]interface{}, newPath string, pathVal interface{}, args []interface{}) {
		if len(args) != 1 {
			return
		}

		envMap, ok := getMap(args[0])
		if !ok {
			log.Printf("expected map[string]interface{} for env var map definition, got %T", args[0])
			return
		}

		// Build new env var
		newEnvName, ok := getString(envMap, "name")
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
		if valFrom, vOk := getNestedValue(pathVal, "valueFrom"); vOk && valFrom != nil {
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
			setInterim(interim, newPath, []interface{}{newEnvVar})
			return
		}

		existingEnvs, ok := interim[newPath].([]interface{})
		if !ok {
			log.Printf("Error: expected []interface{} at path %s, got %T", newPath, interim[newPath])
			return
		}

		if !hasDuplicateEnv(existingEnvs, newEnvName) {
			setInterim(interim, newPath, append(existingEnvs, newEnvVar))
		}
	},
}

// mapMergeEnvs merges lists of environment variables at the specified path.
// It takes a slice of environment variable maps and merges them with any existing
// environment variables at the target path.
// Example:
//   - pathVal: []map[string]interface{}{{"name": "VAR1", "value": "val1"}}
//   - Result: Merges the new env vars with any existing ones at newPath
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
			newEnvMap, ok := getMap(newEnv)
			if !ok {
				log.Printf("Warning: expected map[string]interface{} in newEnvs, got %T", newEnv)
				continue
			}

			newName, ok := getString(newEnvMap, "name")
			if !ok || newName == "" {
				log.Printf("Warning: missing or invalid 'name' field in environment variable: %v", newEnvMap)
				continue
			}

			if !hasDuplicateEnv(mergedEnvs, newName) {
				mergedEnvs = append(mergedEnvs, newEnv)
			} else {
				// Override existing env with new env
				for i, existingEnv := range mergedEnvs {
					if existingMap, ok := getMap(existingEnv); ok {
						if existingName, ok := getString(existingMap, "name"); ok && existingName == newName {
							mergedEnvs[i] = newEnv
						}
					}
				}
			}
		}

		if len(mergedEnvs) > 0 {
			setInterim(interim, newPath, mergedEnvs)
		}
	},
}

var mapOverrideType = MappingProcessor{
	name: "mapOverrideType",
	runFunc: func(interim map[string]interface{}, newPath string, pathVal interface{}, args []interface{}) {
		{
			if len(args) != 1 {
				return
			}
			newType, ok := getString(args[0], "newType")

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
					setInterim(interim, newPath, string(newPathVal))

				case newType == "int":
					switch {
					case pathValType == "string":
						convertedInt, convErr := strconv.Atoi(pathVal.(string))
						if convErr != nil {
							log.Println(convErr)
						} else {
							setInterim(interim, newPath, convertedInt)
						}
					}
				}
			}
		}
	},
}

func hasDuplicateEnv(existingEnvs []interface{}, newEnvName string) bool {
	for _, existingEnv := range existingEnvs {
		if existingMap, ok := getMap(existingEnv); ok {
			if existingName, ok := getString(existingMap, "name"); ok && existingName == newEnvName {
				return true
			}
		}
	}
	return false
}
