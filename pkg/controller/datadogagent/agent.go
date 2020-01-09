// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagent

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
	"sigs.k8s.io/yaml"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	edsdatadoghqv1alpha1 "github.com/datadog/extendeddaemonset/pkg/apis/datadoghq/v1alpha1"
)

func (r *ReconcileDatadogAgent) reconcileAgent(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	result, err := r.manageAgentDependencies(logger, dda, newStatus)
	if shouldReturn(result, err) {
		return result, err
	}

	if newStatus.Agent != nil && newStatus.Agent.DaemonsetName != "" && newStatus.Agent.DaemonsetName != daemonsetName(dda) {
		return result, fmt.Errorf("Datadog agent DaemonSet cannot be renamed once created")
	}

	nameNamespace := types.NamespacedName{
		Name:      daemonsetName(dda),
		Namespace: dda.ObjectMeta.Namespace,
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

	if dda.Spec.Agent == nil {
		if ds != nil {
			if err = r.deleteDaemonSet(logger, dda, ds); err != nil {
				return result, err
			}
		}
		if eds != nil {
			if err = r.deleteExtendedDaemonSet(logger, dda, eds); err != nil {
				return result, err
			}
		}
		newStatus.Agent = nil
		return result, err
	}

	if supportExtendedDaemonset && datadoghqv1alpha1.BoolValue(dda.Spec.Agent.UseExtendedDaemonset) {
		if ds != nil {
			// TODO manage properly the migration from DS to EDS
			err = r.deleteDaemonSet(logger, dda, ds)
			if err != nil {
				return result, err
			}
			result.RequeueAfter = 5 * time.Second
			return result, nil
		}
		if eds == nil {
			return r.createNewExtendedDaemonSet(logger, dda, newStatus)
		}

		return r.updateExtendedDaemonSet(logger, dda, eds, newStatus)
	}

	// Case when Daemonset is requested
	if eds != nil && supportExtendedDaemonset {
		// if EDS exist delete before creating or updating the Daemonset
		err = r.deleteExtendedDaemonSet(logger, dda, eds)
		if err != nil {
			return result, err
		}
		result.RequeueAfter = 5 * time.Second
		return result, nil
	}
	if ds == nil {
		return r.createNewDaemonSet(logger, dda, newStatus)
	}

	return r.updateDaemonSet(logger, dda, ds, newStatus)

}

func (r *ReconcileDatadogAgent) deleteDaemonSet(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgent, ds *appsv1.DaemonSet) error {
	err := r.client.Delete(context.TODO(), ds)
	if err != nil {
		return err
	}
	logger.Info("Delete DaemonSet", "daemonSet.Namespace", ds.Namespace, "daemonSet.Name", ds.Name)
	eventInfo := buildEventInfo(ds.Name, ds.Namespace, daemonSetKind, datadog.DeletionEvent)
	r.recordEvent(agentdeployment, eventInfo)
	return err
}

func (r *ReconcileDatadogAgent) deleteExtendedDaemonSet(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgent, eds *edsdatadoghqv1alpha1.ExtendedDaemonSet) error {
	err := r.client.Delete(context.TODO(), eds)
	if err != nil {
		return err
	}
	logger.Info("Delete DaemonSet", "extendedDaemonSet.Namespace", eds.Namespace, "extendedDaemonSet.Name", eds.Name)
	eventInfo := buildEventInfo(eds.Name, eds.Namespace, extendedDaemonSetKind, datadog.DeletionEvent)
	r.recordEvent(agentdeployment, eventInfo)
	return err
}

func (r *ReconcileDatadogAgent) createNewExtendedDaemonSet(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	var err error
	// ExtendedDaemonSet up to date didn't exist yet, create a new one
	var newEDS *edsdatadoghqv1alpha1.ExtendedDaemonSet
	var hash string
	if newEDS, hash, err = newExtendedDaemonSetFromInstance(logger, agentdeployment, nil); err != nil {
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
	eventInfo := buildEventInfo(newEDS.Name, newEDS.Namespace, extendedDaemonSetKind, datadog.CreationEvent)
	r.recordEvent(agentdeployment, eventInfo)
	now := metav1.NewTime(time.Now())
	newStatus.Agent = updateExtendedDaemonSetStatus(newEDS, newStatus.Agent, &now)

	return reconcile.Result{}, nil
}

func (r *ReconcileDatadogAgent) createNewDaemonSet(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	var err error
	// DaemonSet up to date didn't exist yet, create a new one
	var newDS *appsv1.DaemonSet
	var hash string
	if newDS, hash, err = newDaemonSetFromInstance(logger, agentdeployment, nil); err != nil {
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
	eventInfo := buildEventInfo(newDS.Name, newDS.Namespace, daemonSetKind, datadog.CreationEvent)
	r.recordEvent(agentdeployment, eventInfo)
	now := metav1.NewTime(time.Now())
	newStatus.Agent = updateDaemonSetStatus(newDS, newStatus.Agent, &now)

	return reconcile.Result{}, nil
}

func (r *ReconcileDatadogAgent) updateExtendedDaemonSet(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgent, eds *edsdatadoghqv1alpha1.ExtendedDaemonSet, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	newEDS, newHash, err := newExtendedDaemonSetFromInstance(logger, agentdeployment, eds.Spec.Selector)
	if err != nil {
		return reconcile.Result{}, err
	}

	if comparison.IsSameSpecMD5Hash(newHash, eds.GetAnnotations()) {
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
	eventInfo := buildEventInfo(updatedEds.Name, updatedEds.Namespace, extendedDaemonSetKind, datadog.UpdateEvent)
	r.recordEvent(agentdeployment, eventInfo)
	newStatus.Agent = updateExtendedDaemonSetStatus(updatedEds, newStatus.Agent, &now)
	return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
}

func getHashAnnotation(annotations map[string]string) string {
	return annotations[datadoghqv1alpha1.MD5AgentDeploymentAnnotationKey]
}

func (r *ReconcileDatadogAgent) updateDaemonSet(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgent, ds *appsv1.DaemonSet, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	// Update values from current DS in any case
	updateDaemonSetStatus(ds, newStatus.Agent, nil)

	newDS, newHash, err := newDaemonSetFromInstance(logger, agentdeployment, ds.Spec.Selector)
	if err != nil {
		return reconcile.Result{}, err
	}

	if comparison.IsSameSpecMD5Hash(newHash, ds.GetAnnotations()) {
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
	eventInfo := buildEventInfo(updatedDS.Name, updatedDS.Namespace, daemonSetKind, datadog.UpdateEvent)
	r.recordEvent(agentdeployment, eventInfo)
	newStatus.Agent = updateDaemonSetStatus(updatedDS, newStatus.Agent, &now)
	return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *ReconcileDatadogAgent) manageAgentDependencies(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	result, err := r.manageAgentRBACs(logger, dda)
	if shouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageSystemProbeDependencies(logger, dda)
	if shouldReturn(result, err) {
		return result, err
	}

	result, err = r.manageConfigMap(logger, dda, getAgentCustomConfigConfigMapName(dda), buildAgentConfigurationConfigMap)
	if shouldReturn(result, err) {
		return result, err
	}

	return reconcile.Result{}, nil
}

// newExtendedDaemonSetFromInstance creates an ExtendedDaemonSet from a given DatadogAgent
func newExtendedDaemonSetFromInstance(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgent, selector *metav1.LabelSelector) (*edsdatadoghqv1alpha1.ExtendedDaemonSet, string, error) {
	template, err := newAgentPodTemplate(logger, agentdeployment, selector)
	if err != nil {
		return nil, "", err
	}
	eds := &edsdatadoghqv1alpha1.ExtendedDaemonSet{
		ObjectMeta: newDaemonsetObjectMetaData(agentdeployment),
		Spec: edsdatadoghqv1alpha1.ExtendedDaemonSetSpec{
			Selector: selector,
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
	hash, err := comparison.SetMD5GenerationAnnotation(&eds.ObjectMeta, agentdeployment.Spec)
	if err != nil {
		return nil, "", err
	}
	return eds, hash, nil
}

// newDaemonSetFromInstance creates a DaemonSet from a given DatadogAgent
func newDaemonSetFromInstance(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgent, selector *metav1.LabelSelector) (*appsv1.DaemonSet, string, error) {
	template, err := newAgentPodTemplate(logger, agentdeployment, selector)
	if err != nil {
		return nil, "", err
	}

	if selector == nil {
		selector = &metav1.LabelSelector{
			MatchLabels: template.Labels,
		}
	}
	ds := &appsv1.DaemonSet{
		ObjectMeta: newDaemonsetObjectMetaData(agentdeployment),
		Spec: appsv1.DaemonSetSpec{
			Selector: selector,
			Template: *template,
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: *agentdeployment.Spec.Agent.DeploymentStrategy.UpdateStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDaemonSet{
					MaxUnavailable: agentdeployment.Spec.Agent.DeploymentStrategy.RollingUpdate.MaxUnavailable,
				},
			},
		},
	}
	hash, err := comparison.SetMD5GenerationAnnotation(&ds.ObjectMeta, agentdeployment.Spec)
	if err != nil {
		return nil, "", err
	}
	return ds, hash, nil
}

func daemonsetName(agentdeployment *datadoghqv1alpha1.DatadogAgent) string {
	if agentdeployment.Spec.Agent.DaemonsetName != "" {
		return agentdeployment.Spec.Agent.DaemonsetName
	}
	return agentdeployment.Name
}

func newDaemonsetObjectMetaData(agentdeployment *datadoghqv1alpha1.DatadogAgent) metav1.ObjectMeta {
	labels := getDefaultLabels(agentdeployment, datadoghqv1alpha1.DefaultAgentResourceSuffix, getAgentVersion(agentdeployment))
	labels[datadoghqv1alpha1.AgentDeploymentNameLabelKey] = agentdeployment.Name
	labels[datadoghqv1alpha1.AgentDeploymentComponentLabelKey] = datadoghqv1alpha1.DefaultAgentResourceSuffix
	for key, val := range agentdeployment.Labels {
		labels[key] = val
	}
	annotations := map[string]string{}

	return metav1.ObjectMeta{
		Name:        daemonsetName(agentdeployment),
		Namespace:   agentdeployment.Namespace,
		Labels:      labels,
		Annotations: annotations,
	}
}

func getAgentCustomConfigConfigMapName(dda *datadoghqv1alpha1.DatadogAgent) string {
	return fmt.Sprintf("%s-datadog-yaml", dda.Name)
}

func buildAgentConfigurationConfigMap(dda *datadoghqv1alpha1.DatadogAgent) (*corev1.ConfigMap, error) {
	if dda.Spec.Agent.CustomConfig == "" {
		return nil, nil
	}

	// Validate that user input is valid YAML
	// Maybe later we can implement that directly verifies against Agent configuration?
	m := make(map[interface{}]interface{})
	if err := yaml.Unmarshal([]byte(dda.Spec.Agent.CustomConfig), m); err != nil {
		return nil, fmt.Errorf("Unable to parse YAML from 'Agent.CustomConfig' field: %w", err)
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        getAgentCustomConfigConfigMapName(dda),
			Namespace:   dda.Namespace,
			Labels:      getDefaultLabels(dda, dda.Name, getAgentVersion(dda)),
			Annotations: getDefaultAnnotations(dda),
		},
		Data: map[string]string{
			datadoghqv1alpha1.AgentCustomConfigVolumeSubPath: dda.Spec.Agent.CustomConfig,
		},
	}

	return configMap, nil
}
