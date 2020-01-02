// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagentdeployment

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1beta1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	pdbMinAvailableInstances = 1
)

type (
	pdbBuilder func(dad *datadoghqv1alpha1.DatadogAgentDeployment) *policyv1.PodDisruptionBudget
)

func (r *ReconcileDatadogAgentDeployment) manageClusterAgentPDB(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	cleanUpCondition := dad.Spec.ClusterAgent == nil
	return r.managePDB(logger, dad, newStatus, getClusterAgentPDBName(dad), buildClusterAgentPDB, cleanUpCondition)
}

func (r *ReconcileDatadogAgentDeployment) manageClusterChecksRunnerPDB(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus) (reconcile.Result, error) {
	cleanUpCondition := !needClusterChecksRunner(dad)
	return r.managePDB(logger, dad, newStatus, getClusterChecksRunnerPDBName(dad), buildClusterChecksRunnerPDB, cleanUpCondition)
}

func (r *ReconcileDatadogAgentDeployment) managePDB(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus, pdbName string, builder pdbBuilder, cleanUp bool) (reconcile.Result, error) {
	if cleanUp {
		return r.cleanupPDB(logger, dad, newStatus, pdbName)
	}

	pdb := &policyv1.PodDisruptionBudget{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: dad.Namespace, Name: pdbName}, pdb)
	if err != nil {
		if errors.IsNotFound(err) {
			return r.createPDB(logger, dad, newStatus, builder)
		}
		return reconcile.Result{}, err
	}

	return r.updateIfNeededPDB(logger, dad, pdb, newStatus, builder)
}

func (r *ReconcileDatadogAgentDeployment) createPDB(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus, builder pdbBuilder) (reconcile.Result, error) {
	newPdb := builder(dad)
	// Set DatadogAgentDeployment instance  instance as the owner and controller
	if err := controllerutil.SetControllerReference(dad, newPdb, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	if err := r.client.Create(context.TODO(), newPdb); err != nil {
		return reconcile.Result{}, err
	}
	logger.Info("Create PDB", "name", newPdb.Name)
	r.recordEvent(dad, corev1.EventTypeNormal, "Create PDB", fmt.Sprintf("%s/%s", newPdb.Namespace, newPdb.Name), datadog.CreationEvent)

	return reconcile.Result{Requeue: true}, nil
}

func (r *ReconcileDatadogAgentDeployment) updateIfNeededPDB(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, currentPDB *policyv1.PodDisruptionBudget, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus, builder pdbBuilder) (reconcile.Result, error) {
	if !ownedByDatadogOperator(currentPDB.OwnerReferences) {
		return reconcile.Result{}, nil
	}
	newPDB := builder(dad)
	result := reconcile.Result{}
	if !(apiequality.Semantic.DeepEqual(newPDB.Spec, currentPDB.Spec) &&
		apiequality.Semantic.DeepEqual(newPDB.Labels, currentPDB.Labels) &&
		apiequality.Semantic.DeepEqual(newPDB.Annotations, currentPDB.Annotations)) {

		updatedPDB := currentPDB.DeepCopy()
		updatedPDB.Labels = newPDB.Labels
		updatedPDB.Annotations = newPDB.Annotations
		updatedPDB.Spec = newPDB.Spec

		if err := r.client.Update(context.TODO(), updatedPDB); err != nil {
			return reconcile.Result{}, err
		}
		r.recordEvent(dad, corev1.EventTypeNormal, "Update PDB", fmt.Sprintf("%s/%s", updatedPDB.Namespace, updatedPDB.Name), datadog.UpdateEvent)
		result.Requeue = true
	}

	return result, nil
}

func (r *ReconcileDatadogAgentDeployment) cleanupPDB(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, newStatus *datadoghqv1alpha1.DatadogAgentDeploymentStatus, pdbName string) (reconcile.Result, error) {
	pdb := &policyv1.PodDisruptionBudget{}
	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: dad.Namespace, Name: pdbName}, pdb)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	if ownedByDatadogOperator(pdb.OwnerReferences) {
		err = r.client.Delete(context.TODO(), pdb)
	}

	return reconcile.Result{}, err
}

func buildClusterAgentPDB(dad *datadoghqv1alpha1.DatadogAgentDeployment) *policyv1.PodDisruptionBudget {
	labels := getDefaultLabels(dad, datadoghqv1alpha1.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dad))
	annotations := getDefaultAnnotations(dad)
	metadata := metav1.ObjectMeta{
		Name:        getClusterAgentPDBName(dad),
		Namespace:   dad.Namespace,
		Labels:      labels,
		Annotations: annotations,
	}
	matchLabels := map[string]string{
		datadoghqv1alpha1.AgentDeploymentNameLabelKey:      dad.Name,
		datadoghqv1alpha1.AgentDeploymentComponentLabelKey: datadoghqv1alpha1.DefaultClusterAgentResourceSuffix,
	}

	return buildPDB(metadata, matchLabels, pdbMinAvailableInstances)
}

func buildClusterChecksRunnerPDB(dad *datadoghqv1alpha1.DatadogAgentDeployment) *policyv1.PodDisruptionBudget {
	labels := getDefaultLabels(dad, datadoghqv1alpha1.DefaultClusterChecksRunnerResourceSuffix, getAgentVersion(dad))
	annotations := getDefaultAnnotations(dad)
	metadata := metav1.ObjectMeta{
		Name:        getClusterChecksRunnerPDBName(dad),
		Namespace:   dad.Namespace,
		Labels:      labels,
		Annotations: annotations,
	}
	matchLabels := map[string]string{
		datadoghqv1alpha1.AgentDeploymentNameLabelKey:      dad.Name,
		datadoghqv1alpha1.AgentDeploymentComponentLabelKey: datadoghqv1alpha1.DefaultClusterChecksRunnerResourceSuffix,
	}

	return buildPDB(metadata, matchLabels, pdbMinAvailableInstances)
}

func buildPDB(metadata metav1.ObjectMeta, matchLabels map[string]string, minAvailable int) *policyv1.PodDisruptionBudget {
	minAvailableStr := intstr.FromInt(pdbMinAvailableInstances)

	pdb := &policyv1.PodDisruptionBudget{
		ObjectMeta: metadata,
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvailableStr,
			Selector: &metav1.LabelSelector{
				MatchLabels: matchLabels,
			},
		},
	}

	return pdb
}
