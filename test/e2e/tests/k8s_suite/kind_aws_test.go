// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package k8ssuite

import (
	"fmt"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentwithoperatorparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/operatorparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-operator/test/e2e/common"
	"github.com/DataDog/datadog-operator/test/e2e/provisioners"
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

	// NOTE: We use WithDDAOptions with the correct namespace instead of WithoutDDA() due to a bug
	// in the e2e-framework v0.75.0-rc.7 where WithoutDDA() doesn't properly disable DDA deployment.
	// The e2e-framework checks `operatorDDAOptions != nil` instead of `len(operatorDDAOptions) > 0`,
	// causing DDA deployment with default namespace "datadog" even when WithoutDDA() is called.
	// See CI_MIGRATION_ANALYSIS.md for details.
	ddaOptions := []agentwithoperatorparams.Option{
		agentwithoperatorparams.WithNamespace(common.NamespaceName),
	}

	provisionerOptions := []provisioners.KubernetesProvisionerOption{
		provisioners.WithTestName("e2e-operator"),
		provisioners.WithOperatorOptions(operatorOptions...),
		provisioners.WithDDAOptions(ddaOptions...),
	}

	e2eOpts := []e2e.SuiteOption{
		e2e.WithStackName(fmt.Sprintf("operator-awskind-%s", strings.ReplaceAll(common.K8sVersion, ".", "-"))),
		e2e.WithProvisioner(provisioners.KubernetesProvisioner(provisionerOptions...)),
	}

	e2e.Run(t, &awsKindSuite{}, e2eOpts...)
}
