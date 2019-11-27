// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagentdeployment

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	edsdatadoghqv1alpha1 "github.com/datadog/extendeddaemonset/pkg/apis/datadoghq/v1alpha1"
)

func (r *ReconcileDatadogAgentDeployment) reconcileAgent(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	result, err := r.manageAgentDependencies(logger, dad, newStatus)
	if shouldReturn(result, err) {
		return result, err
	}

	nameNamespace := types.NamespacedName{
		Name:      dad.ObjectMeta.Name,
		Namespace: dad.ObjectMeta.Namespace,
	}
	// check if EDS or DS already exist
	eds := &edsdatadoghqv1alpha1.ExtendedDaemonSet{}
	if supportExtendedDaemonset {
		if err2 := r.client.Get(context.TODO(), nameNamespace, eds); err2 != nil {
			if !errors.IsNotFound(err2) {
				return result, err2
			}
			eds = nil
		}
	} else {
		eds = nil
	}

	ds := &appsv1.DaemonSet{}
	if err2 := r.client.Get(context.TODO(), nameNamespace, ds); err2 != nil {
		if !errors.IsNotFound(err2) {
			return result, err2
		}
		ds = nil
	}

	if dad.Spec.Agent == nil {
		if ds != nil {
			if err = r.deleteDaemonSet(logger, dad, ds); err != nil {
				return result, err
			}
		}
		if eds != nil {
			if err = r.deleteExtendedDaemonSet(logger, dad, eds); err != nil {
				return result, err
			}
		}
		newStatus.Agent = nil
		return result, err
	}

	if supportExtendedDaemonset && datadoghqv1alpha1.BoolValue(dad.Spec.Agent.UseExtendedDaemonset) {
		if ds != nil {
			// TODO manage properly the migration from DS to EDS
			err = r.deleteDaemonSet(logger, dad, ds)
			if err != nil {
				return result, err
			}
			result.RequeueAfter = 5 * time.Second
			return result, nil
		}
		if eds == nil {
			return r.createNewExtendedDaemonSet(logger, dad, newStatus)
		}

		return r.updateExtendedDaemonSet(logger, dad, eds, newStatus)
	}

	// Case when Daemonset is requested
	if eds != nil && supportExtendedDaemonset {
		// if EDS exist delete before creating or updating the Daemonset
		err = r.deleteExtendedDaemonSet(logger, dad, eds)
		if err != nil {
			return result, err
		}
		result.RequeueAfter = 5 * time.Second
		return result, nil
	}
	if ds == nil {
		return r.createNewDaemonSet(logger, dad, newStatus)
	}

	return r.updateDaemonSet(logger, dad, ds, newStatus)

}

func (r *ReconcileDatadogAgentDeployment) deleteDaemonSet(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgentDeployment, ds *appsv1.DaemonSet) error {
	err := r.client.Delete(context.TODO(), ds)
	if err != nil {
		return err
	}
	logger.Info("Delete DaemonSet", "daemonSet.Namespace", ds.Namespace, "daemonSet.Name", ds.Name)
	r.recorder.Event(agentdeployment, corev1.EventTypeNormal, "Delete DaemonSet", fmt.Sprintf("%s/%s", ds.Namespace, ds.Name))
	return err
}

func (r *ReconcileDatadogAgentDeployment) deleteExtendedDaemonSet(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgentDeployment, eds *edsdatadoghqv1alpha1.ExtendedDaemonSet) error {
	err := r.client.Delete(context.TODO(), eds)
	if err != nil {
		return err
	}
	logger.Info("Delete DaemonSet", "extendedDaemonSet.Namespace", eds.Namespace, "extendedDaemonSet.Name", eds.Name)
	r.recorder.Event(agentdeployment, corev1.EventTypeNormal, "Delete ExtendedDaemonSet", fmt.Sprintf("%s/%s", eds.Namespace, eds.Name))
	return err
}

func (r *ReconcileDatadogAgentDeployment) createNewExtendedDaemonSet(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgentDeployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	var err error
	// ExtendedDaemonSet up to date didn't exist yet, create a new one
	var newEDS *edsdatadoghqv1alpha1.ExtendedDaemonSet
	var hash string
	if newEDS, hash, err = newExtendedDaemonSetFromInstance(logger, agentdeployment); err != nil {
		return reconcile.Result{}, err
	}

	// Set ExtendedDaemonSet instance as the owner and controller
	if err = controllerutil.SetControllerReference(agentdeployment, newEDS, r.scheme); err != nil {
		return reconcile.Result{}, err
	}

	logger.Info("Creating a new ExtendedDaemonSet", "extendedDaemonSet.Namespace", newEDS.Namespace, "extendedDaemonSet.Name", newEDS.Name, "agentdeployment.Status.Agent.CurrentHash", hash)

	err = r.client.Create(context.TODO(), newEDS)
	if err != nil {
		return reconcile.Result{}, err
	}
	r.recorder.Event(agentdeployment, corev1.EventTypeNormal, "Create ExtendedDaemonSet", fmt.Sprintf("%s/%s", newEDS.Namespace, newEDS.Name))
	now := metav1.NewTime(time.Now())
	updateStatusWithExtendedDaemonset(newEDS, newStatus, &now)

	return reconcile.Result{}, nil
}

func (r *ReconcileDatadogAgentDeployment) createNewDaemonSet(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgentDeployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	var err error
	// DaemonSet up to date didn't exist yet, create a new one
	var newDS *appsv1.DaemonSet
	var hash string
	if newDS, hash, err = newDaemonSetFromInstance(logger, agentdeployment); err != nil {
		return reconcile.Result{}, err
	}

	// Set DaemonSet instance as the owner and controller
	if err = controllerutil.SetControllerReference(agentdeployment, newDS, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.Info("Creating a new DaemonSet", "daemonSet.Namespace", newDS.Namespace, "daemonSet.Name", newDS.Name, "agentdeployment.Status.Agent.CurrentHash", hash)
	err = r.client.Create(context.TODO(), newDS)
	if err != nil {
		return reconcile.Result{}, err
	}
	r.recorder.Event(agentdeployment, corev1.EventTypeNormal, "Create DaemonSet", fmt.Sprintf("%s/%s", newDS.Namespace, newDS.Name))
	now := metav1.NewTime(time.Now())
	updateStatusWithDaemonset(newDS, newStatus, &now)

	return reconcile.Result{}, nil
}

func (r *ReconcileDatadogAgentDeployment) updateExtendedDaemonSet(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgentDeployment, eds *edsdatadoghqv1alpha1.ExtendedDaemonSet, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	newEDS, hash, err := newExtendedDaemonSetFromInstance(logger, agentdeployment)
	if err != nil {
		return reconcile.Result{}, err
	}
	var needUpdate bool
	if !comparison.CompareSpecMD5Hash(hash, eds.GetAnnotations()) {
		needUpdate = true
	}

	updateStatusWithExtendedDaemonset(eds, newStatus, nil)
	if !needUpdate {
		// no update needed so return now
		return reconcile.Result{}, nil
	}

	// Set ExtendedDaemonSet instance as the owner and controller
	if err = controllerutil.SetControllerReference(agentdeployment, eds, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.Info("Updating an existing ExtendedDaemonSet", "extendedDaemonSet.Namespace", newEDS.Namespace, "extendedDaemonSet.Name", newEDS.Name)

	// Copy possibly changed fields
	updatedEds := eds.DeepCopy()
	updatedEds.Spec = *newEDS.Spec.DeepCopy()
	for k, v := range newEDS.Annotations {
		updatedEds.Annotations[k] = v
	}
	for k, v := range newEDS.Labels {
		updatedEds.Labels[k] = v
	}
	now := metav1.NewTime(time.Now())
	err = r.client.Update(context.TODO(), updatedEds)
	if err != nil {
		return reconcile.Result{}, err
	}
	r.recorder.Event(agentdeployment, corev1.EventTypeNormal, "Update ExtendedDaemonSet", fmt.Sprintf("%s/%s", updatedEds.Namespace, updatedEds.Name))
	updateStatusWithExtendedDaemonset(updatedEds, newStatus, &now)
	return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
}

func updateStatusWithExtendedDaemonset(eds *edsdatadoghqv1alpha1.ExtendedDaemonSet, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus, updateTime *metav1.Time) {
	if newStatus.Agent == nil {
		newStatus.Agent = &datadoghqv1alpha1.DatadogAgentDeploymentAgentStatus{}
	}
	if updateTime != nil {
		newStatus.Agent.LastUpdate = updateTime
	}
	newStatus.Agent.CurrentHash = getHashAnnotation(eds.Annotations)
	newStatus.Agent.Desired = eds.Status.Desired
	newStatus.Agent.Current = eds.Status.Current
	newStatus.Agent.Ready = eds.Status.Ready
	newStatus.Agent.Available = eds.Status.Available
	newStatus.Agent.UpToDate = eds.Status.UpToDate
	newStatus.Agent.State = datadoghqv1alpha1.DatadogAgentDeploymentAgentState(eds.Status.State)
}

func getHashAnnotation(annotations map[string]string) string {
	return annotations[datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey]
}

func (r *ReconcileDatadogAgentDeployment) updateDaemonSet(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgentDeployment, ds *appsv1.DaemonSet, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	newDS, hash, err := newDaemonSetFromInstance(logger, agentdeployment)
	if err != nil {
		return reconcile.Result{}, err
	}

	var needUpdate bool
	if !comparison.CompareSpecMD5Hash(hash, ds.GetAnnotations()) {
		needUpdate = true
	}

	updateStatusWithDaemonset(ds, newStatus, nil)
	if !needUpdate {
		// no update needed so return now
		return reconcile.Result{}, nil
	}

	// Set DaemonSet instance as the owner and controller
	if err = controllerutil.SetControllerReference(agentdeployment, ds, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	logger.Info("Updating an existing DaemonSet", "daemonSet.Namespace", newDS.Namespace, "daemonSet.Name", newDS.Name)

	// Copy possibly changed fields
	updatedDS := ds.DeepCopy()
	updatedDS.Spec = *newDS.Spec.DeepCopy()
	for k, v := range newDS.Annotations {
		updatedDS.Annotations[k] = v
	}
	for k, v := range newDS.Labels {
		updatedDS.Labels[k] = v
	}
	now := metav1.NewTime(time.Now())
	err = r.client.Update(context.TODO(), updatedDS)
	if err != nil {
		return reconcile.Result{}, err
	}
	r.recorder.Event(agentdeployment, corev1.EventTypeNormal, "Update DaemonSet", fmt.Sprintf("%s/%s", updatedDS.Namespace, updatedDS.Name))
	updateStatusWithDaemonset(updatedDS, newStatus, &now)
	return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *ReconcileDatadogAgentDeployment) manageAgentDependencies(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	result, err := r.manageAgentRBACs(logger, dad)
	if shouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageSystemProbeDependencies(logger, dad)
	if shouldReturn(result, err) {
		return result, err
	}

	return reconcile.Result{}, nil
}

func updateStatusWithDaemonset(ds *appsv1.DaemonSet, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus, updateTime *metav1.Time) {
	if newStatus.Agent == nil {
		newStatus.Agent = &datadoghqv1alpha1.DatadogAgentDeploymentAgentStatus{}
	}
	if updateTime != nil {
		newStatus.Agent.LastUpdate = updateTime
	}
	newStatus.Agent.CurrentHash = getHashAnnotation(ds.Annotations)
	newStatus.Agent.Desired = ds.Status.DesiredNumberScheduled
	newStatus.Agent.Current = ds.Status.CurrentNumberScheduled
	newStatus.Agent.Ready = ds.Status.NumberReady
	newStatus.Agent.Available = ds.Status.NumberAvailable
	newStatus.Agent.UpToDate = ds.Status.UpdatedNumberScheduled
	newStatus.Agent.State = datadoghqv1alpha1.DatadogAgentDeploymentAgentStateRunning
}

// newExtendedDaemonSetFromInstance creates an ExtendedDaemonSet from a given DatadogAgentDeployment
func newExtendedDaemonSetFromInstance(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgentDeployment) (*edsdatadoghqv1alpha1.ExtendedDaemonSet, string, error) {
	template, err := newAgentPodTemplate(logger, agentdeployment)
	if err != nil {
		return nil, "", err
	}
	eds := &edsdatadoghqv1alpha1.ExtendedDaemonSet{
		ObjectMeta: newDaemonsetObjectMetaData(agentdeployment),
		Spec: edsdatadoghqv1alpha1.ExtendedDaemonSetSpec{
			Template: *template,
			Strategy: edsdatadoghqv1alpha1.ExtendedDaemonSetSpecStrategy{
				Canary:             agentdeployment.Spec.Agent.DeploymentStrategy.Canary.DeepCopy(),
				ReconcileFrequency: agentdeployment.Spec.Agent.DeploymentStrategy.ReconcileFrequency.DeepCopy(),
				RollingUpdate: edsdatadoghqv1alpha1.ExtendedDaemonSetSpecStrategyRollingUpdate{
					MaxUnavailable:            agentdeployment.Spec.Agent.DeploymentStrategy.RollingUpdate.MaxUnavailable,
					MaxPodSchedulerFailure:    agentdeployment.Spec.Agent.DeploymentStrategy.RollingUpdate.MaxPodSchedulerFailure,
					MaxParallelPodCreation:    agentdeployment.Spec.Agent.DeploymentStrategy.RollingUpdate.MaxParallelPodCreation,
					SlowStartIntervalDuration: agentdeployment.Spec.Agent.DeploymentStrategy.RollingUpdate.SlowStartIntervalDuration,
					SlowStartAdditiveIncrease: agentdeployment.Spec.Agent.DeploymentStrategy.RollingUpdate.SlowStartAdditiveIncrease,
				},
			},
		},
	}
	hash, err := comparison.SetMD5AgentSpecAnnotation(&eds.ObjectMeta, agentdeployment)
	if err != nil {
		return nil, "", err
	}
	return eds, hash, nil
}

// newDaemonSetFromInstance creates a DaemonSet from a given DatadogAgentDeployment
func newDaemonSetFromInstance(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgentDeployment) (*appsv1.DaemonSet, string, error) {
	template, err := newAgentPodTemplate(logger, agentdeployment)
	if err != nil {
		return nil, "", err
	}
	ds := &appsv1.DaemonSet{
		ObjectMeta: newDaemonsetObjectMetaData(agentdeployment),
		Spec: appsv1.DaemonSetSpec{
			Template: *template,
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: *agentdeployment.Spec.Agent.DeploymentStrategy.UpdateStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDaemonSet{
					MaxUnavailable: agentdeployment.Spec.Agent.DeploymentStrategy.RollingUpdate.MaxUnavailable,
				},
			},
		},
	}
	ds.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: ds.Spec.Template.Labels,
	}
	hash, err := comparison.SetMD5AgentSpecAnnotation(&ds.ObjectMeta, agentdeployment)
	if err != nil {
		return nil, "", err
	}
	return ds, hash, nil
}

func newDaemonsetObjectMetaData(agentdeployment *datadoghqv1alpha1.DatadogAgentDeployment) metav1.ObjectMeta {
	labels := getDefaultLabels(agentdeployment, datadoghqv1alpha1.DefaultAgentResourceSuffix, getAgentVersion(agentdeployment))
	labels[datadoghqv1alpha1.AgentDeploymentNameLabelKey] = agentdeployment.Name
	labels[datadoghqv1alpha1.AgentDeploymentComponentLabelKey] = datadoghqv1alpha1.DefaultAgentResourceSuffix
	for key, val := range agentdeployment.Labels {
		labels[key] = val
	}
	annotations := map[string]string{}

	return metav1.ObjectMeta{
		Name:        agentdeployment.Name,
		Namespace:   agentdeployment.Namespace,
		Labels:      labels,
		Annotations: annotations,
	}
}
