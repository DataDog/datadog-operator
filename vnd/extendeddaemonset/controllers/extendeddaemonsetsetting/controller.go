// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package extendeddaemonsetsetting

import (
	"context"
	"fmt"
	"sort"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
)

// Reconciler is the internal reconciler for ExtendedDaemonSetReplicaSet.
type Reconciler struct {
	options  ReconcilerOptions
	client   client.Client
	scheme   *runtime.Scheme
	log      logr.Logger
	recorder record.EventRecorder
}

// ReconcilerOptions provides options read from command line.
type ReconcilerOptions struct{}

// NewReconciler returns a reconciler for DatadogAgent.
func NewReconciler(options ReconcilerOptions, client client.Client, scheme *runtime.Scheme, log logr.Logger, recorder record.EventRecorder) (*Reconciler, error) {
	return &Reconciler{
		options:  options,
		client:   client,
		scheme:   scheme,
		log:      log,
		recorder: recorder,
	}, nil
}

// Reconcile reads that state of the cluster for a ExtendedDaemonsetSetting object and makes changes based on the state read
// and what is in the ExtendedDaemonsetSetting.Spec.
func (r *Reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := r.log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	reqLogger.Info("Reconciling ExtendedDaemonsetSetting")

	// Fetch the ExtendedDaemonsetSetting instance
	instance := &datadoghqv1alpha1.ExtendedDaemonsetSetting{}
	err := r.client.Get(ctx, request.NamespacedName, instance)
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

	newStatus := instance.Status.DeepCopy()
	newStatus.Error = ""
	if instance.Spec.Reference == nil || instance.Spec.Reference.Name == "" {
		newStatus.Error = "missing reference"

		newStatus.Error = "missing reference in spec"
		newStatus.Status = datadoghqv1alpha1.ExtendedDaemonsetSettingStatusError

		return r.updateExtendedDaemonsetSetting(ctx, instance, newStatus)
	}

	edsNodesList := &datadoghqv1alpha1.ExtendedDaemonsetSettingList{}
	if err = r.client.List(ctx, edsNodesList, &client.ListOptions{Namespace: instance.Namespace}); err != nil {
		return r.updateExtendedDaemonsetSetting(ctx, instance, newStatus)
	}

	nodesList := &corev1.NodeList{}
	if err = r.client.List(ctx, nodesList); err != nil {
		newStatus.Status = datadoghqv1alpha1.ExtendedDaemonsetSettingStatusError
		newStatus.Error = fmt.Sprintf("unable to get nodes, err:%v", err)
	}

	var otherEdsNode string
	otherEdsNode, err = searchPossibleConflict(instance, nodesList, edsNodesList)
	if err != nil {
		newStatus.Status = datadoghqv1alpha1.ExtendedDaemonsetSettingStatusError
		newStatus.Error = fmt.Sprintf("conflict with another ExtendedDaemonsetSetting: %s", otherEdsNode)
	}

	if newStatus.Error == "" {
		newStatus.Status = datadoghqv1alpha1.ExtendedDaemonsetSettingStatusValid
	}

	return r.updateExtendedDaemonsetSetting(ctx, instance, newStatus)
}

func (r *Reconciler) updateExtendedDaemonsetSetting(ctx context.Context, edsNode *datadoghqv1alpha1.ExtendedDaemonsetSetting, newStatus *datadoghqv1alpha1.ExtendedDaemonsetSettingStatus) (reconcile.Result, error) {
	if apiequality.Semantic.DeepEqual(&edsNode.Status, newStatus) {
		return reconcile.Result{}, nil
	}
	newEdsNode := edsNode.DeepCopy()
	newEdsNode.Status = *newStatus
	err := r.client.Status().Update(ctx, newEdsNode)

	return reconcile.Result{}, err
}

func searchPossibleConflict(instance *datadoghqv1alpha1.ExtendedDaemonsetSetting, nodeList *corev1.NodeList, edsNodeList *datadoghqv1alpha1.ExtendedDaemonsetSettingList) (string, error) {
	var edsNodes edsNodeByCreationTimestampAndPhase
	for id := range edsNodeList.Items {
		edsNodes = append(edsNodes, &edsNodeList.Items[id])
	}
	sort.Sort(edsNodes)

	nodesAlreadySelected := map[string]string{}
	for _, node := range nodeList.Items {
		for _, edsNode := range edsNodes {
			selector, err2 := metav1.LabelSelectorAsSelector(&edsNode.Spec.NodeSelector)
			if err2 != nil {
				return "", err2
			}
			if selector.Matches(labels.Set(node.Labels)) {
				if edsNode.Name == instance.Name {
					if previousEdsNode, found := nodesAlreadySelected[node.Name]; found {
						return previousEdsNode, fmt.Errorf("extendedDaemonsetSetting already assigned to the node %s", node.Name)
					}
				}
				nodesAlreadySelected[node.Name] = edsNode.Name
			}
		}
		nodesAlreadySelected[node.Name] = ""
	}

	return "", nil
}
