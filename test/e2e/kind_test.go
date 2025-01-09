// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"context"
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/DataDog/test-infra-definitions/common/utils"
	localKubernetes "github.com/DataDog/test-infra-definitions/components/kubernetes"
	resAws "github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/kustomize"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
)

type kindEnv struct {
	Kind *components.KubernetesCluster
}

type kindSuite struct {
	e2e.BaseSuite[kindEnv]
	datadogClient
}

type datadogClient struct {
	ctx        context.Context
	metricsApi *datadogV1.MetricsApi
	logsApi    *datadogV1.LogsApi
}

func (suite *kindSuite) SetupSuite() {
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

// kindProvisioner Pulumi E2E provisioner to deploy the Operator binary with kustomize and deploy DDA manifest
func kindProvisioner(k8sVersion string, extraKustomizeResources []string) provisioners.Provisioner {
	return provisioners.NewTypedPulumiProvisioner[kindEnv]("kind-operator", func(ctx *pulumi.Context, env *kindEnv) error {
		// Provision AWS environment
		awsEnv, err := resAws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		// Create EC2 VM
		vm, err := ec2.NewVM(awsEnv, "kind", ec2.WithUserData(UserData))
		if err != nil {
			return err
		}
		if err := vm.Export(ctx, nil); err != nil {
			return err
		}

		// Create kind cluster
		kindClusterName := strings.ReplaceAll(ctx.Stack(), ".", "-")

		err = ctx.Log.Info(fmt.Sprintf("Creating kind cluster with K8s version: %s", k8sVersion), nil)
		if err != nil {
			return err
		}

		installEcrCredsHelperCmd, err := ec2.InstallECRCredentialsHelper(awsEnv, vm)
		if err != nil {
			return err
		}

		kindCluster, err := localKubernetes.NewKindCluster(&awsEnv, vm, awsEnv.CommonNamer().ResourceName("kind"), k8sVersion, utils.PulumiDependsOn(installEcrCredsHelperCmd))
		if err != nil {
			return err
		}
		if err := kindCluster.Export(ctx, &env.Kind.ClusterOutput); err != nil {
			return err
		}

		// Build Kubernetes provider
		kindKubeProvider, err := kubernetes.NewProvider(ctx, awsEnv.Namer.ResourceName("k8s-provider"), &kubernetes.ProviderArgs{
			EnableServerSideApply: pulumi.BoolPtr(true),
			Kubeconfig:            kindCluster.KubeConfig,
		})
		if err != nil {
			return err
		}

		// Deploy resources from kustomize config/e2e directory
		kustomizeDirPath, err := filepath.Abs(mgrKustomizeDirPath)
		if err != nil {
			return err
		}

		updateKustomization(kustomizeDirPath, extraKustomizeResources)

		e2eKustomize, err := kustomize.NewDirectory(ctx, "e2e-manager",
			kustomize.DirectoryArgs{
				Directory: pulumi.String(kustomizeDirPath),
			},
			pulumi.Provider(kindKubeProvider))
		if err != nil {
			return err
		}

		pulumi.DependsOn([]pulumi.Resource{e2eKustomize})

		// Create imagePullSecret to pull E2E operator image from ECR
		if imgPullPassword != "" {
			_, err = utils.NewImagePullSecret(&awsEnv, namespaceName, pulumi.Provider(kindKubeProvider))
			if err != nil {
				return err
			}
		}

		// Create datadog agent secret
		_, err = corev1.NewSecret(ctx, "datadog-secret", &corev1.SecretArgs{
			Metadata: metav1.ObjectMetaArgs{
				Namespace: pulumi.String(namespaceName),
				Name:      pulumi.String("datadog-secret"),
			},
			StringData: pulumi.StringMap{
				"api-key": awsEnv.CommonEnvironment.AgentAPIKey(),
				"app-key": awsEnv.CommonEnvironment.AgentAPPKey(),
			},
		}, pulumi.Provider(kindKubeProvider))
		if err != nil {
			return err
		}

		// Create datadog cluster name configMap
		// TODO: remove this when NewAgentWithOperator is available in test-infra-definitions
		_, err = corev1.NewConfigMap(ctx, "datadog-cluster-name", &corev1.ConfigMapArgs{
			Metadata: metav1.ObjectMetaArgs{
				Namespace: pulumi.String(namespaceName),
				Name:      pulumi.String("datadog-cluster-name"),
			},
			Data: pulumi.StringMap{
				"DD_CLUSTER_NAME": pulumi.String(kindClusterName),
			},
		}, pulumi.Provider(kindKubeProvider))
		if err != nil {
			return err
		}

		return nil

	}, runner.ConfigMap{
		"ddagent:deploy":                           auto.ConfigValue{Value: "false"},
		"ddtestworkload:deploy":                    auto.ConfigValue{Value: "false"},
		"ddagent:fakeintake":                       auto.ConfigValue{Value: "false"},
		"dddogstatsd:deploy":                       auto.ConfigValue{Value: "false"},
		"ddinfra:deployFakeintakeWithLoadBalancer": auto.ConfigValue{Value: "false"},
		"ddagent:imagePullRegistry":                auto.ConfigValue{Value: "669783387624.dkr.ecr.us-east-1.amazonaws.com"},
		"ddagent:imagePullUsername":                auto.ConfigValue{Value: "AWS"},
		"ddagent:imagePullPassword":                auto.ConfigValue{Value: imgPullPassword},
		"ddinfra:kubernetesVersion":                auto.ConfigValue{Value: k8sVersion},
	})
}

func verifyAgentPodLogs(c *assert.CollectT, collectorOutput string) {
	var agentLogs []interface{}
	logsJson := parseCollectorJson(collectorOutput)

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

	checksJson := parseCollectorJson(collectorOutput)
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

func verifyAPILogs(s *kindSuite, c *assert.CollectT) {
	logQuery := fmt.Sprintf("kube_cluster_name:%s", s.Env().Kind.ClusterName)
	requestBody := datadogV1.LogsListRequest{
		Query: &logQuery,
		Time: datadogV1.LogsListRequestTime{
			From: time.Now().AddDate(0, 0, -1), // One day ago
			To:   time.Now(),
		},
		Limit: datadog.PtrInt32(100),
	}

	resp, _, err := s.datadogClient.logsApi.ListLogs(s.datadogClient.ctx, requestBody)

	assert.NoError(c, err, "failed to query logs: %v", err)
	assert.True(c, len(resp.Logs) > 0, fmt.Sprintf("expected logs to not be empty: %s", err))
}

func verifyKSMCheck(s *kindSuite, c *assert.CollectT) {
	metricQuery := fmt.Sprintf("exclude_null(avg:kubernetes_state.container.running{kube_cluster_name:%s, kube_container_name:*})", s.Env().Kind.ClusterName)

	resp, _, err := s.datadogClient.metricsApi.QueryMetrics(s.datadogClient.ctx, time.Now().AddDate(0, 0, -1).Unix(), time.Now().Unix(), metricQuery)

	assert.True(c, len(resp.Series) > 0, fmt.Sprintf("expected metric series to not be empty: %s", err))
}

func verifyHTTPCheck(s *kindSuite, c *assert.CollectT) {
	metricQuery := fmt.Sprintf("exclude_null(avg:network.http.can_connect{kube_cluster_name:%s})", s.Env().Kind.ClusterName)

	resp, _, err := s.datadogClient.metricsApi.QueryMetrics(s.datadogClient.ctx, time.Now().AddDate(0, 0, -1).Unix(), time.Now().Unix(), metricQuery)
	assert.EqualValues(c, *resp.Status, "ok")
	for _, series := range resp.Series {
		for _, point := range series.Pointlist {
			assert.Equal(c, int(*point[1]), 1)
		}
	}
	assert.True(c, len(resp.Series) > 0, fmt.Sprintf("expected metric series to not be empty: %s", err))
}
