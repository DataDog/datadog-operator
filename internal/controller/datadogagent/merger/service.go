// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"fmt"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/dependencies"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	corev1 "k8s.io/api/core/v1"
)

// ServiceManager is used to manage service resources.
type ServiceManager interface {
	AddService(name, namespace string, selector map[string]string, ports []corev1.ServicePort, internalTrafficPolicy *corev1.ServiceInternalTrafficPolicyType) error
}

// NewServiceManager returns a new ServiceManager instance
func NewServiceManager(store dependencies.StoreClient) ServiceManager {
	manager := &serviceManagerImpl{
		store: store,
	}
	return manager
}

// serviceManagerImpl is used to manage service resources.
type serviceManagerImpl struct {
	store dependencies.StoreClient
}

// AddService creates or updates service
func (m *serviceManagerImpl) AddService(name, namespace string, selector map[string]string, ports []corev1.ServicePort, internalTrafficPolicy *corev1.ServiceInternalTrafficPolicyType) error {
	obj, _ := m.store.GetOrCreate(kubernetes.ServicesKind, namespace, name)
	service, ok := obj.(*corev1.Service)
	if !ok {
		return fmt.Errorf("unable to get from the store the Service %s", name)
	}

	if len(ports) > 0 {
		service.Spec.Ports = append(service.Spec.Ports, ports...)
	}
	if selector != nil {
		service.Spec.Selector = selector
	}
	service.Spec.Type = corev1.ServiceTypeClusterIP
	if internalTrafficPolicy != nil {
		service.Spec.InternalTrafficPolicy = internalTrafficPolicy
	}
	return m.store.AddOrUpdate(kubernetes.ServicesKind, service)
}
