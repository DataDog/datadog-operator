// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"github.com/DataDog/datadog-operator/api/crds/datadoghq/common"
	corev1 "k8s.io/api/core/v1"
)

// VolumeMountManager use to add a Volume to Pod and associated containers.
type VolumeMountManager interface {
	// Add the volumeMount to all containers of the PodTemplate.
	AddVolumeMount(volumeMount *corev1.VolumeMount)
	// Add the volumeMount to one container of the PodTemplate.
	AddVolumeMountToContainer(volumeMount *corev1.VolumeMount, containerName common.AgentContainerName)
	// Add the volumeMount to an init container pfo the PodTemplate.
	AddVolumeMountToInitContainer(volumeMount *corev1.VolumeMount, containerName common.AgentContainerName)
	// Add the volumeMount to a list of containers in the PodTemplate.
	AddVolumeMountToContainers(volumeMount *corev1.VolumeMount, containerNames []common.AgentContainerName)
	// Add the volumeMount to the container matching the containerName.
	// Provide merge functions if the merge is specific.
	AddVolumeMountToContainerWithMergeFunc(volumeMount *corev1.VolumeMount, containerName common.AgentContainerName, volumeMountMergeFunc VolumeMountMergeFunction) error
}

// NewVolumeMountManager returns a new instance of the VolumeMountManager
func NewVolumeMountManager(podTmpl *corev1.PodTemplateSpec) VolumeMountManager {
	return &volumeMountManagerImpl{
		podTmpl: podTmpl,
	}
}

type volumeMountManagerImpl struct {
	podTmpl *corev1.PodTemplateSpec
}

func (impl *volumeMountManagerImpl) AddVolumeMount(volumeMount *corev1.VolumeMount) {
	_ = impl.AddVolumeMountWithMergeFunc(volumeMount, DefaultVolumeMountMergeFunction)
}

func (impl *volumeMountManagerImpl) AddVolumeMountToContainer(volumeMount *corev1.VolumeMount, containerName common.AgentContainerName) {
	for id, container := range impl.podTmpl.Spec.Containers {
		if container.Name == string(containerName) {
			_, _ = AddVolumeMountToContainerWithMergeFunc(&impl.podTmpl.Spec.Containers[id], volumeMount, DefaultVolumeMountMergeFunction)
		}
	}
}

func (impl *volumeMountManagerImpl) AddVolumeMountToInitContainer(volumeMount *corev1.VolumeMount, containerName common.AgentContainerName) {
	for id, container := range impl.podTmpl.Spec.InitContainers {
		if container.Name == string(containerName) {
			_, _ = AddVolumeMountToContainerWithMergeFunc(&impl.podTmpl.Spec.InitContainers[id], volumeMount, DefaultVolumeMountMergeFunction)
		}
	}
}

func (impl *volumeMountManagerImpl) AddVolumeMountToContainers(volumeMount *corev1.VolumeMount, containerNames []common.AgentContainerName) {
	for _, containerName := range containerNames {
		impl.AddVolumeMountToContainer(volumeMount, containerName)
	}
}

func (impl *volumeMountManagerImpl) AddVolumeMountWithMergeFunc(volumeMount *corev1.VolumeMount, volumeMountMergeFunc VolumeMountMergeFunction) error {
	for id, cont := range impl.podTmpl.Spec.Containers {
		if _, ok := AllAgentContainers[common.AgentContainerName(cont.Name)]; ok {
			if _, err := AddVolumeMountToContainerWithMergeFunc(&impl.podTmpl.Spec.Containers[id], volumeMount, volumeMountMergeFunc); err != nil {
				return err
			}
		}
	}
	return nil
}

func (impl *volumeMountManagerImpl) AddVolumeMountToContainerWithMergeFunc(volumeMount *corev1.VolumeMount, containerName common.AgentContainerName, volumeMountMergeFunc VolumeMountMergeFunction) error {
	for id, container := range impl.podTmpl.Spec.Containers {
		if container.Name == string(containerName) {
			_, err := AddVolumeMountToContainerWithMergeFunc(&impl.podTmpl.Spec.Containers[id], volumeMount, volumeMountMergeFunc)
			return err
		}
	}
	return nil
}

// AddVolumeMountToContainerWithMergeFunc is used to add a corev1.VolumeMount to a container
// the mergeFunc can be provided to change the default merge behavior
func AddVolumeMountToContainerWithMergeFunc(container *corev1.Container, volumeMount *corev1.VolumeMount, mergeFunc VolumeMountMergeFunction) ([]corev1.VolumeMount, error) {
	var found bool
	for id, cVolumeMount := range container.VolumeMounts {
		if volumeMount.Name == cVolumeMount.Name || volumeMount.MountPath == cVolumeMount.MountPath {
			if mergeFunc == nil {
				mergeFunc = DefaultVolumeMountMergeFunction
			}
			newVolumeMount, err := mergeFunc(&cVolumeMount, volumeMount)
			if err != nil {
				return nil, err
			}
			container.VolumeMounts[id] = *newVolumeMount
			found = true
		}
	}

	if !found {
		container.VolumeMounts = append(container.VolumeMounts, *volumeMount)
	}
	return container.VolumeMounts, nil
}

// VolumeMountMergeFunction signature for corev1.VolumeMount merge function
type VolumeMountMergeFunction func(current, newVolumeMount *corev1.VolumeMount) (*corev1.VolumeMount, error)

// DefaultVolumeMountMergeFunction default corev1.VolumeMount merge function
// default correspond to OverrideCurrentVolumeMountMergeOption
func DefaultVolumeMountMergeFunction(current, newVolumeMount *corev1.VolumeMount) (*corev1.VolumeMount, error) {
	return OverrideCurrentVolumeMountMergeFunction(current, newVolumeMount)
}

// OverrideCurrentVolumeMountMergeFunction used when the existing corev1.VolumeMount new to be replace by the new one.
func OverrideCurrentVolumeMountMergeFunction(current, newVolumeMount *corev1.VolumeMount) (*corev1.VolumeMount, error) {
	return newVolumeMount.DeepCopy(), nil
}

// IgnoreNewVolumeMountMergeFunction used when the existing corev1.VolumeMount needs to be kept.
func IgnoreNewVolumeMountMergeFunction(current, newVolumeMount *corev1.VolumeMount) (*corev1.VolumeMount, error) {
	return current.DeepCopy(), nil
}

// ErrorOnMergeAttemptdVolumeMountMergeFunction used to avoid replacing an existing VolumeMount
func ErrorOnMergeAttemptdVolumeMountMergeFunction(current, newVolumeMount *corev1.VolumeMount) (*corev1.VolumeMount, error) {
	return nil, errMergeAttempted
}
