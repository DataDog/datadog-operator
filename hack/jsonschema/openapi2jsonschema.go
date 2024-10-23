//nolint:forbidigo
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func extractSchema(targetDir string, spec map[string]interface{}) {
	if versions, ok := spec["versions"].([]interface{}); ok {
		for _, version := range versions {
			versionData := version.(map[string]interface{})
			if schema, ok := versionData["schema"].(map[string]interface{}); ok {
				if openAPIV3Schema, ok := schema["openAPIV3Schema"].(map[string]interface{}); ok {
					processAndWriteSchema(targetDir, openAPIV3Schema, spec, versionData["name"].(string))
				}
			}
		}
	}
}

func processAndWriteSchema(targetDir string, schema map[string]interface{}, spec map[string]interface{}, versionName string) {
	schema = additionalProperties(schema, true)
	schema = replaceIntOrString(schema)

	name := spec["names"].(map[string]interface{})["plural"].(string)
	group := spec["group"]

	filename := fmt.Sprintf("%s_%s_%s.json", group, name, versionName)
	filename = strings.ToLower(filename)
	writeSchemaFile(schema, filepath.Join(targetDir, filename))
}

func additionalProperties(data map[string]interface{}, skip bool) map[string]interface{} {
	if _, exists := data["properties"]; exists && !skip {
		if _, exists := data["additionalProperties"]; !exists {
			data["additionalProperties"] = false
		}
	}

	for _, v := range data {
		if subData, ok := v.(map[string]interface{}); ok {
			additionalProperties(subData, false)
		}
	}

	return data
}

func replaceIntOrString(data map[string]interface{}) map[string]interface{} {
	if format, exists := data["format"]; exists && format == "int-or-string" {
		data["oneOf"] = []interface{}{
			map[string]interface{}{"type": "string"},
			map[string]interface{}{"type": "integer"},
		}
		delete(data, "format")
	}

	for _, v := range data {
		if subData, ok := v.(map[string]interface{}); ok {
			replaceIntOrString(subData)
		}
	}

	return data
}

func writeSchemaFile(schema map[string]interface{}, filename string) {
	schemaJSON, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		fmt.Printf("Error marshalling JSON: %v\n", err)
		return
	}

	if err := os.WriteFile(filename, schemaJSON, 0o644); err != nil {
		fmt.Printf("Error writing file %s: %v\n", filename, err)
	}
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Missing FILE parameter.\nUsage: openapi2jsonschema [FILE]")
		os.Exit(1)
	}

	for _, crdFile := range os.Args[1:] {
		f, err := os.ReadFile(crdFile)
		if err != nil {
			fmt.Printf("Error reading file %s: %v\n", crdFile, err)
			continue
		}

		targetDir := filepath.Dir(crdFile)
		var y map[string]interface{}
		if err := yaml.Unmarshal(f, &y); err != nil {
			fmt.Printf("Error unmarshalling YAML: %v\n", err)
			continue
		}

		if spec, ok := y["spec"].(map[string]interface{}); ok {
			extractSchema(targetDir, spec)
		}
	}
}
