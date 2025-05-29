// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package provisioners

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"
	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentwithoperatorparams"
	fakeintakeComp "github.com/DataDog/test-infra-definitions/components/datadog/fakeintake"
	"github.com/DataDog/test-infra-definitions/components/datadog/operator"
	"github.com/DataDog/test-infra-definitions/components/datadog/operatorparams"
	kubeComp "github.com/DataDog/test-infra-definitions/components/kubernetes"
	"github.com/DataDog/test-infra-definitions/resources/local"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/kustomize"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-operator/test/e2e/common"
)

// KubernetesProvisionerParams contains all the parameters needed to create a Kubernetes environment
type KubernetesProvisionerParams struct {
	name               string
	testName           string
	operatorOptions    []operatorparams.Option
	ddaOptions         []agentwithoperatorparams.Option
	k8sVersion         string
	kustomizeResources []string

	fakeintakeOptions []fakeintake.Option
	extraConfigParams runner.ConfigMap
	yamlWorkloads     []YAMLWorkload
	workloadAppFuncs  []func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error)
	local             bool
}

func newKubernetesProvisionerParams() *KubernetesProvisionerParams {
	return &KubernetesProvisionerParams{
		name:               "local-kind",
		testName:           "",
		ddaOptions:         []agentwithoperatorparams.Option{},
		operatorOptions:    []operatorparams.Option{},
		k8sVersion:         common.K8sVersion,
		kustomizeResources: nil,
		fakeintakeOptions:  []fakeintake.Option{},
		extraConfigParams:  runner.ConfigMap{},
		yamlWorkloads:      []YAMLWorkload{},
		workloadAppFuncs:   []func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error){},
		local:              false,
	}
}

// KubernetesProvisionerOption is a function that modifies the KubernetesProvisionerParams
type KubernetesProvisionerOption func(params *KubernetesProvisionerParams) error

// WithTestName sets the name of the test
func WithTestName(name string) KubernetesProvisionerOption {
	return func(params *KubernetesProvisionerParams) error {
		params.testName = name
		return nil
	}
}

// WithK8sVersion sets the kubernetes version
func WithK8sVersion(k8sVersion string) KubernetesProvisionerOption {
	return func(params *KubernetesProvisionerParams) error {
		params.k8sVersion = k8sVersion
		return nil
	}
}

// WithOperatorOptions adds options to the DatadogAgent resource
func WithOperatorOptions(opts ...operatorparams.Option) KubernetesProvisionerOption {
	return func(params *KubernetesProvisionerParams) error {
		params.operatorOptions = opts
		return nil
	}
}

// WithDDAOptions adds options to the DatadogAgent resource
func WithDDAOptions(opts ...agentwithoperatorparams.Option) KubernetesProvisionerOption {
	return func(params *KubernetesProvisionerParams) error {
		params.ddaOptions = opts
		return nil
	}
}

// WithoutDDA removes the DatadogAgent resource
func WithoutDDA() KubernetesProvisionerOption {
	return func(params *KubernetesProvisionerParams) error {
		params.ddaOptions = nil
		return nil
	}
}

// WithLocal uses the localKindRunFunc to create a local kind environment
func WithLocal(local bool) KubernetesProvisionerOption {
	return func(params *KubernetesProvisionerParams) error {
		params.local = local
		return nil
	}
}

// YAMLWorkload defines the parameters for a Kubernetes resource's YAML file
type YAMLWorkload struct {
	Name string
	Path string
}

// WithYAMLWorkload adds a workload app to the environment for given YAML file path
func WithYAMLWorkload(yamlWorkload YAMLWorkload) KubernetesProvisionerOption {
	return func(params *KubernetesProvisionerParams) error {
		params.yamlWorkloads = append(params.yamlWorkloads, yamlWorkload)
		return nil
	}
}

// KubernetesProvisioner generic Kubernetes provisioner wrapper that creates a new provisioner
// Inspired by https://github.com/DataDog/datadog-agent/blob/main/test/new-e2e/pkg/environments/local/kubernetes/kind.go
func KubernetesProvisioner(opts ...KubernetesProvisionerOption) provisioners.TypedProvisioner[environments.Kubernetes] {
	// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
	// and it's easy to forget about it, leading to hard to debug issues.
	var provisioner provisioners.TypedProvisioner[environments.Kubernetes]

	params := newKubernetesProvisionerParams()
	_ = optional.ApplyOptions(params, opts)
	inCI := os.Getenv("GITLAB_CI")

	if !params.local || strings.ToLower(inCI) == "true" {
		provisioner = provisioners.NewTypedPulumiProvisioner("gke", func(ctx *pulumi.Context, env *environments.Kubernetes) error {
			// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
			// and it's easy to forget about it, leading to hard to debug issues.
			pprams := newKubernetesProvisionerParams()
			pprams.extraConfigParams = runner.ConfigMap{
				"ddinfra:kubernetesVersion": auto.ConfigValue{Value: "1.32"},
			}
			_ = optional.ApplyOptions(pprams, opts)

			return GkeRunFunc(ctx, env, pprams)

		}, params.extraConfigParams)
		return provisioner
	}

	provisioner = provisioners.NewTypedPulumiProvisioner("local-kind", func(ctx *pulumi.Context, env *environments.Kubernetes) error {
		// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
		// and it's easy to forget about it, leading to hard to debug issues.
		pprams := newKubernetesProvisionerParams()
		_ = optional.ApplyOptions(pprams, opts)

		return localKindRunFunc(ctx, env, pprams)

	}, params.extraConfigParams)

	return provisioner
}

// localKindRunFunc is the Pulumi run function that runs the local Kind provisioner
func localKindRunFunc(ctx *pulumi.Context, env *environments.Kubernetes, params *KubernetesProvisionerParams) error {
	localEnv, err := local.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	kindCluster, err := kubeComp.NewLocalKindCluster(&localEnv, localEnv.CommonNamer().ResourceName("local-kind"), params.k8sVersion)
	if err != nil {
		return err
	}

	if err = kindCluster.Export(ctx, &env.KubernetesCluster.ClusterOutput); err != nil {
		return err
	}

	// Build Kubernetes provider
	kindKubeProvider, err := kubernetes.NewProvider(ctx, localEnv.CommonNamer().ResourceName("k8s-provider"), &kubernetes.ProviderArgs{
		Kubeconfig:            kindCluster.KubeConfig,
		EnableServerSideApply: pulumi.BoolPtr(true),
	})
	if err != nil {
		return err
	}

	if params.fakeintakeOptions != nil {
		fakeintakeOpts := []fakeintake.Option{fakeintake.WithLoadBalancer()}
		params.fakeintakeOptions = append(fakeintakeOpts, params.fakeintakeOptions...)

		fakeIntake, intakeErr := fakeintakeComp.NewLocalDockerFakeintake(&localEnv, "fakeintake")
		if intakeErr != nil {
			return intakeErr
		}
		if err = fakeIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput); err != nil {
			return err
		}

		if params.ddaOptions != nil {
			params.ddaOptions = append(params.ddaOptions, agentwithoperatorparams.WithFakeIntake(fakeIntake))
		}
	} else {
		env.FakeIntake = nil
	}

	ns, err := corev1.NewNamespace(ctx, localEnv.CommonNamer().ResourceName("k8s-namespace"), &corev1.NamespaceArgs{Metadata: &metav1.ObjectMetaArgs{
		Name: pulumi.String("e2e-operator"),
	}}, pulumi.Provider(kindKubeProvider))

	if err != nil {
		return err
	}

	// Install kustomizations
	kustomizeAppFunc := KustomizeWorkloadAppFunc(params.testName, params.kustomizeResources)

	e2eKustomize, err := kustomizeAppFunc(&localEnv, kindKubeProvider)
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

		operatorComp, err = operator.NewOperator(&localEnv, localEnv.CommonNamer().ResourceName("operator"), kindKubeProvider, params.operatorOptions...)
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

		ddaComp, aErr := agent.NewDDAWithOperator(&localEnv, "agent-with-operator", kindKubeProvider, params.ddaOptions...)
		if aErr != nil {
			return aErr
		}

		if err = ddaComp.Export(ctx, &env.Agent.KubernetesAgentOutput); err != nil {
			return err
		}
	} else {
		env.Agent = nil
	}

	for _, workload := range params.yamlWorkloads {
		_, err = yaml.NewConfigFile(ctx, workload.Name, &yaml.ConfigFileArgs{
			File: workload.Path,
		}, pulumi.Provider(kindKubeProvider))
		if err != nil {
			return err
		}
	}

	for _, appFunc := range params.workloadAppFuncs {
		_, err := appFunc(&localEnv, kindKubeProvider)
		if err != nil {
			return err
		}
	}

	return nil
}

// KustomizeWorkloadAppFunc Installs the operator e2e kustomize directory and any extra kustomize resources
func KustomizeWorkloadAppFunc(name string, extraKustomizeResources []string) func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
	return func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
		k8sComponent := &kubeComp.Workload{}
		if err := e.Ctx().RegisterComponentResource("dd:apps", fmt.Sprintf("kustomize-%s", name), k8sComponent, pulumi.DeleteBeforeReplace(true)); err != nil {
			return nil, err
		}

		// Install kustomizations
		kustomizeDirPath, err := filepath.Abs(NewMgrKustomizeDirPath)
		if err != nil {
			return nil, err
		}

		err = UpdateKustomization(kustomizeDirPath, extraKustomizeResources)
		if err != nil {
			return nil, err
		}
		kustomizeOpts := []pulumi.ResourceOption{
			pulumi.Provider(kubeProvider),
			pulumi.Parent(k8sComponent),
		}

		_, err = kustomize.NewDirectory(e.Ctx(), "e2e-manager",
			kustomize.DirectoryArgs{
				Directory: pulumi.String(kustomizeDirPath),
			}, kustomizeOpts...)
		if err != nil {
			return nil, err
		}
		return k8sComponent, nil
	}
}

// YAMLWorkloadAppFunc Applies a Kubernetes resource yaml file
func YAMLWorkloadAppFunc(yamlWorkload YAMLWorkload) func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
	return func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error) {
		k8sComponent := &kubeComp.Workload{}
		if err := e.Ctx().RegisterComponentResource("dd:apps", "k8s-apply", k8sComponent); err != nil {
			return nil, err
		}
		_, err := yaml.NewConfigFile(e.Ctx(), yamlWorkload.Name, &yaml.ConfigFileArgs{
			File: yamlWorkload.Path,
		}, pulumi.Provider(kubeProvider))
		if err != nil {
			return nil, err
		}
		return k8sComponent, nil
	}
}
