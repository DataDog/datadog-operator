// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/defaults"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
)

func TestJemallocDisableReconcilesDaemonSet(t *testing.T) {
	for _, tt := range []struct {
		name     string
		removed  bool
		override bool
	}{
		{name: "disabled"},
		{name: "removed", removed: true},
		{name: "disabled preserves overrides", override: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			r := newRolloutTestReconciler(false)
			ddai := newRolloutTestDDAI()
			ddai.Spec.Global = &v2alpha1.GlobalConfig{Credentials: &v2alpha1.DatadogCredentials{
				APISecret: &v2alpha1.SecretConfig{SecretName: "datadog-secret", KeyName: "api-key"},
			}}
			ddai.Annotations = map[string]string{component.EnableJemallocAnnotation: "true"}
			if tt.override {
				ddai.Spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.NodeAgentComponentName: {
						Env: []corev1.EnvVar{
							{Name: "LD_PRELOAD", Value: "/custom/allocator.so"},
							{Name: "MALLOC_CONF", Value: "dirty_decay_ms:1000"},
						},
					},
				}
			}
			defaults.DefaultDatadogAgentSpec(&ddai.Spec)
			reconcile := func() *appsv1.DaemonSet {
				configured, enabled, required, _ := feature.BuildFeatures(ddai, &ddai.Spec, nil, &feature.Options{Logger: logr.Discard()})
				_, managers := r.setupDependencies(ctx, ddai)
				status := &v1alpha1.DatadogAgentInternalStatus{}
				_, err := r.reconcileV2Agent(ctx, required, append(configured, enabled...), ddai, managers, status, "", make(map[string][]string))
				require.NoError(t, err)
				list := &appsv1.DaemonSetList{}
				require.NoError(t, r.client.List(ctx, list, client.InNamespace(ddai.Namespace)))
				require.Len(t, list.Items, 1)
				return &list.Items[0]
			}
			before := reconcile()
			for _, c := range before.Spec.Template.Spec.Containers {
				require.Len(t, allocatorEnv(c.Env), 2, c.Name)
			}
			if tt.removed {
				delete(ddai.Annotations, component.EnableJemallocAnnotation)
			} else {
				ddai.Annotations[component.EnableJemallocAnnotation] = "false"
			}
			after := reconcile()
			for _, c := range after.Spec.Template.Spec.Containers {
				if tt.override {
					assert.Equal(t, ddai.Spec.Override[v2alpha1.NodeAgentComponentName].Env, allocatorEnv(c.Env), c.Name)
				} else {
					assert.Empty(t, allocatorEnv(c.Env), c.Name)
				}
			}
			if !tt.override {
				assert.NotEqual(t, before.ResourceVersion, after.ResourceVersion, "disabling jemalloc must update the existing workload")
			}
		})
	}
}

func allocatorEnv(env []corev1.EnvVar) []corev1.EnvVar {
	var result []corev1.EnvVar
	for _, e := range env {
		if e.Name == "LD_PRELOAD" || e.Name == "MALLOC_CONF" {
			result = append(result, e)
		}
	}
	return result
}
