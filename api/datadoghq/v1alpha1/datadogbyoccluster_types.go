// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// DatadogBYOCClusterSpec defines the desired state of DatadogBYOCCluster.
// +k8s:openapi-gen=true
type DatadogBYOCClusterSpec struct {
	// Release identifies the BYOC release artifact.
	// +kubebuilder:validation:Required
	Release *DatadogBYOCClusterReleaseSpec `json:"release,omitempty"`

	// Datadog configures the connection to Datadog.
	// +optional
	// +kubebuilder:default={}
	Datadog *DatadogBYOCClusterDatadogSpec `json:"datadog,omitempty"`

	// Provider configures the cloud provider used by the BYOC cluster.
	Provider *DatadogBYOCClusterProviderSpec `json:"provider,omitempty"`

	// Identity configures the Kubernetes identity used by the BYOC workloads.
	Identity *DatadogBYOCClusterIdentitySpec `json:"identity,omitempty"`

	// Global configures settings shared by all BYOC workloads.
	// +optional
	Global DatadogBYOCClusterGlobalSpec `json:"global,omitempty"`

	// Components configures the workloads that compose the BYOC cluster.
	// +kubebuilder:validation:Required
	Components *DatadogBYOCClusterComponentsSpec `json:"components,omitempty"`

	// NodeConfig contains the Quickwit node configuration.
	// +optional
	// +kubebuilder:pruning:PreserveUnknownFields
	NodeConfig *runtime.RawExtension `json:"nodeConfig,omitempty"`
}

// DatadogBYOCClusterReleaseSpec identifies a BYOC release artifact.
// +k8s:openapi-gen=true
// +kubebuilder:validation:XValidation:rule="has(self.tag) != has(self.digest)",message="exactly one of tag or digest must be specified"
type DatadogBYOCClusterReleaseSpec struct {
	// Repository is the OCI repository containing BYOC release artifacts.
	// The public Datadog BYOC release repository is used when this field is omitted.
	// +optional
	Repository *string `json:"repository,omitempty"`

	// Tag is the OCI tag of the BYOC release artifact.
	// +optional
	Tag *string `json:"tag,omitempty"`

	// Digest is the OCI digest of the BYOC release artifact.
	// +optional
	// +kubebuilder:validation:Pattern=`^sha256:[a-f0-9]{64}$`
	Digest *string `json:"digest,omitempty"`
}

// DatadogBYOCClusterDatadogSpec defines the Datadog connection settings.
// +k8s:openapi-gen=true
type DatadogBYOCClusterDatadogSpec struct {
	// Site is the Datadog site used by the BYOC workloads.
	// +optional
	// +kubebuilder:default=datadoghq.com
	Site *string `json:"site,omitempty"`

	// APIKeySecretRef references the Kubernetes Secret containing the Datadog API key.
	APIKeySecretRef *corev1.SecretKeySelector `json:"apiKeySecretRef,omitempty"`

	// BYOCTelemetry controls the export of BYOC product telemetry.
	// +optional
	// +kubebuilder:default=true
	BYOCTelemetry *bool `json:"byocTelemetry,omitempty"`

	// DogstatsdServer configures the DogStatsD server used by the BYOC workloads.
	// +optional
	// +kubebuilder:default={}
	DogstatsdServer *DatadogBYOCClusterDogstatsdServerSpec `json:"dogstatsdServer,omitempty"`
}

// DatadogBYOCClusterDogstatsdServerSpec defines a DogStatsD server endpoint.
// +k8s:openapi-gen=true
type DatadogBYOCClusterDogstatsdServerSpec struct {
	// Host is the DogStatsD server host.
	// +optional
	Host *string `json:"host,omitempty"`

	// Port is the DogStatsD server port.
	// +optional
	// +kubebuilder:default=8125
	Port *int32 `json:"port,omitempty"`
}

// DatadogBYOCClusterProviderSpec defines the cloud provider configuration.
// +k8s:openapi-gen=true
type DatadogBYOCClusterProviderSpec struct {
	// AWS configures an AWS-hosted BYOC cluster.
	// +optional
	AWS *DatadogBYOCClusterAWSSpec `json:"aws,omitempty"`
}

// DatadogBYOCClusterAWSSpec defines the AWS configuration.
// +k8s:openapi-gen=true
type DatadogBYOCClusterAWSSpec struct {
	// AccountID is the AWS account ID that owns the BYOC resources.
	AccountID *string `json:"accountID,omitempty"`

	// Region is the AWS region used by the BYOC cluster.
	Region *string `json:"region,omitempty"`

	// Partition is the AWS partition used by the BYOC cluster.
	// +optional
	Partition *string `json:"partition,omitempty"`

	// IRSARoleARN is the ARN of the IAM role associated with the BYOC ServiceAccount through IRSA.
	// +optional
	IRSARoleARN *string `json:"irsaRoleARN,omitempty"`
}

// DatadogBYOCClusterIdentitySpec defines the Kubernetes identity configuration.
// +k8s:openapi-gen=true
type DatadogBYOCClusterIdentitySpec struct {
	// ServiceAccountName is the ServiceAccount used by the BYOC workloads.
	ServiceAccountName *string `json:"serviceAccountName,omitempty"`
}

// DatadogBYOCClusterGlobalSpec defines settings shared by all BYOC workloads.
// +k8s:openapi-gen=true
type DatadogBYOCClusterGlobalSpec struct {
	// Labels are applied to all managed resources.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations are applied to all managed resources.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Env contains environment variables injected into all BYOC containers.
	// +optional
	// +listType=map
	// +listMapKey=name
	Env []corev1.EnvVar `json:"env,omitempty"`

	// EnvFrom contains environment sources injected into all BYOC containers.
	// +optional
	// +listType=atomic
	EnvFrom []corev1.EnvFromSource `json:"envFrom,omitempty"`

	// Volumes contains additional volumes attached to all BYOC Pods.
	// +optional
	// +listType=map
	// +listMapKey=name
	Volumes []corev1.Volume `json:"volumes,omitempty"`

	// VolumeMounts contains additional volume mounts for all BYOC containers.
	// +optional
	// +listType=map
	// +listMapKey=mountPath
	VolumeMounts []corev1.VolumeMount `json:"volumeMounts,omitempty"`

	// Tolerations are applied to all BYOC Pods.
	// +optional
	// +listType=atomic
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Affinity is applied to all BYOC Pods.
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// TopologySpreadConstraints are applied to all BYOC Pods.
	// +optional
	// +listType=map
	// +listMapKey=topologyKey
	// +listMapKey=whenUnsatisfiable
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`

	// PodDisruptionBudget configures Pod disruption budgets for all BYOC components.
	// When omitted, the Operator creates a budget with maxUnavailable set to 1.
	// Set this field to an empty object to disable the default budgets.
	// +optional
	PodDisruptionBudget *DatadogBYOCClusterPodDisruptionBudgetSpec `json:"podDisruptionBudget,omitempty"`
}

// DatadogBYOCClusterComponentsSpec defines the BYOC workloads.
// +k8s:openapi-gen=true
type DatadogBYOCClusterComponentsSpec struct {
	// Metastore configures the primary Metastore workload.
	// +kubebuilder:validation:Required
	Metastore *DatadogBYOCClusterMetastoreComponentSpec `json:"metastore,omitempty"`

	// ReadOnlyMetastore configures the read-only Metastore workload.
	// +optional
	ReadOnlyMetastore *DatadogBYOCClusterMetastoreComponentSpec `json:"readOnlyMetastore,omitempty"`

	// Indexer configures the Indexer workload.
	// +kubebuilder:validation:Required
	Indexer *DatadogBYOCClusterStatefulComponentSpec `json:"indexer,omitempty"`

	// Searcher configures the Searcher workload.
	// +kubebuilder:validation:Required
	Searcher *DatadogBYOCClusterStatefulComponentSpec `json:"searcher,omitempty"`

	// ControlPlane configures the Control Plane workload.
	// +kubebuilder:validation:Required
	ControlPlane *DatadogBYOCClusterComponentSpec `json:"controlPlane,omitempty"`

	// Compactor configures the Compactor workload.
	// +optional
	Compactor *DatadogBYOCClusterComponentSpec `json:"compactor,omitempty"`

	// Janitor configures the Janitor workload.
	// +kubebuilder:validation:Required
	Janitor *DatadogBYOCClusterComponentSpec `json:"janitor,omitempty"`
}

// DatadogBYOCClusterComponentSpec defines common workload settings.
// +k8s:openapi-gen=true
type DatadogBYOCClusterComponentSpec struct {
	// Replicas is the desired replica count.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// Env contains additional environment variables for the component container.
	// +optional
	// +listType=map
	// +listMapKey=name
	Env []corev1.EnvVar `json:"env,omitempty"`

	// EnvFrom contains additional environment sources for the component container.
	// +optional
	// +listType=atomic
	EnvFrom []corev1.EnvFromSource `json:"envFrom,omitempty"`

	// Volumes contains additional volumes attached to the component Pods.
	// +optional
	// +listType=map
	// +listMapKey=name
	Volumes []corev1.Volume `json:"volumes,omitempty"`

	// VolumeMounts contains additional volume mounts for the component container.
	// +optional
	// +listType=map
	// +listMapKey=mountPath
	VolumeMounts []corev1.VolumeMount `json:"volumeMounts,omitempty"`

	// Resources defines CPU and memory requirements for the component.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// Annotations are applied to the component resource.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// Labels are applied to the component resource.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// NodeSelector selects nodes on which the component Pods may run.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations are applied to the component Pods.
	// +optional
	// +listType=atomic
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Affinity is applied to the component Pods.
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// TopologySpreadConstraints are applied to the component Pods.
	// +optional
	// +listType=map
	// +listMapKey=topologyKey
	// +listMapKey=whenUnsatisfiable
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`

	// PodDisruptionBudget configures the component Pod disruption budget.
	// When omitted, the global setting or Operator default is used.
	// Set this field to an empty object to disable the budget for this component.
	// +optional
	PodDisruptionBudget *DatadogBYOCClusterPodDisruptionBudgetSpec `json:"podDisruptionBudget,omitempty"`

	// InitContainers are added to the component Pods.
	// +optional
	// +listType=map
	// +listMapKey=name
	InitContainers []corev1.Container `json:"initContainers,omitempty"`

	// TerminationGracePeriodSeconds is the grace period before a component Pod is forcibly terminated.
	// +optional
	TerminationGracePeriodSeconds *int64 `json:"terminationGracePeriodSeconds,omitempty"`
}

// DatadogBYOCClusterPodDisruptionBudgetSpec defines the availability constraint for voluntary Pod disruptions.
// +k8s:openapi-gen=true
// +kubebuilder:validation:XValidation:rule="!(has(self.minAvailable) && has(self.maxUnavailable))",message="minAvailable and maxUnavailable are mutually exclusive"
type DatadogBYOCClusterPodDisruptionBudgetSpec struct {
	// MinAvailable is the minimum number or percentage of Pods that must remain available after an eviction.
	// +optional
	MinAvailable *intstr.IntOrString `json:"minAvailable,omitempty"`

	// MaxUnavailable is the maximum number or percentage of Pods that may be unavailable after an eviction.
	// +optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
}

// DatadogBYOCClusterStatefulComponentSpec defines settings for a stateful BYOC workload.
// When Resources is specified, its memory limit is required for Quickwit node configuration sizing.
// +k8s:openapi-gen=true
// +kubebuilder:validation:XValidation:rule="!has(self.resources) || (has(self.resources.limits) && 'memory' in self.resources.limits)",message="resources.limits.memory must be specified when resources is set"
type DatadogBYOCClusterStatefulComponentSpec struct {
	DatadogBYOCClusterComponentSpec `json:",inline"`

	// Autoscaling configures horizontal autoscaling for the component.
	// +optional
	Autoscaling *DatadogBYOCClusterAutoscalingSpec `json:"autoscaling,omitempty"`

	// Storage configures the data volume for the component.
	// When omitted, the component default is used.
	// +optional
	Storage *DatadogBYOCClusterStorageSpec `json:"storage,omitempty"`
}

// DatadogBYOCClusterMetastoreComponentSpec defines settings for a Metastore workload.
// +k8s:openapi-gen=true
type DatadogBYOCClusterMetastoreComponentSpec struct {
	DatadogBYOCClusterComponentSpec `json:",inline"`

	// Database configures the PostgreSQL database used by the Metastore.
	Database *DatadogBYOCClusterDatabaseSpec `json:"database,omitempty"`
}

// DatadogBYOCClusterAutoscalingSpec defines horizontal autoscaling settings.
// +k8s:openapi-gen=true
type DatadogBYOCClusterAutoscalingSpec struct {
	// MinReplicas is the lower limit for the number of replicas.
	// +optional
	MinReplicas *int32 `json:"minReplicas,omitempty"`

	// MaxReplicas is the upper limit for the number of replicas.
	// +optional
	MaxReplicas *int32 `json:"maxReplicas,omitempty"`

	// Metrics contains the specifications used to calculate the desired replica count.
	// +optional
	// +listType=atomic
	Metrics []autoscalingv2.MetricSpec `json:"metrics,omitempty"`

	// Behavior configures scaling behavior in both directions.
	// +optional
	Behavior *autoscalingv2.HorizontalPodAutoscalerBehavior `json:"behavior,omitempty"`
}

// DatadogBYOCClusterStorageSpec defines storage for a stateful BYOC workload.
// +k8s:openapi-gen=true
// +kubebuilder:validation:XValidation:rule="has(self.emptyDir) != has(self.volumeClaimTemplate)",message="exactly one storage type must be specified"
type DatadogBYOCClusterStorageSpec struct {
	// EmptyDir configures an emptyDir volume for the component.
	// +optional
	EmptyDir *corev1.EmptyDirVolumeSource `json:"emptyDir,omitempty"`

	// VolumeClaimTemplate configures a persistent volume claim template for the component.
	// +optional
	VolumeClaimTemplate *DatadogBYOCClusterEmbeddedPersistentVolumeClaim `json:"volumeClaimTemplate,omitempty"`
}

// DatadogBYOCClusterEmbeddedPersistentVolumeClaim is an embedded PersistentVolumeClaim with reduced metadata.
// +k8s:openapi-gen=true
type DatadogBYOCClusterEmbeddedPersistentVolumeClaim struct {
	metav1.TypeMeta `json:",inline"`

	// Metadata contains metadata relevant to the embedded PersistentVolumeClaim.
	// +optional
	DatadogBYOCClusterEmbeddedObjectMetadata `json:"metadata,omitempty"`

	// Spec defines the desired characteristics of the persistent volume claim.
	// +optional
	Spec corev1.PersistentVolumeClaimSpec `json:"spec,omitempty"`
}

// DatadogBYOCClusterEmbeddedObjectMetadata contains metadata relevant to an embedded resource.
// +k8s:openapi-gen=true
type DatadogBYOCClusterEmbeddedObjectMetadata struct {
	// Labels are applied to the embedded resource.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations are applied to the embedded resource.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// DatadogBYOCClusterDatabaseSpec defines a Metastore database connection.
// +k8s:openapi-gen=true
type DatadogBYOCClusterDatabaseSpec struct {
	// URISecretRef references the Kubernetes Secret containing the database URI.
	URISecretRef *corev1.SecretKeySelector `json:"uriSecretRef,omitempty"`
}

// DatadogBYOCClusterStatus defines the observed state of DatadogBYOCCluster.
// +k8s:openapi-gen=true
type DatadogBYOCClusterStatus struct {
	// Conditions contains cluster-wide observations owned by the Operator.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// Indexer contains the observed state of the Indexer StatefulSet.
	// +optional
	Indexer *DatadogBYOCClusterStatefulSetStatus `json:"indexer,omitempty"`

	// Searcher contains the observed state of the Searcher StatefulSet.
	// +optional
	Searcher *DatadogBYOCClusterStatefulSetStatus `json:"searcher,omitempty"`

	// Metastore contains the observed state of the primary Metastore Deployment.
	// +optional
	Metastore *DatadogBYOCClusterDeploymentStatus `json:"metastore,omitempty"`

	// ReadOnlyMetastore contains the observed state of the read-only Metastore Deployment.
	// +optional
	ReadOnlyMetastore *DatadogBYOCClusterDeploymentStatus `json:"readOnlyMetastore,omitempty"`

	// ControlPlane contains the observed state of the Control Plane Deployment.
	// +optional
	ControlPlane *DatadogBYOCClusterDeploymentStatus `json:"controlPlane,omitempty"`

	// Compactor contains the observed state of the Compactor Deployment.
	// +optional
	Compactor *DatadogBYOCClusterDeploymentStatus `json:"compactor,omitempty"`

	// Janitor contains the observed state of the Janitor Deployment.
	// +optional
	Janitor *DatadogBYOCClusterDeploymentStatus `json:"janitor,omitempty"`
}

// DatadogBYOCClusterStatefulSetStatus defines the observed state of a StatefulSet component.
// +k8s:openapi-gen=true
type DatadogBYOCClusterStatefulSetStatus struct {
	// ObservedGeneration is the most recent generation observed by the component controller.
	// +optional
	ObservedGeneration *int64 `json:"observedGeneration,omitempty"`

	// Replicas is the total number of Pods created by the StatefulSet.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// ReadyReplicas is the number of Pods with a Ready condition.
	// +optional
	ReadyReplicas *int32 `json:"readyReplicas,omitempty"`
}

// DatadogBYOCClusterDeploymentStatus defines the observed state of a Deployment component.
// +k8s:openapi-gen=true
type DatadogBYOCClusterDeploymentStatus struct {
	// Replicas is the total number of Pods created by the Deployment.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// ReadyReplicas is the number of Pods with a Ready condition.
	// +optional
	ReadyReplicas *int32 `json:"readyReplicas,omitempty"`

	// UnavailableReplicas is the number of unavailable Pods.
	// +optional
	UnavailableReplicas *int32 `json:"unavailableReplicas,omitempty"`

	// AvailableReplicas is the number of available Pods.
	// +optional
	AvailableReplicas *int32 `json:"availableReplicas,omitempty"`
}

// DatadogBYOCCluster is the Schema for the datadogbyocclusters API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=datadogbyocclusters,scope=Namespaced,shortName=ddbyoc
// +k8s:openapi-gen=true
type DatadogBYOCCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatadogBYOCClusterSpec   `json:"spec,omitempty"`
	Status DatadogBYOCClusterStatus `json:"status,omitempty"`
}

// DatadogBYOCClusterList contains a list of DatadogBYOCCluster resources.
// +kubebuilder:object:root=true
type DatadogBYOCClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatadogBYOCCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatadogBYOCCluster{}, &DatadogBYOCClusterList{})
}
