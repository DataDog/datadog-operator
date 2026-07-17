// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"encoding/json"
	"testing"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

const testManagedAgentInstallationTargetHash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

var testManagedAgentInstallationIdentity = NewEKSManagedAgentInstallationIdentity(
	"123e4567-e89b-42d3-a456-426614174000",
	testManagedAgentInstallationTargetHash,
)

type managedAgentInstallationTestClient struct {
	client.Client
}

func (c *managedAgentInstallationTestClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if dda, ok := obj.(*v2alpha1.DatadogAgent); ok && dda.UID == "" {
		dda.UID = types.UID("created-dda-uid")
	}
	return c.Client.Create(ctx, obj, opts...)
}

func (c *managedAgentInstallationTestClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if _, ok := obj.(*v2alpha1.DatadogAgent); ok {
		profile := &v1alpha1.DatadogAgentProfile{}
		if err := c.Client.Get(ctx, managedAgentInstallationWindowsProfileKey, profile); err == nil {
			if err := c.Client.Delete(ctx, profile); err != nil && !apierrors.IsNotFound(err) {
				return err
			}
		} else if !apierrors.IsNotFound(err) {
			return err
		}
	}
	return c.Client.Delete(ctx, obj, opts...)
}

func TestManagedAgentInstallationConfigRejectsCredentials(t *testing.T) {
	for _, raw := range []json.RawMessage{
		json.RawMessage(`{"spec":{"global":{"credentials":{"apiKey":"api-key"}}}}`),
		json.RawMessage(`{"spec":{"global":{"credentials":null}}}`),
		json.RawMessage(`{"spec":{"global":{"clusterAgentToken":"token"}}}`),
		json.RawMessage(`{"spec":{"global":{"clusterAgentTokenSecret":{"secretName":"other"}}}}`),
	} {
		_, err := buildFleetDatadogAgentSpec(raw)
		assert.Error(t, err)
	}
}

func TestManagedAgentInstallationConfigAllowsAgentConfiguration(t *testing.T) {
	for name, raw := range map[string]string{
		"component image": `{"spec":{"override":{"nodeAgent":{"image":{"name":"registry.example/datadog-agent:latest"}}}}}`,
		"global registry": `{"spec":{"global":{"registry":"registry.example"}}}`,
		"service account": `{"spec":{"override":{"nodeAgent":{"serviceAccountName":"datadog-agent"}}}}`,
		"node selector":   `{"spec":{"override":{"nodeAgent":{"nodeSelector":{"kubernetes.io/os":"linux"}}}}}`,
		"OTLP endpoint":   `{"spec":{"features":{"otlp":{"receiver":{"protocols":{"grpc":{"endpoint":"0.0.0.0:4317"}}}}}}}`,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := buildFleetDatadogAgentSpec(json.RawMessage(raw))
			require.NoError(t, err)
		})
	}
}

func TestManagedAgentInstallationConfigRejectsInvalidDocument(t *testing.T) {
	for _, raw := range []json.RawMessage{
		json.RawMessage(`{"spec":null}`),
		json.RawMessage(`{"spec":{"unknownField":true}}`),
		json.RawMessage(`{"spec":{}} {}`),
	} {
		_, err := buildFleetDatadogAgentSpec(raw)
		assert.Error(t, err)
	}
}

func TestManagedAgentInstallationResourcesAbsentUsesOwnedInventory(t *testing.T) {
	ddai := &v1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{
		Namespace: fleetDatadogAgentNamespace,
		Name:      "datadog-agent-internal",
		Labels: map[string]string{
			fleetManagedAgentInstallationProviderLabel: string(testManagedAgentInstallationIdentity.Provider()),
			fleetInstallationIDLabel:                   testManagedAgentInstallationIdentity.InstallationID(),
			fleetTargetIDLabel:                         testManagedAgentInstallationIdentity.TargetID(),
		},
	}}
	daemon, kubeClient, _ := testManagedAgentInstallationDaemon(nil, ddai)

	absent, err := daemon.managedAgentInstallationResourcesAbsent(context.Background(), managedAgentInstallationTarget, "")
	require.NoError(t, err)
	assert.False(t, absent)

	require.NoError(t, kubeClient.Delete(context.Background(), ddai))
	absent, err = daemon.managedAgentInstallationResourcesAbsent(context.Background(), managedAgentInstallationTarget, "")
	require.NoError(t, err)
	assert.True(t, absent)
}

func TestManagedAgentInstallationResourcesAbsentRejectsReplacement(t *testing.T) {
	replacement := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{
		Namespace: managedAgentInstallationTarget.Namespace,
		Name:      managedAgentInstallationTarget.Name,
		UID:       types.UID("replacement-uid"),
	}}
	daemon, _, _ := testManagedAgentInstallationDaemon(nil, replacement)

	absent, err := daemon.managedAgentInstallationResourcesAbsent(
		context.Background(), managedAgentInstallationTarget, types.UID("deleted-uid"),
	)

	assert.False(t, absent)
	require.ErrorContains(t, err, "was recreated while uninstalling")
}

func testManagedAgentInstallationDaemon(rcState []*pbgo.PackageState, objects ...client.Object) (*Daemon, client.Client, *mockRCClient) {
	scheme := testFleetScheme()
	_ = corev1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)
	_ = apiregistrationv1.AddToScheme(scheme)
	_ = v1alpha1.AddToScheme(scheme)
	baseClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&v2alpha1.DatadogAgent{}).
		WithObjects(objects...).
		Build()
	kubeClient := &managedAgentInstallationTestClient{Client: baseClient}
	rcClient := &mockRCClient{state: rcState}
	daemon := &Daemon{
		rcClient:                             rcClient,
		client:                               kubeClient,
		apiReader:                            kubeClient,
		managedAgentInstallationIdentity:     testManagedAgentInstallationIdentity,
		managedAgentInstallationTaskReserved: true,
		configs:                              make(map[string]installerConfig),
		statusUpdates:                        make(chan ddaStatusSnapshot, 32),
	}
	return daemon, kubeClient, rcClient
}

func testFleetCredentialSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: fleetCredentialSecretName, Namespace: fleetDatadogAgentNamespace},
		Data:       map[string][]byte{fleetCredentialAPIKey: []byte("api-key-value")},
	}
}
