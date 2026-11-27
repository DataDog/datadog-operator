// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
)

func operatorTestPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog-operator-abc", Namespace: "datadog"},
		Spec: corev1.PodSpec{Containers: []corev1.Container{
			{Name: "sidecar", Image: "busybox"},
			{
				Name:    "operator",
				Image:   "gcr.io/datadoghq/operator:1.22.0",
				Command: []string{"/manager"},
				Args:    []string{"--enable-leader-election", "--pprof", "-loglevel=debug", "--secretBackendArgs", "--token " + testToken, "--metrics-addr", ":8383"},
				Env: []corev1.EnvVar{
					{Name: "DD_API_KEY", Value: testAPIKey},
					{Name: "DD_APP_KEY", ValueFrom: &corev1.EnvVarSource{SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: "datadog-secret"}, Key: "app-key"}}},
					{Name: "POD_NAME", ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.name"}}},
					{Name: "DD_SITE", Value: "datadoghq.eu"},
					{Name: "MY_TOKEN", Value: testToken},
				},
				EnvFrom: []corev1.EnvFromSource{{SecretRef: &corev1.SecretEnvSource{LocalObjectReference: corev1.LocalObjectReference{Name: "extra"}}}},
				Ports:   []corev1.ContainerPort{{Name: "health", ContainerPort: 8085}},
				ReadinessProbe: &corev1.Probe{ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
					Path: "/readyz", Port: intstr.FromString("health"),
				}}},
			},
		}},
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionFalse, Reason: "ContainersNotReady"}},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "operator", Ready: false, Started: ptr.To(true), RestartCount: 3,
				LastTerminationState: corev1.ContainerState{Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled", ExitCode: 137}},
			}},
		},
	}
}

func TestOperatorContainer(t *testing.T) {
	pod := operatorTestPod()
	assert.Equal(t, "operator", OperatorContainer(pod).Name)
	assert.Nil(t, OperatorContainer(nil))
	assert.Equal(t, "a", OperatorContainer(&corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "a"}, {Name: "b"}}}}).Name)
	assert.Equal(t, "manager", OperatorContainer(&corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "a"}, {Name: "manager"}}}}).Name)
}

func TestContainerFlags(t *testing.T) {
	container := &corev1.Container{Command: []string{"/manager", "-a=1"}, Args: []string{"--b", "2", "--c", "--d=", "-", "--", "positional", "--a=3", "--secretBackendArgs", "--x y", "--e"}}
	assert.Equal(t, [][2]string{{"a", "1"}, {"b", "2"}, {"c", "true"}, {"d", ""}, {"a", "3"}, {"secretBackendArgs", "--x y"}, {"e", "true"}}, ContainerFlags(container))
	value, ok := ContainerFlag(container, "a")
	assert.True(t, ok)
	assert.Equal(t, "3", value)
	_, ok = ContainerFlag(container, "missing")
	assert.False(t, ok)
	assert.Nil(t, ContainerFlags(nil))
}

func TestOperatorPodConfigCollector(t *testing.T) {
	env := testEnv(t)
	env.OperatorPod = operatorTestPod()
	files, manifest := runCollectors(t, env, OperatorPodConfigCollector())
	require.Equal(t, StatusOK, manifest.Collectors[0].Status, manifest.Collectors[0].Errors)
	config := files["operator/config.yaml"]
	assert.Contains(t, config, "container: operator")
	assert.Contains(t, config, "image: gcr.io/datadoghq/operator:1.22.0")
	assert.Contains(t, config, "loglevel: debug")
	assert.Contains(t, config, "metrics-addr: :8383")
	assert.Contains(t, config, "pprof: \"true\"")
	assert.Contains(t, config, "secretBackendArgs: '********'")
	assert.Contains(t, config, "- name: DD_API_KEY\n  value: '********'")
	assert.Contains(t, config, "- name: MY_TOKEN\n  value: '********'")
	assert.Contains(t, config, "- name: DD_SITE\n  value: datadoghq.eu")
	assert.Contains(t, config, "- from: Secret datadog-secret, key app-key\n  name: DD_APP_KEY")
	assert.Contains(t, config, "- from: field metadata.name\n  name: POD_NAME")
	assert.NotContains(t, config, "token")
	assert.Contains(t, config, "Secret extra")
	assertNoSecrets(t, files, testToken, testAPIKey)

	env.OperatorPod = nil
	_, manifest = runCollectors(t, env, OperatorPodConfigCollector())
	assert.Equal(t, StatusError, manifest.Collectors[0].Status)
}

func TestLeaderElectionCollector(t *testing.T) {
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: LeaderElectionID, Namespace: "datadog"},
		Spec:       coordinationv1.LeaseSpec{HolderIdentity: ptr.To("datadog-operator-abc_1234"), LeaseDurationSeconds: ptr.To[int32](60)},
	}
	env := testEnv(t, lease)
	env.OperatorPod = operatorTestPod()
	files, manifest := runCollectors(t, env, LeaderElectionCollector(LeaderElectionID))
	require.Equal(t, StatusOK, manifest.Collectors[0].Status, manifest.Collectors[0].Errors)
	assert.Contains(t, files["operator/leader-election.yaml"], "holderIdentity: datadog-operator-abc_1234")
	assert.Contains(t, files["operator/leader-election.yaml"], "leaderPod: datadog-operator-abc")

	// No Lease (older Operators lock a ConfigMap): partial, file still written.
	files, manifest = runCollectors(t, testEnv(t), LeaderElectionCollector(LeaderElectionID))
	assert.Equal(t, StatusPartial, manifest.Collectors[0].Status)
	assert.Contains(t, files["operator/leader-election.yaml"], "id: datadog-operator-lock")

	_, manifest = runCollectors(t, Env{}, LeaderElectionCollector(LeaderElectionID))
	assert.Equal(t, StatusError, manifest.Collectors[0].Status)
}

func TestCollectHealthEndpoints(t *testing.T) {
	collector := func(get HealthGetter) Collector {
		return Collector{Name: "health", Collect: func(ctx context.Context, c *Collection) error {
			return CollectHealthEndpoints(ctx, c, get)
		}}
	}
	files, manifest := runCollectors(t, Env{}, collector(func(_ context.Context, endpoint string) (string, []byte, error) {
		if endpoint == "/readyz?verbose" {
			return "500 Internal Server Error", []byte("[-]ping failed"), nil
		}
		return "200 OK", []byte("[+]ping ok"), nil
	}))
	assert.Equal(t, StatusOK, manifest.Collectors[0].Status)
	assert.Contains(t, files["operator/health.txt"], "GET /healthz?verbose: 200 OK\n[+]ping ok")
	assert.Contains(t, files["operator/health.txt"], "GET /readyz?verbose: 500 Internal Server Error\n[-]ping failed")

	files, manifest = runCollectors(t, Env{}, collector(func(_ context.Context, endpoint string) (string, []byte, error) {
		if endpoint == "/readyz?verbose" {
			return "", nil, errors.New("connection refused")
		}
		return "200 OK", nil, nil
	}))
	assert.Equal(t, StatusPartial, manifest.Collectors[0].Status)
	assert.Contains(t, files, "operator/health.txt")

	// The Operator registers no readiness checks: /readyz is a 404.
	files, manifest = runCollectors(t, Env{}, collector(func(_ context.Context, endpoint string) (string, []byte, error) {
		if endpoint == "/readyz?verbose" {
			return "404 Not Found", []byte("404 page not found"), nil
		}
		return "200 OK", []byte("healthz check passed"), nil
	}))
	assert.Equal(t, StatusOK, manifest.Collectors[0].Status)
	assert.Contains(t, files["operator/health.txt"], "GET /readyz?verbose: 404 Not Found (no checks are registered for this endpoint)")
	assert.NotContains(t, files["operator/health.txt"], "404 page not found")

	_, manifest = runCollectors(t, Env{}, collector(func(context.Context, string) (string, []byte, error) {
		return "", nil, errors.New("connection refused")
	}))
	assert.Equal(t, StatusError, manifest.Collectors[0].Status)
}

func TestPodHealth(t *testing.T) {
	pod := operatorTestPod()
	health := SummarizePodHealth(pod)
	assert.Equal(t, "datadog-operator-abc", health.Pod)
	require.Len(t, health.Conditions, 1)
	require.Len(t, health.Containers, 1)
	assert.Equal(t, int32(3), health.Containers[0].RestartCount)
	assert.Equal(t, "OOMKilled", health.Containers[0].LastState.Terminated.Reason)
	assert.NotNil(t, health.Containers[0].ReadinessProbe)

	assert.Equal(t, "8085", HealthProbePort(pod))
	pod.Spec.Containers[1].ReadinessProbe.HTTPGet.Port = intstr.FromInt32(9000)
	assert.Equal(t, "9000", HealthProbePort(pod))
	pod.Spec.Containers[1].ReadinessProbe.HTTPGet.Port = intstr.FromString("missing")
	assert.Equal(t, DefaultHealthProbePort, HealthProbePort(pod))
	assert.Equal(t, DefaultHealthProbePort, HealthProbePort(nil))
}

func TestRedaction(t *testing.T) {
	assert.Equal(t, redacted, RedactFlagValue("secretBackendConfig", "x"))
	assert.Equal(t, redacted, RedactFlagValue("apiKey", "x"))
	assert.Equal(t, "", RedactFlagValue("apiKey", ""))
	assert.Equal(t, "debug", RedactFlagValue("loglevel", "debug"))
	assert.Equal(t, redacted, RedactEnvValue("DD_API_KEY_FILE", "/x"))
	assert.Equal(t, "datadoghq.com", RedactEnvValue("DD_SITE", "datadoghq.com"))
	assert.True(t, IsCollectedEnv("DD_SITE"))
	assert.False(t, IsCollectedEnv("HOME"))
}
