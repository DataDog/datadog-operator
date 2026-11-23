package datadoggenericresource

import (
	"context"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

// CreateResult holds the resource metadata returned by a successful create API call.
// CreatedTime is nil if the API response did not include a creation time; the caller will use `now` as fallback.
type CreateResult struct {
	ID          string
	CreatedTime *metav1.Time
	Creator     string
	// RemoteDefinitionHash is set by handlers that implement remoteDefinitionHasher.
	RemoteDefinitionHash string
}

// ResourceHandler defines the CRUD operations for a Datadog resource type.
// Each implementation holds its own API client so the caller does not need to supply one.
type ResourceHandler interface {
	createResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) (CreateResult, error)
	getResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error
	updateResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error
	deleteResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error
}

// remoteDefinitionHasher is an optional extension of ResourceHandler. Handlers
// that implement it record a hash of the remote definition after each write, and
// periodic force syncs skip the update while the remote resource still has that
// hash. Other handlers update on every force sync. An empty hash never matches.
type remoteDefinitionHasher interface {
	// getRemoteDefinitionHash fetches the remote resource, in place of
	// getResource, and returns the hash of its definition.
	getRemoteDefinitionHash(auth context.Context, instance *v1alpha1.DatadogGenericResource) (string, error)
	// updateResourceWithHash updates the remote resource, in place of
	// updateResource, and returns the hash of the definition it now has.
	updateResourceWithHash(auth context.Context, instance *v1alpha1.DatadogGenericResource) (string, error)
}

// liveStateRefresher is an optional extension of ResourceHandler for resource
// types with live Datadog-side state. Only these handlers are polled and get
// the PollingDisabled state when state polling is disabled.
type liveStateRefresher interface {
	// refreshState fetches the live state. On error, the caller keeps the
	// last-known state and reports the failure via the StateSynced condition.
	refreshState(auth context.Context, instance *v1alpha1.DatadogGenericResource) (*string, error)
}
