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
	"strings"
	"time"

	"github.com/DataDog/datadog-operator/cmd/yaml-mapper/constants"
	"github.com/DataDog/datadog-operator/cmd/yaml-mapper/utils"
	"helm.sh/helm/v3/pkg/chartutil"
)

var (
	// defaultDDAMap Embedded Helm-to-DDA mapping file
	//go:embed mapping_datadog_helm_to_datadogagent_crd.yaml
	defaultDDAMap []byte
)

// defaultFileHeader Default file header for the mapped DDA custom resource output
var defaultFileHeader = map[string]interface{}{
	"apiVersion": "datadoghq.com/v2alpha1",
	"kind":       "DatadogAgent",
	"metadata":   map[string]interface{}{},
}

// MapConfig Configuration for the yaml mapper.
type MapConfig struct {
	MappingPath string
	SourcePath  string
	DestPath    string
	DDAName     string
	Namespace   string
	UpdateMap   bool
	PrintOutput bool
	HeaderPath  string
}

// Mapper Yaml mapper contains the mapper config and collection of mapping functions.
type Mapper struct {
	MapProcessors map[string]MappingRunFunc
	MapConfig
}

// NewMapper Returns a new Mapper instance.
func NewMapper(config MapConfig) *Mapper {
	return &Mapper{
		MapProcessors: mapFuncRegistry(),
		MapConfig:     config,
	}
}

// Run executes the Yaml Mapper based on the provided MapperConfig.
// If UpdateMap is enabled, the mapper updates the provided mapping using the provided Helm source yaml and exits.
// Otherwise, the mapper maps the Helm source yaml to a DDA custom resource and writes it to the destination file.
func (m *Mapper) Run() error {
	config := m.MapConfig
	mappingValues, sourceValues, err := m.loadInputs()
	if err != nil {
		return err
	}
	if config.UpdateMap {
		return m.updateMapping(sourceValues, mappingValues)
	}

	dda, err := m.mapValues(sourceValues, mappingValues)
	if err != nil {
		return err
	}

	return m.writeDDA(dda, config)
}

// loadInputs builds the mapping and Helm source Values from the inputted mapping and Helm source filepaths, respectively.
func (m *Mapper) loadInputs() (mappingValues chartutil.Values, sourceValues chartutil.Values, err error) {
	config := m.MapConfig
	tmpSourcePath := ""
	tmpMappingPath := ""
	sourcePath := config.SourcePath
	mappingPath := config.MappingPath

	// If updating mapping:
	// Use latest datadog chart values.yaml as sourcePath if none provided
	if config.UpdateMap && sourcePath == "" {
		tmpSourcePath = utils.FetchLatestValuesFile()
		m.MapConfig.SourcePath = tmpSourcePath

	}

	// Use latest mappingPath if none provided
	if mappingPath == "" {
		tmpMappingPath, _ = utils.FetchLatestDDAMapping()
		m.MapConfig.MappingPath = tmpMappingPath
	}

	// Read mapping file
	mapping, err := os.ReadFile(m.MapConfig.MappingPath)
	if err != nil {
		// Fall back on embedded default mapping
		mapping = defaultDDAMap
	}
	mappingValues, err = chartutil.ReadValues(mapping)
	if err != nil {
		return nil, nil, err
	}

	// Read source yaml file
	source, err := os.ReadFile(m.MapConfig.SourcePath)
	if err != nil {
		return nil, nil, err
	}

	// Cleanup tmpSourcePath after it's been read
	if tmpSourcePath != "" {
		defer os.Remove(tmpSourcePath)
	}
	if tmpMappingPath != "" {
		defer os.Remove(tmpMappingPath)
	}

	sourceValues, err = chartutil.ReadValues(source)
	if err != nil {
		return nil, nil, err
	}

	// Handle deprecated helm keys
	sourceValues = utils.ApplyDeprecationRules(sourceValues)

	return mappingValues, sourceValues, nil
}

// mapValues maps the Helm source Values to a DDA custom resource based on the provided mapping Values.
func (m *Mapper) mapValues(sourceValues chartutil.Values, mappingValues chartutil.Values) (map[string]interface{}, error) {
	var ddaName = m.MapConfig.DDAName
	var interim = map[string]interface{}{}

	if m.MapConfig.HeaderPath == "" {
		interim = defaultFileHeader
		if ddaName == "" {
			ddaName = "datadog"
		}
		utils.MergeOrSet(interim, "metadata.name", ddaName)

		if m.MapConfig.Namespace != "" {
			utils.MergeOrSet(interim, "metadata.namespace", m.MapConfig.Namespace)
		}
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
			if mapVal, ok := utils.GetPathMap(sourceValues[sourceKey]); ok && mapVal != nil {
				pathVal = mapVal
			} else if tableVal, err := sourceValues.Table(sourceKey); err == nil && len(tableVal) == 1 {
				pathVal = tableVal
			} else {
				continue
			}
		}

		destKey, _ := mappingValues[sourceKey]
		if (destKey == "" || destKey == nil) && pathVal != nil {
			log.Printf("Warning: DDA destination key not found: %s\n", sourceKey)
			continue
		}

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
				utils.MergeOrSet(interim, "metadata.name", name)
				break
			}
			utils.MergeOrSet(interim, typedDestKey, pathVal)

		case []interface{}:
			// Provide support for the case where one source key may map to multiple destination keys
			for _, val := range typedDestKey {
				if s, sOk := val.(string); sOk {
					utils.MergeOrSet(interim, s, pathVal)
				} else {
					log.Printf("Warning: expected string in dest slice for %q, got %T", sourceKey, val)
				}
			}

		case map[string]interface{}:
			// Perform further processing
			newPath, _ := utils.GetPathString(typedDestKey, "newPath")

			if mapFuncName, mOk := utils.GetPathString(typedDestKey, "mapFunc"); mOk {
				args, _ := utils.GetPathSlice(typedDestKey, "args")
				if run := m.MapProcessors[mapFuncName]; run != nil {
					run(interim, newPath, pathVal, args)
				} else {
					log.Printf("Warning: unknown mapFunc %q for %q", mapFuncName, sourceKey)
				}
			}
		default:
			if interim != nil {
				utils.MergeOrSet(interim, destKey.(string), pathVal)
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
		dda = utils.InsertAtPath(k, v, dda)
	}
	return dda, nil
}

// writeDDA writes a DDA map[string]interface{} object to a configured destination filepath.
// If the destPath is not provided, a new file is created.
func (m *Mapper) writeDDA(dda map[string]interface{}, cfg MapConfig) error {
	destPath := cfg.DestPath
	headerPath := cfg.HeaderPath

	// Pretty print to YAML format
	out, err := chartutil.Values(dda).YAML()
	if err != nil {
		return fmt.Errorf("error encoding DDA object to YAML: %w", err)
	}

	// Read header yaml file
	var header []byte
	if headerPath != "" {
		header, err = os.ReadFile(headerPath)
		if err != nil {
			return fmt.Errorf("error reading headerPath %v; %w", headerPath, err)
		}
	}

	if len(header) > 0 {
		out = string(header) + out
	}

	if cfg.PrintOutput {
		log.Println("")
		log.Println(out)
	}

	// Create destination file if it doesn't exist
	_, err = os.Stat(destPath)
	if err != nil {
		if destPath != "" {
			file, e := os.Create(destPath)
			if e != nil {
				return fmt.Errorf("error creating destination file: %v; %w", destPath, e)
			}
			destPath = file.Name()
		} else {
			newDestPath := fmt.Sprintf("dda.yaml.%v", time.Now().Format("20060102-150405"))
			file, e := os.Create(newDestPath)
			if e != nil {
				return fmt.Errorf("error creating new destination file: %v; %w", newDestPath, e)
			}
			destPath = file.Name()
		}
	}

	err = os.WriteFile(destPath, []byte(out), 0660)
	if err != nil {
		return fmt.Errorf("error writing to destination file %v; %w", destPath, err)
	}

	log.Printf("YAML file successfully written to: %v", destPath)

	return nil
}

// updateMapping merges keys from the source YAML into the mapping YAML.
// It adds any keys that exist in the source but are missing in the mapping file,
// preserving existing mappings.
func (m *Mapper) updateMapping(sourceValues chartutil.Values, mappingValues chartutil.Values) error {
	// Populate interim map with keys from latest chart's values.yaml
	interim := flattenValues(sourceValues, make(map[string]interface{}), "")
	// Add back existing key values from mapping file
	for sourceKey, sourceVal := range mappingValues {
		utils.MergeOrSet(interim, sourceKey, sourceVal)
	}
	newMapYaml, e := chartutil.Values(interim).YAML()
	if e != nil {
		return e
	}
	if strings.HasPrefix(m.MapConfig.MappingPath, constants.DefaultDDAMappingPath) {
		newMapYaml = `# This file maps keys from the Datadog Helm chart (YAML) to the DatadogAgent CustomResource spec (YAML).
` + newMapYaml
	}

	if m.MapConfig.PrintOutput {
		log.Println("")
		log.Println(newMapYaml)
	}

	e = os.WriteFile(m.MapConfig.MappingPath, []byte(newMapYaml), 0660)
	if e != nil {
		log.Printf("Error updating mapping yaml. %v", e)
		return e
	}

	log.Printf("Mapping file, %s, successfully updated", m.MapConfig.MappingPath)

	return nil
}

// flattenValues builds a mapping of dotted-key paths from a provided Values source.
func flattenValues(sourceValues chartutil.Values, valuesMap map[string]interface{}, prefix string) map[string]interface{} {
	for key, value := range sourceValues {
		currentKey := prefix + key
		// If the value is a map, recursive call to get nested keys.
		if nestedMap, ok := utils.GetPathMap(value); ok {
			flattenValues(nestedMap, valuesMap, currentKey+".")
		} else {
			valuesMap[currentKey] = ""
		}
	}
	return valuesMap
}
