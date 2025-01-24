package datadoggenericresource

import (
	"context"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ResourceHandler interface {
	createResourcefunc(r *Reconciler, logger logr.Logger, instance *v1alpha1.DatadogGenericResource, status *v1alpha1.DatadogGenericResourceStatus, now metav1.Time, hash string) error
	getResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error
	updateResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error
	deleteResourcefunc(r *Reconciler, instance *v1alpha1.DatadogGenericResource) error
}

type CRUDClient interface {
	createResource(auth context.Context, i any) (any, error)
	getResource(auth context.Context, resourceID string) error
	updateResource(auth context.Context, resourceID string, i any) (any, error)
	deleteResource(auth context.Context, resourceID string) error
}
