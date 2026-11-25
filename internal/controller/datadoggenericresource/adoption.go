// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	ctrutils "github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/condition"
)

// Reasons of the Error condition for resources with spec.id.
const (
	// Permanent until the spec changes: no Datadog API call.
	reasonAdoptionInvalidID       = "AdoptionInvalidID"
	reasonAdoptionUnsupportedType = "AdoptionUnsupportedType"
	reasonIDMismatch              = "IDMismatch"
	// Adoption is blocked by another owner in the cluster.
	reasonAdoptionConflict = "AdoptionConflict"
	// A DatadogMonitor, DatadogDashboard or DatadogSLO manages the object, or
	// the Operator couldn't check whether one does.
	reasonAdoptionLegacyOwner      = "AdoptionLegacyOwner"
	reasonAdoptionLegacyCheckError = "AdoptionLegacyCheckError"
	// The GET of the resource to adopt failed.
	reasonAdoptionTargetNotFound = "AdoptionTargetNotFound"
	reasonAdoptionForbidden      = "AdoptionForbidden"
	reasonAdoptionGetError       = "AdoptionGetError"
	// The adopted resource is gone. It is never recreated while spec.id is set.
	reasonAdoptedResourceNotFound = "AdoptedResourceNotFound"

	adoptedReason      = "Adopted"
	adoptedEventReason = "Adopted"
)

// specIDGateReasons are the errors that clear on their own once the spec is
// fixed, without any Datadog API call to confirm it.
var specIDGateReasons = []string{reasonAdoptionInvalidID, reasonAdoptionUnsupportedType, reasonIDMismatch}

// IDIndexField indexes each DatadogGenericResource by type and ID, from both
// spec.id and status.id, to find other resources that manage or adopt the same
// Datadog object.
const IDIndexField = "ddgr.datadoghq.com/type-id"

func idIndexKey(resourceType v1alpha1.SupportedResourcesType, id string) string {
	return string(resourceType) + "/" + id
}

// IDIndexValues is the indexer function of IDIndexField.
func IDIndexValues(obj client.Object) []string {
	ddgr, ok := obj.(*v1alpha1.DatadogGenericResource)
	if !ok {
		return nil
	}
	var keys []string
	if ddgr.Spec.ID != "" {
		keys = append(keys, idIndexKey(ddgr.Spec.Type, ddgr.Spec.ID))
	}
	if ddgr.Status.Id != "" && ddgr.Status.Id != ddgr.Spec.ID {
		keys = append(keys, idIndexKey(ddgr.Spec.Type, ddgr.Status.Id))
	}
	return keys
}

var (
	integerIDPattern  = regexp.MustCompile(`^[1-9][0-9]{0,18}$`)
	publicIDPattern   = regexp.MustCompile(`^[a-z0-9]{3}-[a-z0-9]{3}-[a-z0-9]{3}$`)
	uuidPattern       = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
	genericIDPattern  = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	errInvalidIDValue = errors.New("invalid spec.id")
)

// validateSpecID checks that spec.id can identify a resource of the given
// type. It returns the Error condition reason with the error.
func validateSpecID(resourceType v1alpha1.SupportedResourcesType, id string) (string, error) {
	var pattern *regexp.Regexp
	var format string
	switch resourceType {
	case v1alpha1.SoftwareCatalogEntity:
		return reasonAdoptionUnsupportedType, fmt.Errorf("spec.id is not supported for %s: entities are upserted by kind, namespace and name, so creating the resource already updates an existing entity", resourceType)
	case v1alpha1.Monitor, v1alpha1.Notebook:
		pattern, format = integerIDPattern, "a positive integer"
	case v1alpha1.Dashboard, v1alpha1.SyntheticsAPITest, v1alpha1.SyntheticsBrowserTest:
		pattern, format = publicIDPattern, "of the form abc-def-ghi"
	case v1alpha1.Downtime, v1alpha1.MonitorNotificationRule:
		pattern, format = uuidPattern, "a UUID"
	default:
		pattern, format = genericIDPattern, "made of letters, digits, '-' and '_'"
	}
	if !pattern.MatchString(id) {
		return reasonAdoptionInvalidID, fmt.Errorf("%w %q: a %s ID must be %s", errInvalidIDValue, id, resourceType, format)
	}
	return "", nil
}

// specHash is the hash of the spec without spec.id, which doesn't change the
// payload sent to Datadog. Writing the ID back into a manifest therefore
// doesn't trigger an update, and resources without spec.id keep the hash
// they had before the field existed.
func specHash(spec *v1alpha1.DatadogGenericResourceSpec) (string, error) {
	payload := *spec
	payload.ID = ""
	return comparison.GenerateMD5ForSpec(&payload)
}

// checkSpecID validates spec.id against status.id. When it returns false,
// the reconcile must stop without calling the Datadog API: the error is in
// status and only a spec change can fix it.
func checkSpecID(instance *v1alpha1.DatadogGenericResource, status *v1alpha1.DatadogGenericResourceStatus, now metav1.Time) bool {
	if instance.Spec.ID != "" {
		if reason, err := validateSpecID(instance.Spec.Type, instance.Spec.ID); err != nil {
			updateErrStatus(status, now, v1alpha1.DatadogSyncStatusValidateError, reason, err)
			return false
		}
		if instance.Status.Id != "" && instance.Status.Id != instance.Spec.ID {
			err := fmt.Errorf("spec.id %q doesn't match the managed resource %q: spec.id can't be changed once the resource is managed; revert it, or delete and recreate the DatadogGenericResource", instance.Spec.ID, instance.Status.Id)
			updateErrStatus(status, now, v1alpha1.DatadogSyncStatusValidateError, reasonIDMismatch, err)
			return false
		}
	}
	// The spec is valid again: clear a previous spec.id error that no other
	// operation would clear.
	if c := meta.FindStatusCondition(status.Conditions, string(condition.DatadogConditionTypeError)); c != nil && slices.Contains(specIDGateReasons, c.Reason) {
		condition.RemoveStatusCondition(&status.Conditions, condition.DatadogConditionTypeError)
		if status.SyncStatus == v1alpha1.DatadogSyncStatusValidateError {
			status.SyncStatus = ""
			if status.Id != "" {
				status.SyncStatus = v1alpha1.DatadogSyncStatusOK
			}
		}
	}
	return true
}

// retryAfterError is an error that is retried after a fixed delay rather than
// the default backoff.
type retryAfterError struct {
	err   error
	after time.Duration
}

func (e *retryAfterError) Error() string { return e.err.Error() }
func (e *retryAfterError) Unwrap() error { return e.err }

// adopt takes over the existing Datadog resource identified by spec.id. It
// only records the ID: CurrentHash stays empty, so the next reconcile takes
// the usual update path and writes jsonSpec. Persisting the ID first means a
// crash before that update can't lose it.
func (r *Reconciler) adopt(ctx context.Context, auth context.Context, handler ResourceHandler, instance *v1alpha1.DatadogGenericResource, status *v1alpha1.DatadogGenericResourceStatus, now metav1.Time) error {
	logger := ctrl.LoggerFrom(ctx)
	id := instance.Spec.ID

	// Same stale-cache guard as create(): the cache may not have the status
	// written by the previous reconcile yet.
	latest := &v1alpha1.DatadogGenericResource{}
	if err := r.apiReader.Get(ctx, client.ObjectKeyFromObject(instance), latest); err != nil {
		if apierrors.IsNotFound(err) {
			return errCreateSkipped
		}
		return fmt.Errorf("%w: %w", errPreCreateRead, err)
	}
	if latest.UID != instance.UID || !latest.DeletionTimestamp.IsZero() || latest.Status.Id != "" || latest.Spec.ID != id {
		logger.Info("Skipping adoption because DatadogGenericResource changed; requeuing")
		return errCreateSkipped
	}

	if owner, err := r.conflictingOwner(ctx, instance); err != nil {
		updateErrStatus(status, now, v1alpha1.DatadogSyncStatusGetError, reasonAdoptionConflict, err)
		return err
	} else if owner != "" {
		err := fmt.Errorf("can't adopt %s %s: %s", instance.Spec.Type, id, owner)
		updateErrStatus(status, now, v1alpha1.DatadogSyncStatusGetError, reasonAdoptionConflict, err)
		return &retryAfterError{err: err, after: r.forceSyncPeriod}
	}

	// Two controllers must never manage the same object. Retry at the requeue
	// period rather than the force sync period: during a migration, the legacy
	// CR is expected to go away shortly.
	if owner, err := r.legacyOwners.owner(ctx, instance.Spec.Type, id); err != nil {
		updateErrStatus(status, now, v1alpha1.DatadogSyncStatusGetError, reasonAdoptionLegacyCheckError, err)
		return &retryAfterError{err: err, after: r.requeuePeriod}
	} else if owner != "" {
		err := fmt.Errorf("can't adopt %s %s: %s", instance.Spec.Type, id, owner)
		logger.Info("Adoption blocked by a legacy resource", "id", id, "type", instance.Spec.Type, "reason", owner)
		updateErrStatus(status, now, v1alpha1.DatadogSyncStatusGetError, reasonAdoptionLegacyOwner, err)
		return &retryAfterError{err: err, after: r.requeuePeriod}
	}

	probe := instance.DeepCopy()
	probe.Status.Id = id
	get := handler.getResource
	if g, ok := handler.(adoptionGetter); ok {
		get = g.getResourceToAdopt
	}
	if err := get(auth, probe); err != nil {
		reason := reasonAdoptionGetError
		switch apiStatusCode(err) {
		case http.StatusNotFound:
			reason = reasonAdoptionTargetNotFound
			err = fmt.Errorf("%s %s not found in Datadog; it is not created while spec.id is set: %w", instance.Spec.Type, id, err)
		case http.StatusUnauthorized, http.StatusForbidden:
			reason = reasonAdoptionForbidden
		}
		logger.Error(err, "error getting the resource to adopt", "id", id, "type", instance.Spec.Type)
		updateErrStatus(status, now, v1alpha1.DatadogSyncStatusGetError, reason, err)
		return err
	}

	status.Id = id
	status.CurrentHash = ""
	status.RemoteDefinitionHash = ""
	status.SyncStatus = v1alpha1.DatadogSyncStatusOK
	condition.UpdateStatusConditions(&status.Conditions, now, condition.DatadogConditionTypeAdopted, metav1.ConditionTrue, adoptedReason, fmt.Sprintf("Adopted existing %s %s", instance.Spec.Type, id))
	condition.RemoveStatusCondition(&status.Conditions, condition.DatadogConditionTypeError)
	r.recorder.Event(instance, corev1.EventTypeNormal, adoptedEventReason, fmt.Sprintf("Adopted existing %s %s; the next sync writes jsonSpec to it", instance.Spec.Type, id))
	logger.Info("Adopted existing Datadog resource", "id", id, "type", instance.Spec.Type)
	return nil
}

// conflictingOwner returns a description of another DatadogGenericResource in
// the cluster that already manages, or has priority to adopt, the object
// identified by instance.Spec.ID, or an empty string if there is none.
//
// A resource that already manages the object wins. Between resources that are
// both waiting to adopt it, the oldest wins, then the first by namespace and
// name, so concurrent reconciles agree on the outcome.
func (r *Reconciler) conflictingOwner(ctx context.Context, instance *v1alpha1.DatadogGenericResource) (string, error) {
	list := &v1alpha1.DatadogGenericResourceList{}
	if err := r.client.List(ctx, list, client.MatchingFields{IDIndexField: idIndexKey(instance.Spec.Type, instance.Spec.ID)}); err != nil {
		return "", fmt.Errorf("listing DatadogGenericResources with the same ID: %w", err)
	}
	for i := range list.Items {
		other := &list.Items[i]
		if (other.Namespace == instance.Namespace && other.Name == instance.Name) || other.Spec.Type != instance.Spec.Type {
			continue
		}
		name := other.Namespace + "/" + other.Name
		if other.Status.Id == instance.Spec.ID {
			// Its finalizer deletes the object, so it still blocks while terminating.
			return fmt.Sprintf("DatadogGenericResource %s already manages it", name), nil
		}
		if other.Status.Id != "" || other.Spec.ID != instance.Spec.ID || !other.DeletionTimestamp.IsZero() {
			// It manages another object, or it will never adopt this one.
			continue
		}
		if adoptsFirst(other, instance) {
			return fmt.Sprintf("DatadogGenericResource %s also adopts it and takes precedence", name), nil
		}
	}
	return "", nil
}

// adoptsFirst reports whether a takes precedence over b to adopt an object.
func adoptsFirst(a, b *v1alpha1.DatadogGenericResource) bool {
	if !a.CreationTimestamp.Equal(&b.CreationTimestamp) {
		return a.CreationTimestamp.Before(&b.CreationTimestamp)
	}
	return a.Namespace+"/"+a.Name < b.Namespace+"/"+b.Name
}

// setAdoptedResourceNotFound reports that the resource identified by spec.id
// no longer exists in Datadog.
func setAdoptedResourceNotFound(status *v1alpha1.DatadogGenericResourceStatus, now metav1.Time, instance *v1alpha1.DatadogGenericResource, err error) {
	err = fmt.Errorf("%s %s no longer exists in Datadog and is not recreated while spec.id is set; restore it, or remove spec.id to let the Operator create a new one: %w", instance.Spec.Type, instance.Spec.ID, err)
	updateErrStatus(status, now, v1alpha1.DatadogSyncStatusGetError, reasonAdoptedResourceNotFound, err)
}

// apiStatusCode returns the HTTP status code of a Datadog API error, or 0.
func apiStatusCode(err error) int {
	if apiErr, ok := errors.AsType[*ctrutils.APIError](err); ok && apiErr.StatusCode != 0 {
		return apiErr.StatusCode
	}
	if strings.Contains(err.Error(), ctrutils.NotFoundString) {
		return http.StatusNotFound
	}
	return 0
}
