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
		SecurityContext:    &corev1.PodSecurityContext{FSGroup: new(pomskyUserID)},
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
			StartupProbe:  &corev1.Probe{ProbeHandler: healthProbeHandler(startupProbePath), FailureThreshold: startupProbeFailureThreshold, PeriodSeconds: startupProbePeriodSeconds},
			LivenessProbe: &corev1.Probe{ProbeHandler: healthProbeHandler(livenessProbePath), TimeoutSeconds: livenessProbeTimeoutSeconds},
			SecurityContext: &corev1.SecurityContext{
				RunAsNonRoot:           new(true),
				RunAsUser:              new(pomskyUserID),
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
	dogstatsdHost := corev1.EnvVar{Name: envCloudPremDogstatsdHost, ValueFrom: field("status.hostIP")}
	if host := datadog.DogstatsdServer.Host; host != nil && *host != "" {
		dogstatsdHost = corev1.EnvVar{Name: envCloudPremDogstatsdHost, Value: *host}
	}

	env := []corev1.EnvVar{
		{Name: envKubernetesNamespace, ValueFrom: field("metadata.namespace")},
		{Name: envKubernetesComponent, ValueFrom: field("metadata.labels['app.kubernetes.io/component']")},
		{Name: envKubernetesPodName, ValueFrom: field("metadata.name")},
		{Name: envKubernetesNodeName, ValueFrom: field("spec.nodeName")},
		{Name: envKubernetesPodIP, ValueFrom: field("status.podIP")},
		{Name: envKubernetesLimitsCPU, ValueFrom: resourceField("limits.cpu")},
		{Name: envKubernetesLimitsMemory, ValueFrom: resourceField("limits.memory")},
		{Name: envKubernetesRequestsCPU, ValueFrom: resourceField("requests.cpu")},
		{Name: envQuickwitNumCPUs, ValueFrom: resourceField("requests.cpu")},
		{Name: envKubernetesRequestsMemory, ValueFrom: resourceField("requests.memory")},
		{Name: envQuickwitConfig, Value: nodeConfigPath},
		{Name: envQuickwitClusterID, Value: clusterID},
		{Name: envQuickwitNodeID, Value: "$(KUBERNETES_POD_NAME)"},
		{Name: envQuickwitAvailabilityZone, ValueFrom: field("metadata.labels['topology.kubernetes.io/zone']")},
		{Name: envQuickwitPeerSeeds, Value: headlessServiceName(cluster.Name)},
		{Name: envQuickwitAdvertiseAddress, Value: "$(KUBERNETES_POD_IP)"},
		{Name: envQuickwitClusterEndpoint, Value: fmt.Sprintf("http://%s.%s.svc.%s:%d", ComponentResourceName(cluster.Name, MetastoreComponentName), cluster.Namespace, defaultClusterDomain, restPort)},
		dogstatsdHost,
		{Name: envCloudPremDogstatsdPort, Value: fmt.Sprint(*datadog.DogstatsdServer.Port)},
		{Name: envCloudPremReverseConnection, Value: "true"},
		{Name: envCloudPremMinShards, Value: cloudPremMinShards},
		{Name: envDatadogSite, Value: site},
	}
	if provider := cluster.Spec.Provider; provider != nil && provider.AWS != nil && provider.AWS.Region != nil && *provider.AWS.Region != "" {
		env = append(env, corev1.EnvVar{Name: envAWSRegion, Value: *provider.AWS.Region})
	}
	if datadog.APIKeySecretRef != nil {
		env = append(env, corev1.EnvVar{Name: envDatadogAPIKey, ValueFrom: &corev1.EnvVarSource{SecretKeyRef: datadog.APIKeySecretRef}})
	}
	if *datadog.BYOCTelemetry {
		env = append(env, telemetryEnvironment(clusterID, site, in.image)...)
	}
	if cluster.Spec.Components.Compactor != nil {
		env = append(env, corev1.EnvVar{Name: envQuickwitStandaloneCompactors, Value: "true"})
	}
	env = append(env, c.env()...)
	env = append(env,
		corev1.EnvVar{Name: envNoColor, Value: "true"},
		corev1.EnvVar{Name: envQuickwitDisableIngestV1, Value: "true"},
		corev1.EnvVar{Name: envQuickwitDisableTelemetry, Value: "true"},
		corev1.EnvVar{Name: envQuickwitLogFormat, Value: quickwitLogFormat},
		corev1.EnvVar{Name: envQuickwitRandomSplitPrefix, Value: "true"},
	)
	env = controllerutils.MergeEnv(env, cluster.Spec.Global.Env)
	return controllerutils.MergeEnv(env, c.spec.Env)
}

func telemetryEnvironment(clusterID, site string, image byocimage.ResolvedImage) []corev1.EnvVar {
	host := site
	if site == "datadoghq.com" || site == "datadoghq.eu" || site == "ddog-gov.com" {
		host = "app." + site
	}
	intake := "https://" + host + telemetryIntakePath
	return []corev1.EnvVar{
		{Name: envQuickwitOpenTelemetryExporter, Value: "true"},
		{Name: envBYOCTelemetryEnabled, Value: "true"},
		{Name: envOTelResourceAttributes, Value: "cluster_id=" + clusterID + ",node_id=$(QW_NODE_ID),host.name=$(KUBERNETES_NODE_NAME)"},
		{Name: envOTelExporterProtocol, Value: telemetryExporterProtocol},
		{Name: envOTelExporterLogsEndpoint, Value: intake + "logs"},
		{Name: envOTelExporterMetricsTemporality, Value: telemetryMetricsTemporality},
		{Name: envOTelExporterMetricsEndpoint, Value: intake + "metrics"},
		{Name: envOTelExporterTracesEndpoint, Value: intake + "traces"},
		{Name: envOTelTracesSampler, Value: telemetryTracesSampler},
		{Name: envOTelTracesSamplerArg, Value: telemetryTracesSamplerRatio},
		{Name: envImageName, Value: image.Repository},
		{Name: envImageTag, Value: image.Tag},
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
