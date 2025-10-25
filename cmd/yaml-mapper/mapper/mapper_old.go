// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package mapper

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	"helm.sh/helm/v3/pkg/chartutil"
)

func MapYaml(mappingFile string, sourceFile string, destFile string, prefixFile string, ddaName string, namespace string, updateMap bool, printPtr bool) {
	// If updating mapping:
	// Use latest datadog chart values.yaml as sourceFile if none provided
	// Use default mappingFile if none provided
	tmpSourceFile := ""
	if updateMap {
		if sourceFile == "" {
			tmpSourceFile = getLatestValuesFile()
			sourceFile = tmpSourceFile
		}
	}

	if mappingFile == "" {
		latestMapping, _ := getLatestDDAMapping()
		mappingFile = latestMapping
	}

	log.Printf("Mapping values to DDA...")
	log.Println("Mapper Config: ")
	log.Println("mappingFile:", mappingFile)
	log.Println("sourceFile:", sourceFile)
	log.Println("destFile:", destFile)
	log.Println("ddaName:", ddaName)
	log.Println("namespace:", namespace)
	log.Println("updateMap:", updateMap)
	log.Println("printOutput:", printPtr)
	log.Println("")

	// Read mapping file
	mapping, err := os.ReadFile(mappingFile)
	if err != nil {
		// Fall back on embedded default mapping
		mapping = defaultDDAMap
	}
	mappingValues, err := chartutil.ReadValues(mapping)
	if err != nil {
		log.Println(err)
		return
	}

	// Read source yaml file
	source, err := os.ReadFile(sourceFile)
	if err != nil {
		log.Println(err)
		return
	}

	// Cleanup tmpSourceFile after it's been read
	if tmpSourceFile != "" {
		defer os.Remove(tmpSourceFile)
	}

	sourceValues, err := chartutil.ReadValues(source)
	if err != nil {
		log.Println(err)
		return
	}

	// Handle deprecated helm keys
	sourceValues = foldDeprecated(sourceValues)

	// Create an interim map that that has period-delimited destination key as the key, and the value from the source.yaml for the value
	//var pathVal interface{}
	var interim = map[string]interface{}{}

	if prefixFile == "" {
		interim = defaultFilePrefix
		if ddaName == "" {
			ddaName = "datadog"
		}
		setInterim(interim, "metadata.name", ddaName)

		if namespace != "" {
			setInterim(interim, "metadata.namespace", namespace)
		}
	}

	if updateMap {
		// Populate interim map with keys from latest chart's values.yaml
		interim = parseValues(sourceValues, make(map[string]interface{}), "")
		// Add back existing key values from mapping file
		for sourceKey, sourceVal := range mappingValues {
			setInterim(interim, sourceKey, sourceVal)
		}
		newMapYaml, e := chartutil.Values(interim).YAML()
		if e != nil {
			log.Println(e)
		}
		if mappingFile == defaultDDAMappingPath || tmpSourceFile != "" {
			newMapYaml = `# This file maps keys from the Datadog Helm chart (YAML) to the DatadogAgent CustomResource spec (YAML).
` + newMapYaml
		}

		if printPtr {
			log.Println("")
			log.Println(newMapYaml)
		}

		e = os.WriteFile(mappingFile, []byte(newMapYaml), 0660)
		if e != nil {
			log.Printf("Error updating mapping yaml. %v", e)
		}

		log.Printf("Mapping file, %s, successfully updated", mappingFile)
	}
	// Collect and sort mapping keys for deterministic processing order
	mappingKeys := make([]string, 0, len(mappingValues))
	for k := range mappingValues {
		mappingKeys = append(mappingKeys, k)
	}
	sort.Strings(mappingKeys)

	// Map values.yaml => DDA
	for _, sourceKey := range mappingKeys {
		pathVal, _ := sourceValues.PathValue(sourceKey)
		if pathVal == nil {
			if mapVal, ok := getMap(sourceValues[sourceKey]); ok && mapVal != nil {
				pathVal = mapVal
			} else if tableVal, err := sourceValues.Table(sourceKey); err == nil && len(tableVal) == 1 {
				pathVal = tableVal
			} else {
				continue
			}
		}

		destKey, ok := mappingValues[sourceKey]
		switch typedDestKey := destKey.(type) {
		case string:
			if destKey == "" {
				continue
			}
			if destKey == "metadata.name" {
				name := pathVal
				if ddaName != "" {
					log.Printf("Warning: found conflicting name for DDA. Mapper config provided: %s. Helm key %s provided: %v. Using Helm-provided value.", ddaName, sourceKey, name)
				}
				if s, sOk := name.(string); sOk && len(s) > 63 {
					name = s[:63]
				}
				setInterim(interim, "metadata.name", name)
				break
			}
			setInterim(interim, typedDestKey, pathVal)

		case []interface{}:
			// Provide support for the case where one source key may map to multiple destination keys
			for _, val := range typedDestKey {
				if s, sOk := val.(string); sOk {
					setInterim(interim, s, pathVal)
				} else {
					log.Printf("Warning: expected string in dest slice for %q, got %T", sourceKey, val)
				}
			}

		case map[string]interface{}:
			// Perform further processing
			newPath, _ := getString(typedDestKey, "newPath")

			if mapFuncName, mOk := getString(typedDestKey, "mapFunc"); mOk {
				args, _ := getSlice(typedDestKey, "args")
				if run := registry()[mapFuncName]; run != nil {
					run(interim, newPath, pathVal, args)
				} else {
					log.Printf("Warning: unknown mapFunc %q for %q", mapFuncName, sourceKey)
				}
			}
		default:
			if !ok || destKey == "" || destKey == nil {
				log.Printf("Warning: DDA destination key not found: %s\n", sourceKey)
				continue
			} else if interim != nil {
				setInterim(interim, destKey.(string), pathVal)
			}
		}
	}

	// Sort interim keys to ensure deterministic nesting/merge order
	interimKeys := make([]string, 0, len(interim))
	for k := range interim {
		interimKeys = append(interimKeys, k)
	}
	sort.Strings(interimKeys)

	// Create final mapping with properly nested map keys (converted from period-delimited keys)
	dda := make(map[string]interface{})
	for _, k := range interimKeys {
		v := interim[k]
		dda = makeTable(k, v, dda)
	}

	// Write final DDA mapping
	writeDDA(dda, destFile, prefixFile, printPtr)
}

func writeDDA(dda map[string]interface{}, destFile string, prefixFile string, printOutput bool) {
	// Pretty print to YAML format
	out, err := chartutil.Values(dda).YAML()
	if err != nil {
		log.Println(err)
	}

	// Read prefix yaml file
	var prefix []byte
	if prefixFile != "" {
		prefix, err = os.ReadFile(prefixFile)
		if err != nil {
			log.Println(err)
			return
		}
	}

	if len(prefix) > 0 {
		out = string(prefix) + out
	}

	if printOutput {
		log.Println("")
		log.Println(out)
	}

	// Create destination file if it doesn't exist
	_, err = os.Stat(destFile)
	if err != nil {
		file, e := os.Create(fmt.Sprintf("dda.yaml.%s", time.Now().Format("20060102-150405")))
		if e != nil {
			log.Println(e)
		}
		destFile = file.Name()
	}

	err = os.WriteFile(destFile, []byte(out), 0660)
	if err != nil {
		log.Println(err)
	}

	log.Println("YAML file successfully written to", destFile)
}
