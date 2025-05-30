// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// ConfigMapManager is used to manage configmap resources.
type ConfigMapManager interface {
	AddConfigMap(name, namespace string, data map[string]string) error
}

// NewConfigMapManager returns a new ConfigMapManager instance
func NewConfigMapManager(store store.StoreClient) ConfigMapManager {
	manager := &configMapManagerImpl{
		store: store,
	}
	return manager
}

// configMapManagerImpl is used to manage configmap resources.
type configMapManagerImpl struct {
	store store.StoreClient
}

// AddConfigMap creates or updates a kubernetes network policy
func (m *configMapManagerImpl) AddConfigMap(name, namespace string, data map[string]string) error {
	obj, _ := m.store.GetOrCreate(kubernetes.ConfigMapKind, namespace, name)
	cm, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return fmt.Errorf("unable to get from the store the ConfigMap %s/%s", namespace, name)
	}

	cm.Data = data

	return m.store.AddOrUpdate(kubernetes.ConfigMapKind, cm)
}
