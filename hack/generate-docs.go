package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"sort"
	"strings"

	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"
)

const (
	crdFile         = "config/crd/bases/v1/datadoghq.com_datadogagents.yaml"
	headerFile      = "hack/generate-docs/header.markdown"
	footerFile      = "hack/generate-docs/$VERSION_footer.markdown"
	v2OverridesFile = "hack/generate-docs/v2alpha1_overrides.markdown"
	docsFile        = "docs/configuration.$VERSION.md"
)

type parameterDoc struct {
	name        string
	description string
}

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

func generateContent_v2alpha1(f *os.File, crd apiextensions.CustomResourceDefinitionVersion) {
	writePropsTable(f, crd.Schema.OpenAPIV3Schema.Properties["spec"].Properties)

	overridesMarkdown := mustReadFile(v2OverridesFile)
	mustWrite(f, overridesMarkdown)
	mustWriteString(f, "\n")
	mustWriteString(f, "| Parameter | Description |\n")
	mustWriteString(f, "| --------- | ----------- |\n")

	overrideProps := crd.Schema.OpenAPIV3Schema.Properties["spec"].Properties["override"]
	writeOverridesRecursive(f, "[key]", overrideProps.AdditionalProperties.Schema.Properties)
}

func writePropsTable(f *os.File, props map[string]apiextensions.JSONSchemaProps) {
	docs := getParameterDocs([]string{}, props)

	sort.Slice(docs, func(i, j int) bool {
		return docs[i].name < docs[j].name
	})
	mustWriteString(f, "| Parameter | Description |\n")
	mustWriteString(f, "| --------- | ----------- |\n")
	for _, doc := range docs {
		mustWriteString(f, fmt.Sprintf("| %s | %s |\n", doc.name, doc.description))
	}
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

func exampleFile(version string) string {
	return fmt.Sprintf("hack/generate-docs/%s_example.markdown", version)
}

func writeOverridesRecursive(f *os.File, prefix string, props map[string]apiextensions.JSONSchemaProps) {
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
			writeOverridesRecursive(f, prefix+"."+doc.name+".[key]", valueTypeProps)
		} else {
			mustWriteString(f, fmt.Sprintf("| %s | %s |\n", prefix+"."+doc.name, doc.description))
		}
	}
}
