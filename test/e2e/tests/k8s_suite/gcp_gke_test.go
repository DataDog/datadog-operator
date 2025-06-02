// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package k8ssuite

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-operator/test/e2e/common"
	"github.com/DataDog/datadog-operator/test/e2e/provisioners"
	"github.com/DataDog/test-infra-definitions/components/datadog/operatorparams"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"strings"
	"testing"
)

type gkeSuite struct {
	k8sSuite
}

func TestGKESuite(t *testing.T) {
	operatorOptions := []operatorparams.Option{
		operatorparams.WithNamespace(common.NamespaceName),
		operatorparams.WithOperatorFullImagePath(common.OperatorImageName),
		operatorparams.WithHelmValues(`
installCRDs: false
imagePullSecrets: 
  - name: registry-credentials
`),
	}
	t.Logf("WHAT IS LENGTH OF PW: " + fmt.Sprint(len(common.ImgPullPassword)))

	provisionerOptions := []provisioners.KubernetesProvisionerOption{
		provisioners.WithTestName("e2e-operator"),
		provisioners.WithOperatorOptions(operatorOptions...),
		provisioners.WithoutDDA(),
		provisioners.WithExtraConfigParams(runner.ConfigMap{
			"ddinfra:kubernetesVersion": auto.ConfigValue{Value: common.K8sVersion},
			"ddagent:imagePullRegistry": auto.ConfigValue{Value: "669783387624.dkr.ecr.us-east-1.amazonaws.com"},
			"ddagent:imagePullUsername": auto.ConfigValue{Value: "AWS"},
			"ddagent:imagePullPassword": auto.ConfigValue{Value: common.ImgPullPassword},
			"ddinfra:env":               auto.ConfigValue{Value: "gcp/agent-sandbox"},
		}),
	}

	e2eOpts := []e2e.SuiteOption{
		e2e.WithStackName(fmt.Sprintf("operator-gke-%s", strings.ReplaceAll(common.K8sVersion, ".", "-"))),
		e2e.WithProvisioner(provisioners.KubernetesProvisioner(provisionerOptions...)),
		e2e.WithDevMode(),
		e2e.WithSkipDeleteOnFailure(),
	}

	e2e.Run(t, &gkeSuite{}, e2eOpts...)
}
