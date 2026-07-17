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

	updater := NewRemoteConfigUpdater(nil, logr.Discard(), ManagedAgentInstallationIdentity{}, nil)
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
		rcClient:                         current,
		rcService:                        &rcservice.CoreAgentService{},
		managedAgentInstallationIdentity: validManagedAgentInstallationIdentity,
		managedAgentInstallationReadinessTags: func(context.Context) ([]string, error) {
			return []string{
				"managed_agent_installation_ack:" + acknowledgedOperationID,
				"operator_config_updates:ready",
			}, nil
		},
		logger: logr.Discard(),
		updaterTags: append(
			[]string{"updater_type:datadog-operator"},
			managedAgentInstallationIdentityUpdaterTags(t)...,
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
	assert.Equal(t, "stable-client-id", updater.GetClientID())
	assert.Equal(t, installerState, updater.GetInstallerState())
	assert.Same(t, updater, updater.Client())

	require.NoError(t, updater.RefreshUpdaterTags(context.Background()))
	assert.Equal(t, 1, createdClients, "unchanged updater tags must not replace the client")
}

func TestGetUpdaterTags(t *testing.T) {
	clusterUID := types.UID("test-cluster-uid")
	acknowledgedOperationID := "123e4567-e89b-42d3-a456-426614174010"

	tests := []struct {
		name          string
		clusterName   string
		identity      ManagedAgentInstallationIdentity
		objects       []client.Object
		readinessTags []string
		want          []string
	}{
		{
			name:        "with managed Agent installation identity",
			clusterName: "test-cluster",
			identity:    validManagedAgentInstallationIdentity,
			want: []string{
				"updater_type:datadog-operator",
				"cluster_name:test-cluster",
				"eks_installation_id:" + validManagedAgentInstallationIdentity.InstallationID(),
				"eks_arn_sha256:" + validManagedAgentInstallationTargetHash,
				"managed_agent_installation:eks-addon-config-v1",
			},
		},
		{
			name:        "with acknowledged managed Agent installation install",
			clusterName: "test-cluster",
			identity:    validManagedAgentInstallationIdentity,
			readinessTags: []string{
				"managed_agent_installation_ack:" + acknowledgedOperationID,
				"operator_config_updates:ready",
			},
			want: []string{
				"updater_type:datadog-operator",
				"cluster_name:test-cluster",
				"eks_installation_id:" + validManagedAgentInstallationIdentity.InstallationID(),
				"eks_arn_sha256:" + validManagedAgentInstallationTargetHash,
				"managed_agent_installation:eks-addon-config-v1",
				"managed_agent_installation_ack:" + acknowledgedOperationID,
				"operator_config_updates:ready",
			},
		},
		{
			name:        "without ready tag after uninstall intent",
			clusterName: "test-cluster",
			identity:    validManagedAgentInstallationIdentity,
			want: []string{
				"updater_type:datadog-operator",
				"cluster_name:test-cluster",
				"eks_installation_id:" + validManagedAgentInstallationIdentity.InstallationID(),
				"eks_arn_sha256:" + validManagedAgentInstallationTargetHash,
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
				kubeClient:                       newFakeClient(t, tt.objects...),
				logger:                           logr.Discard(),
				managedAgentInstallationIdentity: tt.identity,
				managedAgentInstallationReadinessTags: func(context.Context) ([]string, error) {
					return tt.readinessTags, nil
				},
				serviceConf: RcServiceConfiguration{
					clusterName: tt.clusterName,
				},
			}

			got, err := updater.getUpdaterTags(context.Background())
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRefreshUpdaterTagsPreservesCurrentClientWhenAcknowledgementEvidenceIsUnavailable(t *testing.T) {
	current, err := rcclient.NewClient(stoppedConfigFetcher{}, rcclient.WithoutTufVerification())
	require.NoError(t, err)
	t.Cleanup(current.Close)

	updater := &RemoteConfigUpdater{
		rcClient:                         current,
		rcService:                        &rcservice.CoreAgentService{},
		managedAgentInstallationIdentity: validManagedAgentInstallationIdentity,
		managedAgentInstallationReadinessTags: func(context.Context) ([]string, error) {
			return nil, errors.New("managed Agent installation acknowledgement state unavailable")
		},
		logger: logr.Discard(),
		updaterTags: append(
			[]string{"updater_type:datadog-operator"},
			managedAgentInstallationIdentityUpdaterTags(t)...,
		),
	}
	require.NoError(t, updater.configureService("api-key", "datadoghq.com", "", "", "", "https://config.datadoghq.com"))

	err = updater.RefreshUpdaterTags(context.Background())

	require.ErrorContains(t, err, "acknowledgement state unavailable")
	assert.Same(t, current, updater.rcClient)
}

func managedAgentInstallationIdentityUpdaterTags(t *testing.T) []string {
	t.Helper()
	tags, err := validManagedAgentInstallationIdentity.UpdaterTags()
	require.NoError(t, err)
	return tags
}

func newFakeClient(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()

	scheme := runtime.NewScheme()
	assert.NoError(t, corev1.AddToScheme(scheme))

	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).Build()
}
