// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/defaults"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	clusterchecksfeature "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/clusterchecks"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func newTestReconciler(sch *runtime.Scheme, objects ...client.Object) *Reconciler {
	fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(objects...).Build()
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "component_clusterchecksrunner_groups_test"})
	return &Reconciler{
		client:   fakeClient,
		recorder: recorder,
		scheme:   sch,
	}
}

func newTestDDAI(name string, runners []datadoghqv2alpha1.ClusterChecksRunnerGroup) *datadoghqv1alpha1.DatadogAgentInternal {
	annotations := map[string]string{}
	if len(runners) > 0 {
		raw, err := json.Marshal(runners)
		if err != nil {
			panic(err)
		}
		annotations[datadoghqv2alpha1.AnnotationExperimentalClusterChecksRunnerGroups] = string(raw)
	}

	ddai := &datadoghqv1alpha1.DatadogAgentInternal{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogAgentInternal",
			APIVersion: "datadoghq.com/v1alpha1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   "ns-1",
			Annotations: annotations,
		},
		Spec: datadoghqv2alpha1.DatadogAgentSpec{
			Global: &datadoghqv2alpha1.GlobalConfig{
				Credentials: &datadoghqv2alpha1.DatadogCredentials{
					APIKey: ptr.To("test-api-key"),
				},
			},
			Features: &datadoghqv2alpha1.DatadogFeatures{
				ClusterChecks: &datadoghqv2alpha1.ClusterChecksFeatureConfig{
					Enabled: ptr.To(true),
				},
			},
		},
	}
	defaults.DefaultDatadogAgentSpec(&ddai.Spec)
	return ddai
}

func TestApplyClusterChecksRunnerGroupCompatibility(t *testing.T) {
	tests := []struct {
		name  string
		group datadoghqv2alpha1.ClusterChecksRunnerGroup
		want  []corev1.EnvVar
	}{
		{
			name:  "no include or exclude",
			group: datadoghqv2alpha1.ClusterChecksRunnerGroup{Name: "default"},
			want: []corev1.EnvVar{
				{Name: clusterchecksfeature.DDCLCRunnerChecksExclude, Value: ""},
			},
		},
		{
			name:  "include only",
			group: datadoghqv2alpha1.ClusterChecksRunnerGroup{Name: "ksm", ChecksInclude: []string{"kubernetes_state_core"}},
			want: []corev1.EnvVar{
				{Name: clusterchecksfeature.DDCLCRunnerChecksInclude, Value: "kubernetes_state_core"},
				{Name: clusterchecksfeature.DDCLCRunnerChecksExclude, Value: ""},
			},
		},
		{
			name:  "include and exclude",
			group: datadoghqv2alpha1.ClusterChecksRunnerGroup{Name: "ksm", ChecksInclude: []string{"kubernetes_state_core"}, ChecksExclude: []string{"http_check"}},
			want: []corev1.EnvVar{
				{Name: clusterchecksfeature.DDCLCRunnerChecksInclude, Value: "kubernetes_state_core"},
				{Name: clusterchecksfeature.DDCLCRunnerChecksExclude, Value: "http_check"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			podTemplate := &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: string(apicommon.ClusterChecksRunnersContainerName)},
					},
				},
			}
			podManagers := feature.NewPodTemplateManagers(podTemplate)

			applyClusterChecksRunnerGroupCompatibility(podManagers, tt.group)

			container := podManagers.PodTemplateSpec().Spec.Containers[0]
			assert.Equal(t, tt.want, container.Env)
		})
	}
}

func TestReconcileClusterChecksRunnerGroups(t *testing.T) {
	sch := runtime.NewScheme()
	_ = scheme.AddToScheme(sch)
	_ = datadoghqv1alpha1.AddToScheme(sch)
	ctx := context.Background()

	ddai := newTestDDAI("foo", []datadoghqv2alpha1.ClusterChecksRunnerGroup{
		{Name: "ksm", ChecksInclude: []string{"kubernetes_state_core"}},
	})

	r := newTestReconciler(sch)
	_, resourceManagers := r.setupDependencies(ctx, ddai)

	params := &ReconcileComponentParams{
		DDAI: ddai,
		RequiredComponents: feature.RequiredComponents{
			ClusterAgent: feature.RequiredComponent{IsRequired: ptr.To(true)},
		},
		ResourceManagers: resourceManagers,
		Status:           &datadoghqv1alpha1.DatadogAgentInternalStatus{},
	}

	result, err := r.ReconcileClusterChecksRunnerGroups(ctx, params)
	require.NoError(t, err)
	assert.True(t, result.IsZero())

	deploymentList := &appsv1.DeploymentList{}
	require.NoError(t, r.client.List(ctx, deploymentList))
	require.Len(t, deploymentList.Items, 1)

	deployment := deploymentList.Items[0]
	assert.Equal(t, "ksm", deployment.Labels[clusterChecksRunnerGroupLabelKey])

	found := false
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name != string(apicommon.ClusterChecksRunnersContainerName) {
			continue
		}
		for _, env := range container.Env {
			if env.Name == clusterchecksfeature.DDCLCRunnerChecksInclude {
				assert.Equal(t, "kubernetes_state_core", env.Value)
				found = true
			}
		}
	}
	assert.True(t, found, "expected DD_CLC_RUNNER_CHECKS_INCLUDE env var on the runner container")
}

func TestReconcileClusterChecksRunnerGroups_NoClusterAgent(t *testing.T) {
	sch := runtime.NewScheme()
	_ = scheme.AddToScheme(sch)
	_ = datadoghqv1alpha1.AddToScheme(sch)
	ctx := context.Background()

	ddai := newTestDDAI("foo", []datadoghqv2alpha1.ClusterChecksRunnerGroup{
		{Name: "ksm", ChecksInclude: []string{"kubernetes_state_core"}},
	})

	r := newTestReconciler(sch)

	params := &ReconcileComponentParams{
		DDAI:               ddai,
		RequiredComponents: feature.RequiredComponents{},
		Status:             &datadoghqv1alpha1.DatadogAgentInternalStatus{},
	}

	_, err := r.ReconcileClusterChecksRunnerGroups(ctx, params)
	require.NoError(t, err)

	deploymentList := &appsv1.DeploymentList{}
	require.NoError(t, r.client.List(ctx, deploymentList))
	assert.Len(t, deploymentList.Items, 0)
}

func TestCleanupOrphanedClusterChecksRunnerGroups(t *testing.T) {
	sch := runtime.NewScheme()
	_ = scheme.AddToScheme(sch)
	_ = datadoghqv1alpha1.AddToScheme(sch)
	ctx := context.Background()

	ddai := newTestDDAI("foo", nil)

	existingKept := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo-cluster-checks-runner-ksm",
			Namespace: "ns-1",
			Labels: map[string]string{
				apicommon.AgentDeploymentComponentLabelKey: constants.DefaultClusterChecksRunnerResourceSuffix,
				kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
				kubernetes.AppKubernetesPartOfLabelKey:     object.NewPartOfLabelValue(ddai).String(),
				clusterChecksRunnerGroupLabelKey:           "ksm",
			},
		},
	}
	existingOrphaned := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo-cluster-checks-runner-removed",
			Namespace: "ns-1",
			Labels: map[string]string{
				apicommon.AgentDeploymentComponentLabelKey: constants.DefaultClusterChecksRunnerResourceSuffix,
				kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
				kubernetes.AppKubernetesPartOfLabelKey:     object.NewPartOfLabelValue(ddai).String(),
				clusterChecksRunnerGroupLabelKey:           "removed",
			},
		},
	}
	defaultDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo-cluster-checks-runner",
			Namespace: "ns-1",
			Labels: map[string]string{
				apicommon.AgentDeploymentComponentLabelKey: constants.DefaultClusterChecksRunnerResourceSuffix,
				kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
				kubernetes.AppKubernetesPartOfLabelKey:     object.NewPartOfLabelValue(ddai).String(),
			},
		},
	}

	r := newTestReconciler(sch, existingKept, existingOrphaned, defaultDeployment)

	_, err := r.cleanupOrphanedClusterChecksRunnerGroups(ctx, ddai, []datadoghqv2alpha1.ClusterChecksRunnerGroup{
		{Name: "ksm"},
	})
	require.NoError(t, err)

	deploymentList := &appsv1.DeploymentList{}
	require.NoError(t, r.client.List(ctx, deploymentList))

	var names []string
	for _, d := range deploymentList.Items {
		names = append(names, d.Name)
	}
	assert.ElementsMatch(t, []string{"foo-cluster-checks-runner-ksm", "foo-cluster-checks-runner"}, names)
}
