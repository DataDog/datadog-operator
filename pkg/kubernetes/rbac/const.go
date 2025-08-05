// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rbac

// This file tracks string constants that are used to set up RBAC configurations.

const (
	Wildcard = "*"

	// API Groups
	AdmissionAPIGroup        = "admissionregistration.k8s.io"
	APIExtensionsAPIGroup    = "apiextensions.k8s.io"
	AppsAPIGroup             = "apps"
	AuthorizationAPIGroup    = "authorization.k8s.io"
	AutoscalingAPIGroup      = "autoscaling"
	AutoscalingK8sIoAPIGroup = "autoscaling.k8s.io"
	BatchAPIGroup            = "batch"
	CertificatesAPIGroup     = "certificates.k8s.io"
	CoordinationAPIGroup     = "coordination.k8s.io"
	CoreAPIGroup             = ""
	DatadogAPIGroup          = "datadoghq.com"
	DiscoveryAPIGroup        = "discovery.k8s.io"
	ExtensionsAPIGroup       = "extensions"
	ExternalMetricsAPIGroup  = "external.metrics.k8s.io"
	NetworkingAPIGroup       = "networking.k8s.io"
	OpenShiftQuotaAPIGroup   = "quota.openshift.io"
	PolicyAPIGroup           = "policy"
	RbacAPIGroup             = "rbac.authorization.k8s.io"
	RegistrationAPIGroup     = "apiregistration.k8s.io"
	StorageAPIGroup          = "storage.k8s.io"
	EKSMetricsAPIGroup       = "metrics.eks.amazonaws.com"

	// Resources

	APIServicesResource                 = "apiservices"
	CertificatesSigningRequestsResource = "certificatesigningrequests"
	ClusterResourceQuotasResource       = "clusterresourcequotas"
	ClusterRoleBindingResource          = "clusterrolebindings"
	ClusterRoleResource                 = "clusterroles"
	ComponentStatusesResource           = "componentstatuses"
	ConfigMapsResource                  = "configmaps"
	CronjobsResource                    = "cronjobs"
	CustomResourceDefinitionsResource   = "customresourcedefinitions"
	DaemonsetsResource                  = "daemonsets"
	DatadogAgentsResource               = "datadogagents"
	DatadogAgentInternalsResource       = "datadogagentinternals"
	DatadogMetricsResource              = "datadogmetrics"
	DatadogMetricsStatusResource        = "datadogmetrics/status"
	DatadogPodAutoscalersResource       = "datadogpodautoscalers"
	DatadogPodAutoscalersStatusResource = "datadogpodautoscalers/status"
	DeploymentsResource                 = "deployments"
	EndpointsResource                   = "endpoints"
	EndpointsSlicesResource             = "endpointslices"
	EventsResource                      = "events"
	ExtendedDaemonSetReplicaSetResource = "extendeddaemonsetreplicasets"
	HorizontalPodAutoscalersRecource    = "horizontalpodautoscalers"
	IngressesResource                   = "ingresses"
	JobsResource                        = "jobs"
	LeasesResource                      = "leases"
	LimitRangesResource                 = "limitranges"
	MutatingConfigResource              = "mutatingwebhookconfigurations"
	NamespaceResource                   = "namespaces"
	NetworkPolicyResource               = "networkpolicies"
	NodeMetricsResource                 = "nodes/metrics"
	NodeProxyResource                   = "nodes/proxy"
	NodeSpecResource                    = "nodes/spec"
	NodesResource                       = "nodes"
	NodeStats                           = "nodes/stats"
	PersistentVolumeClaimsResource      = "persistentvolumeclaims"
	PersistentVolumesResource           = "persistentvolumes"
	PodDisruptionBudgetsResource        = "poddisruptionbudgets"
	PodsExecResource                    = "pods/exec"
	PodsResource                        = "pods"
	ReplicasetsResource                 = "replicasets"
	ReplicationControllersResource      = "replicationcontrollers"
	ResourceQuotasResource              = "resourcequotas"
	RoleBindingResource                 = "rolebindings"
	RoleResource                        = "roles"
	SecretsResource                     = "secrets"
	ServiceAccountResource              = "serviceaccounts"
	ServicesResource                    = "services"
	StatefulsetsResource                = "statefulsets"
	StorageClassesResource              = "storageclasses"
	SubjectAccessReviewResource         = "subjectaccessreviews"
	ValidatingConfigResource            = "validatingwebhookconfigurations"
	VolumeAttachments                   = "volumeattachments"
	VPAResource                         = "verticalpodautoscalers"
	WpaResource                         = "watermarkpodautoscalers"
	EKSKubeControllerManagerMetrics     = "kcm/metrics"
	EKSKubeSchedulerMetrics             = "ksh/metrics"

	// Non resource URLs

	HealthzURL     = "/healthz"
	MetricsSLIsURL = "/metrics/slis"
	MetricsURL     = "/metrics"
	VersionURL     = "/version"

	// Verbs

	CreateVerb = "create"
	DeleteVerb = "delete"
	GetVerb    = "get"
	ListVerb   = "list"
	PatchVerb  = "patch"
	UpdateVerb = "update"
	WatchVerb  = "watch"

	// RBAC resource kinds (singular)

	ClusterRoleBindingKind = "ClusterRoleBinding"
	ClusterRoleKind        = "ClusterRole"
	RoleKind               = "Role"
	ServiceAccountKind     = "ServiceAccount"
)
