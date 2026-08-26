// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"fmt"
	"maps"
	"slices"

	"github.com/imdario/mergo"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	byocrelease "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/release"
)

const (
	configChecksumAnnotation   = "checksum/config"
	defaultClusterDomain       = "cluster.local"
	nodeConfigMountPath        = "/quickwit/" + nodeConfigFileName
	defaultDataVolumeMountPath = "/quickwit/qwdata"
)

// workloadInput contains the domain inputs used to resolve a component workload.
type workloadInput struct {
	Cluster  *datadoghqv1alpha1.DatadogBYOCCluster
	Release  *byocrelease.ResolvedRelease
	Checksum string
	Name     string
	Spec     *datadoghqv1alpha1.DatadogBYOCClusterComponentSpec
	Defaults workloadDefaults
}

// workloadDefaults contains component defaults expressed with Kubernetes fields.
type workloadDefaults struct {
	ServicePorts []corev1.ServicePort
	PodSpec      corev1.PodSpec
}

// workloadValues contains the resolved Kubernetes values shared by Deployments and StatefulSets.
type workloadValues struct {
	Metadata metav1.ObjectMeta
	Replicas int32
	Selector map[string]string
	Template corev1.PodTemplateSpec
	Service  serviceValues
}

func resolveWorkloadValues(input workloadInput) (workloadValues, error) {
	template, err := resolvePodTemplateSpec(input)
	if err != nil {
		return workloadValues{}, err
	}

	selector := selectorLabels(input.Cluster, input.Name)
	componentLabel := map[string]string{"app.kubernetes.io/component": input.Name}
	componentLabels := labels(input.Cluster, componentLabel, input.Spec.Labels)
	resourceName := ComponentResourceName(input.Cluster.Name, input.Name)

	return workloadValues{
		Metadata: metav1.ObjectMeta{
			Name:        resourceName,
			Namespace:   input.Cluster.Namespace,
			Labels:      labels(input.Cluster, componentLabel, input.Spec.Labels, selector),
			Annotations: annotations(input.Cluster, input.Spec.Annotations),
		},
		Replicas: *input.Spec.Replicas,
		Selector: selector,
		Template: template,
		Service: serviceValues{
			Metadata: metav1.ObjectMeta{
				Name:        resourceName,
				Namespace:   input.Cluster.Namespace,
				Labels:      componentLabels,
				Annotations: annotations(input.Cluster),
			},
			Selector: selector,
			Ports:    slices.Clone(input.Defaults.ServicePorts),
		},
	}, nil
}

func resolvePodTemplateSpec(input workloadInput) (corev1.PodTemplateSpec, error) {
	podSpec, err := resolvePodSpec(input)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}

	componentLabel := map[string]string{"app.kubernetes.io/component": input.Name}
	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      labels(input.Cluster, componentLabel, input.Spec.Labels, selectorLabels(input.Cluster, input.Name)),
			Annotations: annotations(input.Cluster, input.Spec.Annotations, map[string]string{configChecksumAnnotation: input.Checksum}),
		},
		Spec: podSpec,
	}, nil
}

func resolvePodSpec(input workloadInput) (corev1.PodSpec, error) {
	if len(input.Defaults.PodSpec.Containers) != 1 {
		return corev1.PodSpec{}, fmt.Errorf("%s defaults must define exactly one container", input.Name)
	}

	global := input.Cluster.Spec.Global
	affinity, err := resolveAffinity(input.Cluster, input.Name, global.Affinity, input.Spec.Affinity)
	if err != nil {
		return corev1.PodSpec{}, fmt.Errorf("merge %s affinity: %w", input.Name, err)
	}

	serviceAccountName := input.Cluster.Name
	if input.Cluster.Spec.Identity != nil && input.Cluster.Spec.Identity.ServiceAccountName != nil && *input.Cluster.Spec.Identity.ServiceAccountName != "" {
		serviceAccountName = *input.Cluster.Spec.Identity.ServiceAccountName
	}

	volumes := []corev1.Volume{
		{Name: "config", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: input.Cluster.Name}, Items: []corev1.KeyToPath{{Key: nodeConfigFileName, Path: nodeConfigFileName}}}}},
	}
	volumes = append(volumes, slices.Clone(input.Defaults.PodSpec.Volumes)...)
	volumes = append(volumes, slices.Clone(global.Volumes)...)
	volumes = append(volumes, slices.Clone(input.Spec.Volumes)...)

	defaultContainer := input.Defaults.PodSpec.Containers[0].DeepCopy()
	volumeMounts := append(slices.Clone(defaultContainer.VolumeMounts), corev1.VolumeMount{Name: "data", MountPath: defaultDataVolumeMountPath})
	volumeMounts = append(volumeMounts, slices.Clone(global.VolumeMounts)...)
	volumeMounts = append(volumeMounts, slices.Clone(input.Spec.VolumeMounts)...)

	resources := defaultContainer.Resources
	if input.Spec.Resources != nil {
		resources = *input.Spec.Resources.DeepCopy()
	}

	var terminationGracePeriodSeconds *int64
	if input.Spec.TerminationGracePeriodSeconds != nil {
		terminationGracePeriodSeconds = ptr.To(*input.Spec.TerminationGracePeriodSeconds)
	}

	return corev1.PodSpec{
		ServiceAccountName: serviceAccountName,
		SecurityContext:    &corev1.PodSecurityContext{FSGroup: ptr.To[int64](1005)},
		DNSConfig:          &corev1.PodDNSConfig{Options: []corev1.PodDNSConfigOption{{Name: "ndots", Value: ptr.To("1")}}},
		InitContainers:     slices.Clone(input.Spec.InitContainers),
		Containers: []corev1.Container{{
			Name:            appName,
			Image:           input.Release.Release.Images.Pomsky.ImageReference(),
			ImagePullPolicy: corev1.PullIfNotPresent,
			Args:            slices.Clone(defaultContainer.Args),
			Env:             resolveEnvironment(input, defaultContainer.Env),
			EnvFrom:         append(slices.Clone(global.EnvFrom), slices.Clone(input.Spec.EnvFrom)...),
			Ports: []corev1.ContainerPort{
				{Name: "rest", ContainerPort: 7280, Protocol: corev1.ProtocolTCP},
				{Name: "grpc", ContainerPort: 7281, Protocol: corev1.ProtocolTCP},
				{Name: "discovery", ContainerPort: 7282, Protocol: corev1.ProtocolUDP},
				{Name: "cloudprem", ContainerPort: 7283, Protocol: corev1.ProtocolTCP},
				{Name: "health", ContainerPort: 7284, Protocol: corev1.ProtocolTCP},
			},
			Resources:      resources,
			VolumeMounts:   volumeMounts,
			StartupProbe:   &corev1.Probe{ProbeHandler: healthProbeHandler("/health/livez"), FailureThreshold: 12, PeriodSeconds: 5},
			LivenessProbe:  &corev1.Probe{ProbeHandler: healthProbeHandler("/health/livez")},
			ReadinessProbe: &corev1.Probe{ProbeHandler: healthProbeHandler("/health/readyz")},
			SecurityContext: &corev1.SecurityContext{
				RunAsNonRoot:           ptr.To(true),
				RunAsUser:              ptr.To[int64](1005),
				ReadOnlyRootFilesystem: ptr.To(true),
			},
		}},
		Volumes:                       volumes,
		NodeSelector:                  maps.Clone(input.Spec.NodeSelector),
		Affinity:                      affinity,
		Tolerations:                   append(slices.Clone(global.Tolerations), slices.Clone(input.Spec.Tolerations)...),
		TopologySpreadConstraints:     append(slices.Clone(global.TopologySpreadConstraints), slices.Clone(input.Spec.TopologySpreadConstraints)...),
		TerminationGracePeriodSeconds: terminationGracePeriodSeconds,
	}, nil
}

func resolveEnvironment(input workloadInput, additional []corev1.EnvVar) []corev1.EnvVar {
	clusterID := input.Cluster.Namespace + "-" + input.Cluster.Name
	datadog := input.Cluster.Spec.Datadog
	site := *datadog.Site
	telemetry := *datadog.BYOCTelemetry
	dogstatsdServer := datadog.DogstatsdServer
	dogstatsdHost := corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "status.hostIP"}}
	dogstatsdPort := *dogstatsdServer.Port
	if dogstatsdServer.Host != nil && *dogstatsdServer.Host != "" {
		dogstatsdHost = corev1.EnvVarSource{}
	}
	resourceField := func(resourceName string) *corev1.EnvVarSource {
		return &corev1.EnvVarSource{ResourceFieldRef: &corev1.ResourceFieldSelector{ContainerName: appName, Resource: resourceName}}
	}
	field := func(fieldPath string) *corev1.EnvVarSource {
		return &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: fieldPath}}
	}
	env := []corev1.EnvVar{
		{Name: "KUBERNETES_NAMESPACE", ValueFrom: field("metadata.namespace")},
		{Name: "KUBERNETES_COMPONENT", ValueFrom: field("metadata.labels['app.kubernetes.io/component']")},
		{Name: "KUBERNETES_POD_NAME", ValueFrom: field("metadata.name")},
		{Name: "KUBERNETES_NODE_NAME", ValueFrom: field("spec.nodeName")},
		{Name: "KUBERNETES_POD_IP", ValueFrom: field("status.podIP")},
		{Name: "KUBERNETES_LIMITS_CPU", ValueFrom: resourceField("limits.cpu")},
		{Name: "KUBERNETES_LIMITS_MEMORY", ValueFrom: resourceField("limits.memory")},
		{Name: "KUBERNETES_REQUESTS_CPU", ValueFrom: resourceField("requests.cpu")},
		{Name: "QW_NUM_CPUS", ValueFrom: resourceField("requests.cpu")},
		{Name: "KUBERNETES_REQUESTS_MEMORY", ValueFrom: resourceField("requests.memory")},
		{Name: "QW_CONFIG", Value: nodeConfigMountPath},
		{Name: "QW_CLUSTER_ID", Value: clusterID},
		{Name: "QW_NODE_ID", Value: "$(KUBERNETES_POD_NAME)"},
		{Name: "QW_AVAILABILITY_ZONE", ValueFrom: field("metadata.labels['topology.kubernetes.io/zone']")},
		{Name: "QW_PEER_SEEDS", Value: headlessServiceName(input.Cluster.Name)},
		{Name: "QW_ADVERTISE_ADDRESS", Value: "$(KUBERNETES_POD_IP)"},
		{Name: "QW_CLUSTER_ENDPOINT", Value: fmt.Sprintf("http://%s-metastore.%s.svc.%s:7280", input.Cluster.Name, input.Cluster.Namespace, defaultClusterDomain)},
		{Name: "CP_DOGSTATSD_SERVER_HOST", ValueFrom: &dogstatsdHost},
		{Name: "CP_DOGSTATSD_SERVER_PORT", Value: fmt.Sprint(dogstatsdPort)},
		{Name: "CP_ENABLE_REVERSE_CONNECTION", Value: "true"},
		{Name: "CP_MIN_SHARDS", Value: "12"},
		{Name: "DD_SITE", Value: site},
	}
	if dogstatsdServer.Host != nil && *dogstatsdServer.Host != "" {
		env[len(env)-5].ValueFrom = nil
		env[len(env)-5].Value = *dogstatsdServer.Host
	}
	if datadog.APIKeySecretRef != nil {
		env = append(env, corev1.EnvVar{Name: "DD_API_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: datadog.APIKeySecretRef.DeepCopy()}})
	}
	if telemetry {
		pomsky := input.Release.Release.Images.Pomsky
		host := site
		if site == "datadoghq.com" || site == "datadoghq.eu" || site == "ddog-gov.com" {
			host = "app." + site
		}
		env = append(env,
			corev1.EnvVar{Name: "QW_ENABLE_OPENTELEMETRY_OTLP_EXPORTER", Value: "true"},
			corev1.EnvVar{Name: "BYOC_TELEMETRY_ENABLED", Value: "true"},
			corev1.EnvVar{Name: "OTEL_RESOURCE_ATTRIBUTES", Value: "cluster_id=" + clusterID + ",node_id=$(QW_NODE_ID),host.name=$(KUBERNETES_NODE_NAME)"},
			corev1.EnvVar{Name: "OTEL_EXPORTER_OTLP_PROTOCOL", Value: "http/protobuf"},
			corev1.EnvVar{Name: "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", Value: "https://" + host + "/api/unstable/byoc-telemetry-intake/v1/logs"},
			corev1.EnvVar{Name: "OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE", Value: "delta"},
			corev1.EnvVar{Name: "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", Value: "https://" + host + "/api/unstable/byoc-telemetry-intake/v1/metrics"},
			corev1.EnvVar{Name: "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", Value: "https://" + host + "/api/unstable/byoc-telemetry-intake/v1/traces"},
			corev1.EnvVar{Name: "OTEL_TRACES_SAMPLER", Value: "parentbased_traceidratio"},
			corev1.EnvVar{Name: "OTEL_TRACES_SAMPLER_ARG", Value: "0.2"},
			corev1.EnvVar{Name: "IMAGE_NAME", Value: pomsky.Repository},
			corev1.EnvVar{Name: "IMAGE_TAG", Value: pomsky.Tag},
		)
	}
	if input.Cluster.Spec.Components.Compactor != nil {
		env = append(env, corev1.EnvVar{Name: "QW_ENABLE_STANDALONE_COMPACTORS", Value: "true"})
	}
	env = append(env, slices.Clone(additional)...)
	env = append(env,
		corev1.EnvVar{Name: "NO_COLOR", Value: "true"},
		corev1.EnvVar{Name: "QW_DISABLE_INGEST_V1", Value: "true"},
		corev1.EnvVar{Name: "QW_DISABLE_TELEMETRY", Value: "true"},
		corev1.EnvVar{Name: "QW_LOG_FORMAT", Value: "DDG"},
		corev1.EnvVar{Name: "QW_RANDOM_SPLIT_PREFIX", Value: "true"},
	)
	env = mergeEnv(env, input.Cluster.Spec.Global.Env)
	return mergeEnv(env, input.Spec.Env)
}

func selectorLabels(cluster *datadoghqv1alpha1.DatadogBYOCCluster, componentName string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":      appName,
		"app.kubernetes.io/instance":  cluster.Name,
		"app.kubernetes.io/component": componentName,
	}
}

func resolveAffinity(cluster *datadoghqv1alpha1.DatadogBYOCCluster, componentName string, global, component *corev1.Affinity) (*corev1.Affinity, error) {
	if global == nil && component == nil {
		return &corev1.Affinity{
			PodAntiAffinity: &corev1.PodAntiAffinity{
				PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{{
					Weight: 100,
					PodAffinityTerm: corev1.PodAffinityTerm{
						LabelSelector: &metav1.LabelSelector{MatchLabels: selectorLabels(cluster, componentName)},
						TopologyKey:   corev1.LabelHostname,
					},
				}},
			},
		}, nil
	}
	return mergeAffinity(global, component)
}

func mergeAffinity(global, component *corev1.Affinity) (*corev1.Affinity, error) {
	if global == nil {
		return component.DeepCopy(), nil
	}
	if component == nil {
		return global.DeepCopy(), nil
	}
	base, err := runtime.DefaultUnstructuredConverter.ToUnstructured(global)
	if err != nil {
		return nil, err
	}
	override, err := runtime.DefaultUnstructuredConverter.ToUnstructured(component)
	if err != nil {
		return nil, err
	}
	if err := mergo.Merge(&base, override, mergo.WithOverride); err != nil {
		return nil, err
	}
	result := &corev1.Affinity{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(base, result); err != nil {
		return nil, err
	}
	return result, nil
}

func mergeEnv(base, override []corev1.EnvVar) []corev1.EnvVar {
	result := slices.Clone(base)
	positions := make(map[string]int, len(result))
	for index := range result {
		positions[result[index].Name] = index
	}
	for _, env := range override {
		if index, found := positions[env.Name]; found {
			result[index] = *env.DeepCopy()
			continue
		}
		positions[env.Name] = len(result)
		result = append(result, *env.DeepCopy())
	}
	return result
}

func healthProbeHandler(path string) corev1.ProbeHandler {
	return corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: path, Port: intstr.FromString("health")}}
}
