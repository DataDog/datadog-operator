// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"reflect"

	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func containsEnv(slice []corev1.EnvVar, name, value string) bool {
	for _, element := range slice {
		if element.Name == name && element.Value == value {
			return true
		}
	}
	return false
}

func containsVolumeMounts(slice []corev1.VolumeMount, name, path string) bool {
	for _, element := range slice {
		if element.Name == name && element.MountPath == path {
			return true
		}
	}
	return false
}

func hasAllClusterLevelRbacResources(policyRules []rbacv1.PolicyRule) bool {
	clusterLevelResources := map[string]bool{
		"services":              true,
		"events":                true,
		"pods":                  true,
		"nodes":                 true,
		"componentstatuses":     true,
		"clusterresourcequotas": true,
	}
	for _, policyRule := range policyRules {
		for _, resource := range policyRule.Resources {
			delete(clusterLevelResources, resource)
		}
	}
	return len(clusterLevelResources) == 0
}

func hasWpaRbacs(policyRules []rbacv1.PolicyRule) bool {
	requiredVerbs := []string{
		rbac.ListVerb,
		rbac.WatchVerb,
		rbac.GetVerb,
	}

	for _, policyRule := range policyRules {
		resourceFound := false
		groupFound := false
		verbsFound := false

		for _, resource := range policyRule.Resources {
			if resource == "watermarkpodautoscalers" {
				resourceFound = true
				break
			}
		}
		for _, group := range policyRule.APIGroups {
			if group == "datadoghq.com" {
				groupFound = true
				break
			}
		}
		if reflect.DeepEqual(policyRule.Verbs, requiredVerbs) {
			verbsFound = true
		}
		if resourceFound && groupFound && verbsFound {
			return true
		}
	}

	return false
}

func hasAdmissionRbacResources(clusterPolicyRules []rbacv1.PolicyRule, policyRules []rbacv1.PolicyRule) bool {
	clusterLevelResources := map[string]bool{
		"validatingwebhookconfigurations": true,
		"mutatingwebhookconfigurations":   true,
		"replicasets":                     true,
		"deployments":                     true,
		"statefulsets":                    true,
		"cronjobs":                        true,
		"jobs":                            true,
	}
	roleResources := map[string]bool{
		"secrets": true,
	}
	for _, policyRule := range clusterPolicyRules {
		for _, resource := range policyRule.Resources {
			delete(clusterLevelResources, resource)
		}
	}
	for _, policyRule := range policyRules {
		for _, resource := range policyRule.Resources {
			delete(roleResources, resource)
		}
	}
	return len(clusterLevelResources) == 0 && len(roleResources) == 0
}

func hasAllNodeLevelRbacResources(policyRules []rbacv1.PolicyRule) bool {
	nodeLevelResources := map[string]bool{
		"endpoints":     true,
		"nodes/metrics": true,
		"nodes/spec":    true,
		"nodes/proxy":   true,
	}
	for _, policyRule := range policyRules {
		for _, resource := range policyRule.Resources {
			delete(nodeLevelResources, resource)
		}
	}
	return len(nodeLevelResources) == 0
}

// dummyManager mocks the metric forwarder by implementing the metricForwardersManager interface
// the metricForwardersManager logic is tested in the util/datadog package
type dummyManager struct{}

func (dummyManager) Register(client.Object) {
}

func (dummyManager) Unregister(client.Object) {
}

func (dummyManager) ProcessError(client.Object, error) {
}

func (dummyManager) ProcessEvent(client.Object, datadog.Event) {
}

func (dummyManager) MetricsForwarderStatusForObj(obj client.Object) *datadog.ConditionCommon {
	return nil
}

func (dummyManager) SetEnabledFeatures(obj client.Object, features []string) {
}
