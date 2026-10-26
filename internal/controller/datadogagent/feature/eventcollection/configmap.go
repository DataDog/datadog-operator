// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eventcollection

import (
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

func buildDefaultConfigMap(namespace, name string, unbundleEvents bool, collectedEventTypes []v2alpha1.EventTypes, collectionMode string) (*corev1.ConfigMap, error) {
	content, err := kubeAPIServerCheckConfig(unbundleEvents, collectedEventTypes, collectionMode)
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

func kubeAPIServerCheckConfig(unbundleEvents bool, collectedEventTypes []v2alpha1.EventTypes, collectionMode string) (string, error) {
	instance := map[string]any{
		"unbundle_events": unbundleEvents,
	}

	// only set collected_event_types when there are some, so a watch-only config
	// doesn't carry an empty list - effectively omitempty
	if len(collectedEventTypes) > 0 {
		instance["collected_event_types"] = collectedEventTypes
	}

	// conditionally set event_collection_mode so that an explicit empty string ""
	// does not cause a misconfiguration - effectively omitempty
	if collectionMode != "" {
		instance["event_collection_mode"] = collectionMode
	}

	cm := map[string]any{
		"init_config": nil,
		"instances": []map[string]any{
			instance,
		},
	}

	b, err := yaml.Marshal(cm)
	return string(b), err
}
