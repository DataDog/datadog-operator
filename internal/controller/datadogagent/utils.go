// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

// getDDAICRDFromConfig is only used in tests
//
//lint:ignore U1000
func getDDAICRDFromConfig(sch *runtime.Scheme) (*apiextensionsv1.CustomResourceDefinition, error) {
	_, filename, _, ok := goruntime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("unable to get caller")
	}
	path := filepath.Join(filepath.Dir(filename), "..", "..", "..", "config", "crd", "bases", "v1", "datadoghq.com_datadogagentinternals.yaml")

	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	codecs := serializer.NewCodecFactory(sch)
	decoder := codecs.UniversalDeserializer()
	obj, _, err := decoder.Decode(body, nil, &apiextensionsv1.CustomResourceDefinition{})
	if err != nil {
		return nil, err
	}

	if crd, ok := obj.(*apiextensionsv1.CustomResourceDefinition); ok {
		return crd, nil
	}

	return nil, fmt.Errorf("decoded object is not a CustomResourceDefinition")
}

func getDDAIGVK() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "datadoghq.com",
		Version: "v1alpha1",
		Kind:    "DatadogAgentInternal",
	}
}
