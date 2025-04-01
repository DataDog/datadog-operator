// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

func TestDependencies(t *testing.T) {
	testLogger := logf.Log.WithName("TestRequiredComponents")

	testScheme := runtime.NewScheme()
	testScheme.AddKnownTypes(v2alpha1.GroupVersion, &v2alpha1.DatadogAgent{})
	storeOptions := &store.StoreOptions{
		Scheme: testScheme,
	}

	tests := []struct {
		name          string
		dda           v2alpha1.DatadogAgent
		expectsErrors bool
	}{
		{
			name: "empty override without errors",
			dda: v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {},
					},
				},
			},
			expectsErrors: false,
		},
		{
			name: "override extraConfd configmap without errors",
			dda: v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							ExtraConfd: &v2alpha1.MultiCustomConfig{
								ConfigMap: &v2alpha1.ConfigMapConfig{
									Name: "cmName",
								},
							},
						},
					},
				},
			},
			expectsErrors: false,
		},
		{
			name: "override extraConfd configData without errors",
			dda: v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							ExtraConfd: &v2alpha1.MultiCustomConfig{
								ConfigDataMap: map[string]string{
									"path_to_file.yaml": "yaml: data",
								},
							},
						},
					},
				},
			},
			expectsErrors: false,
		},
		{
			name: "override extraChecksd configmap without errors",
			dda: v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							ExtraChecksd: &v2alpha1.MultiCustomConfig{
								ConfigMap: &v2alpha1.ConfigMapConfig{
									Name: "cmName",
								},
							},
						},
					},
				},
			},
			expectsErrors: false,
		},
		{
			name: "override extraChecksd configData without errors",
			dda: v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							ExtraChecksd: &v2alpha1.MultiCustomConfig{
								ConfigDataMap: map[string]string{
									"path_to_file.py": "print('hello')",
								},
							},
						},
					},
				},
			},
			expectsErrors: false,
		},
		{
			name: "override don't createRbac without errors",
			dda: v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.ClusterAgentComponentName: {
							CreateRbac: apiutils.NewBoolPointer(false),
						},
					},
				},
			},
			expectsErrors: false,
		},
		{
			name: "override clusterAgent createPDB without errors",
			dda: v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.ClusterAgentComponentName: {
							CreatePodDisruptionBudget: apiutils.NewBoolPointer(true),
						},
					},
				},
			},
		},
		{
			name: "override clusterChecksRunner createPDB without errors",
			dda: v2alpha1.DatadogAgent{
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.ClusterChecksRunnerComponentName: {
							CreatePodDisruptionBudget: apiutils.NewBoolPointer(true),
						},
					},
					Features: &v2alpha1.DatadogFeatures{
						ClusterChecks: &v2alpha1.ClusterChecksFeatureConfig{
							UseClusterChecksRunners: apiutils.NewBoolPointer(true),
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := store.NewStore(&test.dda, storeOptions)
			manager := feature.NewResourceManagers(store)

			errs := Dependencies(testLogger, manager, &test.dda)

			if test.expectsErrors {
				assert.NotEmpty(t, errs)
			} else {
				assert.Empty(t, errs)
			}
		})
	}
}

func TestServiceAccountAnnotationOverride(t *testing.T) {
	customServiceAccount := "fake"
	customServiceAccountAnnotations := map[string]string{
		"eks.amazonaws.com/role-arn": "arn:aws:iam::123456789012:role/datadog-role",
		"really.important":           "annotation",
	}
	ddaName := "test-dda"
	tests := []struct {
		name string
		dda  *v2alpha1.DatadogAgent
		want map[v2alpha1.ComponentName]map[string]interface{}
	}{
		{
			name: "custom serviceaccount annotations for dda, dca and clc",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Name: ddaName,
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.ClusterAgentComponentName: {
							ServiceAccountName:        &customServiceAccount,
							ServiceAccountAnnotations: customServiceAccountAnnotations,
						},
						v2alpha1.ClusterChecksRunnerComponentName: {
							ServiceAccountAnnotations: customServiceAccountAnnotations,
						},
						v2alpha1.NodeAgentComponentName: {
							ServiceAccountAnnotations: customServiceAccountAnnotations,
						},
					},
				},
			},
			want: map[v2alpha1.ComponentName]map[string]interface{}{
				v2alpha1.ClusterAgentComponentName: {
					"name":        customServiceAccount,
					"annotations": customServiceAccountAnnotations,
				},
				v2alpha1.NodeAgentComponentName: {
					"name":        fmt.Sprintf("%s-%s", ddaName, constants.DefaultAgentResourceSuffix),
					"annotations": customServiceAccountAnnotations,
				},
				v2alpha1.ClusterChecksRunnerComponentName: {
					"name":        fmt.Sprintf("%s-%s", ddaName, constants.DefaultClusterChecksRunnerResourceSuffix),
					"annotations": customServiceAccountAnnotations,
				},
			},
		},
		{
			name: "custom serviceaccount annotations for dca",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Name: ddaName,
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.ClusterAgentComponentName: {
							ServiceAccountName:        &customServiceAccount,
							ServiceAccountAnnotations: customServiceAccountAnnotations,
						},
					},
				},
			},
			want: map[v2alpha1.ComponentName]map[string]interface{}{
				v2alpha1.NodeAgentComponentName: {
					"name":        fmt.Sprintf("%s-%s", ddaName, constants.DefaultAgentResourceSuffix),
					"annotations": map[string]string{},
				},
				v2alpha1.ClusterAgentComponentName: {
					"name":        customServiceAccount,
					"annotations": customServiceAccountAnnotations,
				},
				v2alpha1.ClusterChecksRunnerComponentName: {
					"name":        fmt.Sprintf("%s-%s", ddaName, constants.DefaultClusterChecksRunnerResourceSuffix),
					"annotations": map[string]string{},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := map[v2alpha1.ComponentName]map[string]interface{}{
				v2alpha1.NodeAgentComponentName: {
					"name":        constants.GetAgentServiceAccount(tt.dda),
					"annotations": getSaAnnotations(tt.dda, v2alpha1.NodeAgentComponentName),
				},
				v2alpha1.ClusterChecksRunnerComponentName: {
					"name":        constants.GetClusterChecksRunnerServiceAccount(tt.dda),
					"annotations": getSaAnnotations(tt.dda, v2alpha1.ClusterChecksRunnerComponentName),
				},
				v2alpha1.ClusterAgentComponentName: {
					"name":        constants.GetClusterAgentServiceAccount(tt.dda),
					"annotations": getSaAnnotations(tt.dda, v2alpha1.ClusterAgentComponentName),
				},
			}
			for componentName, sa := range tt.want {
				if res[componentName]["name"] != sa["name"] {
					t.Errorf("Service Account Override Name error = %v, want %v", res[componentName], tt.want[componentName])
				}
				if !mapsEqual(res[componentName]["annotations"].(map[string]string), sa["annotations"].(map[string]string)) {
					t.Errorf("Service Account Override Annotation error = %v, want %v", res[componentName], tt.want[componentName])
				}
			}
		})
	}
}

func getSaAnnotations(dda *v2alpha1.DatadogAgent, componentName v2alpha1.ComponentName) map[string]string {
	defaultAnnotations := map[string]string{}
	if dda.Spec.Override[componentName] != nil && dda.Spec.Override[componentName].ServiceAccountAnnotations != nil {
		return dda.Spec.Override[componentName].ServiceAccountAnnotations
	}
	return defaultAnnotations
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if bValue, ok := b[key]; !ok || value != bValue {
			return false
		}
	}
	return true
}
