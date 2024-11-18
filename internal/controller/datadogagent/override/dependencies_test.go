// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
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
