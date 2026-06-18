// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package k8ssuite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentwithoperatorparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/operatorparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-operator/test/e2e/common"
	"github.com/DataDog/datadog-operator/test/e2e/provisioners"
	"github.com/DataDog/datadog-operator/test/e2e/tests/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	gkeAutopilotDDAName       = "datadog-agent-gke-autopilot"
	gkeAutopilotAgentSelector = common.NodeAgentSelector + ",agent.datadoghq.com/name=" + gkeAutopilotDDAName
	systemProbeContainerName  = "system-probe"
)

type gkeAutopilotSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestGKEAutopilotSuite(t *testing.T) {
	operatorOptions := []operatorparams.Option{
		operatorparams.WithNamespace(common.NamespaceName),
		operatorparams.WithOperatorFullImagePath(common.OperatorImageName),
		operatorparams.WithHelmValues(`installCRDs: false
rbac:
  create: false
serviceAccount:
  create: false
  name: datadog-operator-e2e-controller-manager
`),
	}

	ddaConfigPath, err := common.GetAbsPath(filepath.Join(common.ManifestsPath, "datadog-agent-gke-autopilot.yaml"))
	require.NoError(t, err)

	ddaOptions := []agentwithoperatorparams.Option{
		agentwithoperatorparams.WithNamespace(common.NamespaceName),
		agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
			Name:         gkeAutopilotDDAName,
			YamlFilePath: ddaConfigPath,
		}),
	}

	provisionerOptions := []provisioners.GKEProvisionerOption{
		provisioners.WithGKEName("operator-autopilot"),
		provisioners.WithGKETestName("e2e-operator-gke-autopilot"),
		provisioners.WithGKEK8sVersion(common.K8sVersion),
		provisioners.WithGKEOperatorOptions(operatorOptions...),
		provisioners.WithGKEDDAOptions(ddaOptions...),
		provisioners.WithGKEAutopilot(),
	}

	e2eOpts := []e2e.SuiteOption{
		e2e.WithStackName("operator-gke-autopilot"),
		e2e.WithProvisioner(provisioners.GKEProvisioner(provisionerOptions...)),
	}

	e2e.Run(t, &gkeAutopilotSuite{}, e2eOpts...)
}

func (s *gkeAutopilotSuite) TestAutopilotDDA() {
	s.T().Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		if k8sConfig := s.Env().KubernetesCluster.KubernetesClient.K8sConfig; k8sConfig != nil {
			if err := utils.DeleteAllDatadogResources(ctx, k8sConfig, common.NamespaceName); err != nil {
				s.T().Logf("Warning: failed to delete Datadog resources during cleanup: %v", err)
			}
		}
	})

	s.Run("Verify Operator", func() {
		s.Assert().EventuallyWithT(func(c *assert.CollectT) {
			utils.VerifyOperator(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client())
		}, 10*time.Minute, 15*time.Second, "could not validate operator pod in time")
	})

	s.Run("Verify Autopilot Agent", func() {
		s.Assert().EventuallyWithT(func(c *assert.CollectT) {
			utils.VerifyAgentPods(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), gkeAutopilotAgentSelector)
			utils.VerifyNumPodsForSelector(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), 1, common.ClusterAgentSelector+",agent.datadoghq.com/name="+gkeAutopilotDDAName)

			agentPods, err := s.runningAgentPods()
			assert.NoError(c, err)
			assert.NotEmpty(c, agentPods)

			for _, pod := range agentPods {
				assertContainerPresent(c, pod, systemProbeContainerName)

				output, _, err := s.Env().KubernetesCluster.KubernetesClient.PodExec(common.NamespaceName, pod.Name, coreAgentContainerName, agentStatusCommand("logs agent", "-j"))
				assert.NoError(c, err)
				utils.VerifyAgentPodLogs(c, output)
			}

			s.verifyAPILogs(c)
		}, 15*time.Minute, 30*time.Second, "could not validate GKE Autopilot Agent in time")
	})
}

func (s *gkeAutopilotSuite) runningAgentPods() ([]corev1.Pod, error) {
	agentPods, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(context.TODO(), metav1.ListOptions{
		LabelSelector: gkeAutopilotAgentSelector,
		FieldSelector: "status.phase=Running",
	})
	if err != nil {
		return nil, err
	}

	return agentPods.Items, nil
}

func (s *gkeAutopilotSuite) verifyAPILogs(t assert.TestingT) {
	logs, err := s.Env().FakeIntake.Client().FilterLogs("agent")
	assert.NoError(t, err)
	assert.NotEmptyf(t, logs, "expected fake intake-ingested logs to not be empty")
}
