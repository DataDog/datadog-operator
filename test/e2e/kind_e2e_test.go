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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	localKubernetes "github.com/DataDog/test-infra-definitions/components/kubernetes"
	resAws "github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/google/go-cmp/cmp"
	pulumik8s "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/kustomize"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type kindEnv struct {
	Kind *components.KubernetesCluster
}

type kindSuite struct {
	e2e.BaseSuite[kindEnv]
	ImageTag   string
	K8sVersion string
}

func TestKindSuite(t *testing.T) {
	e2eParams := []e2e.SuiteOption{e2e.WithProvisioner(kindProvisioner(k8sVersion, ddaConfig, namespace)), e2e.WithDevMode()}
	e2e.Run[kindEnv](t, &kindSuite{}, e2eParams...)
}

// kindProvisioner Pulumi E2E provisioner to deploy the Operator binary with kustomize and deploy DDA manifest
func kindProvisioner(k8sVersion string, ddaConfig string, namespace string) e2e.Provisioner {
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
		if err != nil {
			return err
		}
		if k8sVersion == "" {
			k8sVersion = awsEnv.KubernetesVersion()
		}

		ctx.Log.Info(fmt.Sprintf("Creating kind cluster with K8s version: %s", k8sVersion), nil)

		kindCluster, err := localKubernetes.NewKindCluster(*awsEnv.CommonEnvironment, vm, awsEnv.CommonNamer.ResourceName("kind"), kindClusterName, k8sVersion, pulumi.Timeouts(&pulumi.CustomTimeouts{
			Create: "30m",
			Update: "30m",
			Delete: "30m",
		}))
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

		_, err = kustomize.NewDirectory(ctx, "e2e-default",
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

		if ddaConfig != "" {
			// Wait for 90 seconds for Operator to be ready before creating dda.
			time.Sleep(90 * time.Second)
			_, err = yaml.NewConfigFile(ctx, "datadog-agent", &yaml.ConfigFileArgs{
				File: ddaConfig,
				Transformations: []yaml.Transformation{
					ddaTransformationFunc(kindClusterName, awsEnv.CommonEnvironment.AgentAPIKey()),
				},
			}, pulumi.Provider(kindKubeProvider))
			if err != nil {
				return err
			}
			// Wait for DDA resources to be ready
			time.Sleep(30 * time.Second)
		}

		return nil

	}, runner.ConfigMap{
		"ddagent:deploy":                           auto.ConfigValue{Value: "false"},
		"ddtestworkload:deploy":                    auto.ConfigValue{Value: "false"},
		"ddagent:fakeintake":                       auto.ConfigValue{Value: "false"},
		"dddogstatsd:deploy":                       auto.ConfigValue{Value: "false"},
		"ddinfra:deployFakeintakeWithLoadBalancer": auto.ConfigValue{Value: "false"},
	})
}

func (suite *kindSuite) SetupSuite() {
	// Setup kind E2E test suite with CI environment variables
	if tag, ok := os.LookupEnv("TARGET_IMAGE"); ok {
		//suite.ImageTag = tag
		imageTag = tag
	}
	if version, ok := os.LookupEnv("K8S_VERSION"); ok {
		//suite.K8sVersion = version
		k8sVersion = version
	}
	suite.BaseSuite.SetupSuite()
}

func (suite *kindSuite) TearDownSuite() {
	suite.BaseSuite.TearDownSuite()
}

func (s *kindSuite) TestKindRun() {
	s.T().Run("Operator deploys to kind cluster", func(t *testing.T) {
		s.Assert().NotNil(s.Env().Kind.Client())
		k8sClient := s.Env().Kind.Client()
		s.Assert().NotNil(k8sClient.CoreV1().Pods(namespace))
		s.Assert().NotNil(k8sClient.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=datadog-operator"}))

		// Operator pod is running
		pods, err := k8sClient.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=datadog-operator"})
		s.Assert().NoError(err)
		s.Assert().NotEmpty(pods)
		s.Assert().Eventually(func() bool {
			if len(pods.Items) == 1 {
				return true
			}
			return false
		}, 5*time.Second, time.Second, "There should be one operator pod per cluster.")
		for _, pod := range pods.Items {
			s.Assert().Equal(corev1.PodPhase("Running"), pod.Status.Phase, fmt.Sprintf("Operator pod status is not `Running`: Got: %s", pod.Status.Phase))
			if imageTag != "" {
				s.Assert().Equal(s.ImageTag, pod.Status.ContainerStatuses[0].Image, fmt.Sprintf("Operator pod is not running the expected image tag. Got: %s", pod.Status.ContainerStatuses[0].Image))
			}
		}
	})

	s.T().Run("Minimal DDA deploys agent resources", func(t *testing.T) {
		// Update kind cluster to deploy DDA
		ddaConfigPath, err := getAbsPath(ddaMinimalPath)
		s.Assert().NoError(err)
		s.BaseSuite.UpdateEnv(kindProvisioner(k8sVersion, ddaConfigPath, namespace))

		s.Assert().NotNil(s.Env().Kind.Client())
		k8sClient := s.Env().Kind.Client()

		// Agent pods are present
		s.Assert().NotNil(k8sClient.CoreV1().Pods(namespace))
		s.Assert().NotNil(k8sClient.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "agent.datadoghq.com/component=agent"}))
		s.Assert().NotNil(k8sClient.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "agent.datadoghq.com/component=cluster-agent"}))

		nodes, err := k8sClient.CoreV1().Nodes().List(context.Background(), metav1.ListOptions{})
		s.Assert().NoError(err)
		s.Assert().NotEmpty(nodes)

		// Agent pods are running
		agentPods, err := k8sClient.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "agent.datadoghq.com/component=agent"})
		s.Assert().NoError(err)
		s.Assert().NotEmpty(agentPods)
		s.Assert().Greater(len(nodes.Items), 0, "There should be at least 1 node in the cluster.")
		s.Assert().Equal(len(nodes.Items), len(agentPods.Items), "There should be one agent pod per node.")
		for _, pod := range agentPods.Items {
			s.Assert().Eventually(func() bool {
				if cmp.Equal(corev1.PodPhase("Running"), pod.Status.Phase) {
					return true
				}
				return false
			}, 60*time.Second, 10*time.Second, fmt.Sprintf("Agent pod status is not `Running`: Got: %s", pod.Status.Phase))
		}

		// DCA pod is running
		dcaPods, err := k8sClient.CoreV1().Pods(namespace).List(context.Background(), metav1.ListOptions{LabelSelector: "agent.datadoghq.com/component=cluster-agent"})
		s.Assert().NoError(err)
		s.Assert().NotEmpty(dcaPods)
		s.Assert().Equal(1, len(dcaPods.Items), "There should be one DCA pod per cluster.")
		for _, pod := range dcaPods.Items {
			s.Assert().Eventually(func() bool {
				if cmp.Equal(corev1.PodPhase("Running"), pod.Status.Phase) {
					return true
				}
				return false
			}, 60*time.Second, 10*time.Second, fmt.Sprintf("DCA pod status is not `Running`: Got: %s", pod.Status.Phase))
		}
	})
}
