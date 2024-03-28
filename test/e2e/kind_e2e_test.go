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
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	localKubernetes "github.com/DataDog/test-infra-definitions/components/kubernetes"
	resAws "github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/gruntwork-io/terratest/modules/k8s"
	pulumik8s "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	pulumicorev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/kustomize"
	pulumimetav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type kindEnv struct {
	Kind *components.KubernetesCluster
}

type kindSuite struct {
	e2e.BaseSuite[kindEnv]
}

func TestKindSuite(t *testing.T) {
	e2eParams := []e2e.SuiteOption{
		e2e.WithProvisioner(kindProvisioner(k8sVersion)),
		e2e.WithDevMode(),
	}

	e2e.Run[kindEnv](t, &kindSuite{}, e2eParams...)
}

// kindProvisioner Pulumi E2E provisioner to deploy the Operator binary with kustomize and deploy DDA manifest
func kindProvisioner(k8sVersion string) e2e.Provisioner {
	return e2e.NewTypedPulumiProvisioner[kindEnv]("kind-operator", func(ctx *pulumi.Context, env *kindEnv) error {
		// Provision AWS environment
		awsEnv, err := resAws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		// Create EC2 VM
		vm, err := ec2.NewVM(awsEnv, "kind")
		if err != nil {
			return err
		}
		if err := vm.Export(ctx, nil); err != nil {
			return err
		}

		// Create kind cluster
		kindClusterName := ctx.Stack()
		err = ctx.Log.Info(fmt.Sprintf("Creating kind cluster with K8s version: %s", k8sVersion), nil)
		if err != nil {
			return err
		}

		kindCluster, err := localKubernetes.NewKindCluster(*awsEnv.CommonEnvironment, vm, awsEnv.CommonNamer.ResourceName("kind"), kindClusterName, k8sVersion)
		if err != nil {
			return err
		}
		if err := kindCluster.Export(ctx, &env.Kind.ClusterOutput); err != nil {
			return err
		}

		// Build Kubernetes provider
		kindKubeProvider, err := pulumik8s.NewProvider(ctx, awsEnv.Namer.ResourceName("k8s-provider"), &pulumik8s.ProviderArgs{
			EnableServerSideApply: pulumi.BoolPtr(true),
			Kubeconfig:            kindCluster.KubeConfig,
		})
		if err != nil {
			return err
		}

		// Deploy resources from kustomize config/default directory
		kustomizeDirPath, err := filepath.Abs(mgrKustomizeDirPath)
		if err != nil {
			return err
		}

		_, err = kustomize.NewDirectory(ctx, "e2e-manager",
			kustomize.DirectoryArgs{
				Directory: pulumi.String(kustomizeDirPath),
				Transformations: []yaml.Transformation{
					operatorTransformationFunc(),
				},
			},
			pulumi.Provider(kindKubeProvider))
		if err != nil {
			return err
		}

		// Create imagePullSecret to pull E2E operator image from ECR
		if imgPullPassword != "" {
			_, err = agent.NewImagePullSecret(*awsEnv.CommonEnvironment, namespaceName, pulumi.Provider(kindKubeProvider))
			if err != nil {
				return err
			}
		}

		// Create datadog agent secret
		_, err = pulumicorev1.NewSecret(ctx, "datadog-secret", &pulumicorev1.SecretArgs{
			Metadata: pulumimetav1.ObjectMetaArgs{
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

func (suite *kindSuite) SetupSuite() {
	suite.BaseSuite.SetupSuite()
}

func (suite *kindSuite) TearDownSuite() {
	suite.BaseSuite.TearDownSuite()
}

func (s *kindSuite) TestKindRun() {
	// Get E2E kubernetes context and set up terratest kubectlOptions
	cleanUpContext, err := contextConfig(s.Env().Kind.ClusterOutput.KubeConfig)
	s.Assert().NoError(err, "Error retrieving E2E kubeconfig.")
	defer cleanUpContext()

	kubectlOptions = k8s.NewKubectlOptions("", kubeConfigPath, namespaceName)

	s.T().Run("Operator deploys to kind cluster", func(t *testing.T) {
		s.Assert().NotNil(s.Env().Kind.Client())
		k8sClient := s.Env().Kind.Client()
		s.Assert().NotNil(k8sClient.CoreV1().Pods(namespaceName))
		s.Assert().NotNil(k8sClient.CoreV1().Pods(namespaceName).List(context.Background(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=datadog-operator"}))

		// Operator pod is created
		verifyNumPodsForSelector(t, kubectlOptions, 1, "app.kubernetes.io/name=datadog-operator")

		pods, err := k8sClient.CoreV1().Pods(namespaceName).List(context.Background(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=datadog-operator"})
		s.Assert().NoError(err)
		s.Assert().NotEmpty(pods)

		// Operator pod is available
		for _, pod := range pods.Items {
			k8s.WaitUntilPodAvailable(t, kubectlOptions, pod.Name, 9, 15*time.Second)
		}
	})

	s.T().Run("Minimal DDA deploys agent resources", func(t *testing.T) {
		ddaConfigPath, err := getAbsPath(ddaMinimalPath)
		s.Assert().NoError(err)

		// Install DDA
		k8s.KubectlApply(t, kubectlOptions, ddaConfigPath)
		defer k8s.KubectlDelete(t, kubectlOptions, ddaConfigPath)

		s.Assert().NotNil(s.Env().Kind.Client())
		k8sClient := s.Env().Kind.Client()

		// Get nodesList
		k8s.WaitUntilAllNodesReady(t, kubectlOptions, 9, 15*time.Second)
		nodes, err := k8sClient.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
		s.Assert().NoError(err)
		s.Assert().NotEmpty(nodes)

		// Agent pods are created
		verifyNumPodsForSelector(t, kubectlOptions, len(nodes.Items), "agent.datadoghq.com/component=agent")
		verifyNumPodsForSelector(t, kubectlOptions, 1, "agent.datadoghq.com/component=cluster-agent")

		// Agent pods are available
		agentPods, err := k8sClient.CoreV1().Pods(namespaceName).List(context.Background(), metav1.ListOptions{LabelSelector: "agent.datadoghq.com/component=agent"})
		s.Assert().NoError(err)
		s.Assert().NotEmpty(agentPods)
		for _, pod := range agentPods.Items {
			k8s.WaitUntilPodAvailable(t, kubectlOptions, pod.Name, 9, 15*time.Second)
		}

		// DCA pod is Available
		dcaPods, err := k8sClient.CoreV1().Pods(namespaceName).List(context.Background(), metav1.ListOptions{LabelSelector: "agent.datadoghq.com/component=cluster-agent"})
		s.Assert().NoError(err)
		s.Assert().NotEmpty(dcaPods)
		for _, pod := range dcaPods.Items {
			k8s.WaitUntilPodAvailable(t, kubectlOptions, pod.Name, 9, 15*time.Second)
		}
	})
}
