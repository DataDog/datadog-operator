// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package k8ssuite

import (
	"fmt"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/operatorparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-operator/test/e2e/common"
	"github.com/DataDog/datadog-operator/test/e2e/provisioners"
	"strings"
	"testing"
)

type awsKindSuite struct {
	k8sSuite
}

func TestAWSKindSuite(t *testing.T) {
	operatorOptions := []operatorparams.Option{
		operatorparams.WithNamespace(common.NamespaceName),
		operatorparams.WithOperatorFullImagePath(common.OperatorImageName),
		operatorparams.WithHelmValues(`installCRDs: false
rbac:
  create: false
serviceAccount:
  create: false
  name: datadog-operator-e2e-controller-manager
`),
	}

	provisionerOptions := []provisioners.KubernetesProvisionerOption{
		provisioners.WithTestName("e2e-operator"),
		provisioners.WithOperatorOptions(operatorOptions...),
		provisioners.WithoutDDA(),
	}

	e2eOpts := []e2e.SuiteOption{
		e2e.WithStackName(fmt.Sprintf("operator-awskind-%s", strings.ReplaceAll(common.K8sVersion, ".", "-"))),
		e2e.WithProvisioner(provisioners.KubernetesProvisioner(provisionerOptions...)),
	}

	e2e.Run(t, &awsKindSuite{}, e2eOpts...)
}
