// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/suite"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-operator/test/e2e/common"
	"github.com/DataDog/datadog-operator/test/e2e/provisioners"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentwithoperatorparams"
	"github.com/DataDog/test-infra-definitions/components/datadog/fakeintake"
	"github.com/DataDog/test-infra-definitions/components/datadog/operatorparams"
	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	restclient "k8s.io/client-go/rest"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/stretchr/testify/assert"
)

type k8sSuite struct {
	e2e.BaseSuite[provisioners.K8sEnv]
	datadogClient

	kubeProvider *kubernetes.Provider
	fakeintake   *fakeintake.Fakeintake

	K8sConfig *restclient.Config
	//K8sClient kubernetes.Interface
}

type datadogClient struct {
	ctx        context.Context
	metricsApi *datadogV1.MetricsApi
	logsApi    *datadogV1.LogsApi
}

func (suite *k8sSuite) SetupSuite() {
	apiKey, err := runner.GetProfile().SecretStore().Get(parameters.APIKey)
	suite.Require().NoError(err)
	appKey, err := runner.GetProfile().SecretStore().Get(parameters.APPKey)
	suite.Require().NoError(err)
	suite.datadogClient.ctx = context.WithValue(
		context.Background(),
		datadog.ContextAPIKeys,
		map[string]datadog.APIKey{
			"apiKeyAuth": {
				Key: apiKey,
			},
			"appKeyAuth": {
				Key: appKey,
			},
		},
	)
	configuration := datadog.NewConfiguration()
	client := datadog.NewAPIClient(configuration)
	suite.datadogClient.metricsApi = datadogV1.NewMetricsApi(client)
	suite.datadogClient.logsApi = datadogV1.NewLogsApi(client)
}

func TestK8sSuite(t *testing.T) {
	suite.Run(t, &k8sSuite{})
}

func (s *k8sSuite) TestGenericK8s() {
	var ddaConfigPath string
	var kubectlOptions *k8s.KubectlOptions
	kubeConfigPath, err := k8s.GetKubeConfigPathE(s.T())
	s.Require().NoError(err)
	kubectlOptions = k8s.NewKubectlOptions("", kubeConfigPath, common.NamespaceName)

	s.T().Run("Minimal DDA config", func(t *testing.T) {
		common.VerifyOperator(t, kubectlOptions)

		// Install DDA
		ddaConfigPath, err = common.GetAbsPath(common.DdaMinimalPath)
		assert.NoError(t, err)

		operatorOpts := []operatorparams.Option{
			operatorparams.WithNamespace(common.NamespaceName),
			operatorparams.WithOperatorFullImagePath(common.OperatorImageName),
			operatorparams.WithHelmValues("installCRDs: false"),
		}

		ddaOpts := []agentwithoperatorparams.Option{
			agentwithoperatorparams.WithNamespace(common.NamespaceName),
			agentwithoperatorparams.WithTLSKubeletVerify(false),
			agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
				Name:         "dda-minimum",
				YamlFilePath: ddaConfigPath,
			}),
		}

		provisionerOptions := []provisioners.KubernetesProvisionerOption{
			provisioners.WithK8sVersion(common.K8sVersion),
			provisioners.WithOperatorOptions(operatorOpts...),
			provisioners.WithDDAOptions(ddaOpts...),
		}

		s.UpdateEnv(provisioners.KubernetesProvisioner(provisioners.LocalKindRunFunc, provisionerOptions...))

		s.EventuallyWithT(func(c *assert.CollectT) {
			common.VerifyAgentPods(t, kubectlOptions, common.NodeAgentSelector+",agent.datadoghq.com/name=dda-minimum")
			common.VerifyNumPodsForSelector(t, kubectlOptions, 1, common.ClusterAgentSelector+",agent.datadoghq.com/name=dda-minimum")

		}, 900*time.Second, 10*time.Second, "Agent pods did not become ready in time.")

		agentPods, err := k8s.ListPodsE(t, kubectlOptions, v1.ListOptions{
			LabelSelector: common.NodeAgentSelector + ",agent.datadoghq.com/name=dda-minimum",
			FieldSelector: "status.phase=Running",
		})
		assert.NoError(t, err)

		s.EventuallyWithTf(func(c *assert.CollectT) {
			for _, pod := range agentPods {
				output, err := k8s.RunKubectlAndGetOutputE(t, kubectlOptions, "exec", "-it", pod.Name, "--", "agent", "status", "collector", "-j")
				assert.NoError(c, err)

				verifyCheck(c, output, "kubelet")
			}
		}, 900*time.Second, 30*time.Second, "could not validate kubelet check on agent pod")

		s.EventuallyWithTf(func(c *assert.CollectT) {
			metricNames, err := s.Env().FakeIntake.Client().GetMetricNames()
			s.Assert().NoError(err)
			s.Assert().Contains(metricNames, "kubernetes.cpu.usage.total")

			metrics, err := s.Env().FakeIntake.Client().FilterMetrics("kubernetes.cpu.usage.total")
			s.Assert().NoError(err)
			for _, metric := range metrics {
				for _, points := range metric.Points {
					s.Assert().Greater(points.Value, float64(0))
				}
			}
		}, 600*time.Second, 30*time.Second, "Could not verify kubelet metrics in time")

		s.EventuallyWithTf(func(c *assert.CollectT) {
			clusterAgentPods, err := k8s.ListPodsE(t, kubectlOptions, v1.ListOptions{
				LabelSelector: common.ClusterAgentSelector + ",agent.datadoghq.com/e2e-test=datadog-agent-minimum",
			})
			assert.NoError(t, err)

			for _, pod := range clusterAgentPods {
				k8s.WaitUntilPodAvailable(t, kubectlOptions, pod.Name, 9, 15*time.Second)
				output, err := k8s.RunKubectlAndGetOutputE(t, kubectlOptions, "exec", "-it", pod.Name, "--", "agent", "status", "collector", "-j")
				assert.NoError(t, err)

				verifyCheck(c, output, "kubernetes_state_core")
			}
		}, 1200*time.Second, 30*time.Second, "could not validate kubernetes_state_core check on cluster agent pod")

		s.EventuallyWithTf(func(c *assert.CollectT) {
			verifyKSMCheck(s)
		}, 600*time.Second, 30*time.Second, "could not validate KSM (cluster check) metrics in time")
		// })

		s.T().Run("KSM check works (cluster check runner)", func(t *testing.T) {
			// Update DDA
			ddaConfigPath, err = common.GetAbsPath(filepath.Join(common.ManifestsPath, "datadog-agent-ccr-enabled.yaml"))
			assert.NoError(t, err)
			// k8s.KubectlApply(t, kubectlOptions, ddaConfigPath)
			operatorOpts := []operatorparams.Option{
				operatorparams.WithNamespace(common.NamespaceName),
				operatorparams.WithOperatorFullImagePath(common.OperatorImageName),
				operatorparams.WithHelmValues("installCRDs: false"),
			}

			ddaOpts := []agentwithoperatorparams.Option{
				agentwithoperatorparams.WithNamespace(common.NamespaceName),
				agentwithoperatorparams.WithTLSKubeletVerify(false),
				agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
					Name:         "dda-minimum",
					YamlFilePath: ddaConfigPath,
				}),
			}

			provisionerOptions := []provisioners.KubernetesProvisionerOption{
				provisioners.WithK8sVersion(common.K8sVersion),
				provisioners.WithOperatorOptions(operatorOpts...),
				provisioners.WithDDAOptions(ddaOpts...),
			}

			s.UpdateEnv(provisioners.KubernetesProvisioner(provisioners.LocalKindRunFunc, provisionerOptions...))
			common.VerifyAgentPods(t, kubectlOptions, "app.kubernetes.io/instance=datadog-ccr-enabled-agent")
			common.VerifyNumPodsForSelector(t, kubectlOptions, 1, "app.kubernetes.io/instance=datadog-ccr-enabled-cluster-agent")
			common.VerifyNumPodsForSelector(t, kubectlOptions, 1, "app.kubernetes.io/instance=datadog-ccr-enabled-cluster-checks-runner")

			s.EventuallyWithTf(func(c *assert.CollectT) {
				ccrPods, err := k8s.ListPodsE(t, kubectlOptions, v1.ListOptions{
					LabelSelector: "app.kubernetes.io/instance=datadog-ccr-enabled-cluster-checks-runner",
				})
				assert.NoError(c, err)

				for _, ccr := range ccrPods {
					k8s.WaitUntilPodAvailable(t, kubectlOptions, ccr.Name, 9, 15*time.Second)
					output, err := k8s.RunKubectlAndGetOutputE(t, kubectlOptions, "exec", "-it", ccr.Name, "--", "agent", "status", "collector", "-j")
					assert.NoError(c, err)

					verifyCheck(c, output, "kubernetes_state_core")
				}
			}, 1200*time.Second, 30*time.Second, "could not validate kubernetes_state_core check on cluster check runners pod")

			s.EventuallyWithTf(func(c *assert.CollectT) {
				verifyKSMCheck(s)
			}, 600*time.Second, 30*time.Second, "could not validate kubernetes_state_core (cluster check on CCR) check in time")
		})

		s.T().Run("Autodiscovery works", func(t *testing.T) {
			// Install DDA
			ddaConfigPath, err = common.GetAbsPath(common.DdaMinimalPath)
			assert.NoError(t, err)

			ddaOpts := []agentwithoperatorparams.Option{
				agentwithoperatorparams.WithNamespace(common.NamespaceName),
				agentwithoperatorparams.WithTLSKubeletVerify(false),
				agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{Name: "dda-autodiscovery", YamlFilePath: ddaConfigPath}),
			}

			provisionerOptions := []provisioners.KubernetesProvisionerOption{
				provisioners.WithK8sVersion(common.K8sVersion),
				provisioners.WithOperatorOptions(provisioners.DefaultOperatorOptions...),
				provisioners.WithDDAOptions(ddaOpts...),
				provisioners.WithYAMLWorkload(provisioners.YAMLWorkload{Name: "nginx", Path: strings.Join([]string{common.ManifestsPath, "autodiscovery-annotation.yaml"}, "/")}),
			}

			// Add nginx with annotations
			s.UpdateEnv(provisioners.KubernetesProvisioner(provisioners.LocalKindRunFunc, provisionerOptions...))

			common.VerifyNumPodsForSelector(t, kubectlOptions, 1, "app=nginx")
			common.VerifyAgentPods(t, kubectlOptions, common.NodeAgentSelector+",agent.datadoghq.com/name=dda-autodiscovery")

			// check agent pods for http check
			s.EventuallyWithTf(func(c *assert.CollectT) {
				agentPods, err := k8s.ListPodsE(t, kubectlOptions, v1.ListOptions{
					LabelSelector: common.NodeAgentSelector + ",agent.datadoghq.com/name=dda-autodiscovery",
					FieldSelector: "status.phase=Running",
				})
				assert.NoError(c, err)

				for _, pod := range agentPods {
					output, err := k8s.RunKubectlAndGetOutputE(t, kubectlOptions, "exec", "-it", pod.Name, "--", "agent", "status", "-j")
					assert.NoError(c, err)

					verifyCheck(c, output, "http_check")
				}
			}, 900*time.Second, 30*time.Second, "could not validate http check on agent pod")

			s.EventuallyWithTf(func(c *assert.CollectT) {
				verifyHTTPCheck(s)
			}, 600*time.Second, 30*time.Second, "could not validate http.can_connect check fake intake client")
		})

		s.T().Run("Logs collection works", func(t *testing.T) {
			// Update DDA
			ddaConfigPath, err = common.GetAbsPath(filepath.Join(common.ManifestsPath, "datadog-agent-logs.yaml"))
			assert.NoError(t, err)

			k8s.KubectlApply(t, kubectlOptions, ddaConfigPath)
			common.VerifyAgentPods(t, kubectlOptions, "app.kubernetes.io/instance=datadog-agent-logs-agent")

			// Verify logs collection on agent pod
			s.EventuallyWithTf(func(c *assert.CollectT) {
				agentPods, err := k8s.ListPodsE(t, kubectlOptions, v1.ListOptions{
					LabelSelector: "app.kubernetes.io/instance=datadog-agent-logs-agent",
				})
				assert.NoError(c, err)

				for _, pod := range agentPods {
					k8s.WaitUntilPodAvailable(t, kubectlOptions, pod.Name, 9, 15*time.Second)

					output, err := k8s.RunKubectlAndGetOutputE(t, kubectlOptions, "exec", "-it", pod.Name, "--", "agent", "status", "logs agent", "-j")
					assert.NoError(c, err)

					verifyAgentPodLogs(c, output)
				}
			}, 900*time.Second, 30*time.Second, "could not validate logs status on agent pod")

			s.EventuallyWithTf(func(c *assert.CollectT) {
				verifyAPILogs(s, c)
			}, 600*time.Second, 30*time.Second, "could not valid logs collection with fake intake client")

		})
	})
}

func verifyAgentPodLogs(c *assert.CollectT, collectorOutput string) {
	var agentLogs []interface{}
	logsJson := common.ParseCollectorJson(collectorOutput)

	tailedIntegrations := 0
	if logsJson != nil {
		agentLogs = logsJson["logsStats"].(map[string]interface{})["integrations"].([]interface{})
		for _, log := range agentLogs {
			if integration, ok := log.(map[string]interface{})["sources"].([]interface{})[0].(map[string]interface{}); ok {
				message, exists := integration["messages"].([]interface{})[0].(string)
				if exists && len(message) > 0 {
					num, _ := strconv.Atoi(string(message[0]))
					if num > 0 && strings.Contains(message, "files tailed") {
						tailedIntegrations++
					}
				}
			} else {
				assert.True(c, ok, "Failed to get sources from logs. Possible causes: missing 'sources' field, empty array, or incorrect data format.")
			}
		}
	}
	totalIntegrations := len(agentLogs)
	assert.True(c, tailedIntegrations >= totalIntegrations*80/100, "Expected at least 80%% of integrations to be tailed, got %d/%d", tailedIntegrations, totalIntegrations)
}

func verifyCheck(c *assert.CollectT, collectorOutput string, checkName string) {
	var runningChecks map[string]interface{}

	checksJson := common.ParseCollectorJson(collectorOutput)
	if checksJson != nil {
		runningChecks = checksJson["runnerStats"].(map[string]interface{})["Checks"].(map[string]interface{})
		if check, found := runningChecks[checkName].(map[string]interface{}); found {
			for _, instance := range check {
				assert.EqualValues(c, checkName, instance.(map[string]interface{})["CheckName"].(string))

				lastError, exists := instance.(map[string]interface{})["LastError"].(string)
				assert.True(c, exists)
				assert.Empty(c, lastError)

				totalErrors, exists := instance.(map[string]interface{})["TotalErrors"].(float64)
				assert.True(c, exists)
				assert.Zero(c, totalErrors)

				totalMetricSamples, exists := instance.(map[string]interface{})["TotalMetricSamples"].(float64)
				assert.True(c, exists)
				assert.Greater(c, totalMetricSamples, float64(0))
			}
		} else {
			assert.True(c, found, fmt.Sprintf("Check %s not found or not yet running.", checkName))
		}
	}
}

func verifyAPILogs(s *k8sSuite, c *assert.CollectT) {
	logs, err := s.Env().FakeIntake.Client().FilterLogs("agent")
	s.Assert().NoError(err)
	s.Assert().NotEmptyf(logs, fmt.Sprintf("Expected fake intake-ingested logs to not be empty: %s", err))
	//
	//assert.NoError(c, err, "failed to query logs: %v", err)
	//assert.NotEmptyf(c, logs, fmt.Sprintf("expected logs to not be empty: %s", err))
}

func verifyKSMCheck(s *k8sSuite) {
	metricNames, err := s.Env().FakeIntake.Client().GetMetricNames()
	s.Assert().NoError(err)
	s.Assert().Contains(metricNames, "kubernetes_state.container.running")
	//tags := []string{fmt.Sprintf("kube_container_name:%s", s.Env().KubernetesCluster.ClusterName)}
	fmt.Println(fmt.Sprintf("kube_container_name:%s", s.Env().KubernetesCluster.ClusterName))
	//var opts []client.MatchOpt[*aggregator.MetricSeries]
	//opts = append(opts, client.WithTags[*aggregator.MetricSeries](tags))

	//metrics, err := s.Env().FakeIntake.Client().FilterMetrics("kubernetes_state.container.running", opts...)
	metrics, err := s.Env().FakeIntake.Client().FilterMetrics("kubernetes_state.container.running")
	s.Assert().NoError(err)
	fmt.Println("METRICS: ", metrics)
	s.Assert().NotEmptyf(metrics, fmt.Sprintf("expected metric series to not be empty: %s", err))

	//assert.True(c, len(resp.Series) > 0, fmt.Sprintf("expected metric series to not be empty: %s", err))
}

func verifyHTTPCheck(s *k8sSuite) {
	metricNames, err := s.Env().FakeIntake.Client().GetMetricNames()
	s.Assert().NoError(err)
	s.Assert().Contains(metricNames, "network.http.can_connect")
	metrics, err := s.Env().FakeIntake.Client().FilterMetrics("network.http.can_connect")
	s.Assert().NoError(err)
	s.Assert().Greater(len(metrics), 0)
	for _, metric := range metrics {
		for _, points := range metric.Points {
			s.Assert().Greater(points.Value, float64(0))
		}
	}
}
