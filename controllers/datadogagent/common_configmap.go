// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"

	"github.com/DataDog/datadog-operator/apis/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/configmap"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

type buildConfigMapFunc func(dda *datadoghqv1alpha1.DatadogAgent) (*corev1.ConfigMap, error)

func (r *Reconciler) manageConfigMap(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name string, buildFunc buildConfigMapFunc) (reconcile.Result, error) {
	result := reconcile.Result{}
	newConfigMap, err := buildFunc(dda)
	if err != nil {
		return result, err
	}

	if newConfigMap == nil {
		return r.cleanupConfigMap(logger, dda, name)
	}

	configmap := &corev1.ConfigMap{}
	nameNamespace := types.NamespacedName{Name: newConfigMap.Name, Namespace: newConfigMap.Namespace}
	if err = r.client.Get(context.TODO(), nameNamespace, configmap); err != nil {
		if errors.IsNotFound(err) {
			return r.createConfigMap(logger, dda, newConfigMap)
		}
		return result, err
	}

	if result, err = r.updateIfNeededConfigMap(dda, configmap, newConfigMap); err != nil {
		return result, err
	}
	return result, nil
}

func (r *Reconciler) updateIfNeededConfigMap(dda *datadoghqv1alpha1.DatadogAgent, oldConfigMap, newConfigMap *corev1.ConfigMap) (reconcile.Result, error) {
	result := reconcile.Result{}
	hash, err := comparison.GenerateMD5ForSpec(newConfigMap.Data)
	if err != nil {
		return result, err
	}
	if comparison.IsSameSpecMD5Hash(hash, oldConfigMap.GetAnnotations()) {
		return result, nil
	}

	if err = controllerutil.SetControllerReference(dda, newConfigMap, r.scheme); err != nil {
		return reconcile.Result{}, err
	}
	// Copy possibly changed fields
	updateCM := oldConfigMap.DeepCopy()
	updateCM.Data = newConfigMap.Data
	if updateCM.Annotations == nil {
		updateCM.Annotations = make(map[string]string)
	}
	for k, v := range newConfigMap.Annotations {
		updateCM.Annotations[k] = v
	}
	if updateCM.Labels == nil {
		updateCM.Labels = make(map[string]string)
	}
	for k, v := range newConfigMap.Labels {
		updateCM.Labels[k] = v
	}

	// need to update the hash in the updated configmap to avoid going through this loop everytime
	_, err = comparison.SetMD5DatadogAgentGenerationAnnotation(&updateCM.ObjectMeta, updateCM.Data)
	if err != nil {
		return reconcile.Result{}, err
	}
	err = kubernetes.UpdateFromObject(context.TODO(), r.client, updateCM, oldConfigMap.ObjectMeta)
	if err != nil {
		return reconcile.Result{}, err
	}
	event := buildEventInfo(updateCM.Name, updateCM.Namespace, configMapKind, datadog.UpdateEvent)
	r.recordEvent(dda, event)

	return result, nil
}

func (r *Reconciler) createConfigMap(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, configMap *corev1.ConfigMap) (reconcile.Result, error) {
	result := reconcile.Result{}
	_, err := comparison.SetMD5DatadogAgentGenerationAnnotation(&configMap.ObjectMeta, configMap.Data)
	if err != nil {
		return result, err
	}
	// Set DatadogAgent instance  instance as the owner and controller
	if err = controllerutil.SetControllerReference(dda, configMap, r.scheme); err != nil {
		return result, err
	}

	if err = r.client.Create(context.TODO(), configMap); err != nil {
		return result, err
	}
	logger.V(1).Info("createConfigMap", "configMap.name", configMap.Name, "configMap.Namespace", configMap.Namespace)
	event := buildEventInfo(configMap.Name, configMap.Namespace, configMapKind, datadog.CreationEvent)
	r.recordEvent(dda, event)
	result.Requeue = true
	return result, err
}

func (r *Reconciler) cleanupConfigMap(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, name string) (reconcile.Result, error) {
	configmap := &corev1.ConfigMap{}
	nsName := types.NamespacedName{Name: name, Namespace: dda.Namespace}
	err := r.client.Get(context.TODO(), nsName, configmap)
	if err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	if !CheckOwnerReference(dda, configmap) {
		return reconcile.Result{}, nil
	}
	logger.V(1).Info("deleteConfigMap", "configMap.name", configmap.Name, "configMap.Namespace", configmap.Namespace)
	event := buildEventInfo(configmap.Name, configmap.Namespace, configMapKind, datadog.DeletionEvent)
	r.recordEvent(dda, event)
	return reconcile.Result{}, r.client.Delete(context.TODO(), configmap)
}

func buildConfigurationConfigMap(owner metav1.Object, cfcm *common.CustomConfig, configMapName, subPath string) (*corev1.ConfigMap, error) {
	if cfcm == nil || cfcm.ConfigData == nil {
		return nil, nil
	}

	return configmap.BuildConfiguration(owner, cfcm.ConfigData, configMapName, subPath)
}
