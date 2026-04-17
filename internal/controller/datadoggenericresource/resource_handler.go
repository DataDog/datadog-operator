package datadoggenericresource

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

// CreateResult holds the resource metadata returned by a successful create API call.
// CreatedTime is nil if the API response did not include a creation time; the caller will use `now` as fallback.
type CreateResult struct {
	ID          string
	CreatedTime *metav1.Time
	Creator     string
}

// ResourceHandler defines the CRUD operations for a Datadog resource type.
// Each implementation is stateful: it holds its own API client and auth context,
// so the caller does not need to supply them.
type ResourceHandler interface {
	createResource(instance *v1alpha1.DatadogGenericResource) (CreateResult, error)
	getResource(instance *v1alpha1.DatadogGenericResource) error
	updateResource(instance *v1alpha1.DatadogGenericResource) error
	deleteResource(instance *v1alpha1.DatadogGenericResource) error
}
