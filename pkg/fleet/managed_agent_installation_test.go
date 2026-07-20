// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

const testManagedAgentInstallationTargetHash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

var testManagedAgentInstallationIdentity = NewEKSManagedAgentInstallationIdentity(
	"123e4567-e89b-42d3-a456-426614174000",
	testManagedAgentInstallationTargetHash,
)

type managedAgentInstallationTestClient struct {
	client.Client
}

type rejectManagedAgentInstallationTargetCreateClient struct {
	client.Client
}

func (c *rejectManagedAgentInstallationTargetCreateClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if _, ok := obj.(*v2alpha1.DatadogAgent); ok {
		return apierrors.NewForbidden(
			schema.GroupResource{Group: v2alpha1.GroupVersion.Group, Resource: "datadogagents"},
			obj.GetName(),
			errors.New("target create denied"),
		)
	}
	return c.Client.Create(ctx, obj, opts...)
}

type recoverManagedAgentInstallationAlreadyExistsClient struct {
	client.Client
}

func (c *recoverManagedAgentInstallationAlreadyExistsClient) Create(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	if _, ok := obj.(*v2alpha1.DatadogAgent); !ok {
		return c.Client.Create(ctx, obj, opts...)
	}
	if err := c.Client.Create(ctx, obj, opts...); err != nil {
		return err
	}
	return apierrors.NewAlreadyExists(schema.GroupResource{Group: v2alpha1.GroupVersion.Group, Resource: "datadogagents"}, obj.GetName())
}

type rejectManagedAgentInstallationTargetDeleteClient struct {
	client.Client
}

func (c *rejectManagedAgentInstallationTargetDeleteClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	if _, ok := obj.(*v2alpha1.DatadogAgent); ok {
		return errors.New("target delete denied")
	}
	return c.Client.Delete(ctx, obj, opts...)
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

func TestManagedAgentInstallationRevalidatesExistingResources(t *testing.T) {
	ctx := context.Background()
	daemon, kubeClient, _ := testManagedAgentInstallationDaemon(nil, testFleetCredentialSecret())
	command := testManagedAgentInstallationCommand(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)

	pending, err := daemon.installDatadogAgent(ctx, command)
	requireSyncNoError(t, pending, err)
	pending, err = daemon.installDatadogAgent(ctx, command)
	requireSyncNoError(t, pending, err)

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, kubeClient.Get(ctx, managedAgentInstallationTarget, dda))
	assert.Equal(t, fleetManagedAgentInstallationStateReady, dda.Labels[fleetManagedAgentInstallationStateLabel])
	profile := &v1alpha1.DatadogAgentProfile{}
	require.NoError(t, kubeClient.Get(ctx, managedAgentInstallationWindowsProfileKey, profile))
	require.NoError(t, daemon.validateManagedAgentInstallationWindowsProfile(profile, dda))
}

func TestManagedAgentInstallationInstallFailureModes(t *testing.T) {
	ctx := context.Background()
	command := testManagedAgentInstallationCommand(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)

	t.Run("invalid config", func(t *testing.T) {
		daemon, _, _ := testManagedAgentInstallationDaemon(nil, testFleetCredentialSecret())
		invalid := command
		invalid.Config = json.RawMessage(`{}`)
		_, err := daemon.installDatadogAgent(ctx, invalid)
		require.ErrorContains(t, err, "config must contain spec")
	})

	t.Run("unmanaged target", func(t *testing.T) {
		daemon, _, _ := testManagedAgentInstallationDaemon(nil, testFleetCredentialSecret(), testDDAObject(""))
		_, err := daemon.installDatadogAgent(ctx, command)
		require.ErrorContains(t, err, "is not owned by Fleet Automation")
	})

	t.Run("different accepted config", func(t *testing.T) {
		dda := testFleetManagedDatadogAgent(t, "", testAddonUninstallOperationID)
		daemon, _, _ := testManagedAgentInstallationDaemon(nil, testFleetCredentialSecret(), dda)
		_, err := daemon.installDatadogAgent(ctx, command)
		require.ErrorContains(t, err, "use an update operation")
	})

	t.Run("target read failure", func(t *testing.T) {
		daemon, kubeClient, _ := testManagedAgentInstallationDaemon(nil, testFleetCredentialSecret())
		daemon.apiReader = &failManagedAgentInstallationTargetReadClient{Reader: kubeClient}
		_, err := daemon.installDatadogAgent(ctx, command)
		require.ErrorContains(t, err, "transient managed Agent installation target read failure")
	})

	t.Run("non-retryable create failure", func(t *testing.T) {
		daemon, _, _ := testManagedAgentInstallationDaemon(nil, testFleetCredentialSecret())
		daemon.client = &rejectManagedAgentInstallationTargetCreateClient{Client: daemon.client}
		_, err := daemon.installDatadogAgent(ctx, command)
		require.ErrorContains(t, err, "target create denied")
	})

	t.Run("already-created target is recovered as partial", func(t *testing.T) {
		daemon, kubeClient, rcClient := testManagedAgentInstallationDaemon(
			[]*pbgo.PackageState{{Package: packageDatadogOperator}},
			testFleetCredentialSecret(),
		)
		daemon.client = &recoverManagedAgentInstallationAlreadyExistsClient{Client: daemon.client}
		_, err := daemon.installDatadogAgent(ctx, command)
		require.ErrorContains(t, err, "recovered Fleet-owned resource remains partial")
		dda := &v2alpha1.DatadogAgent{}
		require.NoError(t, kubeClient.Get(ctx, managedAgentInstallationTarget, dda))
		assert.Equal(t, fleetManagedAgentInstallationStatePartial, dda.Labels[fleetManagedAgentInstallationStateLabel])
		assert.Equal(t, fleetPartialConfigVersionPrefix+testAddonInstallOperationID, rcClient.state[0].GetStableConfigVersion())
	})
}

func TestManagedAgentInstallationUninstallRejectsInvalidTargets(t *testing.T) {
	ctx := context.Background()

	t.Run("unmanaged target", func(t *testing.T) {
		daemon, _, _ := testManagedAgentInstallationDaemon(nil, testDDAObject(""))
		_, err := daemon.uninstallDatadogAgent(ctx)
		require.ErrorContains(t, err, "is not owned by Fleet Automation")
	})

	t.Run("different managed installation", func(t *testing.T) {
		dda := testFleetManagedDatadogAgent(t, "", testAddonInstallOperationID)
		dda.Labels[fleetInstallationIDLabel] = "other-installation"
		daemon, _, _ := testManagedAgentInstallationDaemon(nil, dda)
		_, err := daemon.uninstallDatadogAgent(ctx)
		require.ErrorContains(t, err, "belongs to a different managed installation")
	})

	t.Run("target read failure", func(t *testing.T) {
		daemon, kubeClient, _ := testManagedAgentInstallationDaemon(nil)
		daemon.apiReader = &failManagedAgentInstallationTargetReadClient{Reader: kubeClient}
		_, err := daemon.uninstallDatadogAgent(ctx)
		require.ErrorContains(t, err, "transient managed Agent installation target read failure")
	})

	t.Run("delete failure", func(t *testing.T) {
		dda := testFleetManagedDatadogAgent(t, "", testAddonInstallOperationID)
		daemon, _, _ := testManagedAgentInstallationDaemon(nil, dda)
		daemon.client = &rejectManagedAgentInstallationTargetDeleteClient{Client: daemon.client}
		_, err := daemon.uninstallDatadogAgent(ctx)
		require.ErrorContains(t, err, "target delete denied")
	})
}

func TestValidateFleetDatadogAgentInstallReplay(t *testing.T) {
	command := testManagedAgentInstallationCommand(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
	desired, err := buildFleetDatadogAgentSpec(command.Config)
	require.NoError(t, err)
	valid := testFleetManagedDatadogAgent(t, "", testAddonInstallOperationID)
	valid.Spec = *desired.DeepCopy()
	hash, err := fleetDatadogAgentSpecHash(&valid.Spec)
	require.NoError(t, err)
	valid.Annotations[fleetConfigHashAnnotation] = hash
	require.NoError(t, validateFleetDatadogAgentInstallReplay(valid, testAddonInstallOperationID, desired))

	for _, test := range []struct {
		name      string
		mutate    func(*v2alpha1.DatadogAgent)
		wantError string
	}{
		{
			name: "different config",
			mutate: func(dda *v2alpha1.DatadogAgent) {
				dda.Labels[fleetConfigIDLabel] = testAddonUninstallOperationID
			},
			wantError: "use an update operation",
		},
		{
			name: "active experiment",
			mutate: func(dda *v2alpha1.DatadogAgent) {
				dda.Status.Experiment = &v2alpha1.ExperimentStatus{ID: testExperimentID, Phase: v2alpha1.ExperimentPhaseRunning}
			},
			wantError: "active Fleet experiment",
		},
		{
			name: "terminating",
			mutate: func(dda *v2alpha1.DatadogAgent) {
				now := metav1.Now()
				dda.DeletionTimestamp = &now
			},
			wantError: "is terminating",
		},
		{
			name: "invalid credentials",
			mutate: func(dda *v2alpha1.DatadogAgent) {
				dda.Spec.Global.Credentials.APISecret.SecretName = "other-secret"
			},
			wantError: "does not use the Fleet-managed API Secret",
		},
		{
			name: "different spec",
			mutate: func(dda *v2alpha1.DatadogAgent) {
				site := "datadoghq.eu"
				dda.Spec.Global.Site = &site
			},
			wantError: "spec does not match Fleet config",
		},
		{
			name: "stale accepted hash",
			mutate: func(dda *v2alpha1.DatadogAgent) {
				dda.Annotations[fleetConfigHashAnnotation] = "stale"
			},
			wantError: "spec changed after Fleet config",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			dda := valid.DeepCopy()
			test.mutate(dda)
			require.ErrorContains(t, validateFleetDatadogAgentInstallReplay(dda, testAddonInstallOperationID, desired), test.wantError)
		})
	}
}

func TestValidateFleetDatadogAgentInstallCompletion(t *testing.T) {
	command := testManagedAgentInstallationCommand(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
	spec, err := buildFleetDatadogAgentSpec(command.Config)
	require.NoError(t, err)
	valid := testFleetManagedDatadogAgent(t, "", testAddonInstallOperationID)
	valid.Spec = *spec
	hash, err := fleetDatadogAgentSpecHash(&valid.Spec)
	require.NoError(t, err)
	valid.Annotations[fleetConfigHashAnnotation] = hash
	require.NoError(t, validateFleetDatadogAgentInstallCompletion(valid, valid.UID, testAddonInstallOperationID))

	require.ErrorContains(t, validateFleetDatadogAgentInstallCompletion(nil, valid.UID, testAddonInstallOperationID), "disappeared")
	require.ErrorContains(t, validateFleetDatadogAgentInstallCompletion(valid, "replacement", testAddonInstallOperationID), "was replaced")

	wrongConfig := valid.DeepCopy()
	wrongConfig.Labels[fleetConfigIDLabel] = testAddonUninstallOperationID
	require.ErrorContains(t, validateFleetDatadogAgentInstallCompletion(wrongConfig, wrongConfig.UID, testAddonInstallOperationID), "use an update operation")

	partial := valid.DeepCopy()
	partial.Labels[fleetManagedAgentInstallationStateLabel] = fleetManagedAgentInstallationStatePartial
	require.ErrorContains(t, validateFleetDatadogAgentInstallCompletion(partial, partial.UID, testAddonInstallOperationID), "not ready at install completion")
}

func TestPersistManagedAgentInstallationStableConfigValidatesPromotion(t *testing.T) {
	ctx := context.Background()

	t.Run("persists and accepts replay", func(t *testing.T) {
		dda := testFleetManagedDatadogAgent(t, v2alpha1.ExperimentPhasePromoted, testAddonInstallOperationID)
		daemon, kubeClient, _ := testManagedAgentInstallationDaemon(nil, dda)
		require.NoError(t, daemon.persistManagedAgentInstallationStableConfig(ctx, managedAgentInstallationTarget, testExperimentID, "promoted-config"))
		require.NoError(t, daemon.persistManagedAgentInstallationStableConfig(ctx, managedAgentInstallationTarget, testExperimentID, "promoted-config"))
		updated := &v2alpha1.DatadogAgent{}
		require.NoError(t, kubeClient.Get(ctx, managedAgentInstallationTarget, updated))
		assert.Equal(t, "promoted-config", updated.Labels[fleetConfigIDLabel])
		hash, err := fleetDatadogAgentSpecHash(&updated.Spec)
		require.NoError(t, err)
		assert.Equal(t, hash, updated.Annotations[fleetConfigHashAnnotation])
	})

	t.Run("requires complete operation identity", func(t *testing.T) {
		daemon, _, _ := testManagedAgentInstallationDaemon(nil)
		require.ErrorContains(t, daemon.persistManagedAgentInstallationStableConfig(ctx, managedAgentInstallationTarget, "", "promoted-config"), "is incomplete")
		require.ErrorContains(t, daemon.persistManagedAgentInstallationStableConfig(ctx, managedAgentInstallationTarget, testExperimentID, ""), "is incomplete")
	})

	t.Run("requires target", func(t *testing.T) {
		daemon, _, _ := testManagedAgentInstallationDaemon(nil)
		require.Error(t, daemon.persistManagedAgentInstallationStableConfig(ctx, managedAgentInstallationTarget, testExperimentID, "promoted-config"))
	})

	t.Run("requires matching managed installation", func(t *testing.T) {
		dda := testFleetManagedDatadogAgent(t, v2alpha1.ExperimentPhasePromoted, testAddonInstallOperationID)
		dda.Labels[fleetTargetIDLabel] = "other-target"
		daemon, _, _ := testManagedAgentInstallationDaemon(nil, dda)
		require.ErrorContains(t, daemon.persistManagedAgentInstallationStableConfig(ctx, managedAgentInstallationTarget, testExperimentID, "promoted-config"), "belongs to a different managed target")
	})

	t.Run("requires completed install gate", func(t *testing.T) {
		dda := testFleetManagedDatadogAgent(t, v2alpha1.ExperimentPhasePromoted, testAddonInstallOperationID)
		dda.Labels[fleetManagedAgentInstallationStateLabel] = fleetManagedAgentInstallationStatePartial
		daemon, _, _ := testManagedAgentInstallationDaemon(nil, dda)
		require.ErrorContains(t, daemon.persistManagedAgentInstallationStableConfig(ctx, managedAgentInstallationTarget, testExperimentID, "promoted-config"), "install gate")
	})

	t.Run("requires matching promoted experiment", func(t *testing.T) {
		dda := testFleetManagedDatadogAgent(t, v2alpha1.ExperimentPhasePromoted, testAddonInstallOperationID)
		daemon, _, _ := testManagedAgentInstallationDaemon(nil, dda)
		require.ErrorContains(t, daemon.persistManagedAgentInstallationStableConfig(ctx, managedAgentInstallationTarget, "other-experiment", "promoted-config"), "has not promoted experiment")
	})

	t.Run("legacy Fleet target is unchanged", func(t *testing.T) {
		daemon, _, _ := testManagedAgentInstallationDaemon(nil)
		daemon.managedAgentInstallationIdentity = ManagedAgentInstallationIdentity{}
		require.NoError(t, daemon.persistManagedAgentInstallationStableConfig(ctx, managedAgentInstallationTarget, "", ""))
	})
}

func TestManagedAgentInstallationMetadataMutationsRejectConflicts(t *testing.T) {
	ctx := context.Background()
	base := testFleetManagedDatadogAgent(t, "", testAddonInstallOperationID)
	hash, err := fleetDatadogAgentSpecHash(&base.Spec)
	require.NoError(t, err)

	t.Run("record accepted hash", func(t *testing.T) {
		dda := base.DeepCopy()
		delete(dda.Annotations, fleetConfigHashAnnotation)
		daemon, kubeClient, _ := testManagedAgentInstallationDaemon(nil, dda)
		require.NoError(t, daemon.recordFleetDatadogAgentSpecHash(ctx, managedAgentInstallationTarget, dda.UID, testAddonInstallOperationID, hash))
		require.NoError(t, daemon.recordFleetDatadogAgentSpecHash(ctx, managedAgentInstallationTarget, dda.UID, testAddonInstallOperationID, hash))
		updated := &v2alpha1.DatadogAgent{}
		require.NoError(t, kubeClient.Get(ctx, managedAgentInstallationTarget, updated))
		assert.Equal(t, hash, updated.Annotations[fleetConfigHashAnnotation])
	})

	for _, test := range []struct {
		name      string
		uid       types.UID
		configID  string
		hash      string
		wantError string
	}{
		{name: "replacement UID", uid: "replacement", configID: testAddonInstallOperationID, hash: hash, wantError: "was replaced"},
		{name: "different config", uid: base.UID, configID: testAddonUninstallOperationID, hash: hash, wantError: "use an update operation"},
		{name: "changed spec", uid: base.UID, configID: testAddonInstallOperationID, hash: "different-hash", wantError: "spec changed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			daemon, _, _ := testManagedAgentInstallationDaemon(nil, base.DeepCopy())
			require.ErrorContains(t, daemon.recordFleetDatadogAgentSpecHash(ctx, managedAgentInstallationTarget, test.uid, test.configID, test.hash), test.wantError)
		})
	}

	t.Run("mark ready rejects replacement", func(t *testing.T) {
		dda := base.DeepCopy()
		dda.Labels[fleetManagedAgentInstallationStateLabel] = fleetManagedAgentInstallationStatePartial
		daemon, _, _ := testManagedAgentInstallationDaemon(nil, dda)
		require.ErrorContains(t, daemon.markFleetDatadogAgentReady(ctx, managedAgentInstallationTarget, "replacement", testAddonInstallOperationID), "was replaced")
	})

	t.Run("mark ready requires target", func(t *testing.T) {
		daemon, _, _ := testManagedAgentInstallationDaemon(nil)
		require.Error(t, daemon.markFleetDatadogAgentReady(ctx, managedAgentInstallationTarget, base.UID, testAddonInstallOperationID))
	})

	t.Run("mark ready requires matching config", func(t *testing.T) {
		dda := base.DeepCopy()
		dda.Labels[fleetManagedAgentInstallationStateLabel] = fleetManagedAgentInstallationStatePartial
		daemon, _, _ := testManagedAgentInstallationDaemon(nil, dda)
		require.ErrorContains(t, daemon.markFleetDatadogAgentReady(ctx, managedAgentInstallationTarget, dda.UID, testAddonUninstallOperationID), "use an update operation")
	})

	t.Run("mark ready requires accepted spec", func(t *testing.T) {
		dda := base.DeepCopy()
		command := testManagedAgentInstallationCommand(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
		spec, err := buildFleetDatadogAgentSpec(command.Config)
		require.NoError(t, err)
		dda.Spec = *spec
		dda.Labels[fleetManagedAgentInstallationStateLabel] = fleetManagedAgentInstallationStatePartial
		dda.Annotations[fleetConfigHashAnnotation] = "stale"
		daemon, _, _ := testManagedAgentInstallationDaemon(nil, dda)
		require.ErrorContains(t, daemon.markFleetDatadogAgentReady(ctx, managedAgentInstallationTarget, dda.UID, testAddonInstallOperationID), "does not match its accepted Fleet config")
	})

	t.Run("mark partial rejects incomplete ownership", func(t *testing.T) {
		dda := base.DeepCopy()
		delete(dda.Labels, fleetConfigIDLabel)
		daemon, _, _ := testManagedAgentInstallationDaemon(nil, dda)
		_, err := daemon.markFleetDatadogAgentPartial(ctx, managedAgentInstallationTarget, dda.UID)
		require.ErrorContains(t, err, "incomplete or conflicting Fleet ownership metadata")
	})

	t.Run("mark partial requires target", func(t *testing.T) {
		daemon, _, _ := testManagedAgentInstallationDaemon(nil)
		_, err := daemon.markFleetDatadogAgentPartial(ctx, managedAgentInstallationTarget, base.UID)
		require.Error(t, err)
	})

	t.Run("mark partial rejects replacement", func(t *testing.T) {
		daemon, _, _ := testManagedAgentInstallationDaemon(nil, base.DeepCopy())
		_, err := daemon.markFleetDatadogAgentPartial(ctx, managedAgentInstallationTarget, "replacement")
		require.ErrorContains(t, err, "was replaced")
	})

	t.Run("mark partial requires matching installation", func(t *testing.T) {
		dda := base.DeepCopy()
		dda.Labels[fleetInstallationIDLabel] = "other-installation"
		daemon, _, _ := testManagedAgentInstallationDaemon(nil, dda)
		_, err := daemon.markFleetDatadogAgentPartial(ctx, managedAgentInstallationTarget, dda.UID)
		require.ErrorContains(t, err, "different managed installation")
	})
}

func TestValidateFleetCredentialSecretRejectsEmptyAPIKey(t *testing.T) {
	secret := testFleetCredentialSecret()
	secret.Data[fleetCredentialAPIKey] = nil
	daemon, _, _ := testManagedAgentInstallationDaemon(nil, secret)

	err := daemon.validateFleetCredentialSecret(context.Background())

	require.ErrorContains(t, err, "missing non-empty key")
}

func TestManagedAgentInstallationRetainsPartialStateWhenProfileChanges(t *testing.T) {
	ctx := context.Background()
	daemon, kubeClient, rcClient := testManagedAgentInstallationDaemon(
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
		testFleetCredentialSecret(),
	)
	command := testManagedAgentInstallationCommand(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
	pending, err := daemon.installDatadogAgent(ctx, command)
	requireSyncNoError(t, pending, err)

	profile := &v1alpha1.DatadogAgentProfile{}
	require.NoError(t, kubeClient.Get(ctx, managedAgentInstallationWindowsProfileKey, profile))
	delete(profile.Annotations, kubernetes.ProviderAnnotationKey)
	require.NoError(t, kubeClient.Update(ctx, profile))

	_, err = daemon.installDatadogAgent(ctx, command)
	require.ErrorContains(t, err, "differs from the pinned managed Agent installation configuration")
	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, kubeClient.Get(ctx, managedAgentInstallationTarget, dda))
	assert.Equal(t, fleetManagedAgentInstallationStatePartial, dda.Labels[fleetManagedAgentInstallationStateLabel])
	assert.Equal(t, fleetPartialConfigVersionPrefix+testAddonInstallOperationID, rcClient.state[0].GetStableConfigVersion())
}

func TestValidateManagedAgentInstallationCommand(t *testing.T) {
	valid := testManagedAgentInstallationCommand(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
	for _, test := range []struct {
		name   string
		mutate func(*Daemon, *managedAgentInstallationCommand)
	}{
		{name: "missing operation ID", mutate: func(_ *Daemon, command *managedAgentInstallationCommand) { command.Intent.OperationID = "" }},
		{name: "missing local identity", mutate: func(d *Daemon, _ *managedAgentInstallationCommand) {
			d.managedAgentInstallationIdentity = ManagedAgentInstallationIdentity{}
		}},
		{name: "installation mismatch", mutate: func(_ *Daemon, command *managedAgentInstallationCommand) { command.Intent.InstallationID = "other" }},
		{name: "provider mismatch", mutate: func(_ *Daemon, command *managedAgentInstallationCommand) { command.Intent.Provider = "other" }},
		{name: "unsupported desired state", mutate: func(_ *Daemon, command *managedAgentInstallationCommand) { command.Intent.DesiredState = "other" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			daemon, _, _ := testManagedAgentInstallationDaemon(nil)
			command := valid
			test.mutate(daemon, &command)
			require.Error(t, daemon.validateManagedAgentInstallationCommand(command))
		})
	}
}

func TestManagedAgentInstallationOwnershipValidation(t *testing.T) {
	base := testFleetManagedDatadogAgent(t, "", testAddonInstallOperationID)

	owned, err := classifyFleetDatadogAgentOwnership(base)
	require.NoError(t, err)
	assert.True(t, owned)
	unmanaged := testDDAObject("")
	owned, err = classifyFleetDatadogAgentOwnership(unmanaged)
	require.NoError(t, err)
	assert.False(t, owned)

	incomplete := base.DeepCopy()
	delete(incomplete.Annotations, fleetConfigHashAnnotation)
	_, err = classifyFleetDatadogAgentOwnership(incomplete)
	require.ErrorContains(t, err, "incomplete or conflicting")

	require.NoError(t, validateFleetOwnedDatadogAgent(base, testAddonInstallOperationID))
	require.ErrorContains(t, validateFleetOwnedDatadogAgent(base, "other-config"), "use an update operation")
	require.ErrorContains(t, validateFleetOwnedDatadogAgent(unmanaged, ""), "not owned")

	daemon, _, _ := testManagedAgentInstallationDaemon(nil)
	require.NoError(t, daemon.validateFleetDatadogAgentInstallation(base))
	for _, test := range []struct {
		name   string
		mutate func(*v2alpha1.DatadogAgent)
	}{
		{name: "provider", mutate: func(dda *v2alpha1.DatadogAgent) { dda.Labels[fleetManagedAgentInstallationProviderLabel] = "other" }},
		{name: "installation", mutate: func(dda *v2alpha1.DatadogAgent) { dda.Labels[fleetInstallationIDLabel] = "other" }},
		{name: "target", mutate: func(dda *v2alpha1.DatadogAgent) { dda.Labels[fleetTargetIDLabel] = "other" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			dda := base.DeepCopy()
			test.mutate(dda)
			require.Error(t, daemon.validateFleetDatadogAgentInstallation(dda))
		})
	}
	daemon.managedAgentInstallationIdentity = ManagedAgentInstallationIdentity{}
	require.ErrorContains(t, daemon.validateFleetDatadogAgentInstallation(base), "identity is not configured")
	require.NoError(t, daemon.validateFleetDatadogAgentInstallation(unmanaged))
}

func TestManagedAgentInstallationExperimentStateValidation(t *testing.T) {
	ready := testFleetManagedDatadogAgent(t, "", testAddonInstallOperationID)
	require.NoError(t, validateFleetDatadogAgentExperimentOperationState(ready, pendingIntentStart))

	partial := ready.DeepCopy()
	partial.Labels[fleetManagedAgentInstallationStateLabel] = fleetManagedAgentInstallationStatePartial
	require.ErrorContains(t, validateFleetDatadogAgentExperimentOperationState(partial, pendingIntentStart), "install gate")

	partial.Status.Experiment = &v2alpha1.ExperimentStatus{ID: testExperimentID, Phase: v2alpha1.ExperimentPhaseRunning}
	require.NoError(t, validateFleetDatadogAgentExperimentOperationState(partial, pendingIntentStop))
	partial.Status.Experiment.Phase = v2alpha1.ExperimentPhasePromoted
	partial.Annotations[v2alpha1.AnnotationPendingTaskID] = "promote-task"
	partial.Annotations[v2alpha1.AnnotationPendingAction] = string(pendingIntentPromote)
	partial.Annotations[v2alpha1.AnnotationPendingExperimentID] = testExperimentID
	partial.Annotations[v2alpha1.AnnotationPendingPackage] = packageDatadogOperator
	require.NoError(t, validateFleetDatadogAgentExperimentOperationState(partial, pendingIntentPromote))

	ready.Status.Experiment = &v2alpha1.ExperimentStatus{ID: testExperimentID, Phase: v2alpha1.ExperimentPhaseRunning}
	require.ErrorContains(t, validateNoActiveFleetExperiment(ready), "active Fleet experiment")
	ready.Annotations[v2alpha1.AnnotationPendingTaskID] = "task"
	ready.Annotations[v2alpha1.AnnotationPendingAction] = string(pendingIntentStart)
	ready.Annotations[v2alpha1.AnnotationPendingExperimentID] = testExperimentID
	ready.Annotations[v2alpha1.AnnotationPendingPackage] = packageDatadogOperator
	require.ErrorContains(t, validateNoActiveFleetExperiment(ready), "pending Fleet experiment task")
}

func TestManagedAgentInstallationWindowsProfileValidation(t *testing.T) {
	dda := testFleetManagedDatadogAgent(t, "", testAddonInstallOperationID)
	daemon, _, _ := testManagedAgentInstallationDaemon(nil)
	valid := daemon.managedAgentInstallationWindowsProfile(dda)
	require.NoError(t, daemon.validateManagedAgentInstallationWindowsProfile(valid, dda))

	for _, test := range []struct {
		name   string
		mutate func(*v1alpha1.DatadogAgentProfile)
	}{
		{name: "terminating", mutate: func(profile *v1alpha1.DatadogAgentProfile) { now := metav1.Now(); profile.DeletionTimestamp = &now }},
		{name: "ownership labels", mutate: func(profile *v1alpha1.DatadogAgentProfile) { delete(profile.Labels, fleetInstallationIDLabel) }},
		{name: "owner reference", mutate: func(profile *v1alpha1.DatadogAgentProfile) { profile.OwnerReferences[0].UID = "other" }},
		{name: "provider annotation", mutate: func(profile *v1alpha1.DatadogAgentProfile) {
			delete(profile.Annotations, kubernetes.ProviderAnnotationKey)
		}},
		{name: "profile affinity", mutate: func(profile *v1alpha1.DatadogAgentProfile) { profile.Spec.ProfileAffinity = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			profile := valid.DeepCopy()
			test.mutate(profile)
			require.Error(t, daemon.validateManagedAgentInstallationWindowsProfile(profile, dda))
		})
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

func testFleetManagedDatadogAgent(t testing.TB, phase v2alpha1.ExperimentPhase, configID string) *v2alpha1.DatadogAgent {
	t.Helper()
	dda := testDDAObject(phase)
	dda.UID = types.UID("managed-dda-uid")
	dda.Labels = map[string]string{
		fleetManagedByLabel:                        fleetManagedByValue,
		fleetConfigIDLabel:                         configID,
		fleetManagedAgentInstallationStateLabel:    fleetManagedAgentInstallationStateReady,
		fleetManagedAgentInstallationProviderLabel: string(testManagedAgentInstallationIdentity.Provider()),
		fleetInstallationIDLabel:                   testManagedAgentInstallationIdentity.InstallationID(),
		fleetTargetIDLabel:                         testManagedAgentInstallationIdentity.TargetID(),
	}
	if dda.Annotations == nil {
		dda.Annotations = make(map[string]string)
	}
	hash, err := fleetDatadogAgentSpecHash(&dda.Spec)
	require.NoError(t, err)
	dda.Annotations[fleetConfigHashAnnotation] = hash
	return dda
}

func testManagedAgentInstallationCommand(t *testing.T, operationID string, desiredState managedAgentInstallationDesiredState) managedAgentInstallationCommand {
	t.Helper()
	raw := testManagedAgentInstallationIntent(t, operationID, desiredState)
	intent, config, digest, err := decodeManagedAgentInstallationIntent(raw, testManagedAgentInstallationIdentity)
	require.NoError(t, err)
	return newManagedAgentInstallationCommand(intent, config, digest)
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
