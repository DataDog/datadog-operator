// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
)

// EnvVarManager use to manage adding Environment variable to container in a PodTemplateSpec
type EnvVarManager interface {
	// AddEnvVar use to add an environment variable to all containers present in the Pod.
	AddEnvVar(newEnvVar *corev1.EnvVar)
	// AddEnvVarWithMergeFunc use to add an environment variable to all containers present in the Pod.
	// The way the EnvVar is merge with an existing EnvVar can be tune thank to the EnvVarMergeFunction parameter.
	AddEnvVarWithMergeFunc(newEnvVar *corev1.EnvVar, mergeFunc EnvVarMergeFunction) error
	// AddEnvVarToContainer use to add an environment variable to a specific container present in the Pod.
	AddEnvVarToContainer(containerName common.AgentContainerName, newEnvVar *corev1.EnvVar)
	// AddEnvVarToContainers use to add an environment variable to specified containers present in the Pod.
	AddEnvVarToContainers(containerNames []common.AgentContainerName, newEnvVar *corev1.EnvVar)
	// AddEnvVarToInitContainer use to add an environment variable to a specific init container present in the Pod.
	AddEnvVarToInitContainer(containerName common.AgentContainerName, newEnvVar *corev1.EnvVar)
	// AddEnvVarWithMergeFunc use to add an environment variable to a specific container present in the Pod.
	// The way the EnvVar is merge with an existing EnvVar can be tune thank to the EnvVarMergeFunction parameter.
	AddEnvVarToContainerWithMergeFunc(containerName common.AgentContainerName, newEnvVar *corev1.EnvVar, mergeFunc EnvVarMergeFunction) error
}

// NewEnvVarManager return new instance of the EnvVarManager
func NewEnvVarManager(podTmpl *corev1.PodTemplateSpec) EnvVarManager {
	return &envVarManagerImpl{
		podTmpl: podTmpl,
	}
}

type envVarManagerImpl struct {
	podTmpl *corev1.PodTemplateSpec
}

func (impl *envVarManagerImpl) AddEnvVar(newEnvVar *corev1.EnvVar) {
	_ = impl.AddEnvVarWithMergeFunc(newEnvVar, DefaultEnvVarMergeFunction)
}

func (impl *envVarManagerImpl) AddEnvVarWithMergeFunc(newEnvVar *corev1.EnvVar, mergeFunc EnvVarMergeFunction) error {
	for id, cont := range impl.podTmpl.Spec.Containers {
		if _, ok := AllAgentContainers[common.AgentContainerName(cont.Name)]; ok {
			_, err := AddEnvVarToContainer(&impl.podTmpl.Spec.Containers[id], newEnvVar, mergeFunc)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (impl *envVarManagerImpl) AddEnvVarToContainer(containerName common.AgentContainerName, newEnvVar *corev1.EnvVar) {
	_ = impl.AddEnvVarToContainerWithMergeFunc(containerName, newEnvVar, DefaultEnvVarMergeFunction)
}

func (impl *envVarManagerImpl) AddEnvVarToContainers(containerNames []common.AgentContainerName, newEnvVar *corev1.EnvVar) {
	for _, containerName := range containerNames {
		_ = impl.AddEnvVarToContainerWithMergeFunc(containerName, newEnvVar, DefaultEnvVarMergeFunction)
	}
}

func (impl *envVarManagerImpl) AddEnvVarToInitContainer(initContainerName common.AgentContainerName, newEnvVar *corev1.EnvVar) {
	_ = impl.AddEnvVarToInitContainerWithMergeFunc(initContainerName, newEnvVar, DefaultEnvVarMergeFunction)
}

func (impl *envVarManagerImpl) AddEnvVarToContainerWithMergeFunc(containerName common.AgentContainerName, newEnvVar *corev1.EnvVar, mergeFunc EnvVarMergeFunction) error {
	for id := range impl.podTmpl.Spec.Containers {
		if impl.podTmpl.Spec.Containers[id].Name == string(containerName) {
			_, err := AddEnvVarToContainer(&impl.podTmpl.Spec.Containers[id], newEnvVar, mergeFunc)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (impl *envVarManagerImpl) AddEnvVarToInitContainerWithMergeFunc(initContainerName common.AgentContainerName, newEnvVar *corev1.EnvVar, mergeFunc EnvVarMergeFunction) error {
	for id := range impl.podTmpl.Spec.InitContainers {
		if impl.podTmpl.Spec.InitContainers[id].Name == string(initContainerName) {
			_, err := AddEnvVarToContainer(&impl.podTmpl.Spec.InitContainers[id], newEnvVar, mergeFunc)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// EnvVarMergeFunction signature for corev1.EnvVar merge function
type EnvVarMergeFunction func(current, newEnv *corev1.EnvVar) (*corev1.EnvVar, error)

// DefaultEnvVarMergeFunction default corev1.EnvVar merge function
// default correspond to OverrideCurrentEnvVarMergeOption
func DefaultEnvVarMergeFunction(current, newEnv *corev1.EnvVar) (*corev1.EnvVar, error) {
	return OverrideCurrentEnvVarMergeFunction(current, newEnv)
}

// OverrideCurrentEnvVarMergeFunction used when the existing corev1.EnvVar new to be replace by the new one.
func OverrideCurrentEnvVarMergeFunction(current, newEnv *corev1.EnvVar) (*corev1.EnvVar, error) {
	return newEnv.DeepCopy(), nil
}

// IgnoreNewEnvVarMergeFunction used when the existing corev1.EnvVar needs to be kept.
func IgnoreNewEnvVarMergeFunction(current, newEnv *corev1.EnvVar) (*corev1.EnvVar, error) {
	return current.DeepCopy(), nil
}

// AppendToValueEnvVarMergeFunction used when we add the new value to the existing corev1.EnvVar.
func AppendToValueEnvVarMergeFunction(current, newEnv *corev1.EnvVar) (*corev1.EnvVar, error) {
	appendEnvVar := current.DeepCopy()
	appendEnvVar.Value = strings.Join([]string{current.Value, newEnv.Value}, " ")
	return appendEnvVar, nil
}

// ErrorOnMergeAttemptdEnvVarMergeFunction used to avoid replacing an existing EnvVar
func ErrorOnMergeAttemptdEnvVarMergeFunction(current, newEnv *corev1.EnvVar) (*corev1.EnvVar, error) {
	return nil, errMergeAttempted
}

// AddEnvVarToContainer used to add an EnvVar to a container
func AddEnvVarToContainer(container *corev1.Container, envvar *corev1.EnvVar, mergeFunc EnvVarMergeFunction) ([]corev1.EnvVar, error) {
	var found bool
	for id, cEnvVar := range container.Env {
		if envvar.Name == cEnvVar.Name {
			if mergeFunc == nil {
				mergeFunc = DefaultEnvVarMergeFunction
			}
			newEnvVar, err := mergeFunc(&cEnvVar, envvar)
			if err != nil {
				return nil, err
			}
			container.Env[id] = *newEnvVar
			found = true
		}
	}
	if !found {
		container.Env = append(container.Env, *envvar)
	}
	return container.Env, nil
}
