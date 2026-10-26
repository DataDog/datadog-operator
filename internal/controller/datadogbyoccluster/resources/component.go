// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// workload renders the controller running the pods of a component.
type workload interface {
	dataVolumes() []corev1.Volume
	populate(r *ComponentResources, metadata metav1.ObjectMeta, selector map[string]string, template corev1.PodTemplateSpec)
	obsoleteObjects(metadata metav1.ObjectMeta) []client.Object
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

func compactorComponent(spec *datadoghqv1alpha1.DatadogBYOCClusterComponentSpec) component {
	c := component{
		name:              CompactorComponentName,
		quickwitService:   quickwitCompactorServiceName,
		configVolumeMount: configFileVolumeMount(),
	}
	if spec != nil {
		c.spec = spec
		c.workload = deploymentWorkload{replicas: *spec.Replicas}
		c.extraEnv = []corev1.EnvVar{decommissionTimeoutEnv(envQuickwitCompactorDecommissionTimeout, spec)}
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
