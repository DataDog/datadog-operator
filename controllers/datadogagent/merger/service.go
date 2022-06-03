// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"fmt"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/dependencies"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// ServiceManager is used to manage service resources.
type ServiceManager interface {
	AddService(name, namespace string, selector map[string]string, ports []corev1.ServicePort, internalTrafficPolicy *corev1.ServiceInternalTrafficPolicyType) error
	BuildAgentLocalService(dda metav1.Object, nameOverride string) error
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

	service.Spec.Ports = append(service.Spec.Ports, ports...)
	service.Spec.Selector = selector
	service.Spec.Type = corev1.ServiceTypeClusterIP
	service.Spec.InternalTrafficPolicy = internalTrafficPolicy
	m.store.AddOrUpdate(kubernetes.ServicesKind, service)

	return nil
}

// BuildAgentLocalService creates a local service for the node agent
func (m *serviceManagerImpl) BuildAgentLocalService(dda metav1.Object, nameOverride string) error {
	var serviceName string
	if nameOverride != "" {
		serviceName = nameOverride
	} else {
		serviceName = component.GetAgentServiceName(dda)
	}
	serviceInternalTrafficPolicy := corev1.ServiceInternalTrafficPolicyLocal
	selector := map[string]string{
		apicommon.AgentDeploymentNameLabelKey:      dda.GetName(),
		apicommon.AgentDeploymentComponentLabelKey: apicommon.DefaultAgentResourceSuffix,
	}
	ports := []corev1.ServicePort{
		{
			Protocol:   corev1.ProtocolUDP,
			TargetPort: intstr.FromInt(apicommon.DefaultDogstatsdPort),
			Port:       apicommon.DefaultDogstatsdPort,
			Name:       apicommon.DefaultDogstatsdPortName,
		},
	}
	return m.AddService(serviceName, dda.GetNamespace(), selector, ports, &serviceInternalTrafficPolicy)
}
