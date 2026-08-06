// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package provisioners

import (
	"path/filepath"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentwithoperatorparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/operator"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/operatorparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	gcpEnv "github.com/DataDog/datadog-agent/test/e2e-framework/resources/gcp"
	gcpfakeintake "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/gcp/fakeintake"
	gcpgke "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/gcp/gke"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-operator/test/e2e/common"
)

const (
	gkeProvisionerBaseID      = "gcp-gke-"
	defaultGKEProvisionerName = "gke"
)

// GKEProvisionerParams contains all the parameters needed to create a GKE environment.
type GKEProvisionerParams struct {
	name               string
	testName           string
	operatorOptions    []operatorparams.Option
	ddaOptions         []agentwithoperatorparams.Option
	k8sVersion         string
	kustomizeResources []string

	fakeintakeOptions []gcpfakeintake.Option
	extraConfigParams runner.ConfigMap
	yamlWorkloads     []YAMLWorkload
	workloadAppFuncs  []func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error)
	gkeOptions        []gcpgke.Option
}

func newGKEProvisionerParams() *GKEProvisionerParams {
	return &GKEProvisionerParams{
		name:               defaultGKEProvisionerName,
		testName:           "",
		ddaOptions:         []agentwithoperatorparams.Option{},
		operatorOptions:    []operatorparams.Option{},
		k8sVersion:         "",
		kustomizeResources: nil,
		fakeintakeOptions:  []gcpfakeintake.Option{},
		extraConfigParams:  runner.ConfigMap{},
		yamlWorkloads:      []YAMLWorkload{},
		workloadAppFuncs:   []func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error){},
		gkeOptions:         []gcpgke.Option{},
	}
}

// GKEProvisionerOption is a function that modifies GKEProvisionerParams.
type GKEProvisionerOption func(params *GKEProvisionerParams) error

// WithGKEName sets the name of the GKE provisioner.
func WithGKEName(name string) GKEProvisionerOption {
	return func(params *GKEProvisionerParams) error {
		params.name = name
		return nil
	}
}

// WithGKETestName sets the name of the test kustomize workload.
func WithGKETestName(name string) GKEProvisionerOption {
	return func(params *GKEProvisionerParams) error {
		params.testName = name
		return nil
	}
}

// WithGKEK8sVersion sets the Kubernetes version for the GKE cluster.
func WithGKEK8sVersion(k8sVersion string) GKEProvisionerOption {
	return func(params *GKEProvisionerParams) error {
		params.k8sVersion = k8sVersion
		return nil
	}
}

// WithGKEOperatorOptions adds options to the Datadog Operator installation.
func WithGKEOperatorOptions(opts ...operatorparams.Option) GKEProvisionerOption {
	return func(params *GKEProvisionerParams) error {
		params.operatorOptions = opts
		return nil
	}
}

// WithoutGKEOperator removes the Datadog Operator resource.
func WithoutGKEOperator() GKEProvisionerOption {
	return func(params *GKEProvisionerParams) error {
		params.operatorOptions = nil
		return nil
	}
}

// WithGKEDDAOptions adds options to the DatadogAgent resource.
func WithGKEDDAOptions(opts ...agentwithoperatorparams.Option) GKEProvisionerOption {
	return func(params *GKEProvisionerParams) error {
		params.ddaOptions = opts
		return nil
	}
}

// WithoutGKEDDA removes the DatadogAgent resource.
func WithoutGKEDDA() GKEProvisionerOption {
	return func(params *GKEProvisionerParams) error {
		params.ddaOptions = nil
		return nil
	}
}

// WithGKEExtraConfigParams adds extra Pulumi config parameters to the environment.
func WithGKEExtraConfigParams(configMap runner.ConfigMap) GKEProvisionerOption {
	return func(params *GKEProvisionerParams) error {
		params.extraConfigParams = configMap
		return nil
	}
}

// WithGKEKustomizeResources adds extra kustomize resources.
func WithGKEKustomizeResources(k []string) GKEProvisionerOption {
	return func(params *GKEProvisionerParams) error {
		params.kustomizeResources = k
		return nil
	}
}

// WithoutGKEFakeIntake removes the fake intake.
func WithoutGKEFakeIntake() GKEProvisionerOption {
	return func(params *GKEProvisionerParams) error {
		params.fakeintakeOptions = nil
		return nil
	}
}

// WithGKEFakeIntakeOptions adds options to the fake intake VM.
func WithGKEFakeIntakeOptions(opts ...gcpfakeintake.Option) GKEProvisionerOption {
	return func(params *GKEProvisionerParams) error {
		params.fakeintakeOptions = opts
		return nil
	}
}

// WithGKEYAMLWorkload adds a workload app to the environment for a YAML file.
func WithGKEYAMLWorkload(yamlWorkload YAMLWorkload) GKEProvisionerOption {
	return func(params *GKEProvisionerParams) error {
		params.yamlWorkloads = append(params.yamlWorkloads, yamlWorkload)
		return nil
	}
}

// WithGKEWorkloadApp adds a workload app to the environment.
func WithGKEWorkloadApp(appFunc func(e config.Env, kubeProvider *kubernetes.Provider) (*kubeComp.Workload, error)) GKEProvisionerOption {
	return func(params *GKEProvisionerParams) error {
		params.workloadAppFuncs = append(params.workloadAppFuncs, appFunc)
		return nil
	}
}

// WithGKEOptions adds options to the GKE cluster.
func WithGKEOptions(opts ...gcpgke.Option) GKEProvisionerOption {
	return func(params *GKEProvisionerParams) error {
		params.gkeOptions = append(params.gkeOptions, opts...)
		return nil
	}
}

// WithGKEAutopilot creates a GKE Autopilot cluster.
func WithGKEAutopilot() GKEProvisionerOption {
	return WithGKEOptions(gcpgke.WithAutopilot())
}

func newGKEExtraConfig(params *GKEProvisionerParams) runner.ConfigMap {
	extraConfig := params.extraConfigParams
	extraConfig.Merge(runner.ConfigMap{
		"ddagent:imagePullRegistry": auto.ConfigValue{Value: "669783387624.dkr.ecr.us-east-1.amazonaws.com"},
		"ddagent:imagePullUsername": auto.ConfigValue{Value: "AWS"},
		"ddagent:imagePullPassword": auto.ConfigValue{Value: common.ImgPullPassword, Secret: true},
	})
	if params.k8sVersion != "" {
		extraConfig.Merge(runner.ConfigMap{
			"ddinfra:kubernetesVersion": auto.ConfigValue{Value: params.k8sVersion},
		})
	}
	return extraConfig
}

// GKEProvisioner creates a new operator-focused GKE provisioner.
func GKEProvisioner(opts ...GKEProvisionerOption) provisioners.TypedProvisioner[environments.Kubernetes] {
	params := newGKEProvisionerParams()
	_ = optional.ApplyOptions(params, opts)

	return provisioners.NewTypedPulumiProvisioner(gkeProvisionerBaseID+params.name, func(ctx *pulumi.Context, env *environments.Kubernetes) error {
		runParams := newGKEProvisionerParams()
		_ = optional.ApplyOptions(runParams, opts)

		return gkeRunFunc(ctx, env, runParams)
	}, newGKEExtraConfig(params))
}

func gkeRunFunc(ctx *pulumi.Context, env *environments.Kubernetes, params *GKEProvisionerParams) error {
	gcpEnvironment, err := gcpEnv.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	cluster, err := gcpgke.NewGKECluster(gcpEnvironment, params.gkeOptions...)
	if err != nil {
		return err
	}
	if err = cluster.Export(ctx, &env.KubernetesCluster.ClusterOutput); err != nil {
		return err
	}

	if gcpEnvironment.InitOnly() {
		return nil
	}

	if params.fakeintakeOptions != nil {
		fakeIntake, fakeIntakeErr := gcpfakeintake.NewVMInstance(gcpEnvironment, params.fakeintakeOptions...)
		if fakeIntakeErr != nil {
			return fakeIntakeErr
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

	ns, err := corev1.NewNamespace(ctx, gcpEnvironment.Namer.ResourceName("k8s-namespace"), &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String(common.NamespaceName),
		},
	}, pulumi.Provider(cluster.KubeProvider))
	if err != nil {
		return err
	}

	kustomizeAppFunc := KustomizeWorkloadAppFunc(params.testName, params.kustomizeResources)
	e2eKustomize, err := kustomizeAppFunc(&gcpEnvironment, cluster.KubeProvider)
	if err != nil {
		return err
	}

	var operatorComp *operator.Operator
	if params.operatorOptions != nil {
		operatorOpts := []pulumi.ResourceOption{
			pulumi.DependsOn([]pulumi.Resource{e2eKustomize, ns}),
		}
		params.operatorOptions = append(params.operatorOptions, operatorparams.WithPulumiResourceOptions(operatorOpts...))

		operatorComp, err = operator.NewOperator(&gcpEnvironment, gcpEnvironment.Namer.ResourceName("operator"), cluster.KubeProvider, params.operatorOptions...)
		if err != nil {
			return err
		}
	}

	if len(params.ddaOptions) > 0 && params.operatorOptions != nil {
		ddaResourceOpts := []pulumi.ResourceOption{
			pulumi.DependsOn([]pulumi.Resource{e2eKustomize, operatorComp}),
		}
		params.ddaOptions = append(params.ddaOptions, agentwithoperatorparams.WithPulumiResourceOptions(ddaResourceOpts...))

		ddaComp, ddaErr := agent.NewDDAWithOperator(&gcpEnvironment, params.name, cluster.KubeProvider, params.ddaOptions...)
		if ddaErr != nil {
			return ddaErr
		}

		if err = ddaComp.Export(ctx, &env.Agent.KubernetesAgentOutput); err != nil {
			return err
		}
	} else {
		env.Agent = nil
	}

	for _, workload := range params.yamlWorkloads {
		_, err = yaml.NewConfigFile(ctx, workload.Name, &yaml.ConfigFileArgs{
			File: filepath.Clean(workload.Path),
		}, pulumi.Provider(cluster.KubeProvider))
		if err != nil {
			return err
		}
	}

	for _, appFunc := range params.workloadAppFuncs {
		_, err := appFunc(&gcpEnvironment, cluster.KubeProvider)
		if err != nil {
			return err
		}
	}

	return nil
}
