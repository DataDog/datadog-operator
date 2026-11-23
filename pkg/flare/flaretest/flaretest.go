// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package flaretest holds test helpers shared by the flare packages.
package flaretest

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"sync/atomic"
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

// Intake is a fake flare intake that records the last flare it received.
type Intake struct {
	Server   *httptest.Server
	Posts    atomic.Int32
	Status   int
	Response string

	mu       sync.Mutex
	path     string
	apiKey   string
	fields   map[string][]string
	fileName string
	file     []byte
}

// Received is what the intake recorded from the last flare.
type Received struct {
	Path, APIKey, FileName string
	Fields                 map[string][]string
	File                   []byte
}

// NewIntake starts a fake intake. HEAD requests get a 404, so that no
// redirect is followed.
func NewIntake(t *testing.T) *Intake {
	in := &Intake{Status: http.StatusOK, Response: `{"case_id": 1234}`}
	in.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		in.Posts.Add(1)
		in.mu.Lock()
		defer in.mu.Unlock()
		in.path = r.URL.Path
		in.apiKey = r.Header.Get("Dd-Api-Key")
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		in.fields = r.MultipartForm.Value
		if files := r.MultipartForm.File["flare_file"]; len(files) == 1 {
			in.fileName = files[0].Filename
			file, _ := files[0].Open()
			in.file, _ = io.ReadAll(file)
			file.Close()
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(in.Status)
		_, _ = w.Write([]byte(in.Response))
	}))
	t.Cleanup(in.Server.Close)
	return in
}

// Last returns what the intake recorded from the last flare.
func (in *Intake) Last() Received {
	in.mu.Lock()
	defer in.mu.Unlock()
	return Received{Path: in.path, APIKey: in.apiKey, FileName: in.fileName, Fields: in.fields, File: in.file}
}

// ReadZipFile returns the files of a zip archive file by name, and removes
// the file at the end of the test.
func ReadZipFile(t *testing.T, path string) map[string]string {
	t.Helper()
	t.Cleanup(func() { _ = os.Remove(path) })
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return ReadZip(t, data)
}

// ReadZip returns the files of a zip archive by name.
func ReadZip(t *testing.T, data []byte) map[string]string {
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

// Scheme returns a scheme with the Kubernetes, CRD and Datadog types.
func Scheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(s))
	require.NoError(t, apiextensionsv1.AddToScheme(s))
	require.NoError(t, v1alpha1.AddToScheme(s))
	require.NoError(t, v2alpha1.AddToScheme(s))
	return s
}

// Clients returns a controller-runtime client holding objs, and a clientset
// holding the objs of Kubernetes types.
func Clients(t *testing.T, objs ...client.Object) (client.Client, *kubefake.Clientset) {
	t.Helper()
	reader := fake.NewClientBuilder().WithScheme(Scheme(t)).WithObjects(objs...).Build()
	var kubeObjs []runtime.Object
	for _, obj := range objs {
		if gvk, _, err := clientgoscheme.Scheme.ObjectKinds(obj); err == nil && len(gvk) > 0 {
			kubeObjs = append(kubeObjs, obj.DeepCopyObject())
		}
	}
	return reader, kubefake.NewSimpleClientset(kubeObjs...)
}
