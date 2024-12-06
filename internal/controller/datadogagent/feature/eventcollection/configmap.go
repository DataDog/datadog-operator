// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eventcollection

import (
	"github.com/DataDog/datadog-operator/api/crds/datadoghq/v2alpha1"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func buildDefaultConfigMap(namespace, name string, unbundleEvents bool, collectedEventTypes []v2alpha1.EventTypes) (*corev1.ConfigMap, error) {
	content, err := kubeAPIServerCheckConfig(unbundleEvents, collectedEventTypes)
	if err != nil {
		return nil, err
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string]string{
			kubeAPIServerConfigFileName: content,
		},
	}, nil
}

func kubeAPIServerCheckConfig(unbundleEvents bool, collectedEventTypes []v2alpha1.EventTypes) (string, error) {
	cm := map[string]any{
		"init_config": nil,
		"instances": []map[string]any{
			{
				"unbundle_events":       unbundleEvents,
				"collected_event_types": collectedEventTypes,
			},
		},
	}

	b, err := yaml.Marshal(cm)
	return string(b), err
}
