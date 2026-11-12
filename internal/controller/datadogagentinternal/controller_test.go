// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"reflect"
	"slices"
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestDefaultDataPlaneEnabled(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		ddai    *v1alpha1.DatadogAgentInternal
		want    bool
	}{
		{
			name:    "ordinary DDAI uses enabled runtime option",
			enabled: true,
			ddai:    &v1alpha1.DatadogAgentInternal{},
			want:    true,
		},
		{
			name:    "ordinary DDAI uses disabled runtime option",
			enabled: false,
			ddai:    &v1alpha1.DatadogAgentInternal{},
			want:    false,
		},
		{
			name:    "Windows profile disables the default",
			enabled: true,
			ddai: &v1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{constants.ProfileLabelKey: "windows"},
				Annotations: map[string]string{
					kubernetes.ProviderAnnotationKey: kubernetes.WindowsProvider,
				},
			}},
			want: false,
		},
		{
			name:    "unprofiled DDAI ignores Windows annotation",
			enabled: true,
			ddai: &v1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					kubernetes.ProviderAnnotationKey: kubernetes.WindowsProvider,
				},
			}},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{options: ReconcilerOptions{DefaultDataPlaneLinuxEnabled: tt.enabled}}
			assert.Equal(t, tt.want, r.reconcilerOptionsToFeatureOptions(context.Background(), tt.ddai).DefaultDataPlaneEnabled)
		})
	}
}

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

		if slices.Contains(policyRule.Resources, "watermarkpodautoscalers") {
			resourceFound = true
		}
		if slices.Contains(policyRule.APIGroups, "datadoghq.com") {
			groupFound = true
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
