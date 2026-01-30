// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package provisioners

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/eks"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	eksprovisioner "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/kubernetes/eks"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"

	"github.com/DataDog/datadog-operator/test/e2e/common"
)

const (
	eksProvisionerBaseID      = "aws-eks-"
	defaultEKSProvisionerName = "eks"
)

// EKSProvisionerParams contains all the parameters needed to create an EKS environment
type EKSProvisionerParams struct {
	name              string
	k8sVersion        string
	extraConfigParams runner.ConfigMap
	eksOptions        []eks.Option
}

func newEKSProvisionerParams() *EKSProvisionerParams {
	return &EKSProvisionerParams{
		name:              defaultEKSProvisionerName,
		k8sVersion:        common.K8sVersion,
		extraConfigParams: runner.ConfigMap{},
		eksOptions:        []eks.Option{},
	}
}

// EKSProvisionerOption is a function that modifies the EKSProvisionerParams
type EKSProvisionerOption func(params *EKSProvisionerParams) error

// WithEKSName sets the name of the EKS provisioner
func WithEKSName(name string) EKSProvisionerOption {
	return func(params *EKSProvisionerParams) error {
		params.name = name
		return nil
	}
}

// WithEKSK8sVersion sets the Kubernetes version for the EKS cluster
func WithEKSK8sVersion(k8sVersion string) EKSProvisionerOption {
	return func(params *EKSProvisionerParams) error {
		params.k8sVersion = k8sVersion
		return nil
	}
}

// WithEKSExtraConfigParams adds extra config parameters to the environment
func WithEKSExtraConfigParams(configMap runner.ConfigMap) EKSProvisionerOption {
	return func(params *EKSProvisionerParams) error {
		params.extraConfigParams = configMap
		return nil
	}
}

// WithEKSLinuxNodeGroup adds a Linux (amd64) node group to the EKS cluster
func WithEKSLinuxNodeGroup() EKSProvisionerOption {
	return func(params *EKSProvisionerParams) error {
		params.eksOptions = append(params.eksOptions, eks.WithLinuxNodeGroup())
		return nil
	}
}

// WithEKSLinuxARMNodeGroup adds a Linux (arm64) node group to the EKS cluster
func WithEKSLinuxARMNodeGroup() EKSProvisionerOption {
	return func(params *EKSProvisionerParams) error {
		params.eksOptions = append(params.eksOptions, eks.WithLinuxARMNodeGroup())
		return nil
	}
}

// newEKSRunOpts translates EKSProvisionerParams into eks.RunOption for the EKS provisioner
func newEKSRunOpts(params *EKSProvisionerParams) []eks.RunOption {
	runOpts := []eks.RunOption{
		eks.WithName(eksProvisionerBaseID + params.name),
		eks.WithoutFakeIntake(),
		eks.WithoutAgent(),
	}

	// Add EKS options
	if len(params.eksOptions) > 0 {
		runOpts = append(runOpts, eks.WithEKSOptions(params.eksOptions...))
	}

	return runOpts
}

// newEKSExtraConfig returns the extra config params for the EKS provisioner
func newEKSExtraConfig(params *EKSProvisionerParams) runner.ConfigMap {
	extraConfig := params.extraConfigParams
	extraConfig.Merge(runner.ConfigMap{
		"ddinfra:kubernetesVersion": auto.ConfigValue{Value: params.k8sVersion},
		// EKS requires a Linux node group
		"ddinfra:eksLinuxNodeGroup": auto.ConfigValue{Value: "true"},
	})
	return extraConfig
}

// EKSProvisioner creates a new EKS provisioner for E2E tests
func EKSProvisioner(opts ...EKSProvisionerOption) provisioners.TypedProvisioner[environments.Kubernetes] {
	params := newEKSProvisionerParams()
	_ = optional.ApplyOptions(params, opts)

	runOpts := newEKSRunOpts(params)
	extraConfig := newEKSExtraConfig(params)

	return eksprovisioner.Provisioner(
		eksprovisioner.WithRunOptions(runOpts...),
		eksprovisioner.WithExtraConfigParams(extraConfig),
	)
}
