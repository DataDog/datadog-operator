package datadoggenericresource

import (
	"context"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
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
// Each implementation holds its own API client so the caller does not need to supply one.
type ResourceHandler interface {
	createResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) (CreateResult, error)
	getResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error
	updateResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error
	deleteResource(auth context.Context, instance *v1alpha1.DatadogGenericResource) error
	// refreshState fetches the live Datadog-side state of the resource.
	// Returns (nil, nil) for resource types that do not expose live state — the
	// caller should leave .status.state untouched in that case. On error, the
	// caller should preserve last-known state and surface the failure via the
	// StateSynced condition.
	refreshState(auth context.Context, instance *v1alpha1.DatadogGenericResource) (*string, error)
}

// forceSyncComparer is an optional extension of ResourceHandler. Handlers that
// implement it opt in to skipping the periodic force-sync update when the
// remote resource already matches the spec; other handlers always update.
type forceSyncComparer interface {
	// remoteMatchesSpec fetches the remote resource, in place of getResource,
	// and reports whether it already holds every field that updating it from
	// the spec would set. It returns an error only when the remote resource
	// cannot be fetched; a spec that cannot be compared reports false so that
	// the update runs and surfaces any problem with the spec.
	remoteMatchesSpec(auth context.Context, instance *v1alpha1.DatadogGenericResource) (bool, error)
}

// monitorStateHandler is an optional extension of ResourceHandler for
// resources whose live state is the state of a Datadog monitor. Handlers that
// implement it opt in to having that state fetched by the shared monitor state
// poller, in batches shared with other resources, instead of by refreshState.
type monitorStateHandler interface {
	// stateMonitorID returns the ID of the monitor whose state the resource
	// with the given status reports.
	stateMonitorID(status *v1alpha1.DatadogGenericResourceStatus) (int64, error)
	// stateFromMonitor returns the resource state that a polled monitor reports.
	stateFromMonitor(monitor datadogV1.Monitor) string
}
