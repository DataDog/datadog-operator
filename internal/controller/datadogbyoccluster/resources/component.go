// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

// component describes how a BYOC component is rendered.
type component struct {
	name string
	// spec and workload are nil when an optional component is disabled.
	spec     *datadoghqv1alpha1.DatadogBYOCClusterComponentSpec
	workload workload

	quickwitService   string
	configVolumeMount corev1.VolumeMount
	extraServicePorts []corev1.ServicePort
	extraEnv          []corev1.EnvVar
}

// ComponentResources contains the resources managed for a component.
// Exactly one of Deployment and StatefulSet is set.
type ComponentResources struct {
	Service             *corev1.Service
	Deployment          *appsv1.Deployment
	StatefulSet         *appsv1.StatefulSet
	HPA                 *autoscalingv2.HorizontalPodAutoscaler
	PodDisruptionBudget *policyv1.PodDisruptionBudget
}

// components returns every BYOC component in apply order.
func components(cluster *datadoghqv1alpha1.DatadogBYOCCluster) []component {
	spec := cluster.Spec.Components
	headlessService := headlessServiceName(cluster.Name)
	return []component{
		metastoreComponent(MetastoreComponentName, quickwitMetastoreServiceName, envQuickwitMetastoreURI, spec.Metastore),
		{
			name:              IndexerComponentName,
			spec:              &spec.Indexer.DatadogBYOCClusterComponentSpec,
			workload:          statefulSetWorkload{spec: spec.Indexer, serviceName: headlessService},
			quickwitService:   quickwitIndexerServiceName,
			configVolumeMount: corev1.VolumeMount{Name: configVolumeName, MountPath: quickwitDirectory},
			extraEnv:          []corev1.EnvVar{decommissionTimeoutEnv(envQuickwitIngestDecommissionTimeout, &spec.Indexer.DatadogBYOCClusterComponentSpec)},
		},
		{
			name:              SearcherComponentName,
			spec:              &spec.Searcher.DatadogBYOCClusterComponentSpec,
			workload:          statefulSetWorkload{spec: spec.Searcher, serviceName: headlessService},
			quickwitService:   quickwitSearcherServiceName,
			configVolumeMount: configFileVolumeMount(),
			extraServicePorts: []corev1.ServicePort{{Name: "cloudprem", Port: cloudpremPort, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromString("cloudprem")}},
		},
		{
			name:              ControlPlaneComponentName,
			spec:              spec.ControlPlane,
			workload:          deploymentWorkload{replicas: *spec.ControlPlane.Replicas, strategy: appsv1.RecreateDeploymentStrategyType},
			quickwitService:   quickwitControlPlaneServiceName,
			configVolumeMount: configFileVolumeMount(),
		},
		{
			name:              JanitorComponentName,
			spec:              spec.Janitor,
			workload:          deploymentWorkload{replicas: *spec.Janitor.Replicas, strategy: appsv1.RecreateDeploymentStrategyType},
			quickwitService:   quickwitJanitorServiceName,
			configVolumeMount: configFileVolumeMount(),
		},
		metastoreComponent(ReadOnlyMetastoreComponentName, quickwitReadOnlyMetastoreServiceName, envQuickwitReadOnlyMetastoreURI, spec.ReadOnlyMetastore),
		compactorComponent(spec.Compactor),
	}
}

func metastoreComponent(name, quickwitService, databaseURIEnv string, spec *datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec) component {
	c := component{
		name:              name,
		quickwitService:   quickwitService,
		configVolumeMount: configFileVolumeMount(),
	}
	if spec != nil {
		c.spec = &spec.DatadogBYOCClusterComponentSpec
		c.workload = deploymentWorkload{replicas: *spec.Replicas}
		if database := spec.Database; database != nil && database.URISecretRef != nil {
			c.extraEnv = []corev1.EnvVar{{Name: databaseURIEnv, ValueFrom: &corev1.EnvVarSource{SecretKeyRef: database.URISecretRef}}}
		}
	}
	return c
}

func compactorComponent(spec *datadoghqv1alpha1.DatadogBYOCClusterCompactorComponentSpec) component {
	c := component{
		name:              CompactorComponentName,
		quickwitService:   quickwitCompactorServiceName,
		configVolumeMount: configFileVolumeMount(),
	}
	if spec != nil {
		c.spec = &spec.DatadogBYOCClusterComponentSpec
		c.workload = autoscalableDeploymentWorkload{deploymentWorkload: deploymentWorkload{replicas: *spec.Replicas}, autoscaling: spec.Autoscaling}
		c.extraEnv = []corev1.EnvVar{decommissionTimeoutEnv(envQuickwitCompactorDecommissionTimeout, c.spec)}
	}
	return c
}

// decommissionTimeoutEnv leaves part of the termination grace period for the process to exit.
func decommissionTimeoutEnv(name string, spec *datadoghqv1alpha1.DatadogBYOCClusterComponentSpec) corev1.EnvVar {
	return corev1.EnvVar{Name: name, Value: fmt.Sprintf("%ds", *spec.TerminationGracePeriodSeconds*9/10)}
}

func (c component) enabled() bool {
	return c.spec != nil
}

func (c component) servicePorts() []corev1.ServicePort {
	ports := []corev1.ServicePort{
		{Name: "rest", Port: restPort, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromString("rest")},
		{Name: "grpc", Port: grpcPort, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromString("grpc")},
	}
	ports = append(ports, c.extraServicePorts...)
	return append(ports, corev1.ServicePort{Name: "health", Port: healthPort, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromString("health")})
}

func configFileVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{Name: configVolumeName, MountPath: nodeConfigPath, SubPath: nodeConfigFileName}
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
	c.workload.populate(r, workloadObjectMeta(cluster, c), selectorLabels(cluster, c.name), template)
	return r, nil
}

func (r *ComponentResources) objects() []client.Object {
	objects := []client.Object{r.Service}
	if r.Deployment != nil {
		objects = append(objects, r.Deployment)
	}
	if r.StatefulSet != nil {
		objects = append(objects, r.StatefulSet)
	}
	if r.HPA != nil {
		objects = append(objects, r.HPA)
	}
	if r.PodDisruptionBudget != nil {
		objects = append(objects, r.PodDisruptionBudget)
	}
	return objects
}

func (r *ComponentResources) obsoleteObjects(cluster *datadoghqv1alpha1.DatadogBYOCCluster, c component) []client.Object {
	metadata := componentObjectMeta(cluster, c.name)
	objects := c.workload.obsoleteObjects(metadata)
	if r.PodDisruptionBudget == nil {
		objects = append(objects, &policyv1.PodDisruptionBudget{ObjectMeta: metadata})
	}
	return objects
}

// disabledComponentObjects returns the resources of a disabled component. Only Deployment components are optional.
func disabledComponentObjects(cluster *datadoghqv1alpha1.DatadogBYOCCluster, c component) []client.Object {
	metadata := componentObjectMeta(cluster, c.name)
	return []client.Object{
		&corev1.Service{ObjectMeta: metadata},
		&appsv1.Deployment{ObjectMeta: metadata},
		&autoscalingv2.HorizontalPodAutoscaler{ObjectMeta: metadata},
		&policyv1.PodDisruptionBudget{ObjectMeta: metadata},
	}
}
