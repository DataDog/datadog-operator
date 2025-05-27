// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build e2e

package tests

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-operator/test/e2e/common"
	"github.com/DataDog/datadog-operator/test/e2e/provisioners"
	"github.com/DataDog/datadog-operator/test/e2e/tests/utils"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentwithoperatorparams"
	"github.com/DataDog/test-infra-definitions/components/datadog/operatorparams"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	updater "github.com/DataDog/datadog-operator/test/e2e/rc-updater"
	"github.com/DataDog/datadog-operator/test/e2e/rc-updater/api"
	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var defaultOperatorOpts = []operatorparams.Option{
	operatorparams.WithNamespace(common.NamespaceName),
	operatorparams.WithOperatorFullImagePath(common.OperatorImageName),
	operatorparams.WithHelmValues("installCRDs: false"),
}

var defaultDDAOpts = []agentwithoperatorparams.Option{
	agentwithoperatorparams.WithNamespace(common.NamespaceName),
}

type updaterSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
	local          bool
	apiClient      *api.Client
	kubectlOptions *k8s.KubectlOptions
	cleanUpContext func()
	ddaConfigPath  string
	clusterName    string
	configID       string
}

func TestUpdaterSuite(t *testing.T) {
	apiKey, _ := api.GetAPIKey()
	appKey, _ := api.GetAPPKey()
	require.NotEmpty(t, apiKey, "Could not get APIKey")
	require.NotEmpty(t, appKey, "Could not get APPKey")

	operatorOptions := []operatorparams.Option{
		operatorparams.WithNamespace(common.NamespaceName),
		operatorparams.WithOperatorFullImagePath(common.OperatorImageName),
		operatorparams.WithHelmValues(`
installCRDs: false
remoteConfiguration:
  enabled: true
`),
	}

	provisionerOptions := []provisioners.KubernetesProvisionerOption{
		provisioners.WithTestName("e2e-rc-updater"),
		provisioners.WithOperatorOptions(operatorOptions...),
		provisioners.WithoutDDA(),
	}

	e2eOpts := []e2e.SuiteOption{
		e2e.WithStackName(fmt.Sprintf("operator-kind-rc-%s", strings.ReplaceAll(common.K8sVersion, ".", "-"))),
		e2e.WithProvisioner(provisioners.KubernetesProvisioner(provisionerOptions...)),
		e2e.WithSkipDeleteOnFailure(),
		e2e.WithDevMode(),
	}

	e2e.Run(t, &updaterSuite{clusterName: "rc-updater-e2e-test-cluster"}, e2eOpts...)

}

// apiClient: datadog api client
// dda manifest
func (u *updaterSuite) SetupSuite() {
	u.apiClient = api.NewClient()
	ddaManifest := filepath.Join(common.ManifestsPath, "datadog-agent-rc-updater.yaml")
	ddaConfigPath, err := common.GetAbsPath(ddaManifest)
	u.Assert().NoError(err, "Error retrieving dda config.")
	u.ddaConfigPath = ddaConfigPath

}

func (u *updaterSuite) Clustername() string {
	return u.clusterName
}

func (u *updaterSuite) Client() *api.Client {
	return u.apiClient
}

func (u *updaterSuite) TestOperatorDeployed() {
	utils.VerifyOperator(u.T(), nil, common.NamespaceName, u.Env().KubernetesCluster.Client())
}

// apply the dda manifest and then verify the agent is ready
func (u *updaterSuite) TestAgentReady() {
	ddaConfigPath, err := common.GetAbsPath(u.ddaConfigPath)
	assert.NoError(u.T(), err)

	ddaOpts := []agentwithoperatorparams.Option{
		agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
			Name:         "rc-updater-dda",
			YamlFilePath: path.Join(ddaConfigPath, "datadog-agent-rc-updater.yaml"),
		}),
	}
	ddaOpts = append(ddaOpts, defaultDDAOpts...)

	provisionerOptions := []provisioners.KubernetesProvisionerOption{
		provisioners.WithTestName("e2e-operator-ksm-ccr"),
		provisioners.WithK8sVersion(common.K8sVersion),
		provisioners.WithOperatorOptions(defaultOperatorOpts...),
		provisioners.WithDDAOptions(ddaOpts...),
		provisioners.WithLocal(u.local),
	}

	u.UpdateEnv(provisioners.KubernetesProvisioner(provisionerOptions...))
	utils.VerifyAgentPods(u.T(), nil, common.NamespaceName, u.Env().KubernetesCluster.Client(), common.NodeAgentSelector+",agent.datadoghq.com/e2e-test=datadog-agent-rc")
}

// make an api call
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
