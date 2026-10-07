// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

// component describes how a BYOC component is rendered.
type component struct {
	name string
	// spec is nil when an optional component is disabled.
	spec *datadoghqv1alpha1.DatadogBYOCClusterComponentSpec
	// stateful is set for components rendered as a StatefulSet.
	stateful *datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec

	quickwitService   string
	configVolumeMount corev1.VolumeMount
	extraServicePorts []corev1.ServicePort
	strategy          appsv1.DeploymentStrategyType

	databaseURIEnv string
	database       *datadoghqv1alpha1.DatadogBYOCClusterDatabaseSpec
	// decommissionTimeoutEnv is set to a fraction of the termination grace period.
	decommissionTimeoutEnv string
}

// components returns every BYOC component in apply order.
func components(cluster *datadoghqv1alpha1.DatadogBYOCCluster) []component {
	spec := cluster.Spec.Components
	return []component{
		metastoreComponent(MetastoreComponentName, quickwitMetastoreServiceName, envQuickwitMetastoreURI, spec.Metastore),
		{
			name:                   IndexerComponentName,
			spec:                   &spec.Indexer.DatadogBYOCClusterComponentSpec,
			stateful:               spec.Indexer,
			quickwitService:        quickwitIndexerServiceName,
			configVolumeMount:      corev1.VolumeMount{Name: configVolumeName, MountPath: quickwitDirectory},
			decommissionTimeoutEnv: envQuickwitIngestDecommissionTimeout,
		},
		{
			name:              SearcherComponentName,
			spec:              &spec.Searcher.DatadogBYOCClusterComponentSpec,
			stateful:          spec.Searcher,
			quickwitService:   quickwitSearcherServiceName,
			configVolumeMount: configFileVolumeMount(),
			extraServicePorts: []corev1.ServicePort{{Name: "cloudprem", Port: cloudpremPort, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromString("cloudprem")}},
		},
		{
			name:              ControlPlaneComponentName,
			spec:              spec.ControlPlane,
			quickwitService:   quickwitControlPlaneServiceName,
			configVolumeMount: configFileVolumeMount(),
			strategy:          appsv1.RecreateDeploymentStrategyType,
		},
		{
			name:              JanitorComponentName,
			spec:              spec.Janitor,
			quickwitService:   quickwitJanitorServiceName,
			configVolumeMount: configFileVolumeMount(),
			strategy:          appsv1.RecreateDeploymentStrategyType,
		},
		metastoreComponent(ReadOnlyMetastoreComponentName, quickwitReadOnlyMetastoreServiceName, envQuickwitReadOnlyMetastoreURI, spec.ReadOnlyMetastore),
		{
			name:                   CompactorComponentName,
			spec:                   spec.Compactor,
			quickwitService:        quickwitCompactorServiceName,
			configVolumeMount:      configFileVolumeMount(),
			decommissionTimeoutEnv: envQuickwitCompactorDecommissionTimeout,
		},
	}
}

func metastoreComponent(name, quickwitService, databaseURIEnv string, spec *datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec) component {
	c := component{
		name:              name,
		quickwitService:   quickwitService,
		configVolumeMount: configFileVolumeMount(),
		databaseURIEnv:    databaseURIEnv,
	}
	if spec != nil {
		c.spec = &spec.DatadogBYOCClusterComponentSpec
		c.database = spec.Database
	}
	return c
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

func (c component) env() []corev1.EnvVar {
	var env []corev1.EnvVar
	if c.database != nil && c.database.URISecretRef != nil {
		env = append(env, corev1.EnvVar{Name: c.databaseURIEnv, ValueFrom: &corev1.EnvVarSource{SecretKeyRef: c.database.URISecretRef}})
	}
	if c.decommissionTimeoutEnv != "" {
		env = append(env, corev1.EnvVar{Name: c.decommissionTimeoutEnv, Value: fmt.Sprintf("%ds", *c.spec.TerminationGracePeriodSeconds*9/10)})
	}
	return env
}

func (c component) dataVolumes() []corev1.Volume {
	if c.stateful == nil {
		return []corev1.Volume{{Name: dataVolumeName, VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}}
	}
	if storage := c.stateful.Storage; storage != nil && storage.VolumeClaimTemplate == nil && storage.EmptyDir != nil {
		return []corev1.Volume{{Name: dataVolumeName, VolumeSource: corev1.VolumeSource{EmptyDir: storage.EmptyDir}}}
	}
	return nil
}

func configFileVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{Name: configVolumeName, MountPath: nodeConfigPath, SubPath: nodeConfigFileName}
}
