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

	// Datadog configures the connection used by the Observability Pipelines Worker.
	// +kubebuilder:validation:Required
	Datadog *DatadogObservabilityPipelinesWorkerDatadogSpec `json:"datadog,omitempty"`

	// Image is the fully resolved Observability Pipelines Worker image.
	// +kubebuilder:validation:Required
	Image *DatadogObservabilityPipelinesWorkerImageSpec `json:"image,omitempty"`

	// Identity configures an existing ServiceAccount used by the worker.
	// When omitted, the controller creates and owns a dedicated worker ServiceAccount.
	// +optional
	Identity *DatadogBYOCClusterIdentitySpec `json:"identity,omitempty"`

	// Service configures the Service exposing the worker ports.
	// +optional
	Service *DatadogObservabilityPipelinesWorkerServiceSpec `json:"service,omitempty"`
}

// DatadogObservabilityPipelinesWorkerPort defines a network port exposed by the worker.
// +k8s:openapi-gen=true
type DatadogObservabilityPipelinesWorkerPort struct {
	// Name identifies the port.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=15
	Name string `json:"name"`

	// Port is the port number exposed by the worker container and Service.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port int32 `json:"port"`

	// Protocol is the network protocol for the port.
	// +kubebuilder:default=TCP
	// +kubebuilder:validation:Enum=TCP;UDP;SCTP
	// +optional
	Protocol corev1.Protocol `json:"protocol,omitempty"`
}

// DatadogObservabilityPipelinesWorkerServiceSpec configures the Service exposing the worker.
// +k8s:openapi-gen=true
type DatadogObservabilityPipelinesWorkerServiceSpec struct {
	// Type determines how the Service is exposed.
	// +kubebuilder:default=ClusterIP
	// +kubebuilder:validation:Enum=ClusterIP;NodePort;LoadBalancer
	// +optional
	Type corev1.ServiceType `json:"type,omitempty"`
}

// DatadogObservabilityPipelinesWorkerDatadogSpec defines the Datadog connection settings.
// +k8s:openapi-gen=true
type DatadogObservabilityPipelinesWorkerDatadogSpec struct {
	// Site is the Datadog site used by the Observability Pipelines Worker.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Site *string `json:"site,omitempty"`

	// APIKeySecretRef references the Kubernetes Secret containing the Datadog API key.
	// +kubebuilder:validation:Required
	APIKeySecretRef *corev1.SecretKeySelector `json:"apiKeySecretRef,omitempty"`
}

// DatadogObservabilityPipelinesWorkerImageSpec defines a fully resolved container image.
// +k8s:openapi-gen=true
// +kubebuilder:validation:XValidation:rule="has(self.repository)",message="repository must be specified"
// +kubebuilder:validation:XValidation:rule="has(self.tag) != has(self.digest)",message="exactly one of tag or digest must be specified"
type DatadogObservabilityPipelinesWorkerImageSpec DatadogBYOCImageSpec

// DatadogObservabilityPipelinesWorkerStatus defines the observed state of DatadogObservabilityPipelinesWorker.
// +k8s:openapi-gen=true
type DatadogObservabilityPipelinesWorkerStatus struct {
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
