// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package resources builds the Kubernetes resources managed by a
// DatadogObservabilityPipelinesWorker controller.
package resources

import (
	"fmt"
	"maps"
	"slices"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	controllerutils "github.com/DataDog/datadog-operator/internal/controller/utils"
)

const (
	workerContainerName = "worker"
	dataVolumeName      = "data"
	dataDirectory       = "/var/lib/observability-pipelines-worker"

	workerAPIPort int32 = 8686
)

// Resources is the complete set of Kubernetes resources rendered for a Worker.
type Resources struct {
	Service             *corev1.Service
	ServiceAccount      *corev1.ServiceAccount
	StatefulSet         *appsv1.StatefulSet
	HPA                 *autoscalingv2.HorizontalPodAutoscaler
	PodDisruptionBudget *policyv1.PodDisruptionBudget
}

// Objects returns the resources in apply order.
func (r *Resources) Objects() []client.Object {
	objects := make([]client.Object, 0, 5)
	if r.ServiceAccount != nil {
		objects = append(objects, r.ServiceAccount)
	}
	if r.Service != nil {
		objects = append(objects, r.Service)
	}
	objects = append(objects, r.StatefulSet)
	if r.HPA != nil {
		objects = append(objects, r.HPA)
	}
	if r.PodDisruptionBudget != nil {
		objects = append(objects, r.PodDisruptionBudget)
	}
	return objects
}

// BuildResources builds deterministic Kubernetes resources for a Worker after
// the remote pipeline phase has resolved a pipeline ID.
func BuildResources(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker, pipelineID string) (*Resources, error) {
	image, pullPolicy, pullSecrets, err := resolveImage(worker.Spec.Image)
	if err != nil {
		return nil, err
	}

	name := worker.Name
	selector := selectorLabels(name)
	labels := controllerutils.MergeStringMaps(worker.Spec.Labels, selector)
	metadata := metav1.ObjectMeta{
		Name:        name,
		Namespace:   worker.Namespace,
		Labels:      labels,
		Annotations: maps.Clone(worker.Spec.Annotations),
	}

	serviceAccountName := name
	var serviceAccount *corev1.ServiceAccount
	if worker.Spec.Identity != nil && worker.Spec.Identity.ServiceAccountName != nil && *worker.Spec.Identity.ServiceAccountName != "" {
		serviceAccountName = *worker.Spec.Identity.ServiceAccountName
	} else {
		serviceAccount = &corev1.ServiceAccount{
			ObjectMeta:                   *metadata.DeepCopy(),
			AutomountServiceAccountToken: new(false),
		}
	}

	ports := servicePorts(worker.Spec.Ports)
	var service *corev1.Service
	if len(ports) > 0 {
		serviceType := corev1.ServiceTypeClusterIP
		if worker.Spec.Service != nil {
			if worker.Spec.Service.Type != "" {
				serviceType = worker.Spec.Service.Type
			}
		}
		service = newService(metadata, selector, ports, serviceType)
	}

	replicas := ptr.Deref(worker.Spec.Replicas, int32(2))
	terminationGracePeriodSeconds := ptr.Deref(worker.Spec.TerminationGracePeriodSeconds, int64(70))
	podSpec := corev1.PodSpec{
		ServiceAccountName:            serviceAccountName,
		DNSPolicy:                     corev1.DNSClusterFirst,
		ImagePullSecrets:              pullSecrets,
		InitContainers:                worker.Spec.InitContainers,
		Containers:                    []corev1.Container{newWorkerContainer(worker, pipelineID, image, pullPolicy, terminationGracePeriodSeconds)},
		Volumes:                       dataVolumes(worker.Spec.Storage, worker.Spec.Volumes),
		NodeSelector:                  maps.Clone(worker.Spec.NodeSelector),
		Affinity:                      worker.Spec.Affinity.DeepCopy(),
		Tolerations:                   slices.Clone(worker.Spec.Tolerations),
		TopologySpreadConstraints:     worker.Spec.TopologySpreadConstraints,
		TerminationGracePeriodSeconds: new(terminationGracePeriodSeconds),
	}

	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: *metadata.DeepCopy(),
		Spec: appsv1.StatefulSetSpec{
			Replicas:            new(replicas),
			ServiceName:         name,
			PodManagementPolicy: appsv1.ParallelPodManagement,
			Selector:            &metav1.LabelSelector{MatchLabels: maps.Clone(selector)},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      maps.Clone(labels),
					Annotations: maps.Clone(worker.Spec.Annotations),
				},
				Spec: podSpec,
			},
			VolumeClaimTemplates: volumeClaimTemplates(worker.Spec.Storage),
		},
	}
	podDisruptionBudget, err := newPodDisruptionBudget(metadata, selector, worker.Spec.PodDisruptionBudget)
	if err != nil {
		return nil, err
	}

	resources := &Resources{
		Service:             service,
		ServiceAccount:      serviceAccount,
		StatefulSet:         statefulSet,
		PodDisruptionBudget: podDisruptionBudget,
	}
	if worker.Spec.Autoscaling != nil {
		statefulSet.Spec.Replicas = nil
		resources.HPA, err = newHPA(metadata, worker.Spec.Autoscaling)
		if err != nil {
			return nil, err
		}
	}
	return resources, nil
}

func resolveImage(spec *datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerImageSpec) (string, corev1.PullPolicy, []corev1.LocalObjectReference, error) {
	if spec == nil {
		return "", "", nil, fmt.Errorf("worker image is required")
	}
	image := datadoghqv1alpha1.DatadogBYOCImageSpec(*spec)
	if image.Repository == nil || *image.Repository == "" {
		return "", "", nil, fmt.Errorf("worker image repository is required")
	}
	if (image.Tag == nil) == (image.Digest == nil) {
		return "", "", nil, fmt.Errorf("worker image must specify exactly one of tag or digest")
	}

	reference := *image.Repository
	if image.Digest != nil {
		reference += "@" + *image.Digest
	} else {
		reference += ":" + *image.Tag
	}
	return reference, ptr.Deref(image.PullPolicy, corev1.PullIfNotPresent), slices.Clone(image.ImagePullSecrets), nil
}

func newService(metadata metav1.ObjectMeta, selector map[string]string, ports []corev1.ServicePort, serviceType corev1.ServiceType) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: *metadata.DeepCopy(),
		Spec: corev1.ServiceSpec{
			Type:     serviceType,
			Selector: maps.Clone(selector),
			Ports:    slices.Clone(ports),
		},
	}
}

func servicePorts(ports []datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerPort) []corev1.ServicePort {
	result := make([]corev1.ServicePort, 0, len(ports))
	for _, port := range ports {
		protocol := port.Protocol
		if protocol == "" {
			protocol = corev1.ProtocolTCP
		}
		result = append(result, corev1.ServicePort{
			Name:       port.Name,
			Port:       port.Port,
			Protocol:   protocol,
			TargetPort: intstr.FromInt32(port.Port),
		})
	}
	return result
}

func containerPorts(ports []datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerPort) []corev1.ContainerPort {
	result := make([]corev1.ContainerPort, 0, len(ports)+1)
	for _, port := range ports {
		protocol := port.Protocol
		if protocol == "" {
			protocol = corev1.ProtocolTCP
		}
		result = append(result, corev1.ContainerPort{Name: port.Name, ContainerPort: port.Port, Protocol: protocol})
	}
	return append(result, corev1.ContainerPort{Name: "api", ContainerPort: workerAPIPort, Protocol: corev1.ProtocolTCP})
}

func newWorkerContainer(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker, pipelineID, image string, pullPolicy corev1.PullPolicy, terminationGracePeriodSeconds int64) corev1.Container {
	requiredEnvironment := []corev1.EnvVar{
		{Name: "DD_API_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: worker.Spec.Datadog.APIKeySecretRef.DeepCopy()}},
		{Name: "DD_OP_PIPELINE_ID", Value: pipelineID},
		{Name: "DD_SITE", Value: *worker.Spec.Datadog.Site},
		{Name: "DD_OP_DATA_DIR", Value: dataDirectory},
		{Name: "DD_OP_API_ENABLED", Value: "true"},
		{Name: "DD_OP_API_ADDRESS", Value: "0.0.0.0:" + strconv.Itoa(int(workerAPIPort))},
		{Name: "DD_OP_GRACEFUL_SHUTDOWN_LIMIT_SECS", Value: strconv.FormatInt(max(10, terminationGracePeriodSeconds-10), 10)},
	}
	return corev1.Container{
		Name:            workerContainerName,
		Image:           image,
		ImagePullPolicy: pullPolicy,
		Args:            []string{"run"},
		Env:             controllerutils.MergeEnv(worker.Spec.Env, requiredEnvironment),
		EnvFrom:         slices.Clone(worker.Spec.EnvFrom),
		Ports:           containerPorts(worker.Spec.Ports),
		Resources:       ptr.Deref(worker.Spec.Resources, corev1.ResourceRequirements{}),
		VolumeMounts:    dataVolumeMounts(worker.Spec.VolumeMounts),
		LivenessProbe:   workerProbe(5),
		ReadinessProbe:  workerProbe(3),
	}
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
	volumes := slices.Clone(additional)
	if storage == nil || storage.EmptyDir == nil {
		return volumes
	}
	return controllerutils.MergeVolumes(volumes, []corev1.Volume{{Name: dataVolumeName, VolumeSource: corev1.VolumeSource{EmptyDir: storage.EmptyDir.DeepCopy()}}})
}

func dataVolumeMounts(additional []corev1.VolumeMount) []corev1.VolumeMount {
	return controllerutils.MergeVolumeMounts(additional, []corev1.VolumeMount{{Name: dataVolumeName, MountPath: dataDirectory}})
}

func volumeClaimTemplates(storage *datadoghqv1alpha1.DatadogBYOCClusterStorageSpec) []corev1.PersistentVolumeClaim {
	if storage == nil || storage.VolumeClaimTemplate == nil {
		return nil
	}
	template := storage.VolumeClaimTemplate
	return []corev1.PersistentVolumeClaim{{
		TypeMeta: template.TypeMeta,
		ObjectMeta: metav1.ObjectMeta{
			Name:        dataVolumeName,
			Labels:      maps.Clone(template.Metadata.Labels),
			Annotations: maps.Clone(template.Metadata.Annotations),
		},
		Spec: *template.Spec.DeepCopy(),
	}}
}

func newHPA(metadata metav1.ObjectMeta, autoscaling *datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec) (*autoscalingv2.HorizontalPodAutoscaler, error) {
	if autoscaling.MaxReplicas == nil {
		return nil, fmt.Errorf("worker autoscaling maxReplicas is required")
	}
	return &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: *metadata.DeepCopy(),
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{APIVersion: "apps/v1", Kind: "StatefulSet", Name: metadata.Name},
			MinReplicas:    autoscaling.MinReplicas,
			MaxReplicas:    *autoscaling.MaxReplicas,
			Metrics:        slices.Clone(autoscaling.Metrics),
			Behavior:       autoscaling.Behavior.DeepCopy(),
		},
	}, nil
}

func newPodDisruptionBudget(metadata metav1.ObjectMeta, selector map[string]string, spec *datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec) (*policyv1.PodDisruptionBudget, error) {
	if spec != nil && spec.MinAvailable == nil && spec.MaxUnavailable == nil {
		return nil, nil
	}
	if spec != nil && spec.MinAvailable != nil && spec.MaxUnavailable != nil {
		return nil, fmt.Errorf("worker pod disruption budget minAvailable and maxUnavailable are mutually exclusive")
	}
	budget := &policyv1.PodDisruptionBudget{
		ObjectMeta: *metadata.DeepCopy(),
		Spec: policyv1.PodDisruptionBudgetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: maps.Clone(selector)},
		},
	}
	if spec == nil {
		budget.Spec.MaxUnavailable = new(intstr.FromInt32(1))
	} else {
		budget.Spec.MinAvailable = spec.MinAvailable
		budget.Spec.MaxUnavailable = spec.MaxUnavailable
	}
	return budget, nil
}

func selectorLabels(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     "observability-pipelines-worker",
		"app.kubernetes.io/instance": name,
	}
}
