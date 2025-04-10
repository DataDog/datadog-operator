// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package enabledefault

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/test"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/pkg/testutils"
)

type InstallInfoData struct {
	InstallMethod InstallMethod `yaml:"install_method"`
}

type InstallMethod struct {
	Tool             string `yaml:"tool"`
	ToolVersion      string `yaml:"tool_version"`
	InstallerVersion string `yaml:"installer_version"`
}

func Test_getInstallInfoValue(t *testing.T) {
	tests := []struct {
		name                   string
		toolVersionEnvVarValue string
		expectedToolVersion    string
	}{
		{
			name:                   "Env var empty/unset (os.Getenv returns unset env var as empty string)",
			toolVersionEnvVarValue: "",
			expectedToolVersion:    "unknown",
		},
		{
			name:                   "Env var set",
			toolVersionEnvVarValue: "foo",
			expectedToolVersion:    "foo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(InstallInfoToolVersion, tt.toolVersionEnvVarValue)
			installInfo := InstallInfoData{}

			test := getInstallInfoValue()

			err := yaml.Unmarshal([]byte(test), &installInfo)
			assert.NoError(t, err)

			assert.Equal(t, "datadog-operator", installInfo.InstallMethod.Tool)
			assert.Equal(t, tt.expectedToolVersion, installInfo.InstallMethod.ToolVersion)
			assert.Equal(t, "0.0.0", installInfo.InstallMethod.InstallerVersion)
		})
	}
}

func Test_defaultFeature_ManageClusterAgent(t *testing.T) {
	tests := test.FeatureTestSuite{
		{
			Name: "Manage Cluster Agent service account name env variable",
			DDA: testutils.NewDatadogAgentBuilder().
				WithName("datadog").
				WithEventCollectionKubernetesEvents(true).
				Build(),
			WantConfigure: true,
			ClusterAgent:  test.NewDefaultComponentTest().WithWantFunc(defaultFeatureManageClusterAgentWantFunc),
		},
	}

	tests.Run(t, buildDefaultFeature)
}

func Test_defaultFeature_ADP(t *testing.T) {
	adpEnabledEnvVar := &corev1.EnvVar{
		Name:  common.DDADPEnabled,
		Value: "true",
	}

	tests := test.FeatureTestSuite{
		{
			Name: "adp disabled (default)",
			DDA: testutils.NewDatadogAgentBuilder().
				BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
					assert.NotContains(t, agentEnvVars, adpEnabledEnvVar, "DD_ADP_ENABLED should not be set to true when ADP is not enabled")
				},
			),
		},
		{
			Name: "adp disabled (forced)",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAnnotations(map[string]string{
					utils.EnableADPAnnotation: "false",
				}).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
					assert.NotContains(t, agentEnvVars, adpEnabledEnvVar, "DD_ADP_ENABLED should not be set to true when ADP is not enabled")
				},
			),
		},
		{
			Name: "adp enabled",
			DDA: testutils.NewDatadogAgentBuilder().
				WithAnnotations(map[string]string{
					utils.EnableADPAnnotation: "true",
				}).
				BuildWithDefaults(),
			WantConfigure: true,
			Agent: test.NewDefaultComponentTest().WithWantFunc(
				func(t testing.TB, mgrInterface feature.PodTemplateManagers) {
					mgr := mgrInterface.(*fake.PodTemplateManagers)
					agentEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.CoreAgentContainerName]
					assert.Contains(t, agentEnvVars, adpEnabledEnvVar, "DD_ADP_ENABLED should be set to true when ADP is enabled")
				},
			),
		},
	}

	tests.Run(t, buildDefaultFeature)
}

func defaultFeatureManageClusterAgentWantFunc(t testing.TB, mgrInterface feature.PodTemplateManagers) {
	mgr := mgrInterface.(*fake.PodTemplateManagers)
	dcaEnvVars := mgr.EnvVarMgr.EnvVarsByC[apicommon.AllContainers]

	want := &corev1.EnvVar{
		Name:  DDClusterAgentServiceAccountName,
		Value: "datadog-cluster-agent",
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("couldn't marshal the DCA service account name env variable: %v", err)
		return
	}

	// look for the service account name environment variable
	for _, in := range dcaEnvVars {
		if in.Name == want.Name {
			inJSON, err := json.Marshal(in)
			if err != nil {
				t.Fatalf("couldn't marshal env variable: %v", err)
				return
			}

			assert.Equal(t, string(wantJSON), string(inJSON), "wrong DCA service account name env \ndiff = %s", cmp.Diff(string(wantJSON), string(inJSON)))
			return
		}
	}
	t.Fatalf("Service account name missing in DCA envvars")
}
