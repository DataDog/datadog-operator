// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetesactions

import (
	"strings"
	"testing"

	"github.com/go-logr/logr/funcr"
	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/pkg/testutils"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"
)

func Test_kubernetesActionsFeature_Configure(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name:          "KubernetesActions not configured",
			DDA:           testutils.NewDatadogAgentBuilder().Build(),
			WantConfigure: false,
		},
		{
			Name: "KubernetesActions explicitly disabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithKubernetesActionsEnabled(false).
				Build(),
			WantConfigure: false,
		},
		{
			Name: "KubernetesActions enabled but cluster agent version too low",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithKubernetesActionsEnabled(true).
				WithClusterAgentTag("7.83.0").
				Build(),
			WantConfigure: false,
		},
		{
			Name: "KubernetesActions enabled at minimum cluster agent version",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithKubernetesActionsEnabled(true).
				WithClusterAgentTag("7.84.0").
				Build(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(kubernetesActionsClusterAgentWantFunc),
		},
		{
			Name: "KubernetesActions enabled on a minimum cluster agent release candidate",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithKubernetesActionsEnabled(true).
				WithClusterAgentTag("7.84.0-rc.1").
				Build(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(kubernetesActionsClusterAgentWantFunc),
		},
		{
			Name: "KubernetesActions enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithKubernetesActionsEnabled(true).
				Build(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(kubernetesActionsClusterAgentWantFunc),
		},
		{
			Name: "KubernetesActions enabled with RC and PAR",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithKubernetesActionsEnabled(true).
				WithKubernetesActionsCommunication(true, true).
				Build(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(kubernetesActionsClusterAgentWantFunc),
		},
		{
			// Older Cluster Agents always use Remote Configuration: rc=false is ignored and
			// no channel setting is passed.
			Name: "KubernetesActions enabled with PAR only on a cluster agent without channel settings",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithKubernetesActionsEnabled(true).
				WithKubernetesActionsCommunication(false, true).
				WithClusterAgentTag("7.85.0").
				Build(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(kubernetesActionsClusterAgentWantFunc),
		},
		{
			Name: "KubernetesActions enabled with PAR only",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithKubernetesActionsEnabled(true).
				WithKubernetesActionsCommunication(false, true).
				WithClusterAgentTag("7.86.0").
				Build(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(kubernetesActionsPAROnlyClusterAgentWantFunc),
		},
		{
			// Both channels are on by default: no channel setting is passed.
			Name: "KubernetesActions enabled with default channels on a cluster agent with channel settings",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithKubernetesActionsEnabled(true).
				WithClusterAgentTag("7.86.0").
				Build(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(kubernetesActionsClusterAgentWantFunc),
		},
		{
			Name: "KubernetesActions enabled with RC only",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithKubernetesActionsEnabled(true).
				WithKubernetesActionsCommunication(true, false).
				WithClusterAgentTag("7.86.0").
				Build(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(kubernetesActionsRCOnlyClusterAgentWantFunc),
		},
		{
			Name: "KubernetesActions enabled with RC and PAR on a cluster agent with channel settings",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithKubernetesActionsEnabled(true).
				WithKubernetesActionsCommunication(true, true).
				WithClusterAgentTag("7.86.0").
				Build(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(kubernetesActionsClusterAgentWantFunc),
		},
		{
			// Remote Configuration disabled globally: the feature is still configured (the
			// Cluster Agent refuses to start, which is intended) and the operator logs why.
			Name: "KubernetesActions enabled with Remote Configuration disabled globally",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithKubernetesActionsEnabled(true).
				WithRemoteConfigEnabled(false).
				Build(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(kubernetesActionsClusterAgentWantFunc),
		},
		{
			Name: "KubernetesActions enabled with no communication channel",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("ddaDCA").
				WithKubernetesActionsEnabled(true).
				WithKubernetesActionsCommunication(false, false).
				Build(),
			WantConfigure: false,
		},
	}

	tests.Run(t, buildKubernetesActionsFeature)
}

func TestKubernetesActionsAutoEnabledForRemediation(t *testing.T) {
	enabled := &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true), KubernetesRemediationsEnabled: new(true)}
	for _, tt := range []struct {
		name           string
		par            *v2alpha1.PrivateActionRunnerFeatureConfig
		actions        *v2alpha1.KubernetesActionsFeatureConfig
		annotations    map[string]string
		version        string
		wantEnabled    bool
		wantRCDisabled bool
	}{
		{name: "typed remediation", par: enabled, wantEnabled: true},
		{name: "empty actions block", par: enabled, actions: &v2alpha1.KubernetesActionsFeatureConfig{}, wantEnabled: true},
		{name: "cluster PAR only", par: &v2alpha1.PrivateActionRunnerFeatureConfig{ClusterAgent: &v2alpha1.PrivateActionRunnerComponentConfig{Enabled: new(true)}, KubernetesRemediationsEnabled: new(true)}, wantEnabled: true},
		{name: "explicit disable", par: enabled, actions: &v2alpha1.KubernetesActionsFeatureConfig{Enabled: new(false)}},
		{name: "disabled PAR", par: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(false), KubernetesRemediationsEnabled: new(true)}},
		{name: "disabled cluster PAR", par: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true), KubernetesRemediationsEnabled: new(true), ClusterAgent: &v2alpha1.PrivateActionRunnerComponentConfig{Enabled: new(false)}}},
		{name: "disabled remediation", par: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true), KubernetesRemediationsEnabled: new(false)}},
		{name: "legacy annotation", annotations: map[string]string{featureutils.EnableClusterAgentPrivateActionRunnerAnnotation: "true", featureutils.ClusterAgentPrivateActionRunnerK8sRemediationEnabled: "true"}, wantEnabled: true},
		{name: "annotation fallback with typed PAR", par: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true)}, annotations: map[string]string{featureutils.ClusterAgentPrivateActionRunnerK8sRemediationEnabled: "true"}, wantEnabled: true},
		{name: "typed false overrides annotation", par: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true), KubernetesRemediationsEnabled: new(false)}, annotations: map[string]string{featureutils.ClusterAgentPrivateActionRunnerK8sRemediationEnabled: "true"}},
		{name: "too old for Kubernetes Actions", par: enabled, version: "7.83.0"},
		{name: "preserve channel settings", par: enabled, actions: &v2alpha1.KubernetesActionsFeatureConfig{Communication: &v2alpha1.KubernetesActionsCommunicationConfig{RemoteConfiguration: &v2alpha1.KubernetesActionsRemoteConfigurationConfig{Enabled: new(false)}}}, wantEnabled: true, wantRCDisabled: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			version := tt.version
			if version == "" {
				version = "7.86.0"
			}
			dda := testutils.NewDatadogAgentBuilder().WithClusterAgentTag(version).Build()
			dda.Annotations = tt.annotations
			if dda.Spec.Features == nil {
				dda.Spec.Features = &v2alpha1.DatadogFeatures{}
			}
			dda.Spec.Features.PrivateActionRunner = tt.par
			dda.Spec.Features.KubernetesActions = tt.actions
			before := dda.DeepCopy()
			f := buildKubernetesActionsFeature(nil)
			req := f.Configure(dda, &dda.Spec, nil)
			assert.Equal(t, tt.wantEnabled, req.ClusterAgent.IsEnabled())
			assert.Equal(t, before, dda)
			if tt.wantEnabled {
				managers := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})
				assert.NoError(t, f.ManageClusterAgent(managers))
				if tt.wantRCDisabled {
					kubernetesActionsPAROnlyClusterAgentWantFunc(t, managers)
				} else {
					kubernetesActionsClusterAgentWantFunc(t, managers)
				}
			}
		})
	}
}

// kubernetesActionsClusterAgentWantFunc expects only the feature switch: both channels are on
// (the agent default), or the Cluster Agent predates the channel settings.
func kubernetesActionsClusterAgentWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	assertKubernetesActionsEnvVars(t, mgrInterface)
}

func kubernetesActionsPAROnlyClusterAgentWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	assertKubernetesActionsEnvVars(t, mgrInterface, &corev1.EnvVar{Name: DDKubeActionsRemoteConfigEnabled, Value: "false"})
}

func kubernetesActionsRCOnlyClusterAgentWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	assertKubernetesActionsEnvVars(t, mgrInterface, &corev1.EnvVar{Name: DDKubeActionsPAREnabled, Value: "false"})
}

func assertKubernetesActionsEnvVars(t testing.TB, mgrInterface feature.PodTemplateManagers, channelEnvVars ...*corev1.EnvVar) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.ClusterAgentContainerName]

	want := append([]*corev1.EnvVar{
		{
			Name:  DDKubeActionsEnabled,
			Value: "true",
		},
	}, channelEnvVars...)
	assert.True(t, apiutils.IsEqualStruct(dcaEnvVars, want), "DCA envvars \ndiff = %s", cmp.Diff(dcaEnvVars, want))
}

func Test_kubernetesActionsFeature_NoOpManagers(t *testing.T) {
	f := buildKubernetesActionsFeature(nil)

	assert.NoError(t, f.ManageSingleContainerNodeAgent(nil))
	assert.NoError(t, f.ManageNodeAgent(nil))
	assert.NoError(t, f.ManageClusterChecksRunner(nil))
	assert.NoError(t, f.ManageOtelAgentGateway(nil))
}

func TestKubernetesActionsPARLogs(t *testing.T) {
	const disabledPARWarning = "Warning: Kubernetes Actions communication.privateActionRunner.enabled is true, but features.privateActionRunner disables the Private Action Runner on the Cluster Agent; Kubernetes actions cannot be delivered through Private Action Runner"
	const missingPARMessage = "Kubernetes Actions communication.privateActionRunner.enabled is true and features.privateActionRunner is not configured; the Private Action Runner is enabled automatically on the Cluster Agent"
	disabled := &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(false)}
	for _, tt := range []struct {
		name            string
		par             *v2alpha1.PrivateActionRunnerFeatureConfig
		communication   *v2alpha1.KubernetesActionsCommunicationConfig
		actionsDisabled bool
		oldAgent        bool
		verbosity       int
		wantMessage     string
	}{
		{name: "shared PAR disabled with default channels", par: disabled, wantMessage: disabledPARWarning},
		{name: "empty PAR block disables automatic activation", par: &v2alpha1.PrivateActionRunnerFeatureConfig{}, wantMessage: disabledPARWarning},
		{name: "cluster runner explicitly disabled", par: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true), ClusterAgent: &v2alpha1.PrivateActionRunnerComponentConfig{Enabled: new(false)}}, wantMessage: disabledPARWarning},
		{name: "only node runner enabled", par: &v2alpha1.PrivateActionRunnerFeatureConfig{NodeAgent: &v2alpha1.PrivateActionRunnerComponentConfig{Enabled: new(true)}}, wantMessage: disabledPARWarning},
		{name: "PAR-only channel with disabled runner", par: disabled, communication: &v2alpha1.KubernetesActionsCommunicationConfig{RemoteConfiguration: &v2alpha1.KubernetesActionsRemoteConfigurationConfig{Enabled: new(false)}, PrivateActionRunner: &v2alpha1.KubernetesActionsPrivateActionRunnerConfig{Enabled: new(true)}}, wantMessage: disabledPARWarning},
		{name: "PAR feature absent at default log level"},
		{name: "PAR feature absent with default channels", verbosity: 1, wantMessage: missingPARMessage},
		{name: "PAR feature absent with explicit PAR channel", communication: &v2alpha1.KubernetesActionsCommunicationConfig{PrivateActionRunner: &v2alpha1.KubernetesActionsPrivateActionRunnerConfig{Enabled: new(true)}}, verbosity: 1, wantMessage: missingPARMessage},
		{name: "PAR feature absent with defaulted PAR channel", communication: &v2alpha1.KubernetesActionsCommunicationConfig{PrivateActionRunner: &v2alpha1.KubernetesActionsPrivateActionRunnerConfig{}}, verbosity: 1, wantMessage: missingPARMessage},
		{name: "PAR feature absent with PAR channel disabled", communication: &v2alpha1.KubernetesActionsCommunicationConfig{PrivateActionRunner: &v2alpha1.KubernetesActionsPrivateActionRunnerConfig{Enabled: new(false)}}},
		{name: "PAR feature absent with Kubernetes Actions disabled", actionsDisabled: true},
		{name: "PAR feature absent with unsupported Cluster Agent version", oldAgent: true},
		{name: "shared PAR enabled", par: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true)}},
		{name: "empty cluster settings inherit shared enablement", par: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(true), ClusterAgent: &v2alpha1.PrivateActionRunnerComponentConfig{}}},
		{name: "cluster runner enabled over shared disable", par: &v2alpha1.PrivateActionRunnerFeatureConfig{Enabled: new(false), ClusterAgent: &v2alpha1.PrivateActionRunnerComponentConfig{Enabled: new(true)}}},
		{name: "cluster runner enabled without shared enablement", par: &v2alpha1.PrivateActionRunnerFeatureConfig{ClusterAgent: &v2alpha1.PrivateActionRunnerComponentConfig{Enabled: new(true)}}},
		{name: "PAR delivery channel disabled", par: disabled, communication: &v2alpha1.KubernetesActionsCommunicationConfig{PrivateActionRunner: &v2alpha1.KubernetesActionsPrivateActionRunnerConfig{Enabled: new(false)}}},
		{name: "Kubernetes Actions disabled", par: disabled, actionsDisabled: true},
		{name: "unsupported Cluster Agent version", par: disabled, oldAgent: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var messages []string
			logger := funcr.New(func(_, message string) {
				if strings.Contains(message, "Kubernetes Actions communication.privateActionRunner.enabled") {
					messages = append(messages, message)
				}
			}, funcr.Options{Verbosity: tt.verbosity})
			dda := testutils.NewDatadogAgentBuilder().WithKubernetesActionsEnabled(!tt.actionsDisabled).WithClusterAgentTag("7.86.0").Build()
			dda.Spec.Features.PrivateActionRunner = tt.par
			dda.Spec.Features.KubernetesActions.Communication = tt.communication
			if tt.oldAgent {
				dda.Spec.Override[v2alpha1.ClusterAgentComponentName].Image.Tag = "7.83.0"
			}
			f := buildKubernetesActionsFeature(&feature.Options{Logger: logger}).(*kubernetesActionsFeature)
			req := f.Configure(dda, &dda.Spec, nil)
			if tt.wantMessage != "" {
				if assert.Len(t, messages, 1) {
					assert.Contains(t, messages[0], tt.wantMessage)
				}
				assert.True(t, req.ClusterAgent.IsEnabled(), "logging must not disable Kubernetes Actions")
				assert.True(t, f.parEnabled, "logging must not change the requested delivery channel")
			} else {
				assert.Empty(t, messages)
			}
		})
	}
}
