// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package controlplaneconfiguration

import (
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func init() {
	fmt.Println("Registering control plane configuration feature")
	if err := feature.Register(feature.ControlPlaneConfigurationIDType, buildControlPlaneConfigurationFeature); err != nil {
		fmt.Printf("Failed to register control plane configuration feature: %v\n", err)
		panic(err)
	}
	fmt.Println("Successfully registered control plane configuration feature")
}

func buildControlPlaneConfigurationFeature(options *feature.Options) feature.Feature {
	controlplaneFeat := &controlPlaneConfigurationFeature{
		logger: options.Logger,
	}
	return controlplaneFeat
}

type controlPlaneConfigurationFeature struct {
	enabled                bool
	owner                  metav1.Object
	logger                 logr.Logger
	provider               string
	defaultConfigMapName   string
	openshiftConfigMapName string
}

// ID returns the ID of the Feature
func (f *controlPlaneConfigurationFeature) ID() feature.IDType {
	return feature.ControlPlaneConfigurationIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *controlPlaneConfigurationFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	f.owner = dda
	f.defaultConfigMapName = defaultConfigMapName
	f.openshiftConfigMapName = openshiftConfigMapName
	controlPlaneConfiguration := dda.Spec.Features.ControlPlaneConfiguration
	f.logger.Info("Control plane configuration feature state",
		"feature", feature.ControlPlaneConfigurationIDType,
		"enabled", controlPlaneConfiguration != nil && apiutils.BoolValue(controlPlaneConfiguration.Enabled),
		"config", controlPlaneConfiguration)

	if controlPlaneConfiguration != nil && apiutils.BoolValue(controlPlaneConfiguration.Enabled) {
		f.enabled = true
		f.logger.V(1).Info("Control plane configuration feature enabled",
			"feature", feature.ControlPlaneConfigurationIDType,
			"requiredComponents", reqComp)
		reqComp.ClusterAgent.IsRequired = apiutils.NewBoolPointer(true)
		reqComp.ClusterAgent.Containers = []apicommon.AgentContainerName{apicommon.ClusterAgentContainerName}
		f.logger.V(1).Info("Control plane configuration feature requirements set",
			"feature", feature.ControlPlaneConfigurationIDType,
			"requiredComponents", reqComp)
	}
	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *controlPlaneConfigurationFeature) ManageDependencies(managers feature.ResourceManagers) error {
	if !f.enabled {
		return nil
	}

	// Create configmaps for control plane configuration
	// default configmap
	defaultConfigMap, err := f.buildControlPlaneConfigurationConfigMap(kubernetes.DefaultProvider, f.defaultConfigMapName)
	if err != nil {
		return fmt.Errorf("failed to build default configmap: %w", err)
	}
	defaultConfigMap.Name = f.defaultConfigMapName

	if err := managers.Store().AddOrUpdate(kubernetes.ConfigMapKind, defaultConfigMap); err != nil {
		return fmt.Errorf("failed to add default configmap to store: %w", err)
	}

	// openshift configmap
	openshiftConfigMap, err := f.buildControlPlaneConfigurationConfigMap(kubernetes.OpenshiftRHCOSType, f.openshiftConfigMapName)
	if err != nil {
		return fmt.Errorf("failed to build openshift configmap: %w", err)
	}
	openshiftConfigMap.Name = f.openshiftConfigMapName

	if err := managers.Store().AddOrUpdate(kubernetes.ConfigMapKind, openshiftConfigMap); err != nil {
		return fmt.Errorf("failed to add openshift configmap to store: %w", err)
	}

	// Add OpenShift-specific RBAC if provider is OpenShift RHCOS
	if f.provider == kubernetes.OpenshiftRHCOSType {
		// Create SecurityContextConstraints
		scc := getSecurityContextConstraints(f.owner.GetName())

		if err := managers.Store().AddOrUpdate(securityContextConstraintsKind, scc); err != nil {
			return fmt.Errorf("failed to add SecurityContextConstraints to store: %w", err)
		}

		// Create RoleBinding for the SCC
		roleBinding := getRoleBinding(securityContextConstraintsName, f.owner.GetName(), f.owner.GetNamespace())

		if err := managers.Store().AddOrUpdate(kubernetes.RoleBindingKind, roleBinding); err != nil {
			return fmt.Errorf("failed to add RoleBinding to store: %w", err)
		}
	}

	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
func (f *controlPlaneConfigurationFeature) ManageClusterAgent(managers feature.PodTemplateManagers, provider string) error {
	f.provider = provider
	fmt.Println("controlplaneconfiguration feature")
	_, providerValue := kubernetes.GetProviderLabelKeyValue(provider)
	fmt.Println("manageclusteragent providerValue", providerValue)

	// Add the writable emptyDir volume for all providers
	agentConfDVolume := &corev1.Volume{
		Name: emptyDirVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	managers.Volume().AddVolume(agentConfDVolume)

	// Add volume mount to cluster-agent container
	agentConfDVolumeMount := corev1.VolumeMount{
		Name:      emptyDirVolumeName,
		MountPath: controlPlaneConfigurationVolumeMountPath,
		ReadOnly:  false,
	}
	managers.VolumeMount().AddVolumeMountToContainer(&agentConfDVolumeMount, apicommon.ClusterAgentContainerName)

	// Select the appropriate configmap based on provider
	var configMapName string
	if providerValue == kubernetes.OpenshiftRHCOSType {
		fmt.Print("openshift, adding configmaps")
		configMapName = f.openshiftConfigMapName
	} else if providerValue == kubernetes.EKSAMIType {
		fmt.Print("eks provider detected")
		configMapName = f.defaultConfigMapName // TODO: add eks configmap and update here
	} else {
		configMapName = f.defaultConfigMapName
	}

	// Add the controlplane configuration configmap volume
	configMapVolume := &corev1.Volume{
		Name: controlPlaneConfigurationVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configMapName,
				},
			},
		},
	}
	managers.Volume().AddVolume(configMapVolume)

	// Add volume mount for the configmap
	configMapVolumeMount := corev1.VolumeMount{
		Name:      controlPlaneConfigurationVolumeName,
		MountPath: controlPlaneConfigurationVolumeMountPath,
		ReadOnly:  true,
	}
	managers.VolumeMount().AddVolumeMountToContainer(&configMapVolumeMount, apicommon.ClusterAgentContainerName)

	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *controlPlaneConfigurationFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *controlPlaneConfigurationFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	f.provider = provider
	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
func (f *controlPlaneConfigurationFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	// Create volume for etcd client certs
	etcdCertsVolume := &corev1.Volume{
		Name: etcdCertsVolumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: etcdCertsSecretName,
			},
		},
	}
	managers.Volume().AddVolume(etcdCertsVolume)

	// Add volume mount to cluster-checks-runner container
	etcdCertsVolumeMount := corev1.VolumeMount{
		Name:      etcdCertsVolumeName,
		MountPath: etcdCertsVolumeMountPath,
		ReadOnly:  true,
	}
	managers.VolumeMount().AddVolumeMountToContainer(&etcdCertsVolumeMount, apicommon.ClusterChecksRunnersContainerName)

	return nil
}
