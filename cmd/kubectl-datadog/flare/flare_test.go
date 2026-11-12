// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"archive/zip"
	"bytes"
	"context"
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
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	var names []string
	for _, f := range r.File {
		names = append(names, f.Name)
	}
	assert.Contains(t, names, "operator/logs/datadog-operator-abc/manager.log")
	assert.Contains(t, names, "operator/pods.yaml")
}

func TestNilLeaderPod(t *testing.T) {
	o := &options{}
	_, err := o.getVersion(context.Background(), nil)
	require.Error(t, err)
	require.Error(t, o.collectVersion(context.Background(), nil, nil))
	require.Error(t, o.collectMetrics(context.Background(), nil, nil))
	assert.Contains(t, getArchivePath(), "datadog-operator-")
}
