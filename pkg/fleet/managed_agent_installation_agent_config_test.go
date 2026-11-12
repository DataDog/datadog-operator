// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	v1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

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

// testGenericManagedAgentInstallationDaemon builds the standard test daemon with the generic managed Agent installation identity, so it decodes generic provider intents.
func testGenericManagedAgentInstallationDaemon(rcState []*pbgo.PackageState, objects ...client.Object) (*Daemon, client.Client, *mockRCClient) {
	daemon, kubeClient, rcClient := testManagedAgentInstallationDaemon(rcState, objects...)
	daemon.managedAgentInstallationIdentity = validGenericManagedAgentInstallationIdentity
	return daemon, kubeClient, rcClient
}

func TestDecodeManagedAgentInstallationIntentWithAgentConfig(t *testing.T) {
	base := genericIntentJSONWith(t, nil)
	agentConfig := map[string]any{"spec": map[string]any{
		"features": map[string]any{"logCollection": map[string]any{"enabled": true}},
		"override": map[string]any{"nodeAgent": map[string]any{"nodeSelector": map[string]any{"gpu": "true"}}},
	}}
	rawWithConfig := genericIntentJSONWith(t, map[string]any{"agentConfig": agentConfig})
	require.NotEqual(t, string(base), string(rawWithConfig))

	intent, config, digest, err := decodeManagedAgentInstallationIntent(rawWithConfig, validGenericManagedAgentInstallationIdentity)
	require.NoError(t, err)
	require.NotNil(t, intent.AgentConfig)
	require.NotNil(t, intent.AgentConfig.Spec.Features)
	require.NotNil(t, intent.AgentConfig.Spec.Features.LogCollection)

	var decoded datadogAgentManagedAgentInstallationConfig
	require.NoError(t, json.Unmarshal(config, &decoded))
	require.NotNil(t, decoded.Spec)
	require.NotNil(t, decoded.Spec.Global)
	// The bootstrap identity fields stay authoritative over the fragment.
	assert.Equal(t, "test-cluster", *decoded.Spec.Global.ClusterName)
	assert.Equal(t, "datadoghq.com", *decoded.Spec.Global.Site)
	require.NotNil(t, decoded.Spec.Features.LogCollection)
	require.NotNil(t, decoded.Spec.Features.LogCollection.Enabled)
	assert.True(t, *decoded.Spec.Features.LogCollection.Enabled)
	// Fragment selectors merge with, not replace, the bootstrap linux selector.
	linuxSelector := map[string]string{corev1.LabelOSStable: string(corev1.Linux)}
	assert.Equal(t, map[string]string{
		corev1.LabelOSStable: string(corev1.Linux),
		"gpu":                "true",
	}, decoded.Spec.Override[v2alpha1.NodeAgentComponentName].NodeSelector)
	assert.Equal(t, linuxSelector, decoded.Spec.Override[v2alpha1.ClusterAgentComponentName].NodeSelector)
	assert.Len(t, digest, 64)

	// The same content decodes to the same digest, and a changed agentConfig changes the digest (replay protection covers the settings).
	_, _, repeatedDigest, err := decodeManagedAgentInstallationIntent([]byte(rawWithConfig), validGenericManagedAgentInstallationIdentity)
	require.NoError(t, err)
	assert.Equal(t, digest, repeatedDigest)

	_, _, baseDigest, err := decodeManagedAgentInstallationIntent(base, validGenericManagedAgentInstallationIdentity)
	require.NoError(t, err)
	assert.NotEqual(t, digest, baseDigest)

	// Reformatted-but-equivalent agentConfig JSON keeps the digest stable.
	reformatted := strings.Replace(string(rawWithConfig), `"enabled":true`, `"enabled" :  true`, 1)
	require.NotEqual(t, string(rawWithConfig), reformatted)
	_, _, reformattedDigest, err := decodeManagedAgentInstallationIntent([]byte(reformatted), validGenericManagedAgentInstallationIdentity)
	require.NoError(t, err)
	assert.Equal(t, digest, reformattedDigest)
}

func TestDecodeManagedAgentInstallationIntentRejectsUnsafeAgentSettings(t *testing.T) {
	base := string(genericIntentJSONWith(t, nil))
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
			_, _, _, err := decodeManagedAgentInstallationIntent(test.raw, validGenericManagedAgentInstallationIdentity)
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

func assertInstallErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error containing %q, got nil", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("expected an error containing %q, got %q", want, err.Error())
	}
}

func testGPUAgentProfile(name string) decodedManagedAgentInstallationAgentProfile {
	return decodedManagedAgentInstallationAgentProfile{
		Name: name,
		ProfileAffinity: &v1alpha1.ProfileAffinity{ProfileNodeAffinity: []corev1.NodeSelectorRequirement{{
			Key: "nvidia.com/gpu.present", Operator: corev1.NodeSelectorOpIn, Values: []string{"true"},
		}}},
		Config: &v2alpha1.DatadogAgentSpec{Features: &v2alpha1.DatadogFeatures{GPU: &v2alpha1.GPUFeatureConfig{Enabled: new(true)}}},
	}
}

func TestDecodeManagedAgentInstallationIntentWithAgentProfiles(t *testing.T) {
	rawWithProfiles := genericIntentJSONWith(t, map[string]any{
		"agentProfiles": json.RawMessage(testAgentProfileJSON(t, "gpu-nodes")),
	})

	intent, config, digest, err := decodeManagedAgentInstallationIntent(rawWithProfiles, validGenericManagedAgentInstallationIdentity)
	if err != nil {
		t.Fatalf("expected the intent to decode, got %v", err)
	}
	if len(intent.AgentProfiles) != 1 {
		t.Fatalf("expected one agent profile, got %d", len(intent.AgentProfiles))
	}
	profile := intent.AgentProfiles[0]
	if profile.Name != "gpu-nodes" {
		t.Errorf("expected the profile name gpu-nodes, got %q", profile.Name)
	}
	if profile.ProfileAffinity == nil || len(profile.ProfileAffinity.ProfileNodeAffinity) != 1 ||
		profile.ProfileAffinity.ProfileNodeAffinity[0].Key != "nvidia.com/gpu.present" {
		t.Errorf("expected the profile affinity to be decoded, got %+v", profile.ProfileAffinity)
	}
	if profile.Config == nil || profile.Config.Features == nil || profile.Config.Features.GPU == nil ||
		profile.Config.Features.GPU.Enabled == nil || !*profile.Config.Features.GPU.Enabled {
		t.Errorf("expected the profile config to enable the GPU feature, got %+v", profile.Config)
	}

	var decoded datadogAgentManagedAgentInstallationConfig
	if err := json.Unmarshal(config, &decoded); err != nil {
		t.Fatalf("expected the merged config to decode, got %v", err)
	}
	if decoded.Spec == nil || decoded.Spec.Global == nil || decoded.Spec.Global.ClusterName == nil ||
		*decoded.Spec.Global.ClusterName != "test-cluster" {
		t.Errorf("expected the bootstrap cluster name to stay authoritative, got %+v", decoded.Spec)
	}
	if len(digest) != 64 {
		t.Errorf("expected a sha256 digest, got %q", digest)
	}

	_, _, repeatedDigest, err := decodeManagedAgentInstallationIntent(rawWithProfiles, validGenericManagedAgentInstallationIdentity)
	if err != nil {
		t.Fatalf("expected the intent to decode twice, got %v", err)
	}
	if digest != repeatedDigest {
		t.Errorf("expected a stable digest, got %q then %q", digest, repeatedDigest)
	}
}

func TestDecodeManagedAgentInstallationAgentProfilesRejectsEdgeCases(t *testing.T) {
	base := string(genericIntentJSONWith(t, nil))
	settingsAt := func(settings string) []byte {
		t.Helper()
		raw := strings.Replace(base, `"desiredState":"installed"`, `"desiredState":"installed"`+settings, 1)
		if raw == base {
			t.Fatalf("settings were not applied: %s", settings)
		}
		return []byte(raw)
	}

	names := make([]string, 0, managedAgentInstallationMaxAgentProfiles+1)
	for i := 1; i <= managedAgentInstallationMaxAgentProfiles+1; i++ {
		names = append(names, fmt.Sprintf(`{"name":"profile-%d"}`, i))
	}

	tests := []struct {
		name      string
		raw       []byte
		wantError string
		direct    bool
	}{
		{
			name:      "trailing content after the profiles",
			raw:       json.RawMessage(`[{"name":"gpu-nodes"}] trailing`),
			wantError: "trailing JSON content",
			direct:    true,
		},
		{
			name:      "too many profiles",
			raw:       settingsAt(`,"agentProfiles":[` + strings.Join(names, ",") + `]`),
			wantError: fmt.Sprintf("exceed the maximum of %d", managedAgentInstallationMaxAgentProfiles),
		},
		{
			name:      "profile config without spec",
			raw:       settingsAt(`,"agentProfiles":[{"name":"gpu-nodes","profileAffinity":{"profileNodeAffinity":[{"key":"k","operator":"In","values":["v"]}]},"config":{}}]`),
			wantError: "config must contain spec",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var err error
			if test.direct {
				_, err = decodeManagedAgentInstallationAgentProfiles(test.raw)
			} else {
				_, _, _, err = decodeManagedAgentInstallationIntent(test.raw, validGenericManagedAgentInstallationIdentity)
			}
			assertInstallErrorContains(t, err, test.wantError)
		})
	}
}

func TestDecodeManagedAgentInstallationIntentMergesFragmentWithoutOverride(t *testing.T) {
	raw := genericIntentJSONWith(t, map[string]any{
		"agentConfig": map[string]any{"spec": map[string]any{
			"features": map[string]any{"logCollection": map[string]any{"enabled": true}},
		}},
	})

	intent, config, _, err := decodeManagedAgentInstallationIntent(raw, validGenericManagedAgentInstallationIdentity)
	if err != nil {
		t.Fatalf("expected the intent to decode, got %v", err)
	}
	if intent.AgentConfig == nil || intent.AgentConfig.Spec == nil || intent.AgentConfig.Spec.Override != nil {
		t.Fatalf("expected a fragment without override, got %+v", intent.AgentConfig)
	}

	var decoded datadogAgentManagedAgentInstallationConfig
	if err := json.Unmarshal(config, &decoded); err != nil {
		t.Fatalf("expected the merged config to decode, got %v", err)
	}
	if decoded.Spec == nil || decoded.Spec.Override == nil {
		t.Fatalf("expected the merge to create component overrides, got %+v", decoded.Spec)
	}
	linuxSelector := map[string]string{corev1.LabelOSStable: string(corev1.Linux)}
	for _, component := range []v2alpha1.ComponentName{v2alpha1.NodeAgentComponentName, v2alpha1.ClusterAgentComponentName} {
		override := decoded.Spec.Override[component]
		if override == nil {
			t.Fatalf("expected an override for %s, got nil", component)
		}
		if got, want := override.NodeSelector, linuxSelector; len(got) != len(want) || got[corev1.LabelOSStable] != want[corev1.LabelOSStable] {
			t.Errorf("expected the bootstrap linux selector for %s, got %v", component, got)
		}
	}
}

func TestManagedAgentInstallationAgentProfileValidationBranches(t *testing.T) {
	daemon, _, _ := testManagedAgentInstallationDaemon(nil)
	dda := testFleetManagedDatadogAgent(t, "", testAddonInstallOperationID)
	wanted := testGPUAgentProfile("gpu-nodes")

	valid := daemon.managedAgentInstallationAgentProfile(dda, wanted)
	if err := daemon.validateManagedAgentInstallationAgentProfile(valid, dda, wanted); err != nil {
		t.Fatalf("expected the freshly built profile to validate, got %v", err)
	}

	terminating := valid.DeepCopy()
	terminating.DeletionTimestamp = &metav1.Time{Time: time.Now()}
	assertInstallErrorContains(t, daemon.validateManagedAgentInstallationAgentProfile(terminating, dda, wanted), "terminating")

	foreign := valid.DeepCopy()
	delete(foreign.Labels, fleetManagedByLabel)
	assertInstallErrorContains(t, daemon.validateManagedAgentInstallationAgentProfile(foreign, dda, wanted), "invalid managed Agent installation ownership")

	unowned := valid.DeepCopy()
	unowned.OwnerReferences = nil
	assertInstallErrorContains(t, daemon.validateManagedAgentInstallationAgentProfile(unowned, dda, wanted), "is not owned by")
}

func TestManagedAgentInstallationInstallAgentProfilesFailureModes(t *testing.T) {
	ctx := context.Background()
	newCommand := func(t *testing.T) managedAgentInstallationCommand {
		t.Helper()
		command := testManagedAgentInstallationCommand(t, testAddonInstallOperationID, managedAgentInstallationDesiredStateInstalled)
		command.Intent.AgentProfiles = []decodedManagedAgentInstallationAgentProfile{testGPUAgentProfile("gpu-nodes")}
		return command
	}
	newExistingDDA := func(t *testing.T, command managedAgentInstallationCommand) *v2alpha1.DatadogAgent {
		t.Helper()
		dda := testFleetManagedDatadogAgent(t, "", testAddonInstallOperationID)
		spec, err := buildFleetDatadogAgentSpec(command.Config)
		if err != nil {
			t.Fatalf("failed to build the DatadogAgent spec: %v", err)
		}
		dda.Spec = *spec
		hash, err := fleetDatadogAgentSpecHash(&dda.Spec)
		if err != nil {
			t.Fatalf("failed to hash the DatadogAgent spec: %v", err)
		}
		dda.Annotations[fleetConfigHashAnnotation] = hash
		return dda
	}
	faultProfileCreate := func(t *testing.T, daemon *Daemon) {
		t.Helper()
		daemon.client = &managedAgentInstallationFaultClient{
			Client: daemon.client,
			createError: func(obj client.Object) error {
				if profile, ok := obj.(*v1alpha1.DatadogAgentProfile); ok && profile.Name == "gpu-nodes" {
					return errors.New("agent profile create failed")
				}
				return nil
			},
		}
	}

	t.Run("create failure on existing target", func(t *testing.T) {
		command := newCommand(t)
		dda := newExistingDDA(t, command)
		daemon, _, _ := testManagedAgentInstallationDaemon(nil, testFleetCredentialSecret(), dda)
		faultProfileCreate(t, daemon)
		assertInstallErrorContains(t, daemon.installDatadogAgent(ctx, command), "agent profile create failed")
	})

	t.Run("create failure on new target", func(t *testing.T) {
		command := newCommand(t)
		daemon, _, _ := testManagedAgentInstallationDaemon(nil, testFleetCredentialSecret())
		faultProfileCreate(t, daemon)
		assertInstallErrorContains(t, daemon.installDatadogAgent(ctx, command), "agent profile create failed")
	})

	t.Run("read failure during revalidation", func(t *testing.T) {
		command := newCommand(t)
		daemon, kubeClient, _ := testManagedAgentInstallationDaemon(nil, testFleetCredentialSecret())
		reads := 0
		daemon.apiReader = &managedAgentInstallationFaultClient{
			Client: kubeClient,
			getError: func(key client.ObjectKey, _ client.Object) error {
				if key.Name == "gpu-nodes" {
					reads++
					if reads == 2 {
						return errors.New("agent profile read failed")
					}
				}
				return nil
			},
		}
		assertInstallErrorContains(t, daemon.installDatadogAgent(ctx, command), "agent profile read failed")
	})

	t.Run("read failure during revalidation on existing target", func(t *testing.T) {
		command := newCommand(t)
		dda := newExistingDDA(t, command)
		seedDaemon, _, _ := testManagedAgentInstallationDaemon(nil)
		seeded := seedDaemon.managedAgentInstallationAgentProfile(dda, testGPUAgentProfile("gpu-nodes"))
		daemon, kubeClient, _ := testManagedAgentInstallationDaemon(nil, testFleetCredentialSecret(), dda, seeded)
		reads := 0
		daemon.apiReader = &managedAgentInstallationFaultClient{
			Client: kubeClient,
			getError: func(key client.ObjectKey, _ client.Object) error {
				if key.Name == "gpu-nodes" {
					reads++
					if reads == 2 {
						return errors.New("agent profile read failed")
					}
				}
				return nil
			},
		}
		assertInstallErrorContains(t, daemon.installDatadogAgent(ctx, command), "agent profile read failed")
	})

	t.Run("read failure while ensuring profiles", func(t *testing.T) {
		command := newCommand(t)
		daemon, kubeClient, _ := testManagedAgentInstallationDaemon(nil, testFleetCredentialSecret())
		daemon.apiReader = &managedAgentInstallationFaultClient{
			Client: kubeClient,
			getError: func(key client.ObjectKey, _ client.Object) error {
				if key.Name == "gpu-nodes" {
					return errors.New("agent profile read failed")
				}
				return nil
			},
		}
		assertInstallErrorContains(t, daemon.installDatadogAgent(ctx, command), "agent profile read failed")
	})

	t.Run("drifted profile on existing target", func(t *testing.T) {
		command := newCommand(t)
		dda := newExistingDDA(t, command)
		seedDaemon, _, _ := testManagedAgentInstallationDaemon(nil)
		drifted := seedDaemon.managedAgentInstallationAgentProfile(dda, testGPUAgentProfile("gpu-nodes"))
		drifted.Spec.Config.Features.GPU.Enabled = new(false)
		daemon, _, _ := testManagedAgentInstallationDaemon(nil, testFleetCredentialSecret(), dda, drifted)
		assertInstallErrorContains(t, daemon.installDatadogAgent(ctx, command), "differs from the pinned")
	})
}

func TestManagedAgentInstallationAcknowledgeValidatesAgentProfiles(t *testing.T) {
	ctx := context.Background()
	daemon, kubeClient, _ := testGenericManagedAgentInstallationDaemon(
		[]*pbgo.PackageState{{Package: packageDatadogOperator}},
		testFleetCredentialSecret(),
	)
	installRaw := genericIntentJSONWith(t, map[string]any{
		"agentProfiles": json.RawMessage(testAgentProfileJSON(t, "gpu-nodes")),
	})
	putManagedAgentInstallationIntentConfigMap(t, kubeClient, installRaw)
	if err := daemon.handleManagedAgentInstallationIntent(ctx, managedAgentInstallationIntentSnapshot{raw: installRaw}); err != nil {
		t.Fatalf("expected the install intent to be handled, got %v", err)
	}

	// Drift the managed profile away from the pinned intent configuration.
	profile := &v1alpha1.DatadogAgentProfile{}
	if err := kubeClient.Get(ctx, daemon.managedAgentInstallationKey("gpu-nodes"), profile); err != nil {
		t.Fatalf("expected the managed profile to exist, got %v", err)
	}
	profile.Spec.Config.Features.GPU.Enabled = new(false)
	if err := kubeClient.Update(ctx, profile); err != nil {
		t.Fatalf("expected the managed profile to be updated, got %v", err)
	}

	var acknowledged map[string]any
	if err := json.Unmarshal(installRaw, &acknowledged); err != nil {
		t.Fatalf("failed to decode the install intent: %v", err)
	}
	acknowledged["acknowledgedOperationID"] = testAddonInstallOperationID
	acknowledgeRaw, err := json.Marshal(acknowledged)
	if err != nil {
		t.Fatalf("failed to encode the acknowledge intent: %v", err)
	}

	assertInstallErrorContains(
		t,
		daemon.handleManagedAgentInstallationIntent(ctx, managedAgentInstallationIntentSnapshot{raw: acknowledgeRaw}),
		"differs from the pinned",
	)

	intent, _, _, err := decodeManagedAgentInstallationIntent(acknowledgeRaw, validGenericManagedAgentInstallationIdentity)
	if err != nil {
		t.Fatalf("expected the acknowledge intent to decode, got %v", err)
	}
	if _, err := daemon.validateAcknowledgedManagedAgentInstallation(ctx, intent.AgentProfiles); err == nil {
		t.Fatal("expected the acknowledged installation validation to reject the drifted profile")
	} else if !strings.Contains(err.Error(), "differs from the pinned") {
		t.Fatalf("expected the pinned configuration error, got %v", err)
	}
}

func TestManagedAgentInstallationResourcesAbsentProfileListFailure(t *testing.T) {
	ctx := context.Background()
	daemon, kubeClient, _ := testManagedAgentInstallationDaemon(nil, testFleetCredentialSecret())
	daemon.apiReader = &managedAgentInstallationFaultClient{
		Client: kubeClient,
		listError: func(list client.ObjectList) error {
			if _, ok := list.(*v1alpha1.DatadogAgentProfileList); ok {
				return errors.New("agent profile list failed")
			}
			return nil
		},
	}

	_, err := daemon.managedAgentInstallationResourcesAbsent(ctx, managedAgentInstallationTarget, "")
	assertInstallErrorContains(t, err, "list managed DatadogAgentProfile resources")
}
