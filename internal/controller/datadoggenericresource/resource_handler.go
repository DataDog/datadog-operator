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

// remoteRead is the result of reading a remote resource.
type remoteRead struct {
	// Hash is the hash of the definition read, or "" if the handler does not
	// hash definitions or the read is NotModified.
	Hash string
	// ETag identifies the response, for handlers whose reads support ETags.
	// Handlers send status.Etag in If-None-Match.
	ETag string
	// NotModified reports that the resource, including its state, has not
	// changed since the read that returned status.Etag.
	NotModified bool
	// State is the live state read, or nil if it is not available.
	State *string
}

// remoteDefinitionHasher is an optional extension of ResourceHandler. Handlers
// that implement it record a hash of the remote definition after each write, and
// periodic force syncs skip the update while the remote resource still has that
// hash. Other handlers update on every force sync. An empty hash never matches.
type remoteDefinitionHasher interface {
	// readRemoteDefinition fetches the remote resource, in place of
	// getResource, and returns the hash of its definition.
	readRemoteDefinition(auth context.Context, instance *v1alpha1.DatadogGenericResource) (remoteRead, error)
	// updateResourceWithHash updates the remote resource, in place of
	// updateResource, and returns the hash of the definition it now has.
	updateResourceWithHash(auth context.Context, instance *v1alpha1.DatadogGenericResource) (string, error)
}

// liveStateRefresher is an optional extension of ResourceHandler for resource
// types with live Datadog-side state. Only these handlers are polled and get
// the PollingDisabled state when state polling is disabled.
type liveStateRefresher interface {
	// refreshState fetches the live state, and the definition hash for handlers
	// that implement remoteDefinitionHasher. On error, the caller keeps the
	// last-known state and reports the failure via the StateSynced condition.
	refreshState(auth context.Context, instance *v1alpha1.DatadogGenericResource) (remoteRead, error)
}
