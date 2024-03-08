// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package e2e

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	localKubernetes "github.com/DataDog/test-infra-definitions/components/kubernetes"
	resAws "github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	pulumik8s "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/kustomize"
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
}

func TestKindSuite(t *testing.T) {
	e2eParams := []e2e.SuiteOption{e2e.WithProvisioner(kindProvisioner()), e2e.WithDevMode()}
	e2e.Run[kindEnv](t, &kindSuite{}, e2eParams...)
}

// kindProvisioner Pulumi E2E provisioner to install the DatadogAgent CRD, RBACs, and deploy DatadogAgent manifest
func kindProvisioner() e2e.Provisioner {
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

		kindCluster, err := localKubernetes.NewKindCluster(*awsEnv.CommonEnvironment, vm, awsEnv.CommonNamer.ResourceName("kind"), kindClusterName, awsEnv.KubernetesVersion(), pulumi.Timeouts(&pulumi.CustomTimeouts{
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
		// TODO: update manager IMG version

		managerDir := filepath.Join("..", "..", "config", "default")

		kustomizeDirectoryPath, err := filepath.Abs(managerDir)
		if err != nil {
			return err
		}

		_, err = kustomize.NewDirectory(ctx, "e2e-default",
			kustomize.DirectoryArgs{
				Directory: pulumi.String(kustomizeDirectoryPath),
			},
			pulumi.Provider(kindKubeProvider))

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
	})
}

func (suite *kindSuite) SetupSuite() {
	suite.BaseSuite.SetupSuite()
}

func (suite *kindSuite) TearDownSuite() {
	suite.BaseSuite.TearDownSuite()
}

func (s *kindSuite) TestKindRun() {
	s.Assert().NotNil(s.Env().Kind.Client())
	k8sClient := s.Env().Kind.Client()
	s.Assert().NotNil(k8sClient.CoreV1().Pods("system"))
	s.Assert().NotNil(k8sClient.CoreV1().Pods("system").List(context.Background(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=datadog-operator"}))

	// Assert datadog-operator pod is running
	pods, err := k8sClient.CoreV1().Pods("system").List(context.Background(), metav1.ListOptions{LabelSelector: "app.kubernetes.io/name=datadog-operator"})
	s.Assert().NoError(err)
	s.Assert().NotEmpty(pods)
	for _, pod := range pods.Items {
		s.Assert().Equal(corev1.PodPhase("Running"), pod.Status.Phase)
	}
}
