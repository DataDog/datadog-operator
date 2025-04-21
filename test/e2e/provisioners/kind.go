// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package provisioners

import (
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/kubernetes"

	"path/filepath"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"
	"github.com/DataDog/datadog-operator/test/e2e/common"
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
)

const (
	provisionerBaseID      = "aws-kind"
	defaultProvisionerName = "kind"
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
		name:               defaultProvisionerName,
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

// newAWSK8sProvisionerOpts Translates the generic KubernetesProvisionerParams into a list of awskubernetes.ProvisionerOption for the AWS Kind provisioner
func newAWSK8sProvisionerOpts(params *KubernetesProvisionerParams) []awskubernetes.ProvisionerOption {
	provisionerName := provisionerBaseID + params.name

	extraConfig := params.extraConfigParams
	extraConfig.Merge(runner.ConfigMap{
		"ddinfra:kubernetesVersion": auto.ConfigValue{Value: params.k8sVersion},
		"ddagent:imagePullRegistry": auto.ConfigValue{Value: "669783387624.dkr.ecr.us-east-1.amazonaws.com"},
		"ddagent:imagePullUsername": auto.ConfigValue{Value: "AWS"},
		"ddagent:imagePullPassword": auto.ConfigValue{Value: common.ImgPullPassword},
	})

	newOpts := []awskubernetes.ProvisionerOption{
		awskubernetes.WithName(provisionerName),
		awskubernetes.WithOperator(),
		awskubernetes.WithOperatorDDAOptions(params.ddaOptions...),
		awskubernetes.WithOperatorOptions(params.operatorOptions...),
		awskubernetes.WithExtraConfigParams(extraConfig),
		awskubernetes.WithWorkloadApp(KustomizeWorkloadAppFunc(params.testName, params.kustomizeResources)),
		awskubernetes.WithFakeIntakeOptions(params.fakeintakeOptions...),
		awskubernetes.WithEC2VMOptions([]ec2.VMOption{ec2.WithUserData(UserData), ec2.WithInstanceType("m5.xlarge")}...),
	}

	for _, yamlWorkload := range params.yamlWorkloads {
		newOpts = append(newOpts, awskubernetes.WithWorkloadApp(YAMLWorkloadAppFunc(yamlWorkload)))
	}

	return newOpts
}

// KubernetesProvisionerOption is a function that modifies the KubernetesProvisionerParams
type KubernetesProvisionerOption func(params *KubernetesProvisionerParams) error

// WithName sets the name of the provisioner
func WithName(name string) KubernetesProvisionerOption {
	return func(params *KubernetesProvisionerParams) error {
		params.name = name
		return nil
	}
}

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

// WithoutOperator removes the Datadog Operator resource
func WithoutOperator() KubernetesProvisionerOption {
	return func(params *KubernetesProvisionerParams) error {
		params.operatorOptions = nil
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

// WithExtraConfigParams adds extra config parameters to the environment
func WithExtraConfigParams(configMap runner.ConfigMap) KubernetesProvisionerOption {
	return func(params *KubernetesProvisionerParams) error {
		params.extraConfigParams = configMap
		return nil
	}
}

// WithKustomizeResources adds extra kustomize resources
func WithKustomizeResources(k []string) KubernetesProvisionerOption {
	return func(params *KubernetesProvisionerParams) error {
		params.kustomizeResources = k
		return nil
	}
}

// WithoutFakeIntake removes the fake intake
func WithoutFakeIntake() KubernetesProvisionerOption {
	return func(params *KubernetesProvisionerParams) error {
		params.fakeintakeOptions = nil
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

// WithWorkloadApp adds a workload app to the environment
func WithWorkloadApp(appFunc func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error)) KubernetesProvisionerOption {
	return func(params *KubernetesProvisionerParams) error {
		params.workloadAppFuncs = append(params.workloadAppFuncs, appFunc)
		return nil
	}
}

// KubernetesProvisioner generic Kubernetes provisioner wrapper that creates a new provisioner
// Inspired by https://github.com/DataDog/datadog-agent/blob/main/test/new-e2e/pkg/environments/local/kubernetes/kind.go
func KubernetesProvisioner(opts ...KubernetesProvisionerOption) provisioners.TypedProvisioner[environments.Kubernetes] {
	// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
	// and it's easy to forget about it, leading to hard to debug issues.
	var awsK8sOpts []awskubernetes.ProvisionerOption
	var provisioner provisioners.TypedProvisioner[environments.Kubernetes]

	params := newKubernetesProvisionerParams()
	_ = optional.ApplyOptions(params, opts)
	inCI := os.Getenv("GITLAB_CI")

	if !params.local || strings.ToLower(inCI) == "true" {
		awsK8sOpts = newAWSK8sProvisionerOpts(params)
		provisioner = awskubernetes.KindProvisioner(awsK8sOpts...)
		return provisioner
	}

	provisionerName := "local-" + params.name

	provisioner = provisioners.NewTypedPulumiProvisioner(provisionerName, func(ctx *pulumi.Context, env *environments.Kubernetes) error {
		// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
		// and it's easy to forget about it, leading to hard to debug issues.
		params := newKubernetesProvisionerParams()
		_ = optional.ApplyOptions(params, opts)

		return localKindRunFunc(ctx, env, params)

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

		fakeIntake, err := fakeintakeComp.NewLocalDockerFakeintake(&localEnv, "fakeintake")
		if err != nil {
			return err
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

		ddaComp, err := agent.NewDDAWithOperator(&localEnv, params.name, kindKubeProvider, params.ddaOptions...)
		if err != nil {
			return err
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
