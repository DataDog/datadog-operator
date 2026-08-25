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
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
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
	indexerSpec := components.Indexer.DatadogBYOCClusterComponentSpec.DeepCopy()
	searcherSpec := components.Searcher.DatadogBYOCClusterComponentSpec.DeepCopy()
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
			Replicas:     2,
			ServicePorts: componentServicePorts(),
			PodSpec: corev1.PodSpec{
				TerminationGracePeriodSeconds: ptr.To[int64](300),
				Volumes:                       statefulDataVolumes(components.Indexer),
				Containers: []corev1.Container{{
					Args:         []string{"run", "--service", "indexer"},
					Env:          decommissionTimeoutEnvironment("QW_INGEST_DECOMMISSION_TIMEOUT", indexerSpec.TerminationGracePeriodSeconds, 300),
					Resources:    statefulDefaultResources(),
					VolumeMounts: []corev1.VolumeMount{{Name: "config", MountPath: "/quickwit/"}},
				}},
			},
		}),
		components.Indexer,
		statefulSetDefaults{
			PodManagementPolicy: appsv1.OrderedReadyPodManagement,
			Autoscaling: autoscalingDefaults{
				MinReplicas: ptr.To[int32](2),
				MaxReplicas: 10,
				Metrics:     cpuUtilizationMetrics(70),
				Behavior:    hpaBehavior(0, 300),
			},
		},
	).build()
	if err != nil {
		return nil, err
	}

	resources.searcher, err = newStatefulSetBuilder(
		newWorkload("searcher", searcherSpec, workloadDefaults{
			Replicas:     2,
			ServicePorts: componentServicePorts(corev1.ServicePort{Name: "cloudprem", Port: 7283, Protocol: corev1.ProtocolTCP, TargetPort: intstr.FromString("cloudprem")}),
			PodSpec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Args:         []string{"run", "--service", "searcher"},
					Resources:    statefulDefaultResources(),
					VolumeMounts: []corev1.VolumeMount{{Name: "config", MountPath: nodeConfigMountPath, SubPath: nodeConfigFileName}},
				}},
				Volumes: statefulDataVolumes(components.Searcher),
			},
		}),
		components.Searcher,
		statefulSetDefaults{
			PodManagementPolicy: appsv1.OrderedReadyPodManagement,
			Autoscaling: autoscalingDefaults{
				MinReplicas: ptr.To[int32](2),
				MaxReplicas: 10,
				Metrics:     cpuUtilizationMetrics(50),
				Behavior:    hpaBehavior(60, 300),
			},
		},
	).build()
	if err != nil {
		return nil, err
	}

	resources.metastore, err = newDeploymentBuilder(
		newWorkload("metastore", metastoreSpec, workloadDefaults{
			Replicas:     2,
			ServicePorts: componentServicePorts(),
			PodSpec: corev1.PodSpec{
				Volumes: []corev1.Volume{defaultDataVolume()},
				Containers: []corev1.Container{{
					Args:         []string{"run", "--service", "metastore"},
					Env:          databaseEnvironment("QW_METASTORE_URI", metastoreDatabase(components.Metastore)),
					Resources:    deploymentDefaultResources(),
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
			Replicas:     1,
			ServicePorts: componentServicePorts(),
			PodSpec: corev1.PodSpec{
				Volumes: []corev1.Volume{defaultDataVolume()},
				Containers: []corev1.Container{{
					Args:         []string{"run", "--service", "control_plane"},
					Resources:    deploymentDefaultResources(),
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
			Replicas:     1,
			ServicePorts: componentServicePorts(),
			PodSpec: corev1.PodSpec{
				Volumes: []corev1.Volume{defaultDataVolume()},
				Containers: []corev1.Container{{
					Args:         []string{"run", "--service", "janitor"},
					Resources:    deploymentDefaultResources(),
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
				Replicas:     2,
				ServicePorts: componentServicePorts(),
				PodSpec: corev1.PodSpec{
					Volumes: []corev1.Volume{defaultDataVolume()},
					Containers: []corev1.Container{{
						Args:         []string{"run", "--service", "metastore_read_replica"},
						Env:          databaseEnvironment("QW_METASTORE_READ_REPLICA_URI", metastoreDatabase(components.ReadOnlyMetastore)),
						Resources:    deploymentDefaultResources(),
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
				Replicas:     1,
				ServicePorts: componentServicePorts(),
				PodSpec: corev1.PodSpec{
					TerminationGracePeriodSeconds: ptr.To[int64](60),
					Volumes:                       []corev1.Volume{defaultDataVolume()},
					Containers: []corev1.Container{{
						Args:         []string{"run", "--service", "compactor"},
						Env:          decommissionTimeoutEnvironment("QW_COMPACTOR_DECOMMISSION_TIMEOUT", compactorSpec.TerminationGracePeriodSeconds, 60),
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

func decommissionTimeoutEnvironment(name string, terminationGracePeriod *int64, defaultTerminationGracePeriod int64) []corev1.EnvVar {
	grace := ptr.Deref(terminationGracePeriod, defaultTerminationGracePeriod)
	return []corev1.EnvVar{{Name: name, Value: fmt.Sprintf("%ds", grace*9/10)}}
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
	if spec.PersistentVolumeClaim != nil {
		return nil
	}
	return []corev1.Volume{defaultDataVolume()}
}

func statefulDefaultResources() corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("13100Mi")},
		Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("3600m"), corev1.ResourceMemory: resource.MustParse("13100Mi")},
	}
}

func deploymentDefaultResources() corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Limits:   corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("4Gi")},
		Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2"), corev1.ResourceMemory: resource.MustParse("4Gi")},
	}
}
