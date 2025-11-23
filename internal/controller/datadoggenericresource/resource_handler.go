package datadoggenericresource

import (
	"context"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

type ResourceHandler interface {
	createResourcefunc(auth context.Context, r *Reconciler, logger logr.Logger, instance *v1alpha1.DatadogGenericResource, status *v1alpha1.DatadogGenericResourceStatus, now metav1.Time, hash string) error
	getResourcefunc(auth context.Context, r *Reconciler, instance *v1alpha1.DatadogGenericResource) error
	updateResourcefunc(auth context.Context, r *Reconciler, instance *v1alpha1.DatadogGenericResource) error
	deleteResourcefunc(auth context.Context, r *Reconciler, instance *v1alpha1.DatadogGenericResource) error
}
