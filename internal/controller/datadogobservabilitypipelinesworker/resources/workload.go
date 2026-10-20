// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"crypto/sha256"
	"encoding/hex"
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
	annotations := maps.Clone(worker.Spec.Annotations)
	if config := worker.Spec.PipelineConfig; config != nil {
		// OPW does not reload the pipeline file, so a changed pipeline must roll the pods.
		checksum := sha256.Sum256([]byte(*config))
		annotations = controllerutils.MergeStringMaps(annotations, map[string]string{pipelineChecksumAnnotation: hex.EncodeToString(checksum[:])})
	}
	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      podLabels(worker),
			Annotations: annotations,
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
		Volumes:                       workerVolumes(worker),
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
		Name:            ContainerName,
		Image:           image.reference,
		ImagePullPolicy: image.pullPolicy,
		Args:            workerArgs(worker),
		Env:             newEnvironment(worker),
		EnvFrom:         spec.EnvFrom,
		Ports:           containerPorts(spec.Ports),
		Resources:       ptr.Deref(spec.Resources, corev1.ResourceRequirements{}),
		VolumeMounts:    controllerutils.MergeVolumeMounts(spec.VolumeMounts, workerVolumeMounts(worker)),
		LivenessProbe:   workerProbe(livenessProbeFailureThreshold),
		ReadinessProbe:  workerProbe(readinessProbeFailureThreshold),
	}
}

// newEnvironment lets user variables override source addresses but not the variables the Worker requires.
func newEnvironment(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) []corev1.EnvVar {
	spec := worker.Spec
	var sources []corev1.EnvVar
	for _, port := range spec.Ports {
		if name, ok := sourceAddressEnvNames[port.Name]; ok {
			sources = append(sources, corev1.EnvVar{Name: name, Value: listenAddress + ":" + strconv.Itoa(int(port.Port))})
		}
	}
	required := []corev1.EnvVar{
		{Name: envDatadogAPIKey, ValueFrom: &corev1.EnvVarSource{SecretKeyRef: spec.Datadog.APIKeySecretRef}},
		{Name: envPipelineID, Value: pipelineID(worker)},
		{Name: envDatadogSite, Value: *spec.Datadog.Site},
		{Name: envDataDirectory, Value: DataDirectory},
		{Name: envAPIEnabled, Value: "true"},
		{Name: envAPIAddress, Value: listenAddress + ":" + strconv.Itoa(int(APIPort))},
		{Name: envGracefulShutdownLimitSeconds, Value: strconv.FormatInt(max(minGracefulShutdownLimitSeconds, *spec.TerminationGracePeriodSeconds-gracefulShutdownMarginSeconds), 10)},
	}
	return controllerutils.MergeEnv(controllerutils.MergeEnv(sources, spec.Env), required)
}

func workerArgs(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) []string {
	if worker.Spec.PipelineConfig == nil {
		return []string{"run"}
	}
	// --skip-key-validation stops startup from checking the pipeline ID against Remote Config.
	return []string{"run", "--pipeline-config=" + pipelineConfigDir + "/" + pipelineConfigFileName, "--skip-key-validation"}
}

// pipelineID falls back to the Worker UID in local mode, which is a UUID unique to the Worker.
func pipelineID(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) string {
	if worker.Spec.PipelineID != nil {
		return *worker.Spec.PipelineID
	}
	return string(worker.UID)
}

func workerVolumes(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) []corev1.Volume {
	volumes := dataVolumes(worker.Spec.Storage, worker.Spec.Volumes)
	if worker.Spec.PipelineConfig == nil {
		return volumes
	}
	return controllerutils.MergeVolumes(volumes, []corev1.Volume{{
		Name: pipelineConfigVolumeName,
		VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: pipelineConfigMapName(worker)},
		}},
	}})
}

func workerVolumeMounts(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) []corev1.VolumeMount {
	mounts := []corev1.VolumeMount{{Name: DataVolumeName, MountPath: DataDirectory}}
	if worker.Spec.PipelineConfig != nil {
		mounts = append(mounts, corev1.VolumeMount{Name: pipelineConfigVolumeName, MountPath: pipelineConfigDir, ReadOnly: true})
	}
	return mounts
}

func containerPorts(ports []datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerPort) []corev1.ContainerPort {
	result := make([]corev1.ContainerPort, 0, len(ports)+1)
	for _, port := range ports {
		result = append(result, corev1.ContainerPort{Name: port.Name, ContainerPort: port.Port, Protocol: portProtocol(port)})
	}
	return append(result, corev1.ContainerPort{Name: APIPortName, ContainerPort: APIPort, Protocol: corev1.ProtocolTCP})
}

func workerProbe(failureThreshold int32) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler:        corev1.ProbeHandler{TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt32(APIPort)}},
		InitialDelaySeconds: probeInitialDelaySeconds,
		TimeoutSeconds:      probeTimeoutSeconds,
		PeriodSeconds:       probePeriodSeconds,
		SuccessThreshold:    probeSuccessThreshold,
		FailureThreshold:    failureThreshold,
	}
}

func dataVolumes(storage *datadoghqv1alpha1.DatadogBYOCClusterStorageSpec, additional []corev1.Volume) []corev1.Volume {
	if storage == nil || storage.EmptyDir == nil {
		return additional
	}
	return controllerutils.MergeVolumes(additional, []corev1.Volume{{Name: DataVolumeName, VolumeSource: corev1.VolumeSource{EmptyDir: storage.EmptyDir}}})
}
