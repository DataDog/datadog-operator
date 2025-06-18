// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package k8ssuite

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-operator/test/e2e/common"
	"github.com/DataDog/datadog-operator/test/e2e/provisioners"
	"github.com/DataDog/test-infra-definitions/components/datadog/operatorparams"
	"testing"
)

type gkeSuite struct {
	k8sSuite
}

func TestGKESuite(t *testing.T) {
	operatorOptions := []operatorparams.Option{
		operatorparams.WithHelmValues(`
installCRDs: false
`),
		operatorparams.WithNamespace(common.NamespaceName),
	}

	provisionerOptions := []provisioners.KubernetesProvisionerOption{
		//provisioners.WithTestName("e2e-operator"),
		provisioners.WithOperatorOptions(operatorOptions...),
		provisioners.WithoutDDA(),
		//provisioners.WithExtraConfigParams(runner.ConfigMap{
		//	"ddagent:imagePullRegistry": auto.ConfigValue{Value: "669783387624.dkr.ecr.us-east-1.amazonaws.com"},
		//	"ddagent:imagePullUsername": auto.ConfigValue{Value: "AWS"},
		//	"ddagent:imagePullPassword": auto.ConfigValue{Value: common.ImgPullPassword},
		//	"ddinfra:env":               auto.ConfigValue{Value: "gcp/agent-qa"},
		//}),
	}

	e2eOpts := []e2e.SuiteOption{
		e2e.WithProvisioner(provisioners.KubernetesProvisioner(provisionerOptions...)),
		e2e.WithDevMode(),
		e2e.WithSkipDeleteOnFailure(),
	}

	e2e.Run(t, &gkeSuite{}, e2eOpts...)
}
