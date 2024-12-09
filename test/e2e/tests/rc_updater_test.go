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

func (u *updaterSuite) TestRcUpdaterK8s() {
	u.T().Run("RC Updater E2E Test", func(t *testing.T) {
		clusterName := u.BaseSuite.Env().KubernetesCluster.ClusterName
		helmOperatorTemplate := `remoteConfiguration:
  enabled: true
apiKeyExistingSecret: dda-datadog-credentials
appKeyExistingSecret: dda-datadog-credentials
clusterName: %s
site: datadoghq.com
`
		// Update environment to use remoteConfig on the operator and create the DDA

		operatorOpts := []operatorparams.Option{
			operatorparams.WithNamespace(common.NamespaceName),
			operatorparams.WithOperatorFullImagePath(common.OperatorImageName),
			operatorparams.WithHelmValues("installCRDs: false"),
			// Remote configuration requires API, app keys, cluster name and site.
			// TODO: verify api/app keys are RC enabled.
			// String literal as otherwise, `remoteConfiguration.enabled: true` is not used.
			// Cluster name is retrieved from stack context.
			operatorparams.WithHelmValues(fmt.Sprintf(helmOperatorTemplate, clusterName)),
		}
		ddaManifest := filepath.Join(common.ManifestsPath, "datadog-agent-rc-updater.yaml")
		ddaConfigPath, _ := common.GetAbsPath(ddaManifest)

		ddaOpts := []agentwithoperatorparams.Option{
			agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
				Name:         "datadog-agent-rc-updater",
				YamlFilePath: ddaConfigPath,
			}),
			agentwithoperatorparams.WithNamespace(common.NamespaceName),
			agentwithoperatorparams.WithTLSKubeletVerify(false),
		}

		provisionerOpts := []provisioners.KubernetesProvisionerOption{
			provisioners.WithK8sVersion(common.K8sVersion),
			// Disable fake intake for RC platform
			provisioners.WithoutFakeIntake(),
			provisioners.WithOperatorOptions(operatorOpts...),
			provisioners.WithDDAOptions(ddaOpts...),
		}

		u.UpdateEnv(provisioners.KubernetesProvisioner(provisioners.LocalKindRunFunc, provisionerOpts...))

		common.VerifyOperator(u.T(), u.kubectlOptions)

		u.EventuallyWithT(func(c *assert.CollectT) {
			common.VerifyAgentPods(t, u.kubectlOptions, common.NodeAgentSelector+",agent.datadoghq.com/name=datadog-agent-rc-updater")
		}, 60*time.Second, 10*time.Second, "Agent pods did not become ready in time.")

		configRequest := api.ConfigurationRequest{
			Data: api.ConfigurationData{
				Type: "configuration",
				Attributes: api.ConfigurationAttrs{
					Name:  "Enable All",
					Scope: fmt.Sprintf("cluster_name:%s", clusterName),
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

		u.EventuallyWithTf(func(c *assert.CollectT) {
			updater.CheckFeaturesState(u, c, clusterName, true)
		}, 20*time.Minute, 30*time.Second, "Checking if features were enabled timed out")
	})
}

func (u *updaterSuite) Client() *api.Client {
	return u.apiClient
}
