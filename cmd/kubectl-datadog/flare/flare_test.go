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
	kubefake "k8s.io/client-go/kubernetes/fake"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	pkgflare "github.com/DataDog/datadog-operator/pkg/flare"
	"github.com/DataDog/datadog-operator/pkg/flare/flaretest"
	"github.com/DataDog/datadog-operator/pkg/version"
)

func TestMetricsPort(t *testing.T) {
	tests := []struct {
		name      string
		container corev1.Container
		want      string
		wantErr   bool
	}{
		{name: "default", container: corev1.Container{}, want: "8080"},
		{name: "flag with equal", container: corev1.Container{Args: []string{"-metrics-addr=:8383"}}, want: "8383"},
		{name: "flag with space", container: corev1.Container{Args: []string{"--metrics-addr", "0.0.0.0:9090"}}, want: "9090"},
		{name: "env", container: corev1.Container{Env: []corev1.EnvVar{{Name: "DD_METRICS_ADDR", Value: ":8384"}}}, want: "8384"},
		{name: "flag wins over env", container: corev1.Container{Args: []string{"-metrics-addr=:8385"}, Env: []corev1.EnvVar{{Name: "DD_METRICS_ADDR", Value: ":8384"}}}, want: "8385"},
		{name: "disabled", container: corev1.Container{Args: []string{"-metrics-addr=0"}}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			port, err := metricsPort(&corev1.Pod{Spec: corev1.PodSpec{Containers: []corev1.Container{tt.container}}})
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, port)
		})
	}
}

func TestValidateSite(t *testing.T) {
	email, apiKey = "user@example.com", "key"
	t.Cleanup(func() { email, apiKey, ddSite = "", "", "us" })
	for site, want := range map[string]string{"us": "datadoghq.com", "EU": "datadoghq.eu"} {
		ddSite = site
		o := &options{}
		require.NoError(t, o.validate())
		assert.Equal(t, want, o.site)
	}
	ddSite = "mars"
	require.Error(t, (&options{}).validate())
}

func TestGetLeader(t *testing.T) {
	ctx := context.Background()
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "datadog-operator-abc", Namespace: "datadog"}}
	lease := &coordinationv1.Lease{
		ObjectMeta: metav1.ObjectMeta{Name: leaderElectionID, Namespace: "datadog"},
		Spec:       coordinationv1.LeaseSpec{HolderIdentity: ptr.To("datadog-operator-abc_1234")},
	}

	reader := fake.NewClientBuilder().WithObjects(lease).Build()
	leader, err := getLeader(ctx, reader, kubefake.NewSimpleClientset(pod), "datadog")
	require.NoError(t, err)
	assert.Equal(t, "datadog-operator-abc", leader.Name)

	// Older Operators: the ConfigMap lock owned by the leader pod.
	cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: leaderElectionID, Namespace: "datadog", OwnerReferences: []metav1.OwnerReference{{Kind: "Pod", Name: "datadog-operator-abc"}}}}
	leader, err = getLeader(ctx, fake.NewClientBuilder().Build(), kubefake.NewSimpleClientset(pod, cm), "datadog")
	require.NoError(t, err)
	assert.Equal(t, "datadog-operator-abc", leader.Name)

	_, err = getLeader(ctx, fake.NewClientBuilder().Build(), kubefake.NewSimpleClientset(), "datadog")
	require.Error(t, err)

	emptyLease := &coordinationv1.Lease{ObjectMeta: metav1.ObjectMeta{Name: leaderElectionID, Namespace: "datadog"}}
	_, err = getLeader(ctx, fake.NewClientBuilder().WithObjects(emptyLease).Build(), kubefake.NewSimpleClientset(), "datadog")
	require.ErrorContains(t, err, "leader name not found")
}

func TestCollectOperatorLogs(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog-operator-abc", Namespace: "datadog", Labels: map[string]string{"app.kubernetes.io/name": "datadog-operator"}},
		Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "manager"}}},
	}
	clientset := kubefake.NewSimpleClientset(pod)
	env := pkgflare.Env{Clientset: clientset, OperatorNamespace: "datadog"}
	archive := pkgflare.NewArchive(pkgflare.ArchiveOptions{})
	manifest := pkgflare.Run(context.Background(), env, archive, []pkgflare.Collector{{Name: "logs", Collect: func(ctx context.Context, c *pkgflare.Collection) error {
		return collectOperatorLogs(ctx, c, clientset, "datadog")
	}}, {Name: "no-pods", Collect: func(ctx context.Context, c *pkgflare.Collection) error {
		return collectOperatorLogs(ctx, c, clientset, "empty")
	}}}, pkgflare.RunOptions{})
	assert.Equal(t, pkgflare.StatusOK, manifest.Collectors[0].Status)
	assert.Equal(t, pkgflare.StatusError, manifest.Collectors[1].Status)
	data, err := archive.Close(manifest)
	require.NoError(t, err)
	files := flaretest.ReadZip(t, data)
	assert.Contains(t, files, "operator/logs/datadog-operator-abc/manager.log")
	assert.Contains(t, files, "operator/pods.yaml")
}

func TestNilLeaderPod(t *testing.T) {
	o := &options{}
	_, err := o.collectVersion(context.Background(), nil, nil)
	require.Error(t, err)
	require.Error(t, o.collectMetrics(context.Background(), nil, nil))
	require.Error(t, o.collectHealth(context.Background(), nil, nil))
}

func TestResolveVersion(t *testing.T) {
	ctx := context.Background()
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog-operator-abc", Namespace: "datadog"},
		Spec: corev1.PodSpec{Containers: []corev1.Container{
			{Name: "sidecar", Image: "busybox:1.36.1"},
			{Name: "manager", Image: "gcr.io/datadoghq/operator:1.22.0", Command: []string{"/manager"}},
		}},
	}
	jsonOut := []byte(`{"version":"v1.23.0_abc","commit":"abc","go":"go1.26","os":"linux/arm64"}`)
	textOut := []byte("Version:\n- Version: 163b93d0\n- Git Commit: 163b93d0\n")
	exitErr := errors.New("command terminated with exit code 2: flag provided but not defined: -version-format")

	t.Run("json", func(t *testing.T) {
		var calls [][]string
		info, source, err := resolveVersion(ctx, pod, func(_ context.Context, p *corev1.Pod, container string, command []string) ([]byte, error) {
			assert.Equal(t, "manager", container)
			calls = append(calls, command)
			return jsonOut, nil
		})
		require.NoError(t, err)
		assert.Equal(t, "v1.23.0_abc", info.Version)
		assert.Equal(t, "exec /manager --version --version-format json", source)
		assert.Equal(t, [][]string{{"/manager", "--version", "--version-format", "json"}}, calls, "no shell: the image has none")
	})

	t.Run("text, for Operators without --version-format", func(t *testing.T) {
		info, source, err := resolveVersion(ctx, pod, func(_ context.Context, _ *corev1.Pod, _ string, command []string) ([]byte, error) {
			if len(command) > 2 {
				return nil, exitErr
			}
			return textOut, nil
		})
		require.NoError(t, err)
		assert.Equal(t, "163b93d0", info.Version)
		assert.Equal(t, "exec /manager --version", source)
	})

	t.Run("image tag when exec fails", func(t *testing.T) {
		info, source, err := resolveVersion(ctx, pod, func(context.Context, *corev1.Pod, string, []string) ([]byte, error) {
			return nil, errors.New("pods/exec is forbidden")
		})
		require.NoError(t, err)
		assert.Equal(t, "1.22.0", info.Version)
		assert.Contains(t, source, "image tag")
	})

	t.Run("unknown", func(t *testing.T) {
		noTag := pod.DeepCopy()
		noTag.Spec.Containers[1].Image = "registry.local:5000/operator@sha256:abc"
		_, _, err := resolveVersion(ctx, noTag, func(context.Context, *corev1.Pod, string, []string) ([]byte, error) {
			return []byte("garbage"), nil
		})
		require.ErrorContains(t, err, "unable to get the Operator version")
		_, _, err = resolveVersion(ctx, nil, nil)
		require.Error(t, err)
	})
}

func TestCollectVersionFile(t *testing.T) {
	const gitCommit = "0332fa2858bd59c1fcbe5fd4a6b0e4ea4a24c8d3"
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Name: "op"}, Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "manager"}}}}
	archive := pkgflare.NewArchive(pkgflare.ArchiveOptions{Root: "root"})
	var got *version.JSON
	manifest := pkgflare.Run(context.Background(), pkgflare.Env{}, archive, []pkgflare.Collector{{Name: "operator/version", Collect: func(ctx context.Context, c *pkgflare.Collection) error {
		info, source, err := resolveVersion(ctx, pod, func(context.Context, *corev1.Pod, string, []string) ([]byte, error) {
			return []byte(`{"version":"v1.23.0_abc","commit":"` + gitCommit + `"}`), nil
		})
		require.NoError(t, err)
		got, err = writeVersion(c, pod, info, source)
		return err
	}}}, pkgflare.RunOptions{})
	require.Equal(t, pkgflare.StatusOK, manifest.Collectors[0].Status)
	assert.Equal(t, "1.23.0", got.Version, "normalized for the intake host and the manifest")
	data, err := archive.Close(manifest)
	require.NoError(t, err)
	files := flaretest.ReadZip(t, data)
	require.Contains(t, files, "root/operator/version.yaml")
	// The git commit has the shape of an application key, but is kept.
	assert.Contains(t, files["root/operator/version.yaml"], "commit: "+gitCommit)
}

func TestImageTag(t *testing.T) {
	for image, want := range map[string]string{
		"gcr.io/datadoghq/operator:1.22.0":          "1.22.0",
		"datadog/operator:v1.22.0-rc.1":             "v1.22.0-rc.1",
		"registry.local:5000/operator":              "",
		"registry.local:5000/operator:latest":       "",
		"gcr.io/datadoghq/operator:1.22.0@sha256:a": "1.22.0",
		"operator": "",
	} {
		assert.Equal(t, want, imageTag(image), image)
	}
}
