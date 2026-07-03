// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"errors"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// ServiceManager is used to manage service resources.
type ServiceManager interface {
	AddService(name, namespace string, selector map[string]string, ports []corev1.ServicePort, internalTrafficPolicy *corev1.ServiceInternalTrafficPolicy) error
}

// ErrServicePortConflict reports a same-name Service port with a different spec.
var ErrServicePortConflict = errors.New("service port conflict")

// NewServiceManager returns a new ServiceManager instance
func NewServiceManager(store store.StoreClient) ServiceManager {
	manager := &serviceManagerImpl{
		store: store,
	}
	return manager
}

// serviceManagerImpl is used to manage service resources.
type serviceManagerImpl struct {
	store store.StoreClient
}

// AddService creates or updates service
// If configurable fields are added or deleted, update `isEqualServiceSpec` in `pkg/equality/equality.go`
func (m *serviceManagerImpl) AddService(name, namespace string, selector map[string]string, ports []corev1.ServicePort, internalTrafficPolicy *corev1.ServiceInternalTrafficPolicy) error {
	obj, _ := m.store.GetOrCreate(kubernetes.ServicesKind, namespace, name)
	service, ok := obj.(*corev1.Service)
	if !ok {
		return fmt.Errorf("unable to get from the store the Service %s", name)
	}

	if len(ports) > 0 {
		mergedPorts, err := mergeServicePorts(service.Spec.Ports, ports)
		if err != nil {
			return fmt.Errorf("unable to add ports to Service %s: %w", name, err)
		}
		service.Spec.Ports = mergedPorts
	}
	if selector != nil {
		service.Spec.Selector = selector
	}
	service.Spec.Type = corev1.ServiceTypeClusterIP
	// k8s default InternalTrafficPolicy is Cluster
	clusterPolicy := corev1.ServiceInternalTrafficPolicyCluster
	service.Spec.InternalTrafficPolicy = &clusterPolicy
	if internalTrafficPolicy != nil {
		service.Spec.InternalTrafficPolicy = internalTrafficPolicy
	}
	return m.store.AddOrUpdate(kubernetes.ServicesKind, service)
}

func mergeServicePorts(existingPorts, newPorts []corev1.ServicePort) ([]corev1.ServicePort, error) {
	ports := append([]corev1.ServicePort{}, existingPorts...)
	portsByName := map[string]int{}
	for i, port := range ports {
		if port.Name != "" {
			portsByName[port.Name] = i
		}
	}

	for _, port := range newPorts {
		if port.Name == "" {
			ports = append(ports, port)
			continue
		}
		if existingIndex, found := portsByName[port.Name]; found {
			if !reflect.DeepEqual(ports[existingIndex], port) {
				return nil, fmt.Errorf("port %q conflicts with existing port: %w", port.Name, ErrServicePortConflict)
			}
			continue
		}
		portsByName[port.Name] = len(ports)
		ports = append(ports, port)
	}

	return ports, nil
}
