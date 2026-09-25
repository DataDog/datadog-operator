// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogsyntheticsprivatelocation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	datadogV1 "github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlhandler "sigs.k8s.io/controller-runtime/pkg/handler"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/finalizer"
	"github.com/DataDog/datadog-operator/pkg/config"
	ctrutils "github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	condition "github.com/DataDog/datadog-operator/pkg/controller/utils/condition"
	"github.com/DataDog/datadog-operator/pkg/datadogclient"
)

const (
	defaultRequeuePeriod    = 60 * time.Second
	defaultErrRequeuePeriod = 5 * time.Second
	defaultForceSyncPeriod  = 60 * time.Minute

	forceSyncPeriodEnvVar = "DD_SYNTHETICS_PRIVATE_LOCATION_FORCE_SYNC_PERIOD"

	eventReasonPrefix = "DatadogSyntheticsPrivateLocation"
)

// Reconciler reconciles DatadogSyntheticsPrivateLocation objects, managing both
// the remote Datadog private location and the in-cluster worker Deployment.
type Reconciler struct {
	client       client.Client
	scheme       *runtime.Scheme
	log          logr.Logger
	recorder     record.EventRecorder
	credsManager *config.CredentialManager

	ddClientSynthetics *datadogV1.SyntheticsApi

	requeuePeriod   time.Duration
	forceSyncPeriod time.Duration
}

type ReconcilerOptions struct {
	RequeuePeriod   time.Duration
	ForceSyncPeriod time.Duration
}

func NewReconciler(client client.Client, credsManager *config.CredentialManager, scheme *runtime.Scheme, log logr.Logger, recorder record.EventRecorder, opts ...ReconcilerOptions) *Reconciler {
	options := ReconcilerOptions{}
	if len(opts) > 0 {
		options = opts[0]
	}

	requeuePeriod := options.RequeuePeriod
	if requeuePeriod <= 0 {
		requeuePeriod = defaultRequeuePeriod
	}
	forceSyncPeriod := options.ForceSyncPeriod
	if forceSyncPeriod <= 0 {
		forceSyncPeriod = forceSyncPeriodFromEnv(log, defaultForceSyncPeriod)
	}

	return &Reconciler{
		client:             client,
		credsManager:       credsManager,
		scheme:             scheme,
		log:                log,
		recorder:           recorder,
		ddClientSynthetics: datadogclient.InitSyntheticsPrivateLocationClient(),
		requeuePeriod:      requeuePeriod,
		forceSyncPeriod:    forceSyncPeriod,
	}
}

// Reconcile is the entry point called by the controller wrapper with the
// freshly fetched instance.
func (r *Reconciler) Reconcile(ctx context.Context, instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation) (ctrl.Result, error) {
	logger := r.log.WithValues("datadogsyntheticsprivatelocation", types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace})

	result, err := r.internalReconcile(ctx, logger, instance)
	// No status to persist on deletion: the finalizer has already removed the
	// instance (and the finalizer path never sets status).
	if instance.GetDeletionTimestamp().IsZero() {
		if patchErr := applyStatusPatch(ctx, r.client, instance); patchErr != nil {
			err = errors.Join(err, patchErr)
		}
	}
	return result, err
}

func (r *Reconciler) internalReconcile(ctx context.Context, logger logr.Logger, instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation) (ctrl.Result, error) {
	logger.Info("Reconciling DatadogSyntheticsPrivateLocation")
	now := metav1.NewTime(time.Now())

	// Reset conditions so the SSA status patch carries only conditions set
	// during this pass.
	instance.Status.Conditions = nil

	auth, credErr := r.credsManager.GetAuth()
	if credErr != nil {
		return ctrl.Result{RequeueAfter: defaultErrRequeuePeriod}, fmt.Errorf("unable to get credentials: %w", credErr)
	}

	fin := finalizer.NewFinalizer(logger, r.client, deleteResource(logger, auth, r.ddClientSynthetics), r.requeuePeriod, defaultErrRequeuePeriod)
	result, err := fin.HandleFinalizer(ctx, instance, instance.Status.ID, datadogSyntheticsPrivateLocationFinalizerName)
	if ctrutils.ShouldReturn(result, err) {
		return result, err
	}

	if specErr := validateSpec(instance); specErr != nil {
		logger.Error(specErr, "invalid DatadogSyntheticsPrivateLocation spec")
		setErrorCondition(instance, now, "InvalidSpec", specErr)
		instance.Status.SyncStatus = datadoghqv1alpha1.DatadogSyntheticsPrivateLocationSyncStatusSyncError
		// A spec fix arrives as a watch event; a retry would not make progress.
		return ctrl.Result{}, nil
	}

	newHash, err := comparison.GenerateMD5ForSpec(&instance.Spec)
	if err != nil {
		logger.Error(err, "error generating spec hash")
		return ctrl.Result{RequeueAfter: defaultErrRequeuePeriod}, err
	}

	shouldCreate := instance.Status.ID == ""
	shouldUpdate := false
	shouldForceSync := false

	if !shouldCreate {
		if newHash != instance.Status.CurrentHash {
			logger.V(1).Info("DatadogSyntheticsPrivateLocation manifest has changed")
			shouldUpdate = true
		} else if instance.Status.LastForceSyncTime == nil ||
			r.forceSyncPeriod-now.Sub(instance.Status.LastForceSyncTime.Time) <= 0 {
			shouldForceSync = true
		}
	}

	remoteOK := true

	switch {
	case shouldCreate:
		if err := r.create(auth, ctx, logger, instance, now, newHash); err != nil {
			return r.handleRemoteError(logger, instance, now, err, datadoghqv1alpha1.DatadogSyntheticsPrivateLocationSyncStatusCreateError)
		}

	case shouldUpdate, shouldForceSync:
		// Verify the private location still exists before touching it.
		pl, getErr := getPrivateLocation(auth, r.ddClientSynthetics, instance.Status.ID)
		if getErr != nil {
			return r.handleRemoteError(logger, instance, now, getErr, datadoghqv1alpha1.DatadogSyntheticsPrivateLocationSyncStatusUpdateError)
		}
		if pl == nil {
			// The private location was deleted out of band. Auto-recreating
			// would mint a new ID and orphan tests pinned to the old one, so
			// surface the drift and wait for a human decision.
			r.handleOutBandDeletion(logger, instance, now)
			remoteOK = false
			break
		}

		if _, err := updatePrivateLocation(auth, r.ddClientSynthetics, instance); err != nil {
			return r.handleRemoteError(logger, instance, now, err, datadoghqv1alpha1.DatadogSyntheticsPrivateLocationSyncStatusUpdateError)
		}

		if shouldUpdate {
			instance.Status.CurrentHash = newHash
		}
		instance.Status.LastForceSyncTime = &now
		instance.Status.SyncStatus = datadoghqv1alpha1.DatadogSyntheticsPrivateLocationSyncStatusOK
		setSyncedConditions(instance, now, condition.DatadogConditionTypeUpdated, "private location updated in Datadog")
		r.recorder.Eventf(instance, corev1.EventTypeNormal, eventReasonPrefix+"Updated", "Updated private location %s", instance.Status.ID)

	default:
		// No remote work due: refresh the remote view to detect drift.
		pl, getErr := getPrivateLocation(auth, r.ddClientSynthetics, instance.Status.ID)
		if getErr != nil {
			return r.handleRemoteError(logger, instance, now, getErr, datadoghqv1alpha1.DatadogSyntheticsPrivateLocationSyncStatusSyncError)
		}
		if pl == nil {
			r.handleOutBandDeletion(logger, instance, now)
			remoteOK = false
		} else {
			instance.Status.SyncStatus = datadoghqv1alpha1.DatadogSyntheticsPrivateLocationSyncStatusOK
			setSuccessConditions(instance, now)
		}
	}

	return r.ownedResourcesAndRequeue(ctx, logger, instance, now, remoteOK)
}

// create creates the remote private location and immediately persists the
// write-once worker config: the Datadog API never returns it again.
func (r *Reconciler) create(auth context.Context, kubeCtx context.Context, logger logr.Logger, instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation, now metav1.Time, newHash string) error {
	resp, err := createPrivateLocation(auth, r.ddClientSynthetics, instance)
	if err != nil {
		return err
	}

	// Record the ID as soon as the remote create succeeded: a failure from
	// here on must not retry creation, or Datadog would end up with duplicate
	// private locations.
	pl := resp.GetPrivateLocation()
	instance.Status.ID = pl.GetId()

	configJSON, err := json.Marshal(resp.GetConfig())
	if err != nil {
		return fmt.Errorf("marshalling private location config: %w", err)
	}

	creds, credErr := r.credsManager.GetCredentials()
	if credErr != nil {
		return credErr
	}

	if err := reconcileConfigSecret(kubeCtx, r.client, r.scheme, instance, configJSON, resolveSite(creds)); err != nil {
		return err
	}

	instance.Status.ConfigSecretName = configSecretName(instance)
	instance.Status.Created = new(now)
	instance.Status.CurrentHash = newHash
	instance.Status.LastForceSyncTime = &now
	instance.Status.SyncStatus = datadoghqv1alpha1.DatadogSyntheticsPrivateLocationSyncStatusOK
	setSyncedConditions(instance, now, condition.DatadogConditionTypeCreated, "private location created in Datadog")
	r.recorder.Eventf(instance, corev1.EventTypeNormal, eventReasonPrefix+"Created", "Created private location %s", instance.Status.ID)

	return nil
}

// ownedResourcesAndRequeue reconciles the owned ServiceAccount, Secret,
// Deployment and PodDisruptionBudget, then computes the next requeue.
func (r *Reconciler) ownedResourcesAndRequeue(ctx context.Context, logger logr.Logger, instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation, now metav1.Time, remoteOK bool) (ctrl.Result, error) {
	configOK := true

	saName, err := reconcileServiceAccount(ctx, r.client, r.scheme, instance)
	if err != nil {
		logger.Error(err, "error reconciling service account")
		return r.errorResult(instance, now, err)
	}

	creds, credErr := r.credsManager.GetCredentials()
	if credErr != nil {
		return r.errorResult(instance, now, credErr)
	}
	// Re-merge the worker config over the existing Secret in case
	// spec.worker changed. On the create path this is a no-op: the Secret was
	// just persisted with the same merged content.
	if configErr := reconcileConfigSecret(ctx, r.client, r.scheme, instance, nil, resolveSite(creds)); configErr != nil {
		logger.Error(configErr, "error reconciling config secret")
		r.handleConfigMissing(logger, instance, now, configErr)
		configOK = false
	}

	// The Deployment mounts the config Secret; if the write-once config is
	// missing the worker cannot run, so skip Deployment/PDB instead of
	// crash-looping pods on an unusable config.
	if configOK {
		if statusProbesRequested(instance) && !statusProbesSupported(instance) {
			logger.Info("Worker version does not support status probes, not enabling them", "tag", workerImageTag(instance), "minVersion", statusProbesMinWorkerVersion)
			r.recorder.Eventf(instance, corev1.EventTypeWarning, eventReasonPrefix+"StatusProbesUnsupported",
				"Status probes need worker version %s or later, image tag is %s: the probes are not enabled", statusProbesMinWorkerVersion, workerImageTag(instance))
		}

		depStatus, err := reconcileDeployment(ctx, r.client, r.scheme, instance, saName)
		if err != nil {
			logger.Error(err, "error reconciling deployment")
			return r.errorResult(instance, now, err)
		}
		instance.Status.Deployment = depStatus

		if err := reconcilePodDisruptionBudget(ctx, r.client, r.scheme, instance); err != nil {
			logger.Error(err, "error reconciling pod disruption budget")
			return r.errorResult(instance, now, err)
		}
	}

	instance.Status.ObservedGeneration = instance.Generation
	setReadyCondition(instance, now)

	if !configOK {
		return ctrl.Result{RequeueAfter: defaultErrRequeuePeriod}, nil
	}
	if !remoteOK {
		// Out-of-band deletion: re-check for restoration on the force-sync
		// cadence; nothing else makes progress until a human acts.
		return ctrl.Result{RequeueAfter: r.forceSyncPeriod, Priority: ptr.To(ctrlhandler.LowPriority)}, nil
	}

	// Scheduled maintenance, not an informer event: yield to new events.
	return ctrl.Result{RequeueAfter: r.nextRequeueAfter(instance, now), Priority: ptr.To(ctrlhandler.LowPriority)}, nil
}

// nextRequeueAfter returns the time until the next force sync, bounded by the
// requeue period so a lost watch event cannot stall reconciliation.
func (r *Reconciler) nextRequeueAfter(instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation, now metav1.Time) time.Duration {
	if instance.Status.LastForceSyncTime == nil {
		return r.requeuePeriod
	}
	untilForceSync := r.forceSyncPeriod - now.Sub(instance.Status.LastForceSyncTime.Time)
	if untilForceSync <= 0 || r.requeuePeriod < untilForceSync {
		return r.requeuePeriod
	}
	return untilForceSync
}

func (r *Reconciler) errorResult(instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation, now metav1.Time, err error) (ctrl.Result, error) {
	setErrorCondition(instance, now, "ReconcileError", err)
	return ctrl.Result{RequeueAfter: defaultErrRequeuePeriod}, err
}

// handleRemoteError maps remote API errors onto the status: permanent errors
// back off to the force-sync period, transient ones retry quickly.
func (r *Reconciler) handleRemoteError(logger logr.Logger, instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation, now metav1.Time, err error, syncStatus datadoghqv1alpha1.DatadogSyntheticsPrivateLocationSyncStatus) (ctrl.Result, error) {
	logger.Error(err, "error syncing private location with Datadog")
	instance.Status.SyncStatus = syncStatus
	setErrorCondition(instance, now, "SyncError", err)

	if ctrutils.IsPermanentAPIError(err) {
		return ctrl.Result{RequeueAfter: r.forceSyncPeriod}, nil
	}
	return ctrl.Result{RequeueAfter: defaultErrRequeuePeriod}, err
}

// handleOutBandDeletion records drift: the private location no longer exists
// in Datadog, and the operator refuses to auto-recreate it.
func (r *Reconciler) handleOutBandDeletion(logger logr.Logger, instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation, now metav1.Time) {
	logger.Info("Private location no longer exists in Datadog", "Private Location ID", instance.Status.ID)
	instance.Status.SyncStatus = datadoghqv1alpha1.DatadogSyntheticsPrivateLocationSyncStatusDeleted
	setErrorCondition(instance, now, reasonPrivateLocationDeleted, errors.New("the private location no longer exists in Datadog and will not be auto-recreated; recreate the DatadogSyntheticsPrivateLocation or restore the private location"))
	r.recorder.Eventf(instance, corev1.EventTypeWarning, eventReasonPrefix+"Deleted", "Private location %s no longer exists in Datadog", instance.Status.ID)
}

// handleConfigMissing records a lost worker config: the write-once secrets
// cannot be recovered from the Datadog API.
func (r *Reconciler) handleConfigMissing(logger logr.Logger, instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation, now metav1.Time, err error) {
	logger.Error(err, "worker config is missing and cannot be recovered")
	setErrorCondition(instance, now, reasonConfigMissing, errors.New("the worker config Secret is missing its Datadog-provided data and cannot be recovered; recreate the DatadogSyntheticsPrivateLocation"))
}

// forceSyncPeriodFromEnv returns the force-sync period configured via
// DD_SYNTHETICS_PRIVATE_LOCATION_FORCE_SYNC_PERIOD, or def when unset or invalid.
func forceSyncPeriodFromEnv(logger logr.Logger, def time.Duration) time.Duration {
	value := os.Getenv(forceSyncPeriodEnvVar)
	if value == "" {
		return def
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		logger.Error(err, "invalid force sync period, using default", "env", forceSyncPeriodEnvVar, "value", value, "default", def.String())
		return def
	}
	return parsed
}

// validateSpec checks spec invariants that the CRD schema cannot express.
func validateSpec(instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation) error {
	if instance.Spec.Name == "" || instance.Spec.Description == "" {
		return errors.New("spec.name and spec.description are required")
	}
	if _, err := parseConfigOverride(instance.GetAnnotations()[datadoghqv1alpha1.DatadogSPLWorkerConfigOverrideAnnotation]); err != nil {
		return err
	}
	if instance.Spec.Worker != nil && instance.Spec.Worker.PodDisruptionBudget != nil {
		pdb := instance.Spec.Worker.PodDisruptionBudget
		if pdb.Enabled {
			if (pdb.MinAvailable != nil) == (pdb.MaxUnavailable != nil) {
				return errors.New("spec.worker.podDisruptionBudget requires exactly one of minAvailable, maxUnavailable")
			}
		}
	}
	return nil
}
