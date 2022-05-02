// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rbac

import (
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RoleBindingInfo contains the required information to build a Cluster Role Binding
type RoleBindingInfo struct {
	Name               string
	RoleName           string
	ServiceAccountName string
}

// BuildClusterRoleBinding creates a ClusterRoleBinding object
func BuildClusterRoleBinding(owner metav1.Object, info RoleBindingInfo, agentVersion string) *rbacv1.ClusterRoleBinding {
	return &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Labels: object.GetDefaultLabels(owner, object.NewPartOfLabelValue(owner).String(), agentVersion),
			Name:   info.Name,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbac.RbacAPIGroup,
			Kind:     rbac.ClusterRoleKind,
			Name:     info.RoleName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbac.ServiceAccountKind,
				Name:      info.ServiceAccountName,
				Namespace: owner.GetNamespace(),
			},
		},
	}
}
