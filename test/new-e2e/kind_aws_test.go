// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build e2e
// +build e2e

package e2e

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-operator/test/new-e2e/common"
	"github.com/DataDog/datadog-operator/test/new-e2e/provisioners"
	"github.com/DataDog/test-infra-definitions/components/datadog/operatorparams"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"testing"
)

type awsKindSuite struct {
	k8sSuite
}

// not working yet
func TestAWSKindSuite(t *testing.T) {
	operatorOptions := make([]operatorparams.Option, 0)
	operatorOptions = append(operatorOptions, defaultOperatorOpts...)

	provisionerOptions := []provisioners.KubernetesProvisionerOption{
		provisioners.WithK8sVersion(common.K8sVersion),
		provisioners.WithOperatorOptions(operatorOptions...),
		provisioners.WithoutDDA(),
		provisioners.WithExtraConfigParams(runner.ConfigMap{
			"ddinfra:kubernetesVersion": auto.ConfigValue{Value: common.K8sVersion},
			"ddagent:deploy":            auto.ConfigValue{Value: "false"},
			"ddtestworkload:deploy":     auto.ConfigValue{Value: "false"},
			"dddogstatsd:deploy":        auto.ConfigValue{Value: "false"},
			"ddagent:imagePullRegistry": auto.ConfigValue{Value: "669783387624.dkr.ecr.us-east-1.amazonaws.com"},
			"ddagent:imagePullUsername": auto.ConfigValue{Value: "AWS"},
			"ddagent:imagePullPassword": auto.ConfigValue{Value: common.ImgPullPassword},
		}),
	}

	e2e.Run(t, &awsKindSuite{}, e2e.WithProvisioner(provisioners.KubernetesProvisioner(provisioners.AWSKindRunFunc, provisionerOptions...)), e2e.WithSkipDeleteOnFailure(), e2e.WithDevMode())
}
