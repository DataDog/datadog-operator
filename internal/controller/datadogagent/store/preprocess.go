// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package store

import (
	"fmt"

	v1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

// preprocessorFunc defines a function type for preprocessing objects before apply.
// objStore is the object from the store and objAPIServer is the object from the API server (if it exists).
// objAPIServer will be nil for creates, non-nil for updates.
type preprocessorFunc func(objStore, objAPIServer client.Object) (client.Object, error)

var preprocessorRegistry = map[kubernetes.ObjectKind]preprocessorFunc{
	kubernetes.ClusterRolesKind:          preprocessClusterRole,
	kubernetes.RolesKind:                 preprocessRole,
	kubernetes.ServicesKind:              preprocessService,
	kubernetes.APIServiceKind:            preprocessResourceVersion,
	kubernetes.CiliumNetworkPoliciesKind: preprocessResourceVersion,
	kubernetes.PodDisruptionBudgetsKind:  preprocessResourceVersion,
}

// applyPreprocessing applies registered preprocessor for the given kind, if any
func (ds *Store) applyPreprocessing(kind kubernetes.ObjectKind, objStore, objAPIServer client.Object) (client.Object, error) {
	if preprocessor, exists := preprocessorRegistry[kind]; exists {
		return preprocessor(objStore, objAPIServer)
	}
	return objStore, nil
}

// preprocessClusterRole holds preprocessing rules for ClusterRole
// - normalizes policy rules to minimize duplicates
// - ensures deterministic output
func preprocessClusterRole(objStore, objAPIServer client.Object) (client.Object, error) {
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
func preprocessRole(objStore, objAPIServer client.Object) (client.Object, error) {
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
func preprocessService(objStore, objAPIServer client.Object) (client.Object, error) {
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

// preprocessResourceVersion sets the resource version from the API server object if it exists
// Required for APIService, CiliumNetworkPolicies, and PodDisruptionBudgets
func preprocessResourceVersion(objStore, objAPIServer client.Object) (client.Object, error) {
	if objAPIServer != nil {
		objStore.SetResourceVersion(objAPIServer.GetResourceVersion())
	}
	return objStore, nil
}
