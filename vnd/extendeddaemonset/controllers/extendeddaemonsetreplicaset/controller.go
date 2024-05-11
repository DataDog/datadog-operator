// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package extendeddaemonsetreplicaset

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilserrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/flowcontrol"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/DataDog/extendeddaemonset/controllers/extendeddaemonsetreplicaset/conditions"
	"github.com/DataDog/extendeddaemonset/controllers/extendeddaemonsetreplicaset/strategy"
	"github.com/DataDog/extendeddaemonset/pkg/controller/utils"
)

// Reconciler is the internal reconciler for ExtendedDaemonSetReplicaSet.
type Reconciler struct {
	options           ReconcilerOptions
	client            client.Client
	scheme            *runtime.Scheme
	log               logr.Logger
	recorder          record.EventRecorder
	failedPodsBackOff *flowcontrol.Backoff
}

// ReconcilerOptions provides options read from command line.
type ReconcilerOptions struct {
	IsNodeAffinitySupported bool
}

// NewReconciler returns a reconciler for DatadogAgent.
func NewReconciler(options ReconcilerOptions, client client.Client, scheme *runtime.Scheme, log logr.Logger, recorder record.EventRecorder) (*Reconciler, error) {
	return &Reconciler{
		options:           options,
		client:            client,
		scheme:            scheme,
		log:               log,
		recorder:          recorder,
		failedPodsBackOff: flowcontrol.NewBackOff(10*time.Second, 15*time.Minute),
	}, nil
}

// Reconcile reads that state of the cluster for a ExtendedDaemonSetReplicaSet object and makes changes based on the state read
// and what is in the ExtendedDaemonSetReplicaSet.Spec.
func (r *Reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	now := metav1.NewTime(time.Now())
	rand := rand.Uint32()
	reqLogger := r.log.WithValues("Req.NS", request.Namespace, "Req.Name", request.Name, "Req.TS", now.Unix(), "Req.Rand", rand)
	reqLogger.Info("Reconciling ExtendedDaemonSetReplicaSet")

	// Fetch the ExtendedDaemonSetReplicaSet replicaSetInstance
	replicaSetInstance, needReturn, err := r.retrievedReplicaSet(request)
	if needReturn {
		return reconcile.Result{}, err
	}

	// First retrieve the Parent DDaemonset
	daemonsetInstance, err := r.getDaemonsetOwner(replicaSetInstance)
	if err != nil {
		return reconcile.Result{}, err
	}

	if !datadoghqv1alpha1.IsDefaultedExtendedDaemonSet(daemonsetInstance) {
		message := "Parent ExtendedDaemonSet is not defaulted, requeuing"
		reqLogger.Info(message)
		err = fmt.Errorf("parent ExtendedDaemonSet is not defaulted")
		newStatus := replicaSetInstance.Status.DeepCopy()
		// Updating the status with a new condition will trigger a new Event on the ExtendedDaemonSet-controller
		// and so the ExtendedDamonset will be defaulted.
		// It is better to only update a Resource Kind from its own controller, to avoid conccurent update.
		conditions.UpdateErrorCondition(newStatus, now, err, message)
		err = r.updateReplicaSet(replicaSetInstance, newStatus)

		return reconcile.Result{RequeueAfter: time.Second}, err
	}

	lastResyncTimeStampCond := conditions.GetExtendedDaemonSetReplicaSetStatusCondition(&replicaSetInstance.Status, datadoghqv1alpha1.ConditionTypeLastFullSync)
	if lastResyncTimeStampCond != nil {
		nextSyncTS := lastResyncTimeStampCond.LastUpdateTime.Add(daemonsetInstance.Spec.Strategy.ReconcileFrequency.Duration)
		if nextSyncTS.After(now.Time) {
			requeueDuration := nextSyncTS.Sub(now.Time)
			// reqLogger.V(1).Info("Reconcile, skip this resync", "requeueAfter", requeueDuration)
			return reconcile.Result{RequeueAfter: requeueDuration}, nil
		}
	}

	// retrieved and build information for the strategy
	strategyParams, err := r.buildStrategyParams(reqLogger, daemonsetInstance, replicaSetInstance)
	if err != nil {
		return reconcile.Result{}, err
	}

	// now apply the strategy depending on the ReplicaSet state
	strategyResult, err := r.applyStrategy(reqLogger, daemonsetInstance, now, strategyParams)
	newStatus := strategyResult.NewStatus
	result := strategyResult.Result

	// for the reste of the actions we will try to execute as many actions as we can so we will store possible errors in a list
	// each action can be executed in parallel.
	var errs []error
	if err != nil {
		errs = append(errs, err)
	}

	var desc string
	status := corev1.ConditionTrue
	if len(strategyResult.UnscheduledNodesDueToResourcesConstraints) > 0 {
		desc = fmt.Sprintf("nodes:%s", strings.Join(strategyResult.UnscheduledNodesDueToResourcesConstraints, ";"))
	} else {
		status = corev1.ConditionFalse
	}
	conditions.UpdateExtendedDaemonSetReplicaSetStatusCondition(newStatus, now, datadoghqv1alpha1.ConditionTypeUnschedule, status, "", desc, false, false)

	// start actions on pods
	requeueAfter := 5 * time.Second
	if daemonsetInstance.Spec.Strategy.ReconcileFrequency != nil {
		requeueAfter = daemonsetInstance.Spec.Strategy.ReconcileFrequency.Duration
	}

	lastPodDeletionCondition := conditions.GetExtendedDaemonSetReplicaSetStatusCondition(newStatus, datadoghqv1alpha1.ConditionTypePodDeletion)
	if lastPodDeletionCondition != nil && now.Sub(lastPodDeletionCondition.LastUpdateTime.Time) < requeueAfter {
		reqLogger.V(1).Info("Delay pods deletion", "deplay", requeueAfter, "since", now.Sub(lastPodDeletionCondition.LastUpdateTime.Time))
		result.RequeueAfter = requeueAfter
	} else {
		errs = append(errs, deletePods(reqLogger, r.client, strategyParams.PodByNodeName, strategyResult.PodsToDelete)...)
		if len(strategyResult.PodsToDelete) > 0 {
			conditions.UpdateExtendedDaemonSetReplicaSetStatusCondition(newStatus, now, datadoghqv1alpha1.ConditionTypePodDeletion, corev1.ConditionTrue, "", "pods deleted", false, true)
		}
	}

	lastPodCreationCondition := conditions.GetExtendedDaemonSetReplicaSetStatusCondition(newStatus, datadoghqv1alpha1.ConditionTypePodCreation)
	if lastPodCreationCondition != nil && now.Sub(lastPodCreationCondition.LastUpdateTime.Time) < daemonsetInstance.Spec.Strategy.ReconcileFrequency.Duration {
		reqLogger.V(1).Info("Delay pods creation", "deplay:", requeueAfter, "since", now.Sub(lastPodDeletionCondition.LastUpdateTime.Time))
		result.RequeueAfter = requeueAfter
	} else {
		errs = append(errs, createPods(reqLogger, r.client, r.scheme, r.options.IsNodeAffinitySupported, replicaSetInstance, strategyResult.PodsToCreate)...)
		if len(strategyResult.PodsToCreate) > 0 {
			conditions.UpdateExtendedDaemonSetReplicaSetStatusCondition(newStatus, now, datadoghqv1alpha1.ConditionTypePodCreation, corev1.ConditionTrue, "", "pods created", false, true)
		}
	}

	err = utilserrors.NewAggregate(errs)
	conditions.UpdateErrorCondition(newStatus, now, err, "")
	conditions.UpdateExtendedDaemonSetReplicaSetStatusCondition(newStatus, now, datadoghqv1alpha1.ConditionTypeLastFullSync, corev1.ConditionTrue, "", "full sync", true, true)

	reqLogger.V(1).Info("Updating ExtendedDaemonSetReplicaSet status")
	err = r.updateReplicaSet(replicaSetInstance, newStatus)

	// Garbage collect the failedPodsBackOff map once per minute,
	// i.e. whenever the seconds [0,59] is less than the reconcile frequency
	if now.Time.Second() < int(daemonsetInstance.Spec.Strategy.ReconcileFrequency.Duration.Seconds()) {
		// Garbage collect records that have aged past the maxDuration (here: 15min)
		r.failedPodsBackOff.GC()
	}

	return result, err
}

func (r *Reconciler) buildStrategyParams(logger logr.Logger, daemonset *datadoghqv1alpha1.ExtendedDaemonSet, replicaset *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet) (*strategy.Parameters, error) {
	rsStatus := retrieveReplicaSetStatus(daemonset, replicaset.Name)

	// Retrieve the Node associated to the replicaset (with node selector)
	nodeList, podList, err := r.getPodAndNodeList(logger, daemonset, replicaset)
	if err != nil {
		return nil, err
	}

	strategyParams := &strategy.Parameters{
		EDSName:          daemonset.Name,
		Strategy:         &daemonset.Spec.Strategy,
		Replicaset:       replicaset,
		ReplicaSetStatus: string(rsStatus),
		Logger:           logger.WithValues("strategy", rsStatus),
		NewStatus:        replicaset.Status.DeepCopy(),
	}
	var nodesFilter []string
	if daemonset.Status.Canary != nil {
		strategyParams.CanaryNodes = daemonset.Status.Canary.Nodes
		if daemonset.Status.ActiveReplicaSet == replicaset.Name {
			nodesFilter = strategyParams.CanaryNodes
		}
	}

	// Associate Pods to Nodes
	strategyParams.NodeByName, strategyParams.PodByNodeName, strategyParams.PodToCleanUp, strategyParams.UnscheduledPods = r.FilterAndMapPodsByNode(logger.WithValues("status", string(rsStatus)), replicaset, nodeList, podList, nodesFilter)

	return strategyParams, nil
}

func (r *Reconciler) applyStrategy(logger logr.Logger, daemonset *datadoghqv1alpha1.ExtendedDaemonSet, now metav1.Time, strategyParams *strategy.Parameters) (*strategy.Result, error) {
	var strategyResult *strategy.Result
	var err error
	logger.V(1).Info("DaemonsetStatus: ", "status", daemonset.Status)
	switch strategy.ReplicaSetStatus(strategyParams.ReplicaSetStatus) {
	case strategy.ReplicaSetStatusActive:
		logger.Info("manage deployment")
		conditions.UpdateExtendedDaemonSetReplicaSetStatusCondition(strategyParams.NewStatus, now, datadoghqv1alpha1.ConditionTypeCanary, corev1.ConditionFalse, "", "", false, false)
		conditions.UpdateExtendedDaemonSetReplicaSetStatusCondition(strategyParams.NewStatus, now, datadoghqv1alpha1.ConditionTypeCanaryPaused, corev1.ConditionFalse, "", "", false, false)
		conditions.UpdateExtendedDaemonSetReplicaSetStatusCondition(strategyParams.NewStatus, now, datadoghqv1alpha1.ConditionTypeCanaryFailed, corev1.ConditionFalse, "", "", false, false)
		strategyResult, err = strategy.ManageDeployment(r.client, daemonset, strategyParams, now)
	case strategy.ReplicaSetStatusCanary:
		conditions.UpdateExtendedDaemonSetReplicaSetStatusCondition(strategyParams.NewStatus, now, datadoghqv1alpha1.ConditionTypeCanary, corev1.ConditionTrue, "", "", false, false)
		conditions.UpdateExtendedDaemonSetReplicaSetStatusCondition(strategyParams.NewStatus, now, datadoghqv1alpha1.ConditionTypeActive, corev1.ConditionFalse, "", "", false, false)
		logger.Info("manage canary deployment")
		strategyResult, err = strategy.ManageCanaryDeployment(r.client, daemonset, strategyParams)
	case strategy.ReplicaSetStatusUnknown:
		conditions.UpdateExtendedDaemonSetReplicaSetStatusCondition(strategyParams.NewStatus, now, datadoghqv1alpha1.ConditionTypeCanary, corev1.ConditionFalse, "", "", false, false)
		conditions.UpdateExtendedDaemonSetReplicaSetStatusCondition(strategyParams.NewStatus, now, datadoghqv1alpha1.ConditionTypeActive, corev1.ConditionFalse, "", "", false, false)
		logger.Info("ignore this replicaset, since it's not the replicas active or canary")
		strategyResult, err = strategy.ManageUnknown(r.client, strategyParams)
	}

	return strategyResult, err
}

func (r *Reconciler) getPodAndNodeList(logger logr.Logger, daemonset *datadoghqv1alpha1.ExtendedDaemonSet, replicaset *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet) (*strategy.NodeList, *corev1.PodList, error) {
	var nodeList *strategy.NodeList
	var podList *corev1.PodList
	var err error
	nodeList, err = r.getNodeList(daemonset, replicaset)
	if err != nil {
		logger.Error(err, "unable to list associated pods")

		return nodeList, podList, err
	}

	// Retrieve the Node associated to the replicaset
	podList, err = r.getPodList(daemonset)
	if err != nil {
		logger.Error(err, "unable to list associated pods")

		return nodeList, podList, err
	}

	var oldPodList *corev1.PodList
	oldPodList, err = r.getOldDaemonsetPodList(daemonset)
	if err != nil {
		logger.Error(err, "unable to list associated pods")

		return nodeList, podList, err
	}
	podList.Items = append(podList.Items, oldPodList.Items...)

	return nodeList, podList, nil
}

func (r *Reconciler) retrievedReplicaSet(request reconcile.Request) (*datadoghqv1alpha1.ExtendedDaemonSetReplicaSet, bool, error) {
	replicaSetInstance := &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}
	err := r.client.Get(context.TODO(), request.NamespacedName, replicaSetInstance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return nil, true, nil
		}
		// Error reading the object - requeue the request.
		return nil, true, err
	}

	return replicaSetInstance, false, nil
}

func (r *Reconciler) updateReplicaSet(replicaset *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet, newStatus *datadoghqv1alpha1.ExtendedDaemonSetReplicaSetStatus) error {
	// compare
	if !apiequality.Semantic.DeepEqual(&replicaset.Status, newStatus) {
		newRS := replicaset.DeepCopy()
		newRS.Status = *newStatus

		return r.client.Status().Update(context.TODO(), newRS)
	}

	return nil
}

func (r *Reconciler) getDaemonsetOwner(replicaset *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet) (*datadoghqv1alpha1.ExtendedDaemonSet, error) {
	ownerName, err := retrieveOwnerReference(replicaset)
	if err != nil {
		return nil, err
	}
	daemonsetInstance := &datadoghqv1alpha1.ExtendedDaemonSet{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: ownerName, Namespace: replicaset.Namespace}, daemonsetInstance)
	if err != nil {
		return nil, err
	}

	return daemonsetInstance, nil
}

func (r *Reconciler) getExtendedDaemonsetSettings(eds *datadoghqv1alpha1.ExtendedDaemonSet) ([]*datadoghqv1alpha1.ExtendedDaemonsetSetting, error) {
	edsNodeList := &datadoghqv1alpha1.ExtendedDaemonsetSettingList{}
	err := r.client.List(context.TODO(), edsNodeList, &client.ListOptions{Namespace: eds.Namespace})
	if err != nil {
		return nil, err
	}
	var outputList []*datadoghqv1alpha1.ExtendedDaemonsetSetting
	for index, edsNode := range edsNodeList.Items {
		if edsNode.Spec.Reference == nil {
			continue
		}
		if edsNode.Spec.Reference.Name == eds.Name {
			outputList = append(outputList, &edsNodeList.Items[index])
		}
	}

	return outputList, nil
}

func (r *Reconciler) getPodList(ds *datadoghqv1alpha1.ExtendedDaemonSet) (*corev1.PodList, error) {
	podList := &corev1.PodList{}
	podSelector := labels.Set{datadoghqv1alpha1.ExtendedDaemonSetNameLabelKey: ds.Name}
	podListOptions := []client.ListOption{
		client.MatchingLabelsSelector{
			Selector: podSelector.AsSelectorPreValidated(),
		},
	}
	if err := r.client.List(context.TODO(), podList, podListOptions...); err != nil {
		return nil, err
	}

	return podList, nil
}

func (r *Reconciler) getNodeList(eds *datadoghqv1alpha1.ExtendedDaemonSet, replicaset *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet) (*strategy.NodeList, error) {
	nodeItemList := &strategy.NodeList{}

	listOptions := []client.ListOption{}
	if replicaset.Spec.Selector != nil {
		selector, err := utils.ConvertLabelSelector(r.log, replicaset.Spec.Selector)
		if err != nil {
			return nil, err
		}

		listOptions = []client.ListOption{
			client.MatchingLabelsSelector{
				Selector: selector,
			},
		}
	}
	nodeList := &corev1.NodeList{}
	if err := r.client.List(context.TODO(), nodeList, listOptions...); err != nil {
		return nil, err
	}

	extendedDaemonsetSettings, err := r.getExtendedDaemonsetSettings(eds)
	if err != nil {
		return nil, err
	}

	for index, node := range nodeList.Items {
		var edsNodeSelected *datadoghqv1alpha1.ExtendedDaemonsetSetting
		for _, edsNode := range extendedDaemonsetSettings {
			if edsNode.Status.Status != datadoghqv1alpha1.ExtendedDaemonsetSettingStatusValid {
				continue
			}
			selector, err2 := metav1.LabelSelectorAsSelector(&edsNode.Spec.NodeSelector)
			if err2 != nil {
				return nil, err2
			}
			if selector.Matches(labels.Set(node.Labels)) {
				edsNodeSelected = edsNode

				break
			}
		}
		nodeItemList.Items = append(nodeItemList.Items, strategy.NewNodeItem(&nodeList.Items[index], edsNodeSelected))
	}

	return nodeItemList, nil
}

func (r *Reconciler) getOldDaemonsetPodList(ds *datadoghqv1alpha1.ExtendedDaemonSet) (*corev1.PodList, error) {
	podList := &corev1.PodList{}

	oldDsName, ok := ds.GetAnnotations()[datadoghqv1alpha1.ExtendedDaemonSetOldDaemonsetAnnotationKey]
	if !ok {
		return podList, nil
	}

	oldDaemonset := &appsv1.DaemonSet{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: ds.Namespace, Name: oldDsName}, oldDaemonset)
	if err != nil {
		if errors.IsNotFound(err) {
			return podList, nil
		}
		// Error reading the object - requeue the request.
		return nil, err
	}
	podListOptions := []client.ListOption{}
	if oldDaemonset.Spec.Selector != nil {
		selector, err2 := utils.ConvertLabelSelector(r.log, oldDaemonset.Spec.Selector)
		if err2 != nil {
			return nil, err2
		}

		podListOptions = []client.ListOption{
			client.MatchingLabelsSelector{
				Selector: selector,
			},
		}
	}

	if err = r.client.List(context.TODO(), podList, podListOptions...); err != nil {
		return nil, err
	}

	// filter by ownerreferences,
	// This is to prevent issue with label selector that match between DS and EDS
	var filterPods []corev1.Pod
	for id, pod := range podList.Items {
		selected := false
		for _, ref := range pod.OwnerReferences {
			if ref.Kind == "DaemonSet" && ref.Name == oldDsName {
				selected = true

				break
			}
		}
		if selected {
			filterPods = append(filterPods, podList.Items[id])
		}
	}
	podList.Items = filterPods

	return podList, nil
}

func retrieveOwnerReference(obj *datadoghqv1alpha1.ExtendedDaemonSetReplicaSet) (string, error) {
	for _, ref := range obj.OwnerReferences {
		if ref.Kind == "ExtendedDaemonSet" {
			return ref.Name, nil
		}
	}

	return "", fmt.Errorf("unable to retrieve the owner reference name")
}

// This method has the effect of deciding if the ERS should be ignored or something should be updated. Note that this
// never returns strategy.ReplicaSetStatusCanaryFailed because that case does not need to be managed (if it is Failed, it has
// already been processed).
func retrieveReplicaSetStatus(daemonset *datadoghqv1alpha1.ExtendedDaemonSet, replicassetName string) strategy.ReplicaSetStatus {
	switch daemonset.Status.ActiveReplicaSet {
	case "":
		return strategy.ReplicaSetStatusUnknown
	case replicassetName:
		return strategy.ReplicaSetStatusActive
	default:
		if daemonset.Status.Canary != nil && daemonset.Status.Canary.ReplicaSet == replicassetName {
			return strategy.ReplicaSetStatusCanary
		}

		return strategy.ReplicaSetStatusUnknown
	}
}
