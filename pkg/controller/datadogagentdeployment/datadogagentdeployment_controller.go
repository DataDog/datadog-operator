// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagentdeployment

import (
	"context"
	"time"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/condition"
	edsdatadoghqv1alpha1 "github.com/datadog/extendeddaemonset/pkg/apis/datadoghq/v1alpha1"

	"github.com/go-logr/logr"
	"github.com/spf13/pflag"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var (
	log                           = logf.Log.WithName("DataDogAgentDeployment")
	supportExtendedDaemonset bool = false
)

const (
	defaultRequeuPeriod = 15 * time.Second
)

func init() {
	pflag.BoolVarP(&supportExtendedDaemonset, "supportExtendedDaemonset", "", false, "support ExtendedDaemonset")
}

// Add creates a new DatadogAgentDeployment Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileDatadogAgentDeployment{client: mgr.GetClient(), scheme: mgr.GetScheme(), recorder: mgr.GetEventRecorderFor("DatadogAgentDeployment")}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("datadogdeployment-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to primary resource DatadogAgentDeployment
	err = c.Watch(&source.Kind{Type: &datadoghqv1alpha1.DatadogAgentDeployment{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	if supportExtendedDaemonset {
		// Watch for changes to secondary resource ExtendedDaemonSet and requeue the owner DatadogAgentDeployment
		err = c.Watch(&source.Kind{Type: &edsdatadoghqv1alpha1.ExtendedDaemonSet{}}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &datadoghqv1alpha1.DatadogAgentDeployment{},
		})
		if err != nil {
			return err
		}
	}

	// Watch for changes to secondary resource Secret and requeue the owner DatadogAgentDeployment
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &datadoghqv1alpha1.DatadogAgentDeployment{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource ConfigMap and requeue the owner DatadogAgentDeployment
	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &datadoghqv1alpha1.DatadogAgentDeployment{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource DaemonSet and requeue the owner DatadogAgentDeployment
	err = c.Watch(&source.Kind{Type: &appsv1.DaemonSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &datadoghqv1alpha1.DatadogAgentDeployment{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Deployment and requeue the owner DatadogAgentDeployment
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &datadoghqv1alpha1.DatadogAgentDeployment{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Role and requeue the owner DatadogAgentDeployment
	err = c.Watch(&source.Kind{Type: &rbacv1.Role{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &datadoghqv1alpha1.DatadogAgentDeployment{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource RoleBinding and requeue the owner DatadogAgentDeployment
	err = c.Watch(&source.Kind{Type: &rbacv1.RoleBinding{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &datadoghqv1alpha1.DatadogAgentDeployment{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource ServiceAccount and requeue the owner DatadogAgentDeployment
	err = c.Watch(&source.Kind{Type: &corev1.ServiceAccount{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &datadoghqv1alpha1.DatadogAgentDeployment{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource ClusterRole and requeue the owner DatadogAgentDeployment
	err = c.Watch(&source.Kind{Type: &rbacv1.ClusterRole{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &datadoghqv1alpha1.DatadogAgentDeployment{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource ClusterRoleBinding and requeue the owner DatadogAgentDeployment
	err = c.Watch(&source.Kind{Type: &rbacv1.ClusterRoleBinding{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &datadoghqv1alpha1.DatadogAgentDeployment{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileDatadogAgentDeployment implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileDatadogAgentDeployment{}

// ReconcileDatadogAgentDeployment reconciles a DatadogAgentDeployment object
type ReconcileDatadogAgentDeployment struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client   client.Client
	scheme   *runtime.Scheme
	recorder record.EventRecorder
}

// Reconcile reads that state of the cluster for a DatadogAgentDeployment object and makes changes based on the state read
// and what is in the DatadogAgentDeployment.Spec
// TODO(user): Modify this Reconcile function to implement your Controller logic.  This example creates
// a Pod as an example
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
func (r *ReconcileDatadogAgentDeployment) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling DatadogAgentDeployment")

	// Fetch the DatadogAgentDeployment instance
	instance := &datadoghqv1alpha1.DatadogAgentDeployment{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if !datadoghqv1alpha1.IsDefaultedDatadogAgentDeployment(instance) {
		reqLogger.Info("Defaulting values")
		defaultedInstance := datadoghqv1alpha1.DefaultDatadogAgentDeployment(instance)
		err = r.client.Update(context.TODO(), defaultedInstance)
		if err != nil {
			reqLogger.Error(err, "failed to update DatadogAgentDeployment")
			return reconcile.Result{}, err
		}
		// DatadogAgentDeployment is now defaulted return and requeue
		return reconcile.Result{Requeue: true}, nil
	}

	newStatus := instance.Status.DeepCopy()

	reconcileFuncs :=
		[]reconcileFuncInterface{
			r.reconcileClusterAgent,
			r.reconcileClusterChecksRunner,
			r.reconcileAgent,
		}
	var result reconcile.Result
	for _, reconcileFunc := range reconcileFuncs {
		result, err = reconcileFunc(reqLogger, instance, newStatus)
		if shouldReturn(result, err) {
			return r.updateStatusIfNeeded(reqLogger, instance, newStatus, result, err)
		}
	}

	// Always requeue
	if !result.Requeue {
		result.RequeueAfter = defaultRequeuPeriod
	}
	return r.updateStatusIfNeeded(reqLogger, instance, newStatus, result, err)
}

type reconcileFuncInterface func(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error)

func (r *ReconcileDatadogAgentDeployment) updateStatusIfNeeded(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgentDeployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus, result reconcile.Result, currentError error) (reconcile.Result, error) {
	now := metav1.NewTime(time.Now())
	condition.UpdateDatadogAgentDeploymentStatusConditionsFailure(newStatus, now, currentError)
	if currentError == nil {
		condition.UpdateDatadogAgentDeploymentStatusCondition(newStatus, now, datadoghqv1alpha1.ConditionTypeActive, corev1.ConditionTrue, "DatadogAgentDeployment reconcile ok", false)
	} else {
		condition.UpdateDatadogAgentDeploymentStatusCondition(newStatus, now, datadoghqv1alpha1.ConditionTypeActive, corev1.ConditionFalse, "DatadogAgentDeployment reconcile error", false)
	}

	if !apiequality.Semantic.DeepEqual(&agentdeployment.Status, newStatus) {
		updateAgentDeployment := agentdeployment.DeepCopy()
		updateAgentDeployment.Status = *newStatus
		if err := r.client.Status().Update(context.TODO(), updateAgentDeployment); err != nil {
			if errors.IsConflict(err) {
				logger.V(1).Info("unable to update DatadogAgentDeployment status due to update conflict")
				return reconcile.Result{RequeueAfter: time.Second}, nil
			}
			logger.Error(err, "unable to update DatadogAgentDeployment status")
			return reconcile.Result{}, err
		}
	}

	return result, currentError
}
