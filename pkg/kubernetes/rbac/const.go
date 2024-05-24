// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rbac

// Consts used to setup Rbac config
// API Groups
const (
	CoreAPIGroup             = ""
	ExtensionsAPIGroup       = "extensions"
	OpenShiftQuotaAPIGroup   = "quota.openshift.io"
	RbacAPIGroup             = "rbac.authorization.k8s.io"
	AutoscalingAPIGroup      = "autoscaling"
	CertificatesAPIGroup     = "certificates.k8s.io"
	StorageAPIGroup          = "storage.k8s.io"
	CoordinationAPIGroup     = "coordination.k8s.io"
	DatadogAPIGroup          = "datadoghq.com"
	AdmissionAPIGroup        = "admissionregistration.k8s.io"
	AppsAPIGroup             = "apps"
	BatchAPIGroup            = "batch"
	PolicyAPIGroup           = "policy"
	NetworkingAPIGroup       = "networking.k8s.io"
	AutoscalingK8sIoAPIGroup = "autoscaling.k8s.io"
	AuthorizationAPIGroup    = "authorization.k8s.io"
	ExternalMetricsAPIGroup  = "external.metrics.k8s.io"
	RegistrationAPIGroup     = "apiregistration.k8s.io"
	APIExtensionsAPIGroup    = "apiextensions.k8s.io"

	// Resources

	APIServicesResource                 = "apiservices"
	CustomResourceDefinitionsResource   = "customresourcedefinitions"
	ServicesResource                    = "services"
	EventsResource                      = "events"
	EndpointsResource                   = "endpoints"
	PodsResource                        = "pods"
	PodsExecResource                    = "pods/exec"
	NodesResource                       = "nodes"
	ComponentStatusesResource           = "componentstatuses"
	CertificatesSigningRequestsResource = "certificatesigningrequests"
	ConfigMapsResource                  = "configmaps"
	ResourceQuotasResource              = "resourcequotas"
	ReplicationControllersResource      = "replicationcontrollers"
	LimitRangesResource                 = "limitranges"
	PersistentVolumeClaimsResource      = "persistentvolumeclaims"
	PersistentVolumesResource           = "persistentvolumes"
	LeasesResource                      = "leases"
	ClusterResourceQuotasResource       = "clusterresourcequotas"
	NodeMetricsResource                 = "nodes/metrics"
	NodeSpecResource                    = "nodes/spec"
	NodeProxyResource                   = "nodes/proxy"
	NodeStats                           = "nodes/stats"
	HorizontalPodAutoscalersRecource    = "horizontalpodautoscalers"
	DatadogMetricsResource              = "datadogmetrics"
	DatadogMetricsStatusResource        = "datadogmetrics/status"
	WpaResource                         = "watermarkpodautoscalers"
	MutatingConfigResource              = "mutatingwebhookconfigurations"
	ValidatingConfigResource            = "validatingwebhookconfigurations"
	SecretsResource                     = "secrets"
	PodDisruptionBudgetsResource        = "poddisruptionbudgets"
	ReplicasetsResource                 = "replicasets"
	DeploymentsResource                 = "deployments"
	StatefulsetsResource                = "statefulsets"
	DaemonsetsResource                  = "daemonsets"
	JobsResource                        = "jobs"
	CronjobsResource                    = "cronjobs"
	StorageClassesResource              = "storageclasses"
	VolumeAttachments                   = "volumeattachments"
	ExtendedDaemonSetReplicaSetResource = "extendeddaemonsetreplicasets"
	ServiceAccountResource              = "serviceaccounts"
	NamespaceResource                   = "namespaces"
	PodSecurityPolicyResource           = "podsecuritypolicies"
	ClusterRoleBindingResource          = "clusterrolebindings"
	RoleBindingResource                 = "rolebindings"
	NetworkPolicyResource               = "networkpolicies"
	IngressesResource                   = "ingresses"
	VPAResource                         = "verticalpodautoscalers"
	SubjectAccessReviewResource         = "subjectaccessreviews"
	ClusterRoleResource                 = "clusterroles"
	RoleResource                        = "roles"

	// Non resource URLs

	VersionURL     = "/version"
	HealthzURL     = "/healthz"
	MetricsURL     = "/metrics"
	MetricsSLIsURL = "/metrics/slis"

	// Verbs

	GetVerb    = "get"
	ListVerb   = "list"
	WatchVerb  = "watch"
	UpdateVerb = "update"
	CreateVerb = "create"
	DeleteVerb = "delete"

	// Rbac resource kinds

	ClusterRoleKind    = "ClusterRole"
	RoleKind           = "Role"
	ServiceAccountKind = "ServiceAccount"
)
