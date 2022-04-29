// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package configmap

import (
	"fmt"

	"gopkg.in/yaml.v2"

	"github.com/DataDog/datadog-operator/controllers/datadogagent/object"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BuildConfiguration use to generate a configmap containing a configuration file in yaml.
func BuildConfiguration(owner metav1.Object, configDataPointer *string, configMapName, subPath string) (*corev1.ConfigMap, error) {
	if configDataPointer == nil {
		return nil, nil
	}
	configData := *configDataPointer
	if configData == "" {
		return nil, nil
	}

	// Validate that user input is valid YAML
	// Maybe later we can implement that directly verifies against Agent configuration?
	m := make(map[interface{}]interface{})
	if err := yaml.Unmarshal([]byte(configData), m); err != nil {
		return nil, fmt.Errorf("unable to parse YAML from 'customConfig.ConfigData' field: %w", err)
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        configMapName,
			Namespace:   owner.GetNamespace(),
			Labels:      object.GetDefaultLabels(owner, owner.GetName(), ""),
			Annotations: object.GetDefaultAnnotations(owner),
		},
		Data: map[string]string{
			subPath: configData,
		},
	}
	return configMap, nil
}
