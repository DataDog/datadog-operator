// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/version"
	fakediscovery "k8s.io/client-go/discovery/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

var managedLabels = map[string]string{managedByLabel: managedByOperator}

func TestDatadogResourcesCollectors(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog", Namespace: "datadog", Annotations: map[string]string{
			"kubectl.kubernetes.io/last-applied-configuration": `{"apiKey":"` + testAPIKey + `"}`,
		}},
		Spec: v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{
			ClusterName:       ptr.To("my-cluster"),
			Credentials:       &v2alpha1.DatadogCredentials{APIKey: ptr.To(testAPIKey), AppKey: ptr.To(testAppKey)},
			ClusterAgentToken: ptr.To(testToken),
		}},
		Status: v2alpha1.DatadogAgentStatus{Conditions: []metav1.Condition{{Type: "Reconcile", Status: metav1.ConditionTrue, Reason: "ReconcileSucceeded"}}},
	}
	profile := &v1alpha1.DatadogAgentProfile{ObjectMeta: metav1.ObjectMeta{Name: "linux", Namespace: "datadog"}}
	monitor := &v1alpha1.DatadogMonitor{
		ObjectMeta: metav1.ObjectMeta{Name: "cpu", Namespace: "apps"},
		Spec:       v1alpha1.DatadogMonitorSpec{Query: "avg(last_5m):avg:system.cpu.user{*} > 90"},
		Status:     v1alpha1.DatadogMonitorStatus{ID: 1234, MonitorState: "OK"},
	}
	event := &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: "datadog.1", Namespace: "datadog"},
		InvolvedObject: corev1.ObjectReference{Kind: "DatadogAgent", APIVersion: "datadoghq.com/v2alpha1", Name: "datadog", Namespace: "datadog"},
		Reason:         "Create DaemonSet",
		Message:        "datadog/datadog-agent",
		Type:           corev1.EventTypeNormal,
	}
	otherEvent := &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: "pod.1", Namespace: "apps"},
		InvolvedObject: corev1.ObjectReference{Kind: "Pod", APIVersion: "v1", Name: "app"},
		Reason:         "Pulled",
	}

	files, manifest := runCollectors(t, testEnv(t, dda, profile, monitor, event, otherEvent), DatadogResourcesCollectors()...)
	for _, result := range manifest.Collectors {
		assert.Equal(t, StatusOK, result.Status, "%s: %v", result.Name, result.Errors)
	}

	ddaFile := files["datadog/datadogagents/datadog/datadog.yaml"]
	require.NotEmpty(t, ddaFile)
	assert.Contains(t, ddaFile, "clusterName: my-cluster")
	assert.Contains(t, ddaFile, "ReconcileSucceeded")
	assert.NotContains(t, ddaFile, "last-applied-configuration")
	assert.Contains(t, files, "datadog/datadogagentprofiles/datadog/linux.yaml")
	assert.Contains(t, files, "datadog/datadogagentinternals/NONE.txt")

	summary := files["datadog/resources-summary.yaml"]
	assert.Contains(t, summary, "monitorState: OK")
	assert.Contains(t, summary, "name: cpu")
	// The summary has the status, not the spec.
	assert.NotContains(t, summary, "system.cpu.user")

	assert.Contains(t, files["datadog/events/datadogagent.yaml"], "Create DaemonSet")
	for name := range files {
		assert.NotContains(t, name, "events/pod")
	}
	assertNoSecrets(t, files, testAPIKey, testAppKey, testToken)
}

func TestWorkloadsCollector(t *testing.T) {
	selector := &metav1.LabelSelector{MatchLabels: map[string]string{"app": "datadog-agent"}}
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog-agent", Namespace: "datadog", Labels: managedLabels},
		Spec: appsv1.DaemonSetSpec{Selector: selector, Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name: "agent",
			Env:  []corev1.EnvVar{{Name: "DD_API_KEY", Value: testAPIKey}},
		}}}}},
		Status: appsv1.DaemonSetStatus{DesiredNumberScheduled: 3, NumberReady: 2},
	}
	dca := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog-cluster-agent", Namespace: "datadog", Labels: managedLabels},
		Spec:       appsv1.DeploymentSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "dca"}}},
		Status:     appsv1.DeploymentStatus{Replicas: 1, ReadyReplicas: 1},
	}
	unmanaged := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "app", Namespace: "apps"}}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog-agent-abcde", Namespace: "datadog", Labels: map[string]string{"app": "datadog-agent"}},
		Spec:       corev1.PodSpec{NodeName: "node-1", Containers: []corev1.Container{{Name: "agent", Image: "gcr.io/datadoghq/agent:7"}}},
		Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{
			Name: "agent", RestartCount: 4, State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
		}}},
	}
	event := &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: "e1", Namespace: "datadog"},
		InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "datadog-agent-abcde"},
		Reason:         "BackOff",
		Message:        "Back-off restarting failed container",
	}

	files, manifest := runCollectors(t, testEnv(t, ds, dca, unmanaged, pod, event), WorkloadsCollector())
	require.Len(t, manifest.Collectors, 1)
	assert.Equal(t, StatusOK, manifest.Collectors[0].Status, manifest.Collectors[0].Errors)

	assert.Contains(t, files, "workloads/daemonsets/datadog/datadog-agent.yaml")
	assert.Contains(t, files, "workloads/deployments/datadog/datadog-cluster-agent.yaml")
	assert.NotContains(t, files, "workloads/deployments/apps/app.yaml")
	assert.Contains(t, files["workloads/rollout-status.yaml"], "DaemonSet datadog/datadog-agent")
	pods := files["workloads/pods/datadog/daemonset-datadog-agent.yaml"]
	assert.Contains(t, pods, "restarts: 4")
	assert.Contains(t, pods, "CrashLoopBackOff")
	assert.Contains(t, pods, "node: node-1")
	assert.Contains(t, files["workloads/events/datadog/daemonset-datadog-agent.yaml"], "BackOff")
	assertNoSecrets(t, files, testAPIKey)
}

func TestConfigCollectors(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog-cluster-agent-confd", Namespace: "datadog", Labels: managedLabels},
		Data:       map[string]string{"datadog.yaml": "api_key: " + testAPIKey + "\nlog_level: info\n"},
	}
	otherCM := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "apps"}, Data: map[string]string{"x": "y"}}
	secret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "datadog-secret", Namespace: "datadog", Labels: managedLabels}, StringData: map[string]string{"api-key": testAPIKey}}
	operatorRole := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "datadog-operator"}, Rules: []rbacv1.PolicyRule{{APIGroups: []string{""}, Resources: []string{"pods"}, Verbs: []string{"get"}}}}
	operatorBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog-operator"},
		RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "datadog-operator"},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: "datadog-operator", Namespace: "datadog"}},
	}
	agentRole := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "datadog-agent", Labels: managedLabels}}
	webhook := &admissionregistrationv1.MutatingWebhookConfiguration{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog-webhook"},
		Webhooks:   []admissionregistrationv1.MutatingWebhook{{Name: "datadog.webhook.lib.injection", ClientConfig: admissionregistrationv1.WebhookClientConfig{CABundle: []byte("CA-BUNDLE-BYTES")}}},
	}
	otherWebhook := &admissionregistrationv1.ValidatingWebhookConfiguration{ObjectMeta: metav1.ObjectMeta{Name: "istio"}}
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "datadogagents.datadoghq.com", Annotations: map[string]string{"controller-gen.kubebuilder.io/version": "v0.17.0"}},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{Group: "datadoghq.com", Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
			{Name: "v2alpha1", Served: true, Storage: true, Schema: &apiextensionsv1.CustomResourceValidation{OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{Description: "LARGE-SCHEMA"}}},
		}},
		Status: apiextensionsv1.CustomResourceDefinitionStatus{StoredVersions: []string{"v2alpha1"}},
	}
	otherCRD := &apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "certificates.cert-manager.io"}, Spec: apiextensionsv1.CustomResourceDefinitionSpec{Group: "cert-manager.io"}}
	deploy := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name: "datadog-operator", Namespace: "datadog",
		Labels:      map[string]string{"app.kubernetes.io/managed-by": "Helm", "helm.sh/chart": "datadog-operator-2.10.0"},
		Annotations: map[string]string{"meta.helm.sh/release-name": "my-operator"},
	}}
	rs := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "datadog-operator-abc", Namespace: "datadog", OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "datadog-operator"}}}}
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog-operator-abc-xyz", Namespace: "datadog", OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "datadog-operator-abc"}}},
		Spec:       corev1.PodSpec{ServiceAccountName: "datadog-operator"},
	}

	env := testEnv(t, cm, otherCM, secret, operatorRole, operatorBinding, agentRole, webhook, otherWebhook, crd, otherCRD, deploy, rs, pod)
	env.OperatorPod = pod
	files, manifest := runCollectors(t, env, ConfigCollectors()...)
	for _, result := range manifest.Collectors {
		assert.Equal(t, StatusOK, result.Status, "%s: %v", result.Name, result.Errors)
	}

	assert.Contains(t, files["config/configmaps/datadog/datadog-cluster-agent-confd.yaml"], "log_level: info")
	assert.NotContains(t, files, "config/configmaps/apps/app-config.yaml")
	for name, content := range files {
		assert.NotContains(t, name, "secret", "Secrets are never collected")
		assert.NotContains(t, content, "datadog-secret", "Secrets are never collected")
	}
	assert.Contains(t, files["config/rbac/managed-clusterroles.yaml"], "name: datadog-agent")
	operatorRBAC := files["config/rbac/operator-serviceaccount-rbac.yaml"]
	assert.Contains(t, operatorRBAC, "serviceAccount: datadog/datadog-operator")
	assert.Contains(t, operatorRBAC, "pods")
	assert.Contains(t, files, "config/webhooks/mutating/datadog-webhook.yaml")
	assert.NotContains(t, files, "config/webhooks/validating/istio.yaml")
	crds := files["config/crds.yaml"]
	assert.Contains(t, crds, "datadogagents.datadoghq.com")
	assert.Contains(t, crds, "storage: true")
	assert.NotContains(t, crds, "cert-manager")
	assert.NotContains(t, crds, "LARGE-SCHEMA")
	assert.Contains(t, files["operator/helm.yaml"], "installedWithHelm: true")
	assert.Contains(t, files["operator/helm.yaml"], "datadog-operator-2.10.0")
	assert.Contains(t, files, "operator/deployment.yaml")
	assert.Contains(t, files, "operator/pods/datadog-operator-abc-xyz.yaml")
	assertNoSecrets(t, files, testAPIKey, "CA-BUNDLE-BYTES", "Q0EtQlVORExFLUJZVEVT")
}

func TestClusterCollector(t *testing.T) {
	kubeSystem := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system", UID: types.UID("cluster-uid-1234")}}
	linux := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-1"},
		Spec:       corev1.NodeSpec{ProviderID: "aws:///us-east-1a/i-123"},
		Status: corev1.NodeStatus{
			NodeInfo:   corev1.NodeSystemInfo{OperatingSystem: "linux", Architecture: "amd64", KubeletVersion: "v1.30.1"},
			Conditions: []corev1.NodeCondition{{Type: corev1.NodeReady, Status: corev1.ConditionTrue}},
		},
	}
	windows := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "node-2"},
		Status:     corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{OperatingSystem: "windows", Architecture: "amd64"}},
	}
	env := testEnv(t, kubeSystem, linux, windows)
	env.ClusterName = "my-cluster"
	env.Clientset.(*kubefake.Clientset).Discovery().(*fakediscovery.FakeDiscovery).FakedServerVersion = &version.Info{GitVersion: "v1.30.1-eks-abc"}

	files, manifest := runCollectors(t, env, ClusterCollector())
	assert.Equal(t, StatusOK, manifest.Collectors[0].Status, manifest.Collectors[0].Errors)
	cluster := files["cluster.yaml"]
	assert.Contains(t, cluster, "clusterID: cluster-uid-1234")
	assert.Contains(t, cluster, "clusterName: my-cluster")
	assert.Contains(t, cluster, "distribution: eks")
	assert.Contains(t, cluster, "count: 2")
	assert.Contains(t, cluster, "windows: 1")
	assert.Contains(t, cluster, "v1.30.1-eks-abc")
	assert.Contains(t, cluster, "- node-2")

	id, err := ClusterID(context.Background(), env.Reader)
	require.NoError(t, err)
	assert.Equal(t, "cluster-uid-1234", id)
}

func TestCollectPodLogs(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog-operator-xyz", Namespace: "datadog"},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "manager"}, {Name: "sidecar"}}},
		Status: corev1.PodStatus{ContainerStatuses: []corev1.ContainerStatus{
			{Name: "manager", RestartCount: 2},
			{Name: "sidecar"},
		}},
	}
	env := testEnv(t, pod)
	var logOptions []corev1.PodLogOptions
	env.LogTailLines = 50
	env.Clientset.(*kubefake.Clientset).PrependReactor("get", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "log" {
			return false, nil, nil
		}
		logOptions = append(logOptions, *action.(k8stesting.GenericAction).GetValue().(*corev1.PodLogOptions))
		return true, &runtime.Unknown{Raw: []byte(`{"level":"info","msg":"started","apiKey":"` + testToken + `"}` + "\nDD_API_KEY=" + testAPIKey + "\n")}, nil
	})

	files, manifest := runCollectors(t, env, Collector{Name: "logs", Collect: func(ctx context.Context, c *Collection) error {
		CollectPodLogs(ctx, c, "operator/logs", pod)
		return nil
	}})
	assert.Equal(t, StatusOK, manifest.Collectors[0].Status, manifest.Collectors[0].Errors)
	assert.Contains(t, files, "operator/logs/datadog-operator-xyz/manager.log")
	assert.Contains(t, files, "operator/logs/datadog-operator-xyz/manager.previous.log")
	assert.Contains(t, files, "operator/logs/datadog-operator-xyz/sidecar.log")
	assert.NotContains(t, files, "operator/logs/datadog-operator-xyz/sidecar.previous.log")
	assert.Contains(t, files["operator/logs/datadog-operator-xyz/manager.log"], `"msg":"started"`)
	assertNoSecrets(t, files, testToken, testAPIKey)

	require.Len(t, logOptions, 3)
	previous := 0
	for _, opts := range logOptions {
		assert.Equal(t, int64(50), *opts.TailLines)
		assert.Equal(t, DefaultLogLimitBytes, *opts.LimitBytes)
		if opts.Previous {
			previous++
			assert.Equal(t, "manager", opts.Container)
		}
	}
	assert.Equal(t, 1, previous)
}

func TestDistribution(t *testing.T) {
	node := func(providerID string, labels map[string]string) []corev1.Node {
		return []corev1.Node{{ObjectMeta: metav1.ObjectMeta{Labels: labels}, Spec: corev1.NodeSpec{ProviderID: providerID}}}
	}
	tests := []struct {
		version string
		nodes   []corev1.Node
		want    string
	}{
		{"v1.30.1-eks-abc", nil, "eks"},
		{"v1.30.1-gke.100", nil, "gke"},
		{"v1.30.1+k3s1", nil, "k3s"},
		{"v1.30.1+rke2r1", nil, "rke2"},
		{"v1.30.1", node("", map[string]string{"node.openshift.io/os_id": "rhcos"}), "openshift"},
		{"v1.30.1", node("azure:///subscriptions/x", nil), "aks"},
		{"v1.30.1", node("aws:///us-east-1a/i-1", nil), "aws"},
		{"v1.30.1", node("gce://project/zone/name", nil), "gce"},
		{"v1.30.1", node("kind://docker/kind/kind-control-plane", nil), "kind"},
		{"v1.30.1", node("nebius://instance", nil), "nebius"},
		{"v1.30.1", node("", nil), "unknown"},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, distribution(&version.Info{GitVersion: tt.version}, tt.nodes), tt.version)
	}
	assert.Equal(t, "unknown", distribution(nil, nil))
}

func TestContainerState(t *testing.T) {
	now := metav1.Now()
	assert.Contains(t, containerState(corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: now}}), "running since")
	assert.Contains(t, containerState(corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled", ExitCode: 137, FinishedAt: now}}), "OOMKilled (exit code 137)")
	assert.Empty(t, containerState(corev1.ContainerState{}))
}

func TestOperatorRBACWithRoleBindings(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "op", Namespace: "datadog"}}
	role := &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "leader-election", Namespace: "datadog"}, Rules: []rbacv1.PolicyRule{{Resources: []string{"leases"}}}}
	sa := rbacv1.Subject{Kind: rbacv1.ServiceAccountKind, Name: "default", Namespace: "datadog"}
	roleBinding := &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "leader-election", Namespace: "datadog"}, RoleRef: rbacv1.RoleRef{Kind: "Role", Name: "leader-election"}, Subjects: []rbacv1.Subject{sa}}
	clusterRoleBinding := &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "view", Namespace: "datadog"}, RoleRef: rbacv1.RoleRef{Kind: "ClusterRole", Name: "missing"}, Subjects: []rbacv1.Subject{sa}}
	otherBinding := &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "other", Namespace: "datadog"}, RoleRef: rbacv1.RoleRef{Kind: "Role", Name: "x"}, Subjects: []rbacv1.Subject{{Kind: "User", Name: "bob"}}}
	env := testEnv(t, pod, role, roleBinding, clusterRoleBinding, otherBinding)
	env.OperatorPod = pod
	files, manifest := runCollectors(t, env, Collector{Name: "rbac", Collect: collectRBAC})
	assert.Equal(t, StatusPartial, manifest.Collectors[0].Status, "the missing ClusterRole is reported")
	rbac := files["config/rbac/operator-serviceaccount-rbac.yaml"]
	assert.Contains(t, rbac, "serviceAccount: datadog/default")
	assert.Contains(t, rbac, "leases")
	assert.NotContains(t, rbac, "bob")
}

func TestOperatorDeploymentErrors(t *testing.T) {
	_, manifest := runCollectors(t, testEnv(t), Collector{Name: "deployment", Collect: collectOperatorDeployment})
	assert.Equal(t, StatusError, manifest.Collectors[0].Status)

	orphan := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "op", Namespace: "datadog"}}
	_, err := operatorDeployment(context.Background(), testEnv(t).Reader, orphan)
	require.ErrorContains(t, err, "isn't owned by a Deployment")

	owned := orphan.DeepCopy()
	owned.OwnerReferences = []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "missing"}}
	_, err = operatorDeployment(context.Background(), testEnv(t).Reader, owned)
	require.ErrorContains(t, err, "ReplicaSet")
}

func TestCollectPodLogsLimits(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "op", Namespace: "datadog"}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "manager"}}}}
	_, manifest := runCollectors(t, Env{}, Collector{Name: "no-clientset", Collect: func(ctx context.Context, c *Collection) error {
		CollectPodLogs(ctx, c, "logs", pod)
		return nil
	}})
	assert.Equal(t, StatusPartial, manifest.Collectors[0].Status)

	env := testEnv(t, pod)
	archive := NewArchive(ArchiveOptions{MaxBytes: manifestReserve})
	manifest = Run(context.Background(), env, archive, []Collector{{Name: "full", Collect: func(ctx context.Context, c *Collection) error {
		CollectPodLogs(ctx, c, "logs", pod)
		return nil
	}}}, RunOptions{})
	assert.Equal(t, StatusPartial, manifest.Collectors[0].Status)
	assert.Contains(t, manifest.Collectors[0].Errors[0], "size limit")

	env.Clientset.(*kubefake.Clientset).PrependReactor("get", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, assert.AnError
	})
	_, manifest = runCollectors(t, env, Collector{Name: "error", Collect: func(ctx context.Context, c *Collection) error {
		CollectPodLogs(ctx, c, "logs", pod)
		return nil
	}})
	assert.Equal(t, StatusPartial, manifest.Collectors[0].Status)
}

func TestHelpers(t *testing.T) {
	assert.Equal(t, "_", safeName(""))
	assert.Equal(t, "a_b", safeName("a/b"))
	assert.True(t, isNotInstalled(assert.AnError) == false)
	a := NewArchive(ArchiveOptions{})
	require.NoError(t, a.AddJSON("x.json", map[string]string{"a": "b"}))
	assert.Equal(t, []FileInfo{{Path: "x.json", Size: int64(len("{\n  \"a\": \"b\"\n}"))}}, a.Files())
	require.Error(t, a.AddJSON("bad.json", make(chan int)))
	require.Error(t, a.AddYAML("bad.yaml", make(chan int)))
}

// failingReader fails every read, like an API server that refuses access.
type failingReader struct{ client.Reader }

func (failingReader) Get(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error {
	return assert.AnError
}

func (failingReader) List(context.Context, client.ObjectList, ...client.ListOption) error {
	return assert.AnError
}

func TestCollectorsReportAPIErrors(t *testing.T) {
	clientset := kubefake.NewSimpleClientset()
	clientset.PrependReactor("*", "*", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, assert.AnError
	})
	clientset.Discovery().(*fakediscovery.FakeDiscovery).PrependReactor("*", "*", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, assert.AnError
	})
	env := Env{Reader: failingReader{}, Clientset: clientset, OperatorPod: &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "op", Namespace: "datadog"}}}
	collectors := append(DatadogResourcesCollectors(), ConfigCollectors()...)
	collectors = append(collectors, WorkloadsCollector(), ClusterCollector())
	files, manifest := runCollectors(t, env, collectors...)
	assert.False(t, manifest.Complete)
	for _, result := range manifest.Collectors {
		assert.Contains(t, []string{StatusError, StatusPartial}, result.Status, result.Name)
		assert.NotEmpty(t, result.Errors, result.Name)
	}
	assert.Contains(t, files, ManifestFile)
}
