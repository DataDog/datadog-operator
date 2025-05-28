// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package provisioners

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentwithoperatorparams"
	"github.com/DataDog/test-infra-definitions/components/datadog/operator"
	"github.com/DataDog/test-infra-definitions/components/datadog/operatorparams"
	"github.com/DataDog/test-infra-definitions/resources/gcp"
	"github.com/DataDog/test-infra-definitions/resources/gcp/gke"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// GkeRunFunc is the Pulumi run function that runs the GCP GKE provisioner
func GkeRunFunc(ctx *pulumi.Context, env *environments.Kubernetes, params *KubernetesProvisionerParams) error {
	gcpEnv, err := gcp.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	_, kubeConfig, err := gke.NewCluster(gcpEnv, gcpEnv.CommonNamer().ResourceName("gke"), false)
	if err != nil {
		return err
	}

	// Build Kubernetes provider
	kubeProvider, err := kubernetes.NewProvider(ctx, gcpEnv.CommonNamer().ResourceName("k8s-provider"), &kubernetes.ProviderArgs{
		Kubeconfig:            kubeConfig,
		EnableServerSideApply: pulumi.BoolPtr(true),
	})
	if err != nil {
		return err
	}

	ns, err := corev1.NewNamespace(ctx, gcpEnv.CommonNamer().ResourceName("k8s-namespace"), &corev1.NamespaceArgs{Metadata: &metav1.ObjectMetaArgs{
		Name: pulumi.String("e2e-operator"),
	}}, pulumi.Provider(kubeProvider))

	if err != nil {
		return err
	}

	// Install kustomizations
	kustomizeAppFunc := KustomizeWorkloadAppFunc(params.testName, params.kustomizeResources)

	e2eKustomize, err := kustomizeAppFunc(&gcpEnv, kubeProvider)
	if err != nil {
		return err
	}

	// Create Operator component
	var operatorComp *operator.Operator
	if params.operatorOptions != nil {
		operatorOpts := []pulumi.ResourceOption{
			pulumi.DependsOn([]pulumi.Resource{e2eKustomize, ns}),
		}
		params.operatorOptions = append(params.operatorOptions, operatorparams.WithPulumiResourceOptions(operatorOpts...))

		operatorComp, err = operator.NewOperator(&gcpEnv, gcpEnv.CommonNamer().ResourceName("operator"), kubeProvider, params.operatorOptions...)
		if err != nil {
			return err
		}
	}

	// Setup DDA options
	if params.ddaOptions != nil && params.operatorOptions != nil {
		ddaResourceOpts := []pulumi.ResourceOption{
			pulumi.DependsOn([]pulumi.Resource{e2eKustomize, operatorComp}),
		}
		params.ddaOptions = append(
			params.ddaOptions,
			agentwithoperatorparams.WithPulumiResourceOptions(ddaResourceOpts...))

		ddaComp, aErr := agent.NewDDAWithOperator(&gcpEnv, params.name, kubeProvider, params.ddaOptions...)
		if aErr != nil {
			return aErr
		}

		if err = ddaComp.Export(ctx, &env.Agent.KubernetesAgentOutput); err != nil {
			return err
		}
	}

	for _, workload := range params.yamlWorkloads {
		_, err = yaml.NewConfigFile(ctx, workload.Name, &yaml.ConfigFileArgs{
			File: workload.Path,
		}, pulumi.Provider(kubeProvider))
		if err != nil {
			return err
		}
	}

	for _, appFunc := range params.workloadAppFuncs {
		_, err := appFunc(&gcpEnv, kubeProvider)
		if err != nil {
			return err
		}
	}

	return nil
}
