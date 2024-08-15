// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package feature

import (
	"github.com/DataDog/datadog-operator/api/datadoghq/common/v1"
	apicommonv1 "github.com/DataDog/datadog-operator/api/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/dependencies"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/merger"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
)

// RequiredComponents use to know which component need to be enabled for the feature
type RequiredComponents struct {
	ClusterAgent        RequiredComponent
	Agent               RequiredComponent
	ClusterChecksRunner RequiredComponent
}

// IsEnabled returns true if any of the components need to be enabled
func (rc *RequiredComponents) IsEnabled() bool {
	return rc.ClusterAgent.IsEnabled() || rc.Agent.IsEnabled() || rc.ClusterChecksRunner.IsEnabled()
}

// IsConfigured returns true if any of the components need to be configured even if not enabled
func (rc *RequiredComponents) IsConfigured() bool {
	return rc.ClusterAgent.IsConfigured() || rc.Agent.IsConfigured() || rc.ClusterChecksRunner.IsConfigured()
}

// Merge use to merge 2 RequiredComponents
// merge priority: false > true > nil
// *
func (rc *RequiredComponents) Merge(in *RequiredComponents) *RequiredComponents {
	rc.ClusterAgent.Merge(&in.ClusterAgent)
	rc.Agent.Merge(&in.Agent)
	rc.ClusterChecksRunner.Merge(&in.ClusterChecksRunner)
	return rc
}

// RequiredComponent is used to know if a component is required and which containers are required.
// If isRequired is set to:
//   - nil: the feature doesn't need the corresponding component.
//   - true: the feature needs the corresponding component.
//   - false: the feature does not need the corresponding component, but if it runs the feature needs to be configured (e.g. explicitly disabled).
type RequiredComponent struct {
	IsRequired *bool
	Containers []apicommonv1.AgentContainerName
}

// IsEnabled returns true if the Feature needs the current RequiredComponent
func (rc *RequiredComponent) IsEnabled() bool {
	return apiutils.BoolValue(rc.IsRequired) || len(rc.Containers) > 0
}

// IsConfigured returns true if the Feature does not require the RequiredComponent, but if it runs it needs to be configured appropriately.
func (rc *RequiredComponent) IsConfigured() bool {
	return rc.IsRequired != nil || len(rc.Containers) > 0
}

// IsPrivileged checks whether component requires privileged access.
func (rc *RequiredComponent) IsPrivileged() bool {
	for _, container := range rc.Containers {
		if container == common.SecurityAgentContainerName || container == common.SystemProbeContainerName {
			return true
		}
	}
	return false
}

func (rc *RequiredComponent) SingleContainerStrategyEnabled() bool {
	return len(rc.Containers) == 1 &&
		rc.Containers[0] == apicommonv1.UnprivilegedSingleAgentContainerName
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
	if a == nil && b == nil {
		return nil
	} else if a == nil && b != nil {
		return b
	} else if b == nil && a != nil {
		return a
	}
	if !apiutils.BoolValue(a) || !apiutils.BoolValue(b) {
		return apiutils.NewBoolPointer(false)
	}
	return apiutils.NewBoolPointer(true)
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

// Feature interface
// It returns `true` if the Feature is used, else it return `false`.
type Feature interface {
	// ID returns the ID of the Feature
	ID() IDType
	// Configure use to configure the internal of a Feature
	// It should return `true` if the feature is enabled, else `false`.
	Configure(dda *v2alpha1.DatadogAgent) RequiredComponents
	// ManageDependencies allows a feature to manage its dependencies.
	// Feature's dependencies should be added in the store.
	ManageDependencies(managers ResourceManagers, components RequiredComponents) error
	// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
	// It should do nothing if the feature doesn't need to configure it.
	ManageClusterAgent(managers PodTemplateManagers) error
	// ManageNodeAget allows a feature to configure the Node Agent's corev1.PodTemplateSpec
	// It should do nothing if the feature doesn't need to configure it.
	ManageNodeAgent(managers PodTemplateManagers, provider string) error
	// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
	// if SingleContainerStrategy is enabled and can be used with the configured feature set.
	// It should do nothing if the feature doesn't need to configure it.
	ManageSingleContainerNodeAgent(managers PodTemplateManagers, provider string) error
	// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunnerAgent's corev1.PodTemplateSpec
	// It should do nothing if the feature doesn't need to configure it.
	ManageClusterChecksRunner(managers PodTemplateManagers) error
}

// Options option that can be pass to the Interface.Configure function
type Options struct {
	SupportExtendedDaemonset bool

	Logger logr.Logger

	ProcessChecksInCoreAgentEnabled bool
	OtelAgentEnabled                bool
}

// BuildFunc function type used by each Feature during its factory registration.
// It returns the Feature interface.
type BuildFunc func(options *Options) Feature

// ResourceManagers used to access the different resources manager.
type ResourceManagers interface {
	Store() dependencies.StoreClient
	RBACManager() merger.RBACManager
	PodSecurityManager() merger.PodSecurityManager
	SecretManager() merger.SecretManager
	NetworkPolicyManager() merger.NetworkPolicyManager
	ServiceManager() merger.ServiceManager
	CiliumPolicyManager() merger.CiliumPolicyManager
	ConfigMapManager() merger.ConfigMapManager
	APIServiceManager() merger.APIServiceManager
}

// NewResourceManagers return new instance of the ResourceManagers interface
func NewResourceManagers(store dependencies.StoreClient) ResourceManagers {
	return &resourceManagersImpl{
		store:         store,
		rbac:          merger.NewRBACManager(store),
		podSecurity:   merger.NewPodSecurityManager(store),
		secret:        merger.NewSecretManager(store),
		networkPolicy: merger.NewNetworkPolicyManager(store),
		service:       merger.NewServiceManager(store),
		cilium:        merger.NewCiliumPolicyManager(store),
		configMap:     merger.NewConfigMapManager(store),
		apiService:    merger.NewAPIServiceManager(store),
	}
}

type resourceManagersImpl struct {
	store         dependencies.StoreClient
	rbac          merger.RBACManager
	podSecurity   merger.PodSecurityManager
	secret        merger.SecretManager
	networkPolicy merger.NetworkPolicyManager
	service       merger.ServiceManager
	cilium        merger.CiliumPolicyManager
	configMap     merger.ConfigMapManager
	apiService    merger.APIServiceManager
}

func (impl *resourceManagersImpl) Store() dependencies.StoreClient {
	return impl.store
}

func (impl *resourceManagersImpl) RBACManager() merger.RBACManager {
	return impl.rbac
}

func (impl *resourceManagersImpl) PodSecurityManager() merger.PodSecurityManager {
	return impl.podSecurity
}

func (impl *resourceManagersImpl) SecretManager() merger.SecretManager {
	return impl.secret
}

func (impl *resourceManagersImpl) NetworkPolicyManager() merger.NetworkPolicyManager {
	return impl.networkPolicy
}

func (impl *resourceManagersImpl) ServiceManager() merger.ServiceManager {
	return impl.service
}

func (impl *resourceManagersImpl) CiliumPolicyManager() merger.CiliumPolicyManager {
	return impl.cilium
}

func (impl *resourceManagersImpl) ConfigMapManager() merger.ConfigMapManager {
	return impl.configMap
}

func (impl *resourceManagersImpl) APIServiceManager() merger.APIServiceManager {
	return impl.apiService
}

// PodTemplateManagers used to access the different PodTemplateSpec manager.
type PodTemplateManagers interface {
	// PodTemplateSpec used to access directly the PodTemplateSpec.
	PodTemplateSpec() *corev1.PodTemplateSpec
	// EnvVar used to access the EnvVarManager to manage the Environment variable defined in the PodTemplateSpec.
	EnvVar() merger.EnvVarManager
	// Volume used to access the VolumeManager to manage the Volume defined in the PodTemplateSpec.
	Volume() merger.VolumeManager
	// VolumeMount used to access the VolumeMountManager to manage the VolumeMount defined in the PodTemplateSpec.
	VolumeMount() merger.VolumeMountManager
	// SecurityContext is used to access the SecurityContextManager to manage container Security Context defined in the PodTemplateSpec.
	SecurityContext() merger.SecurityContextManager
	// Annotation is used access the AnnotationManager to manage PodTemplateSpec annotations.
	Annotation() merger.AnnotationManager
	// Ports used to access PortManager that allows to manage the Ports defined in the PodTemplateSpec.
	Port() merger.PortManager
}

// NewPodTemplateManagers use to create a new instance of PodTemplateManagers from
// a corev1.PodTemplateSpec argument
func NewPodTemplateManagers(podTmpl *corev1.PodTemplateSpec) PodTemplateManagers {
	return &podTemplateManagerImpl{
		podTmpl:                podTmpl,
		envVarManager:          merger.NewEnvVarManager(podTmpl),
		volumeManager:          merger.NewVolumeManager(podTmpl),
		volumeMountManager:     merger.NewVolumeMountManager(podTmpl),
		securityContextManager: merger.NewSecurityContextManager(podTmpl),
		annotationManager:      merger.NewAnnotationManager(podTmpl),
		portManager:            merger.NewPortManager(podTmpl),
	}
}

type podTemplateManagerImpl struct {
	podTmpl                *corev1.PodTemplateSpec
	envVarManager          merger.EnvVarManager
	volumeManager          merger.VolumeManager
	volumeMountManager     merger.VolumeMountManager
	securityContextManager merger.SecurityContextManager
	annotationManager      merger.AnnotationManager
	portManager            merger.PortManager
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

func (impl *podTemplateManagerImpl) VolumeMount() merger.VolumeMountManager {
	return impl.volumeMountManager
}

func (impl *podTemplateManagerImpl) SecurityContext() merger.SecurityContextManager {
	return impl.securityContextManager
}

func (impl *podTemplateManagerImpl) Annotation() merger.AnnotationManager {
	return impl.annotationManager
}

func (impl *podTemplateManagerImpl) Port() merger.PortManager {
	return impl.portManager
}
