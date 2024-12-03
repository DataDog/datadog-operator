// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	teste2e "github.com/DataDog/datadog-operator/test/e2e"
	"github.com/DataDog/datadog-operator/test/e2e/rc-updater"
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-operator/test/e2e/rc-updater/api"
	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type updaterSuite struct {
	e2e.BaseSuite[teste2e.kindEnv]
	apiClient      *api.Client
	kubectlOptions *k8s.KubectlOptions
	cleanUpContext func()
	ddaConfigPath  string
	clusterName    string
	configID       string
}

func TestUpdaterSuite(t *testing.T) {

	e2eParams := []e2e.SuiteOption{
		e2e.WithStackName(fmt.Sprintf("operator-kind-rc-%s", teste2e.k8sVersion)),
		e2e.WithProvisioner(kindProvisioner(teste2e.k8sVersion, []string{"rc-e2e-manager.yaml"})),
	}

	apiKey, _ := api.GetAPIKey()
	appKey, _ := api.GetAPPKey()
	require.NotEmpty(t, apiKey, "Could not get APIKey")
	require.NotEmpty(t, appKey, "Could not get APPKey")
	e2e.Run[teste2e.kindEnv](t, &updaterSuite{clusterName: "rc-updater-e2e-test-cluster"}, e2eParams...)

}

func (u *updaterSuite) SetupSuite() {
	u.BaseSuite.SetupSuite()
	u.apiClient = api.NewClient()
	cleanUpContext, err := teste2e.contextConfig(u.Env().Kind.ClusterOutput.KubeConfig)
	u.Assert().NoError(err, "Error retrieving E2E kubeconfig.")
	u.cleanUpContext = cleanUpContext
	u.kubectlOptions = k8s.NewKubectlOptions("", teste2e.kubeConfigPath, teste2e.namespaceName)
	ddaManifest := filepath.Join(teste2e.manifestsPath, "datadog-agent-rc-updater.yaml")
	ddaConfigPath, err := teste2e.getAbsPath(ddaManifest)
	u.Assert().NoError(err, "Error retrieving dda config.")
	u.ddaConfigPath = ddaConfigPath

}

func (u *updaterSuite) TearDownSuite() {
	teste2e.deleteDda(u.T(), u.kubectlOptions, u.ddaConfigPath)
	u.cleanUpContext()
	if u.configID != "" {
		u.Client().DeleteConfig(u.configID)
	}
	u.BaseSuite.TearDownSuite()

}

func (u *updaterSuite) Clustername() string {
	return u.clusterName
}

func (u *updaterSuite) Client() *api.Client {
	return u.apiClient
}

func (u *updaterSuite) TestOperatorDeployed() {
	teste2e.verifyOperator(u.T(), u.kubectlOptions)

}

func (u *updaterSuite) TestAgentReady() {
	k8s.KubectlApply(u.T(), u.kubectlOptions, u.ddaConfigPath)
	teste2e.verifyAgentPods(u.T(), u.kubectlOptions, teste2e.nodeAgentSelector+",agent.datadoghq.com/e2e-test=datadog-agent-rc")
}

func (u *updaterSuite) TestEnableFeatures() {
	// Wait for the agent to be deployed
	time.Sleep(3 * time.Minute)

	configRequest := api.ConfigurationRequest{
		Data: api.ConfigurationData{
			Type: "configuration",
			Attributes: api.ConfigurationAttrs{
				Name:  "Enable All",
				Scope: fmt.Sprintf("cluster_name:%s", u.Clustername()),
				Parameters: api.ConfigurationParams{
					CloudWorkloadSecurity:             true,
					CloudSecurityPostureManagement:    true,
					HostsVulnerabilityManagement:      true,
					ContainersVulnerabilityManagement: true,
					UniversalServiceMonitoring:        true,
				},
				Enabled: true,
			},
		},
	}

	resp, err := u.Client().ApplyConfig(configRequest)
	require.NoError(u.T(), err, "Failed to apply config")
	u.configID = resp.Data.ID
	updater.TestConfigsContent(u.T(), resp.Data.Attributes.Content)

}

func (u *updaterSuite) TestFeaturesEnabled() {
	u.EventuallyWithTf(func(c *assert.CollectT) {
		updater.CheckFeaturesState(u, c, u.Clustername(), true)
	}, 20*time.Minute, 30*time.Second, "Checking if features were enabled timed out")
}
