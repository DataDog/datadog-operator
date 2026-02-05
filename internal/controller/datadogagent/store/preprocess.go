// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package store

import (
	"fmt"
	"strings"

	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

// preprocessorFunc defines a function type for preprocessing objects before apply.
// objStore is the object from the store and objAPIServer is the object from the API server (if it exists).
// objAPIServer will be nil for creates, non-nil for updates.
type preprocessorFunc func(ds *Store, objStore, objAPIServer client.Object) (client.Object, error)

var preprocessorRegistry = map[kubernetes.ObjectKind]preprocessorFunc{
	kubernetes.ClusterRolesKind:          preprocessClusterRole,
	kubernetes.RolesKind:                 preprocessRole,
	kubernetes.ServicesKind:              preprocessService,
	kubernetes.ConfigMapsKind:            preprocessConfigMap,
	kubernetes.APIServiceKind:            preprocessResourceVersion,
	kubernetes.CiliumNetworkPoliciesKind: preprocessResourceVersion,
	kubernetes.PodDisruptionBudgetsKind:  preprocessResourceVersion,
}

// applyPreprocessing applies registered preprocessor for the given kind, if any
func (ds *Store) applyPreprocessing(kind kubernetes.ObjectKind, objStore, objAPIServer client.Object) (client.Object, error) {
	if preprocessor, exists := preprocessorRegistry[kind]; exists {
		return preprocessor(ds, objStore, objAPIServer)
	}
	return objStore, nil
}

// preprocessClusterRole holds preprocessing rules for ClusterRole
// - normalizes policy rules to minimize duplicates
// - ensures deterministic output
func preprocessClusterRole(ds *Store, objStore, objAPIServer client.Object) (client.Object, error) {
	cr, ok := objStore.(*rbacv1.ClusterRole)
	if !ok {
		return nil, fmt.Errorf("expected *rbacv1.ClusterRole, got %T", objStore)
	}
	if len(cr.Rules) > 0 {
		cr.Rules = rbac.NormalizePolicyRules(cr.Rules)
	}
	return cr, nil
}

// preprocessRole holds preprocessing rules for Role
// - normalizes policy rules to minimize duplicates
// - ensures deterministic output
func preprocessRole(ds *Store, objStore, objAPIServer client.Object) (client.Object, error) {
	role, ok := objStore.(*rbacv1.Role)
	if !ok {
		return nil, fmt.Errorf("expected *rbacv1.Role, got %T", objStore)
	}
	if len(role.Rules) > 0 {
		role.Rules = rbac.NormalizePolicyRules(role.Rules)
	}
	return role, nil
}

// preprocessService holds preprocessing rules for Service
// - ClusterIP and ClusterIPs are immutable and must be preserved during updates
// - sets the resource version from the API server object if it exists to prevent invalid value error
func preprocessService(ds *Store, objStore, objAPIServer client.Object) (client.Object, error) {
	svcStore, ok := objStore.(*v1.Service)
	if !ok {
		return nil, fmt.Errorf("expected *v1.Service, got %T", objStore)
	}
	if objAPIServer != nil {
		svcAPI, ok := objAPIServer.(*v1.Service)
		if !ok {
			return nil, fmt.Errorf("expected *v1.Service from API server, got %T", objAPIServer)
		}
		svcStore.Spec.ClusterIP = svcAPI.Spec.ClusterIP
		svcStore.Spec.ClusterIPs = svcAPI.Spec.ClusterIPs
		svcStore.SetResourceVersion(svcAPI.GetResourceVersion())
	}
	return svcStore, nil
}

// preprocessConfigMap holds preprocessing rules for ConfigMap
func preprocessConfigMap(ds *Store, objStore, objAPIServer client.Object) (client.Object, error) {
	cm, ok := objStore.(*v1.ConfigMap)
	if !ok {
		return nil, fmt.Errorf("expected *v1.ConfigMap, got %T", objStore)
	}

	// Generate annotation key from config ID
	id, ok := cm.Labels[constants.ConfigIDLabelKey]
	if !ok {
		// For now, return the original configmap to continue to equality check
		// TODO: replace once all configmaps have config ID and move to preprocess hash
		// return nil, errors.New("config ID label not found for configmap, unable to store hash")
		ds.logger.V(2).Info("config ID label not found for configmap, unable to store hash", "configmap", cm.Name)
		return cm, nil
	}
	annotationKey := object.GetChecksumAnnotationKey(id)

	// Compute MD5 hash
	hash, err := comparison.GenerateMD5ForSpec(cm.Data)
	if err != nil {
		return nil, err
	}

	// Store hash for each component the configmap belongs to
	components := GetComponentsFromLabels(cm.Labels)
	for _, component := range components {
		if ds.componentAnnotations[component] == nil {
			ds.componentAnnotations[component] = make(map[string]string)
		}
		ds.componentAnnotations[component][annotationKey] = hash
	}

	// Add hash annotation to configmap
	cm.SetAnnotations(object.MergeAnnotationsLabels(ds.logger, cm.GetAnnotations(), map[string]string{annotationKey: hash}, "*"))

	return cm, nil
}

// preprocessResourceVersion sets the resource version from the API server object if it exists
// Required for APIService, CiliumNetworkPolicies, and PodDisruptionBudgets
func preprocessResourceVersion(ds *Store, objStore, objAPIServer client.Object) (client.Object, error) {
	if objAPIServer != nil {
		objStore.SetResourceVersion(objAPIServer.GetResourceVersion())
	}
	return objStore, nil
}

// GetComponentsFromLabels extracts component names from labels
func GetComponentsFromLabels(labels map[string]string) []v2alpha1.ComponentName {
	var components []v2alpha1.ComponentName
	for key := range labels {
		if strings.HasPrefix(key, constants.OperatorComponentLabelKeyPrefix) {
			componentName := strings.TrimPrefix(key, constants.OperatorComponentLabelKeyPrefix)
			components = append(components, v2alpha1.ComponentName(componentName))
		}
	}
	return components
}
