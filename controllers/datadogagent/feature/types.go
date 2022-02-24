// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package feature

import (
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/dependencies"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/merger"

	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
)

// Feature Feature interface
// It returns `true` if the Feature is used, else it return `false`.
type Feature interface {
	// Configure use to configure the internal of a Feature
	// It should return `true` if the feature is enabled, else `false`.
	Configure(dda *v2alpha1.DatadogAgent) bool
	// ConfigureV1 use to configure the internal of a Feature from v1alpha1.DatadogAgent
	// It should return `true` if the feature is enabled, else `false`.
	ConfigureV1(dda *v1alpha1.DatadogAgent) bool
	// ManageDependencies allows a feature to manage its dependencies.
	// Feature's dependencies should be added in the store.
	ManageDependencies(managers ResourcesManagers) error
	// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
	// It should do nothing if the feature doesn't need to configure it.
	ManageClusterAgent(managers PodTemplateManagers) error
	// ManageNodeAget allows a feature to configure the Node Agent's corev1.PodTemplateSpec
	// It should do nothing if the feature doesn't need to configure it.
	ManageNodeAgent(managers PodTemplateManagers) error
	// ManageClusterCheckRunnerAgent allows a feature to configure the ClusterCheckRunnerAgent's corev1.PodTemplateSpec
	// It should do nothing if the feature doesn't need to configure it.
	ManageClusterCheckRunnerAgent(managers PodTemplateManagers) error
}

// Options option that can be pass to the Interface.Configure function
type Options struct {
	SupportExtendedDaemonset bool

	Logger logr.Logger
}

// BuildFunc function type used by each Feature during its factory registration.
// It returns the Feature interface.
type BuildFunc func(options *Options) Feature

// ResourcesManagers used to access the different resources manager.
type ResourcesManagers interface {
	Store() dependencies.StoreClient
	RBACManager() merger.RBACManager
}

// NewResourcesManagers return new instance of the ResourcesManagers interface
func NewResourcesManagers(store dependencies.StoreClient) ResourcesManagers {
	return &resourcesManagersImpl{
		store: store,
		rbac:  merger.NewRBACManager(store),
	}
}

type resourcesManagersImpl struct {
	store dependencies.StoreClient
	rbac  merger.RBACManager
}

func (impl *resourcesManagersImpl) Store() dependencies.StoreClient {
	return impl.store
}

func (impl *resourcesManagersImpl) RBACManager() merger.RBACManager {
	return impl.rbac
}

// PodTemplateManagers used to access the different PodTemplateSpec manager.
type PodTemplateManagers interface {
	// PodTemplateSpec used to access directly to the PodTemplateSpec.
	PodTemplateSpec() *corev1.PodTemplateSpec
	// EnvVar used to access EnvVarManager that allows to manage the Environment variable defined in the PodTemplateSpec.
	EnvVar() merger.EnvVarManager
	// EnvVar used to access VolumeManager that allows to manage the Volume and VolumeMount defined in the PodTemplateSpec.
	Volume() merger.VolumeManager
}

// NewPodTemplateManagers use to create a new instance of PodTemplateManagers from
// a corev1.PodTemplateSpec argument
func NewPodTemplateManagers(podTmpl *corev1.PodTemplateSpec) PodTemplateManagers {
	return &podTemplateManagerImpl{
		podTmpl:       podTmpl,
		envVarManager: merger.NewEnvVarManager(podTmpl),
		volumeManager: merger.NewVolumeManager(podTmpl),
	}
}

type podTemplateManagerImpl struct {
	podTmpl       *corev1.PodTemplateSpec
	envVarManager merger.EnvVarManager
	volumeManager merger.VolumeManager
}

func (impl *podTemplateManagerImpl) PodTemplateSpec() *corev1.PodTemplateSpec {
	return impl.podTmpl
}

func (impl *podTemplateManagerImpl) EnvVar() merger.EnvVarManager {
	return impl.envVarManager
}

func (impl *podTemplateManagerImpl) Volume() merger.VolumeManager {
	return impl.volumeManager
}
