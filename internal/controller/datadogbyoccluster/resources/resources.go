// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	byocdefaults "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/defaults"
	byocrelease "github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/release"
)

// Resources is the complete set of Kubernetes resources managed for a cluster.
type Resources struct {
	configMap       *corev1.ConfigMap
	serviceAccount  *corev1.ServiceAccount
	headlessService *corev1.Service

	indexer   *StatefulSetResources
	searcher  *StatefulSetResources
	metastore *DeploymentResources

	controlPlane      *DeploymentResources
	janitor           *DeploymentResources
	readOnlyMetastore *DeploymentResources
	compactor         *DeploymentResources
}

// Shared returns the cluster-wide resources in apply order.
func (r *Resources) Shared() []client.Object {
	return []client.Object{r.configMap, r.serviceAccount, r.headlessService}
}

// Indexer returns the indexer resources.
func (r *Resources) Indexer() *StatefulSetResources {
	return r.indexer
}

// Searcher returns the searcher resources.
func (r *Resources) Searcher() *StatefulSetResources {
	return r.searcher
}

// Metastore returns the metastore resources.
func (r *Resources) Metastore() *DeploymentResources {
	return r.metastore
}

// ControlPlane returns the control plane resources.
func (r *Resources) ControlPlane() *DeploymentResources {
	return r.controlPlane
}

// Janitor returns the janitor resources.
func (r *Resources) Janitor() *DeploymentResources {
	return r.janitor
}

// ReadOnlyMetastore returns the read-only metastore resources when enabled.
func (r *Resources) ReadOnlyMetastore() *DeploymentResources {
	return r.readOnlyMetastore
}

// Compactor returns the compactor resources when enabled.
func (r *Resources) Compactor() *DeploymentResources {
	return r.compactor
}

// BuildResources builds the deterministic Kubernetes resources for a resolved release.
func BuildResources(cluster *datadoghqv1alpha1.DatadogBYOCCluster, release *byocrelease.ResolvedRelease) (*Resources, error) {
	cluster = byocdefaults.Apply(cluster)

	configMap, err := newConfigMapBuilder(cluster).build()
	if err != nil {
		return nil, err
	}

	checksum := sha256.Sum256([]byte(configMap.Data[nodeConfigFileName]))
	newWorkload := func(name string, spec *datadoghqv1alpha1.DatadogBYOCClusterComponentSpec, defaults workloadDefaults) workloadInput {
		return workloadInput{
			Cluster:  cluster,
			Release:  release,
			Checksum: hex.EncodeToString(checksum[:]),
			Name:     name,
			Spec:     spec,
			Defaults: defaults,
		}
	}

	components := cluster.Spec.Components
	indexerStatefulSpec := components.Indexer
	searcherStatefulSpec := components.Searcher
	indexerSpec := indexerStatefulSpec.DatadogBYOCClusterComponentSpec.DeepCopy()
	searcherSpec := searcherStatefulSpec.DatadogBYOCClusterComponentSpec.DeepCopy()
	metastoreSpec := components.Metastore.DatadogBYOCClusterComponentSpec.DeepCopy()
	controlPlaneSpec := components.ControlPlane.DeepCopy()
	janitorSpec := components.Janitor.DeepCopy()

	resources := &Resources{
		configMap:       configMap,
		serviceAccount:  newServiceAccountBuilder(cluster).build(),
		headlessService: newHeadlessServiceBuilder(cluster).build(),
	}

	resources.indexer, err = newStatefulSetBuilder(
		newWorkload("indexer", indexerSpec, workloadDefaults{
			ServicePorts: componentServicePorts(),
			PodSpec: corev1.PodSpec{
				Volumes: statefulDataVolumes(indexerStatefulSpec),
				Containers: []corev1.Container{{
					Args:         []string{"run", "--service", "indexer"},
					Env:          decommissionTimeoutEnvironment("QW_INGEST_DECOMMISSION_TIMEOUT", *indexerSpec.TerminationGracePeriodSeconds),
					VolumeMounts: []corev1.VolumeMount{{Name: "config", MountPath: "/quickwit/"}},
				}},
			},
		}),
		indexerStatefulSpec,
		statefulSetDefaults{
			PodManagementPolicy: appsv1.OrderedReadyPodManagement,
		},
	).build()
	if err != nil {
		return nil, err
	}

	resources.searcher, err = newStatefulSetBuilder(
		newWorkload("searcher", searcherSpec, workloadDefaults{
			ServicePorts: componentServicePorts(corev1.ServicePort{Name: "cloudprem", Port: 7283, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromString("cloudprem")}),
			PodSpec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Args:         []string{"run", "--service", "searcher"},
					VolumeMounts: []corev1.VolumeMount{{Name: "config", MountPath: nodeConfigMountPath, SubPath: nodeConfigFileName}},
				}},
				Volumes: statefulDataVolumes(searcherStatefulSpec),
			},
		}),
		searcherStatefulSpec,
		statefulSetDefaults{
			PodManagementPolicy: appsv1.OrderedReadyPodManagement,
		},
	).build()
	if err != nil {
		return nil, err
	}

	resources.metastore, err = newDeploymentBuilder(
		newWorkload("metastore", metastoreSpec, workloadDefaults{
			ServicePorts: componentServicePorts(),
			PodSpec: corev1.PodSpec{
				Volumes: []corev1.Volume{defaultDataVolume()},
				Containers: []corev1.Container{{
					Args:         []string{"run", "--service", "metastore"},
					Env:          databaseEnvironment("QW_METASTORE_URI", metastoreDatabase(components.Metastore)),
					VolumeMounts: []corev1.VolumeMount{defaultConfigVolumeMount()},
				}},
			},
		}),
		deploymentDefaults{},
	).build()
	if err != nil {
		return nil, err
	}

	resources.controlPlane, err = newDeploymentBuilder(
		newWorkload("control-plane", controlPlaneSpec, workloadDefaults{
			ServicePorts: componentServicePorts(),
			PodSpec: corev1.PodSpec{
				Volumes: []corev1.Volume{defaultDataVolume()},
				Containers: []corev1.Container{{
					Args:         []string{"run", "--service", "control_plane"},
					VolumeMounts: []corev1.VolumeMount{defaultConfigVolumeMount()},
				}},
			},
		}),
		deploymentDefaults{Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType}},
	).build()
	if err != nil {
		return nil, err
	}

	resources.janitor, err = newDeploymentBuilder(
		newWorkload("janitor", janitorSpec, workloadDefaults{
			ServicePorts: componentServicePorts(),
			PodSpec: corev1.PodSpec{
				Volumes: []corev1.Volume{defaultDataVolume()},
				Containers: []corev1.Container{{
					Args:         []string{"run", "--service", "janitor"},
					VolumeMounts: []corev1.VolumeMount{defaultConfigVolumeMount()},
				}},
			},
		}),
		deploymentDefaults{Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType}},
	).build()
	if err != nil {
		return nil, err
	}

	if components.ReadOnlyMetastore != nil {
		readOnlyMetastoreSpec := components.ReadOnlyMetastore.DatadogBYOCClusterComponentSpec.DeepCopy()
		resources.readOnlyMetastore, err = newDeploymentBuilder(
			newWorkload("metastore-ro", readOnlyMetastoreSpec, workloadDefaults{
				ServicePorts: componentServicePorts(),
				PodSpec: corev1.PodSpec{
					Volumes: []corev1.Volume{defaultDataVolume()},
					Containers: []corev1.Container{{
						Args:         []string{"run", "--service", "metastore_read_replica"},
						Env:          databaseEnvironment("QW_METASTORE_READ_REPLICA_URI", metastoreDatabase(components.ReadOnlyMetastore)),
						VolumeMounts: []corev1.VolumeMount{defaultConfigVolumeMount()},
					}},
				},
			}),
			deploymentDefaults{},
		).build()
		if err != nil {
			return nil, err
		}
	}

	if components.Compactor != nil {
		compactorSpec := components.Compactor.DeepCopy()
		resources.compactor, err = newDeploymentBuilder(
			newWorkload("compactor", compactorSpec, workloadDefaults{
				ServicePorts: componentServicePorts(),
				PodSpec: corev1.PodSpec{
					Volumes: []corev1.Volume{defaultDataVolume()},
					Containers: []corev1.Container{{
						Args:         []string{"run", "--service", "compactor"},
						Env:          decommissionTimeoutEnvironment("QW_COMPACTOR_DECOMMISSION_TIMEOUT", *compactorSpec.TerminationGracePeriodSeconds),
						VolumeMounts: []corev1.VolumeMount{defaultConfigVolumeMount()},
					}},
				},
			}),
			deploymentDefaults{},
		).build()
		if err != nil {
			return nil, err
		}
	}
	return resources, nil
}

func metastoreDatabase(spec *datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec) *datadoghqv1alpha1.DatadogBYOCClusterDatabaseSpec {
	return spec.Database
}

func decommissionTimeoutEnvironment(name string, terminationGracePeriod int64) []corev1.EnvVar {
	return []corev1.EnvVar{{Name: name, Value: fmt.Sprintf("%ds", terminationGracePeriod*9/10)}}
}

func databaseEnvironment(name string, database *datadoghqv1alpha1.DatadogBYOCClusterDatabaseSpec) []corev1.EnvVar {
	if database == nil || database.URISecretRef == nil {
		return nil
	}
	return []corev1.EnvVar{{Name: name, ValueFrom: &corev1.EnvVarSource{SecretKeyRef: database.URISecretRef.DeepCopy()}}}
}

func componentServicePorts(additional ...corev1.ServicePort) []corev1.ServicePort {
	ports := []corev1.ServicePort{
		{Name: "rest", Port: 7280, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromString("rest")},
		{Name: "grpc", Port: 7281, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromString("grpc")},
	}
	ports = append(ports, additional...)
	return append(ports, corev1.ServicePort{Name: "health", Port: 7284, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromString("health")})
}

func defaultConfigVolumeMount() corev1.VolumeMount {
	return corev1.VolumeMount{Name: "config", MountPath: nodeConfigMountPath, SubPath: nodeConfigFileName}
}

func defaultDataVolume() corev1.Volume {
	return corev1.Volume{Name: "data", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}
}

func statefulDataVolumes(spec *datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec) []corev1.Volume {
	if spec.Storage.VolumeClaimTemplate != nil {
		return nil
	}
	if spec.Storage.EmptyDir != nil {
		return []corev1.Volume{{Name: "data", VolumeSource: corev1.VolumeSource{EmptyDir: spec.Storage.EmptyDir.DeepCopy()}}}
	}
	return nil
}
