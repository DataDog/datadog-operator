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
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	localKubernetes "github.com/DataDog/test-infra-definitions/components/kubernetes"
	resAws "github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/gruntwork-io/terratest/modules/k8s"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/kustomize"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
}

func TestKindSuite(t *testing.T) {
	e2eParams := []e2e.SuiteOption{
		e2e.WithStackName(fmt.Sprintf("operator-kind-%s", k8sVersion)),
		e2e.WithProvisioner(kindProvisioner(k8sVersion, nil)),
		e2e.WithDevMode(),
	}

	e2e.Run[kindEnv](t, &kindSuite{}, e2eParams...)
}

// kindProvisioner Pulumi E2E provisioner to deploy the Operator binary with kustomize and deploy DDA manifest
func kindProvisioner(k8sVersion string, extraKustomizeResources []string) e2e.Provisioner {
	return e2e.NewTypedPulumiProvisioner[kindEnv]("kind-operator", func(ctx *pulumi.Context, env *kindEnv) error {
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

		kindCluster, err := localKubernetes.NewKindCluster(&awsEnv, vm, awsEnv.CommonNamer().ResourceName("kind"), kindClusterName, k8sVersion, utils.PulumiDependsOn(installEcrCredsHelperCmd))
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

		if extraKustomizeResources == nil {
			extraKustomizeResources = []string{defaultMgrFileName}
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
			_, err = agent.NewImagePullSecret(&awsEnv, namespaceName, pulumi.Provider(kindKubeProvider))
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

func (s *kindSuite) TestKindRun() {
	var ddaConfigPath string

	// Get E2E kubernetes context and set up terratest kubectlOptions
	cleanUpContext, err := contextConfig(s.Env().Kind.ClusterOutput.KubeConfig)
	s.Assert().NoError(err, "Error retrieving E2E kubeconfig.")
	defer cleanUpContext()

	kubectlOptions = k8s.NewKubectlOptions("", kubeConfigPath, namespaceName)

	s.T().Run("Operator deploys to kind cluster", func(t *testing.T) {
		verifyOperator(t, kubectlOptions)
	})

	s.T().Run("Minimal DDA deploys agent resources", func(t *testing.T) {
		// Install DDA
		ddaConfigPath, err = getAbsPath(ddaMinimalPath)
		assert.NoError(t, err)
		k8s.KubectlApply(t, kubectlOptions, ddaConfigPath)
		verifyAgentPods(t, kubectlOptions, nodeAgentSelector+",agent.datadoghq.com/e2e-test=datadog-agent-minimum")
		verifyNumPodsForSelector(t, kubectlOptions, 1, clusterAgentSelector+",agent.datadoghq.com/e2e-test=datadog-agent-minimum")
	})

	s.T().Run("Kubelet check works", func(t *testing.T) {
		s.EventuallyWithTf(func(c *assert.CollectT) {
			agentPods, err := k8s.ListPodsE(t, kubectlOptions, v1.ListOptions{
				LabelSelector: nodeAgentSelector + ",agent.datadoghq.com/e2e-test=datadog-agent-minimum",
			})
			assert.NoError(c, err)

			for _, pod := range agentPods {
				k8s.WaitUntilPodAvailable(t, kubectlOptions, pod.Name, 9, 15*time.Second)

				output, err := k8s.RunKubectlAndGetOutputE(t, kubectlOptions, "exec", "-it", pod.Name, "--", "agent", "status", "collector", "-j")
				assert.NoError(c, err)

				verifyCheck(c, output, "kubelet")
			}
		}, 900*time.Second, 30*time.Second, fmt.Sprintf("could not validate kubelet check on agent pod"))

		metricQuery := fmt.Sprintf("exclude_null(avg:kubernetes.cpu.usage.total{kube_cluster_name:%s, container_id:*})", s.Env().Kind.ClusterName)
		s.EventuallyWithTf(func(c *assert.CollectT) {
			resp, _, err := s.datadogClient.metricsApi.QueryMetrics(s.datadogClient.ctx, time.Now().Add(-time.Minute*5).Unix(), time.Now().Add(time.Minute*5).Unix(), metricQuery)

			assert.Truef(c, len(resp.Series) > 0, "expected metric series for query `%s` to not be empty: %s", metricQuery, err)
		}, 600*time.Second, 30*time.Second, fmt.Sprintf("metric series has not changed to not empty with query %s", metricQuery))
	})

	s.T().Run("KSM Check Works (cluster check)", func(t *testing.T) {
		s.EventuallyWithTf(func(c *assert.CollectT) {
			clusterAgentPods, err := k8s.ListPodsE(t, kubectlOptions, v1.ListOptions{
				LabelSelector: clusterAgentSelector + ",agent.datadoghq.com/e2e-test=datadog-agent-minimum",
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
			verifyKSMCheck(s, c)
		}, 600*time.Second, 30*time.Second, "could not validate kubernetes_state_core check with api client")
	})

	s.T().Run("KSM Check Works (cluster check runner)", func(t *testing.T) {
		// Update DDA
		ddaConfigPath, err = getAbsPath(filepath.Join(manifestsPath, "datadog-agent-ccr-enabled.yaml"))
		assert.NoError(t, err)
		k8s.KubectlApply(t, kubectlOptions, ddaConfigPath)
		verifyAgentPods(t, kubectlOptions, nodeAgentSelector+",agent.datadoghq.com/e2e-test=datadog-agent-ccr-enabled")
		verifyNumPodsForSelector(t, kubectlOptions, 1, clusterAgentSelector+",agent.datadoghq.com/e2e-test=datadog-agent-ccr-enabled")
		verifyNumPodsForSelector(t, kubectlOptions, 1, clusterCheckRunnerSelector+",agent.datadoghq.com/e2e-test=datadog-agent-ccr-enabled")

		s.EventuallyWithTf(func(c *assert.CollectT) {
			ccrPods, err := k8s.ListPodsE(t, kubectlOptions, v1.ListOptions{
				LabelSelector: clusterCheckRunnerSelector + ",agent.datadoghq.com/e2e-test=datadog-agent-ccr-enabled",
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
			verifyKSMCheck(s, c)
		}, 600*time.Second, 30*time.Second, "could not validate kubernetes_state_core check with api client")
	})

	s.T().Run("Cleanup DDA", func(t *testing.T) {
		deleteDda(t, kubectlOptions, ddaConfigPath)
	})
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

func verifyKSMCheck(s *kindSuite, c *assert.CollectT) {
	metricQuery := fmt.Sprintf("exclude_null(avg:kubernetes_state.container.running{kube_cluster_name:%s, kube_container_name:*})", s.Env().Kind.ClusterName)

	resp, _, err := s.datadogClient.metricsApi.QueryMetrics(s.datadogClient.ctx, time.Now().AddDate(0, 0, -1).Unix(), time.Now().Unix(), metricQuery)

	assert.True(c, len(resp.Series) > 0, fmt.Sprintf("expected metric series to not be empty: %s", err))
}
