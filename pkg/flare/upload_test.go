// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlareURL(t *testing.T) {
	tests := []struct {
		site, ddURL, version, want string
	}{
		{"", "", "1.22.0", "https://1-22-0-flare.agent.datadoghq.com"},
		{"datadoghq.eu", "", "v1.22.0-rc.1", "https://1-22-0-flare.agent.datadoghq.eu"},
		{"us5.datadoghq.com", "", "1.22.0", "https://1-22-0-flare.agent.us5.datadoghq.com"},
		{"ddog-gov.com", "", "1.22.0", "https://1-22-0-flare.agent.ddog-gov.com"},
		{"datadoghq.com", "", "dev", "https://0-0-0-flare.agent.datadoghq.com"},
		{"datadoghq.com", "https://api.datadoghq.eu", "1.22.0", "https://1-22-0-flare.agent.datadoghq.eu"},
		{"datadoghq.com", "http://fakeintake.local:8080/", "1.22.0", "http://fakeintake.local:8080"},
		{"datadoghq.com", "https://proxy.example.com/datadog?x=1", "1.22.0", "https://proxy.example.com/datadog"},
	}
	for _, tt := range tests {
		got, err := FlareURL(tt.site, tt.ddURL, tt.version)
		require.NoError(t, err)
		assert.Equal(t, tt.want, got, "site=%q ddURL=%q", tt.site, tt.ddURL)
	}
	_, err := FlareURL("", "not a url", "1.0.0")
	assert.Error(t, err)
}

// fakeIntake records the multipart form of the last flare it received.
type fakeIntake struct {
	server   *httptest.Server
	posts    atomic.Int32
	path     string
	apiKey   string
	fields   map[string][]string
	fileName string
	file     []byte
	status   int
	response string
}

func newFakeIntake(t *testing.T) *fakeIntake {
	f := &fakeIntake{status: http.StatusOK, response: `{"case_id": 1234}`}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		f.posts.Add(1)
		f.path = r.URL.Path
		f.apiKey = r.Header.Get("Dd-Api-Key")
		if err := r.ParseMultipartForm(64 << 20); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		f.fields = r.MultipartForm.Value
		if files := r.MultipartForm.File["flare_file"]; len(files) == 1 {
			f.fileName = files[0].Filename
			file, _ := files[0].Open()
			f.file, _ = io.ReadAll(file)
			file.Close()
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(f.response))
	}))
	t.Cleanup(f.server.Close)
	return f
}

func TestUploadRequestShape(t *testing.T) {
	intake := newFakeIntake(t)
	uploader := &Uploader{BaseURL: intake.server.URL, APIKey: testAPIKey}
	caseID, err := uploader.Upload(context.Background(), UploadRequest{
		CaseID:          "98765",
		Email:           "user@example.com",
		Hostname:        "datadog-operator.cluster-uid",
		Source:          SourceRemoteConfig,
		RCTaskUUID:      "task-uuid",
		FlareSource:     "fleet",
		Tags:            []string{"a:b", "c:d"},
		OperatorVersion: "1.22.0",
		ArchiveName:     "flare.zip",
		Archive:         []byte("zip-bytes"),
	})
	require.NoError(t, err)
	assert.Equal(t, "1234", caseID)
	assert.Equal(t, "/support/flare/98765", intake.path)
	assert.Equal(t, testAPIKey, intake.apiKey)
	assert.Equal(t, map[string][]string{
		"case_id":          {"98765"},
		"email":            {"user@example.com"},
		"source":           {"remote-config"},
		"rc_task_uuid":     {"task-uuid"},
		"flare_source":     {"fleet"},
		"tags":             {"a:b", "c:d"},
		"operator_version": {"1.22.0"},
		"hostname":         {"datadog-operator.cluster-uid"},
	}, intake.fields)
	assert.Equal(t, "flare.zip", intake.fileName)
	assert.Equal(t, []byte("zip-bytes"), intake.file)
}

func TestUploadErrors(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		response string
		wantErr  string
	}{
		{name: "forbidden", status: http.StatusForbidden, response: `{}`, wantErr: "API key"},
		{name: "server error", status: http.StatusInternalServerError, response: `oops`, wantErr: "500"},
		{name: "intake error", status: http.StatusOK, response: `{"error": "invalid case", "request_uuid": "r-1"}`, wantErr: "invalid case"},
		{name: "invalid json", status: http.StatusOK, response: `not json`, wantErr: "unable to read"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intake := newFakeIntake(t)
			intake.status, intake.response = tt.status, tt.response
			uploader := &Uploader{BaseURL: intake.server.URL, APIKey: testAPIKey}
			_, err := uploader.Upload(context.Background(), UploadRequest{Hostname: "h", Archive: []byte("x")})
			require.ErrorContains(t, err, tt.wantErr)
			assert.NotContains(t, err.Error(), testAPIKey)
			// A POST that reached the intake is never retried.
			assert.Equal(t, int32(1), intake.posts.Load())
		})
	}

	_, err := (&Uploader{BaseURL: "http://127.0.0.1:1"}).Upload(context.Background(), UploadRequest{})
	require.ErrorContains(t, err, "no API key")
}

func TestUploadRetriesConnectionFailures(t *testing.T) {
	// The HEAD request succeeds, then the intake is unreachable.
	intake := newFakeIntake(t)
	failing := &http.Client{Transport: &failAfterHead{base: http.DefaultTransport}}
	uploader := &Uploader{BaseURL: intake.server.URL, APIKey: testAPIKey, HTTPClient: failing, RetryDelay: 1}
	_, err := uploader.Upload(context.Background(), UploadRequest{Hostname: "h", Archive: []byte("x")})
	require.Error(t, err)
	assert.Equal(t, int32(uploadAttempts), failing.Transport.(*failAfterHead).posts.Load())
}

type failAfterHead struct {
	base  http.RoundTripper
	posts atomic.Int32
}

func (f *failAfterHead) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method == http.MethodHead {
		return f.base.RoundTrip(r)
	}
	f.posts.Add(1)
	r.URL.Host = "127.0.0.1:1"
	return f.base.RoundTrip(r)
}

func TestUploadResolveErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	_, err := (&Uploader{BaseURL: server.URL + "?api_key=secret", APIKey: testAPIKey}).Upload(context.Background(), UploadRequest{})
	require.ErrorContains(t, err, "unable to reach the flare intake")
	assert.NotContains(t, err.Error(), "secret")

	_, err = (&Uploader{BaseURL: "http://127.0.0.1:1", APIKey: testAPIKey}).Upload(context.Background(), UploadRequest{})
	require.ErrorContains(t, err, "unable to reach the flare intake")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = (&Uploader{BaseURL: server.URL, APIKey: testAPIKey}).Upload(ctx, UploadRequest{})
	require.Error(t, err)
}

func TestUploadUnexpectedContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(strings.Repeat("x", 300)))
	}))
	defer server.Close()
	_, err := (&Uploader{BaseURL: server.URL, APIKey: testAPIKey}).Upload(context.Background(), UploadRequest{Hostname: "h"})
	require.ErrorContains(t, err, "unexpected content type")
}

func TestIsRetryableUploadError(t *testing.T) {
	assert.True(t, isRetryableUploadError(&net.DNSError{Err: "no such host"}))
	assert.True(t, isRetryableUploadError(&net.OpError{Op: "dial", Err: errors.New("refused")}))
	assert.False(t, isRetryableUploadError(&net.OpError{Op: "read", Err: errors.New("reset")}))
	assert.True(t, isRetryableUploadError(errors.New("net/http: TLS handshake timeout")))
	assert.False(t, isRetryableUploadError(errors.New("other")))
	assert.Equal(t, "https://x/y", stripQuery("https://x/y?api_key=1"))
	assert.Equal(t, "%zz", stripQuery("%zz"))
}
