// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	corev1 "k8s.io/api/core/v1"
)

// EnvFromVarManager use to manage adding Environment variable to container in a PodTemplateSpec
type EnvFromVarManager interface {
	// AddEnvFromVar use to add an envFromSource variable to all containers present in the Pod.
	AddEnvFromVar(newEnvVar *corev1.EnvFromSource)
	// AddEnvFromVarWithMergeFunc is used to add an envFromSource variable to all containers present in the Pod.
	// The way the EnvFromVar is merged with an existing EnvFromVar can be tuned thanks to the EnvFromSourceFromMergeFunction parameter.
	AddEnvFromVarWithMergeFunc(newEnvFromVar *corev1.EnvFromSource, mergeFunc EnvFromSourceFromMergeFunction) error
	// AddEnvFromVarToContainer is used to add an envFromSource to a specific container present in the Pod.
	AddEnvFromVarToContainer(containerName common.AgentContainerName, newEnvFromVar *corev1.EnvFromSource)
	// AddEnvFromVarToContainers is used to add an envFromSource variable to specified containers present in the Pod.
	AddEnvFromVarToContainers(containerNames []common.AgentContainerName, newEnvFromVar *corev1.EnvFromSource)
	// AddEnvFromVarToInitContainer is used to add an envFromSource variable to a specific init container present in the Pod.
	AddEnvFromVarToInitContainer(containerName common.AgentContainerName, newEnvFromVar *corev1.EnvFromSource)
	// AddEnvFromVarToContainerWithMergeFunc use to add an envFromSource variable to a specific container present in the Pod.
	// The way the EnvFromVar is merged with an existing EnvFromVar can be tuned thanks to the EnvFromSourceFromMergeFunction parameter.
	AddEnvFromVarToContainerWithMergeFunc(containerName common.AgentContainerName, newEnvFromVar *corev1.EnvFromSource, mergeFunc EnvFromSourceFromMergeFunction) error
}

// NewEnvFromManager returns new instance of the EnvFromVarManager
func NewEnvFromVarManager(podTmpl *corev1.PodTemplateSpec) EnvFromVarManager {
	return &envFromVarManagerImpl{
		podTmpl: podTmpl,
	}
}

type envFromVarManagerImpl struct {
	podTmpl *corev1.PodTemplateSpec
}

func (impl *envFromVarManagerImpl) AddEnvFromVar(newEnvFromVar *corev1.EnvFromSource) {
	_ = impl.AddEnvFromVarWithMergeFunc(newEnvFromVar, DefaultEnvFromSourceFromMergeFunction)
}

func (impl *envFromVarManagerImpl) AddEnvFromVarWithMergeFunc(newEnvFromVar *corev1.EnvFromSource, mergeFunc EnvFromSourceFromMergeFunction) error {
	for id, cont := range impl.podTmpl.Spec.Containers {
		if _, ok := AllAgentContainers[common.AgentContainerName(cont.Name)]; ok {
			_, err := AddEnvFromSourceFromToContainer(&impl.podTmpl.Spec.Containers[id], newEnvFromVar, mergeFunc)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (impl *envFromVarManagerImpl) AddEnvFromVarToContainer(containerName common.AgentContainerName, newEnvFromVar *corev1.EnvFromSource) {
	_ = impl.AddEnvFromVarToContainerWithMergeFunc(containerName, newEnvFromVar, DefaultEnvFromSourceFromMergeFunction)
}

func (impl *envFromVarManagerImpl) AddEnvFromVarToContainers(containerNames []common.AgentContainerName, newEnvFromVar *corev1.EnvFromSource) {
	for _, containerName := range containerNames {
		_ = impl.AddEnvFromVarToContainerWithMergeFunc(containerName, newEnvFromVar, DefaultEnvFromSourceFromMergeFunction)
	}
}

func (impl *envFromVarManagerImpl) AddEnvFromVarToInitContainer(initContainerName common.AgentContainerName, newEnvFromVar *corev1.EnvFromSource) {
	_ = impl.AddEnvFromVarToInitContainerWithMergeFunc(initContainerName, newEnvFromVar, DefaultEnvFromSourceFromMergeFunction)
}

func (impl *envFromVarManagerImpl) AddEnvFromVarToContainerWithMergeFunc(containerName common.AgentContainerName, newEnvFromVar *corev1.EnvFromSource, mergeFunc EnvFromSourceFromMergeFunction) error {
	for id := range impl.podTmpl.Spec.Containers {
		if impl.podTmpl.Spec.Containers[id].Name == string(containerName) {
			_, err := AddEnvFromSourceFromToContainer(&impl.podTmpl.Spec.Containers[id], newEnvFromVar, mergeFunc)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (impl *envFromVarManagerImpl) AddEnvFromVarToInitContainerWithMergeFunc(initContainerName common.AgentContainerName, newEnvFromVar *corev1.EnvFromSource, mergeFunc EnvFromSourceFromMergeFunction) error {
	for id := range impl.podTmpl.Spec.InitContainers {
		if impl.podTmpl.Spec.InitContainers[id].Name == string(initContainerName) {
			_, err := AddEnvFromSourceFromToContainer(&impl.podTmpl.Spec.InitContainers[id], newEnvFromVar, mergeFunc)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// EnvFromSourceFromMergeFunction signature for corev1.EnvFromSource merge function
type EnvFromSourceFromMergeFunction func(current, newEnv *corev1.EnvFromSource) (*corev1.EnvFromSource, error)

// DefaultEnvFromSourceFromMergeFunction default corev1.EnvFromSource merge function
// default correspond to OverrideCurrentEnvFromSourceMergeOption
func DefaultEnvFromSourceFromMergeFunction(current, newEnv *corev1.EnvFromSource) (*corev1.EnvFromSource, error) {
	return OverrideCurrentEnvFromSourceFromMergeFunction(current, newEnv)
}

// OverrideCurrentEnvFromSourceFromMergeFunction used when the existing corev1.EnvFromSource new to be replace by the new one.
func OverrideCurrentEnvFromSourceFromMergeFunction(current, newEnv *corev1.EnvFromSource) (*corev1.EnvFromSource, error) {
	return newEnv.DeepCopy(), nil
}

// AddEnvFromSourceFromToContainer use to add an EnvFromSource to container.
func AddEnvFromSourceFromToContainer(container *corev1.Container, envFromSource *corev1.EnvFromSource, mergeFunc EnvFromSourceFromMergeFunction) ([]corev1.EnvFromSource, error) {
	var found bool
	for id, cEnvFromSource := range container.EnvFrom {
		if cEnvFromSource.ConfigMapRef != nil && envFromSource.ConfigMapRef != nil && cEnvFromSource.ConfigMapRef.Name == envFromSource.ConfigMapRef.Name {
			found = true
		} else if cEnvFromSource.SecretRef != nil && envFromSource.SecretRef != nil && cEnvFromSource.SecretRef.Name == envFromSource.SecretRef.Name {
			found = true
		}

		if found {
			if mergeFunc == nil {
				mergeFunc = DefaultEnvFromSourceFromMergeFunction
			}
			newEnvFromSource, err := mergeFunc(&cEnvFromSource, envFromSource)
			if err != nil {
				return nil, err
			}
			container.EnvFrom[id] = *newEnvFromSource
		}
	}
	if !found {
		container.EnvFrom = append(container.EnvFrom, *envFromSource)
	}
	return container.EnvFrom, nil
}
