// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	"errors"
	"testing"

	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-operator/pkg/config"
)

type fakeAPIKeyCredentialsGetter struct {
	creds config.Creds
	err   error
	calls int
}

func (f *fakeAPIKeyCredentialsGetter) GetCredentialsForMetadata() (config.Creds, error) {
	f.calls++
	return f.creds, f.err
}

func TestSetupWithCredentials(t *testing.T) {
	tests := []struct {
		name          string
		getter        *fakeAPIKeyCredentialsGetter
		wantErr       string
		wantRCStarted bool
	}{
		{
			name:          "API key without App key starts Remote Configuration",
			getter:        &fakeAPIKeyCredentialsGetter{creds: config.Creds{APIKey: "api-key"}},
			wantRCStarted: true,
		},
		{
			name:          "API key and App key start Remote Configuration",
			getter:        &fakeAPIKeyCredentialsGetter{creds: config.Creds{APIKey: "api-key", AppKey: "app-key"}},
			wantRCStarted: true,
		},
		{
			name:    "credentials error is returned and Remote Configuration is not started",
			getter:  &fakeAPIKeyCredentialsGetter{err: errors.New("empty API key")},
			wantErr: "unable to get API key for Remote Configuration: empty API key",
		},
		{
			name:    "empty API key is rejected",
			getter:  &fakeAPIKeyCredentialsGetter{creds: config.Creds{AppKey: "app-key"}},
			wantErr: "error obtaining API key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TMPDIR", t.TempDir())
			t.Setenv("DD_SITE", "datadoghq.com")
			t.Setenv("DD_REMOTE_CONFIGURATION_RC_DD_URL", "https://config.datadoghq.com")

			originalNewRemoteConfigClient := newRemoteConfigClient
			t.Cleanup(func() {
				newRemoteConfigClient = originalNewRemoteConfigClient
			})
			newRemoteConfigClient = func(_ rcclient.ConfigFetcher, options ...func(*rcclient.Options)) (*rcclient.Client, error) {
				options = append(options, rcclient.WithoutTufVerification())
				return rcclient.NewClient(stoppedConfigFetcher{}, options...)
			}

			updater := NewRemoteConfigUpdater(nil, logr.Discard())
			err := updater.SetupWithCredentials(tt.getter)

			assert.Equal(t, 1, tt.getter.calls)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.wantRCStarted, updater.Client() != nil)
			if tt.wantRCStarted {
				require.NoError(t, updater.Stop())
			}
		})
	}
}

// *config.CredentialManager must satisfy APIKeyCredentialsGetter so cmd/main.go
// can pass it to SetupWithCredentials.
var _ APIKeyCredentialsGetter = (*config.CredentialManager)(nil)
