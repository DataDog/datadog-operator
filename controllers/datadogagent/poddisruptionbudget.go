// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	policyv1 "k8s.io/api/policy/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
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
	pdbBuilder        func(dda *datadoghqv1alpha1.DatadogAgent) *policyv1.PodDisruptionBudget
	pdbV1Beta1Builder func(dda *datadoghqv1alpha1.DatadogAgent) *policyv1beta1.PodDisruptionBudget
)

func (r *Reconciler) manageClusterAgentPDB(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	if dda.Spec.ClusterAgent.Replicas != nil && *dda.Spec.ClusterAgent.Replicas > 1 {
		cleanUpCondition := !isClusterAgentEnabled(dda.Spec.ClusterAgent)
		return r.managePDB(logger, dda, getClusterAgentPDBName(dda), buildClusterAgentPDBV1, buildClusterAgentPDBV1Beta1, cleanUpCondition)
	}
	return reconcile.Result{}, nil
}

func (r *Reconciler) manageClusterChecksRunnerPDB(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	cleanUpCondition := !needClusterChecksRunner(dda)
	return r.managePDB(logger, dda, getClusterChecksRunnerPDBName(dda), buildClusterChecksRunnerPDBV1, buildClusterChecksRunnerPDBV1Beta1, cleanUpCondition)
}

func (r *Reconciler) managePDB(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, pdbName string, builder pdbBuilder, builderv1beta1 pdbV1Beta1Builder, cleanUp bool) (reconcile.Result, error) {
	useV1Beta1 := r.platformInfo.UseV1Beta1PDB()
	if useV1Beta1 {
		pdbV1Beta1 := &policyv1beta1.PodDisruptionBudget{}
		if cleanUp {
			return r.cleanupPDB(dda, pdbName, pdbV1Beta1)
		}
		err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: dda.Namespace, Name: pdbName}, pdbV1Beta1)
		if err != nil {
			if errors.IsNotFound(err) {
				return r.createPDBV1Beta1(logger, dda, builderv1beta1)
			}
			return reconcile.Result{}, err
		}
		r.updateIfNeededPDBV1Beta1(dda, pdbV1Beta1, builderv1beta1)
	} else {
		pdbV1 := &policyv1.PodDisruptionBudget{}
		if cleanUp {
			return r.cleanupPDB(dda, pdbName, pdbV1)
		}

		err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: dda.Namespace, Name: pdbName}, pdbV1)
		if err != nil {
			if errors.IsNotFound(err) {
				return r.createPDBV1(logger, dda, builder)
			}
			return reconcile.Result{}, err
		}
		r.updateIfNeededPDBV1(dda, pdbV1, builder)
	}

	return reconcile.Result{}, nil
}

func (r *Reconciler) createPDBV1(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, builder pdbBuilder) (reconcile.Result, error) {
	newPdb := builder(dda)
	// Set DatadogAgent instance  instance as the owner and controller
	if err := controllerutil.SetControllerReference(dda, newPdb, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	if err := r.client.Create(context.TODO(), newPdb); err != nil {
		return reconcile.Result{}, err
	}
	logger.Info("Create PDB", "name", newPdb.Name)
	event := buildEventInfo(newPdb.Name, newPdb.Namespace, podDisruptionBudgetKind, datadog.CreationEvent)
	r.recordEvent(dda, event)

	return reconcile.Result{Requeue: true}, nil
}

func (r *Reconciler) createPDBV1Beta1(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, builder pdbV1Beta1Builder) (reconcile.Result, error) {
	newPdb := builder(dda)
	// Set DatadogAgent instance  instance as the owner and controller
	if err := controllerutil.SetControllerReference(dda, newPdb, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	if err := r.client.Create(context.TODO(), newPdb); err != nil {
		return reconcile.Result{}, err
	}
	logger.Info("Create PDB", "name", newPdb.Name)
	event := buildEventInfo(newPdb.Name, newPdb.Namespace, podDisruptionBudgetKind, datadog.CreationEvent)
	r.recordEvent(dda, event)

	return reconcile.Result{Requeue: true}, nil
}

func (r *Reconciler) updateIfNeededPDBV1(dda *datadoghqv1alpha1.DatadogAgent, currentPDB *policyv1.PodDisruptionBudget, builder pdbBuilder) (reconcile.Result, error) {
	if !CheckOwnerReference(dda, currentPDB) {
		return reconcile.Result{}, nil
	}
	newPDB := builder(dda)
	result := reconcile.Result{}
	if !(apiequality.Semantic.DeepEqual(newPDB.Spec, currentPDB.Spec) &&
		apiequality.Semantic.DeepEqual(newPDB.Labels, currentPDB.Labels) &&
		apiequality.Semantic.DeepEqual(newPDB.Annotations, currentPDB.Annotations)) {
		updatedPDB := currentPDB.DeepCopy()
		updatedPDB.Labels = newPDB.Labels
		updatedPDB.Annotations = newPDB.Annotations
		updatedPDB.Spec = newPDB.Spec

		if err := kubernetes.UpdateFromObject(context.TODO(), r.client, updatedPDB, currentPDB.ObjectMeta); err != nil {
			return reconcile.Result{}, err
		}
		event := buildEventInfo(updatedPDB.Name, updatedPDB.Namespace, podDisruptionBudgetKind, datadog.UpdateEvent)
		r.recordEvent(dda, event)
		result.Requeue = true
	}

	return result, nil
}

func (r *Reconciler) updateIfNeededPDBV1Beta1(dda *datadoghqv1alpha1.DatadogAgent, currentPDB *policyv1beta1.PodDisruptionBudget, builder pdbV1Beta1Builder) (reconcile.Result, error) {
	if !CheckOwnerReference(dda, currentPDB) {
		return reconcile.Result{}, nil
	}
	newPDB := builder(dda)
	result := reconcile.Result{}
	if !(apiequality.Semantic.DeepEqual(newPDB.Spec, currentPDB.Spec) &&
		apiequality.Semantic.DeepEqual(newPDB.Labels, currentPDB.Labels) &&
		apiequality.Semantic.DeepEqual(newPDB.Annotations, currentPDB.Annotations)) {
		updatedPDB := currentPDB.DeepCopy()
		updatedPDB.Labels = newPDB.Labels
		updatedPDB.Annotations = newPDB.Annotations
		updatedPDB.Spec = newPDB.Spec

		if err := kubernetes.UpdateFromObject(context.TODO(), r.client, updatedPDB, currentPDB.ObjectMeta); err != nil {
			return reconcile.Result{}, err
		}
		event := buildEventInfo(updatedPDB.Name, updatedPDB.Namespace, podDisruptionBudgetKind, datadog.UpdateEvent)
		r.recordEvent(dda, event)
		result.Requeue = true
	}

	return result, nil
}

func (r *Reconciler) cleanupPDB(dda *datadoghqv1alpha1.DatadogAgent, pdbName string, pdbObj client.Object) (reconcile.Result, error) {
	err := r.client.Get(context.TODO(), types.NamespacedName{Namespace: dda.Namespace, Name: pdbName}, pdbObj)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	if CheckOwnerReference(dda, pdbObj) {
		err = r.client.Delete(context.TODO(), pdbObj)
	}

	return reconcile.Result{}, err
}

func buildClusterAgentPDBV1(dda *datadoghqv1alpha1.DatadogAgent) *policyv1.PodDisruptionBudget {
	labels := object.GetDefaultLabels(dda, apicommon.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
	annotations := object.GetDefaultAnnotations(dda)
	metadata := metav1.ObjectMeta{
		Name:        getClusterAgentPDBName(dda),
		Namespace:   dda.Namespace,
		Labels:      labels,
		Annotations: annotations,
	}
	matchLabels := map[string]string{
		apicommon.AgentDeploymentNameLabelKey:     dda.Name,
		kubernetes.AppKubernetesComponentLabelKey: apicommon.DefaultClusterAgentResourceSuffix,
	}

	return buildPDBV1(metadata, matchLabels, pdbMinAvailableInstances)
}

func buildClusterAgentPDBV1Beta1(dda *datadoghqv1alpha1.DatadogAgent) *policyv1beta1.PodDisruptionBudget {
	labels := object.GetDefaultLabels(dda, apicommon.DefaultClusterAgentResourceSuffix, getClusterAgentVersion(dda))
	annotations := object.GetDefaultAnnotations(dda)
	metadata := metav1.ObjectMeta{
		Name:        getClusterAgentPDBName(dda),
		Namespace:   dda.Namespace,
		Labels:      labels,
		Annotations: annotations,
	}
	matchLabels := map[string]string{
		apicommon.AgentDeploymentNameLabelKey:     dda.Name,
		kubernetes.AppKubernetesComponentLabelKey: apicommon.DefaultClusterAgentResourceSuffix,
	}

	return buildPDBV1Beta1(metadata, matchLabels, pdbMinAvailableInstances)
}

func buildClusterChecksRunnerPDBV1(dda *datadoghqv1alpha1.DatadogAgent) *policyv1.PodDisruptionBudget {
	labels := object.GetDefaultLabels(dda, apicommon.DefaultClusterChecksRunnerResourceSuffix, getAgentVersion(dda))
	annotations := object.GetDefaultAnnotations(dda)
	metadata := metav1.ObjectMeta{
		Name:        getClusterChecksRunnerPDBName(dda),
		Namespace:   dda.Namespace,
		Labels:      labels,
		Annotations: annotations,
	}
	matchLabels := map[string]string{
		apicommon.AgentDeploymentNameLabelKey:     dda.Name,
		kubernetes.AppKubernetesComponentLabelKey: apicommon.DefaultClusterChecksRunnerResourceSuffix,
	}

	return buildPDBV1(metadata, matchLabels, pdbMinAvailableInstances)
}

func buildClusterChecksRunnerPDBV1Beta1(dda *datadoghqv1alpha1.DatadogAgent) *policyv1beta1.PodDisruptionBudget {
	labels := object.GetDefaultLabels(dda, apicommon.DefaultClusterChecksRunnerResourceSuffix, getAgentVersion(dda))
	annotations := object.GetDefaultAnnotations(dda)
	metadata := metav1.ObjectMeta{
		Name:        getClusterChecksRunnerPDBName(dda),
		Namespace:   dda.Namespace,
		Labels:      labels,
		Annotations: annotations,
	}
	matchLabels := map[string]string{
		apicommon.AgentDeploymentNameLabelKey:     dda.Name,
		kubernetes.AppKubernetesComponentLabelKey: apicommon.DefaultClusterChecksRunnerResourceSuffix,
	}

	return buildPDBV1Beta1(metadata, matchLabels, pdbMinAvailableInstances)
}

func buildPDBV1(metadata metav1.ObjectMeta, matchLabels map[string]string, minAvailable int) *policyv1.PodDisruptionBudget {
	minAvailableStr := intstr.FromInt(minAvailable)

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

func buildPDBV1Beta1(metadata metav1.ObjectMeta, matchLabels map[string]string, minAvailable int) *policyv1beta1.PodDisruptionBudget {
	minAvailableStr := intstr.FromInt(minAvailable)

	pdb := &policyv1beta1.PodDisruptionBudget{
		ObjectMeta: metadata,
		Spec: policyv1beta1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvailableStr,
			Selector: &metav1.LabelSelector{
				MatchLabels: matchLabels,
			},
		},
	}

	return pdb
}
