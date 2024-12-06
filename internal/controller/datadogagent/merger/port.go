// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"github.com/DataDog/datadog-operator/api/crds/datadoghq/common"
	corev1 "k8s.io/api/core/v1"
)

// PortManager use to manage adding ports to a container in a PodTemplateSpec
type PortManager interface {
	// AddPortToContainer use to add a port to a specific container present in the Pod.
	AddPortToContainer(containerName common.AgentContainerName, newPort *corev1.ContainerPort)
	// AddPortWithMergeFunc use to add a port to a specific container present in the Pod.
	// The way the Port is merge with an existing Port can be tune thank to the PortMergeFunction parameter.
	AddPortToContainerWithMergeFunc(containerName common.AgentContainerName, newPort *corev1.ContainerPort, mergeFunc PortMergeFunction) error
}

// NewPortManager return new instance of the PortManager
func NewPortManager(podTmpl *corev1.PodTemplateSpec) PortManager {
	return &portManagerImpl{
		podTmpl: podTmpl,
	}
}

type portManagerImpl struct {
	podTmpl *corev1.PodTemplateSpec
}

func (impl *portManagerImpl) AddPortToContainer(containerName common.AgentContainerName, newPort *corev1.ContainerPort) {
	_ = impl.AddPortToContainerWithMergeFunc(containerName, newPort, DefaultPortMergeFunction)
}

func (impl *portManagerImpl) AddPortToContainerWithMergeFunc(containerName common.AgentContainerName, newPort *corev1.ContainerPort, mergeFunc PortMergeFunction) error {
	for id := range impl.podTmpl.Spec.Containers {
		if impl.podTmpl.Spec.Containers[id].Name == string(containerName) {
			_, err := AddPortToContainer(&impl.podTmpl.Spec.Containers[id], newPort, mergeFunc)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// PortMergeFunction signature for corev1.ContainerPort merge function
type PortMergeFunction func(current, newPort *corev1.ContainerPort) (*corev1.ContainerPort, error)

// DefaultPortMergeFunction default corev1.ContainerPort merge function
// default correspond to OverrideCurrentPortMergeFunction
func DefaultPortMergeFunction(current, newPort *corev1.ContainerPort) (*corev1.ContainerPort, error) {
	return OverrideCurrentPortMergeFunction(current, newPort)
}

// OverrideCurrentPortMergeFunction used when the existing corev1.ContainerPort new to be replace by the new one.
func OverrideCurrentPortMergeFunction(current, newPort *corev1.ContainerPort) (*corev1.ContainerPort, error) {
	return newPort.DeepCopy(), nil
}

// IgnoreNewPortMergeFunction used when the existing corev1.ContainerPort needs to be kept.
func IgnoreNewPortMergeFunction(current, newPort *corev1.ContainerPort) (*corev1.ContainerPort, error) {
	return current.DeepCopy(), nil
}

// ErrorOnMergeAttemptdPortMergeFunction used to avoid replacing an existing ContainerPort
func ErrorOnMergeAttemptdPortMergeFunction(current, newPort *corev1.ContainerPort) (*corev1.ContainerPort, error) {
	return nil, errMergeAttempted
}

// AddPortToContainer used to add an Port to a container
func AddPortToContainer(container *corev1.Container, newPort *corev1.ContainerPort, mergeFunc PortMergeFunction) ([]corev1.ContainerPort, error) {
	var found bool
	for id, cPort := range container.Ports {
		if cPort.Name == newPort.Name {
			if mergeFunc == nil {
				mergeFunc = DefaultPortMergeFunction
			}
			mergedPort, err := mergeFunc(&cPort, newPort)
			if err != nil {
				return nil, err
			}
			container.Ports[id] = *mergedPort
			found = true
		}
	}
	if !found {
		container.Ports = append(container.Ports, *newPort)
	}
	return container.Ports, nil
}
