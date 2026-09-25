// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogsyntheticsprivatelocation

import (
	"context"
	"fmt"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
)

const (
	statusProbesPort int32 = 8080

	configVolumeName      = "worker-config"
	configVolumeMountPath = "/etc/datadog"
	runVolumeName         = "run"
	runVolumeMountPath    = "/run"

	enableStatusProbesEnvVar = "DATADOG_WORKER_ENABLE_STATUS_PROBES"
	statusProbesPortEnvVar   = "DATADOG_WORKER_STATUS_PROBES_PORT"
)

func statusProbesEnabled(instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation) bool {
	return featureutils.HasFeatureEnableAnnotation(instance, datadoghqv1alpha1.DatadogSPLStatusProbesEnabledAnnotation)
}

func buildDeployment(instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation, saName string) *appsv1.Deployment {
	w := &datadoghqv1alpha1.DatadogSPLWorker{}
	if instance.Spec.Worker != nil {
		w = instance.Spec.Worker
	}

	image, pullPolicy := workerImage(instance)

	volumes := make([]corev1.Volume, 0, 2+len(w.ExtraVolumes))
	volumes = append(volumes,
		corev1.Volume{
			Name: configVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: configSecretName(instance),
				},
			},
		},
		corev1.Volume{
			Name: runVolumeName,
			VolumeSource: corev1.VolumeSource{
				// /run is required by s6-overlay and must always be writable.
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	)
	volumes = append(volumes, w.ExtraVolumes...)

	volumeMounts := make([]corev1.VolumeMount, 0, 2+len(w.ExtraVolumeMounts))
	volumeMounts = append(volumeMounts,
		corev1.VolumeMount{Name: configVolumeName, MountPath: configVolumeMountPath},
		corev1.VolumeMount{Name: runVolumeName, MountPath: runVolumeMountPath},
	)
	volumeMounts = append(volumeMounts, w.ExtraVolumeMounts...)

	env := make([]corev1.EnvVar, 0, len(w.Env)+2)
	env = append(env, w.Env...)

	probesEnabled := statusProbesEnabled(instance)
	if probesEnabled {
		// The worker only opens the status endpoints when this env var is set,
		// so the env var and the probes must stay in sync.
		env = append(env,
			corev1.EnvVar{
				Name:  enableStatusProbesEnvVar,
				Value: strconv.FormatBool(true),
			},
			corev1.EnvVar{
				Name:  statusProbesPortEnvVar,
				Value: strconv.Itoa(int(statusProbesPort)),
			},
		)
	}

	var livenessProbe, readinessProbe *corev1.Probe
	if probesEnabled {
		port := intstr.FromInt32(statusProbesPort)
		livenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/liveness",
					Port:   port,
					Scheme: corev1.URISchemeHTTP,
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       60,
			TimeoutSeconds:      10,
		}
		readinessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path:   "/readiness",
					Port:   port,
					Scheme: corev1.URISchemeHTTP,
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       10,
			TimeoutSeconds:      2,
		}
	}

	var imagePullSecrets []corev1.LocalObjectReference
	if w.Image != nil {
		imagePullSecrets = append(imagePullSecrets, w.Image.PullSecrets...)
	}
	imagePullSecrets = append(imagePullSecrets, w.ImagePullSecrets...)

	dnsPolicy := corev1.DNSClusterFirst
	if w.DNSPolicy != nil {
		dnsPolicy = *w.DNSPolicy
	}

	priorityClassName := ""
	if w.PriorityClassName != nil {
		priorityClassName = *w.PriorityClassName
	}

	deploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
			Labels:    splLabels(instance),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr.To[int32](1),
			Selector: &metav1.LabelSelector{
				MatchLabels: selectorLabels(instance),
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      podLabels(instance),
					Annotations: w.PodAnnotations,
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: saName,
					NodeSelector:       w.NodeSelector,
					Affinity:           w.Affinity,
					Tolerations:        w.Tolerations,
					PriorityClassName:  priorityClassName,
					HostAliases:        w.HostAliases,
					DNSPolicy:          dnsPolicy,
					DNSConfig:          w.DNSConfig,
					ImagePullSecrets:   imagePullSecrets,
					SecurityContext:    w.PodSecurityContext,
					Containers: []corev1.Container{
						{
							Name:            workerContainerName,
							Image:           image,
							ImagePullPolicy: pullPolicy,
							Env:             env,
							EnvFrom:         w.EnvFrom,
							VolumeMounts:    volumeMounts,
							Resources:       w.Resources,
							SecurityContext: w.SecurityContext,
							LivenessProbe:   livenessProbe,
							ReadinessProbe:  readinessProbe,
						},
					},
					Volumes: volumes,
				},
			},
		},
	}
	if w.Replicas != nil {
		deploy.Spec.Replicas = w.Replicas
	}

	return deploy
}

// reconcileDeployment creates or updates the worker Deployment and returns its
// observed status.
func reconcileDeployment(ctx context.Context, kubeClient client.Client, scheme *runtime.Scheme, instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation, saName string) (*datadoghqv2alpha1.DeploymentStatus, error) {
	logger := ctrl.LoggerFrom(ctx)

	desired := buildDeployment(instance, saName)
	if refErr := controllerutil.SetControllerReference(instance, desired, scheme); refErr != nil {
		return nil, fmt.Errorf("setting owner reference on deployment: %w", refErr)
	}

	nsName := client.ObjectKey{Name: desired.Name, Namespace: desired.Namespace}
	current := &appsv1.Deployment{}
	err := kubeClient.Get(ctx, nsName, current)
	switch {
	case apierrors.IsNotFound(err):
		logger.Info("Creating deployment", "deployment", nsName)
		if createErr := kubeClient.Create(ctx, desired); createErr != nil {
			return nil, fmt.Errorf("creating deployment: %w", createErr)
		}
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("getting deployment: %w", err)
	}

	// Full replacement of spec and labels ensures external drift is reverted.
	// The DeepEqual guard avoids unnecessary writes that would trigger the
	// Owns() watch and cause a reconcile loop.
	if !apiequality.Semantic.DeepEqual(current.Spec, desired.Spec) ||
		!apiequality.Semantic.DeepEqual(current.Labels, desired.Labels) {
		logger.Info("Updating deployment to match desired state", "deployment", nsName)
		current.Spec = desired.Spec
		current.Labels = desired.Labels
		if updateErr := kubeClient.Update(ctx, current); updateErr != nil {
			return nil, fmt.Errorf("updating deployment: %w", updateErr)
		}
	}

	return deploymentStatus(current), nil
}

func deploymentStatus(d *appsv1.Deployment) *datadoghqv2alpha1.DeploymentStatus {
	return &datadoghqv2alpha1.DeploymentStatus{
		DeploymentName:      d.Name,
		Replicas:            d.Status.Replicas,
		UpdatedReplicas:     d.Status.UpdatedReplicas,
		ReadyReplicas:       d.Status.ReadyReplicas,
		AvailableReplicas:   d.Status.AvailableReplicas,
		UnavailableReplicas: d.Status.UnavailableReplicas,
	}
}
