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

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentwithoperatorparams"
	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/operator"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/operatorparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/local"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	kindvmprovisioner "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/kustomize"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-operator/test/e2e/common"
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

// newKindVMRunOpts Translates the generic KubernetesProvisionerParams into kindvm.RunOption for the AWS Kind-on-VM provisioner
func newKindVMRunOpts(params *KubernetesProvisionerParams) []kindvm.RunOption {
	provisionerName := provisionerBaseID + params.name

	runOpts := []kindvm.RunOption{
		kindvm.WithName(provisionerName),
		kindvm.WithVMOptions(ec2.WithUserData(UserData), ec2.WithInstanceType("m5.xlarge")),
	}

	// Add operator deployment if options are provided
	if params.operatorOptions != nil {
		runOpts = append(runOpts, kindvm.WithDeployOperator())
		runOpts = append(runOpts, kindvm.WithOperatorOptions(params.operatorOptions...))

		// Pass DDA options only when explicitly provided
		if len(params.ddaOptions) > 0 {
			runOpts = append(runOpts, kindvm.WithOperatorDDAOptions(params.ddaOptions...))
		}
		// Note: When WithoutDDA() is called, no DDA options are passed.
		// The e2e-framework (fixed in PR #45390) now correctly handles this case
		// by not deploying a DDA when operatorDDAOptions is nil or empty.
	}

	// Add fakeintake options if provided
	if params.fakeintakeOptions != nil {
		runOpts = append(runOpts, kindvm.WithFakeintakeOptions(params.fakeintakeOptions...))
	} else {
		runOpts = append(runOpts, kindvm.WithoutFakeIntake())
	}

	// Add kustomize workload
	runOpts = append(runOpts, kindvm.WithWorkloadApp(KustomizeWorkloadAppFunc(params.testName, params.kustomizeResources)))

	// Add YAML workloads
	for _, yamlWorkload := range params.yamlWorkloads {
		runOpts = append(runOpts, kindvm.WithWorkloadApp(YAMLWorkloadAppFunc(yamlWorkload)))
	}

	return runOpts
}

// newKindVMExtraConfig returns the extra config params for the Kind-on-VM provisioner
func newKindVMExtraConfig(params *KubernetesProvisionerParams) runner.ConfigMap {
	extraConfig := params.extraConfigParams
	extraConfig.Merge(runner.ConfigMap{
		"ddinfra:kubernetesVersion": auto.ConfigValue{Value: params.k8sVersion},
		"ddagent:imagePullRegistry": auto.ConfigValue{Value: "669783387624.dkr.ecr.us-east-1.amazonaws.com"},
		"ddagent:imagePullUsername": auto.ConfigValue{Value: "AWS"},
		"ddagent:imagePullPassword": auto.ConfigValue{Value: common.ImgPullPassword},
	})
	return extraConfig
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

// WithoutDDA removes the DatadogAgent resource and prevents DDA deployment.
// This is used during cleanup to avoid deploying a new DDA before Pulumi stack destroy.
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
// Inspired by https://github.com/DataDog/datadog-agent/blob/main/test/e2e-framework/testing/provisioners/aws/kubernetes/kindvm/kind.go
func KubernetesProvisioner(opts ...KubernetesProvisionerOption) provisioners.TypedProvisioner[environments.Kubernetes] {
	// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
	// and it's easy to forget about it, leading to hard to debug issues.
	var provisioner provisioners.TypedProvisioner[environments.Kubernetes]

	params := newKubernetesProvisionerParams()
	_ = optional.ApplyOptions(params, opts)
	inCI := os.Getenv("GITLAB_CI")

	if !params.local || strings.ToLower(inCI) == "true" {
		runOpts := newKindVMRunOpts(params)
		extraConfig := newKindVMExtraConfig(params)
		provisioner = kindvmprovisioner.Provisioner(
			kindvmprovisioner.WithRunOptions(runOpts...),
			kindvmprovisioner.WithExtraConfigParams(extraConfig),
		)
		return provisioner
	}

	provisionerName := "local-" + params.name

	provisioner = provisioners.NewTypedPulumiProvisioner(provisionerName, func(ctx *pulumi.Context, env *environments.Kubernetes) error {
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

	// Setup DDA options (only if DDA options are explicitly provided)
	if len(params.ddaOptions) > 0 && params.operatorOptions != nil {
		ddaResourceOpts := []pulumi.ResourceOption{
			pulumi.DependsOn([]pulumi.Resource{e2eKustomize, operatorComp}),
		}
		params.ddaOptions = append(
			params.ddaOptions,
			agentwithoperatorparams.WithPulumiResourceOptions(ddaResourceOpts...))

		ddaComp, aErr := agent.NewDDAWithOperator(&localEnv, params.name, kindKubeProvider, params.ddaOptions...)
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
