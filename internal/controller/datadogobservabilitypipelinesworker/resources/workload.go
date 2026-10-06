// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"maps"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	controllerutils "github.com/DataDog/datadog-operator/internal/controller/utils"
)

func newPodTemplate(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker, image workerImage) corev1.PodTemplateSpec {
	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      podLabels(worker),
			Annotations: maps.Clone(worker.Spec.Annotations),
		},
		Spec: newPodSpec(worker, image),
	}
}

func newPodSpec(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker, image workerImage) corev1.PodSpec {
	spec := worker.Spec
	serviceAccountName, _ := serviceAccountName(worker)
	return corev1.PodSpec{
		ServiceAccountName:            serviceAccountName,
		DNSPolicy:                     corev1.DNSClusterFirst,
		ImagePullSecrets:              image.pullSecrets,
		InitContainers:                spec.InitContainers,
		Containers:                    []corev1.Container{newWorkerContainer(worker, image)},
		Volumes:                       dataVolumes(spec.Storage, spec.Volumes),
		NodeSelector:                  spec.NodeSelector,
		Affinity:                      spec.Affinity,
		Tolerations:                   spec.Tolerations,
		TopologySpreadConstraints:     spec.TopologySpreadConstraints,
		TerminationGracePeriodSeconds: spec.TerminationGracePeriodSeconds,
	}
}

func newWorkerContainer(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker, image workerImage) corev1.Container {
	spec := worker.Spec
	return corev1.Container{
		Name:            workerContainerName,
		Image:           image.reference,
		ImagePullPolicy: image.pullPolicy,
		Args:            []string{"run"},
		Env:             newEnvironment(worker),
		EnvFrom:         spec.EnvFrom,
		Ports:           containerPorts(spec.Ports),
		Resources:       ptr.Deref(spec.Resources, corev1.ResourceRequirements{}),
		VolumeMounts:    controllerutils.MergeVolumeMounts(spec.VolumeMounts, []corev1.VolumeMount{{Name: dataVolumeName, MountPath: dataDirectory}}),
		LivenessProbe:   workerProbe(5),
		ReadinessProbe:  workerProbe(3),
	}
}

// newEnvironment lets user variables override source addresses but not the variables the Worker requires.
func newEnvironment(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) []corev1.EnvVar {
	spec := worker.Spec
	var sources []corev1.EnvVar
	for _, port := range spec.Ports {
		if name, ok := sourceAddressEnvNames[port.Name]; ok {
			sources = append(sources, corev1.EnvVar{Name: name, Value: "0.0.0.0:" + strconv.Itoa(int(port.Port))})
		}
	}
	required := []corev1.EnvVar{
		{Name: "DD_API_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: spec.Datadog.APIKeySecretRef}},
		{Name: "DD_OP_PIPELINE_ID", Value: *spec.PipelineID},
		{Name: "DD_SITE", Value: *spec.Datadog.Site},
		{Name: "DD_OP_DATA_DIR", Value: dataDirectory},
		{Name: "DD_OP_API_ENABLED", Value: "true"},
		{Name: "DD_OP_API_ADDRESS", Value: "0.0.0.0:" + strconv.Itoa(int(workerAPIPort))},
		{Name: "DD_OP_GRACEFUL_SHUTDOWN_LIMIT_SECS", Value: strconv.FormatInt(max(10, *spec.TerminationGracePeriodSeconds-10), 10)},
	}
	return controllerutils.MergeEnv(controllerutils.MergeEnv(sources, spec.Env), required)
}

func containerPorts(ports []datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerPort) []corev1.ContainerPort {
	result := make([]corev1.ContainerPort, 0, len(ports)+1)
	for _, port := range ports {
		result = append(result, corev1.ContainerPort{Name: port.Name, ContainerPort: port.Port, Protocol: portProtocol(port)})
	}
	return append(result, corev1.ContainerPort{Name: "api", ContainerPort: workerAPIPort, Protocol: corev1.ProtocolTCP})
}

func workerProbe(failureThreshold int32) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler:        corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt32(workerAPIPort)}},
		InitialDelaySeconds: 15,
		TimeoutSeconds:      15,
		PeriodSeconds:       10,
		SuccessThreshold:    1,
		FailureThreshold:    failureThreshold,
	}
}

func dataVolumes(storage *datadoghqv1alpha1.DatadogBYOCClusterStorageSpec, additional []corev1.Volume) []corev1.Volume {
	if storage == nil || storage.EmptyDir == nil {
		return additional
	}
	return controllerutils.MergeVolumes(additional, []corev1.Volume{{Name: dataVolumeName, VolumeSource: corev1.VolumeSource{EmptyDir: storage.EmptyDir}}})
}
