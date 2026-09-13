// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatadogObservabilityPipelinesWorkerSpec defines the desired state of DatadogObservabilityPipelinesWorker.
// When Resources is specified, its memory limit is required for worker buffer sizing.
// +k8s:openapi-gen=true
type DatadogObservabilityPipelinesWorkerSpec struct {
	DatadogBYOCClusterPipelineComponentSpec `json:",inline"`

	// Datadog configures the connection used to manage the Observability Pipeline.
	// +kubebuilder:validation:Required
	Datadog *DatadogObservabilityPipelinesWorkerDatadogSpec `json:"datadog,omitempty"`

	// Image is the fully resolved Observability Pipelines Worker image.
	// +kubebuilder:validation:Required
	Image *DatadogObservabilityPipelinesWorkerImageSpec `json:"image,omitempty"`

	// Identity configures an existing ServiceAccount used by the worker.
	// When omitted, the controller creates and owns a dedicated worker ServiceAccount.
	// +optional
	Identity *DatadogBYOCClusterIdentitySpec `json:"identity,omitempty"`
}

// DatadogObservabilityPipelinesWorkerDatadogSpec defines the Datadog connection settings.
// +k8s:openapi-gen=true
type DatadogObservabilityPipelinesWorkerDatadogSpec struct {
	// Site is the Datadog site used to manage the Observability Pipeline.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Site *string `json:"site,omitempty"`

	// APIKeySecretRef references the Kubernetes Secret containing the Datadog API key.
	// +kubebuilder:validation:Required
	APIKeySecretRef *corev1.SecretKeySelector `json:"apiKeySecretRef,omitempty"`

	// AppKeySecretRef references the Kubernetes Secret containing the Datadog application key.
	// +kubebuilder:validation:Required
	AppKeySecretRef *corev1.SecretKeySelector `json:"appKeySecretRef,omitempty"`
}

// DatadogObservabilityPipelinesWorkerImageSpec defines a fully resolved container image.
// +k8s:openapi-gen=true
// +kubebuilder:validation:XValidation:rule="has(self.repository)",message="repository must be specified"
// +kubebuilder:validation:XValidation:rule="has(self.tag) != has(self.digest)",message="exactly one of tag or digest must be specified"
type DatadogObservabilityPipelinesWorkerImageSpec DatadogBYOCImageSpec

// DatadogObservabilityPipelinesWorkerStatus defines the observed state of DatadogObservabilityPipelinesWorker.
// +k8s:openapi-gen=true
type DatadogObservabilityPipelinesWorkerStatus struct {
	// GeneratedPipelineID is the ID of the pipeline created by the controller.
	// It is not populated when spec.pipelineID selects an existing pipeline.
	// +optional
	GeneratedPipelineID *string `json:"generatedPipelineID,omitempty"`

	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	ObservedGeneration *int64 `json:"observedGeneration,omitempty"`

	// Replicas is the total number of Pods created by the worker StatefulSet.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// ReadyReplicas is the number of worker Pods with a Ready condition.
	// +optional
	ReadyReplicas *int32 `json:"readyReplicas,omitempty"`

	// Conditions contains observations of the pipeline and worker workload state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// DatadogObservabilityPipelinesWorker is the Schema for the datadogobservabilitypipelinesworkers API.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=datadogobservabilitypipelinesworkers,scope=Namespaced,shortName=ddopw
// +k8s:openapi-gen=true
type DatadogObservabilityPipelinesWorker struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Required
	Spec   DatadogObservabilityPipelinesWorkerSpec   `json:"spec,omitempty"`
	Status DatadogObservabilityPipelinesWorkerStatus `json:"status,omitempty"`
}

// DatadogObservabilityPipelinesWorkerList contains a list of DatadogObservabilityPipelinesWorker resources.
// +kubebuilder:object:root=true
type DatadogObservabilityPipelinesWorkerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatadogObservabilityPipelinesWorker `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatadogObservabilityPipelinesWorker{}, &DatadogObservabilityPipelinesWorkerList{})
}
