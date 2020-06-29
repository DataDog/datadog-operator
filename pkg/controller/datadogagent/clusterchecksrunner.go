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
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
)

func (r *ReconcileDatadogAgent) reconcileClusterChecksRunner(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	result, err := r.manageClusterChecksRunnerDependencies(logger, dda, newStatus)
	if shouldReturn(result, err) {
		return result, err
	}

	if !needClusterChecksRunner(dda) {
		return r.cleanupClusterChecksRunner(logger, dda, newStatus)
	}

	if newStatus.ClusterChecksRunner != nil &&
		newStatus.ClusterChecksRunner.DeploymentName != "" &&
		newStatus.ClusterChecksRunner.DeploymentName != getClusterChecksRunnerName(dda) {
		return result, fmt.Errorf("datadog cluster checks runner Deployment cannot be renamed once created")
	}

	nsName := types.NamespacedName{
		Name:      getClusterChecksRunnerName(dda),
		Namespace: dda.Namespace,
	}
	// ClusterChecksRunnerDeployment attached to this instance
	ClusterChecksRunnerDeployment := &appsv1.Deployment{}
	if needClusterChecksRunner(dda) {
		err := r.client.Get(context.TODO(), nsName, ClusterChecksRunnerDeployment)
		if err != nil {
			if errors.IsNotFound(err) {
				logger.Info("ClusterChecksRunner deployment not found", "name", nsName.Name, "namespace", nsName.Namespace)
				// Create and attach a ClusterChecksRunner Deployment
				var result reconcile.Result
				result, err = r.createNewClusterChecksRunnerDeployment(logger, dda, newStatus)
				return result, err
			}
			return reconcile.Result{}, err
		}

		result, err := r.updateClusterChecksRunnerDeployment(logger, dda, ClusterChecksRunnerDeployment, newStatus)
		return result, err
	}
	return reconcile.Result{}, nil
}

func needClusterChecksRunner(dda *datadoghqv1alpha1.DatadogAgent) bool {
	if dda.Spec.ClusterAgent != nil && dda.Spec.ClusterChecksRunner != nil && datadoghqv1alpha1.BoolValue(dda.Spec.ClusterAgent.Config.ClusterChecksEnabled) {
		return true
	}

	return false
}

func (r *ReconcileDatadogAgent) createNewClusterChecksRunnerDeployment(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	newDCAW, hash, err := newClusterChecksRunnerDeploymentFromInstance(dda, nil)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Set ClusterChecksRunner Deployment instance as the owner and controller
	if err = controllerutil.SetControllerReference(dda, newDCAW, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.Info("Creating a new Cluster Checks Runner Deployment", "deployment.Namespace", newDCAW.Namespace, "deployment.Name", newDCAW.Name, "agentdeployment.Status.ClusterChecksRunner.CurrentHash", hash)
	newStatus.ClusterChecksRunner = &datadoghqv1alpha1.DeploymentStatus{}
	err = r.client.Create(context.TODO(), newDCAW)
	now := metav1.NewTime(time.Now())
	if err != nil {
		updateStatusWithClusterChecksRunner(nil, newStatus, &now)
		return reconcile.Result{}, err
	}

	updateStatusWithClusterChecksRunner(newDCAW, newStatus, &now)
	event := buildEventInfo(newDCAW.Name, newDCAW.Namespace, deploymentKind, datadog.CreationEvent)
	r.recordEvent(dda, event)
	return reconcile.Result{}, nil
}

func updateStatusWithClusterChecksRunner(dcaw *appsv1.Deployment, newStatus *datadoghqv1alpha1.DatadogAgentStatus, updateTime *metav1.Time) {
	newStatus.ClusterChecksRunner = updateDeploymentStatus(dcaw, newStatus.ClusterChecksRunner, updateTime)
}

func (r *ReconcileDatadogAgent) updateClusterChecksRunnerDeployment(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, dep *appsv1.Deployment, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	newDCAW, hash, err := newClusterChecksRunnerDeploymentFromInstance(dda, dep.Spec.Selector)
	if err != nil {
		return reconcile.Result{}, err
	}

	var needUpdate bool
	if !comparison.IsSameSpecMD5Hash(hash, dep.GetAnnotations()) {
		needUpdate = true
	}

	updateStatusWithClusterChecksRunner(dep, newStatus, nil)

	if !needUpdate {
		return reconcile.Result{}, nil
	}

	logger.Info("update Cluster Checks Runner deployment", "name", dep.Name, "namespace", dep.Namespace)

	// Set DatadogAgent instance  instance as the owner and controller
	if err = controllerutil.SetControllerReference(dda, dep, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.Info("Updating an existing Cluster Checks Runner Deployment", "deployment.Namespace", newDCAW.Namespace, "deployment.Name", newDCAW.Name, "currentHash", hash)

	// Copy possibly changed fields
	updateDca := dep.DeepCopy()
	updateDca.Spec = *newDCAW.Spec.DeepCopy()
	for k, v := range newDCAW.Annotations {
		updateDca.Annotations[k] = v
	}
	for k, v := range newDCAW.Labels {
		updateDca.Labels[k] = v
	}

	now := metav1.NewTime(time.Now())
	err = r.client.Update(context.TODO(), updateDca)
	if err != nil {
		return reconcile.Result{}, err
	}
	event := buildEventInfo(updateDca.Name, updateDca.Namespace, deploymentKind, datadog.UpdateEvent)
	r.recordEvent(dda, event)
	updateStatusWithClusterChecksRunner(updateDca, newStatus, &now)
	return reconcile.Result{}, nil
}

// newClusterChecksRunnerDeploymentFromInstance creates a Cluster Agent Deployment from a given DatadogAgent
func newClusterChecksRunnerDeploymentFromInstance(
	dda *datadoghqv1alpha1.DatadogAgent,
	selector *metav1.LabelSelector) (*appsv1.Deployment, string, error) {
	labels := map[string]string{
		datadoghqv1alpha1.AgentDeploymentNameLabelKey:      dda.Name,
		datadoghqv1alpha1.AgentDeploymentComponentLabelKey: datadoghqv1alpha1.DefaultClusterChecksRunnerResourceSuffix,
	}
	for key, val := range dda.Labels {
		labels[key] = val
	}
	for key, val := range getDefaultLabels(dda, datadoghqv1alpha1.DefaultClusterChecksRunnerResourceSuffix, getClusterChecksRunnerVersion(dda)) {
		labels[key] = val
	}

	if selector != nil {
		for key, val := range selector.MatchLabels {
			labels[key] = val
		}
	} else {
		selector = &metav1.LabelSelector{
			MatchLabels: map[string]string{
				datadoghqv1alpha1.AgentDeploymentNameLabelKey:      dda.Name,
				datadoghqv1alpha1.AgentDeploymentComponentLabelKey: datadoghqv1alpha1.DefaultClusterChecksRunnerResourceSuffix,
			},
		}
	}

	annotations := map[string]string{}
	for key, val := range dda.Annotations {
		annotations[key] = val
	}

	dca := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        getClusterChecksRunnerName(dda),
			Namespace:   dda.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Template: newClusterChecksRunnerPodTemplate(dda, labels, annotations),
			Replicas: dda.Spec.ClusterChecksRunner.Replicas,
			Selector: selector,
		},
	}
	hash, err := comparison.SetMD5GenerationAnnotation(&dca.ObjectMeta, dda.Spec.ClusterChecksRunner)
	return dca, hash, err
}

func (r *ReconcileDatadogAgent) manageClusterChecksRunnerDependencies(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	result, err := r.manageAgentSecret(logger, dda, newStatus)
	if shouldReturn(result, err) {
		return result, err
	}
	result, err = r.manageClusterChecksRunnerPDB(logger, dda)
	if shouldReturn(result, err) {
		return result, err
	}
	result, err = r.manageConfigMap(logger, dda, getClusterChecksRunnerCustomConfigConfigMapName(dda), buildClusterChecksRunnerConfigurationConfigMap)
	if shouldReturn(result, err) {
		return result, err
	}
	result, err = r.manageClusterChecksRunnerRBACs(logger, dda)
	if shouldReturn(result, err) {
		return result, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileDatadogAgent) cleanupClusterChecksRunner(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	nsName := types.NamespacedName{
		Name:      getClusterChecksRunnerName(dda),
		Namespace: dda.Namespace,
	}
	// ClusterChecksRunnerDeployment attached to this instance
	ClusterChecksRunnerDeployment := &appsv1.Deployment{}
	if err := r.client.Get(context.TODO(), nsName, ClusterChecksRunnerDeployment); err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	logger.Info("Deleting Cluster Checks Runner Deployment", "deployment.Namespace", ClusterChecksRunnerDeployment.Namespace, "deployment.Name", ClusterChecksRunnerDeployment.Name)
	event := buildEventInfo(ClusterChecksRunnerDeployment.Name, ClusterChecksRunnerDeployment.Namespace, deploymentKind, datadog.DeletionEvent)
	r.recordEvent(dda, event)
	if err := r.client.Delete(context.TODO(), ClusterChecksRunnerDeployment); err != nil {
		return reconcile.Result{}, err
	}
	newStatus.ClusterChecksRunner = nil
	return reconcile.Result{Requeue: true}, nil
}

// newClusterChecksRunnerPodTemplate generates a PodTemplate from a DatadogClusterChecksRunnerDeployment spec
func newClusterChecksRunnerPodTemplate(dda *datadoghqv1alpha1.DatadogAgent, labels, annotations map[string]string) corev1.PodTemplateSpec {
	// copy Spec to configure the Cluster Checks Runner Pod Template
	clusterChecksRunnerSpec := dda.Spec.ClusterChecksRunner.DeepCopy()

	newPodTemplate := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: getClusterChecksRunnerServiceAccount(dda),
			Containers: []corev1.Container{
				{
					Name:            "cluster-checks-runner",
					Image:           clusterChecksRunnerSpec.Image.Name,
					ImagePullPolicy: *clusterChecksRunnerSpec.Image.PullPolicy,
					Env:             getEnvVarsForClusterChecksRunner(dda),
					VolumeMounts:    getVolumeMountsForClusterChecksRunner(dda),
					LivenessProbe:   getDefaultLivenessProbe(),
					ReadinessProbe:  getDefaultReadinessProbe(),
				},
			},
			Volumes:           getVolumesForClusterChecksRunner(dda),
			Affinity:          getPodAffinity(clusterChecksRunnerSpec.Affinity, getClusterChecksRunnerName(dda)),
			Tolerations:       clusterChecksRunnerSpec.Tolerations,
			PriorityClassName: clusterChecksRunnerSpec.PriorityClassName,
		},
	}

	for key, val := range clusterChecksRunnerSpec.AdditionalLabels {
		newPodTemplate.Labels[key] = val
	}

	for key, val := range clusterChecksRunnerSpec.AdditionalAnnotations {
		newPodTemplate.Annotations[key] = val
	}

	if clusterChecksRunnerSpec.Config.Resources != nil {
		newPodTemplate.Spec.Containers[0].Resources = *clusterChecksRunnerSpec.Config.Resources
	}

	return newPodTemplate
}

func buildClusterChecksRunnerConfigurationConfigMap(dda *datadoghqv1alpha1.DatadogAgent) (*corev1.ConfigMap, error) {
	if dda.Spec.ClusterChecksRunner == nil {
		return nil, nil
	}
	return buildConfigurationConfigMap(dda, dda.Spec.ClusterChecksRunner.CustomConfig, getClusterChecksRunnerCustomConfigConfigMapName(dda), datadoghqv1alpha1.AgentCustomConfigVolumeSubPath)
}

// getEnvVarsForClusterChecksRunner converts Cluster Checks Runner Config into container env vars
func getEnvVarsForClusterChecksRunner(dda *datadoghqv1alpha1.DatadogAgent) []corev1.EnvVar {
	spec := &dda.Spec
	envVars := []corev1.EnvVar{
		{
			Name:  datadoghqv1alpha1.DDClusterName,
			Value: spec.ClusterName,
		},
		{
			Name:      datadoghqv1alpha1.DDAPIKey,
			ValueFrom: getAPIKeyFromSecret(dda),
		},
		{
			Name:  datadoghqv1alpha1.DDSite,
			Value: spec.Site,
		},
		{
			Name:  datadoghqv1alpha1.DDClusterChecksEnabled,
			Value: "true",
		},
		{
			Name:  datadoghqv1alpha1.DDClusterAgentEnabled,
			Value: "true",
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
			Name:  datadoghqv1alpha1.DDExtraConfigProviders,
			Value: datadoghqv1alpha1.ClusterChecksConfigProvider,
		},
		{
			Name:  datadoghqv1alpha1.DDHealthPort,
			Value: strconv.Itoa(int(datadoghqv1alpha1.DefaultAgentHealthPort)),
		},
		{
			Name:  datadoghqv1alpha1.DDAPMEnabled,
			Value: "false",
		},
		{
			Name:  datadoghqv1alpha1.DDProcessAgentEnabled,
			Value: "false",
		},
		{
			Name:  datadoghqv1alpha1.DDLogsEnabled,
			Value: "false",
		},
		{
			Name:  datadoghqv1alpha1.DDEnableMetadataCollection,
			Value: "false",
		},
		{
			Name:  datadoghqv1alpha1.DDClcRunnerEnabled,
			Value: "true",
		},
		{
			Name: datadoghqv1alpha1.DDClcRunnerHost,
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: FieldPathStatusPodIP,
				},
			},
		},
	}

	if spec.ClusterChecksRunner.Config.LogLevel != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDLogLevel,
			Value: *spec.ClusterChecksRunner.Config.LogLevel,
		})
	}

	if spec.Agent.Config.DDUrl != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDddURL,
			Value: *spec.Agent.Config.DDUrl,
		})
	}

	return append(envVars, spec.ClusterChecksRunner.Config.Env...)
}

func getClusterChecksRunnerVersion(dda *datadoghqv1alpha1.DatadogAgent) string {
	// TODO implement this method
	return ""
}

func getClusterChecksRunnerName(dda *datadoghqv1alpha1.DatadogAgent) string {
	if dda.Spec.ClusterChecksRunner != nil && dda.Spec.ClusterChecksRunner.DeploymentName != "" {
		return dda.Spec.ClusterChecksRunner.DeploymentName
	}
	return fmt.Sprintf("%s-%s", dda.Name, "cluster-checks-runner")
}

// getVolumesForClusterChecksRunner defines volumes for the Cluster Checks Runner
func getVolumesForClusterChecksRunner(dda *datadoghqv1alpha1.DatadogAgent) []corev1.Volume {
	volumes := []corev1.Volume{
		{
			Name: "s6-run",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
		{
			Name: "remove-corechecks",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	if dda.Spec.ClusterChecksRunner.CustomConfig != nil {
		volume := getVolumeFromCustomConfigSpec(dda.Spec.ClusterChecksRunner.CustomConfig, getClusterChecksRunnerCustomConfigConfigMapName(dda), datadoghqv1alpha1.AgentCustomConfigVolumeName)
		volumes = append(volumes, volume)
	}
	return append(volumes, dda.Spec.ClusterChecksRunner.Config.Volumes...)
}

// getVolumeMountsForClusterChecksRunner defines volume mounts for the Cluster Checks Runner
func getVolumeMountsForClusterChecksRunner(dda *datadoghqv1alpha1.DatadogAgent) []corev1.VolumeMount {
	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "s6-run",
			MountPath: "/var/run/s6",
		},
		{
			Name:      "remove-corechecks",
			MountPath: fmt.Sprintf("%s/%s", datadoghqv1alpha1.ConfigVolumePath, "conf.d"),
		},
	}
	if dda.Spec.ClusterChecksRunner.CustomConfig != nil {
		volumeMount := getVolumeMountFromCustomConfigSpec(dda.Spec.ClusterChecksRunner.CustomConfig, datadoghqv1alpha1.AgentCustomConfigVolumeName, datadoghqv1alpha1.AgentCustomConfigVolumePath, datadoghqv1alpha1.AgentCustomConfigVolumeSubPath)
		volumeMounts = append(volumeMounts, volumeMount)
	}
	return append(volumeMounts, dda.Spec.ClusterChecksRunner.Config.VolumeMounts...)
}

func getClusterChecksRunnerCustomConfigConfigMapName(dda *datadoghqv1alpha1.DatadogAgent) string {
	return fmt.Sprintf("%s-runner-datadog-yaml", dda.Name)
}
