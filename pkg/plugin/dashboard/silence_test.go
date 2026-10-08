// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dashboard

import (
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/klog/v2"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
)

// captureStderr runs f with os.Stderr redirected and returns what was
// written to it.
func captureStderr(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	orig := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = orig }()
	out := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		out <- string(b)
	}()
	f()
	require.NoError(t, w.Close())
	return <-out
}

// emitLogs writes through every logger SilenceLogging redirects, including
// a client-go warning returned by an API server.
func emitLogs(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Warning", `299 - "client-go warning reached"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"gitVersion":"v1.35.1"}`))
	}))
	defer srv.Close()

	klog.Info("klog info reached")
	klog.Error("klog error reached")
	klog.Warning("klog warning reached")
	log.Print("stdlib log reached")
	slog.Warn("slog reached")
	ctrllog.Log.Info("controller-runtime reached")

	kube, err := kubernetes.NewForConfig(&rest.Config{Host: srv.URL})
	require.NoError(t, err)
	_, err = kube.Discovery().ServerVersion()
	require.NoError(t, err)
}

func TestSilenceLoggingToFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "dashboard.log")
	var restore func()
	stderr := captureStderr(t, func() {
		var err error
		restore, err = SilenceLogging(path)
		require.NoError(t, err)
		emitLogs(t)
	})
	restore()
	assert.Empty(t, stderr, "nothing reaches the terminal")

	b, err := os.ReadFile(path)
	require.NoError(t, err)
	for _, want := range []string{"klog info reached", "klog error reached", "klog warning reached", "stdlib log reached", "slog reached", "controller-runtime reached", "client-go warning reached"} {
		assert.Contains(t, string(b), want)
	}
}

func TestSilenceLoggingDiscard(t *testing.T) {
	stderr := captureStderr(t, func() {
		restore, err := SilenceLogging("")
		require.NoError(t, err)
		defer restore()
		emitLogs(t)
	})
	assert.Empty(t, stderr)

	// Once restored, klog writes to the terminal again, which also shows
	// that the capture works.
	stderr = captureStderr(t, func() { klog.Error("klog after restore") })
	assert.Contains(t, stderr, "klog after restore")
}

func TestSilenceLoggingBadFile(t *testing.T) {
	_, err := SilenceLogging(filepath.Join(t.TempDir(), "missing", "dashboard.log"))
	require.ErrorContains(t, err, "cannot open the log file")
}
