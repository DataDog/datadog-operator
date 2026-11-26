// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func TestKubernetesRemediation(t *testing.T) {
	for _, tt := range []struct {
		name        string
		par         string
		annotations map[string]string
		wantEnabled bool
		wantEnv     string
	}{
		{name: "disabled by default", par: "enabled: true"},
		{name: "typed enable", par: "enabled: true\nkubernetesRemediationsEnabled: true", wantEnabled: true, wantEnv: "true"},
		{name: "cluster only", par: "clusterAgent:\n  enabled: true\nkubernetesRemediationsEnabled: true", wantEnabled: true, wantEnv: "true"},
		{name: "disabled cluster PAR", par: "enabled: true\nclusterAgent:\n  enabled: false\nkubernetesRemediationsEnabled: true"},
		{name: "disabled PAR", par: "kubernetesRemediationsEnabled: true"},
		{name: "legacy annotation", annotations: map[string]string{featureutils.EnableClusterAgentPrivateActionRunnerAnnotation: "true", featureutils.ClusterAgentPrivateActionRunnerK8sRemediationEnabled: "true"}, wantEnabled: true, wantEnv: "true"},
		{name: "annotation requires PAR", annotations: map[string]string{featureutils.ClusterAgentPrivateActionRunnerK8sRemediationEnabled: "true"}},
		{name: "annotation fallback with typed PAR", par: "enabled: true", annotations: map[string]string{featureutils.ClusterAgentPrivateActionRunnerK8sRemediationEnabled: "true"}, wantEnabled: true, wantEnv: "true"},
		{name: "typed false overrides annotation", par: "enabled: true\nkubernetesRemediationsEnabled: false", annotations: map[string]string{featureutils.ClusterAgentPrivateActionRunnerK8sRemediationEnabled: "true"}, wantEnv: "false"},
		{name: "typed true overrides annotation", par: "enabled: true\nkubernetesRemediationsEnabled: true", annotations: map[string]string{featureutils.ClusterAgentPrivateActionRunnerK8sRemediationEnabled: "false"}, wantEnabled: true, wantEnv: "true"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dda := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: "datadog", Namespace: "default", Annotations: tt.annotations}}
			if tt.par != "" {
				dda.Spec.Features = &v2alpha1.DatadogFeatures{PrivateActionRunner: &v2alpha1.PrivateActionRunnerFeatureConfig{}}
				require.NoError(t, yaml.UnmarshalStrict([]byte(tt.par), dda.Spec.Features.PrivateActionRunner))
			}
			before := dda.DeepCopy()
			f := buildPrivateActionRunnerFeature(nil).(*privateActionRunnerFeature)
			f.Configure(dda, &dda.Spec, nil)
			require.NoError(t, f.configErr)
			assert.Equal(t, tt.wantEnabled, f.k8sRemediationEnabled)

			scheme := runtime.NewScheme()
			require.NoError(t, corev1.AddToScheme(scheme))
			require.NoError(t, rbacv1.AddToScheme(scheme))
			require.NoError(t, v2alpha1.AddToScheme(scheme))
			resources := feature.NewResourceManagers(store.NewStore(dda, &store.StoreOptions{Scheme: scheme}))
			require.NoError(t, f.ManageDependencies(resources))
			_, roleFound := resources.Store().Get(kubernetes.ClusterRolesKind, "", f.getRbacResourcesName())
			assert.Equal(t, tt.wantEnabled, roleFound)
			binding, bindingFound := resources.Store().Get(kubernetes.ClusterRoleBindingKind, "", f.getRbacResourcesName())
			require.Equal(t, tt.wantEnabled, bindingFound)
			if tt.wantEnabled {
				assert.Equal(t, f.clusterServiceAccountName, binding.(*rbacv1.ClusterRoleBinding).Subjects[0].Name)
			}

			managers := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: string(apicommon.ClusterAgentContainerName)}}}})
			require.NoError(t, f.ManageClusterAgent(managers))
			envVars := managers.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]
			if tt.wantEnv != "" {
				assert.Contains(t, envVars, &corev1.EnvVar{Name: DDPARKubernetesRemediationsEnabled, Value: tt.wantEnv})
			} else {
				for _, env := range envVars {
					assert.NotEqual(t, DDPARKubernetesRemediationsEnabled, env.Name)
				}
			}
			assert.Equal(t, before, dda)
		})
	}
}
