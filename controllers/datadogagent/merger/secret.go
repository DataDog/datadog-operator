// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/controllers/datadogagent/dependencies"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// SecretManager Kubernetes Secret Manager interface
type SecretManager interface {
	AddSecret(secretNamespace, secretName, key, value string) error
}

// NewSecretManager return new SecretManager instance
func NewSecretManager(store dependencies.StoreClient) SecretManager {
	manager := &secretManagerImpl{
		store: store,
	}
	return manager
}

type secretManagerImpl struct {
	store dependencies.StoreClient
}

func (m *secretManagerImpl) AddSecret(secretNamespace, secretName, key, value string) error {
	obj, _ := m.store.GetOrCreate(kubernetes.SecretsKind, secretNamespace, secretName)
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return fmt.Errorf("unable to get from the store the Secret %s/%s", secretNamespace, secretName)
	}

	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	secret.Data[key] = []byte(value)

	m.store.AddOrUpdate(kubernetes.SecretsKind, secret)
	return nil
}
