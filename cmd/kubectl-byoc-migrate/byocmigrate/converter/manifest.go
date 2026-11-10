// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package converter converts the objects that the CloudPrem and Observability Pipelines Worker Helm charts rendered to
// a DatadogBYOCCluster that adopts them.
package converter

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// Manifest is the objects that a chart rendered, read from a file. Name identifies the manifest in messages.
type Manifest struct {
	Name      string
	Namespace string
	Objects   []client.Object
}

// NewManifest decodes the objects of a manifest, and sets the namespace on the objects that do not set one, as Helm and
// kubectl do when they apply them.
func NewManifest(name, namespace, manifest string) (Manifest, error) {
	objects, err := ParseManifest(manifest)
	if err != nil {
		return Manifest{}, err
	}
	for _, object := range objects {
		if object.GetNamespace() == "" {
			object.SetNamespace(namespace)
		}
	}
	return Manifest{Name: name, Namespace: namespace, Objects: objects}, nil
}

// ParseManifest decodes the objects of a rendered Helm manifest. Objects of kinds that the client-go scheme does not
// know, such as ServiceMonitor, are kept as unstructured objects.
func ParseManifest(manifest string) ([]client.Object, error) {
	reader := utilyaml.NewYAMLReader(bufio.NewReader(strings.NewReader(manifest)))
	var objects []client.Object
	for {
		document, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return objects, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read manifest: %w", err)
		}
		object, err := decodeObject(document)
		if err != nil {
			return nil, err
		}
		if object != nil {
			objects = append(objects, object)
		}
	}
}

// decodeObject returns nil for a document without an object, such as a Helm "# Source:" comment.
func decodeObject(document []byte) (client.Object, error) {
	data, err := yaml.YAMLToJSON(document)
	if err != nil {
		return nil, fmt.Errorf("convert manifest document to JSON: %w", err)
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil, nil
	}
	decoded, _, err := scheme.Codecs.UniversalDeserializer().Decode(data, nil, nil)
	if runtime.IsNotRegisteredError(err) {
		object := &unstructured.Unstructured{}
		if unmarshalErr := object.UnmarshalJSON(data); unmarshalErr != nil {
			return nil, fmt.Errorf("decode manifest document: %w", unmarshalErr)
		}
		return object, nil
	}
	if err != nil {
		return nil, fmt.Errorf("decode manifest document: %w", err)
	}
	object, ok := decoded.(client.Object)
	if !ok {
		return nil, fmt.Errorf("manifest document %T is not a Kubernetes object", decoded)
	}
	return object, nil
}
