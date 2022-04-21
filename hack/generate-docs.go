package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"sort"
	"strings"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	"sigs.k8s.io/yaml"
)

const (
	crdFile       = "config/crd/bases/v1/datadoghq.com_datadogagents.yaml"
	targetVersion = "v1alpha1"
	headerFile    = "hack/generate-docs/header.markdown"
	footerFile    = "hack/generate-docs/footer.markdown"
	docsFile      = "docs/configuration.md"
)

type parameterDoc struct {
	name        string
	description string
}

func main() {
	crdYaml := mustReadFile(crdFile)
	header := mustReadFile(headerFile)
	footer := mustReadFile(footerFile)

	crd := apiextensions.CustomResourceDefinition{}
	err := yaml.Unmarshal(crdYaml, &crd)
	if err != nil {
		panic(fmt.Sprintf("cannot unmarshal yaml CRD: %s", err))
	}

	var props map[string]apiextensions.JSONSchemaProps
	for _, crdVersion := range crd.Spec.Versions {
		if crdVersion.Name == targetVersion {
			props = crdVersion.Schema.OpenAPIV3Schema.Properties["spec"].Properties
			break
		}
	}

	docs := getParameterDocs([]string{}, props)

	sort.Slice(docs, func(i, j int) bool {
		return docs[i].name < docs[j].name
	})

	f, err := os.OpenFile(docsFile, os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		panic(fmt.Sprintf("cannot write to file: %s", err))
	}

	defer func() {
		if err := f.Close(); err != nil {
			panic(fmt.Sprintf("cannot close file: %s", err))
		}
	}()

	mustWrite(f, header)
	mustWriteString(f, "| Parameter | Description |\n")
	mustWriteString(f, "| --------- | ----------- |\n")
	for _, doc := range docs {
		mustWriteString(f, fmt.Sprintf("| %s | %s |\n", doc.name, doc.description))
	}
	mustWrite(f, footer)
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
				description: strings.ReplaceAll(prop.Description, "\n", ""),
			},
		}
	}

	return getParameterDocs(path, prop.Properties)
}
