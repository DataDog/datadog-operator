// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagent

import (
	"context"
	"time"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/condition"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	edsdatadoghqv1alpha1 "github.com/datadog/extendeddaemonset/pkg/apis/datadoghq/v1alpha1"

	"github.com/go-logr/logr"
	"github.com/spf13/pflag"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var (
	log                           = logf.Log.WithName("DatadogAgent")
	supportExtendedDaemonset bool = false
)

const (
	defaultRequeuPeriod = 15 * time.Second
)

func init() {
	pflag.BoolVarP(&supportExtendedDaemonset, "supportExtendedDaemonset", "", false, "support ExtendedDaemonset")
}

// Add creates a new DatadogAgent Controller and adds it to the Manager. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	reconciler, forwarders, err := newReconciler(mgr)
	if err != nil {
		return err
	}
	return add(mgr, reconciler, forwarders.Register)
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) (reconcile.Reconciler, metricForwardersManager, error) {
	forwarders := datadog.NewForwardersManager(mgr.GetClient())
	reconciler := &ReconcileDatadogAgent{
		client:     mgr.GetClient(),
		scheme:     mgr.GetScheme(),
		recorder:   mgr.GetEventRecorderFor("DatadogAgent"),
		forwarders: forwarders,
	}
	return reconciler, forwarders, mgr.Add(forwarders)
}

type metricForwardersManager interface {
	Register(datadog.MonitoredObject)
	Unregister(datadog.MonitoredObject)
	ProcessError(datadog.MonitoredObject, error)
	ProcessEvent(datadog.MonitoredObject, datadog.Event)
	MetricsForwarderStatusForObj(obj datadog.MonitoredObject) *datadoghqv1alpha1.DatadogAgentCondition
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler, registerFunc func(datadog.MonitoredObject)) error {
	// Create a new controller
	c, err := controller.New("datadogdeployment-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// onCreate is used to register a dedicated metrics forwarder
	// when creating a new DatadogAgent
	onCreate := predicate.Funcs{
		CreateFunc: func(ev event.CreateEvent) bool {
			// Register a metrics forwarder that corresponds
			// to this DatadogAgent instance
			registerFunc(ev.Meta)

			// Never ignore a creation event
			return true
		},
	}

	// Watch for changes to primary resource DatadogAgent
	err = c.Watch(&source.Kind{Type: &datadoghqv1alpha1.DatadogAgent{}}, &handler.EnqueueRequestForObject{}, onCreate)
	if err != nil {
		return err
	}

	if supportExtendedDaemonset {
		// Watch for changes to secondary resource ExtendedDaemonSet and requeue the owner DatadogAgent
		err = c.Watch(&source.Kind{Type: &edsdatadoghqv1alpha1.ExtendedDaemonSet{}}, &handler.EnqueueRequestForOwner{
			IsController: true,
			OwnerType:    &datadoghqv1alpha1.DatadogAgent{},
		})
		if err != nil {
			return err
		}
	}

	// Watch for changes to secondary resource Secret and requeue the owner DatadogAgent
	err = c.Watch(&source.Kind{Type: &corev1.Secret{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &datadoghqv1alpha1.DatadogAgent{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource ConfigMap and requeue the owner DatadogAgent
	err = c.Watch(&source.Kind{Type: &corev1.ConfigMap{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &datadoghqv1alpha1.DatadogAgent{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource DaemonSet and requeue the owner DatadogAgent
	err = c.Watch(&source.Kind{Type: &appsv1.DaemonSet{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &datadoghqv1alpha1.DatadogAgent{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Deployment and requeue the owner DatadogAgent
	err = c.Watch(&source.Kind{Type: &appsv1.Deployment{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &datadoghqv1alpha1.DatadogAgent{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource Role and requeue the owner DatadogAgent
	err = c.Watch(&source.Kind{Type: &rbacv1.Role{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &datadoghqv1alpha1.DatadogAgent{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource RoleBinding and requeue the owner DatadogAgent
	err = c.Watch(&source.Kind{Type: &rbacv1.RoleBinding{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &datadoghqv1alpha1.DatadogAgent{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource ServiceAccount and requeue the owner DatadogAgent
	err = c.Watch(&source.Kind{Type: &corev1.ServiceAccount{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &datadoghqv1alpha1.DatadogAgent{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource ClusterRole and requeue the owner DatadogAgent
	err = c.Watch(&source.Kind{Type: &rbacv1.ClusterRole{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &datadoghqv1alpha1.DatadogAgent{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource ClusterRoleBinding and requeue the owner DatadogAgent
	err = c.Watch(&source.Kind{Type: &rbacv1.ClusterRoleBinding{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &datadoghqv1alpha1.DatadogAgent{},
	})
	if err != nil {
		return err
	}

	// Watch for changes to secondary resource PodDisruptionBudget and requeue the owner DatadogAgent
	err = c.Watch(&source.Kind{Type: &policyv1.PodDisruptionBudget{}}, &handler.EnqueueRequestForOwner{
		IsController: true,
		OwnerType:    &datadoghqv1alpha1.DatadogAgent{},
	})
	if err != nil {
		return err
	}

	return nil
}

// blank assignment to verify that ReconcileDatadogAgent implements reconcile.Reconciler
var _ reconcile.Reconciler = &ReconcileDatadogAgent{}

// ReconcileDatadogAgent reconciles a DatadogAgent object
type ReconcileDatadogAgent struct {
	// This client, initialized using mgr.Client() above, is a split client
	// that reads objects from the cache and writes to the apiserver
	client     client.Client
	scheme     *runtime.Scheme
	recorder   record.EventRecorder
	forwarders metricForwardersManager
}

// Reconcile reads that state of the cluster for a DatadogAgent object and makes changes based on the state read
// and what is in the DatadogAgent.Spec
// Note:
// The Controller will requeue the Request to be processed again if the returned error is non-nil or
// Result.Requeue is true, otherwise upon completion it will remove the work from the queue.
//
// Reconcile wraps internelReconcile to send metrics based on reconcile errors
func (r *ReconcileDatadogAgent) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	resp, err := r.internelReconcile(request)
	r.forwarders.ProcessError(getMonitoredObj(request), err)
	return resp, err
}

func (r *ReconcileDatadogAgent) internelReconcile(request reconcile.Request) (reconcile.Result, error) {
	reqLogger := log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling DatadogAgent")

	// Fetch the DatadogAgent instance
	instance := &datadoghqv1alpha1.DatadogAgent{}
	err := r.client.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}

	if result, err := r.handleFinalizer(reqLogger, instance); shouldReturn(result, err) {
		return result, err
	}

	if !datadoghqv1alpha1.IsDefaultedDatadogAgent(instance) {
		reqLogger.Info("Defaulting values")
		defaultedInstance := datadoghqv1alpha1.DefaultDatadogAgent(instance)
		err = r.client.Update(context.TODO(), defaultedInstance)
		if err != nil {
			reqLogger.Error(err, "failed to update DatadogAgent")
			return reconcile.Result{}, err
		}
		// DatadogAgent is now defaulted return and requeue
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
	if !result.Requeue && result.RequeueAfter == 0 {
		result.RequeueAfter = defaultRequeuPeriod
	}
	return r.updateStatusIfNeeded(reqLogger, instance, newStatus, result, err)
}

type reconcileFuncInterface func(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error)

func (r *ReconcileDatadogAgent) updateStatusIfNeeded(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus, result reconcile.Result, currentError error) (reconcile.Result, error) {
	now := metav1.NewTime(time.Now())
	condition.UpdateDatadogAgentStatusConditionsFailure(newStatus, now, datadoghqv1alpha1.ConditionTypeReconcileError, currentError)
	if currentError == nil {
		condition.UpdateDatadogAgentStatusConditions(newStatus, now, datadoghqv1alpha1.ConditionTypeActive, corev1.ConditionTrue, "DatadogAgent reconcile ok", false)
	} else {
		condition.UpdateDatadogAgentStatusConditions(newStatus, now, datadoghqv1alpha1.ConditionTypeActive, corev1.ConditionFalse, "DatadogAgent reconcile error", false)
	}

	// get metrics forwarder status
	if metricsCondition := r.forwarders.MetricsForwarderStatusForObj(agentdeployment); metricsCondition != nil {
		condition.SetDatadogAgentStatusCondition(newStatus, metricsCondition)
	}

	if !apiequality.Semantic.DeepEqual(&agentdeployment.Status, newStatus) {
		updateAgentDeployment := agentdeployment.DeepCopy()
		updateAgentDeployment.Status = *newStatus
		if err := r.client.Status().Update(context.TODO(), updateAgentDeployment); err != nil {
			if apierrors.IsConflict(err) {
				logger.V(1).Info("unable to update DatadogAgent status due to update conflict")
				return reconcile.Result{RequeueAfter: time.Second}, nil
			}
			logger.Error(err, "unable to update DatadogAgent status")
			return reconcile.Result{}, err
		}
	}

	return result, currentError
}
