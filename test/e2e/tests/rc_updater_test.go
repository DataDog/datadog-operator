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
	"github.com/DataDog/test-infra-definitions/components/datadog/agentwithoperatorparams"
	"github.com/DataDog/test-infra-definitions/components/datadog/operatorparams"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-operator/test/e2e/provisioners"
	"github.com/DataDog/datadog-operator/test/e2e/rc-updater/api"
	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestUpdaterSuite(t *testing.T) {

	operatorOpts := []operatorparams.Option{
		operatorparams.WithNamespace(common.NamespaceName),
		operatorparams.WithOperatorFullImagePath(common.OperatorImageName),
		operatorparams.WithHelmValues("installCRDs: false"),
		// Remote configuration requires API, app keys, cluster name and site
		operatorparams.WithHelmValues("remoteConfiguration.enabled: true"),
		operatorparams.WithHelmValues("apiKeyExistingSecret: datadog-secret"),
		operatorparams.WithHelmValues("appKeyExistingSecret: datadog-secret"),
		operatorparams.WithHelmValues("clusterName: rc-updater-e2e-test-cluster"),
		operatorparams.WithHelmValues("site: datadoghq.com"),
	}

	ddaManifest := filepath.Join(common.ManifestsPath, "datadog-agent-rc-updater.yaml")
	ddaConfigPath, err := common.GetAbsPath(ddaManifest)
	assert.NoError(t, err, "Error retrieving dda config.")
	ddaOpts := []agentwithoperatorparams.Option{
		agentwithoperatorparams.WithNamespace(common.NamespaceName),
		agentwithoperatorparams.WithTLSKubeletVerify(false),
		agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
			Name:         "dda-rc-updater-e2e",
			YamlFilePath: ddaConfigPath,
		}),
	}

	provisionerOptions := []provisioners.KubernetesProvisionerOption{
		provisioners.WithK8sVersion(common.K8sVersion),
		provisioners.WithOperatorOptions(operatorOpts...),
		provisioners.WithDDAOptions(ddaOpts...),
	}

	e2eParams := []e2e.SuiteOption{
		e2e.WithStackName(fmt.Sprintf("operator-kind-rc-%s", common.K8sVersion)),
		e2e.WithProvisioner(provisioners.KubernetesProvisioner(provisioners.LocalKindRunFunc, provisionerOptions...)),
	}

	apiKey, _ := api.GetAPIKey()
	appKey, _ := api.GetAPPKey()
	require.NotEmpty(t, apiKey, "Could not get APIKey")
	require.NotEmpty(t, appKey, "Could not get APPKey")
	e2e.Run[provisioners.K8sEnv](t, &updaterSuite{clusterName: "rc-updater-e2e-test-cluster"}, e2eParams...)

}

func (u *updaterSuite) SetupSuite() {
	u.BaseSuite.SetupSuite()
	u.apiClient = api.NewClient()
	// cleanUpContext, err := common.ContextConfig(u.Env().Kubernetes.KubernetesCluster.KubeConfig)
	// u.Assert().NoError(err, "Error retrieving E2E kubeconfig.")
	kubeConfigPath, err := k8s.GetKubeConfigPathE(u.T())
	u.Require().NoError(err)
	// u.cleanUpContext = cleanUpContext
	u.kubectlOptions = k8s.NewKubectlOptions("", kubeConfigPath, common.NamespaceName)
	ddaManifest := filepath.Join(common.ManifestsPath, "datadog-agent-rc-updater.yaml")
	ddaConfigPath, err := common.GetAbsPath(ddaManifest)
	u.Assert().NoError(err, "Error retrieving dda config.")
	u.ddaConfigPath = ddaConfigPath
}

// func (u *updaterSuite) TearDownSuite() {
// 	teste2e.deleteDda(u.T(), u.kubectlOptions, u.ddaConfigPath)
// 	u.cleanUpContext()
// 	if u.configID != "" {
// 		u.Client().DeleteConfig(u.configID)
// 	}
// 	u.BaseSuite.TearDownSuite()

// }

func (u *updaterSuite) Clustername() string {
	return u.clusterName
}

func (u *updaterSuite) Client() *api.Client {
	return u.apiClient
}

func (u *updaterSuite) TestOperatorDeployed() {
	common.VerifyOperator(u.T(), u.kubectlOptions)

}

func (u *updaterSuite) TestAgentReady() {
	// k8s.KubectlApply(u.T(), u.kubectlOptions, u.ddaConfigPath)
	common.VerifyAgentPods(u.T(), u.kubectlOptions, common.NodeAgentSelector+",agent.datadoghq.com/e2e-test=datadog-agent-rc")
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
