// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteflare

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	kubefake "k8s.io/client-go/kubernetes/fake"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/flare"
	"github.com/DataDog/datadog-operator/pkg/remoteconfig"
)

const (
	testAPIKey    = "3290abeefc68e1bbe852a25252bad88c"
	testAppKey    = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaabbbb"
	testToken     = "s3cr3t-T0ken-Value"
	testClusterID = "0f1e2d3c-cluster-uid"
	testNamespace = "datadog"
	testPodName   = "datadog-operator-abc-xyz"
)

type fakeCreds struct {
	apiKey string
	err    error
}

func (f fakeCreds) GetCredentialsForMetadata() (config.Creds, error) {
	return config.Creds{APIKey: f.apiKey, AppKey: testAppKey}, f.err
}

// intake is a fake flare intake.
type intake struct {
	server  *httptest.Server
	mu      sync.Mutex
	path    string
	apiKey  string
	fields  map[string][]string
	archive []byte
	status  int
}

func newIntake(t *testing.T) *intake {
	in := &intake{status: http.StatusOK}
	in.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		in.mu.Lock()
		defer in.mu.Unlock()
		in.path = r.URL.Path
		in.apiKey = r.Header.Get("Dd-Api-Key")
		require.NoError(t, r.ParseMultipartForm(64<<20))
		in.fields = r.MultipartForm.Value
		file, err := r.MultipartForm.File["flare_file"][0].Open()
		require.NoError(t, err)
		in.archive, _ = io.ReadAll(file)
		file.Close()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(in.status)
		if in.status == http.StatusOK {
			_, _ = w.Write([]byte(`{"case_id": 1234}`))
		} else {
			_, _ = w.Write([]byte(`{"error": "intake unavailable"}`))
		}
	}))
	t.Cleanup(in.server.Close)
	return in
}

// localEndpoints serves the Operator metrics and health probes.
func localEndpoints(t *testing.T) *httptest.Server {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/metrics":
			_, _ = w.Write([]byte("# HELP controller_runtime_reconcile_total Total reconciliations\ncontroller_runtime_reconcile_total{controller=\"datadogagent\"} 42\n"))
		case "/healthz":
			_, _ = w.Write([]byte("[+]ping ok\nhealthz check passed\n"))
		case "/readyz":
			_, _ = w.Write([]byte("[+]ping ok\nreadyz check passed\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func testObjects() []client.Object {
	labels := map[string]string{"app.kubernetes.io/name": "datadog-operator", "pod-template-hash": "abc"}
	return []client.Object{
		&corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system", UID: types.UID(testClusterID)}},
		&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: "datadog-operator", Namespace: testNamespace, Labels: map[string]string{"app.kubernetes.io/managed-by": "Helm"}}},
		&appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "datadog-operator-abc", Namespace: testNamespace, OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "datadog-operator"}}}},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: testPodName, Namespace: testNamespace, Labels: labels, OwnerReferences: []metav1.OwnerReference{{Kind: "ReplicaSet", Name: "datadog-operator-abc"}}},
			Spec: corev1.PodSpec{ServiceAccountName: "datadog-operator", Containers: []corev1.Container{{
				Name: "manager",
				Env:  []corev1.EnvVar{{Name: "DD_API_KEY", Value: testAPIKey}},
			}}},
			Status: corev1.PodStatus{Phase: corev1.PodRunning, ContainerStatuses: []corev1.ContainerStatus{{Name: "manager", RestartCount: 1}}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "datadog-operator-abc-replica", Namespace: testNamespace, Labels: labels},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "manager"}}},
		},
		// A pod in another namespace with the same labels: its logs are
		// never collected.
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "datadog-operator-other-ns", Namespace: "other", Labels: labels},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "manager"}}},
		},
		&coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{Name: "datadog-operator-lock", Namespace: testNamespace},
			Spec:       coordinationv1.LeaseSpec{HolderIdentity: ptr.To(testPodName + "_1234")},
		},
		&v2alpha1.DatadogAgent{
			ObjectMeta: metav1.ObjectMeta{Name: "datadog", Namespace: testNamespace},
			Spec: v2alpha1.DatadogAgentSpec{Global: &v2alpha1.GlobalConfig{
				Credentials:       &v2alpha1.DatadogCredentials{APIKey: ptr.To(testAPIKey), AppKey: ptr.To(testAppKey)},
				ClusterAgentToken: ptr.To(testToken),
			}},
		},
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{Name: "datadog-agent", Namespace: testNamespace, Labels: map[string]string{"app.kubernetes.io/managed-by": "datadog-operator"}},
			Spec:       appsv1.DaemonSetSpec{Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "agent"}}},
		},
	}
}

type testSetup struct {
	cfg      Config
	intake   *intake
	logPods  *[]string
	logMutex *sync.Mutex
}

func newTestSetup(t *testing.T) testSetup {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(s))
	require.NoError(t, apiextensionsv1.AddToScheme(s))
	require.NoError(t, v1alpha1.AddToScheme(s))
	require.NoError(t, v2alpha1.AddToScheme(s))
	objs := testObjects()
	reader := fake.NewClientBuilder().WithScheme(s).WithObjects(objs...).Build()
	var kubeObjs []runtime.Object
	for _, obj := range objs {
		if _, _, err := clientgoscheme.Scheme.ObjectKinds(obj); err == nil {
			kubeObjs = append(kubeObjs, obj.DeepCopyObject())
		}
	}
	clientset := kubefake.NewSimpleClientset(kubeObjs...)
	var logPods []string
	var logMutex sync.Mutex
	clientset.PrependReactor("get", "pods", func(action k8stesting.Action) (bool, runtime.Object, error) {
		if action.GetSubresource() != "log" {
			return false, nil, nil
		}
		logMutex.Lock()
		logPods = append(logPods, action.GetNamespace())
		logMutex.Unlock()
		return true, &runtime.Unknown{Raw: []byte(`{"level":"INFO","msg":"Starting","DD_API_KEY":"` + testAPIKey + `"}` + "\n")}, nil
	})

	in := newIntake(t)
	local := localEndpoints(t)
	addr := strings.TrimPrefix(local.URL, "http://")

	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	flags.String("metrics-addr", ":8080", "")
	flags.String("secretBackendConfig", `{"token":"`+testToken+`"}`, "")

	return testSetup{
		intake:   in,
		logPods:  &logPods,
		logMutex: &logMutex,
		cfg: Config{
			Reader:                reader,
			Clientset:             clientset,
			Credentials:           fakeCreds{apiKey: testAPIKey},
			Namespace:             testNamespace,
			PodName:               testPodName,
			ClusterName:           "my-cluster",
			FlareURL:              in.server.URL,
			MetricsAddr:           addr,
			HealthProbeAddr:       addr,
			Features:              map[string]bool{"datadogAgent": true, "datadogMonitor": false},
			LeaderElectionEnabled: true,
			LeaderElectionID:      "datadog-operator-lock",
			IsLeader:              func() bool { return true },
			RemoteConfigState: func() any {
				return remoteconfig.FlareState{Started: true, ClientID: "rc-client", UpdaterTags: []string{"cluster_id:" + testClusterID}, RemoteLogLevel: "debug"}
			},
			Flags: flags,
			Environ: func() []string {
				return []string{"DD_API_KEY=" + testAPIKey, "DD_SITE=datadoghq.eu", "DD_CLUSTER_AGENT_AUTH_TOKEN=" + testToken, "PATH=/usr/bin", "POD_NAME=" + testPodName}
			},
			UploadRetryDelay: time.Millisecond,
		},
	}
}

func testTask() remoteconfig.FlareTask {
	return remoteconfig.FlareTask{
		UUID:       "task-uuid-1",
		CaseID:     "98765",
		UserHandle: "user@example.com",
		Source:     "fleet-automation",
		Tags:       []string{"team:containers"},
	}
}

func unzip(t *testing.T, data []byte) map[string]string {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)
	files := map[string]string{}
	for _, f := range r.File {
		rc, err := f.Open()
		require.NoError(t, err)
		content, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = string(content)
	}
	return files
}

func TestRunnerEndToEnd(t *testing.T) {
	setup := newTestSetup(t)
	runner := NewRunner(setup.cfg, logr.Discard())

	require.NoError(t, runner.Run(context.Background(), testTask()))

	in := setup.intake
	assert.Equal(t, "/support/flare/98765", in.path)
	assert.Equal(t, testAPIKey, in.apiKey)
	assert.Equal(t, []string{"task-uuid-1"}, in.fields["rc_task_uuid"])
	assert.Equal(t, []string{"datadog-operator." + testClusterID}, in.fields["hostname"])
	assert.Equal(t, []string{"remote-config"}, in.fields["source"])
	assert.Equal(t, []string{"user@example.com"}, in.fields["email"])
	assert.Equal(t, []string{"98765"}, in.fields["case_id"])
	assert.Equal(t, []string{"fleet-automation"}, in.fields["flare_source"])
	assert.Equal(t, []string{"team:containers"}, in.fields["tags"])

	files := unzip(t, in.archive)
	root := "datadog-operator." + testClusterID + "/"
	for _, name := range []string{
		"manifest.json",
		"operator/version.yaml",
		"operator/config.yaml",
		"operator/leader-election.yaml",
		"operator/remote-config.yaml",
		"operator/health.txt",
		"operator/metrics.txt",
		"operator/pods.yaml",
		"operator/logs/" + testPodName + "/manager.log",
		"operator/logs/" + testPodName + "/manager.previous.log",
		"operator/logs/datadog-operator-abc-replica/manager.log",
		"operator/deployment.yaml",
		"operator/helm.yaml",
		"datadog/datadogagents/datadog/datadog.yaml",
		"workloads/daemonsets/datadog/datadog-agent.yaml",
		"config/crds.yaml",
		"cluster.yaml",
	} {
		assert.Contains(t, files, root+name)
	}
	for name := range files {
		assert.NotContains(t, name, "other-ns")
	}
	setup.logMutex.Lock()
	for _, ns := range *setup.logPods {
		assert.Equal(t, testNamespace, ns, "logs are only collected in the Operator namespace")
	}
	setup.logMutex.Unlock()

	assert.Contains(t, files[root+"operator/metrics.txt"], `controller_runtime_reconcile_total{controller="datadogagent"} 42`)
	assert.Contains(t, files[root+"operator/health.txt"], "healthz check passed")
	assert.Contains(t, files[root+"operator/health.txt"], "readyz check passed")
	cfg := files[root+"operator/config.yaml"]
	assert.Contains(t, cfg, "DD_SITE: datadoghq.eu")
	assert.Contains(t, cfg, "datadogAgent: true")
	assert.NotContains(t, cfg, "PATH")
	assert.Contains(t, files[root+"operator/leader-election.yaml"], "holderIdentity: "+testPodName+"_1234")
	assert.Contains(t, files[root+"operator/leader-election.yaml"], "isLeader: true")
	assert.Contains(t, files[root+"operator/remote-config.yaml"], "remoteLogLevel: debug")

	var manifest flare.Manifest
	require.NoError(t, json.Unmarshal([]byte(files[root+"manifest.json"]), &manifest))
	assert.Equal(t, "task-uuid-1", manifest.Info["rc_task_uuid"])
	for _, result := range manifest.Collectors {
		assert.Contains(t, []string{flare.StatusOK, flare.StatusPartial}, result.Status, "%s: %v", result.Name, result.Errors)
	}

	// No key or token anywhere in the archive.
	for name, content := range files {
		for _, secret := range []string{testAPIKey, testAppKey, testToken} {
			assert.NotContains(t, content, secret, "secret in %s", name)
		}
	}
}

func TestRunnerPartialFlare(t *testing.T) {
	setup := newTestSetup(t)
	setup.cfg.PodName = "unknown-pod"
	setup.cfg.MetricsAddr = "127.0.0.1:1"
	setup.cfg.RemoteConfigState = nil
	setup.cfg.Gatherer = prometheus.NewRegistry()
	require.NoError(t, NewRunner(setup.cfg, logr.Discard()).Run(context.Background(), testTask()))

	files := unzip(t, setup.intake.archive)
	var manifest flare.Manifest
	require.NoError(t, json.Unmarshal([]byte(files["datadog-operator."+testClusterID+"/manifest.json"]), &manifest))
	assert.False(t, manifest.Complete)
	assert.Subset(t, manifest.FailedCollector, []string{"operator/logs", "operator/deployment", "operator/remote-config", "operator/metrics"})
	assert.Contains(t, manifest.Info["pod_error"], "unknown-pod")
	// The other collectors still ran.
	assert.Contains(t, files, "datadog-operator."+testClusterID+"/datadog/datadogagents/datadog/datadog.yaml")
	// The metrics fall back to the in-process registry.
	assert.Contains(t, files["datadog-operator."+testClusterID+"/operator/metrics.txt"], "in-process")
}

func TestRunnerTimeLimitStillSendsFlare(t *testing.T) {
	setup := newTestSetup(t)
	setup.cfg.Timeout = time.Nanosecond
	require.NoError(t, NewRunner(setup.cfg, logr.Discard()).Run(context.Background(), testTask()))

	files := unzip(t, setup.intake.archive)
	var manifest flare.Manifest
	require.NoError(t, json.Unmarshal([]byte(files["datadog-operator."+testClusterID+"/manifest.json"]), &manifest))
	assert.False(t, manifest.Complete)
	for _, result := range manifest.Collectors {
		assert.Equal(t, flare.StatusSkipped, result.Status, result.Name)
	}
}

func TestRunnerSizeLimit(t *testing.T) {
	setup := newTestSetup(t)
	setup.cfg.MaxBytes = 600 * 1024
	archive, _, manifest := NewRunner(setup.cfg, logr.Discard()).Collect(context.Background(), testTask())
	require.NotNil(t, archive)
	assert.LessOrEqual(t, manifest.TotalBytes, int64(600*1024))
}

func TestRunnerSecureMetricsReadInProcess(t *testing.T) {
	setup := newTestSetup(t)
	setup.cfg.SecureMetrics = true
	registry := prometheus.NewRegistry()
	counter := prometheus.NewCounter(prometheus.CounterOpts{Name: "datadog_operator_test_total", Help: "test"})
	counter.Add(3)
	registry.MustRegister(counter)
	setup.cfg.Gatherer = registry
	data, _, _ := NewRunner(setup.cfg, logr.Discard()).Collect(context.Background(), testTask())
	metrics := unzip(t, data)["datadog-operator."+testClusterID+"/operator/metrics.txt"]
	assert.Contains(t, metrics, "datadog_operator_test_total 3")
	assert.Contains(t, metrics, "requires authentication")
}

func TestRunnerUploadFailures(t *testing.T) {
	setup := newTestSetup(t)
	setup.intake.status = http.StatusInternalServerError
	err := NewRunner(setup.cfg, logr.Discard()).Run(context.Background(), testTask())
	require.ErrorContains(t, err, "500")

	setup = newTestSetup(t)
	setup.cfg.Credentials = fakeCreds{err: errors.New("no secret")}
	err = NewRunner(setup.cfg, logr.Discard()).Run(context.Background(), testTask())
	require.ErrorContains(t, err, "API key")

	setup = newTestSetup(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = NewRunner(setup.cfg, logr.Discard()).Run(ctx, testTask())
	require.ErrorContains(t, err, "canceled")
}

func TestFlareURLFromSite(t *testing.T) {
	setup := newTestSetup(t)
	setup.cfg.FlareURL = ""
	setup.cfg.Site = "datadoghq.eu"
	setup.cfg.DDURL = ""
	// The intake isn't reachable in tests: check the error names the
	// versioned intake of the site.
	setup.cfg.HTTPClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("unreachable " + r.URL.Host)
	})}
	err := NewRunner(setup.cfg, logr.Discard()).Run(context.Background(), testTask())
	require.ErrorContains(t, err, "-flare.agent.datadoghq.eu")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestLocalURL(t *testing.T) {
	for addr, want := range map[string]string{
		":8080":          "http://127.0.0.1:8080",
		"0.0.0.0:8383":   "http://127.0.0.1:8383",
		"10.0.0.1:9090":  "http://10.0.0.1:9090",
		"[::]:8080":      "http://127.0.0.1:8080",
		"localhost:8080": "http://localhost:8080",
	} {
		got, err := localURL("http", addr)
		require.NoError(t, err)
		assert.Equal(t, want, got, addr)
	}
}
