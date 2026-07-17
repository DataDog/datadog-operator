// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	rcclient "github.com/DataDog/datadog-agent/pkg/config/remote/client"
	rcservice "github.com/DataDog/datadog-agent/pkg/config/remote/service"
	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.etcd.io/bbolt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var testAdditionalUpdaterTags = []string{
	"eks_installation_id:123e4567-e89b-42d3-a456-426614174000",
	"eks_arn_sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
	"managed_agent_installation:eks-addon-config-v1",
}

func TestStartClosesServiceWhenClientCreationFails(t *testing.T) {
	t.Setenv("TMPDIR", t.TempDir())

	wantErr := errors.New("client creation failed")
	originalNewRemoteConfigClient := newRemoteConfigClient
	t.Cleanup(func() {
		newRemoteConfigClient = originalNewRemoteConfigClient
	})
	newRemoteConfigClient = func(_ rcclient.ConfigFetcher, _ ...func(*rcclient.Options)) (*rcclient.Client, error) {
		return nil, wantErr
	}

	updater := NewRemoteConfigUpdater(nil, logr.Discard())
	err := updater.Start("api-key", "datadoghq.com", "", "", "", "https://config.datadoghq.com")
	require.ErrorIs(t, err, wantErr)
	assert.Nil(t, updater.rcService)
	assert.Nil(t, updater.rcClient)

	databaseFiles, err := filepath.Glob(filepath.Join(updater.serviceConf.rcDatabaseDir, "remote-config-*.db"))
	require.NoError(t, err)
	require.Len(t, databaseFiles, 1)

	database, err := bbolt.Open(databaseFiles[0], 0600, &bbolt.Options{
		ReadOnly: true,
		Timeout:  100 * time.Millisecond,
	})
	require.NoError(t, err, "the failed setup attempt must release the database lock")
	require.NoError(t, database.Close())
}

func TestInitialInstallerConfigVersion(t *testing.T) {
	tests := []struct {
		name     string
		override string
		want     string
	}{
		{name: "standard Remote Config client", want: "empty"},
		{name: "overridden initial version", override: InstallerStateUnknownConfigVersion, want: InstallerStateUnknownConfigVersion},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			updater := &RemoteConfigUpdater{initialInstallerConfigVersionOverride: test.override}
			assert.Equal(t, test.want, updater.initialInstallerConfigVersion())
		})
	}
}

type stoppedConfigFetcher struct{}

func (stoppedConfigFetcher) ClientGetConfigs(context.Context, *pbgo.ClientGetConfigsRequest) (*pbgo.ClientGetConfigsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "stopped test client")
}

func TestRefreshUpdaterTagsPreservesClientIdentityAndInstallerState(t *testing.T) {
	acknowledgedOperationID := "123e4567-e89b-42d3-a456-426614174010"

	current, err := rcclient.NewClient(stoppedConfigFetcher{}, rcclient.WithoutTufVerification())
	require.NoError(t, err)
	current.ID = "stable-client-id"
	installerState := []*pbgo.PackageState{{
		Package:             "datadog-operator",
		StableConfigVersion: acknowledgedOperationID,
	}}
	current.SetInstallerState(installerState)

	updater := &RemoteConfigUpdater{
		rcClient:              current,
		rcService:             &rcservice.CoreAgentService{},
		additionalUpdaterTags: testAdditionalUpdaterTags,
		dynamicUpdaterTags: func(context.Context) ([]string, error) {
			return []string{
				"managed_agent_installation_ack:" + acknowledgedOperationID,
				"operator_config_updates:ready",
			}, nil
		},
		logger: logr.Discard(),
		updaterTags: append(
			[]string{"updater_type:datadog-operator"},
			testAdditionalUpdaterTags...,
		),
	}
	require.NoError(t, updater.configureService("api-key", "datadoghq.com", "", "", "", "https://config.datadoghq.com"))

	originalNewRemoteConfigClient := newRemoteConfigClient
	t.Cleanup(func() {
		newRemoteConfigClient = originalNewRemoteConfigClient
		updater.rcClient.Close()
	})
	createdClients := 0
	newRemoteConfigClient = func(_ rcclient.ConfigFetcher, options ...func(*rcclient.Options)) (*rcclient.Client, error) {
		createdClients++
		options = append(options, rcclient.WithoutTufVerification())
		return rcclient.NewClient(stoppedConfigFetcher{}, options...)
	}

	require.NoError(t, updater.RefreshUpdaterTags(context.Background()))
	assert.Equal(t, 1, createdClients)
	assert.NotSame(t, current, updater.rcClient)
	assert.Equal(t, "stable-client-id", updater.rcClient.ID)
	assert.Equal(t, installerState, updater.GetInstallerState())
	assert.Same(t, updater, updater.Client())

	require.NoError(t, updater.RefreshUpdaterTags(context.Background()))
	assert.Equal(t, 1, createdClients, "unchanged updater tags must not replace the client")
}

func TestRefreshUpdaterTagsWaitsForPreviousClientCallbacks(t *testing.T) {
	current, err := rcclient.NewClient(stoppedConfigFetcher{}, rcclient.WithoutTufVerification())
	require.NoError(t, err)

	updater := &RemoteConfigUpdater{
		rcClient:              current,
		rcService:             &rcservice.CoreAgentService{},
		additionalUpdaterTags: testAdditionalUpdaterTags,
		dynamicUpdaterTags: func(context.Context) ([]string, error) {
			return []string{"operator_config_updates:ready"}, nil
		},
		logger: logr.Discard(),
		updaterTags: append(
			[]string{"updater_type:datadog-operator"},
			testAdditionalUpdaterTags...,
		),
	}
	require.NoError(t, updater.configureService("api-key", "datadoghq.com", "", "", "", "https://config.datadoghq.com"))

	originalNewRemoteConfigClient := newRemoteConfigClient
	t.Cleanup(func() {
		newRemoteConfigClient = originalNewRemoteConfigClient
		if updater.rcClient != nil {
			updater.rcClient.Close()
		}
	})
	newRemoteConfigClient = func(_ rcclient.ConfigFetcher, options ...func(*rcclient.Options)) (*rcclient.Client, error) {
		options = append(options, rcclient.WithoutTufVerification())
		return rcclient.NewClient(stoppedConfigFetcher{}, options...)
	}

	callbackEntered := make(chan struct{})
	releaseCallback := make(chan struct{})
	updater.Subscribe("TEST_PRODUCT", func(map[string]state.RawConfig, func(string, state.ApplyStatus)) {
		close(callbackEntered)
		<-releaseCallback
	})
	go updater.subscriptions[0].callback(nil, func(string, state.ApplyStatus) {})
	<-callbackEntered

	refreshResult := make(chan error, 1)
	go func() {
		refreshResult <- updater.RefreshUpdaterTags(context.Background())
	}()
	select {
	case err := <-refreshResult:
		close(releaseCallback)
		require.Failf(t, "tag refresh returned before the previous callback completed", "error: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(releaseCallback)
	require.NoError(t, <-refreshResult)
}

func TestGetUpdaterTags(t *testing.T) {
	clusterUID := types.UID("test-cluster-uid")
	acknowledgedOperationID := "123e4567-e89b-42d3-a456-426614174010"

	tests := []struct {
		name           string
		clusterName    string
		additionalTags []string
		objects        []client.Object
		dynamicTags    []string
		want           []string
	}{
		{
			name:           "with additional updater tags",
			clusterName:    "test-cluster",
			additionalTags: testAdditionalUpdaterTags,
			want: []string{
				"updater_type:datadog-operator",
				"cluster_name:test-cluster",
				"eks_installation_id:123e4567-e89b-42d3-a456-426614174000",
				"eks_arn_sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"managed_agent_installation:eks-addon-config-v1",
			},
		},
		{
			name:           "with dynamic updater tags",
			clusterName:    "test-cluster",
			additionalTags: testAdditionalUpdaterTags,
			dynamicTags: []string{
				"managed_agent_installation_ack:" + acknowledgedOperationID,
				"operator_config_updates:ready",
			},
			want: []string{
				"updater_type:datadog-operator",
				"cluster_name:test-cluster",
				"eks_installation_id:123e4567-e89b-42d3-a456-426614174000",
				"eks_arn_sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"managed_agent_installation:eks-addon-config-v1",
				"managed_agent_installation_ack:" + acknowledgedOperationID,
				"operator_config_updates:ready",
			},
		},
		{
			name:           "without dynamic updater tags",
			clusterName:    "test-cluster",
			additionalTags: testAdditionalUpdaterTags,
			want: []string{
				"updater_type:datadog-operator",
				"cluster_name:test-cluster",
				"eks_installation_id:123e4567-e89b-42d3-a456-426614174000",
				"eks_arn_sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"managed_agent_installation:eks-addon-config-v1",
			},
		},
		{
			name:        "with cluster name and cluster uid",
			clusterName: "test-cluster",
			objects: []client.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "kube-system",
						UID:  clusterUID,
					},
				},
			},
			want: []string{
				"updater_type:datadog-operator",
				"cluster_name:test-cluster",
				"cluster_id:" + string(clusterUID),
			},
		},
		{
			name:        "without cluster uid",
			clusterName: "test-cluster",
			want: []string{
				"updater_type:datadog-operator",
				"cluster_name:test-cluster",
			},
		},
		{
			name: "without cluster name",
			objects: []client.Object{
				&corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{
						Name: "kube-system",
						UID:  clusterUID,
					},
				},
			},
			want: []string{
				"updater_type:datadog-operator",
				"cluster_id:" + string(clusterUID),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updater := &RemoteConfigUpdater{
				kubeClient:            newFakeClient(t, tt.objects...),
				logger:                logr.Discard(),
				additionalUpdaterTags: tt.additionalTags,
				dynamicUpdaterTags: func(context.Context) ([]string, error) {
					return tt.dynamicTags, nil
				},
				serviceConf: RcServiceConfiguration{
					clusterName: tt.clusterName,
				},
			}

			got, err := updater.getUpdaterTags(context.Background(), true)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestInitialUpdaterTagsExcludeDynamicTags(t *testing.T) {
	dynamicCalls := 0
	updater := &RemoteConfigUpdater{
		kubeClient:            newFakeClient(t),
		additionalUpdaterTags: testAdditionalUpdaterTags,
		dynamicUpdaterTags: func(context.Context) ([]string, error) {
			dynamicCalls++
			return []string{"operator_config_updates:ready"}, nil
		},
	}

	tags, err := updater.getUpdaterTags(context.Background(), false)

	require.NoError(t, err)
	assert.Zero(t, dynamicCalls)
	assert.NotContains(t, tags, "operator_config_updates:ready")
}

func TestRefreshUpdaterTagsPreservesCurrentClientWhenDynamicTagsAreUnavailable(t *testing.T) {
	current, err := rcclient.NewClient(stoppedConfigFetcher{}, rcclient.WithoutTufVerification())
	require.NoError(t, err)
	t.Cleanup(current.Close)

	updater := &RemoteConfigUpdater{
		rcClient:              current,
		rcService:             &rcservice.CoreAgentService{},
		additionalUpdaterTags: testAdditionalUpdaterTags,
		dynamicUpdaterTags: func(context.Context) ([]string, error) {
			return nil, errors.New("dynamic updater tags unavailable")
		},
		logger: logr.Discard(),
		updaterTags: append(
			[]string{"updater_type:datadog-operator"},
			testAdditionalUpdaterTags...,
		),
	}
	require.NoError(t, updater.configureService("api-key", "datadoghq.com", "", "", "", "https://config.datadoghq.com"))

	err = updater.RefreshUpdaterTags(context.Background())

	require.ErrorContains(t, err, "dynamic updater tags unavailable")
	assert.Same(t, current, updater.rcClient)
}

func newFakeClient(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	assert.NoError(t, corev1.AddToScheme(scheme))

	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
}
