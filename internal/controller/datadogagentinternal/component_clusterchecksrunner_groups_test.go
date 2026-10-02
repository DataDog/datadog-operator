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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	componentccr "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusterchecksrunner"
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

func newTestDDAI(name string, groups []datadoghqv2alpha1.ClusterChecksRunnerGroup, kubeKnob bool) *datadoghqv1alpha1.DatadogAgentInternal {
	annotations := map[string]string{}
	if len(groups) > 0 {
		raw, err := json.Marshal(groups)
		if err != nil {
			panic(err)
		}
		annotations[datadoghqv2alpha1.AnnotationExperimentalClusterChecksRunnerGroups] = string(raw)
	}
	if kubeKnob {
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
				Credentials: &datadoghqv2alpha1.DatadogCredentials{APIKey: new("test-api-key")},
			},
			Features: &datadoghqv2alpha1.DatadogFeatures{
				ClusterChecks: &datadoghqv2alpha1.ClusterChecksFeatureConfig{Enabled: new(true)},
			},
		},
	}
	defaults.DefaultDatadogAgentSpec(&ddai.Spec)
	return ddai
}

// reconcileGroups runs ReconcileClusterChecksRunnerGroups on a fresh fake
// client and returns the resulting Deployments keyed by name.
func reconcileGroups(t *testing.T, ddai *datadoghqv1alpha1.DatadogAgentInternal, clusterAgentEnabled bool) map[string]appsv1.Deployment {
	t.Helper()
	sch := runtime.NewScheme()
	_ = scheme.AddToScheme(sch)
	_ = datadoghqv1alpha1.AddToScheme(sch)
	ctx := context.Background()

	r := newTestReconciler(sch)
	_, resourceManagers := r.setupDependencies(ctx, ddai)
	params := &ReconcileComponentParams{
		DDAI: ddai,
		RequiredComponents: feature.RequiredComponents{
			ClusterAgent: feature.RequiredComponent{IsRequired: new(clusterAgentEnabled)},
		},
		ResourceManagers: resourceManagers,
		Status:           &datadoghqv1alpha1.DatadogAgentInternalStatus{},
	}

	result, err := r.ReconcileClusterChecksRunnerGroups(ctx, params)
	require.NoError(t, err)
	assert.True(t, result.IsZero())

	deploymentList := &appsv1.DeploymentList{}
	require.NoError(t, r.client.List(ctx, deploymentList))
	deployments := make(map[string]appsv1.Deployment, len(deploymentList.Items))
	for _, d := range deploymentList.Items {
		deployments[d.Name] = d
	}
	return deployments
}

// runnerContainer returns the runner container of a Deployment.
func runnerContainer(t *testing.T, deployment appsv1.Deployment) corev1.Container {
	t.Helper()
	for _, container := range deployment.Spec.Template.Spec.Containers {
		if container.Name == string(apicommon.ClusterChecksRunnersContainerName) {
			return container
		}
	}
	t.Fatalf("no runner container in Deployment %s", deployment.Name)
	return corev1.Container{}
}

// runnerEnv returns the value of the named env var on the runner container.
func runnerEnv(t *testing.T, deployment appsv1.Deployment, name string) (string, bool) {
	t.Helper()
	for _, env := range runnerContainer(t, deployment).Env {
		if env.Name == name {
			return env.Value, true
		}
	}
	return "", false
}

func TestReconcileClusterChecksRunnerGroups(t *testing.T) {
	type wantGroup struct {
		replicas *int32
	}
	ksm := []datadoghqv2alpha1.ClusterChecksRunnerGroup{{Name: "ksm", ChecksInclude: []string{"kubernetes_state_core"}}}

	tests := []struct {
		name         string
		groups       []datadoghqv2alpha1.ClusterChecksRunnerGroup
		kubeKnob     bool
		useRunners   bool
		clusterAgent bool
		want         map[string]wantGroup
	}{
		{
			name:         "runners on, user group",
			groups:       ksm,
			useRunners:   true,
			clusterAgent: true,
			want:         map[string]wantGroup{"foo-cluster-checks-runner-ksm": {}},
		},
		{
			name:         "runners off, user group is still materialized",
			groups:       ksm,
			clusterAgent: true,
			want:         map[string]wantGroup{"foo-cluster-checks-runner-ksm": {}},
		},
		{
			name:         "no cluster agent: nothing materialized",
			groups:       ksm,
			useRunners:   true,
			clusterAgent: false,
			want:         map[string]wantGroup{},
		},
		{
			name:         "mixed mode: built-in kube group with 2 replicas",
			kubeKnob:     true,
			clusterAgent: true,
			want:         map[string]wantGroup{"foo-cluster-checks-runner-kube": {replicas: new(int32(2))}},
		},
		{
			name: "user kube group replaces the built-in",
			groups: []datadoghqv2alpha1.ClusterChecksRunnerGroup{
				{Name: "kube", ChecksInclude: []string{"kubernetes_state_core"}, Override: &datadoghqv2alpha1.DatadogAgentComponentOverride{Replicas: new(int32(5))}},
			},
			kubeKnob:     true,
			clusterAgent: true,
			want:         map[string]wantGroup{"foo-cluster-checks-runner-kube": {replicas: new(int32(5))}},
		},
		{
			name:         "knob and runners on: kube group alongside user groups",
			groups:       []datadoghqv2alpha1.ClusterChecksRunnerGroup{{Name: "kafka", ChecksInclude: []string{"kafka_consumer"}}},
			kubeKnob:     true,
			useRunners:   true,
			clusterAgent: true,
			want: map[string]wantGroup{
				"foo-cluster-checks-runner-kube":  {replicas: new(int32(2))},
				"foo-cluster-checks-runner-kafka": {},
			},
		},
		{
			name:         "overlapping claims: groups are ignored",
			groups:       []datadoghqv2alpha1.ClusterChecksRunnerGroup{{Name: "ksm", ChecksInclude: []string{"kubernetes_state_core"}}},
			kubeKnob:     true,
			clusterAgent: true,
			want:         map[string]wantGroup{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ddai := newTestDDAI("foo", tt.groups, tt.kubeKnob)
			ddai.Spec.Features.ClusterChecks.UseClusterChecksRunners = new(tt.useRunners)

			deployments := reconcileGroups(t, ddai, tt.clusterAgent)
			require.Len(t, deployments, len(tt.want))
			for name, want := range tt.want {
				deployment, found := deployments[name]
				require.True(t, found, "missing Deployment %s", name)

				groupName := strings.TrimPrefix(name, "foo-cluster-checks-runner-")
				assert.Equal(t, groupName, deployment.Labels[componentccr.ClusterChecksRunnerGroupLabelKey])
				assert.Equal(t, groupName, deployment.Spec.Selector.MatchLabels[componentccr.ClusterChecksRunnerGroupLabelKey])
				if want.replicas != nil {
					require.NotNil(t, deployment.Spec.Replicas)
					assert.Equal(t, *want.replicas, *deployment.Spec.Replicas)
				}

				// The pod only declares its group; the Cluster Agent owns the claims.
				group, _ := runnerEnv(t, deployment, clusterchecksfeature.DDCLCRunnerGroup)
				assert.Equal(t, groupName, group)
			}
		})
	}
}

func TestCleanupOrphanedClusterChecksRunnerGroups(t *testing.T) {
	sch := runtime.NewScheme()
	_ = scheme.AddToScheme(sch)
	_ = datadoghqv1alpha1.AddToScheme(sch)
	ctx := context.Background()

	ddai := newTestDDAI("foo", nil, false)

	existingKept := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo-cluster-checks-runner-ksm",
			Namespace: "ns-1",
			Labels: map[string]string{
				apicommon.AgentDeploymentComponentLabelKey:    constants.DefaultClusterChecksRunnerResourceSuffix,
				kubernetes.AppKubernetesManageByLabelKey:      "datadog-operator",
				kubernetes.AppKubernetesPartOfLabelKey:        object.NewPartOfLabelValue(ddai).String(),
				componentccr.ClusterChecksRunnerGroupLabelKey: "ksm",
			},
		},
	}
	existingOrphaned := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo-cluster-checks-runner-removed",
			Namespace: "ns-1",
			Labels: map[string]string{
				apicommon.AgentDeploymentComponentLabelKey:    constants.DefaultClusterChecksRunnerResourceSuffix,
				kubernetes.AppKubernetesManageByLabelKey:      "datadog-operator",
				kubernetes.AppKubernetesPartOfLabelKey:        object.NewPartOfLabelValue(ddai).String(),
				componentccr.ClusterChecksRunnerGroupLabelKey: "removed",
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

func TestReconcileClusterChecksRunnerGroups_ComponentOverrideApplied(t *testing.T) {
	// The component-level override (spec.override.clusterChecksRunner, e.g.
	// the image) must reach group Deployments, not only the default CCR.
	ddai := newTestDDAI("foo", nil, true)
	ddai.Spec.Override = map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{
		datadoghqv2alpha1.ClusterChecksRunnerComponentName: {
			Image: &datadoghqv2alpha1.AgentImageConfig{
				Name: "registry.datadoghq.com/datadog-agent",
				Tag:  "component-override-tag",
			},
		},
	}

	deployments := reconcileGroups(t, ddai, true)
	deployment, found := deployments["foo-cluster-checks-runner-kube"]
	require.True(t, found)

	runnerImage := runnerContainer(t, deployment).Image
	assert.Contains(t, runnerImage, "registry.datadoghq.com/datadog-agent")
	assert.Contains(t, runnerImage, "component-override-tag")
}
