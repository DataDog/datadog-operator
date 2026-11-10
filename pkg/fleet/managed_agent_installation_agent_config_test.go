// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	v1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

func testAgentConfigJSON(t *testing.T, spec map[string]any) json.RawMessage {
	t.Helper()
	if spec == nil {
		return nil
	}
	raw, err := json.Marshal(map[string]any{"spec": spec})
	require.NoError(t, err)
	return raw
}

func testAgentProfileJSON(t *testing.T, name string) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal([]map[string]any{{
		"name": name,
		"profileAffinity": map[string]any{
			"profileNodeAffinity": []map[string]any{{
				"key":      "nvidia.com/gpu.present",
				"operator": "In",
				"values":   []string{"true"},
			}},
		},
		"config": map[string]any{
			"spec": map[string]any{
				"features": map[string]any{
					"gpu": map[string]any{"enabled": true},
				},
			},
		},
	}})
	require.NoError(t, err)
	return raw
}

func eksInstallIntentJSONWithAgentSettings(t *testing.T, settings map[string]any) []byte {
	t.Helper()
	intent := eksManagedAgentInstallationIntent{
		Version:        managedAgentInstallationVersion,
		InstallationID: testManagedAgentInstallationIdentity.InstallationID(),
		EKSARNSHA256:   testManagedAgentInstallationTargetHash,
		OperationID:    testAddonInstallOperationID,
		DesiredState:   managedAgentInstallationDesiredStateInstalled,
		Bootstrap:      managedAgentInstallationBootstrap{ClusterName: "test-cluster", Site: "site.example"},
	}
	base, err := json.Marshal(intent)
	require.NoError(t, err)
	if settings == nil {
		return base
	}
	var raw map[string]any
	require.NoError(t, json.Unmarshal(base, &raw))
	for key, value := range settings {
		raw[key] = value
	}
	encoded, err := json.Marshal(raw)
	require.NoError(t, err)
	return encoded
}

func TestDecodeManagedAgentInstallationIntentWithAgentConfig(t *testing.T) {
	base := eksInstallIntentJSONWithAgentSettings(t, nil)
	agentConfig := map[string]any{"spec": map[string]any{
		"features": map[string]any{"apm": map[string]any{"instrumentation": map[string]any{"enabled": true}}},
		"override": map[string]any{"nodeAgent": map[string]any{"nodeSelector": map[string]any{"gpu": "true"}}},
	}}
	rawWithConfig := eksInstallIntentJSONWithAgentSettings(t, map[string]any{"agentConfig": agentConfig})
	require.NotEqual(t, string(base), string(rawWithConfig))

	intent, config, digest, err := decodeManagedAgentInstallationIntent(rawWithConfig, testManagedAgentInstallationIdentity)
	require.NoError(t, err)
	require.NotNil(t, intent.AgentConfig)
	require.NotNil(t, intent.AgentConfig.Spec.Features)
	require.NotNil(t, intent.AgentConfig.Spec.Features.APM)

	var decoded datadogAgentManagedAgentInstallationConfig
	require.NoError(t, json.Unmarshal(config, &decoded))
	require.NotNil(t, decoded.Spec)
	require.NotNil(t, decoded.Spec.Global)
	// The bootstrap identity fields stay authoritative over the fragment.
	assert.Equal(t, "test-cluster", *decoded.Spec.Global.ClusterName)
	assert.Equal(t, "site.example", *decoded.Spec.Global.Site)
	require.NotNil(t, decoded.Spec.Features.APM)
	require.NotNil(t, decoded.Spec.Features.APM.SingleStepInstrumentation)
	require.NotNil(t, decoded.Spec.Features.APM.SingleStepInstrumentation.Enabled)
	assert.True(t, *decoded.Spec.Features.APM.SingleStepInstrumentation.Enabled)
	// Fragment selectors merge with, not replace, the bootstrap linux selector.
	linuxSelector := map[string]string{corev1.LabelOSStable: string(corev1.Linux)}
	assert.Equal(t, map[string]string{
		corev1.LabelOSStable: string(corev1.Linux),
		"gpu":                "true",
	}, decoded.Spec.Override[v2alpha1.NodeAgentComponentName].NodeSelector)
	assert.Equal(t, linuxSelector, decoded.Spec.Override[v2alpha1.ClusterAgentComponentName].NodeSelector)
	assert.Len(t, digest, 64)

	// The same content decodes to the same digest, and a changed agentConfig
	// changes the digest (replay protection covers the settings).
	_, _, repeatedDigest, err := decodeManagedAgentInstallationIntent([]byte(rawWithConfig), testManagedAgentInstallationIdentity)
	require.NoError(t, err)
	assert.Equal(t, digest, repeatedDigest)

	_, _, baseDigest, err := decodeManagedAgentInstallationIntent(base, testManagedAgentInstallationIdentity)
	require.NoError(t, err)
	assert.NotEqual(t, digest, baseDigest)

	// Reformatted-but-equivalent agentConfig JSON keeps the digest stable.
	reformatted := strings.Replace(string(rawWithConfig), `"enabled":true`, `"enabled" :  true`, 1)
	require.NotEqual(t, string(rawWithConfig), reformatted)
	_, _, reformattedDigest, err := decodeManagedAgentInstallationIntent([]byte(reformatted), testManagedAgentInstallationIdentity)
	require.NoError(t, err)
	assert.Equal(t, digest, reformattedDigest)
}

func TestDecodeManagedAgentInstallationIntentRejectsUnsafeAgentSettings(t *testing.T) {
	base := string(testManagedAgentInstallationIntent(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled))
	settingsAt := func(desiredState, settings string) []byte {
		raw := strings.Replace(base, `"desiredState":"installed"`, `"desiredState":"`+desiredState+`"`+settings, 1)
		require.NotEqual(t, base, raw)
		return []byte(raw)
	}
	tests := []struct {
		name      string
		raw       []byte
		wantError string
	}{
		{
			name:      "agent config credentials",
			raw:       settingsAt("installed", `,"agentConfig":{"spec":{"global":{"credentials":{"apiSecret":{"secretName":"other"}}}}}`),
			wantError: "must not contain spec.global.credentials",
		},
		{
			name:      "agent config unknown field",
			raw:       settingsAt("installed", `,"agentConfig":{"spec":{"bogus":true}}`),
			wantError: "unknown field",
		},
		{
			name:      "agent config on uninstall",
			raw:       settingsAt("absent", `,"agentConfig":{"spec":{}}`),
			wantError: "uninstall must not carry agent config",
		},
		{
			name:      "agent profiles on uninstall",
			raw:       settingsAt("absent", `,"agentProfiles":[]`),
			wantError: "must not be empty",
		},
		{
			name:      "agent profile unknown field",
			raw:       settingsAt("installed", `,"agentProfiles":[{"name":"gpu","bogus":true}]`),
			wantError: "unknown field",
		},
		{
			name:      "agent profile invalid name",
			raw:       settingsAt("installed", `,"agentProfiles":[{"name":"Bad_Name"}]`),
			wantError: "invalid name",
		},
		{
			name:      "agent profile reserved name",
			raw:       settingsAt("installed", `,"agentProfiles":[{"name":"datadog-agent-windows"}]`),
			wantError: "reserved",
		},
		{
			name:      "agent profile duplicate name",
			raw:       settingsAt("installed", `,"agentProfiles":[{"name":"gpu","profileAffinity":{"profileNodeAffinity":[{"key":"k","operator":"In","values":["v"]}]},"config":{"spec":{}}},{"name":"gpu","profileAffinity":{"profileNodeAffinity":[{"key":"k","operator":"In","values":["v"]}]},"config":{"spec":{}}}]`),
			wantError: "duplicated",
		},
		{
			name:      "agent profile missing affinity",
			raw:       settingsAt("installed", `,"agentProfiles":[{"name":"gpu","config":{"spec":{"features":{"gpu":{"enabled":true}}}}}]`),
			wantError: "profileAffinity",
		},
		{
			name:      "agent profile feature outside the allowlist",
			raw:       settingsAt("installed", `,"agentProfiles":[{"name":"gpu","profileAffinity":{"profileNodeAffinity":[{"key":"k","operator":"In","values":["v"]}]},"config":{"spec":{"features":{"oomKill":{"enabled":true}}}}}]`),
			wantError: "not supported",
		},
		{
			name:      "agent profile config credentials",
			raw:       settingsAt("installed", `,"agentProfiles":[{"name":"gpu","profileAffinity":{"profileNodeAffinity":[{"key":"k","operator":"In","values":["v"]}]},"config":{"spec":{"global":{"credentials":{"apiKey":"leak"}}}}}]`),
			wantError: "must not contain",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, _, err := decodeManagedAgentInstallationIntent(test.raw, testManagedAgentInstallationIdentity)
			require.ErrorContains(t, err, test.wantError)
		})
	}
}

func TestManagedAgentInstallationAgentProfilesLifecycle(t *testing.T) {
	ctx := context.Background()
	dda := testFleetManagedDatadogAgent(t, "", testAddonInstallOperationID)
	daemon, kubeClient, _ := testManagedAgentInstallationDaemon(nil, dda)

	profiles := []decodedManagedAgentInstallationAgentProfile{{
		Name: "gpu-nodes",
		ProfileAffinity: &v1alpha1.ProfileAffinity{ProfileNodeAffinity: []corev1.NodeSelectorRequirement{{
			Key: "nvidia.com/gpu.present", Operator: corev1.NodeSelectorOpIn, Values: []string{"true"},
		}}},
		Config: &v2alpha1.DatadogAgentSpec{Features: &v2alpha1.DatadogFeatures{GPU: &v2alpha1.GPUFeatureConfig{Enabled: new(true)}}},
	}}

	require.NoError(t, daemon.ensureManagedAgentInstallationAgentProfiles(ctx, dda, profiles))
	created := &v1alpha1.DatadogAgentProfile{}
	require.NoError(t, kubeClient.Get(ctx, daemon.managedAgentInstallationKey("gpu-nodes"), created))
	assert.Equal(t, fleetManagedByValue, created.Labels[fleetManagedByLabel])
	require.Len(t, created.OwnerReferences, 1)
	assert.Equal(t, dda.UID, created.OwnerReferences[0].UID)
	require.NotNil(t, created.Spec.Config)
	require.NotNil(t, created.Spec.Config.Features.GPU)

	// Re-ensuring is idempotent and validating accepts the created profile.
	require.NoError(t, daemon.ensureManagedAgentInstallationAgentProfiles(ctx, dda, profiles))
	require.NoError(t, daemon.validateManagedAgentInstallationAgentProfilesExist(ctx, dda, profiles))

	// A drifted profile is rejected as a state mismatch.
	created.Spec.Config.Features.GPU.Enabled = new(false)
	require.NoError(t, kubeClient.Update(ctx, created))
	require.ErrorContains(t, daemon.validateManagedAgentInstallationAgentProfilesExist(ctx, dda, profiles), "differs from the pinned")
}

func TestManagedAgentInstallationResourcesAbsentWaitsForAgentProfiles(t *testing.T) {
	profile := &v1alpha1.DatadogAgentProfile{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: testManagedAgentInstallationNamespace,
			Name:      "gpu-nodes",
			Labels: map[string]string{
				fleetManagedByLabel:                        fleetManagedByValue,
				fleetManagedAgentInstallationProviderLabel: string(testManagedAgentInstallationIdentity.Provider()),
				fleetInstallationIDLabel:                   testManagedAgentInstallationIdentity.InstallationID(),
				fleetTargetIDLabel:                         testManagedAgentInstallationIdentity.TargetID(),
			},
		},
	}
	daemon, kubeClient, _ := testManagedAgentInstallationDaemon(nil, profile)

	absent, err := daemon.managedAgentInstallationResourcesAbsent(context.Background(), managedAgentInstallationTarget, "")
	require.NoError(t, err)
	assert.False(t, absent)

	require.NoError(t, kubeClient.Delete(context.Background(), profile))
	absent, err = daemon.managedAgentInstallationResourcesAbsent(context.Background(), managedAgentInstallationTarget, "")
	require.NoError(t, err)
	assert.True(t, absent)
}

func TestManagedAgentInstallationInstallWithAgentProfiles(t *testing.T) {
	ctx := context.Background()
	daemon, kubeClient, _ := testManagedAgentInstallationDaemon(nil, testFleetCredentialSecret())

	command := testManagedAgentInstallationCommand(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
	command.Intent.AgentProfiles = []decodedManagedAgentInstallationAgentProfile{{
		Name: "gpu-nodes",
		ProfileAffinity: &v1alpha1.ProfileAffinity{ProfileNodeAffinity: []corev1.NodeSelectorRequirement{{
			Key: "nvidia.com/gpu.present", Operator: corev1.NodeSelectorOpIn, Values: []string{"true"},
		}}},
		Config: &v2alpha1.DatadogAgentSpec{Features: &v2alpha1.DatadogFeatures{GPU: &v2alpha1.GPUFeatureConfig{Enabled: new(true)}}},
	}}

	require.NoError(t, daemon.installDatadogAgent(ctx, command))

	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, kubeClient.Get(ctx, daemon.managedAgentInstallationTarget(), dda))
	assert.Equal(t, "true", dda.Annotations[apicommon.UpdateMetadataAnnotationKey])

	profile := &v1alpha1.DatadogAgentProfile{}
	require.NoError(t, kubeClient.Get(ctx, daemon.managedAgentInstallationKey("gpu-nodes"), profile))
	require.Len(t, profile.OwnerReferences, 1)
	assert.Equal(t, dda.UID, profile.OwnerReferences[0].UID)
	require.NoError(t, daemon.validateManagedAgentInstallationAgentProfilesExist(ctx, dda, command.Intent.AgentProfiles))
}
