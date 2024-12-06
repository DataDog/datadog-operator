// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build e2e
// +build e2e

package e2e

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/datadog-operator/test/e2e/common"
	updater "github.com/DataDog/datadog-operator/test/e2e/rc-updater"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-operator/test/e2e/provisioners"
	"github.com/DataDog/datadog-operator/test/e2e/rc-updater/api"
	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type updaterSuite struct {
	e2e.BaseSuite[provisioners.K8sEnv]
	apiClient      *api.Client
	kubectlOptions *k8s.KubectlOptions
	// cleanUpContext func()
	ddaConfigPath string
	clusterName   string
	configID      string
}

func (u *updaterSuite) SetupSuite() {
	u.BaseSuite.SetupSuite()
	u.apiClient = api.NewClient()
	kubeConfigPath, err := k8s.GetKubeConfigPathE(u.T())
	u.Require().NoError(err)
	u.kubectlOptions = k8s.NewKubectlOptions("", kubeConfigPath, common.NamespaceName)
	ddaManifest := filepath.Join(common.ManifestsPath, "datadog-agent-rc-updater.yaml")
	ddaConfigPath, err := common.GetAbsPath(ddaManifest)
	u.Assert().NoError(err, "Error retrieving dda config.")
	u.ddaConfigPath = ddaConfigPath
}

func TestRcUpdaterK8sSuite(t *testing.T) {
	// TODO: clusterName dynamic
	suite.Run(t, &updaterSuite{clusterName: "rc-updater-e2e-test-cluster"})
}

func (u *updaterSuite) TestRcUpdaterK8s() {
	u.T().Run("RC Updater E2E Test", func(t *testing.T) {
		common.VerifyOperator(u.T(), u.kubectlOptions)

		u.EventuallyWithT(func(c *assert.CollectT) {
			common.VerifyAgentPods(t, u.kubectlOptions, common.NodeAgentSelector+",agent.datadoghq.com/name=datadog-agent-rc-updater")
		}, 60*time.Second, 10*time.Second, "Agent pods did not become ready in time.")

		// TODO: move the functions over here (enableFeatures, testFeaturesEnabled)
	})
}

func (u *updaterSuite) Clustername() string {
	return u.clusterName
}

func (u *updaterSuite) Client() *api.Client {
	return u.apiClient
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
