// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"encoding/json"
	"errors"
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
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	componentccr "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusterchecksrunner"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/defaults"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	clusterchecksfeature "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/clusterchecks"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/global"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/untaint"
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

func newTestDDAI(name string, groups []componentccr.Group, coreKnob bool) *datadoghqv1alpha1.DatadogAgentInternal {
	annotations := map[string]string{}
	if len(groups) > 0 {
		raw, err := json.Marshal(groups)
		if err != nil {
			panic(err)
		}
		annotations[datadoghqv2alpha1.AnnotationExperimentalClusterChecksRunnerGroups] = string(raw)
	}
	if coreKnob {
		annotations[datadoghqv2alpha1.AnnotationExperimentalCoreChecksRunnerDefault] = "true"
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
			Override: map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{
				datadoghqv2alpha1.ClusterAgentComponentName:        {Image: &datadoghqv2alpha1.AgentImageConfig{Tag: "7.86.0"}},
				datadoghqv2alpha1.ClusterChecksRunnerComponentName: {Image: &datadoghqv2alpha1.AgentImageConfig{Tag: "7.86.0"}},
			},
		},
	}
	defaults.DefaultDatadogAgentSpec(&ddai.Spec)
	return ddai
}

func reconcileGroups(t *testing.T, ddai *datadoghqv1alpha1.DatadogAgentInternal, clusterAgentEnabled bool, opts ...ReconcilerOptions) map[string]appsv1.Deployment {
	t.Helper()
	r := newTestReconciler(testScheme())
	if len(opts) > 0 {
		r.options = opts[0]
	}
	require.NoError(t, reconcileGroupsWith(r, ddai, clusterAgentEnabled))
	return listDeployments(t, r)
}

func testScheme() *runtime.Scheme {
	sch := runtime.NewScheme()
	_ = scheme.AddToScheme(sch)
	_ = datadoghqv1alpha1.AddToScheme(sch)
	return sch
}

func reconcileGroupsWith(r *Reconciler, ddai *datadoghqv1alpha1.DatadogAgentInternal, clusterAgentEnabled bool) error {
	ctx := context.Background()
	_, resourceManagers := r.setupDependencies(ctx, ddai)
	params := &ReconcileComponentParams{
		DDAI: ddai,
		RequiredComponents: feature.RequiredComponents{
			ClusterAgent: feature.RequiredComponent{IsRequired: new(clusterAgentEnabled)},
		},
		ResourceManagers: resourceManagers,
		Status:           &datadoghqv1alpha1.DatadogAgentInternalStatus{},
	}
	_, err := r.ReconcileClusterChecksRunnerGroups(ctx, params)
	return err
}

func listDeployments(t *testing.T, r *Reconciler) map[string]appsv1.Deployment {
	t.Helper()
	deploymentList := &appsv1.DeploymentList{}
	require.NoError(t, r.client.List(context.Background(), deploymentList))
	deployments := make(map[string]appsv1.Deployment, len(deploymentList.Items))
	for _, d := range deploymentList.Items {
		deployments[d.Name] = d
	}
	return deployments
}

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
	ksm := []componentccr.Group{{Name: "ksm", Checks: []string{"kubernetes_state_core"}}}

	tests := []struct {
		name         string
		groups       []componentccr.Group
		coreKnob     bool
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
			name:         "mixed mode: built-in core group with 2 replicas",
			coreKnob:     true,
			clusterAgent: true,
			want:         map[string]wantGroup{"foo-cluster-checks-runner-core": {replicas: new(int32(2))}},
		},
		{
			name: "a core entry sets the built-in group's override",
			groups: []componentccr.Group{
				{Name: "core", Checks: []string{"kubernetes_state_core"}, Override: &datadoghqv2alpha1.DatadogAgentComponentOverride{Replicas: new(int32(5))}},
			},
			coreKnob:     true,
			clusterAgent: true,
			want:         map[string]wantGroup{"foo-cluster-checks-runner-core": {replicas: new(int32(5))}},
		},
		{
			name:         "knob and runners on: core group alongside user groups",
			groups:       []componentccr.Group{{Name: "kafka", Checks: []string{"kafka_consumer"}}},
			coreKnob:     true,
			useRunners:   true,
			clusterAgent: true,
			want: map[string]wantGroup{
				"foo-cluster-checks-runner-core":  {replicas: new(int32(2))},
				"foo-cluster-checks-runner-kafka": {},
			},
		},
		{
			name:         "a user group claiming a core check takes it from core",
			groups:       []componentccr.Group{{Name: "ksm", Checks: []string{"kubernetes_state_core"}}},
			coreKnob:     true,
			clusterAgent: true,
			want: map[string]wantGroup{
				"foo-cluster-checks-runner-core": {replicas: new(int32(2))},
				"foo-cluster-checks-runner-ksm":  {},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ddai := newTestDDAI("foo", tt.groups, tt.coreKnob)
			ddai.Spec.Features.ClusterChecks.UseClusterChecksRunners = new(tt.useRunners)

			deployments := reconcileGroups(t, ddai, tt.clusterAgent)
			require.Len(t, deployments, len(tt.want))
			for name, want := range tt.want {
				deployment, found := deployments[name]
				require.True(t, found, "missing Deployment %s", name)

				groupName := strings.TrimPrefix(name, "foo-cluster-checks-runner-")
				assert.Equal(t, groupName, deployment.Labels[componentccr.ClusterChecksRunnerGroupLabelKey])
				assert.Equal(t, groupName, deployment.Spec.Selector.MatchLabels[componentccr.ClusterChecksRunnerGroupLabelKey])
				assert.Equal(t, want.replicas, deployment.Spec.Replicas)

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

	err := r.cleanupOrphanedClusterChecksRunnerGroups(ctx, ddai, []componentccr.Group{
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
	ddai := newTestDDAI("foo", nil, true)
	ddai.Spec.Override[datadoghqv2alpha1.ClusterChecksRunnerComponentName] = &datadoghqv2alpha1.DatadogAgentComponentOverride{
		Image: &datadoghqv2alpha1.AgentImageConfig{
			Name: "registry.datadoghq.com/datadog-agent",
			Tag:  "component-override-tag",
		},
	}

	deployments := reconcileGroups(t, ddai, true)
	deployment, found := deployments["foo-cluster-checks-runner-core"]
	require.True(t, found)

	runnerImage := runnerContainer(t, deployment).Image
	assert.Contains(t, runnerImage, "registry.datadoghq.com/datadog-agent")
	assert.Contains(t, runnerImage, "component-override-tag")
}

func TestClusterChecksRunnerGroupsAgentNotReadyToleration(t *testing.T) {
	groups := []componentccr.Group{{Name: "ksm", Checks: []string{"kubernetes_state_core"}}}
	name := componentccr.GetClusterChecksRunnerGroupName(newTestDDAI("foo", groups, false), "ksm")

	deployments := reconcileGroups(t, newTestDDAI("foo", groups, false), true, ReconcilerOptions{UntaintControllerEnabled: true})
	assert.Contains(t, deployments[name].Spec.Template.Spec.Tolerations, untaint.AgentNotReadyEqualToleration())

	deployments = reconcileGroups(t, newTestDDAI("foo", groups, false), true)
	assert.NotContains(t, deployments[name].Spec.Template.Spec.Tolerations, untaint.AgentNotReadyEqualToleration())
}

func TestReconcileClusterChecksRunnerGroupsRequiresAgent786(t *testing.T) {
	ddai := newTestDDAI("foo", nil, true)
	ddai.Spec.Override[datadoghqv2alpha1.ClusterAgentComponentName].Image.Tag = "7.85.0"
	assert.Empty(t, reconcileGroups(t, ddai, true))

	ddai = newTestDDAI("foo", nil, true)
	ddai.Spec.Override[datadoghqv2alpha1.ClusterChecksRunnerComponentName].Image.Tag = "7.85.0"
	assert.Empty(t, reconcileGroups(t, ddai, true))
}

func TestReconcileClusterChecksRunnerGroupsDoNotInheritNameOrReplicas(t *testing.T) {
	groups := []componentccr.Group{
		{Name: "http", Checks: []string{"http_check"}},
		{Name: "redis", Checks: []string{"redisdb"}, Override: &datadoghqv2alpha1.DatadogAgentComponentOverride{Name: new("renamed"), Replicas: new(int32(3))}},
	}
	ddai := newTestDDAI("foo", groups, false)
	runnerOverride := ddai.Spec.Override[datadoghqv2alpha1.ClusterChecksRunnerComponentName]
	runnerOverride.Name = new("custom-runners")
	runnerOverride.Replicas = new(int32(6))

	deployments := reconcileGroups(t, ddai, true)
	require.Len(t, deployments, 2)
	require.Contains(t, deployments, "foo-cluster-checks-runner-http")
	require.Contains(t, deployments, "foo-cluster-checks-runner-redis")
	assert.Nil(t, deployments["foo-cluster-checks-runner-http"].Spec.Replicas)
	assert.Equal(t, int32(3), *deployments["foo-cluster-checks-runner-redis"].Spec.Replicas)
	assert.Equal(t, int32(6), *runnerOverride.Replicas, "the DDAI's own override must not be mutated")
}

func TestReconcileClusterChecksRunnerGroupsDisabledRunners(t *testing.T) {
	groups := []componentccr.Group{{Name: "http", Checks: []string{"http_check"}}}
	ddai := newTestDDAI("foo", groups, true)
	ddai.Spec.Override[datadoghqv2alpha1.ClusterChecksRunnerComponentName].Disabled = new(true)

	assert.Empty(t, reconcileGroups(t, ddai, true))
}

func TestReconcileClusterChecksRunnerGroupsSkipInvalidNames(t *testing.T) {
	groups := []componentccr.Group{
		{Name: "HTTP_checks", Checks: []string{"http_check"}},
		{Name: "redis", Checks: []string{"redisdb"}},
	}
	deployments := reconcileGroups(t, newTestDDAI("foo", groups, false), true)
	require.Len(t, deployments, 1)
	assert.Contains(t, deployments, "foo-cluster-checks-runner-redis")
}

func TestReconcileClusterChecksRunnerGroupsDCATokenChecksum(t *testing.T) {
	groups := []componentccr.Group{{Name: "http", Checks: []string{"http_check"}}}
	ddai := newTestDDAI("foo", groups, false)
	ddai.Spec.Override[datadoghqv2alpha1.ClusterAgentComponentName].Annotations = map[string]string{
		global.GetDCATokenChecksumAnnotationKey(): "abc123",
	}

	deployments := reconcileGroups(t, ddai, true)
	require.Contains(t, deployments, "foo-cluster-checks-runner-http")
	assert.Equal(t, "abc123", deployments["foo-cluster-checks-runner-http"].Spec.Template.Annotations[global.GetDCATokenChecksumAnnotationKey()])
}

func TestReconcileClusterChecksRunnerGroupsFailingGroupDoesNotBlockOthers(t *testing.T) {
	groups := []componentccr.Group{
		{Name: "bad", Checks: []string{"http_check"}},
		{Name: "good", Checks: []string{"redisdb"}},
	}
	ddai := newTestDDAI("foo", groups, false)
	orphan := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name:      "foo-cluster-checks-runner-gone",
		Namespace: ddai.Namespace,
		Labels: map[string]string{
			apicommon.AgentDeploymentComponentLabelKey:    constants.DefaultClusterChecksRunnerResourceSuffix,
			kubernetes.AppKubernetesManageByLabelKey:      "datadog-operator",
			kubernetes.AppKubernetesPartOfLabelKey:        object.NewPartOfLabelValue(ddai).String(),
			componentccr.ClusterChecksRunnerGroupLabelKey: "gone",
		},
	}}
	fakeClient := fake.NewClientBuilder().WithScheme(testScheme()).WithObjects(orphan).WithInterceptorFuncs(interceptor.Funcs{
		Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if obj.GetName() == "foo-cluster-checks-runner-bad" {
				return errors.New("create rejected")
			}
			return c.Create(ctx, obj, opts...)
		},
	}).Build()
	r := newTestReconciler(testScheme())
	r.client = fakeClient

	err := reconcileGroupsWith(r, ddai, true)
	require.ErrorContains(t, err, `cluster checks runner group "bad"`)

	deployments := listDeployments(t, r)
	assert.Contains(t, deployments, "foo-cluster-checks-runner-good")
	assert.NotContains(t, deployments, "foo-cluster-checks-runner-gone", "orphan cleanup still runs")
}

func TestReconcileClusterChecksRunnerGroupsBrokenAnnotationKeepsCore(t *testing.T) {
	groups := []componentccr.Group{{Name: "http", Checks: []string{"http_check"}}}
	ddai := newTestDDAI("foo", groups, true)
	r := newTestReconciler(testScheme())

	require.NoError(t, reconcileGroupsWith(r, ddai, true))
	require.Contains(t, listDeployments(t, r), "foo-cluster-checks-runner-http")

	ddai.Annotations[datadoghqv2alpha1.AnnotationExperimentalClusterChecksRunnerGroups] = "not-json"
	require.NoError(t, reconcileGroupsWith(r, ddai, true))

	deployments := listDeployments(t, r)
	assert.NotContains(t, deployments, "foo-cluster-checks-runner-http")
	assert.Contains(t, deployments, "foo-cluster-checks-runner-core")
}

func TestRunnerServiceAccountWithOnlyGroups(t *testing.T) {
	ddai := newTestDDAI("foo", nil, true)
	r := newTestReconciler(testScheme())
	ctx := context.Background()
	depsStore, resourceManagers := r.setupDependencies(ctx, ddai)

	require.NoError(t, r.manageGlobalDependencies(ctx, ddai, resourceManagers, feature.RequiredComponents{
		ClusterAgent: feature.RequiredComponent{IsRequired: new(true)},
	}))

	_, found := depsStore.Get(kubernetes.ServiceAccountsKind, ddai.Namespace, constants.GetClusterChecksRunnerServiceAccount(ddai.Name, &ddai.Spec))
	assert.True(t, found, "runner ServiceAccount is created for runner groups")
}

func TestCleanupOldCCRDeploymentsKeepsGroups(t *testing.T) {
	ddai := newTestDDAI("foo", nil, false)
	labels := func(group string) map[string]string {
		l := map[string]string{
			apicommon.AgentDeploymentComponentLabelKey: constants.DefaultClusterChecksRunnerResourceSuffix,
			kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
			kubernetes.AppKubernetesPartOfLabelKey:     object.NewPartOfLabelValue(ddai).String(),
		}
		if group != "" {
			l[componentccr.ClusterChecksRunnerGroupLabelKey] = group
		}
		return l
	}
	r := newTestReconciler(testScheme(),
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "foo-cluster-checks-runner", Namespace: ddai.Namespace, Labels: labels("")}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "foo-old-runners", Namespace: ddai.Namespace, Labels: labels("")}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "foo-cluster-checks-runner-core", Namespace: ddai.Namespace, Labels: labels("core")}},
	)

	require.NoError(t, r.cleanupOldCCRDeployments(context.Background(), ddai))

	deployments := listDeployments(t, r)
	assert.Contains(t, deployments, "foo-cluster-checks-runner")
	assert.Contains(t, deployments, "foo-cluster-checks-runner-core")
	assert.NotContains(t, deployments, "foo-old-runners")
}

func TestReconcileClusterChecksRunnerGroupsServiceAccount(t *testing.T) {
	groups := []componentccr.Group{
		{Name: "http", Checks: []string{"http_check"}},
		{Name: "redis", Checks: []string{"redisdb"}, Override: &datadoghqv2alpha1.DatadogAgentComponentOverride{ServiceAccountName: new("group-account")}},
	}
	ddai := newTestDDAI("foo", groups, false)
	ddai.Spec.Override[datadoghqv2alpha1.ClusterChecksRunnerComponentName].ServiceAccountName = new("runner-account")

	deployments := reconcileGroups(t, ddai, true)
	assert.Equal(t, "runner-account", deployments["foo-cluster-checks-runner-http"].Spec.Template.Spec.ServiceAccountName, "inherited from the default runners")
	assert.Equal(t, "group-account", deployments["foo-cluster-checks-runner-redis"].Spec.Template.Spec.ServiceAccountName, "a group's own ServiceAccount wins")
}
