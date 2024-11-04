// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoghq

import (
	"context"
	"time"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/metrics"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// Reconciler reconciles a DatadogAgentProfile object
type Reconciler struct {
	client  client.Client
	scheme  *runtime.Scheme
	log     logr.Logger
	options DAPReconcilerOptions
}
type DAPReconcilerOptions struct {
	DatadogAgentProfileEnabled bool
	DapControllerFlip          bool
}

//+kubebuilder:rbac:groups=datadoghq.com,resources=datadogagentprofiles,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=datadoghq.com,resources=datadogagentprofiles/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=datadoghq.com,resources=datadogagentprofiles/finalizers,verbs=update

// NewReconciler returns a new Reconciler object
func NewReconciler(options DAPReconcilerOptions, client client.Client, scheme *runtime.Scheme, log logr.Logger) (*Reconciler, error) {
	return &Reconciler{
		client:  client,
		scheme:  scheme,
		log:     log,
		options: options,
	}, nil
}

func (r *Reconciler) Reconcile(ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	// _ = log.FromContext(ctx)

	// // TODO(user): your logic here

	// return ctrl.Result{}, nil
	return r.internalReconcile(ctx, req)
}

func (r *Reconciler) internalReconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	logger := r.log.WithValues("datadogagentprofile", req.NamespacedName)
	logger.Info("Reconciling DatadogAgentProfile")
	var result reconcile.Result
	logger.Info("Reconciling DatadogAgentProfile", "DapControllerFlip", r.options.DapControllerFlip)

	if r.options.DapControllerFlip {
		logger.Info("DapControllerFlip enabled, reconciling in DAP")
		now := metav1.NewTime(time.Now())

		ddaList := datadoghqv2alpha1.DatadogAgentList{}
		err := r.client.List(ctx, &ddaList)
		if err != nil {
			return reconcile.Result{}, err
		}

		instance := &ddaList.Items[0]
		newStatus := instance.Status.DeepCopy()

		// Get a node list for profiles and introspection
		nodeList, e := r.getNodeList(ctx)
		if e != nil {
			return r.updateStatusIfNeededV2(logger, instance, newStatus, result, e, now)
		}

		metrics.DAPEnabled.Set(metrics.TrueValue)
		var profilesByNode map[string]types.NamespacedName

		// on every reconcile
		// group nodes which match profile rules profile -> {nodes matching profile rules}
		//      some additional filtering applied here
		// then apply labels

		_ /*profiles*/, profilesByNode, e = r.profilesToApply(ctx, logger, nodeList, now, instance)
		if err != nil {
			return r.updateStatusIfNeededV2(logger, instance, newStatus, result, e, now)
		}

		if err = r.handleProfiles(ctx, profilesByNode, instance.Namespace); err != nil {
			return r.updateStatusIfNeededV2(logger, instance, newStatus, result, err, now)
		}
	}
	return result, nil
}

func (r *Reconciler) updateStatusIfNeededV2(logger logr.Logger, agentdeployment *datadoghqv2alpha1.DatadogAgent, newStatus *datadoghqv2alpha1.DatadogAgentStatus, result reconcile.Result, currentError error, now metav1.Time) (reconcile.Result, error) {
	if currentError == nil {
		datadoghqv2alpha1.UpdateDatadogAgentStatusConditions(newStatus, now, datadoghqv2alpha1.DatadogAgentReconcileErrorConditionType, metav1.ConditionFalse, "DatadogAgent_reconcile_ok", "DatadogAgent reconcile ok", false)
	} else {
		datadoghqv2alpha1.UpdateDatadogAgentStatusConditions(newStatus, now, datadoghqv2alpha1.DatadogAgentReconcileErrorConditionType, metav1.ConditionTrue, "DatadogAgent_reconcile_error", "DatadogAgent reconcile error", false)
	}

	// r.setMetricsForwarderStatusV2(logger, agentdeployment, newStatus)

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

func (r *Reconciler) profilesToApply(ctx context.Context, logger logr.Logger, nodeList []corev1.Node, now metav1.Time, dda *datadoghqv2alpha1.DatadogAgent) ([]datadoghqv1alpha1.DatadogAgentProfile, map[string]types.NamespacedName, error) {
	profilesList := datadoghqv1alpha1.DatadogAgentProfileList{}
	err := r.client.List(ctx, &profilesList)
	if err != nil {
		return nil, nil, err
	}

	var profileListToApply []datadoghqv1alpha1.DatadogAgentProfile
	profileAppliedByNode := make(map[string]types.NamespacedName, len(nodeList))

	sortedProfiles := agentprofile.SortProfiles(profilesList.Items)
	for _, profile := range sortedProfiles {
		maxUnavailable := agentprofile.GetMaxUnavailable(logger, dda, &profile, len(nodeList), &agent.ExtendedDaemonsetOptions{})
		profileAppliedByNode, err = agentprofile.ApplyProfile(logger, &profile, nodeList, profileAppliedByNode, now, maxUnavailable)
		r.updateDAPStatus(logger, &profile)
		if err != nil {
			// profile is invalid or conflicts
			logger.Error(err, "profile cannot be applied", "datadogagentprofile", profile.Name, "datadogagentprofile_namespace", profile.Namespace)
			continue
		}
		profileListToApply = append(profileListToApply, profile)
	}

	// add default profile
	profileListToApply = agentprofile.ApplyDefaultProfile(profileListToApply, profileAppliedByNode, nodeList)

	return profileListToApply, profileAppliedByNode, nil
}

func (r *Reconciler) getNodeList(ctx context.Context) ([]corev1.Node, error) {
	nodeList := corev1.NodeList{}
	err := r.client.List(ctx, &nodeList)
	if err != nil {
		return nodeList.Items, err
	}

	return nodeList.Items, nil
}

func (r *Reconciler) updateDAPStatus(logger logr.Logger, profile *datadoghqv1alpha1.DatadogAgentProfile) {
	// update dap status for non-default profiles only
	if !agentprofile.IsDefaultProfile(profile.Namespace, profile.Name) {
		if err := r.client.Status().Update(context.TODO(), profile); err != nil {
			if apierrors.IsConflict(err) {
				logger.V(1).Info("unable to update DatadogAgentProfile status due to update conflict")
			}
			logger.Error(err, "unable to update DatadogAgentProfile status")
		}
	}
}

// TODO: should slowStrategy be implemented here instead of labelSelection
func (r *Reconciler) handleProfiles(ctx context.Context, profilesByNode map[string]types.NamespacedName, ddaNamespace string) error {
	if err := r.labelNodesWithProfiles(ctx, profilesByNode); err != nil {
		return err
	}

	if err := r.cleanupPodsForProfilesThatNoLongerApply(ctx, profilesByNode, ddaNamespace); err != nil {
		return err
	}

	return nil
}

// labelNodesWithProfiles sets the "agent.datadoghq.com/datadogagentprofile" label only in
// the nodes where a profile is applied
func (r *Reconciler) labelNodesWithProfiles(ctx context.Context, profilesByNode map[string]types.NamespacedName) error {
	r.log.Info("labeling nodes for profiles", "mapping", profilesByNode)
	for nodeName, profileNamespacedName := range profilesByNode {
		isDefaultProfile := agentprofile.IsDefaultProfile(profileNamespacedName.Namespace, profileNamespacedName.Name)

		node := &corev1.Node{}
		if err := r.client.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
			return err
		}

		newLabels := map[string]string{}
		labelsToRemove := map[string]bool{}
		labelsToAddOrChange := map[string]string{}

		// If the profile is the default one and the label exists in the node,
		// it should be removed.
		if isDefaultProfile {
			if _, profileLabelExists := node.Labels[agentprofile.ProfileLabelKey]; profileLabelExists {
				labelsToRemove[agentprofile.ProfileLabelKey] = true
			}
		} else {
			// If the profile is not the default one and the label does not exist in
			// the node, it should be added. If the label value is outdated, it
			// should be updated.
			if profileLabelValue := node.Labels[agentprofile.ProfileLabelKey]; profileLabelValue != profileNamespacedName.Name {
				labelsToAddOrChange[agentprofile.ProfileLabelKey] = profileNamespacedName.Name
			}
		}

		// Remove old profile label key if it is present
		if _, oldProfileLabelExists := node.Labels[agentprofile.OldProfileLabelKey]; oldProfileLabelExists {
			labelsToRemove[agentprofile.OldProfileLabelKey] = true
		}

		if len(labelsToRemove) > 0 || len(labelsToAddOrChange) > 0 {
			for k, v := range node.Labels {
				if _, ok := labelsToRemove[k]; ok {
					continue
				}
				newLabels[k] = v
			}

			for k, v := range labelsToAddOrChange {
				newLabels[k] = v
			}
		}

		if len(newLabels) == 0 {
			continue
		}

		modifiedNode := node.DeepCopy()
		modifiedNode.Labels = newLabels

		err := r.client.Patch(ctx, modifiedNode, client.MergeFrom(node))
		if err != nil && !errors.IsNotFound(err) {
			return err
		}
	}

	return nil
}

// cleanupPodsForProfilesThatNoLongerApply deletes the agent pods that should
// not be running according to the profiles that need to be applied. This is
// needed because in the affinities we use
// "RequiredDuringSchedulingIgnoredDuringExecution" which means that the pods
// might not always be evicted when there's a change in the profiles to apply.
// Notice that "RequiredDuringSchedulingRequiredDuringExecution" is not
// available in Kubernetes yet.
func (r *Reconciler) cleanupPodsForProfilesThatNoLongerApply(ctx context.Context, profilesByNode map[string]types.NamespacedName, ddaNamespace string) error {
	agentPods := &corev1.PodList{}
	err := r.client.List(
		ctx,
		agentPods,
		client.MatchingLabels(map[string]string{
			apicommon.AgentDeploymentComponentLabelKey: datadoghqv2alpha1.DefaultAgentResourceSuffix,
		}),
		client.InNamespace(ddaNamespace),
	)
	if err != nil {
		return err
	}

	for _, agentPod := range agentPods.Items {
		profileNamespacedName, found := profilesByNode[agentPod.Spec.NodeName]
		if !found {
			continue
		}

		isDefaultProfile := agentprofile.IsDefaultProfile(profileNamespacedName.Namespace, profileNamespacedName.Name)
		expectedProfileLabelValue := profileNamespacedName.Name

		profileLabelValue, profileLabelExists := agentPod.Labels[agentprofile.ProfileLabelKey]

		deletePod := (isDefaultProfile && profileLabelExists) ||
			(!isDefaultProfile && !profileLabelExists) ||
			(!isDefaultProfile && profileLabelValue != expectedProfileLabelValue)

		if deletePod {
			toDelete := corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: agentPod.Namespace,
					Name:      agentPod.Name,
				},
			}
			if err = r.client.Delete(ctx, &toDelete); err != nil {
				return err
			}
		}
	}

	return nil
}
