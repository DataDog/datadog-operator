// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package controlplanemonitoring

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
		client: options.Client,
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
	client                 client.Reader
}

// ID returns the ID of the Feature
func (f *controlPlaneMonitoringFeature) ID() feature.IDType {
	return feature.ControlPlaneMonitoringIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *controlPlaneMonitoringFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) (reqComp feature.RequiredComponents) {
	f.owner = dda
	f.provider = dda.GetAnnotations()[kubernetes.ProviderAnnotationKey]
	f.defaultConfigMapName = defaultConfigMapName
	f.openshiftConfigMapName = openshiftConfigMapName
	f.eksConfigMapName = eksConfigMapName

	controlPlaneMonitoring := ddaSpec.Features.ControlPlaneMonitoring

	if controlPlaneMonitoring != nil && apiutils.BoolValue(controlPlaneMonitoring.Enabled) {
		f.enabled = true
		reqComp.ClusterAgent.IsRequired = ptr.To(true)
		reqComp.ClusterAgent.Containers = []apicommon.AgentContainerName{apicommon.ClusterAgentContainerName}
	}
	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *controlPlaneMonitoringFeature) ManageDependencies(managers feature.ResourceManagers) error {
	if !f.enabled {
		return nil
	}
	// Create ConfigMaps for control plane monitoring
	providerLabel, _ := kubernetes.GetProviderLabelKeyValue(f.provider)
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

		if copied := f.copyOpenShiftEtcdSecret(managers); !copied {
			targetNamespace := f.owner.GetNamespace()
			copyCommand := fmt.Sprintf("oc get secret %s -n %s -o yaml | sed 's/namespace: %s/namespace: %s/' | oc apply -f -", etcdCertsSecretName, etcdCertsSourceNamespace, etcdCertsSourceNamespace, targetNamespace)

			f.logger.Info("OpenShift control plane monitoring requires manual etcd secret copy",
				"command", copyCommand,
				"note", "Run this command if cluster-checks-runner and node-agent pods fail to start due to missing etcd-metric-cert secret")
		}
	} else if f.provider == kubernetes.EKSCloudProvider {
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

func (f *controlPlaneMonitoringFeature) copyOpenShiftEtcdSecret(managers feature.ResourceManagers) bool {
	if f.client == nil {
		f.logger.V(1).Info("Skipping OpenShift etcd metric client secret copy: Kubernetes reader is not configured")
		return false
	}

	// Read the source Secret directly from the API server instead of relying on
	// the controller cache: default operator installs do not watch the
	// openshift-etcd-operator namespace, and broadening the cache would keep
	// sensitive platform Secrets in memory just for this one copy.
	source := &corev1.Secret{}
	sourceKey := types.NamespacedName{
		Namespace: etcdCertsSourceNamespace,
		Name:      etcdCertsSecretName,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := f.client.Get(ctx, sourceKey, source); err != nil {
		f.logger.Info("Unable to copy OpenShift etcd metric client secret automatically", "namespace", sourceKey.Namespace, "name", sourceKey.Name, "error", err)
		return false
	}

	target := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      etcdCertsSecretName,
			Namespace: f.owner.GetNamespace(),
		},
		Type: source.Type,
		Data: maps.Clone(source.Data),
	}
	if err := managers.Store().AddOrUpdate(kubernetes.SecretsKind, target); err != nil {
		f.logger.Info("Unable to add copied OpenShift etcd metric client secret to dependency store", "namespace", target.Namespace, "name", target.Name, "error", err)
		return false
	}

	f.logger.V(1).Info("Copied OpenShift etcd metric client secret", "sourceNamespace", sourceKey.Namespace, "targetNamespace", target.Namespace, "name", target.Name)
	return true
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
func (f *controlPlaneMonitoringFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	providerLabel, _ := kubernetes.GetProviderLabelKeyValue(f.provider)

	// Select the appropriate configmap based on provider
	var configMapName string
	if providerLabel == kubernetes.OpenShiftProviderLabel {
		configMapName = f.openshiftConfigMapName
	} else if f.provider == kubernetes.EKSCloudProvider {
		configMapName = f.eksConfigMapName
	} else {
		return nil
	}

	// Mount checks from configmap to subdirectories
	kubeApiserverVolume := &corev1.Volume{
		Name: kubeApiserverMetricsVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configMapName,
				},
				Items: []corev1.KeyToPath{
					{
						Key:  "kube_apiserver_metrics.yaml",
						Path: "kube_apiserver_metrics.yaml",
					},
				},
			},
		},
	}
	managers.Volume().AddVolume(kubeApiserverVolume)

	kubeApiserverVolumeMount := corev1.VolumeMount{
		Name:      kubeApiserverMetricsVolumeName,
		MountPath: kubeApiserverMetricsMountPath,
		ReadOnly:  true,
	}
	managers.VolumeMount().AddVolumeMountToContainer(&kubeApiserverVolumeMount, apicommon.ClusterAgentContainerName)

	kubeControllerManagerVolume := &corev1.Volume{
		Name: kubeControllerManagerVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configMapName,
				},
				Items: []corev1.KeyToPath{
					{
						Key:  "kube_controller_manager.yaml",
						Path: "kube_controller_manager.yaml",
					},
				},
			},
		},
	}
	managers.Volume().AddVolume(kubeControllerManagerVolume)

	kubeControllerManagerVolumeMount := corev1.VolumeMount{
		Name:      kubeControllerManagerVolumeName,
		MountPath: kubeControllerManagerMountPath,
		ReadOnly:  true,
	}
	managers.VolumeMount().AddVolumeMountToContainer(&kubeControllerManagerVolumeMount, apicommon.ClusterAgentContainerName)

	kubeSchedulerVolume := &corev1.Volume{
		Name: kubeSchedulerVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configMapName,
				},
				Items: []corev1.KeyToPath{
					{
						Key:  "kube_scheduler.yaml",
						Path: "kube_scheduler.yaml",
					},
				},
			},
		},
	}
	managers.Volume().AddVolume(kubeSchedulerVolume)

	kubeSchedulerVolumeMount := corev1.VolumeMount{
		Name:      kubeSchedulerVolumeName,
		MountPath: kubeSchedulerMountPath,
		ReadOnly:  true,
	}
	managers.VolumeMount().AddVolumeMountToContainer(&kubeSchedulerVolumeMount, apicommon.ClusterAgentContainerName)

	if providerLabel == kubernetes.OpenShiftProviderLabel {
		etcdVolume := &corev1.Volume{
			Name: etcdVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configMapName,
					},
					Items: []corev1.KeyToPath{
						{
							Key:  "etcd.yaml",
							Path: "etcd.yaml",
						},
					},
				},
			},
		}
		managers.Volume().AddVolume(etcdVolume)

		etcdVolumeMount := corev1.VolumeMount{
			Name:      etcdVolumeName,
			MountPath: etcdMountPath,
			ReadOnly:  true,
		}
		managers.VolumeMount().AddVolumeMountToContainer(&etcdVolumeMount, apicommon.ClusterAgentContainerName)
	}

	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *controlPlaneMonitoringFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *controlPlaneMonitoringFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	providerLabel, _ := kubernetes.GetProviderLabelKeyValue(f.provider)
	if providerLabel == kubernetes.OpenShiftProviderLabel {
		// Add etcd-certs volume (secret)
		etcdCertsVolume := &corev1.Volume{
			Name: etcdCertsVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName:  etcdCertsSecretName,
					DefaultMode: ptr.To[int32](420),
				},
			},
		}
		managers.Volume().AddVolume(etcdCertsVolume)

		// Add etcd-certs volume mount
		etcdCertsVolumeMount := corev1.VolumeMount{
			Name:      etcdCertsVolumeName,
			MountPath: etcdCertsVolumeMountPath,
			ReadOnly:  true,
		}
		managers.VolumeMount().AddVolumeMountToContainer(&etcdCertsVolumeMount, apicommon.CoreAgentContainerName)

		// Add disable-etcd-autoconf volume (emptyDir)
		disableEtcdAutoconfVolume := &corev1.Volume{
			Name: disableEtcdAutoconfVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		}
		managers.Volume().AddVolume(disableEtcdAutoconfVolume)

		// Add disable-etcd-autoconf volume mount
		disableEtcdAutoconfVolumeMount := corev1.VolumeMount{
			Name:      disableEtcdAutoconfVolumeName,
			MountPath: disableEtcdAutoconfVolumeMountPath,
			ReadOnly:  false,
		}
		managers.VolumeMount().AddVolumeMountToContainer(&disableEtcdAutoconfVolumeMount, apicommon.CoreAgentContainerName)
	}
	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
func (f *controlPlaneMonitoringFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	providerLabel, _ := kubernetes.GetProviderLabelKeyValue(f.provider)
	if providerLabel == kubernetes.OpenShiftProviderLabel {
		// Add etcd-certs volume (secret)
		etcdCertsVolume := &corev1.Volume{
			Name: etcdCertsVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: etcdCertsSecretName,
				},
			},
		}
		managers.Volume().AddVolume(etcdCertsVolume)

		// Add etcd-certs volume mount
		etcdCertsVolumeMount := corev1.VolumeMount{
			Name:      etcdCertsVolumeName,
			MountPath: etcdCertsVolumeMountPath,
			ReadOnly:  true,
		}
		managers.VolumeMount().AddVolumeMountToContainer(&etcdCertsVolumeMount, apicommon.ClusterChecksRunnersContainerName)

		// Add disable-etcd-autoconf volume (emptyDir)
		disableEtcdAutoconfVolume := &corev1.Volume{
			Name: disableEtcdAutoconfVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		}
		managers.Volume().AddVolume(disableEtcdAutoconfVolume)

		// Add disable-etcd-autoconf volume mount
		disableEtcdAutoconfVolumeMount := corev1.VolumeMount{
			Name:      disableEtcdAutoconfVolumeName,
			MountPath: disableEtcdAutoconfVolumeMountPath,
			ReadOnly:  false,
		}
		managers.VolumeMount().AddVolumeMountToContainer(&disableEtcdAutoconfVolumeMount, apicommon.ClusterChecksRunnersContainerName)
	}
	return nil
}

func (f *controlPlaneMonitoringFeature) ManageOtelAgentGateway(managers feature.PodTemplateManagers) error {
	return nil
}
