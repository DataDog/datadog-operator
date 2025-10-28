package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"sort"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

const (
	crdFile                 = "config/crd/bases/v1/datadoghq.com_datadogagents.yaml"
	publicHeaderFile        = "hack/generate-docs/public_header.markdown"
	publicFooterFile        = "hack/generate-docs/public_footer.markdown"
	publicOverridesFile     = "hack/generate-docs/public_overrides.markdown"
	publicDocsFile          = "docs/configuration_public.md"
	headerFile              = "hack/generate-docs/header.markdown"
	footerFile              = "hack/generate-docs/$VERSION_footer.markdown"
	v2OverridesFile         = "hack/generate-docs/v2alpha1_overrides.markdown"
	docsFile                = "docs/configuration.$VERSION.md"
	updatedDescriptionsFile = "hack/generate-docs/updated_descriptions.json"
	typesFile               = "api/datadoghq/v2alpha1/datadogagent_types.go"
)

type parameterDoc struct {
	name        string
	description string
}

type OutputFormat string

const (
	FormatTable OutputFormat = "table" // current format
	FormatHugo  OutputFormat = "hugo"  // new public format
)

func main() {
	crdYaml := mustReadFile(crdFile)
	header := mustReadFile(headerFile)

	crd := apiextensions.CustomResourceDefinition{}
	err := yaml.Unmarshal(crdYaml, &crd)
	if err != nil {
		panic(fmt.Sprintf("cannot unmarshal yaml CRD: %s", err))
	}

	for _, crdVersion := range crd.Spec.Versions {
		generateDoc(header, crdVersion, crdVersion.Name)
		generatePublicDoc(crdVersion, crdVersion.Name)
	}

}

func generateDoc(header []byte, crd apiextensions.CustomResourceDefinitionVersion, version string) {
	file := strings.Replace(docsFile, "$VERSION", version, 1)
	footerFile := strings.Replace(footerFile, "$VERSION", version, 1)
	footer := mustReadFile(footerFile)
	f, err := os.OpenFile(file, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		panic(fmt.Sprintf("cannot write to file: %s", err))
	}

	defer func() {
		if err := f.Close(); err != nil {
			panic(fmt.Sprintf("cannot close file: %s", err))
		}
	}()

	// Write header and example yaml
	exampleYaml := mustReadFile(exampleFile(version))

	mustWrite(f, header)
	mustWriteString(f, "\n")
	mustWrite(f, exampleYaml)
	mustWriteString(f, "\n")

	// Write prop content
	var generator = map[string]func(*os.File, apiextensions.CustomResourceDefinitionVersion){
		"v2alpha1": generateContent_v2alpha1,
	}
	generator[version](f, crd)

	// Write footer
	mustWrite(f, footer)
}

func generatePublicDoc(crd apiextensions.CustomResourceDefinitionVersion, version string) {
	publicHeader := mustReadFile(publicHeaderFile)
	publicFooter := mustReadFile(publicFooterFile)
	publicOverride := mustReadFile(publicOverridesFile)

	f, err := os.OpenFile(publicDocsFile, os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		panic(fmt.Sprintf("cannot write to public docs file: %s", err))
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			panic(fmt.Sprintf("cannot close file: %s", closeErr))
		}
	}()

	// Write header
	mustWrite(f, publicHeader)
	mustWriteString(f, "\n")

	// Load doc-gen annotations from Go source
	annotations, err := ParseDocGenAnnotations(typesFile)
	if err != nil {
		panic(fmt.Sprintf("failed to parse doc-gen annotations: %s", err))
	}

	generatePublicContent(f, crd, annotations)
	mustWrite(f, publicOverride)

	mustWrite(f, publicFooter)
}

func generateContent_v2alpha1(f *os.File, crd apiextensions.CustomResourceDefinitionVersion) {
	nameToDescMap := loadJSONToMap(updatedDescriptionsFile)
	writePropsTable(f, crd.Schema.OpenAPIV3Schema.Properties["spec"].Properties, nameToDescMap)

	overridesMarkdown := mustReadFile(v2OverridesFile)
	mustWrite(f, overridesMarkdown)
	mustWriteString(f, "\n")
	mustWriteString(f, "| Parameter | Description |\n")
	mustWriteString(f, "| --------- | ----------- |\n")

	overrideProps := crd.Schema.OpenAPIV3Schema.Properties["spec"].Properties["override"]
	writeOverridesRecursive(f, "[key]", overrideProps.AdditionalProperties.Schema.Properties, nameToDescMap)
}

func generatePublicContent(f *os.File, crd apiextensions.CustomResourceDefinitionVersion, annotations map[string]FieldAnnotation) {
	nameToDescMap := loadJSONToMap(updatedDescriptionsFile)

	writePropsTablePublic(f, "global-options-list", crd.Schema.OpenAPIV3Schema.Properties["spec"].Properties, nameToDescMap, annotations)

	// Write overrides section content
	mustWriteString(f, "{{% collapse-content title=\"Parameters\" level=\"h4\" expanded=true id=\"overrides-list\" %}}\n\n")
	overrideProps := crd.Schema.OpenAPIV3Schema.Properties["spec"].Properties["override"]
	// Start annotation path with "override" to match annotation keys like "override.containers.livenessProbe"
	writeOverridesRecursivePublic(f, "[key]", []string{"override"}, overrideProps.AdditionalProperties.Schema.Properties, nameToDescMap, annotations)
	mustWriteString(f, "{{% /collapse-content %}}\n\n")
}

func writePropsTable(f *os.File, props map[string]apiextensions.JSONSchemaProps, nameToDescMap map[string]string) {
	docs := getParameterDocs([]string{}, props)

	sort.Slice(docs, func(i, j int) bool {
		return docs[i].name < docs[j].name
	})
	mustWriteString(f, "| Parameter | Description |\n")
	mustWriteString(f, "| --------- | ----------- |\n")
	for _, doc := range docs {
		desc := doc.description
		if newDesc, ok := nameToDescMap[doc.name]; ok {
			// Replace imported description with manual edits
			desc = newDesc
		} else {
			// If needed, remove the name of the parameter from the description itself. This is done for visual appeal in our public docs.
			// Isolate parameter name from full period-delimited name
			paramName := doc.name[strings.LastIndex(doc.name, ".")+1:]
			prefix := cases.Title(language.English, cases.Compact).String(paramName) + " "
			// Remove parameter name from description
			desc = strings.TrimPrefix(desc, prefix)
			// Capitalize new beginning word of description
			desc = strings.ToUpper(desc[:1]) + desc[1:]
		}

		mustWriteString(f, fmt.Sprintf("| %s | %s |\n", doc.name, desc))
	}
}

func writePropsTablePublic(f *os.File, sectionId string, props map[string]apiextensions.JSONSchemaProps, nameToDescMap map[string]string, annotations map[string]FieldAnnotation) {
	// For global props, displayPath and annotationPath are the same (both start empty)
	docs := getParameterDocsPublic([]string{}, []string{}, props, annotations)
	sort.Slice(docs, func(i, j int) bool {
		return docs[i].name < docs[j].name
	})

	mustWriteString(f, fmt.Sprintf("{%% collapse-content title=\"Parameters\" level=\"h4\" expanded=true id=\"%s\" %%}}\n\n", sectionId))
	for _, doc := range docs {
		desc := doc.description
		if newDesc, ok := nameToDescMap[doc.name]; ok {
			desc = newDesc
		} else {
			// Clean up description (same logic as existing)
			paramName := doc.name[strings.LastIndex(doc.name, ".")+1:]
			prefix := cases.Title(language.English, cases.Compact).String(paramName) + " "
			desc = strings.TrimPrefix(desc, prefix)
			desc = strings.ToUpper(desc[:1]) + desc[1:]
		}
		mustWriteString(f, fmt.Sprintf("`%s`", doc.name))
		mustWriteString(f, fmt.Sprintf(": %s\n\n", desc))
	}
	mustWriteString(f, "{{% /collapse-content %}}\n\n")
}

func mustReadFile(path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		panic(fmt.Sprintf("cannot open file %q: %s", path, err))
	}

	defer func() {
		if err = f.Close(); err != nil {
			panic(fmt.Sprintf("cannot close file: %s", err))
		}
	}()

	b, err := ioutil.ReadAll(f)
	if err != nil {
		panic(fmt.Sprintf("cannot read file %q: %s", path, err))
	}

	return b
}

func mustWrite(f io.Writer, b []byte) {
	if _, err := f.Write(b); err != nil {
		panic(fmt.Sprintf("cannot write to file: %s", err))
	}
}

func mustWriteString(f io.StringWriter, b string) {
	if _, err := f.WriteString(b); err != nil {
		panic(fmt.Sprintf("cannot write to file: %s", err))
	}
}

func getParameterDocs(path []string, props map[string]apiextensions.JSONSchemaProps) []parameterDoc {
	parameterDocs := []parameterDoc{}
	for name, prop := range props {
		parameterDocs = append(parameterDocs, getParameterDoc(path, name, prop)...)
	}

	return parameterDocs
}

func getParameterDoc(path []string, name string, prop apiextensions.JSONSchemaProps) []parameterDoc {
	path = append(path, name)
	if len(prop.Properties) == 0 {
		return []parameterDoc{
			{
				name:        strings.Join(path, "."),
				description: strings.ReplaceAll(prop.Description, "\n", " "),
			},
		}
	}

	return getParameterDocs(path, prop.Properties)
}

// getParameterDocsPublic is like getParameterDocs but respects +doc-gen: annotations
// Takes two path parameters:
// - displayPath: used for building the display name (e.g., "livenessProbe.exec.command")
// - annotationPath: used for looking up annotations (e.g., "containers.livenessProbe")
func getParameterDocsPublic(displayPath []string, annotationPath []string, props map[string]apiextensions.JSONSchemaProps, annotations map[string]FieldAnnotation) []parameterDoc {
	parameterDocs := []parameterDoc{}
	for name, prop := range props {
		parameterDocs = append(parameterDocs, getParameterDocPublic(displayPath, annotationPath, name, prop, annotations)...)
	}

	return parameterDocs
}

// getParameterDocPublic is like getParameterDoc but respects +doc-gen: annotations
// Uses two paths:
// - displayPath: for building doc.name (display purposes)
// - annotationPath: for looking up annotations (e.g., "containers.livenessProbe")
func getParameterDocPublic(displayPath []string, annotationPath []string, name string, prop apiextensions.JSONSchemaProps, annotations map[string]FieldAnnotation) []parameterDoc {
	displayPath = append(displayPath, name)
	annotationPath = append(annotationPath, name)

	displayName := strings.Join(displayPath, ".")
	annotationKey := strings.Join(annotationPath, ".")

	// Check for annotations on this field using the annotation path
	annotation, hasAnnotation := annotations[annotationKey]

	// Handle +doc-gen:exclude - skip this field and all children
	if hasAnnotation && annotation.Exclude {
		return []parameterDoc{}
	}

	// Handle +doc-gen:truncate - show field but don't recurse
	if hasAnnotation && annotation.Truncate {
		desc := strings.ReplaceAll(prop.Description, "\n", " ")
		return []parameterDoc{
			{
				name:        displayName,
				description: desc,
			},
		}
	}

	// Handle +doc-gen:link - show field with link, don't recurse
	if hasAnnotation && annotation.Link != "" {
		desc := strings.ReplaceAll(prop.Description, "\n", " ")
		// Append link to description
		desc = fmt.Sprintf("%s See [link](%s) for more information.", desc, annotation.Link)
		return []parameterDoc{
			{
				name:        displayName,
				description: desc,
			},
		}
	}

	// No special annotations - proceed normally
	if len(prop.Properties) == 0 {
		return []parameterDoc{
			{
				name:        displayName,
				description: strings.ReplaceAll(prop.Description, "\n", " "),
			},
		}
	}

	// Recurse into nested properties
	return getParameterDocsPublic(displayPath, annotationPath, prop.Properties, annotations)
}

func exampleFile(version string) string {
	return fmt.Sprintf("hack/generate-docs/%s_example.markdown", version)
}

func writeOverridesRecursive(f *os.File, prefix string, props map[string]apiextensions.JSONSchemaProps, nameToDescMap map[string]string) {
	docs := getParameterDocs([]string{}, props)

	sort.Slice(docs, func(i, j int) bool {
		return docs[i].name < docs[j].name
	})
	for _, doc := range docs {
		if props[doc.name].Type == "array" {
			// https://swagger.io/docs/specification/data-models/data-types/#array
			arrayType := props[doc.name].Items.Schema.Type
			propName := prefix + "." + doc.name
			mustWriteString(f, fmt.Sprintf("| %s `[]%s` | %s |\n", propName, arrayType, doc.description))
		} else if !strings.Contains(doc.name, ".") && props[doc.name].AdditionalProperties != nil {
			// https://swagger.io/docs/specification/data-models/dictionaries/
			mapKeyType := "string"
			mapValueType := props[doc.name].AdditionalProperties.Schema.Type
			propName := prefix + "." + doc.name
			mustWriteString(f, fmt.Sprintf("| %s `map[%s]%s` | %s |\n", propName, mapKeyType, mapValueType, doc.description))
			valueTypeProps := props[doc.name].AdditionalProperties.Schema.Properties
			writeOverridesRecursive(f, prefix+"."+doc.name+".[key]", valueTypeProps, nameToDescMap)
		} else {
			name := prefix + "." + doc.name
			desc := doc.description
			if newDesc, ok := nameToDescMap[name]; ok {
				desc = newDesc
			}
			mustWriteString(f, fmt.Sprintf("| %s | %s |\n", name, desc))
		}
	}
}

// writeOverridesRecursivePublic handles public documentation generation for override fields
func writeOverridesRecursivePublic(f *os.File, prefix string, annotationPath []string, props map[string]apiextensions.JSONSchemaProps, nameToDescMap map[string]string, annotations map[string]FieldAnnotation) {
	// displayPath always starts fresh (like internal), annotationPath accumulates path for annotation lookup in the map
	docs := getParameterDocsPublic([]string{}, annotationPath, props, annotations)

	sort.Slice(docs, func(i, j int) bool {
		return docs[i].name < docs[j].name
	})

	for _, doc := range docs {
		if props[doc.name].Type == "array" {
			arrayType := props[doc.name].Items.Schema.Type
			propName := prefix + "." + doc.name
			mustWriteString(f, fmt.Sprintf("`%s` `[]%s`: %s\n\n", propName, arrayType, doc.description))
		} else if props[doc.name].AdditionalProperties != nil {
			mapKeyType := "string"
			mapValueType := props[doc.name].AdditionalProperties.Schema.Type
			propName := prefix + "." + doc.name
			mustWriteString(f, fmt.Sprintf("`%s` `map[%s]%s`: %s\n\n", propName, mapKeyType, mapValueType, doc.description))

			valueTypeProps := props[doc.name].AdditionalProperties.Schema.Properties
			// Accumulate annotation path with field name (NOT [key])
			newAnnotationPath := append(annotationPath, doc.name)
			// Recurse: displayPath resets to []string{}, annotationPath accumulates
			writeOverridesRecursivePublic(f, prefix+"."+doc.name+".[key]", newAnnotationPath, valueTypeProps, nameToDescMap, annotations)
		} else {
			name := prefix + "." + doc.name
			desc := doc.description
			if newDesc, ok := nameToDescMap[name]; ok {
				desc = newDesc
			}
			mustWriteString(f, fmt.Sprintf("`%s`: %s\n\n", name, desc))
		}
	}
}

func loadJSONToMap(filename string) map[string]string {
	result := make(map[string]string)
	fileBytes := mustReadFile(filename)
	json.Unmarshal(fileBytes, &result)
	return result
}
