// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// SecretManager Kubernetes Secret Manager interface
type SecretManager interface {
	AddSecret(secretNamespace, secretName, key, value string) error
	AddAnnotations(logger logr.Logger, secretNamespace, secretName string, extraAnnotations map[string]string) error
}

// NewSecretManager return new SecretManager instance
func NewSecretManager(store store.StoreClient) SecretManager {
	manager := &secretManagerImpl{
		store: store,
	}
	return manager
}

type secretManagerImpl struct {
	store store.StoreClient
}

func (m *secretManagerImpl) AddSecret(secretNamespace, secretName, key, value string) error {
	obj, _ := m.store.GetOrCreate(kubernetes.SecretsKind, secretNamespace, secretName)
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return fmt.Errorf("unable to get the Secret %s/%s from the store", secretNamespace, secretName)
	}

	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	secret.Data[key] = []byte(value)

	return m.store.AddOrUpdate(kubernetes.SecretsKind, secret)
}

func (m *secretManagerImpl) AddAnnotations(logger logr.Logger, secretNamespace, secretName string, extraAnnotations map[string]string) error {
	obj, _ := m.store.GetOrCreate(kubernetes.SecretsKind, secretNamespace, secretName)
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return fmt.Errorf("unable to get the Secret %s/%s from the store", secretNamespace, secretName)
	}

	if len(extraAnnotations) > 0 {
		annotations := object.MergeAnnotationsLabels(logger, secret.GetAnnotations(), extraAnnotations, "*")
		secret.SetAnnotations(annotations)
	}

	return m.store.AddOrUpdate(kubernetes.SecretsKind, secret)
}
