// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"encoding/json"
	"strings"
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
	return newTestDDAIWithKubeKnob(name, runners, false)
}

// newTestDDAIWithKubeKnob is newTestDDAI with the experimental
// kube-checks-runner-default annotation set to "true" when knob is set.
func newTestDDAIWithKubeKnob(name string, runners []datadoghqv2alpha1.ClusterChecksRunnerGroup, knob bool) *datadoghqv1alpha1.DatadogAgentInternal {
	annotations := map[string]string{}
	if len(runners) > 0 {
		raw, err := json.Marshal(runners)
		if err != nil {
			panic(err)
		}
		annotations[datadoghqv2alpha1.AnnotationExperimentalClusterChecksRunnerGroups] = string(raw)
	}
	if knob {
		annotations[datadoghqv2alpha1.AnnotationExperimentalKubeChecksRunnerDefault] = "true"
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
	// Groups materialize when a CCR-family Deployment is active: enable the
	// default CCR (useClusterChecksRunners), the standard runner-groups
	// configuration. (With both useClusterChecksRunners and the kube knob off,
	// the groups annotation is ignored — see
	// TestReconcileClusterChecksRunnerGroups_IgnoredWithoutRunnersOrKnob.)
	ddai.Spec.Features.ClusterChecks.UseClusterChecksRunners = ptr.To(true)

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

func TestReconcileClusterChecksRunnerGroups_MixedMode(t *testing.T) {
	sch := runtime.NewScheme()
	_ = scheme.AddToScheme(sch)
	_ = datadoghqv1alpha1.AddToScheme(sch)
	ctx := context.Background()

	// Mixed mode: kube-checks-runner-default knob on, useClusterChecksRunners
	// off (the default after DefaultDatadogAgentSpec). The built-in kube group
	// must be materialized even though the default CCR Deployment is not
	// created in this mode.
	ddai := newTestDDAIWithKubeKnob("foo", nil, true)

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
	// Built-in kube group: dedicated Deployment name and group label.
	assert.Equal(t, "foo-cluster-checks-runner-kube", deployment.Name)
	assert.Equal(t, datadoghqv2alpha1.KubeChecksRunnerGroupName, deployment.Labels[clusterChecksRunnerGroupLabelKey])
	// Default replicas 2 from the built-in group's Override.
	require.NotNil(t, deployment.Spec.Replicas)
	assert.Equal(t, int32(2), *deployment.Spec.Replicas)
	// Kube family include env; the union exclude set by the clusterchecks
	// feature for the default CCR is overwritten with the group's own
	// (empty) exclude.
	var includeEnv, excludeEnv string
	var includeFound bool
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name != string(apicommon.ClusterChecksRunnersContainerName) {
			continue
		}
		for _, env := range container.Env {
			switch env.Name {
			case clusterchecksfeature.DDCLCRunnerChecksInclude:
				includeEnv = env.Value
				includeFound = true
			case clusterchecksfeature.DDCLCRunnerChecksExclude:
				excludeEnv = env.Value
			}
		}
	}
	assert.True(t, includeFound, "expected DD_CLC_RUNNER_CHECKS_INCLUDE env var on the runner container")
	assert.Equal(t, strings.Join(datadoghqv2alpha1.KubeChecksRunnerGroupChecksInclude, ","), includeEnv)
	assert.Equal(t, "", excludeEnv)
}

func TestReconcileClusterChecksRunnerGroups_MixedModeUserKubeGroupReplacesBuiltin(t *testing.T) {
	sch := runtime.NewScheme()
	_ = scheme.AddToScheme(sch)
	_ = datadoghqv1alpha1.AddToScheme(sch)
	ctx := context.Background()

	// A user-declared group named "kube" replaces the built-in entirely, so
	// its own include list and replicas override are used.
	ddai := newTestDDAIWithKubeKnob("foo", []datadoghqv2alpha1.ClusterChecksRunnerGroup{
		{Name: "kube", ChecksInclude: []string{"kubernetes_state_core"}, Override: &datadoghqv2alpha1.DatadogAgentComponentOverride{Replicas: ptr.To(int32(5))}},
	}, true)

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

	_, err := r.ReconcileClusterChecksRunnerGroups(ctx, params)
	require.NoError(t, err)

	deploymentList := &appsv1.DeploymentList{}
	require.NoError(t, r.client.List(ctx, deploymentList))
	require.Len(t, deploymentList.Items, 1)

	deployment := deploymentList.Items[0]
	require.NotNil(t, deployment.Spec.Replicas)
	assert.Equal(t, int32(5), *deployment.Spec.Replicas)

	var includeEnv string
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name != string(apicommon.ClusterChecksRunnersContainerName) {
			continue
		}
		for _, env := range container.Env {
			if env.Name == clusterchecksfeature.DDCLCRunnerChecksInclude {
				includeEnv = env.Value
			}
		}
	}
	assert.Equal(t, "kubernetes_state_core", includeEnv)
}

func TestReconcileClusterChecksRunnerGroups_IgnoredWithoutRunnersOrKnob(t *testing.T) {
	sch := runtime.NewScheme()
	_ = scheme.AddToScheme(sch)
	_ = datadoghqv1alpha1.AddToScheme(sch)
	ctx := context.Background()

	// Groups annotation set but neither useClusterChecksRunners nor the kube
	// knob: groups are ignored, nothing is materialized.
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

	_, err := r.ReconcileClusterChecksRunnerGroups(ctx, params)
	require.NoError(t, err)

	deploymentList := &appsv1.DeploymentList{}
	require.NoError(t, r.client.List(ctx, deploymentList))
	assert.Len(t, deploymentList.Items, 0)
}

func TestReconcileClusterChecksRunnerGroups_KnobOnWithRunnersOn(t *testing.T) {
	sch := runtime.NewScheme()
	_ = scheme.AddToScheme(sch)
	_ = datadoghqv1alpha1.AddToScheme(sch)
	ctx := context.Background()

	// Runner-only mode with the knob on: the kube group materializes
	// alongside the user groups (the default CCR Deployment is handled by the
	// component registry, not by this reconcile).
	ddai := newTestDDAIWithKubeKnob("foo", []datadoghqv2alpha1.ClusterChecksRunnerGroup{
		{Name: "kafka", ChecksInclude: []string{"kafka_consumer"}},
	}, true)
	ddai.Spec.Features.ClusterChecks.UseClusterChecksRunners = ptr.To(true)

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

	_, err := r.ReconcileClusterChecksRunnerGroups(ctx, params)
	require.NoError(t, err)

	deploymentList := &appsv1.DeploymentList{}
	require.NoError(t, r.client.List(ctx, deploymentList))
	var names []string
	for _, d := range deploymentList.Items {
		names = append(names, d.Name)
	}
	assert.ElementsMatch(t, []string{"foo-cluster-checks-runner-kube", "foo-cluster-checks-runner-kafka"}, names)
}

func TestReconcileClusterChecksRunnerGroups_ComponentOverrideApplied(t *testing.T) {
	sch := runtime.NewScheme()
	_ = scheme.AddToScheme(sch)
	_ = datadoghqv1alpha1.AddToScheme(sch)
	ctx := context.Background()

	// The component-level override (spec.override.clusterChecksRunner, e.g.
	// the image) must reach group Deployments, not only the default CCR
	// Deployment. The group's own Override is applied after and wins on
	// conflicts.
	ddai := newTestDDAIWithKubeKnob("foo", nil, true)
	ddai.Spec.Override = map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{
		datadoghqv2alpha1.ClusterChecksRunnerComponentName: {
			Image: &datadoghqv2alpha1.AgentImageConfig{
				Name: "registry.datadoghq.com/datadog-agent",
				Tag:  "component-override-tag",
			},
		},
	}

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
	assert.Equal(t, "foo-cluster-checks-runner-kube", deployment.Name)

	// The component-level image override lands on the runner container.
	var runnerImage string
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == string(apicommon.ClusterChecksRunnersContainerName) {
			runnerImage = container.Image
		}
	}
	assert.Contains(t, runnerImage, "registry.datadoghq.com/datadog-agent", "expected the component-level image override on the runner container, got %q", runnerImage)
	assert.Contains(t, runnerImage, "component-override-tag", "expected the component-level image tag override on the runner container, got %q", runnerImage)
}
