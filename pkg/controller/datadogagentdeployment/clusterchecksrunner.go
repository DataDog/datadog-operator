// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagentdeployment

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

func (r *ReconcileDatadogAgentDeployment) reconcileClusterChecksRunner(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	result, err := r.manageClusterChecksRunnerDependencies(logger, dad, newStatus)
	if shouldReturn(result, err) {
		return result, err
	}

	if !needClusterChecksRunner(dad) {
		return r.cleanupClusterChecksRunner(logger, dad, newStatus)
	}

	if newStatus.ClusterChecksRunner != nil &&
		newStatus.ClusterChecksRunner.DeploymentName != "" &&
		newStatus.ClusterChecksRunner.DeploymentName != getClusterChecksRunnerName(dad) {
		return result, fmt.Errorf("Datadog cluster checks runner Deployment cannot be renamed once created")
	}

	nsName := types.NamespacedName{
		Name:      getClusterChecksRunnerName(dad),
		Namespace: dad.Namespace,
	}
	// ClusterChecksRunnerDeployment attached to this instance
	ClusterChecksRunnerDeployment := &appsv1.Deployment{}
	if needClusterChecksRunner(dad) {
		err := r.client.Get(context.TODO(), nsName, ClusterChecksRunnerDeployment)
		if err != nil {
			if errors.IsNotFound(err) {
				logger.Info("ClusterChecksRunner deployment not found", "name", nsName.Name, "namespace", nsName.Namespace)
				// Create and attach a ClusterChecksRunner Deployment
				var result reconcile.Result
				result, err = r.createNewClusterChecksRunnerDeployment(logger, dad, newStatus)
				return result, err
			}
			return reconcile.Result{}, err
		}

		result, err := r.updateClusterChecksRunnerDeployment(logger, dad, ClusterChecksRunnerDeployment, newStatus)
		return result, err
	}
	return reconcile.Result{}, nil
}

func needClusterChecksRunner(dad *datadoghqv1alpha1.DatadogAgentDeployment) bool {
	if dad.Spec.ClusterAgent != nil && datadoghqv1alpha1.BoolValue(dad.Spec.ClusterAgent.Config.ClusterChecksRunnerEnabled) {
		return true
	}

	return false
}

func (r *ReconcileDatadogAgentDeployment) createNewClusterChecksRunnerDeployment(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgentDeployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	newDCAW, hash, err := newClusterChecksRunnerDeploymentFromInstance(logger, agentdeployment, newStatus, nil)
	if err != nil {
		return reconcile.Result{}, err
	}

	// Set ClusterChecksRunner Deployment instance as the owner and controller
	if err = controllerutil.SetControllerReference(agentdeployment, newDCAW, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.Info("Creating a new Cluster Checks Runner Deployment", "deployment.Namespace", newDCAW.Namespace, "deployment.Name", newDCAW.Name, "agentdeployment.Status.ClusterAgent.CurrentHash", hash)
	newStatus.ClusterChecksRunner = &datadoghqv1alpha1.DatadogAgentDeploymentDeploymentStatus{}
	err = r.client.Create(context.TODO(), newDCAW)
	now := metav1.NewTime(time.Now())
	if err != nil {
		updateStatusWithClusterChecksRunner(nil, newStatus, &now)
		return reconcile.Result{}, err
	}

	updateStatusWithClusterChecksRunner(newDCAW, newStatus, &now)
	eventInfo := buildEventInfo(newDCAW.Name, newDCAW.Namespace, deploymentKind, datadog.CreationEvent)
	r.recordEvent(agentdeployment, eventInfo)
	return reconcile.Result{}, nil
}

func updateStatusWithClusterChecksRunner(dcaw *appsv1.Deployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus, updateTime *metav1.Time) {
	newStatus.ClusterChecksRunner = updateDeploymentStatus(dcaw, newStatus.ClusterChecksRunner, updateTime)
}

func (r *ReconcileDatadogAgentDeployment) updateClusterChecksRunnerDeployment(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgentDeployment, dep *appsv1.Deployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	newDCAW, hash, err := newClusterChecksRunnerDeploymentFromInstance(logger, agentdeployment, newStatus, dep.Spec.Selector)
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

	// Set DatadogAgentDeployment instance  instance as the owner and controller
	if err = controllerutil.SetControllerReference(agentdeployment, dep, r.scheme); err != nil {
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
	eventInfo := buildEventInfo(updateDca.Name, updateDca.Namespace, deploymentKind, datadog.UpdateEvent)
	r.recordEvent(agentdeployment, eventInfo)
	updateStatusWithClusterChecksRunner(updateDca, newStatus, &now)
	return reconcile.Result{}, nil
}

// newClusterAgentDeploymentFromInstance creates a Cluster Agent Deployment from a given DatadogAgentDeployment
func newClusterChecksRunnerDeploymentFromInstance(logger logr.Logger,
	agentdeployment *datadoghqv1alpha1.DatadogAgentDeployment,
	newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus,
	selector *metav1.LabelSelector) (*appsv1.Deployment, string, error) {
	labels := map[string]string{
		datadoghqv1alpha1.AgentDeploymentNameLabelKey:      agentdeployment.Name,
		datadoghqv1alpha1.AgentDeploymentComponentLabelKey: datadoghqv1alpha1.DefaultClusterChecksRunnerResourceSuffix,
	}
	for key, val := range agentdeployment.Labels {
		labels[key] = val
	}
	for key, val := range getDefaultLabels(agentdeployment, datadoghqv1alpha1.DefaultClusterChecksRunnerResourceSuffix, getClusterChecksRunnerVersion(agentdeployment)) {
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
				datadoghqv1alpha1.AgentDeploymentComponentLabelKey: datadoghqv1alpha1.DefaultClusterChecksRunnerResourceSuffix,
			},
		}
	}

	annotations := map[string]string{}
	for key, val := range agentdeployment.Annotations {
		annotations[key] = val
	}

	dca := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:        getClusterChecksRunnerName(agentdeployment),
			Namespace:   agentdeployment.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: appsv1.DeploymentSpec{
			Template: newClusterChecksRunnerPodTemplate(logger, agentdeployment, labels, annotations),
			Replicas: agentdeployment.Spec.ClusterChecksRunner.Replicas,
			Selector: selector,
		},
	}
	hash, err := comparison.SetMD5GenerationAnnotation(&dca.ObjectMeta, agentdeployment.Spec.ClusterAgent)
	return dca, hash, err
}

func (r *ReconcileDatadogAgentDeployment) manageClusterChecksRunnerDependencies(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	result, err := r.manageClusterChecksRunnerPDB(logger, dad, newStatus)
	if shouldReturn(result, err) {
		return result, err
	}
	result, err = r.manageClusterChecksRunnerRBACs(logger, dad)
	if shouldReturn(result, err) {
		return result, err
	}
	return reconcile.Result{}, nil
}

func (r *ReconcileDatadogAgentDeployment) cleanupClusterChecksRunner(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	nsName := types.NamespacedName{
		Name:      getClusterChecksRunnerName(dad),
		Namespace: dad.Namespace,
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
	eventInfo := buildEventInfo(ClusterChecksRunnerDeployment.Name, ClusterChecksRunnerDeployment.Namespace, deploymentKind, datadog.DeletionEvent)
	r.recordEvent(dad, eventInfo)
	if err := r.client.Delete(context.TODO(), ClusterChecksRunnerDeployment); err != nil {
		return reconcile.Result{}, err
	}
	newStatus.ClusterChecksRunner = nil
	return reconcile.Result{Requeue: true}, nil
}

// newClusterChecksRunnerPodTemplate generates a PodTemplate from a DatadogClusterChecksRunnerDeployment spec
func newClusterChecksRunnerPodTemplate(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgentDeployment, labels, annotations map[string]string) corev1.PodTemplateSpec {
	// copy Spec to configure the Cluster Checks Runner Pod Template
	ClusterChecksRunnerSpec := agentdeployment.Spec.ClusterChecksRunner.DeepCopy()

	newPodTemplate := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: labels,
		},
		Spec: corev1.PodSpec{
			ServiceAccountName: getClusterChecksRunnerServiceAccount(agentdeployment),
			Containers: []corev1.Container{
				{
					Name:            "cluster-checks-runner",
					Image:           ClusterChecksRunnerSpec.Image.Name,
					ImagePullPolicy: *ClusterChecksRunnerSpec.Image.PullPolicy,
					Env:             getEnvVarsForClusterChecksRunner(logger, agentdeployment),
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "s6-run",
							MountPath: "/var/run/s6",
						},
						{
							Name:      "remove-corechecks",
							MountPath: fmt.Sprintf("%s/%s", datadoghqv1alpha1.ConfigVolumePath, "conf.d"),
						},
					},
					LivenessProbe: getDefaultLivenessProbe(),
				},
			},
			Volumes: []corev1.Volume{
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
			},
			Affinity:    getPodAffinity(ClusterChecksRunnerSpec.Affinity, getClusterChecksRunnerName(agentdeployment)),
			Tolerations: ClusterChecksRunnerSpec.Tolerations,
		},
	}

	if ClusterChecksRunnerSpec.Config.Resources != nil {
		newPodTemplate.Spec.Containers[0].Resources = *ClusterChecksRunnerSpec.Config.Resources
	}

	return newPodTemplate
}

// getEnvVarsForClusterChecksRunner converts Cluster Checks Runner Config into container env vars
func getEnvVarsForClusterChecksRunner(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment) []corev1.EnvVar {
	spec := &dad.Spec
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
			Name:  datadoghqv1alpha1.DDddURL,
			Value: *spec.Agent.Config.DDUrl,
		},
		{
			Name:  datadoghqv1alpha1.DDClusterChecksRunnerEnabled,
			Value: strconv.FormatBool(*spec.ClusterAgent.Config.ClusterChecksRunnerEnabled),
		},
		{
			Name:  datadoghqv1alpha1.DDClusterAgentKubeServiceName,
			Value: getClusterAgentServiceName(dad),
		},
		{
			Name:      datadoghqv1alpha1.DDClusterAgentAuthToken,
			ValueFrom: getClusterAgentAuthToken(dad),
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
	}

	if spec.ClusterChecksRunner.Config.LogLevel != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDLogLevel,
			Value: *spec.ClusterChecksRunner.Config.LogLevel,
		})
	}

	if spec.Credentials.APIKeyExistingSecret != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:      datadoghqv1alpha1.DDAPIKey,
			ValueFrom: getAPIKeyFromSecret(dad),
		})
	} else {
		envVars = append(envVars, corev1.EnvVar{
			Name:  datadoghqv1alpha1.DDAPIKey,
			Value: spec.Credentials.APIKey,
		})
	}

	return append(envVars, spec.Agent.Config.Env...)
}

func getClusterChecksRunnerVersion(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	// TODO implement this method
	return ""
}

func getClusterChecksRunnerName(dad *datadoghqv1alpha1.DatadogAgentDeployment) string {
	if dad.Spec.ClusterChecksRunner != nil && dad.Spec.ClusterChecksRunner.DeploymentName != "" {
		return dad.Spec.ClusterChecksRunner.DeploymentName
	}
	return fmt.Sprintf("%s-%s", dad.Name, "cluster-checks-runner")
}
