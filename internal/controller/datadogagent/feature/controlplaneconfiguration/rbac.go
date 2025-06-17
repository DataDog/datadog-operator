// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package controlplaneconfiguration

import (
	"fmt"

	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/pkg/constants"
	securityv1 "github.com/openshift/api/security/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func getSecurityContextConstraints(ddaName string) *securityv1.SecurityContextConstraints {
	securityContextConstraints := &securityv1.SecurityContextConstraints{
		ObjectMeta: metav1.ObjectMeta{
			Name: securityContextConstraintsName,
			Labels: map[string]string{
				"app.kubernetes.io/name":     "datadog-agent",
				"app.kubernetes.io/instance": ddaName,
			},
		},
		AllowPrivilegedContainer: false,
		AllowHostDirVolumePlugin: true,
		AllowHostIPC:             false,
		AllowHostNetwork:         false,
		AllowHostPID:             false,
		AllowHostPorts:           false,
		AllowPrivilegeEscalation: apiutils.NewBoolPointer(true),
		AllowedCapabilities: []corev1.Capability{
			"SYS_ADMIN",
			"SYS_RESOURCE",
			"SYS_PTRACE",
			"NET_ADMIN",
			"NET_BROADCAST",
			"NET_RAW",
			"IPC_LOCK",
			"CHOWN",
			"DAC_READ_SEARCH",
		},
		RunAsUser: securityv1.RunAsUserStrategyOptions{
			Type: securityv1.RunAsUserStrategyRunAsAny,
		},
		SELinuxContext: securityv1.SELinuxContextStrategyOptions{
			Type: securityv1.SELinuxStrategyRunAsAny,
		},
		FSGroup: securityv1.FSGroupStrategyOptions{
			Type: securityv1.FSGroupStrategyRunAsAny,
		},
		SupplementalGroups: securityv1.SupplementalGroupsStrategyOptions{
			Type: securityv1.SupplementalGroupsStrategyRunAsAny,
		},
		Volumes: []securityv1.FSType{
			securityv1.FSTypeConfigMap,
			securityv1.FSTypeEmptyDir,
			securityv1.FSTypeDownwardAPI,
			securityv1.FSTypeSecret,
			securityv1.FSTypeHostPath,
		},
	}
	return securityContextConstraints
}

func getRoleBinding(securityContextConstraintsName string, ddaName string, ddaNamespace string) *rbacv1.RoleBinding {
	roleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      roleBindingName,
			Namespace: ddaNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":     "datadog-agent",
				"app.kubernetes.io/instance": ddaName,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "security.openshift.io",
			Kind:     "SecurityContextConstraints",
			Name:     securityContextConstraintsName,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      fmt.Sprintf("%s-%s", ddaName, constants.DefaultAgentResourceSuffix),
				Namespace: ddaNamespace,
			},
		},
	}
	return roleBinding
}
