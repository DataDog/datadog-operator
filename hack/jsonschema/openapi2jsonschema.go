//nolint:forbidigo
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

var (
	optionPatchIntOrString bool

	intOrStringSchema = apiextensions.JSONSchemaProps{
		AnyOf: []apiextensions.JSONSchemaProps{
			{Type: "string"},
			{Type: "integer"},
		},
		Pattern: "^(\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))(([KMGTPE]i)|[numkMGTPE]|([eE](\\+|-)?(([0-9]+(\\.[0-9]*)?)|(\\.[0-9]+))))?$",
	}
)

// Will modify schema in place
func processSchema(schema *apiextensions.JSONSchemaProps) {
	if schema == nil {
		return
	}

	// Replace int-or-string map with deddicated fields + oneOf
	// We process from parent PoV as we need to change the parent object
	if schema.AdditionalProperties != nil {
		if schema.AdditionalProperties.Schema != nil {
			if optionPatchIntOrString && schema.AdditionalProperties.Schema.XIntOrString {
				// Add dedicated quantity fields, if we add a new type of resource, we need to add there
				schema.Properties = map[string]apiextensions.JSONSchemaProps{
					"cpu":    intOrStringSchema,
					"memory": intOrStringSchema,
				}

				// Empty the additional properties
				schema.AdditionalProperties = &apiextensions.JSONSchemaPropsOrBool{
					Allows: false,
				}
			} else {
				processSchema(schema.AdditionalProperties.Schema)
			}
		}
	}

	if len(schema.Properties) > 0 && (schema.AdditionalProperties == nil || schema.AdditionalProperties.Schema == nil) {
		schema.AdditionalProperties = &apiextensions.JSONSchemaPropsOrBool{
			Allows: false,
		}
	}

	// Recursively process children
	for key, child := range schema.Properties {
		processSchema(&child)
		schema.Properties[key] = child
	}

	for key, child := range schema.PatternProperties {
		processSchema(&child)
		schema.PatternProperties[key] = child
	}

	if schema.Items != nil {
		processSchema(schema.Items.Schema)
		for i := range schema.Items.JSONSchemas {
			processSchema(&schema.Items.JSONSchemas[i])
		}
	}
}

func writeSchemaFile(schema *apiextensions.JSONSchemaProps, filename string) error {
	// Use remarshal to get a consistent JSON output
	b, err := json.Marshal(schema)
	if err != nil {
		return fmt.Errorf("error marshalling JSON: %w", err)
	}

	var ifce any
	err = json.Unmarshal(b, &ifce)
	if err != nil {
		return fmt.Errorf("error marshalling JSON: %w", err)
	}

	schemaJSON, err := json.MarshalIndent(ifce, "", "  ")
	if err != nil {
		return fmt.Errorf("error marshalling JSON: %w", err)
	}

	if err := os.WriteFile(filename, schemaJSON, 0o644); err != nil {
		return fmt.Errorf("error writing file %s: %w", filename, err)
	}

	return nil
}

func convert(filePath string) error {
	crdYaml, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("error reading file %s: %w", filePath, err)
	}

	crd := apiextensions.CustomResourceDefinition{}
	err = yaml.Unmarshal(crdYaml, &crd)
	if err != nil {
		return fmt.Errorf("cannot unmarshal yaml CRD: %w", err)
	}

	targetDir := filepath.Dir(filePath)
	for _, version := range crd.Spec.Versions {
		targetFileName := strings.ToLower(fmt.Sprintf("%s_%s_%s.json", crd.Spec.Group, crd.Spec.Names.Plural, version.Name))
		processSchema(version.Schema.OpenAPIV3Schema)
		err = writeSchemaFile(version.Schema.OpenAPIV3Schema, filepath.Join(targetDir, targetFileName))
		if err != nil {
			return err
		}
	}

	return nil
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Incorrect parameters.\nUsage: openapi2jsonschema [FILE]")
		os.Exit(1)
	}

	optionPatchIntOrString = os.Getenv("OPT_PATCH_RESOURCE_LIST") == "true"

	err := convert(os.Args[1])
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
