// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-operator/pkg/constants"
)

func TestDelegatedAuthSettingsFromEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    *DelegatedAuthSettings
		wantErr bool
	}{
		{
			name: "not configured",
			env:  map[string]string{},
			want: nil,
		},
		{
			name: "org uuid only defaults to aws",
			env:  map[string]string{constants.DDDelegatedAuthOrgUUID: "org-1"},
			want: &DelegatedAuthSettings{Provider: "aws", OrgUUID: "org-1"},
		},
		{
			name: "explicit provider and region",
			env: map[string]string{
				constants.DDDelegatedAuthOrgUUID:   " org-1 ",
				constants.DDDelegatedAuthProvider:  "AWS",
				constants.DDDelegatedAuthAWSRegion: "eu-west-1",
			},
			want: &DelegatedAuthSettings{Provider: "aws", OrgUUID: "org-1", Region: "eu-west-1"},
		},
		{
			name: "provider without org uuid is ignored",
			env:  map[string]string{constants.DDDelegatedAuthProvider: "aws"},
			want: nil,
		},
		{
			name: "unsupported provider",
			env: map[string]string{
				constants.DDDelegatedAuthOrgUUID:  "org-1",
				constants.DDDelegatedAuthProvider: "gcp",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, k := range []string{constants.DDDelegatedAuthOrgUUID, constants.DDDelegatedAuthProvider, constants.DDDelegatedAuthAWSRegion} {
				t.Setenv(k, tt.env[k])
			}
			got, err := DelegatedAuthSettingsFromEnv()
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// fakeTokenProvider returns tokens with a configurable lifetime and counts calls.
type fakeTokenProvider struct {
	calls    atomic.Int32
	lifetime time.Duration
	now      func() time.Time
	err      error
}

func (f *fakeTokenProvider) Authenticate(_ context.Context, config *datadogapi.DelegatedTokenConfig) (*datadogapi.DelegatedTokenCredentials, error) {
	n := f.calls.Add(1)
	if f.err != nil {
		return nil, f.err
	}
	return &datadogapi.DelegatedTokenCredentials{
		OrgUUID:        config.OrgUUID,
		DelegatedToken: fmt.Sprintf("token-%d", n),
		Expiration:     f.now().Add(f.lifetime),
	}, nil
}

func newFakeTokenSource(provider *fakeTokenProvider, now func() time.Time) *delegatedTokenSource {
	return &delegatedTokenSource{
		config: &datadogapi.DelegatedTokenConfig{OrgUUID: "org-1", Provider: "aws", ProviderAuth: provider},
		now:    now,
	}
}

func TestDelegatedTokenSource_CachesAndRefreshes(t *testing.T) {
	current := time.Unix(1_700_000_000, 0)
	now := func() time.Time { return current }
	provider := &fakeTokenProvider{lifetime: 15 * time.Minute, now: now}
	source := newFakeTokenSource(provider, now)

	tok, err := source.Token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "token-1", tok.DelegatedToken)

	// Still valid: served from cache.
	current = current.Add(10 * time.Minute)
	tok, err = source.Token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "token-1", tok.DelegatedToken)
	assert.EqualValues(t, 1, provider.calls.Load())

	// Within the refresh margin: renewed.
	current = current.Add(4 * time.Minute)
	tok, err = source.Token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "token-2", tok.DelegatedToken)
	assert.EqualValues(t, 2, provider.calls.Load())

	// Callers get a copy, so mutating it doesn't affect the cache.
	tok.DelegatedToken = ""
	tok, err = source.Token(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "token-2", tok.DelegatedToken)
}

func TestDelegatedTokenSource_Error(t *testing.T) {
	provider := &fakeTokenProvider{err: errors.New("boom"), now: time.Now}
	source := newFakeTokenSource(provider, time.Now)
	_, err := source.Token(context.Background())
	require.Error(t, err)
}

func TestDelegatedTokenSource_Concurrent(t *testing.T) {
	provider := &fakeTokenProvider{lifetime: 15 * time.Minute, now: time.Now}
	source := newFakeTokenSource(provider, time.Now)

	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			tok, err := source.Token(context.Background())
			assert.NoError(t, err)
			assert.Equal(t, "token-1", tok.DelegatedToken)
		})
	}
	wg.Wait()
	assert.EqualValues(t, 1, provider.calls.Load())
}

func TestAWSDelegatedTokenProvider_Authenticate(t *testing.T) {
	tests := []struct {
		name           string
		configRegion   string
		sdkRegion      string
		wantSTSBaseURL string
	}{
		{name: "region from SDK", sdkRegion: "eu-west-1", wantSTSBaseURL: "https://sts.eu-west-1.amazonaws.com"},
		{name: "configured region wins", configRegion: "us-west-2", sdkRegion: "eu-west-1", wantSTSBaseURL: "https://sts.us-west-2.amazonaws.com"},
		{name: "global endpoint without region", wantSTSBaseURL: "https://sts.amazonaws.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/api/v2/delegated-token", r.URL.Path)
				authz, found := strings.CutPrefix(r.Header.Get("Authorization"), "Delegated ")
				if !assert.True(t, found, "missing Delegated authorization header") {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				parts := strings.Split(authz, "|")
				if !assert.Len(t, parts, 4) {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				stsURL, _ := base64.StdEncoding.DecodeString(parts[3])
				assert.Equal(t, tt.wantSTSBaseURL, string(stsURL))
				headersJSON, _ := base64.StdEncoding.DecodeString(parts[1])
				var headers map[string][]string
				assert.NoError(t, json.Unmarshal(headersJSON, &headers))
				assert.Equal(t, []string{"org-1"}, headers["x-ddog-org-id"])
				assert.Equal(t, []string{"session"}, headers["X-Amz-Security-Token"])

				_, _ = fmt.Fprintf(w, `{"data":{"attributes":{"access_token":"dd-token","expires":"%d"}}}`, time.Now().Add(time.Hour).Unix())
			}))
			defer server.Close()

			provider := &awsDelegatedTokenProvider{
				region: tt.configRegion,
				loadCredentials: func(context.Context) (aws.Credentials, string, error) {
					return aws.Credentials{AccessKeyID: "AKID", SecretAccessKey: "secret", SessionToken: "session"}, tt.sdkRegion, nil
				},
			}

			ctx := serverContext(t, server.URL)
			tok, err := provider.Authenticate(ctx, &datadogapi.DelegatedTokenConfig{OrgUUID: "org-1", Provider: "aws"})
			require.NoError(t, err)
			assert.Equal(t, "dd-token", tok.DelegatedToken)
			assert.True(t, tok.Expiration.After(time.Now().Add(30*time.Minute)))
		})
	}
}

func staticCredsLoader(context.Context) (aws.Credentials, string, error) {
	return aws.Credentials{AccessKeyID: "AKID", SecretAccessKey: "secret", SessionToken: "session"}, "", nil
}

func TestAWSDelegatedTokenProvider_ErrorDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"errors":[{"title":"Forbidden","detail":"no identity mapping found for arn:aws:sts::123:assumed-role/op"}]}`))
	}))
	defer server.Close()

	provider := &awsDelegatedTokenProvider{loadCredentials: staticCredsLoader}
	_, err := provider.Authenticate(serverContext(t, server.URL), &datadogapi.DelegatedTokenConfig{OrgUUID: "org-1"})
	require.ErrorContains(t, err, "403 Forbidden")
	require.ErrorContains(t, err, "no identity mapping found")
}

func TestAWSDelegatedTokenProvider_Timeout(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	defer close(release)

	provider := &awsDelegatedTokenProvider{
		loadCredentials: staticCredsLoader,
		httpClient:      &http.Client{Timeout: 100 * time.Millisecond},
	}
	start := time.Now()
	_, err := provider.Authenticate(serverContext(t, server.URL), &datadogapi.DelegatedTokenConfig{OrgUUID: "org-1"})
	require.Error(t, err)
	assert.Less(t, time.Since(start), 5*time.Second)
}

func TestAWSDelegatedTokenProvider_CredentialsError(t *testing.T) {
	provider := &awsDelegatedTokenProvider{
		loadCredentials: func(context.Context) (aws.Credentials, string, error) {
			return aws.Credentials{}, "", errors.New("no pod identity")
		},
	}
	_, err := provider.Authenticate(context.Background(), &datadogapi.DelegatedTokenConfig{OrgUUID: "org-1"})
	require.ErrorContains(t, err, "no pod identity")
}

func TestGetAuth_DelegatedAuth(t *testing.T) {
	t.Setenv(constants.DDAPIKey, "")
	t.Setenv(constants.DDAppKey, "")
	t.Setenv(constants.DDURL, "https://api.datadoghq.eu")

	cm := &CredentialManager{logger: logr.Discard()}
	require.NoError(t, cm.parseAPIURL())
	provider := &fakeTokenProvider{lifetime: 15 * time.Minute, now: time.Now}
	cm.delegatedAuth = newFakeTokenSource(provider, time.Now)

	assert.True(t, cm.DelegatedAuthEnabled())
	assert.NotNil(t, cm.DelegatedTokenConfig())

	ctx, err := cm.GetAuth()
	require.NoError(t, err)
	tok, ok := ctx.Value(datadogapi.ContextDelegatedToken).(*datadogapi.DelegatedTokenCredentials)
	require.True(t, ok)
	assert.Equal(t, "token-1", tok.DelegatedToken)
	assert.Nil(t, ctx.Value(datadogapi.ContextAPIKeys), "API/APP keys must not be required nor sent")
	assert.Equal(t, map[string]string{"name": "api.datadoghq.eu", "protocol": "https"}, ctx.Value(datadogapi.ContextServerVariables))

	// Second call reuses the cached token.
	_, err = cm.GetAuth()
	require.NoError(t, err)
	assert.EqualValues(t, 1, provider.calls.Load())
}

func TestGetAuth_DelegatedAuthError(t *testing.T) {
	cm := &CredentialManager{logger: logr.Discard()}
	cm.delegatedAuth = newFakeTokenSource(&fakeTokenProvider{err: errors.New("denied"), now: time.Now}, time.Now)
	_, err := cm.GetAuth()
	require.ErrorContains(t, err, "denied")
}

func TestFetchCredentials_AppKeyOptionalWithDelegatedAuth(t *testing.T) {
	t.Setenv(constants.DDAPIKey, "api")
	t.Setenv(constants.DDAppKey, "")

	cm := &CredentialManager{logger: logr.Discard()}
	_, err := cm.fetchCredentials()
	require.Error(t, err, "app key is required without delegated auth")

	cm.delegatedAuth = newFakeTokenSource(&fakeTokenProvider{now: time.Now}, time.Now)
	creds, err := cm.fetchCredentials()
	require.NoError(t, err)
	assert.Equal(t, Creds{APIKey: "api"}, creds)

	t.Setenv(constants.DDAPIKey, "")
	_, err = cm.fetchCredentials()
	require.Error(t, err, "api key is still required for intake")
}

func TestNewCredentialManager_DelegatedAuthFromEnv(t *testing.T) {
	t.Setenv(constants.DDDelegatedAuthOrgUUID, "org-1")
	t.Setenv(constants.DDDelegatedAuthProvider, "")
	cm := NewCredentialManager(nil)
	require.True(t, cm.DelegatedAuthEnabled())
	assert.Equal(t, "org-1", cm.DelegatedTokenConfig().OrgUUID)

	t.Setenv(constants.DDDelegatedAuthProvider, "azure")
	cm = NewCredentialManager(nil)
	assert.False(t, cm.DelegatedAuthEnabled(), "invalid configuration falls back to keys")
	assert.Nil(t, cm.DelegatedTokenConfig())
}

// serverContext returns a context routing Datadog API calls to rawURL.
func serverContext(t *testing.T, rawURL string) context.Context {
	t.Helper()
	scheme, host, found := strings.Cut(rawURL, "://")
	require.True(t, found)
	ctx := context.WithValue(context.Background(), datadogapi.ContextServerIndex, 1)
	return context.WithValue(ctx, datadogapi.ContextServerVariables, map[string]string{"name": host, "protocol": scheme})
}
