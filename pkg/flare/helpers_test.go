// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/pkg/flare/flaretest"
)

// testEnv returns an Env backed by fake clients holding objs.
func testEnv(t *testing.T, objs ...client.Object) Env {
	t.Helper()
	reader, clientset := flaretest.Clients(t, objs...)
	return Env{Reader: reader, Clientset: clientset, OperatorNamespace: "datadog"}
}

// runCollectors runs collectors on env and returns the files of the
// archive, without the archive root, and the manifest.
func runCollectors(t *testing.T, env Env, collectors ...Collector) (map[string]string, *Manifest) {
	t.Helper()
	archive := NewArchive(ArchiveOptions{Root: "root"})
	manifest := Run(context.Background(), env, archive, collectors, RunOptions{})
	data, err := archive.Close(manifest)
	require.NoError(t, err)
	files := map[string]string{}
	for name, content := range flaretest.ReadZip(t, data) {
		files[strings.TrimPrefix(name, "root/")] = content
	}
	return files, manifest
}

// podLogsCollector collects the logs of pod under dir.
func podLogsCollector(name, dir string, pod *corev1.Pod) Collector {
	return Collector{Name: name, Collect: func(ctx context.Context, c *Collection) error {
		CollectPodLogs(ctx, c, dir, pod)
		return nil
	}}
}

// assertNoSecrets fails if any file contains one of the secrets.
func assertNoSecrets(t *testing.T, files map[string]string, secrets ...string) {
	t.Helper()
	for name, content := range files {
		for _, secret := range secrets {
			require.NotContains(t, content, secret, "secret found in %s", name)
		}
	}
}
