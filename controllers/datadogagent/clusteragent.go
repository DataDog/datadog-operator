// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagent

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func (r *Reconciler) reconcileClusterAgent(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	result, err := r.manageClusterAgentDependencies(logger, dda, newStatus)
	if shouldReturn(result, err) {
		return result, err
	}
	if dda.Spec.ClusterAgent == nil {
		result, err = r.cleanupClusterAgent(logger, dda, newStatus)
		return result, err
	}

	// Generate a Token for clusterAgent-Agent communication if not provided
	if dda.Spec.Credentials.Token == "" {
		if newStatus.ClusterAgent == nil {
			newStatus.ClusterAgent = &datadoghqv1alpha1.DeploymentStatus{}
		}
		if newStatus.ClusterAgent.GeneratedToken == "" {
			newStatus.ClusterAgent.GeneratedToken = generateRandomString(32)
			return reconcile.Result{}, nil
		}
	}

	if newStatus.ClusterAgent != nil &&
		newStatus.ClusterAgent.DeploymentName != "" &&
		newStatus.ClusterAgent.DeploymentName != getClusterAgentName(dda) {
		return result, fmt.Errorf("the Datadog cluster agent Deployment cannot be renamed once created")
	}

	nsName := types.NamespacedName{
		Name:      getClusterAgentName(dda),
		Namespace: dda.Namespace,
	}
	// ClusterAgentDeployment attached to this instance
	clusterAgentDeployment := &appsv1.Deployment{}
	if dda.Spec.ClusterAgent != nil {
		err := r.client.Get(context.TODO(), nsName, clusterAgentDeployment)
		if err != nil {
			if errors.IsNotFound(err) {
				logger.Info("the ClusterAgent deployment is not found", "name", nsName.Name, "namespace", nsName.Namespace)
				// Create and attach a ClusterAgentDeployment
				return r.createNewClusterAgentDeployment(logger, dda, newStatus)
			}
			return reconcile.Result{}, err
		}

		if result, err = r.updateClusterAgentDeployment(logger, dda, clusterAgentDeployment, newStatus); err != nil {
			return result, err
		}

		// Make sure we have at least one Cluster Agent available replica
		if clusterAgentDeployment.Status.AvailableReplicas == 0 {
			return reconcile.Result{RequeueAfter: defaultRequeuePeriod}, fmt.Errorf("cluster agent deployment is not ready yet: 0 pods available out of %d", clusterAgentDeployment.Status.Replicas)
		}
	}

	return reconcile.Result{}, nil
}

func (r *Reconciler) createNewClusterAgentDeployment(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	newDCA, hash, err := newClusterAgentDeploymentFromInstance(agentdeployment, nil)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Set DatadogAgent instance  instance as the owner and controller
	if err = controllerutil.SetControllerReference(agentdeployment, newDCA, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.Info("Creating a new Cluster Agent Deployment", "deployment.Namespace", newDCA.Namespace, "deployment.Name", newDCA.Name, "agentdeployment.Status.ClusterAgent.CurrentHash", hash)
	newStatus.ClusterAgent = &datadoghqv1alpha1.DeploymentStatus{}
	err = r.client.Create(context.TODO(), newDCA)
	now := metav1.NewTime(time.Now())
	if err != nil {
		updateStatusWithClusterAgent(nil, newStatus, &now)
		return reconcile.Result{}, err
	}

	updateStatusWithClusterAgent(newDCA, newStatus, &now)
	event := buildEventInfo(newDCA.Name, newDCA.Namespace, deploymentKind, datadog.CreationEvent)
	r.recordEvent(agentdeployment, event)
	return reconcile.Result{}, nil
}

func updateStatusWithClusterAgent(dca *appsv1.Deployment, newStatus *datadoghqv1alpha1.DatadogAgentStatus, updateTime *metav1.Time) {
	newStatus.ClusterAgent = updateDeploymentStatus(dca, newStatus.ClusterAgent, updateTime)
}

func (r *Reconciler) updateClusterAgentDeployment(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgent, dca *appsv1.Deployment, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	newDCA, hash, err := newClusterAgentDeploymentFromInstance(agentdeployment, dca.Spec.Selector)
	if err != nil {
		return reconcile.Result{}, err
	}

	var needUpdate bool
	if !comparison.IsSameSpecMD5Hash(hash, dca.GetAnnotations()) {
		needUpdate = true
	}

	updateStatusWithClusterAgent(dca, newStatus, nil)

	if !needUpdate {
		return reconcile.Result{}, nil
	}
	logger.Info("update ClusterAgent deployment", "name", dca.Name, "namespace", dca.Namespace)
	// Set DatadogAgent instance  instance as the owner and controller
	if err = controllerutil.SetControllerReference(agentdeployment, dca, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.Info("Updating an existing Cluster Agent Deployment", "deployment.Namespace", newDCA.Namespace, "deployment.Name", newDCA.Name, "currentHash", hash)

	// Copy possibly changed fields
	updateDca := dca.DeepCopy()
	updateDca.Spec = *newDCA.Spec.DeepCopy()
	for k, v := range newDCA.Annotations {
		updateDca.Annotations[k] = v
	}
	for k, v := range newDCA.Labels {
		updateDca.Labels[k] = v
	}

	now := metav1.NewTime(time.Now())
	err = r.client.Update(context.TODO(), updateDca)
	if err != nil {
		return reconcile.Result{}, err
	}
	event := buildEventInfo(updateDca.Name, updateDca.Namespace, deploymentKind, datadog.UpdateEvent)
	r.recordEvent(agentdeployment, event)
	updateStatusWithClusterAgent(updateDca, newStatus, &now)
	return reconcile.Result{}, nil
}

// newClusterAgentDeploymentFromInstance creates a Cluster Agent Deployment from a given DatadogAgent
func newClusterAgentDeploymentFromInstance(agentdeployment *datadoghqv1alpha1.DatadogAgent, selector *metav1.LabelSelector) (*appsv1.Deployment, string, error) {
	labels := map[string]string{
		datadoghqv1alpha1.AgentDeploymentNameLabelKey:      agentdeployment.Name,
		datadoghqv1alpha1.AgentDeploymentComponentLabelKey: datadoghqv1alpha1.DefaultClusterAgentResourceSuffix,
	}
	for key, val := range agentdeployment.Labels {
		labels[key] = val
	}
	for key, val := range getDefaultLabels(agentdeployment, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(agentdeployment)) {
		labels[key] = val
	}

	if selector != nil {
		for key, val := range selector.MatchLabels {
			labels[key] = val
		}
	} else {
		selector = &metav1.LabelSelector{
			MatchLabels: map[string]string{
				datadoghqv1alpha1.AgentDeploymentNameLabelKey:      agentdeployment.Name,
				datadoghqv1alpha1.AgentDeploymentComponentLabelKey: datadoghqv1alpha1.DefaultClusterAgentResourceSuffix,
			},
		}
	}

	annotations := map[string]string{}
	for key, val := range agentdeployment.Annotations {
		annotations[key] = val
	}

	dca := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        getClusterAgentName(agentdeployment),
			Namespace:   agentdeployment.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Template: newClusterAgentPodTemplate(agentdeployment, labels, annotations),
			Replicas: agentdeployment.Spec.ClusterAgent.Replicas,
			Selector: selector,
		},
	}
	hash, err := comparison.SetMD5GenerationAnnotation(&dca.ObjectMeta, dca.Spec)
	return dca, hash, err
}

func (r *Reconciler) manageClusterAgentDependencies(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	result, err := r.manageAgentSecret(logger, dda, newStatus)
	if shouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageConfigMap(logger, dda, getClusterAgentCustomConfigConfigMapName(dda), buildClusterAgentConfigurationConfigMap)
	if shouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageClusterAgentService(logger, dda)
	if shouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageMetricsServerService(logger, dda)
	if shouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageMetricsServerAPIService(logger, dda)
	if shouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageAdmissionControllerService(logger, dda)
	if shouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageClusterAgentPDB(logger, dda)
	if shouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageClusterAgentRBACs(logger, dda)
	if shouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageConfigMap(logger, dda, getInstallInfoConfigMapName(dda), buildInstallInfoConfigMap)
	if shouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageClusterAgentNetworkPolicy(logger, dda)
	if shouldReturn(result, err) {
		return result, err
	}

	return reconcile.Result{}, nil
}

func buildClusterAgentConfigurationConfigMap(dda *datadoghqv1alpha1.DatadogAgent) (*corev1.ConfigMap, error) {
	if dda.Spec.ClusterAgent == nil {
		return nil, nil
	}
	return buildConfigurationConfigMap(dda, dda.Spec.ClusterAgent.CustomConfig, getClusterAgentCustomConfigConfigMapName(dda), datadoghqv1alpha1.ClusterAgentCustomConfigVolumeSubPath)
}

func (r *Reconciler) cleanupClusterAgent(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	nsName := types.NamespacedName{
		Name:      getClusterAgentName(dda),
		Namespace: dda.Namespace,
	}
	// ClusterAgentDeployment attached to this instance
	clusterAgentDeployment := &appsv1.Deployment{}
	if err := r.client.Get(context.TODO(), nsName, clusterAgentDeployment); err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	logger.Info("Deleting Cluster Agent Deployment", "deployment.Namespace", clusterAgentDeployment.Namespace, "deployment.Name", clusterAgentDeployment.Name)
	event := buildEventInfo(clusterAgentDeployment.Name, clusterAgentDeployment.Namespace, clusterRoleBindingKind, datadog.DeletionEvent)
	r.recordEvent(dda, event)
	if err := r.client.Delete(context.TODO(), clusterAgentDeployment); err != nil {
		return reconcile.Result{}, err
	}
	newStatus.ClusterAgent = nil
	return reconcile.Result{Requeue: true}, nil
}

// newClusterAgentPodTemplate generates a PodTemplate from a DatadogClusterAgentDeployment spec
func newClusterAgentPodTemplate(agentdeployment *datadoghqv1alpha1.DatadogAgent, labels, annotations map[string]string) corev1.PodTemplateSpec {
	// copy Spec to configure the Cluster Agent Pod Template
	clusterAgentSpec := agentdeployment.Spec.ClusterAgent.DeepCopy()

	// confd volumes configuration
	confdVolumeSource := corev1.VolumeSource{
		EmptyDir: &corev1.EmptyDirVolumeSource{},
	}
	if agentdeployment.Spec.ClusterAgent.Config.Confd != nil {
		confdVolumeSource = corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: agentdeployment.Spec.ClusterAgent.Config.Confd.ConfigMapName,
				},
			},
		}
	}
	volumes := []corev1.Volume{
		{
			Name: datadoghqv1alpha1.InstallInfoVolumeName,
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: getInstallInfoConfigMapName(agentdeployment),
					},
				},
			},
		},
		{
			Name:         datadoghqv1alpha1.ConfdVolumeName,
			VolumeSource: confdVolumeSource,
		},
	}
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "installinfo",
			SubPath:   "install_info",
			MountPath: "/etc/datadog-agent/install_info",
			ReadOnly:  true,
		},
		{
			Name:      datadoghqv1alpha1.ConfdVolumeName,
			MountPath: datadoghqv1alpha1.ConfdVolumePath,
			ReadOnly:  true,
		},
	}

	if agentdeployment.Spec.ClusterAgent.CustomConfig != nil {
		customConfigVolumeSource := getVolumeFromCustomConfigSpec(
			agentdeployment.Spec.ClusterAgent.CustomConfig,
			getClusterAgentCustomConfigConfigMapName(agentdeployment),
			datadoghqv1alpha1.AgentCustomConfigVolumeName)
		volumes = append(volumes, customConfigVolumeSource)

		// Custom config (datadog-cluster.yaml) volume
		volumeMount := getVolumeMountFromCustomConfigSpec(
			agentdeployment.Spec.ClusterAgent.CustomConfig,
			datadoghqv1alpha1.ClusterAgentCustomConfigVolumeName,
			datadoghqv1alpha1.ClusterAgentCustomConfigVolumePath,
			datadoghqv1alpha1.ClusterAgentCustomConfigVolumeSubPath)
		volumeMounts = append(volumeMounts, volumeMount)
	}

	if isComplianceEnabled(agentdeployment) {
		if agentdeployment.Spec.Agent.Security.Compliance.ConfigDir != nil {
			volumes = append(volumes, corev1.Volume{
				Name: datadoghqv1alpha1.SecurityAgentComplianceConfigDirVolumeName,
				VolumeSource: corev1.VolumeSource{
					ConfigMap: &corev1.ConfigMapVolumeSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: agentdeployment.Spec.Agent.Security.Compliance.ConfigDir.ConfigMapName,
						},
					},
				},
			})
			volumeMounts = append(volumeMounts, corev1.VolumeMount{
				Name:      datadoghqv1alpha1.SecurityAgentComplianceConfigDirVolumeName,
				MountPath: datadoghqv1alpha1.SecurityAgentComplianceConfigDirVolumePath,
				ReadOnly:  true,
			})
		}
	}

	// Add other volumes
	volumes = append(volumes, agentdeployment.Spec.ClusterAgent.Config.Volumes...)
	volumeMounts = append(volumeMounts, agentdeployment.Spec.ClusterAgent.Config.VolumeMounts...)

	podSpec := corev1.PodSpec{
		ServiceAccountName: getClusterAgentServiceAccount(agentdeployment),
		Containers: []corev1.Container{
			{
				Name:            "cluster-agent",
				Image:           clusterAgentSpec.Image.Name,
				ImagePullPolicy: *clusterAgentSpec.Image.PullPolicy,
				Ports: []corev1.ContainerPort{
					{
						ContainerPort: 5005,
						Name:          "agentport",
						Protocol:      "TCP",
					},
				},
				Env:          getEnvVarsForClusterAgent(agentdeployment),
				VolumeMounts: volumeMounts,
			},
		},
		Affinity:          clusterAgentSpec.Affinity,
		Tolerations:       clusterAgentSpec.Tolerations,
		PriorityClassName: agentdeployment.Spec.ClusterAgent.PriorityClassName,
		Volumes:           volumes,
	}

	newPodTemplate := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Spec: podSpec,
	}

	for key, val := range labels {
		newPodTemplate.Labels[key] = val
	}

	for key, val := range annotations {
		newPodTemplate.Annotations[key] = val
	}

	for key, val := range agentdeployment.Spec.ClusterAgent.AdditionalLabels {
		newPodTemplate.Labels[key] = val
	}

	for key, val := range agentdeployment.Spec.ClusterAgent.AdditionalAnnotations {
		newPodTemplate.Annotations[key] = val
	}

	container := &newPodTemplate.Spec.Containers[0]

	if agentdeployment.Spec.ClusterAgent.Config.ExternalMetrics != nil && agentdeployment.Spec.ClusterAgent.Config.ExternalMetrics.Enabled {
		port := getClusterAgentMetricsProviderPort(agentdeployment.Spec.ClusterAgent.Config)
		container.Ports = append(container.Ports, corev1.ContainerPort{
			ContainerPort: port,
			Name:          "metricsapi",
			Protocol:      "TCP",
		})
		probe := &corev1.Probe{
			Handler: corev1.Handler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/healthz",
					Port: intstr.IntOrString{
						IntVal: port,
					},
					Scheme: corev1.URISchemeHTTPS,
				},
			},
		}
		container.LivenessProbe = probe
		container.ReadinessProbe = probe
	}

	if clusterAgentSpec.Config.Resources != nil {
		container.Resources = *clusterAgentSpec.Config.Resources
	}

	return newPodTemplate
}

func getClusterAgentCustomConfigConfigMapName(dda *datadoghqv1alpha1.DatadogAgent) string {
	return fmt.Sprintf("%s-cluster-datadog-yaml", dda.Name)
}

// getEnvVarsForClusterAgent converts Cluster Agent Config into container env vars
func getEnvVarsForClusterAgent(dda *datadoghqv1alpha1.DatadogAgent) []corev1.EnvVar {
	spec := &dda.Spec

	complianceEnabled := isComplianceEnabled(dda)

	envVars := []corev1.EnvVar{
		{
			Name:  datadoghqv1alpha1.DDClusterName,
			Value: spec.ClusterName,
		},
		{
			Name:  datadoghqv1alpha1.DDSite,
			Value: spec.Site,
		},
		{
			Name:  datadoghqv1alpha1.DDClusterChecksEnabled,
			Value: datadoghqv1alpha1.BoolToString(spec.ClusterAgent.Config.ClusterChecksEnabled),
		},
		{
			Name:  datadoghqv1alpha1.DDClusterAgentKubeServiceName,
			Value: getClusterAgentServiceName(dda),
		},
		{
			Name:      datadoghqv1alpha1.DDClusterAgentAuthToken,
			ValueFrom: getClusterAgentAuthToken(dda),
		},
		{
			Name:  datadoghqv1alpha1.DDLeaderElection,
			Value: "true",
		},
		{
			Name:  datadoghqv1alpha1.DDComplianceConfigEnabled,
			Value: strconv.FormatBool(complianceEnabled),
		},
		{
			Name:  datadoghqv1alpha1.DDCollectKubeEvents,
			Value: datadoghqv1alpha1.BoolToString(spec.ClusterAgent.Config.CollectEvents),
		},
	}

	if complianceEnabled {
		if dda.Spec.Agent.Security.Compliance.CheckInterval != nil {
			envVars = append(envVars, corev1.EnvVar{
				Name:  datadoghqv1alpha1.DDComplianceConfigCheckInterval,
				Value: strconv.FormatInt(dda.Spec.Agent.Security.Compliance.CheckInterval.Nanoseconds(), 10),
			})
		}

		if dda.Spec.Agent.Security.Compliance.ConfigDir != nil {
			envVars = append(envVars, corev1.EnvVar{
				Name:  datadoghqv1alpha1.DDComplianceConfigDir,
				Value: datadoghqv1alpha1.SecurityAgentComplianceConfigDirVolumePath,
			})
		}
	}

	if spec.Agent != nil && spec.Agent.Config.DDUrl != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDddURL,
			Value: *spec.Agent.Config.DDUrl,
		})
	}

	if spec.ClusterAgent.Config.LogLevel != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDLogLevel,
			Value: *spec.ClusterAgent.Config.LogLevel,
		})
	}

	if needAgentSecret(dda) {
		envVars = append(envVars, corev1.EnvVar{
			Name:      datadoghqv1alpha1.DDAPIKey,
			ValueFrom: getAPIKeyFromSecret(dda),
		})
	}

	if isMetricsProviderEnabled(spec.ClusterAgent) {
		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDMetricsProviderEnabled,
			Value: strconv.FormatBool(spec.ClusterAgent.Config.ExternalMetrics.Enabled),
		})
		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDMetricsProviderPort,
			Value: strconv.Itoa(int(getClusterAgentMetricsProviderPort(spec.ClusterAgent.Config))),
		})
		envVars = append(envVars, corev1.EnvVar{
			Name:      datadoghqv1alpha1.DDAppKey,
			ValueFrom: getAppKeyFromSecret(dda),
		})
		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DatadogHost,
			Value: getExternalMetricsEndpoint(dda),
		})
		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDMetricsProviderUseDatadogMetric,
			Value: strconv.FormatBool(spec.ClusterAgent.Config.ExternalMetrics.UseDatadogMetrics),
		})
		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDMetricsProviderWPAController,
			Value: strconv.FormatBool(spec.ClusterAgent.Config.ExternalMetrics.WpaController),
		})
	}

	// Cluster Checks config
	if datadoghqv1alpha1.BoolValue(spec.ClusterAgent.Config.ClusterChecksEnabled) {
		envVars = append(envVars, []corev1.EnvVar{
			{
				Name:  datadoghqv1alpha1.DDExtraConfigProviders,
				Value: datadoghqv1alpha1.KubeServicesAndEndpointsConfigProviders,
			},
			{
				Name:  datadoghqv1alpha1.DDExtraListeners,
				Value: datadoghqv1alpha1.KubeServicesAndEndpointsListeners,
			},
		}...)
	}

	if isAdmissionControllerEnabled(spec.ClusterAgent) {
		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDAdmissionControllerEnabled,
			Value: strconv.FormatBool(spec.ClusterAgent.Config.AdmissionController.Enabled),
		})
		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDAdmissionControllerMutateUnlabelled,
			Value: datadoghqv1alpha1.BoolToString(spec.ClusterAgent.Config.AdmissionController.MutateUnlabelled),
		})
		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDAdmissionControllerServiceName,
			Value: getAdmissionControllerServiceName(dda),
		})
	}

	return append(envVars, spec.ClusterAgent.Config.Env...)
}

func getClusterAgentName(dda *datadoghqv1alpha1.DatadogAgent) string {
	if dda.Spec.ClusterAgent != nil && dda.Spec.ClusterAgent.DeploymentName != "" {
		return dda.Spec.ClusterAgent.DeploymentName
	}
	return fmt.Sprintf("%s-%s", dda.Name, "cluster-agent")
}

func getClusterAgentMetricsProviderPort(config datadoghqv1alpha1.ClusterAgentConfig) int32 {
	if config.ExternalMetrics != nil && config.ExternalMetrics.Port != nil {
		return *config.ExternalMetrics.Port
	}
	return int32(datadoghqv1alpha1.DefaultMetricsServerTargetPort)
}

func getAdmissionControllerServiceName(dda *datadoghqv1alpha1.DatadogAgent) string {
	if dda.Spec.ClusterAgent != nil && dda.Spec.ClusterAgent.Config.AdmissionController != nil && dda.Spec.ClusterAgent.Config.AdmissionController.ServiceName != nil {
		return *dda.Spec.ClusterAgent.Config.AdmissionController.ServiceName
	}
	return datadoghqv1alpha1.DefaultAdmissionServiceName
}

// manageClusterAgentRBACs creates deletes and updates the RBACs for the Cluster Agent
func (r *Reconciler) manageClusterAgentRBACs(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	if dda.Spec.ClusterAgent == nil {
		return r.cleanupClusterAgentRbacResources(logger, dda)
	}

	if !isCreateRBACEnabled(dda.Spec.ClusterAgent.Rbac) {
		return reconcile.Result{}, nil
	}

	rbacResourcesName := getClusterAgentRbacResourcesName(dda)
	clusterAgentVersion := getClusterAgentVersion(dda)

	// Create ServiceAccount
	serviceAccount := &corev1.ServiceAccount{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesName, Namespace: dda.Namespace}, serviceAccount); err != nil {
		if errors.IsNotFound(err) {
			return r.createServiceAccount(logger, dda, rbacResourcesName, clusterAgentVersion)
		}
		return reconcile.Result{}, err
	}

	// Create or update ClusterRole
	clusterRole := &rbacv1.ClusterRole{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesName}, clusterRole); err != nil {
		if errors.IsNotFound(err) {
			return r.createClusterAgentClusterRole(logger, dda, rbacResourcesName, clusterAgentVersion)
		}
		return reconcile.Result{}, err
	}
	if result, err := r.updateIfNeededClusterAgentClusterRole(logger, dda, rbacResourcesName, clusterAgentVersion, clusterRole); err != nil {
		return result, err
	}

	// Create ClusterRoleBinding
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesName}, clusterRoleBinding); err != nil {
		if errors.IsNotFound(err) {
			return r.createClusterRoleBinding(logger, dda, roleBindingInfo{
				name:               rbacResourcesName,
				roleName:           rbacResourcesName,
				serviceAccountName: getClusterAgentServiceAccount(dda),
			}, clusterAgentVersion)
		}
		return reconcile.Result{}, err
	}

	// Create or update Role
	role := &rbacv1.Role{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesName, Namespace: dda.Namespace}, role); err != nil {
		if errors.IsNotFound(err) {
			return r.createClusterAgentRole(logger, dda, rbacResourcesName, clusterAgentVersion)
		}
		return reconcile.Result{}, err
	}
	if result, err := r.updateIfNeededClusterAgentRole(logger, dda, rbacResourcesName, clusterAgentVersion, role); err != nil {
		return result, err
	}

	// Create or update RoleBinding
	roleBinding := &rbacv1.RoleBinding{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: rbacResourcesName, Namespace: dda.Namespace}, roleBinding); err != nil {
		if errors.IsNotFound(err) {
			info := roleBindingInfo{
				name:               rbacResourcesName,
				roleName:           rbacResourcesName,
				serviceAccountName: getClusterAgentServiceAccount(dda),
			}
			return r.createClusterAgentRoleBinding(logger, dda, info, clusterAgentVersion)
		}
		return reconcile.Result{}, err
	}

	metricsProviderEnabled := isMetricsProviderEnabled(dda.Spec.ClusterAgent)
	// Create or delete HPA ClusterRoleBinding
	hpaClusterRoleBindingName := getHPAClusterRoleBindingName(dda)
	hpaClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: hpaClusterRoleBindingName}, hpaClusterRoleBinding); err != nil {
		if errors.IsNotFound(err) {
			if metricsProviderEnabled {
				return r.createHPAClusterRoleBinding(logger, dda, hpaClusterRoleBindingName, clusterAgentVersion)
			}
		} else {
			return reconcile.Result{}, err
		}
	} else if !metricsProviderEnabled {
		return r.cleanupClusterRoleBinding(logger, r.client, dda, hpaClusterRoleBindingName)
	}

	// Create or delete external metrics reader ClusterRole and ClusterRoleBinding
	metricsReaderClusterRoleName := getExternalMetricsReaderClusterRoleName(dda, r.versionInfo)

	metricsReaderClusterRole := &rbacv1.ClusterRole{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: metricsReaderClusterRoleName}, metricsReaderClusterRole); err != nil {
		if errors.IsNotFound(err) {
			if metricsProviderEnabled {
				return r.createExternalMetricsReaderClusterRole(logger, dda, metricsReaderClusterRoleName, clusterAgentVersion)
			}
		} else {
			return reconcile.Result{}, err
		}
	} else if !metricsProviderEnabled {
		return r.cleanupClusterRole(logger, r.client, dda, metricsReaderClusterRoleName)
	}

	metricsReaderClusterRoleBinding := &rbacv1.ClusterRoleBinding{}
	if err := r.client.Get(context.TODO(), types.NamespacedName{Name: metricsReaderClusterRoleName}, metricsReaderClusterRoleBinding); err != nil {
		if errors.IsNotFound(err) {
			if metricsProviderEnabled {
				return r.createExternalMetricsReaderClusterRoleBinding(logger, dda, metricsReaderClusterRoleName, clusterAgentVersion)
			}
		} else {
			return reconcile.Result{}, err
		}
	} else if !metricsProviderEnabled {
		return r.cleanupClusterRoleBinding(logger, r.client, dda, metricsReaderClusterRoleName)
	}

	if result, err := r.updateIfNeededClusterAgentRoleBinding(logger, dda, clusterAgentVersion, roleBinding); err != nil {
		return result, err
	}

	return reconcile.Result{}, nil
}

func (r *Reconciler) createClusterAgentClusterRole(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string) (reconcile.Result, error) {
	clusterRole := buildClusterAgentClusterRole(dda, name, agentVersion)
	if err := SetOwnerReference(dda, clusterRole, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.V(1).Info("createClusterAgentClusterRole", "clusterRole.name", clusterRole.Name)
	event := buildEventInfo(clusterRole.Name, clusterRole.Namespace, clusterRoleKind, datadog.CreationEvent)
	r.recordEvent(dda, event)
	return reconcile.Result{Requeue: true}, r.client.Create(context.TODO(), clusterRole)
}

func (r *Reconciler) createClusterAgentRole(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string) (reconcile.Result, error) {
	role := buildClusterAgentRole(dda, name, agentVersion)
	if err := controllerutil.SetControllerReference(dda, role, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.V(1).Info("createClusterAgentRole", "role.name", role.Name)
	event := buildEventInfo(role.Name, role.Namespace, roleKind, datadog.CreationEvent)
	r.recordEvent(dda, event)
	return reconcile.Result{Requeue: true}, r.client.Create(context.TODO(), role)
}

func (r *Reconciler) createAgentClusterRole(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string) (reconcile.Result, error) {
	clusterRole := buildAgentClusterRole(dda, name, agentVersion)
	if err := SetOwnerReference(dda, clusterRole, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.V(1).Info("createAgentClusterRole", "clusterRole.name", clusterRole.Name)
	event := buildEventInfo(clusterRole.Name, clusterRole.Namespace, clusterRoleKind, datadog.CreationEvent)
	r.recordEvent(dda, event)
	return reconcile.Result{Requeue: true}, r.client.Create(context.TODO(), clusterRole)
}

func (r *Reconciler) updateIfNeededClusterAgentClusterRole(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string, clusterRole *rbacv1.ClusterRole) (reconcile.Result, error) {
	newClusterRole := buildClusterAgentClusterRole(dda, name, agentVersion)
	if !apiequality.Semantic.DeepEqual(newClusterRole.Rules, clusterRole.Rules) {
		logger.V(1).Info("updateClusterAgentClusterRole", "clusterRole.name", clusterRole.Name)
		if err := r.client.Update(context.TODO(), newClusterRole); err != nil {
			return reconcile.Result{}, err
		}
		event := buildEventInfo(newClusterRole.Name, newClusterRole.Namespace, clusterRoleKind, datadog.UpdateEvent)
		r.recordEvent(dda, event)
	}
	return reconcile.Result{}, nil
}

func (r *Reconciler) updateIfNeededClusterAgentRole(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string, role *rbacv1.Role) (reconcile.Result, error) {
	newRole := buildClusterAgentRole(dda, name, agentVersion)
	if !apiequality.Semantic.DeepEqual(newRole.Rules, role.Rules) {
		logger.V(1).Info("updateClusterAgentRole", "role.name", newRole.Name)
		if err := r.client.Update(context.TODO(), newRole); err != nil {
			return reconcile.Result{}, err
		}
		event := buildEventInfo(newRole.Name, newRole.Namespace, roleKind, datadog.UpdateEvent)
		r.recordEvent(dda, event)
	}
	return reconcile.Result{}, nil
}

func (r *Reconciler) updateIfNeededAgentClusterRole(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string, clusterRole *rbacv1.ClusterRole) (reconcile.Result, error) {
	newClusterRole := buildAgentClusterRole(dda, name, agentVersion)
	if !apiequality.Semantic.DeepEqual(newClusterRole.Rules, clusterRole.Rules) {
		logger.V(1).Info("updateAgentClusterRole", "clusterRole.name", clusterRole.Name)
		if err := r.client.Update(context.TODO(), newClusterRole); err != nil {
			return reconcile.Result{}, err
		}
		event := buildEventInfo(newClusterRole.Name, newClusterRole.Namespace, clusterRoleKind, datadog.UpdateEvent)
		r.recordEvent(dda, event)
	}
	return reconcile.Result{}, nil
}

// cleanupClusterAgentRbacResources deletes ClusterRole, ClusterRoleBindings, and ServiceAccount of the Cluster Agent
func (r *Reconciler) cleanupClusterAgentRbacResources(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	rbacResourcesName := getClusterAgentRbacResourcesName(dda)
	// Delete ClusterRole
	if result, err := r.cleanupClusterRole(logger, r.client, dda, rbacResourcesName); err != nil {
		return result, err
	}
	// Delete Cluster Role Binding
	if result, err := r.cleanupClusterRoleBinding(logger, r.client, dda, rbacResourcesName); err != nil {
		return result, err
	}
	// Delete HPA Cluster Role Binding
	hpaClusterRoleBindingName := getHPAClusterRoleBindingName(dda)
	if result, err := r.cleanupClusterRoleBinding(logger, r.client, dda, hpaClusterRoleBindingName); err != nil {
		return result, err
	}

	externalMetricsReaderName := getExternalMetricsReaderClusterRoleName(dda, r.versionInfo)
	if result, err := r.cleanupClusterRoleBinding(logger, r.client, dda, externalMetricsReaderName); err != nil {
		return result, err
	}

	if result, err := r.cleanupClusterRole(logger, r.client, dda, externalMetricsReaderName); err != nil {
		return result, err
	}

	// Delete Service Account
	if result, err := r.cleanupServiceAccount(logger, r.client, dda, rbacResourcesName); err != nil {
		return result, err
	}
	return reconcile.Result{}, nil
}

func (r *Reconciler) createClusterAgentRoleBinding(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, info roleBindingInfo, agentVersion string) (reconcile.Result, error) {
	roleBinding := buildRoleBinding(dda, info, agentVersion)
	if err := controllerutil.SetControllerReference(dda, roleBinding, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.V(1).Info("createClusterAgentRoleBinding", "roleBinding.name", roleBinding.Name, "roleBinding.Namespace", roleBinding.Namespace)
	event := buildEventInfo(roleBinding.Name, roleBinding.Namespace, roleBindingKind, datadog.CreationEvent)
	r.recordEvent(dda, event)
	return reconcile.Result{}, r.client.Create(context.TODO(), roleBinding)
}

func (r *Reconciler) updateIfNeededClusterAgentRoleBinding(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, agentVersion string, roleBinding *rbacv1.RoleBinding) (reconcile.Result, error) {
	info := roleBindingInfo{
		name:               getClusterAgentRbacResourcesName(dda),
		roleName:           getClusterAgentRbacResourcesName(dda),
		serviceAccountName: getClusterAgentServiceAccount(dda),
	}
	newRoleBinding := buildRoleBinding(dda, info, agentVersion)
	if !apiequality.Semantic.DeepEqual(newRoleBinding.RoleRef, roleBinding.RoleRef) || !apiequality.Semantic.DeepEqual(newRoleBinding.Subjects, roleBinding.Subjects) {
		logger.V(1).Info("updateAgentClusterRoleBinding", "roleBinding.name", newRoleBinding.Name, "roleBinding.namespace", newRoleBinding.Namespace)
		event := buildEventInfo(newRoleBinding.Name, newRoleBinding.Namespace, roleBindingKind, datadog.UpdateEvent)
		r.recordEvent(dda, event)
		if err := r.client.Update(context.TODO(), newRoleBinding); err != nil {
			return reconcile.Result{}, err
		}
	}
	return reconcile.Result{}, nil
}

// buildAgentClusterRole creates a ClusterRole object for the Agent based on its config
func buildAgentClusterRole(dda *datadoghqv1alpha1.DatadogAgent, name, version string) *rbacv1.ClusterRole {
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Labels: getDefaultLabels(dda, name, version),
			Name:   name,
		},
	}

	rbacRules := []rbacv1.PolicyRule{
		{
			// Get /metrics permissions
			NonResourceURLs: []string{datadoghqv1alpha1.MetricsURL},
			Verbs:           []string{datadoghqv1alpha1.GetVerb},
		},
		{
			// Kubelet connectivity
			APIGroups: []string{datadoghqv1alpha1.CoreAPIGroup},
			Resources: []string{
				datadoghqv1alpha1.NodeMetricsResource,
				datadoghqv1alpha1.NodeSpecResource,
				datadoghqv1alpha1.NodeProxyResource,
				datadoghqv1alpha1.NodeStats,
			},
			Verbs: []string{datadoghqv1alpha1.GetVerb},
		},
		{
			// Leader election check
			APIGroups: []string{datadoghqv1alpha1.CoreAPIGroup},
			Resources: []string{datadoghqv1alpha1.EndpointsResource},
			Verbs:     []string{datadoghqv1alpha1.GetVerb},
		},
	}

	if dda.Spec.ClusterAgent == nil {
		// Cluster Agent is disabled, the Agent needs extra permissions
		// to collect cluster level metrics and events
		rbacRules = append(rbacRules, getDefaultClusterAgentPolicyRules()...)

		if dda.Spec.Agent != nil {
			if datadoghqv1alpha1.BoolValue(dda.Spec.Agent.Config.CollectEvents) {
				rbacRules = append(rbacRules, getEventCollectionPolicyRule())
			}

			if datadoghqv1alpha1.BoolValue(dda.Spec.Agent.Config.LeaderElection) {
				rbacRules = append(rbacRules, getLeaderElectionPolicyRule()...)
			}
		}
	}

	clusterRole.Rules = rbacRules

	return clusterRole
}

// getDefaultClusterAgentPolicyRules returns the default policy rules for the Cluster Agent
// Can be used by the Agent if the Cluster Agent is disabled
func getDefaultClusterAgentPolicyRules() []rbacv1.PolicyRule {
	return append([]rbacv1.PolicyRule{
		{
			APIGroups: []string{datadoghqv1alpha1.CoreAPIGroup},
			Resources: []string{
				datadoghqv1alpha1.ServicesResource,
				datadoghqv1alpha1.EventsResource,
				datadoghqv1alpha1.EndpointsResource,
				datadoghqv1alpha1.PodsResource,
				datadoghqv1alpha1.NodesResource,
				datadoghqv1alpha1.ComponentStatusesResource,
			},
			Verbs: []string{
				datadoghqv1alpha1.GetVerb,
				datadoghqv1alpha1.ListVerb,
				datadoghqv1alpha1.WatchVerb,
			},
		},
		{
			APIGroups: []string{datadoghqv1alpha1.OpenShiftQuotaAPIGroup},
			Resources: []string{datadoghqv1alpha1.ClusterResourceQuotasResource},
			Verbs:     []string{datadoghqv1alpha1.GetVerb, datadoghqv1alpha1.ListVerb},
		},
		{
			NonResourceURLs: []string{datadoghqv1alpha1.VersionURL, datadoghqv1alpha1.HealthzURL},
			Verbs:           []string{datadoghqv1alpha1.GetVerb},
		},
	}, getLeaderElectionPolicyRule()...)
}

// buildClusterRoleBinding creates a ClusterRoleBinding object
func buildClusterRoleBinding(dda *datadoghqv1alpha1.DatadogAgent, info roleBindingInfo, agentVersion string) *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Labels: getDefaultLabels(dda, info.name, agentVersion),
			Name:   info.name,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: datadoghqv1alpha1.RbacAPIGroup,
			Kind:     datadoghqv1alpha1.ClusterRoleKind,
			Name:     info.roleName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      datadoghqv1alpha1.ServiceAccountKind,
				Name:      info.serviceAccountName,
				Namespace: dda.Namespace,
			},
		},
	}
}

// buildClusterAgentClusterRole creates a ClusterRole object for the Cluster Agent based on its config
func buildClusterAgentClusterRole(dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string) *rbacv1.ClusterRole {
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Labels: getDefaultLabels(dda, name, agentVersion),
			Name:   name,
		},
	}

	rbacRules := getDefaultClusterAgentPolicyRules()

	rbacRules = append(rbacRules, rbacv1.PolicyRule{
		// Horizontal Pod Autoscaling
		APIGroups: []string{datadoghqv1alpha1.AutoscalingAPIGroup},
		Resources: []string{datadoghqv1alpha1.HorizontalPodAutoscalersRecource},
		Verbs:     []string{datadoghqv1alpha1.ListVerb, datadoghqv1alpha1.WatchVerb},
	})

	rbacRules = append(rbacRules, rbacv1.PolicyRule{
		APIGroups: []string{datadoghqv1alpha1.CoreAPIGroup},
		Resources: []string{datadoghqv1alpha1.NamespaceResource},
		ResourceNames: []string{
			datadoghqv1alpha1.KubeSystemResourceName,
		},
		Verbs: []string{datadoghqv1alpha1.GetVerb},
	})

	if datadoghqv1alpha1.BoolValue(dda.Spec.ClusterAgent.Config.CollectEvents) {
		rbacRules = append(rbacRules, getEventCollectionPolicyRule())
	}

	if dda.Spec.ClusterAgent.Config.ExternalMetrics != nil && dda.Spec.ClusterAgent.Config.ExternalMetrics.Enabled {
		rbacRules = append(rbacRules, rbacv1.PolicyRule{
			APIGroups: []string{datadoghqv1alpha1.CoreAPIGroup},
			Resources: []string{datadoghqv1alpha1.ConfigMapsResource},
			ResourceNames: []string{
				datadoghqv1alpha1.DatadogCustomMetricsResourceName,
				datadoghqv1alpha1.ExtensionAPIServerAuthResourceName,
			},
			Verbs: []string{datadoghqv1alpha1.GetVerb, datadoghqv1alpha1.UpdateVerb},
		})

		if dda.Spec.ClusterAgent.Config.ExternalMetrics.UseDatadogMetrics {
			rbacRules = append(rbacRules, rbacv1.PolicyRule{
				APIGroups: []string{datadoghqv1alpha1.DatadogAPIGroup},
				Resources: []string{datadoghqv1alpha1.DatadogMetricsResource},
				Verbs: []string{
					datadoghqv1alpha1.ListVerb,
					datadoghqv1alpha1.WatchVerb,
					datadoghqv1alpha1.CreateVerb,
					datadoghqv1alpha1.DeleteVerb},
			})

			// Specific update rule for status subresource
			rbacRules = append(rbacRules, rbacv1.PolicyRule{
				APIGroups: []string{datadoghqv1alpha1.DatadogAPIGroup},
				Resources: []string{datadoghqv1alpha1.DatadogMetricsStatusResource},
				Verbs:     []string{datadoghqv1alpha1.UpdateVerb},
			})
		}

		if dda.Spec.ClusterAgent.Config.ExternalMetrics.WpaController {
			rbacRules = append(rbacRules, rbacv1.PolicyRule{
				APIGroups: []string{datadoghqv1alpha1.DatadogAPIGroup},
				Resources: []string{datadoghqv1alpha1.WpaResource},
				Verbs: []string{
					datadoghqv1alpha1.ListVerb,
					datadoghqv1alpha1.WatchVerb,
					datadoghqv1alpha1.GetVerb},
			})
		}
	}

	if dda.Spec.ClusterAgent.Config.AdmissionController != nil && dda.Spec.ClusterAgent.Config.AdmissionController.Enabled {
		// MutatingWebhooksConfigs
		rbacRules = append(rbacRules, rbacv1.PolicyRule{
			APIGroups: []string{datadoghqv1alpha1.AdmissionAPIGroup},
			Resources: []string{datadoghqv1alpha1.MutatingConfigResource},
			Verbs: []string{
				datadoghqv1alpha1.GetVerb,
				datadoghqv1alpha1.ListVerb,
				datadoghqv1alpha1.WatchVerb,
				datadoghqv1alpha1.CreateVerb,
				datadoghqv1alpha1.UpdateVerb},
		})

		// Secrets
		rbacRules = append(rbacRules, rbacv1.PolicyRule{
			APIGroups: []string{datadoghqv1alpha1.CoreAPIGroup},
			Resources: []string{datadoghqv1alpha1.SecretsResource},
			Verbs: []string{
				datadoghqv1alpha1.GetVerb,
				datadoghqv1alpha1.ListVerb,
				datadoghqv1alpha1.WatchVerb,
				datadoghqv1alpha1.CreateVerb,
				datadoghqv1alpha1.UpdateVerb},
		})

		// ExtendedDaemonsetReplicaSets
		rbacRules = append(rbacRules, rbacv1.PolicyRule{
			APIGroups: []string{datadoghqv1alpha1.GroupVersion.Group},
			Resources: []string{
				datadoghqv1alpha1.ExtendedDaemonSetReplicaSetResource,
			},
			Verbs: []string{datadoghqv1alpha1.GetVerb},
		})

		// Deployments, Replicasets, Statefulsets, Daemonsets,
		rbacRules = append(rbacRules, rbacv1.PolicyRule{
			APIGroups: []string{datadoghqv1alpha1.AppsAPIGroup},
			Resources: []string{
				datadoghqv1alpha1.DeploymentsResource,
				datadoghqv1alpha1.ReplicasetsResource,
				datadoghqv1alpha1.StatefulsetsResource,
				datadoghqv1alpha1.DaemonsetsResource,
			},
			Verbs: []string{datadoghqv1alpha1.GetVerb},
		})

		// Jobs
		rbacRules = append(rbacRules, rbacv1.PolicyRule{
			APIGroups: []string{datadoghqv1alpha1.BatchAPIGroup},
			Resources: []string{datadoghqv1alpha1.JobsResource},
			Verbs:     []string{datadoghqv1alpha1.GetVerb},
		})

		// CronJobs
		rbacRules = append(rbacRules, rbacv1.PolicyRule{
			APIGroups: []string{datadoghqv1alpha1.BatchAPIGroup},
			Resources: []string{datadoghqv1alpha1.CronjobsResource},
			Verbs:     []string{datadoghqv1alpha1.GetVerb},
		})
	}

	if isComplianceEnabled(dda) {
		// ServiceAccounts and Namespaces
		rbacRules = append(rbacRules, rbacv1.PolicyRule{
			APIGroups: []string{datadoghqv1alpha1.CoreAPIGroup},
			Resources: []string{datadoghqv1alpha1.ServiceAccountResource, datadoghqv1alpha1.NamespaceResource},
			Verbs: []string{
				datadoghqv1alpha1.ListVerb,
			},
		})

		// PodSecurityPolicies
		rbacRules = append(rbacRules, rbacv1.PolicyRule{
			APIGroups: []string{datadoghqv1alpha1.PolicyAPIGroup},
			Resources: []string{datadoghqv1alpha1.PodSecurityPolicyResource},
			Verbs: []string{
				datadoghqv1alpha1.ListVerb,
				datadoghqv1alpha1.GetVerb,
				datadoghqv1alpha1.ListVerb,
				datadoghqv1alpha1.WatchVerb,
			},
		})

		// ClusterRoleBindings and RoleBindings
		rbacRules = append(rbacRules, rbacv1.PolicyRule{
			APIGroups: []string{datadoghqv1alpha1.RbacAPIGroup},
			Resources: []string{datadoghqv1alpha1.ClusterRoleBindingResource, datadoghqv1alpha1.RoleBindingResource},
			Verbs: []string{
				datadoghqv1alpha1.ListVerb,
			},
		})

		// NetworkPolicies
		rbacRules = append(rbacRules, rbacv1.PolicyRule{
			APIGroups: []string{datadoghqv1alpha1.NetworkingAPIGroup},
			Resources: []string{datadoghqv1alpha1.NetworkPolicyResource},
			Verbs: []string{
				datadoghqv1alpha1.ListVerb,
			},
		})
	}

	clusterRole.Rules = rbacRules

	return clusterRole
}

// buildClusterAgentRole creates a Role object for the Cluster Agent based on its config
func buildClusterAgentRole(dda *datadoghqv1alpha1.DatadogAgent, name, agentVersion string) *rbacv1.Role {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    getDefaultLabels(dda, name, agentVersion),
			Name:      name,
			Namespace: dda.Namespace,
		},
	}

	rbacRules := getLeaderElectionPolicyRule()

	rbacRules = append(rbacRules, rbacv1.PolicyRule{
		APIGroups: []string{datadoghqv1alpha1.CoreAPIGroup},
		Resources: []string{datadoghqv1alpha1.ConfigMapsResource},
		ResourceNames: []string{
			datadoghqv1alpha1.DatadogClusterIDResourceName,
		},
		Verbs: []string{datadoghqv1alpha1.GetVerb, datadoghqv1alpha1.UpdateVerb, datadoghqv1alpha1.CreateVerb},
	})

	if dda.Spec.ClusterAgent.Config.ExternalMetrics != nil && dda.Spec.ClusterAgent.Config.ExternalMetrics.Enabled {
		rbacRules = append(rbacRules, rbacv1.PolicyRule{
			APIGroups: []string{datadoghqv1alpha1.CoreAPIGroup},
			Resources: []string{datadoghqv1alpha1.ConfigMapsResource},
			ResourceNames: []string{
				datadoghqv1alpha1.DatadogCustomMetricsResourceName,
				datadoghqv1alpha1.ExtensionAPIServerAuthResourceName,
			},
			Verbs: []string{datadoghqv1alpha1.GetVerb, datadoghqv1alpha1.UpdateVerb},
		})
	}

	role.Rules = rbacRules

	return role
}

func getExternalMetricsEndpoint(dda *datadoghqv1alpha1.DatadogAgent) string {
	if dda.Spec.ClusterAgent != nil && dda.Spec.ClusterAgent.Config.ExternalMetrics != nil && dda.Spec.ClusterAgent.Config.ExternalMetrics.Endpoint != nil {
		return *dda.Spec.ClusterAgent.Config.ExternalMetrics.Endpoint
	}

	if dda.Spec.Agent != nil && dda.Spec.Agent.Config.DDUrl != nil {
		return *dda.Spec.Agent.Config.DDUrl
	}

	if dda.Spec.Site != "" {
		return fmt.Sprintf("https://app.%s", dda.Spec.Site)
	}

	return "https://app.datadoghq.com"
}

func (r *Reconciler) manageClusterAgentNetworkPolicy(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	policyName := fmt.Sprintf("%s-%s", dda.Name, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix)

	spec := dda.Spec.ClusterAgent
	if spec == nil || !datadoghqv1alpha1.BoolValue(spec.NetworkPolicy.Create) {
		return r.cleanupNetworkPolicy(logger, dda, policyName)
	}

	return r.ensureNetworkPolicy(logger, dda, policyName, buildClusterAgentNetworkPolicy)
}

func buildClusterAgentNetworkPolicy(dda *datadoghqv1alpha1.DatadogAgent, name string) *networkingv1.NetworkPolicy {
	egressRules := []networkingv1.NetworkPolicyEgressRule{
		// Egress to datadog intake and
		// kubeapi server
		{
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Port: &intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: 443,
					},
				},
			},
		},
	}

	ingressRules := []networkingv1.NetworkPolicyIngressRule{
		// Ingress for the node agents
		{
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Port: &intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: datadoghqv1alpha1.DefaultClusterAgentServicePort,
					},
				},
			},
			From: []networkingv1.NetworkPolicyPeer{
				{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							kubernetes.AppKubernetesInstanceLabelKey: datadoghqv1alpha1.DefaultAgentResourceSuffix,
							kubernetes.AppKubernetesPartOfLabelKey:   dda.Name,
						},
					},
				},
			},
		},
	}

	if datadoghqv1alpha1.BoolValue(dda.Spec.ClusterAgent.Config.ClusterChecksEnabled) {
		ingressRules = append(ingressRules, networkingv1.NetworkPolicyIngressRule{
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Port: &intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: datadoghqv1alpha1.DefaultClusterAgentServicePort,
					},
				},
			},
			From: []networkingv1.NetworkPolicyPeer{
				{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							kubernetes.AppKubernetesInstanceLabelKey: datadoghqv1alpha1.DefaultClusterChecksRunnerResourceSuffix,
							kubernetes.AppKubernetesPartOfLabelKey:   dda.Name,
						},
					},
				},
			},
		})
	}

	if dda.Spec.ClusterAgent.Config.ExternalMetrics != nil && dda.Spec.ClusterAgent.Config.ExternalMetrics.Enabled {
		ingressRules = append(ingressRules, networkingv1.NetworkPolicyIngressRule{
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Port: &intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: int32(datadoghqv1alpha1.DefaultMetricsServerTargetPort),
					},
				},
			},
		})
	}

	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Labels:    getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda)),
			Name:      name,
			Namespace: dda.Namespace,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					kubernetes.AppKubernetesInstanceLabelKey: datadoghqv1alpha1.DefaultClusterAgentResourceSuffix,
					kubernetes.AppKubernetesPartOfLabelKey:   dda.Name,
				},
			},
			Ingress: ingressRules,
			Egress:  egressRules,
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
		},
	}

	return policy
}
