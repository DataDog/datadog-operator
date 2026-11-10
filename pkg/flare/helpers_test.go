// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kubefake "k8s.io/client-go/kubernetes/fake"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// readZip returns the files of a zip archive by name.
func readZip(t *testing.T, data []byte) map[string]string {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	files := map[string]string{}
	for _, f := range r.File {
		rc, err := f.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(rc)
		require.NoError(t, err)
		require.NoError(t, rc.Close())
		files[f.Name] = string(content)
	}
	return files
}

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(s))
	require.NoError(t, apiextensionsv1.AddToScheme(s))
	require.NoError(t, v1alpha1.AddToScheme(s))
	require.NoError(t, v2alpha1.AddToScheme(s))
	return s
}

// testEnv returns an Env backed by fake clients holding objs. Objects are
// added to both the controller-runtime client and the clientset.
func testEnv(t *testing.T, objs ...client.Object) Env {
	t.Helper()
	s := testScheme(t)
	reader := fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
	var kubeObjs []runtime.Object
	for _, obj := range objs {
		gvk, _, err := clientgoscheme.Scheme.ObjectKinds(obj)
		if err == nil && len(gvk) > 0 {
			kubeObjs = append(kubeObjs, obj.DeepCopyObject())
		}
	}
	return Env{
		Reader:            reader,
		Clientset:         kubefake.NewSimpleClientset(kubeObjs...),
		OperatorNamespace: "datadog",
	}
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
	for name, content := range readZip(t, data) {
		files[strings.TrimPrefix(name, "root/")] = content
	}
	return files, manifest
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
