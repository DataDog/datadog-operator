// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

const (
	// DatadogSPLConfigSecretDataKey is the data key in the worker config Secret.
	// The worker reads its configuration from /etc/datadog/synthetics-check-runner.json.
	DatadogSPLConfigSecretDataKey = "synthetics-check-runner.json"

	// DatadogSPLRequiredTag is appended to every private location created by the
	// operator to mark it as Kubernetes-managed.
	DatadogSPLRequiredTag = "generated:kubernetes"

	// DatadogSPLWorkerConfigOverrideAnnotation holds raw JSON merged last into
	// the worker configuration. Datadog-managed keys (accessKey,
	// secretAccessKey, publicKey, privateKey, id) cannot be overridden.
	DatadogSPLWorkerConfigOverrideAnnotation = "synthetics.datadoghq.com/worker-config-override"

	// DatadogSPLStatusProbesEnabledAnnotation enables the worker status
	// endpoints and the matching Deployment liveness and readiness probes when
	// set to "true".
	DatadogSPLStatusProbesEnabledAnnotation = "synthetics.datadoghq.com/status-probes-enabled"
)

// DatadogSyntheticsPrivateLocationSpec defines the desired state of DatadogSyntheticsPrivateLocation
// +k8s:openapi-gen=true
type DatadogSyntheticsPrivateLocationSpec struct {
	// Name of the private location in Datadog.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Description of the private location.
	// +kubebuilder:validation:MinLength=1
	Description string `json:"description"`
	// Tags for the private location. The operator appends the
	// "generated:kubernetes" tag unless disabled in controllerOptions.
	// +listType=atomic
	Tags []string `json:"tags"`
	// Worker configures the private location worker Deployment.
	// +optional
	Worker *DatadogSPLWorker `json:"worker,omitempty"`
	// ControllerOptions tweaks controller behavior.
	// +optional
	ControllerOptions *DatadogSPLControllerOptions `json:"controllerOptions,omitempty"`
}

// DatadogSPLControllerOptions tweaks controller behavior for a DatadogSyntheticsPrivateLocation.
// +k8s:openapi-gen=true
type DatadogSPLControllerOptions struct {
	// DisableRequiredTags disables auto-adding the "generated:kubernetes" tag.
	// +optional
	DisableRequiredTags bool `json:"disableRequiredTags,omitempty"`
}

// DatadogSPLWorker configures the private location worker Deployment.
// +k8s:openapi-gen=true
type DatadogSPLWorker struct {
	// Image is the private location worker container image.
	// +optional
	Image *DatadogSPLImage `json:"image,omitempty"`
	// Replicas is the worker Deployment replica count.
	// +kubebuilder:validation:Minimum=1
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`
	// Config holds worker config overrides merged into the Datadog-provided
	// worker configuration. Set other worker options with the
	// synthetics.datadoghq.com/worker-config-override annotation. Enable the
	// status probes with the synthetics.datadoghq.com/status-probes-enabled
	// annotation.
	// +optional
	Config *DatadogSPLWorkerConfig `json:"config,omitempty"`
	// ServiceAccount configures the worker ServiceAccount.
	// +optional
	ServiceAccount *DatadogSPLServiceAccount `json:"serviceAccount,omitempty"`
	// PodDisruptionBudget optionally creates a PodDisruptionBudget for the worker.
	// +optional
	PodDisruptionBudget *DatadogSPLPodDisruptionBudget `json:"podDisruptionBudget,omitempty"`
	// NodeSelector is a map of key-value pairs. For the worker pod to run on a
	// specific node, the node must have these key-value pairs as labels.
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
	// Affinity specifies the pod's scheduling constraints.
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`
	// Tolerations configure the worker pod tolerations.
	// +optional
	// +listType=atomic
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
	// PriorityClassName indicates the pod's priority.
	// +optional
	PriorityClassName *string `json:"priorityClassName,omitempty"`
	// HostAliases are hosts appended to /etc/hosts.
	// +optional
	// +listType=atomic
	HostAliases []corev1.HostAlias `json:"hostAliases,omitempty"`
	// DNSPolicy defines the DNS policy for the worker pods.
	// Default: ClusterFirst
	// +optional
	DNSPolicy *corev1.DNSPolicy `json:"dnsPolicy,omitempty"`
	// DNSConfig specifies the DNS parameters of the worker pods.
	// +optional
	DNSConfig *corev1.PodDNSConfig `json:"dnsConfig,omitempty"`
	// PodAnnotations are annotations applied to the worker pod template.
	// +optional
	PodAnnotations map[string]string `json:"podAnnotations,omitempty"`
	// PodLabels are labels applied to the worker pod template.
	// +optional
	PodLabels map[string]string `json:"podLabels,omitempty"`
	// CommonLabels are labels applied to all resources managed for this
	// private location (Deployment, Secret, ServiceAccount, PodDisruptionBudget).
	// +optional
	CommonLabels map[string]string `json:"commonLabels,omitempty"`
	// ExtraVolumes are additional volumes added to the worker pod.
	// +optional
	// +listType=map
	// +listMapKey=name
	ExtraVolumes []corev1.Volume `json:"extraVolumes,omitempty"`
	// ExtraVolumeMounts are additional volume mounts added to the worker container.
	// +optional
	// +listType=map
	// +listMapKey=name
	ExtraVolumeMounts []corev1.VolumeMount `json:"extraVolumeMounts,omitempty"`
	// Env are additional environment variables for the worker container.
	// +optional
	// +listType=map
	// +listMapKey=name
	Env []corev1.EnvVar `json:"env,omitempty"`
	// EnvFrom are envFrom sources for the worker container.
	// +optional
	// +listType=atomic
	EnvFrom []corev1.EnvFromSource `json:"envFrom,omitempty"`
	// ImagePullSecrets are secrets used to pull the worker image.
	// +optional
	// +listType=atomic
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
	// PodSecurityContext holds pod-level security attributes.
	// +optional
	PodSecurityContext *corev1.PodSecurityContext `json:"podSecurityContext,omitempty"`
	// SecurityContext holds container-level security attributes for the worker container.
	// +optional
	SecurityContext *corev1.SecurityContext `json:"securityContext,omitempty"`
	// Resources are compute resources for the worker container.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// DatadogSPLImage is the private location worker container image configuration.
// +k8s:openapi-gen=true
type DatadogSPLImage struct {
	// Repository of the worker image.
	// Default: gcr.io/datadoghq/synthetics-private-location-worker
	// +optional
	Repository string `json:"repository,omitempty"`
	// Tag of the worker image.
	// Default: the latest worker version supported by the operator.
	// +optional
	Tag string `json:"tag,omitempty"`
	// PullPolicy of the worker image.
	// +optional
	PullPolicy corev1.PullPolicy `json:"pullPolicy,omitempty"`
	// PullSecrets are secrets used to pull the worker image.
	// +optional
	// +listType=atomic
	PullSecrets []corev1.LocalObjectReference `json:"pullSecrets,omitempty"`
}

// DatadogSPLWorkerConfig holds worker config overrides merged into the
// Datadog-provided worker configuration.
// +k8s:openapi-gen=true
type DatadogSPLWorkerConfig struct {
	// Concurrency is the number of tests the worker runs in parallel.
	// +optional
	Concurrency *int32 `json:"concurrency,omitempty"`
	// ProxyDatadog is the proxy used for Datadog traffic.
	// +optional
	ProxyDatadog *string `json:"proxyDatadog,omitempty"`
	// ProxyTestRequests is the proxy used for test requests.
	// +optional
	ProxyTestRequests *string `json:"proxyTestRequests,omitempty"`
	// ProxyTestRequestsBypassList is the bypass list for the test requests proxy.
	// +optional
	// +listType=atomic
	ProxyTestRequestsBypassList []string `json:"proxyTestRequestsBypassList,omitempty"`
	// ProxyEnableConnectTunnel enables the HTTP CONNECT tunnel through the proxy.
	// +optional
	ProxyEnableConnectTunnel *bool `json:"proxyEnableConnectTunnel,omitempty"`
	// ProxyIgnoreSSLErrors ignores SSL errors for proxied test requests.
	// +optional
	ProxyIgnoreSSLErrors *bool `json:"proxyIgnoreSSLErrors,omitempty"`
	// ReportConfigTelemetry enables reporting config telemetry.
	// +optional
	ReportConfigTelemetry *bool `json:"reportConfigTelemetry,omitempty"`
	// ReportMetrics enables reporting metrics about the worker.
	// +optional
	ReportMetrics *bool `json:"reportMetrics,omitempty"`
}

// DatadogSPLServiceAccount configures the worker ServiceAccount.
// +k8s:openapi-gen=true
type DatadogSPLServiceAccount struct {
	// Create creates a ServiceAccount for the worker Deployment. Default: true.
	// +optional
	Create bool `json:"create"`
	// Name is the name of the ServiceAccount to use or create. Default: the
	// DatadogSyntheticsPrivateLocation name.
	// +optional
	Name string `json:"name,omitempty"`
	// Annotations are annotations applied to the ServiceAccount when created.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// DatadogSPLPodDisruptionBudget configures an optional PodDisruptionBudget for the worker.
// +k8s:openapi-gen=true
type DatadogSPLPodDisruptionBudget struct {
	// Enabled creates a PodDisruptionBudget for the worker Deployment.
	// +optional
	Enabled bool `json:"enabled"`
	// MinAvailable is the minimum number of pods that must be available.
	// +optional
	MinAvailable *intstr.IntOrString `json:"minAvailable,omitempty"`
	// MaxUnavailable is the maximum number of pods that can be unavailable.
	// +optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
}

// DatadogSyntheticsPrivateLocationStatus defines the observed state of DatadogSyntheticsPrivateLocation
// +k8s:openapi-gen=true
type DatadogSyntheticsPrivateLocationStatus struct {
	// ID is the Datadog private location ID.
	// +optional
	ID string `json:"id,omitempty"`
	// ConfigSecretName is the name of the Secret holding the worker configuration.
	// +optional
	ConfigSecretName string `json:"configSecretName,omitempty"`
	// CurrentHash tracks the hash of the current spec to know if the spec has
	// changed and needs an update.
	// +optional
	CurrentHash string `json:"currentHash,omitempty"`
	// LastForceSyncTime is the last time the private location was force synced
	// with the DatadogSyntheticsPrivateLocation resource.
	// +optional
	LastForceSyncTime *metav1.Time `json:"lastForceSyncTime,omitempty"`
	// SyncStatus shows the health of syncing the private location state to Datadog.
	// +optional
	SyncStatus DatadogSyntheticsPrivateLocationSyncStatus `json:"syncStatus,omitempty"`
	// Created is the time the private location was created.
	// +optional
	Created *metav1.Time `json:"created,omitempty"`
	// Deployment is the observed status of the worker Deployment.
	// +optional
	Deployment *v2alpha1.DeploymentStatus `json:"deployment,omitempty"`
	// Conditions represents the latest available observations of the
	// DatadogSyntheticsPrivateLocation's current state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ObservedGeneration is the most recent generation observed for this resource.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// DatadogSyntheticsPrivateLocationSyncStatus is the message reflecting the
// health of private location syncs to Datadog.
// +kubebuilder:validation:Enum=OK;error syncing private location;error creating private location;error updating private location;private location deleted
type DatadogSyntheticsPrivateLocationSyncStatus string

const (
	// DatadogSyntheticsPrivateLocationSyncStatusOK means syncing is OK.
	DatadogSyntheticsPrivateLocationSyncStatusOK DatadogSyntheticsPrivateLocationSyncStatus = "OK"
	// DatadogSyntheticsPrivateLocationSyncStatusSyncError means there is a private location sync error.
	DatadogSyntheticsPrivateLocationSyncStatusSyncError DatadogSyntheticsPrivateLocationSyncStatus = "error syncing private location"
	// DatadogSyntheticsPrivateLocationSyncStatusCreateError means there is a private location creation error.
	DatadogSyntheticsPrivateLocationSyncStatusCreateError DatadogSyntheticsPrivateLocationSyncStatus = "error creating private location"
	// DatadogSyntheticsPrivateLocationSyncStatusUpdateError means there is a private location update error.
	DatadogSyntheticsPrivateLocationSyncStatusUpdateError DatadogSyntheticsPrivateLocationSyncStatus = "error updating private location"
	// DatadogSyntheticsPrivateLocationSyncStatusDeleted means the private location no longer exists in Datadog.
	DatadogSyntheticsPrivateLocationSyncStatusDeleted DatadogSyntheticsPrivateLocationSyncStatus = "private location deleted"
)

// DatadogSyntheticsPrivateLocation is the Schema for the datadogsyntheticsprivatelocations API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=datadogsyntheticsprivatelocations,shortName=ddspl
// +kubebuilder:printcolumn:name="Private Location ID",type=string,JSONPath=".status.id"
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=".status.syncStatus"
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=".metadata.creationTimestamp"
// +k8s:openapi-gen=true
type DatadogSyntheticsPrivateLocation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatadogSyntheticsPrivateLocationSpec   `json:"spec,omitempty"`
	Status DatadogSyntheticsPrivateLocationStatus `json:"status,omitempty"`
}

// DatadogSyntheticsPrivateLocationList contains a list of DatadogSyntheticsPrivateLocation
// +kubebuilder:object:root=true
type DatadogSyntheticsPrivateLocationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatadogSyntheticsPrivateLocation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatadogSyntheticsPrivateLocation{}, &DatadogSyntheticsPrivateLocationList{})
}
