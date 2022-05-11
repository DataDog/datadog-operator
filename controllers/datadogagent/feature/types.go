// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package feature

import (
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/dependencies"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/merger"

	"github.com/go-logr/logr"

	corev1 "k8s.io/api/core/v1"
)

// RequiredComponents use to know which component need to be enabled for the feature
type RequiredComponents struct {
	ClusterAgent       RequiredComponent
	Agent              RequiredComponent
	ClusterCheckRunner RequiredComponent
}

// IsEnabled return true if the Feature need to be enabled
func (rc *RequiredComponents) IsEnabled() bool {
	return rc.ClusterAgent.IsEnabled() || rc.Agent.IsEnabled() || rc.ClusterCheckRunner.IsEnabled()
}

// Merge use to merge 2 RequiredComponents
// merge priority: false > true > nil
// *
func (rc *RequiredComponents) Merge(in *RequiredComponents) *RequiredComponents {
	rc.ClusterAgent.Merge(&in.ClusterAgent)
	rc.Agent.Merge(&in.Agent)
	rc.ClusterCheckRunner.Merge(&in.ClusterCheckRunner)
	return rc
}

// RequiredComponent use to know how if a component is required and which containers are required.
// If set Required to:
//   * true: the feature needs the corresponding component.
//   * false: the corresponding component needs to ne disabled for this feature.
//   * nil: the feature doesn't need the corresponding component.
type RequiredComponent struct {
	IsRequired *bool
	Containers []apicommonv1.AgentContainerName
}

// IsEnabled return true if the Feature need the current RequiredComponent
func (rc *RequiredComponent) IsEnabled() bool {
	return apiutils.BoolValue(rc.IsRequired) || len(rc.Containers) > 0
}

// Merge use to merge 2 RequiredComponents
// merge priority: false > true > nil
// *
func (rc *RequiredComponent) Merge(in *RequiredComponent) *RequiredComponent {
	rc.IsRequired = merge(rc.IsRequired, in.IsRequired)
	rc.Containers = mergeSlices(rc.Containers, in.Containers)
	return rc
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

func mergeSlices(a, b []apicommonv1.AgentContainerName) []apicommonv1.AgentContainerName {
	out := a
	for _, containerB := range b {
		found := false
		for _, containerA := range a {
			if containerA == containerB {
				found = true
				break
			}
		}
		if !found {
			out = append(out, containerB)
		}
	}

	return out
}

// Feature Feature interface
// It returns `true` if the Feature is used, else it return `false`.
type Feature interface {
	// Configure use to configure the internal of a Feature
	// It should return `true` if the feature is enabled, else `false`.
	Configure(dda *v2alpha1.DatadogAgent) RequiredComponents
	// ConfigureV1 use to configure the internal of a Feature from v1alpha1.DatadogAgent
	// It should return `true` if the feature is enabled, else `false`.
	ConfigureV1(dda *v1alpha1.DatadogAgent) RequiredComponents
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
	// Ports used to access PortManager that allows to manage the Ports defined in the PodTemplateSpec.
	Port() merger.PortManager
}

// NewPodTemplateManagers use to create a new instance of PodTemplateManagers from
// a corev1.PodTemplateSpec argument
func NewPodTemplateManagers(podTmpl *corev1.PodTemplateSpec) PodTemplateManagers {
	return &podTemplateManagerImpl{
		podTmpl:       podTmpl,
		envVarManager: merger.NewEnvVarManager(podTmpl),
		volumeManager: merger.NewVolumeManager(podTmpl),
		portManager:   merger.NewPortManager(podTmpl),
	}
}

type podTemplateManagerImpl struct {
	podTmpl       *corev1.PodTemplateSpec
	envVarManager merger.EnvVarManager
	volumeManager merger.VolumeManager
	portManager   merger.PortManager
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

func (impl *podTemplateManagerImpl) Port() merger.PortManager {
	return impl.portManager
}
