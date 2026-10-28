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
	talosConfigMapName     string
	client                 client.Reader

	etcdSecretPresent bool

	// Whether the node agent tolerates the control-plane taint. If true the etcd
	// check is configured and collects etcd metrics; otherwise etcd is disabled.
	talosEtcdEnabled bool
}

// nodeAgentToleratesControlPlane reports whether the node agent's configured
// tolerations tolerate the control-plane taint.
//
// Reads the spec, not the pod template: features run before overrides are
// applied, so the user's tolerations are not in the template yet. Matches with
// ToleratesTaint, which covers Exists, Equal and the empty-key form.
func nodeAgentToleratesControlPlane(logger logr.Logger, ddaSpec *v2alpha1.DatadogAgentSpec) bool {
	override, ok := ddaSpec.Override[v2alpha1.NodeAgentComponentName]
	if !ok || override == nil {
		return false
	}
	taint := corev1.Taint{
		Key:    controlPlaneTaintKey,
		Effect: corev1.TaintEffectNoSchedule,
	}
	for i := range override.Tolerations {
		if override.Tolerations[i].ToleratesTaint(logger, &taint, false) {
			return true
		}
	}
	return false
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
	f.talosConfigMapName = talosConfigMapName

	controlPlaneMonitoring := ddaSpec.Features.ControlPlaneMonitoring

	if controlPlaneMonitoring != nil && apiutils.BoolValue(controlPlaneMonitoring.Enabled) {
		f.enabled = true
		reqComp.ClusterAgent.IsRequired = new(true)
		reqComp.ClusterAgent.Containers = []apicommon.AgentContainerName{apicommon.ClusterAgentContainerName}

		if f.provider == kubernetes.TalosProvider {
			f.talosEtcdEnabled = nodeAgentToleratesControlPlane(f.logger, ddaSpec)
			if f.talosEtcdEnabled {
				// Only the etcd check runs on the node agent.
				reqComp.Agent = feature.RequiredComponent{
					IsRequired: new(true),
					Containers: []apicommon.AgentContainerName{apicommon.CoreAgentContainerName},
				}
			}
		}
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
	} else if f.provider == kubernetes.TalosProvider {
		// Talos ConfigMap
		talosConfigMap, err2 := f.buildControlPlaneMonitoringConfigMap(kubernetes.TalosProvider, f.talosConfigMapName)
		if err2 != nil {
			return fmt.Errorf("failed to build talos configmap: %w", err2)
		}
		talosConfigMap.Name = f.talosConfigMapName

		if err := managers.Store().AddOrUpdate(kubernetes.ConfigMapKind, talosConfigMap); err != nil {
			return fmt.Errorf("failed to add talos configmap to store: %w", err)
		}

		if !f.talosEtcdEnabled {
			f.logger.V(1).Info("Talos control plane monitoring is enabled without etcd",
				"working", "kube_apiserver_metrics, kube_scheduler, kube_controller_manager",
				"notCollected", "etcd",
				"reason", fmt.Sprintf("node agent does not tolerate the %q taint, and etcd's client certs exist only on control-plane nodes", controlPlaneTaintKey),
				"toEnable", fmt.Sprintf("add a toleration for %q (effect NoSchedule) under spec.override.nodeAgent.tolerations", controlPlaneTaintKey),
				"alsoRequired", "scheduler and controller-manager metrics additionally need bind-address 0.0.0.0 via a Talos machine config patch")
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
		return f.keepExistingOpenShiftEtcdSecret(managers)
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
	f.etcdSecretPresent = true
	return true
}

func (f *controlPlaneMonitoringFeature) keepExistingOpenShiftEtcdSecret(managers feature.ResourceManagers) bool {
	if f.client == nil {
		f.logger.V(1).Info("Skipping existing OpenShift etcd metric client secret lookup: Kubernetes reader is not configured")
		return false
	}

	target := &corev1.Secret{}
	targetKey := types.NamespacedName{
		Namespace: f.owner.GetNamespace(),
		Name:      etcdCertsSecretName,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := f.client.Get(ctx, targetKey, target); err != nil {
		f.logger.Info("Unable to keep existing OpenShift etcd metric client secret", "namespace", targetKey.Namespace, "name", targetKey.Name, "error", err)
		return false
	}

	if err := managers.Store().AddOrUpdate(kubernetes.SecretsKind, target); err != nil {
		f.logger.Info("Unable to add existing OpenShift etcd metric client secret to dependency store", "namespace", target.Namespace, "name", target.Name, "error", err)
		return false
	}

	f.logger.V(1).Info("Keeping existing OpenShift etcd metric client secret after source read failure", "namespace", target.Namespace, "name", target.Name)
	f.etcdSecretPresent = true
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
	} else if f.provider == kubernetes.TalosProvider {
		// apiserver, scheduler and controller-manager only; etcd mounts into the
		// node agent instead (ManageNodeAgent).
		configMapName = f.talosConfigMapName
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

// etcdCertsSecretAvailable reports whether ManageDependencies successfully added
// the OpenShift etcd metric client secret to the dependency store for the owner
// namespace. The etcd-certs volume reference is non-optional, so mounting it
// when the secret will not be managed would wedge the pod in ContainerCreating.
func (f *controlPlaneMonitoringFeature) etcdCertsSecretAvailable() bool {
	return f.etcdSecretPresent
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *controlPlaneMonitoringFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	providerLabel, _ := kubernetes.GetProviderLabelKeyValue(f.provider)

	if f.provider == kubernetes.TalosProvider {
		// The Agent image ships node-local autoconf for these three, which fires
		// when the node agent runs on a control-plane node and double-collects
		// alongside the cluster checks. Mask it so the cluster check is the only
		// source.
		// Ordered, not a map: map iteration order is randomized and would make the
		// rendered pod spec differ between reconciles.
		for _, m := range []struct{ name, mountPath string }{
			{disableKubeApiserverMetricsAutoconfVolumeName, kubeApiserverMetricsMountPath},
			{disableKubeControllerManagerAutoconfVolumeName, kubeControllerManagerMountPath},
			{disableKubeSchedulerAutoconfVolumeName, kubeSchedulerMountPath},
		} {
			name, mountPath := m.name, m.mountPath
			vol := &corev1.Volume{
				Name:         name,
				VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
			}
			managers.Volume().AddVolume(vol)

			mount := corev1.VolumeMount{Name: name, MountPath: mountPath}
			managers.VolumeMount().AddVolumeMountToContainer(&mount, apicommon.CoreAgentContainerName)
		}

		if !f.talosEtcdEnabled {
			// No control-plane toleration, so the etcd certs are unreachable.
			// Leave the node agent untouched; do not add a toleration.
			return nil
		}

		etcdConfigVolume := &corev1.Volume{
			Name: etcdVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: f.talosConfigMapName,
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
		managers.Volume().AddVolume(etcdConfigVolume)

		// Mounting over conf.d/etcd.d also shadows the autoconf the image ships.
		etcdConfigVolumeMount := corev1.VolumeMount{
			Name:      etcdVolumeName,
			MountPath: etcdMountPath,
			ReadOnly:  true,
		}
		managers.VolumeMount().AddVolumeMountToContainer(&etcdConfigVolumeMount, apicommon.CoreAgentContainerName)

		etcdCertsVolume := &corev1.Volume{
			Name: talosEtcdCertsVolumeName,
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: talosEtcdCertsHostPath,
				},
			},
		}
		managers.Volume().AddVolume(etcdCertsVolume)

		etcdCertsVolumeMount := corev1.VolumeMount{
			Name:      talosEtcdCertsVolumeName,
			MountPath: talosEtcdCertsMountPath,
			ReadOnly:  true,
		}
		managers.VolumeMount().AddVolumeMountToContainer(&etcdCertsVolumeMount, apicommon.CoreAgentContainerName)

		return nil
	}

	if providerLabel == kubernetes.OpenShiftProviderLabel {
		// Only mount the etcd-certs secret when it is present; the volume is
		// non-optional and would otherwise wedge the pod in ContainerCreating.
		if f.etcdCertsSecretAvailable() {
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
		}

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
		// Only mount the etcd-certs secret when it is present; the volume is
		// non-optional and would otherwise wedge the pod in ContainerCreating.
		if f.etcdCertsSecretAvailable() {
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
		}

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
