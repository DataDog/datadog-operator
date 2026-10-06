// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	byocimage "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/image"
	controllerutils "github.com/DataDog/datadog-operator/internal/controller/utils"
)

// podInput contains the cluster-wide inputs shared by every component pod.
type podInput struct {
	cluster  *datadoghqv1alpha1.DatadogBYOCCluster
	image    byocimage.ResolvedImage
	checksum string
}

func newComponentResources(in podInput, c component) (*ComponentResources, error) {
	cluster := in.cluster
	template, err := newPodTemplate(in, c)
	if err != nil {
		return nil, err
	}
	podDisruptionBudget, err := newPodDisruptionBudget(workloadObjectMeta(cluster, c), selectorLabels(cluster, c.name), cluster.Spec.Global.PodDisruptionBudget, c.spec.PodDisruptionBudget)
	if err != nil {
		return nil, err
	}
	r := &ComponentResources{
		Service:             newService(serviceObjectMeta(cluster, c), selectorLabels(cluster, c.name), c.servicePorts()),
		PodDisruptionBudget: podDisruptionBudget,
	}
	if c.stateful == nil {
		r.Deployment = newDeployment(workloadObjectMeta(cluster, c), selectorLabels(cluster, c.name), template, *c.spec.Replicas, c.strategy)
		return r, nil
	}
	r.StatefulSet = newStatefulSet(workloadObjectMeta(cluster, c), selectorLabels(cluster, c.name), template, headlessServiceName(cluster.Name), c.stateful)
	if c.stateful.Autoscaling != nil {
		r.HPA = newHPA(workloadObjectMeta(cluster, c), c.stateful.Autoscaling)
	}
	return r, nil
}

func workloadObjectMeta(cluster *datadoghqv1alpha1.DatadogBYOCCluster, c component) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:        ComponentResourceName(cluster.Name, c.name),
		Namespace:   cluster.Namespace,
		Labels:      podLabels(cluster, c),
		Annotations: annotations(cluster, c.spec.Annotations),
	}
}

func serviceObjectMeta(cluster *datadoghqv1alpha1.DatadogBYOCCluster, c component) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name:        ComponentResourceName(cluster.Name, c.name),
		Namespace:   cluster.Namespace,
		Labels:      labels(cluster, componentLabel(c.name), c.spec.Labels),
		Annotations: annotations(cluster),
	}
}

// podLabels lets component labels override the global ones without changing the selector.
func podLabels(cluster *datadoghqv1alpha1.DatadogBYOCCluster, c component) map[string]string {
	return labels(cluster, componentLabel(c.name), c.spec.Labels, selectorLabels(cluster, c.name))
}

func newPodTemplate(in podInput, c component) (corev1.PodTemplateSpec, error) {
	podSpec, err := newPodSpec(in, c)
	if err != nil {
		return corev1.PodTemplateSpec{}, err
	}
	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      podLabels(in.cluster, c),
			Annotations: annotations(in.cluster, c.spec.Annotations, map[string]string{configChecksumAnnotation: in.checksum}),
		},
		Spec: podSpec,
	}, nil
}

func newPodSpec(in podInput, c component) (corev1.PodSpec, error) {
	cluster := in.cluster
	global := cluster.Spec.Global
	spec := c.spec

	affinity, err := newAffinity(cluster, c.name, global.Affinity, spec.Affinity)
	if err != nil {
		return corev1.PodSpec{}, fmt.Errorf("merge %s affinity: %w", c.name, err)
	}
	serviceAccountName, _ := serviceAccountName(cluster)

	configVolume := corev1.Volume{
		Name: configVolumeName,
		VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: cluster.Name},
			Items:                []corev1.KeyToPath{{Key: nodeConfigFileName, Path: nodeConfigFileName}},
		}},
	}
	volumes := slices.Concat(
		[]corev1.Volume{configVolume},
		c.dataVolumes(),
		controllerutils.MergeVolumes(global.Volumes, spec.Volumes),
	)
	volumeMounts := slices.Concat(
		[]corev1.VolumeMount{c.configVolumeMount, {Name: dataVolumeName, MountPath: defaultDataPath}},
		controllerutils.MergeVolumeMounts(global.VolumeMounts, spec.VolumeMounts),
	)

	return corev1.PodSpec{
		ServiceAccountName: serviceAccountName,
		ImagePullSecrets:   slices.Clone(in.image.ImagePullSecrets),
		SecurityContext:    &corev1.PodSecurityContext{FSGroup: ptr.To[int64](1005)},
		DNSConfig:          &corev1.PodDNSConfig{Options: []corev1.PodDNSConfigOption{{Name: "ndots", Value: new("1")}}},
		InitContainers:     spec.InitContainers,
		Containers: []corev1.Container{{
			Name:            appName,
			Image:           in.image.GetImageReference(),
			ImagePullPolicy: in.image.GetImagePullPolicy(),
			Args:            []string{"run", "--service", c.quickwitService},
			Env:             newEnvironment(in, c),
			EnvFrom:         append(slices.Clone(global.EnvFrom), spec.EnvFrom...),
			Ports: []corev1.ContainerPort{
				{Name: "rest", ContainerPort: restPort, Protocol: corev1.ProtocolTCP},
				{Name: "grpc", ContainerPort: grpcPort, Protocol: corev1.ProtocolTCP},
				{Name: "discovery", ContainerPort: gossipPort, Protocol: corev1.ProtocolUDP},
				{Name: "cloudprem", ContainerPort: cloudpremPort, Protocol: corev1.ProtocolTCP},
				{Name: "health", ContainerPort: healthPort, Protocol: corev1.ProtocolTCP},
			},
			Resources:     ptr.Deref(spec.Resources, corev1.ResourceRequirements{}),
			VolumeMounts:  volumeMounts,
			StartupProbe:  &corev1.Probe{ProbeHandler: healthProbeHandler("/health/readyz"), FailureThreshold: 12, PeriodSeconds: 5},
			LivenessProbe: &corev1.Probe{ProbeHandler: healthProbeHandler("/health/livez"), TimeoutSeconds: 5},
			SecurityContext: &corev1.SecurityContext{
				RunAsNonRoot:           new(true),
				RunAsUser:              ptr.To[int64](1005),
				ReadOnlyRootFilesystem: new(true),
			},
		}},
		Volumes:                       volumes,
		NodeSelector:                  spec.NodeSelector,
		Affinity:                      affinity,
		Tolerations:                   append(slices.Clone(global.Tolerations), spec.Tolerations...),
		TopologySpreadConstraints:     controllerutils.MergeTopologySpreadConstraints(global.TopologySpreadConstraints, spec.TopologySpreadConstraints),
		TerminationGracePeriodSeconds: spec.TerminationGracePeriodSeconds,
	}, nil
}

func newEnvironment(in podInput, c component) []corev1.EnvVar {
	cluster := in.cluster
	clusterID := cluster.Namespace + "-" + cluster.Name
	datadog := cluster.Spec.Datadog
	site := *datadog.Site
	resourceField := func(resourceName string) *corev1.EnvVarSource {
		return &corev1.EnvVarSource{ResourceFieldRef: &corev1.ResourceFieldSelector{ContainerName: appName, Resource: resourceName}}
	}
	field := func(fieldPath string) *corev1.EnvVarSource {
		return &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: fieldPath}}
	}
	dogstatsdHost := corev1.EnvVar{Name: "CP_DOGSTATSD_SERVER_HOST", ValueFrom: field("status.hostIP")}
	if host := datadog.DogstatsdServer.Host; host != nil && *host != "" {
		dogstatsdHost = corev1.EnvVar{Name: "CP_DOGSTATSD_SERVER_HOST", Value: *host}
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
		{Name: "QW_CONFIG", Value: nodeConfigPath},
		{Name: "QW_CLUSTER_ID", Value: clusterID},
		{Name: "QW_NODE_ID", Value: "$(KUBERNETES_POD_NAME)"},
		{Name: "QW_AVAILABILITY_ZONE", ValueFrom: field("metadata.labels['topology.kubernetes.io/zone']")},
		{Name: "QW_PEER_SEEDS", Value: headlessServiceName(cluster.Name)},
		{Name: "QW_ADVERTISE_ADDRESS", Value: "$(KUBERNETES_POD_IP)"},
		{Name: "QW_CLUSTER_ENDPOINT", Value: fmt.Sprintf("http://%s.%s.svc.%s:%d", ComponentResourceName(cluster.Name, MetastoreComponentName), cluster.Namespace, defaultClusterDomain, restPort)},
		dogstatsdHost,
		{Name: "CP_DOGSTATSD_SERVER_PORT", Value: fmt.Sprint(*datadog.DogstatsdServer.Port)},
		{Name: "CP_ENABLE_REVERSE_CONNECTION", Value: "true"},
		{Name: "CP_MIN_SHARDS", Value: "12"},
		{Name: "DD_SITE", Value: site},
	}
	if provider := cluster.Spec.Provider; provider != nil && provider.AWS != nil && provider.AWS.Region != nil && *provider.AWS.Region != "" {
		env = append(env, corev1.EnvVar{Name: "AWS_REGION", Value: *provider.AWS.Region})
	}
	if datadog.APIKeySecretRef != nil {
		env = append(env, corev1.EnvVar{Name: "DD_API_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: datadog.APIKeySecretRef}})
	}
	if *datadog.BYOCTelemetry {
		env = append(env, telemetryEnvironment(clusterID, site, in.image)...)
	}
	if cluster.Spec.Components.Compactor != nil {
		env = append(env, corev1.EnvVar{Name: "QW_ENABLE_STANDALONE_COMPACTORS", Value: "true"})
	}
	env = append(env, c.env()...)
	env = append(env,
		corev1.EnvVar{Name: "NO_COLOR", Value: "true"},
		corev1.EnvVar{Name: "QW_DISABLE_INGEST_V1", Value: "true"},
		corev1.EnvVar{Name: "QW_DISABLE_TELEMETRY", Value: "true"},
		corev1.EnvVar{Name: "QW_LOG_FORMAT", Value: "DDG"},
		corev1.EnvVar{Name: "QW_RANDOM_SPLIT_PREFIX", Value: "true"},
	)
	env = controllerutils.MergeEnv(env, cluster.Spec.Global.Env)
	return controllerutils.MergeEnv(env, c.spec.Env)
}

func telemetryEnvironment(clusterID, site string, image byocimage.ResolvedImage) []corev1.EnvVar {
	host := site
	if site == "datadoghq.com" || site == "datadoghq.eu" || site == "ddog-gov.com" {
		host = "app." + site
	}
	intake := "https://" + host + "/api/unstable/byoc-telemetry-intake/v1/"
	return []corev1.EnvVar{
		{Name: "QW_ENABLE_OPENTELEMETRY_OTLP_EXPORTER", Value: "true"},
		{Name: "BYOC_TELEMETRY_ENABLED", Value: "true"},
		{Name: "OTEL_RESOURCE_ATTRIBUTES", Value: "cluster_id=" + clusterID + ",node_id=$(QW_NODE_ID),host.name=$(KUBERNETES_NODE_NAME)"},
		{Name: "OTEL_EXPORTER_OTLP_PROTOCOL", Value: "http/protobuf"},
		{Name: "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", Value: intake + "logs"},
		{Name: "OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE", Value: "delta"},
		{Name: "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT", Value: intake + "metrics"},
		{Name: "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", Value: intake + "traces"},
		{Name: "OTEL_TRACES_SAMPLER", Value: "parentbased_traceidratio"},
		{Name: "OTEL_TRACES_SAMPLER_ARG", Value: "0.2"},
		{Name: "IMAGE_NAME", Value: image.Repository},
		{Name: "IMAGE_TAG", Value: image.Tag},
	}
}

func newAffinity(cluster *datadoghqv1alpha1.DatadogBYOCCluster, componentName string, global, component *corev1.Affinity) (*corev1.Affinity, error) {
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
	return controllerutils.MergeAffinity(global, component)
}

func healthProbeHandler(path string) corev1.ProbeHandler {
	return corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{Path: path, Port: intstr.FromString("health")}}
}
