// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"context"

	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlbuilder "sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// DatadogAgentInternalReconciler reconciles a DatadogAgentInternal object.
type DatadogAgentInternalReconciler struct {
	client.Client
	PlatformInfo kubernetes.PlatformInfo
	Scheme       *runtime.Scheme
	Recorder     record.EventRecorder
	Options      datadogagentinternal.ReconcilerOptions
	internal     *datadogagentinternal.Reconciler
}

// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogagentinternals,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogagentinternals/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=datadoghq.com,resources=datadogagentinternals/finalizers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;patch;delete
// +kubebuilder:rbac:groups=apps,resources=controllerrevisions,verbs=get;list;watch

// Reconcile loop for DatadogAgent.
func (r *DatadogAgentInternalReconciler) Reconcile(ctx context.Context, ddai *v1alpha1.DatadogAgentInternal) (ctrl.Result, error) {
	// Get the logger from context (already has namespace, name, reconcileID from controller-runtime)
	// Add our controller name and kind - this enriched logger will be available to all downstream functions
	logger := ctrl.LoggerFrom(ctx).WithName("controllers").WithName("DatadogAgentInternal").WithValues("kind", ddai.Kind)
	ctx = ctrl.LoggerInto(ctx, logger)
	return r.internal.Reconcile(ctx, ddai)
}

// SetupWithManager creates a new DatadogAgent controller.
func (r *DatadogAgentInternalReconciler) SetupWithManager(mgr ctrl.Manager, metricForwardersMgr datadog.MetricsForwardersManager) error {
	generationChanged := ctrlbuilder.WithPredicates(predicate.GenerationChangedPredicate{})
	builder := ctrl.NewControllerManagedBy(mgr).
		Owns(&corev1.Secret{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&appsv1.DaemonSet{}, generationChanged).
		Owns(&appsv1.Deployment{}).
		Owns(&rbacv1.Role{}).
		Owns(&rbacv1.RoleBinding{}).
		Owns(&corev1.ServiceAccount{}).
		// We let PlatformInfo supply PDB object based on the current API version
		Owns(r.PlatformInfo.CreatePDBObject()).
		Owns(&networkingv1.NetworkPolicy{})

	// DatadogAgent is namespaced whereas ClusterRole and ClusterRoleBinding are
	// cluster-scoped. That means that DatadogAgent cannot be their owner, and
	// we cannot use .Owns().
	handlerEnqueue := handler.EnqueueRequestsFromMapFunc(enqueueIfOwnedByDatadogAgentInternal)
	builder.Watches(&rbacv1.ClusterRole{}, handlerEnqueue)
	builder.Watches(&rbacv1.ClusterRoleBinding{}, handlerEnqueue)
	builder.Watches(
		&corev1.Pod{},
		handler.EnqueueRequestsFromMapFunc(enqueueDatadogAgentInternalForPod(mgr.GetAPIReader())),
		ctrlbuilder.WithPredicates(resourceFallbackPodPredicate()),
	)

	if r.Options.ExtendedDaemonsetOptions.Enabled {
		builder = builder.Owns(&edsdatadoghqv1alpha1.ExtendedDaemonSet{})
	}

	if r.Options.SupportCilium {
		policy := &unstructured.Unstructured{}
		policy.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "cilium.io",
			Version: "v2",
			Kind:    "CiliumNetworkPolicy",
		})
		builder = builder.Owns(policy)
	}

	var builderOptions []ctrlbuilder.ForOption
	if r.Options.OperatorMetricsEnabled {
		builderOptions = append(builderOptions, ctrlbuilder.WithPredicates(predicate.Funcs{
			// On `DatadogAgent` object creation, we register a metrics forwarder for it.
			CreateFunc: func(e event.CreateEvent) bool {
				metricForwardersMgr.Register(e.Object)
				return true
			},
		}))
	}
	builderOptions = append(builderOptions, ctrlbuilder.WithPredicates(datadogAgentInternalEventPredicate()))

	or := reconcile.AsReconciler[*v1alpha1.DatadogAgentInternal](r.Client, r)
	if err := builder.For(&datadoghqv1alpha1.DatadogAgentInternal{}, builderOptions...).Complete(or); err != nil {
		return err
	}

	internalOptions := r.Options
	internalOptions.APIReader = mgr.GetAPIReader()
	r.internal = datadogagentinternal.NewReconciler(internalOptions, r.Client, r.PlatformInfo, r.Scheme, r.Recorder, metricForwardersMgr)

	return nil
}

func datadogAgentInternalEventPredicate() predicate.Predicate {
	return predicate.Or(
		predicate.GenerationChangedPredicate{},
		datadogAnnotationChangedPredicate(),
	)
}

func enqueueDatadogAgentInternalForPod(reader client.Reader) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		pod, ok := obj.(*corev1.Pod)
		if !ok || pod.Labels[apicommon.AgentDeploymentComponentLabelKey] != constants.DefaultAgentResourceSuffix {
			return nil
		}
		podOwner := metav1.GetControllerOf(pod)
		if podOwner == nil || podOwner.APIVersion != appsv1.SchemeGroupVersion.String() || podOwner.Kind != "DaemonSet" {
			return nil
		}
		ds := &appsv1.DaemonSet{}
		if err := reader.Get(ctx, client.ObjectKey{Namespace: pod.Namespace, Name: podOwner.Name}, ds); err != nil || ds.UID != podOwner.UID {
			return nil
		}
		ddaiOwner := metav1.GetControllerOf(ds)
		if ddaiOwner == nil || ddaiOwner.APIVersion != datadoghqv1alpha1.GroupVersion.String() || ddaiOwner.Kind != "DatadogAgentInternal" {
			return nil
		}
		return []reconcile.Request{{NamespacedName: client.ObjectKey{Namespace: ds.Namespace, Name: ddaiOwner.Name}}}
	}
}

func resourceFallbackPodPredicate() predicate.Predicate {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			pod, ok := e.Object.(*corev1.Pod)
			return ok && resourceFallbackSchedulingCondition(pod) != nil
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldPod, oldOK := e.ObjectOld.(*corev1.Pod)
			newPod, newOK := e.ObjectNew.(*corev1.Pod)
			if !oldOK || !newOK {
				return false
			}
			return resourceFallbackConditionChanged(oldPod, newPod, corev1.PodScheduled) ||
				resourceFallbackConditionChanged(oldPod, newPod, corev1.PodReady) ||
				containerRolloutStatusChanged(oldPod.Status.InitContainerStatuses, newPod.Status.InitContainerStatuses) ||
				containerRolloutStatusChanged(oldPod.Status.ContainerStatuses, newPod.Status.ContainerStatuses)
		},
		DeleteFunc:  func(event.DeleteEvent) bool { return true },
		GenericFunc: func(event.GenericEvent) bool { return false },
	}
}

func containerRolloutStatusChanged(oldStatuses, newStatuses []corev1.ContainerStatus) bool {
	if len(oldStatuses) != len(newStatuses) {
		return true
	}
	oldByName := make(map[string]corev1.ContainerStatus, len(oldStatuses))
	for i := range oldStatuses {
		oldByName[oldStatuses[i].Name] = oldStatuses[i]
	}
	for i := range newStatuses {
		old, found := oldByName[newStatuses[i].Name]
		if !found || !apiequality.Semantic.DeepEqual(old.Started, newStatuses[i].Started) || old.RestartCount != newStatuses[i].RestartCount ||
			(old.State.Running == nil) != (newStatuses[i].State.Running == nil) ||
			(old.State.Terminated == nil) != (newStatuses[i].State.Terminated == nil) {
			return true
		}
	}
	return false
}

func resourceFallbackConditionChanged(oldPod, newPod *corev1.Pod, conditionType corev1.PodConditionType) bool {
	oldCondition := podCondition(oldPod, conditionType)
	newCondition := podCondition(newPod, conditionType)
	if oldCondition == nil || newCondition == nil {
		return oldCondition != newCondition
	}
	return oldCondition.Status != newCondition.Status || oldCondition.Reason != newCondition.Reason || oldCondition.Message != newCondition.Message
}

func podCondition(pod *corev1.Pod, conditionType corev1.PodConditionType) *corev1.PodCondition {
	for i := range pod.Status.Conditions {
		if pod.Status.Conditions[i].Type == conditionType {
			return &pod.Status.Conditions[i]
		}
	}
	return nil
}

func resourceFallbackSchedulingCondition(pod *corev1.Pod) *corev1.PodCondition {
	return podCondition(pod, corev1.PodScheduled)
}

func enqueueIfOwnedByDatadogAgentInternal(ctx context.Context, obj client.Object) []reconcile.Request {
	labels := obj.GetLabels()

	if labels[kubernetes.AppKubernetesManageByLabelKey] != "datadog-operator" {
		return nil
	}

	partOfLabelVal := object.PartOfLabelValue{Value: labels[kubernetes.AppKubernetesPartOfLabelKey]}
	owner := partOfLabelVal.NamespacedName()

	return []reconcile.Request{{NamespacedName: owner}}
}
