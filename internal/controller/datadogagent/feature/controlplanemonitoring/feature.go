// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package controlplanemonitoring

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
	if err := feature.Register(feature.ControlPlaneMonitoringIDType, buildControlPlaneMonitoringFeature); err != nil {
		panic(err)
	}
}

func buildControlPlaneMonitoringFeature(options *feature.Options) feature.Feature {
	controlplaneFeat := &controlPlaneMonitoringFeature{
		logger: options.Logger,
	}
	return controlplaneFeat
}

type controlPlaneMonitoringFeature struct {
	enabled                bool
	owner                  metav1.Object
	logger                 logr.Logger
	provider               string
	defaultConfigMapName   string
	openshiftConfigMapName string
	eksConfigMapName       string
}

// ID returns the ID of the Feature
func (f *controlPlaneMonitoringFeature) ID() feature.IDType {
	return feature.ControlPlaneMonitoringIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *controlPlaneMonitoringFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) (reqComp feature.RequiredComponents) {
	f.owner = dda
	f.defaultConfigMapName = defaultConfigMapName
	f.openshiftConfigMapName = openshiftConfigMapName
	f.eksConfigMapName = eksConfigMapName

	controlPlaneMonitoring := ddaSpec.Features.ControlPlaneMonitoring

	if controlPlaneMonitoring != nil && apiutils.BoolValue(controlPlaneMonitoring.Enabled) {
		f.enabled = true
		reqComp.ClusterAgent.IsRequired = apiutils.NewBoolPointer(true)
		reqComp.ClusterAgent.Containers = []apicommon.AgentContainerName{apicommon.ClusterAgentContainerName}
	}
	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *controlPlaneMonitoringFeature) ManageDependencies(managers feature.ResourceManagers, provider string) error {
	if !f.enabled {
		return nil
	}
	// Create ConfigMaps for control plane monitoring
	providerLabel, _ := kubernetes.GetProviderLabelKeyValue(provider)
	if providerLabel == kubernetes.OpenShiftProviderLabel {
		// OpenShift ConfigMap
		openshiftConfigMap, err2 := f.buildControlPlaneMonitoringConfigMap(kubernetes.OpenShiftProviderLabel, f.openshiftConfigMapName)
		if err2 != nil {
			return fmt.Errorf("failed to build openshift configmap: %w", err2)
		}
		openshiftConfigMap.Name = f.openshiftConfigMapName

		if err := managers.Store().AddOrUpdate(kubernetes.ConfigMapKind, openshiftConfigMap); err != nil {
			return fmt.Errorf("failed to add openshift configmap to store: %w", err)
		}

		// For OpenShift, etcd monitoring requires manual secret copying
		targetNamespace := f.owner.GetNamespace()
		copyCommand := fmt.Sprintf("oc get secret etcd-client -n openshift-etcd-operator -o yaml | sed 's/name: etcd-client/name: etcd-client-cert/' | sed 's/namespace: openshift-etcd-operator/namespace: %s/' | oc apply -f -", targetNamespace)

		f.logger.Info("OpenShift control plane monitoring requires manual etcd secret copy",
			"command", copyCommand,
			"note", "Run this command if cluster-agent pods fail to start due to missing etcd-client-cert secret")
	} else if providerLabel == kubernetes.EKSProviderLabel {
		// EKS ConfigMap
		eksConfigMap, err2 := f.buildControlPlaneMonitoringConfigMap(kubernetes.EKSProviderLabel, f.eksConfigMapName)
		if err2 != nil {
			return fmt.Errorf("failed to build eks configmap: %w", err2)
		}
		eksConfigMap.Name = f.eksConfigMapName

		if err := managers.Store().AddOrUpdate(kubernetes.ConfigMapKind, eksConfigMap); err != nil {
			return fmt.Errorf("failed to add eks configmap to store: %w", err)
		}
	}

	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
func (f *controlPlaneMonitoringFeature) ManageClusterAgent(managers feature.PodTemplateManagers, provider string) error {
	f.provider = provider
	providerLabel, _ := kubernetes.GetProviderLabelKeyValue(provider)

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
		MountPath: controlPlaneMonitoringVolumeMountPath,
		ReadOnly:  false,
	}
	managers.VolumeMount().AddVolumeMountToContainer(&agentConfDVolumeMount, apicommon.ClusterAgentContainerName)

	// Select the appropriate configmap based on provider
	var configMapName string
	if providerLabel == kubernetes.OpenShiftProviderLabel {
		configMapName = f.openshiftConfigMapName

	} else if providerLabel == kubernetes.EKSProviderLabel {
		configMapName = f.eksConfigMapName
	} else {
		configMapName = f.defaultConfigMapName
		return nil
	}
	// Add the controlplane configuration configmap volume
	configMapVolume := &corev1.Volume{
		Name: controlPlaneMonitoringVolumeName,
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
		Name:      controlPlaneMonitoringVolumeName,
		MountPath: controlPlaneMonitoringVolumeMountPath,
		ReadOnly:  true,
	}
	managers.VolumeMount().AddVolumeMountToContainer(&configMapVolumeMount, apicommon.ClusterAgentContainerName)

	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *controlPlaneMonitoringFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *controlPlaneMonitoringFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	f.provider = provider
	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
func (f *controlPlaneMonitoringFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers, provider string) error {
	providerLabel, _ := kubernetes.GetProviderLabelKeyValue(provider)
	if providerLabel == kubernetes.OpenShiftProviderLabel {
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
	}
	return nil
}
