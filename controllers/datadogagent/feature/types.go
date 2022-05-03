// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package feature

import (
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/dependencies"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/merger"

	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
)

// ComponentsEnabled use to know which component need to be enabled for the feature
// If set to:
//   * true: the feature needs the corresponding component.
//   * false: the corresponding component needs to ne disabled for this feature.
//   * nil: the feature doesn't need the corresponding component.
type ComponentsEnabled struct {
	ClusterAgent       *bool
	Agent              *bool
	ClusterCheckRunner *bool
}

// IsEnabled return true if the Feature need to be enabled
func (cc *ComponentsEnabled) IsEnabled() bool {
	return apiutils.BoolValue(cc.ClusterAgent) || apiutils.BoolValue(cc.Agent) || apiutils.BoolValue(cc.ClusterCheckRunner)
}

// Merge use to merge 2 ComponentsEnabled
// merge priority: false > true > nil
// *
func (cc *ComponentsEnabled) Merge(new *ComponentsEnabled) *ComponentsEnabled {
	cc.ClusterAgent = merge(cc.ClusterAgent, new.ClusterAgent)
	cc.Agent = merge(cc.Agent, new.Agent)
	cc.ClusterCheckRunner = merge(cc.ClusterCheckRunner, new.ClusterCheckRunner)
	return cc
}

func merge(a, b *bool) *bool {
	trueValue := true
	falseValue := false
	if a == nil && b == nil {
		return nil
	} else if a == nil && b != nil {
		return b
	} else if b == nil && a != nil {
		return a
	}
	if !apiutils.BoolValue(a) || !apiutils.BoolValue(b) {
		return &falseValue
	}
	return &trueValue
}

// Feature Feature interface
// It returns `true` if the Feature is used, else it return `false`.
type Feature interface {
	// Configure use to configure the internal of a Feature
	// It should return `true` if the feature is enabled, else `false`.
	Configure(dda *v2alpha1.DatadogAgent) ComponentsEnabled
	// ConfigureV1 use to configure the internal of a Feature from v1alpha1.DatadogAgent
	// It should return `true` if the feature is enabled, else `false`.
	ConfigureV1(dda *v1alpha1.DatadogAgent) ComponentsEnabled
	// ManageDependencies allows a feature to manage its dependencies.
	// Feature's dependencies should be added in the store.
	ManageDependencies(managers ResourceManagers) error
	// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
	// It should do nothing if the feature doesn't need to configure it.
	ManageClusterAgent(managers PodTemplateManagers) error
	// ManageNodeAget allows a feature to configure the Node Agent's corev1.PodTemplateSpec
	// It should do nothing if the feature doesn't need to configure it.
	ManageNodeAgent(managers PodTemplateManagers) error
	// ManageClusterChecksRunner allows a feature to configure the ClusterCheckRunnerAgent's corev1.PodTemplateSpec
	// It should do nothing if the feature doesn't need to configure it.
	ManageClusterChecksRunner(managers PodTemplateManagers) error
}

// Options option that can be pass to the Interface.Configure function
type Options struct {
	SupportExtendedDaemonset bool

	Logger logr.Logger
}

// BuildFunc function type used by each Feature during its factory registration.
// It returns the Feature interface.
type BuildFunc func(options *Options) Feature

// ResourceManagers used to access the different resources manager.
type ResourceManagers interface {
	Store() dependencies.StoreClient
	RBACManager() merger.RBACManager
}

// NewResourceManagers return new instance of the ResourceManagers interface
func NewResourceManagers(store dependencies.StoreClient) ResourceManagers {
	return &resourceManagersImpl{
		store: store,
		rbac:  merger.NewRBACManager(store),
	}
}

type resourceManagersImpl struct {
	store dependencies.StoreClient
	rbac  merger.RBACManager
}

func (impl *resourceManagersImpl) Store() dependencies.StoreClient {
	return impl.store
}

func (impl *resourceManagersImpl) RBACManager() merger.RBACManager {
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
