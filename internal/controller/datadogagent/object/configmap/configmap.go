// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package configmap

import (
	"fmt"

	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/errors"
)

// BuildConfigMapConfigData use to generate a configmap containing a configuration file in yaml.
func BuildConfigMapConfigData(namespace string, configDataPointer *string, configMapName, subPath string) (*corev1.ConfigMap, error) {
	if configDataPointer == nil {
		return nil, nil
	}
	configData := *configDataPointer
	if configData == "" {
		return nil, nil
	}

	// Validate that user input is valid YAML
	// Maybe later we can implement that directly verifies against Agent configuration?
	m := make(map[any]any)
	if err := yaml.Unmarshal([]byte(configData), m); err != nil {
		return nil, fmt.Errorf("unable to parse YAML from 'customConfig.ConfigData' field: %w", err)
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
		},
		Data: map[string]string{
			subPath: configData,
		},
	}
	return configMap, nil
}

// BuildConfigMapMulti use to generate a configmap containing configuration (yaml) or check code (python).
// Use boolean `validate` to validate against yaml.
func BuildConfigMapMulti(namespace string, configDataMap map[string]string, configMapName string, validate bool) (*corev1.ConfigMap, error) {
	var errs []error
	data := make(map[string]string)

	for path, configData := range configDataMap {
		if validate {
			// Validate that user input is valid YAML
			m := make(map[any]any)
			if err := yaml.Unmarshal([]byte(configData), m); err != nil {
				errs = append(errs, fmt.Errorf("unable to parse YAML from 'multiCustomConfig.ConfigDataMap' field: %w", err))
				continue
			}
		}
		data[path] = configData
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
		},
		Data: data,
	}
	return configMap, errors.NewAggregate(errs)
}
