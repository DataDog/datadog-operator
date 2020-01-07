// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagentdeployment

import (
	"context"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/pkg/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

type buildConfigMapFunc func(dad *datadoghqv1alpha1.DatadogAgentDeployment) (*corev1.ConfigMap, error)

func (r *ReconcileDatadogAgentDeployment) manageConfigMap(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, name string, buildFunc buildConfigMapFunc) (reconcile.Result, error) {
	result := reconcile.Result{}
	newConfigMap, err := buildFunc(dad)
	if err != nil {
		return result, err
	}

	if newConfigMap == nil {
		return r.cleanupConfigMap(logger, dad, name)
	}

	configmap := &corev1.ConfigMap{}
	nameNamespace := types.NamespacedName{Name: newConfigMap.Name, Namespace: newConfigMap.Namespace}
	if err = r.client.Get(context.TODO(), nameNamespace, configmap); err != nil {
		if errors.IsNotFound(err) {
			return r.createConfigMap(logger, dad, newConfigMap)
		}
		return result, err
	}

	if result, err := r.updateIfNeededConfigMap(logger, dad, configmap, newConfigMap); err != nil {
		return result, err
	}
	return result, nil
}

func (r *ReconcileDatadogAgentDeployment) updateIfNeededConfigMap(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, oldConfigMap, newConfigMap *corev1.ConfigMap) (reconcile.Result, error) {
	result := reconcile.Result{}
	hash, err := comparison.GenerateMD5ForSpec(newConfigMap.Data)
	if err != nil {
		return result, err
	}

	if comparison.IsSameSpecMD5Hash(hash, oldConfigMap.GetAnnotations()) {
		return result, nil
	}

	if err = controllerutil.SetControllerReference(dad, newConfigMap, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	// Copy possibly changed fields
	updateCM := oldConfigMap.DeepCopy()
	updateCM.Data = newConfigMap.Data
	for k, v := range newConfigMap.Annotations {
		updateCM.Annotations[k] = v
	}
	for k, v := range newConfigMap.Labels {
		updateCM.Labels[k] = v
	}

	err = r.client.Update(context.TODO(), updateCM)
	if err != nil {
		return reconcile.Result{}, err
	}
	eventInfo := buildEventInfo(updateCM.Name, updateCM.Namespace, configMapKind, datadog.UpdateEvent)
	r.recordEvent(dad, eventInfo)

	return result, nil
}

func (r *ReconcileDatadogAgentDeployment) createConfigMap(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, configMap *corev1.ConfigMap) (reconcile.Result, error) {
	result := reconcile.Result{}
	_, err := comparison.SetMD5GenerationAnnotation(&configMap.ObjectMeta, configMap.Data)
	if err != nil {
		return result, err
	}
	// Set DatadogAgentDeployment instance  instance as the owner and controller
	if err = controllerutil.SetControllerReference(dad, configMap, r.scheme); err != nil {
		return result, err
	}

	if err = r.client.Create(context.TODO(), configMap); err != nil {
		return result, err
	}
	logger.V(1).Info("createConfigMap", "configMap.name", configMap.Name, "configMap.Namespace", configMap.Namespace)
	eventInfo := buildEventInfo(configMap.Name, configMap.Namespace, configMapKind, datadog.CreationEvent)
	r.recordEvent(dad, eventInfo)
	result.Requeue = true
	return result, err
}

func (r *ReconcileDatadogAgentDeployment) cleanupConfigMap(logger logr.Logger, dad *datadoghqv1alpha1.DatadogAgentDeployment, name string) (reconcile.Result, error) {
	configmap := &corev1.ConfigMap{}
	nsName := types.NamespacedName{Name: name, Namespace: dad.Namespace}
	err := r.client.Get(context.TODO(), nsName, configmap)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	if !ownedByDatadogOperator(configmap.OwnerReferences) {
		return reconcile.Result{}, nil
	}
	logger.V(1).Info("deleteConfigMap", "configMap.name", configmap.Name, "configMap.Namespace", configmap.Namespace)
	eventInfo := buildEventInfo(configmap.Name, configmap.Namespace, configMapKind, datadog.DeletionEvent)
	r.recordEvent(dad, eventInfo)
	return reconcile.Result{}, r.client.Delete(context.TODO(), configmap)
}
