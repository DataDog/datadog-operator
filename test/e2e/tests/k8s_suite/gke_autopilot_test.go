// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package k8ssuite

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/operatorparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-operator/test/e2e/common"
	"github.com/DataDog/datadog-operator/test/e2e/provisioners"
	"github.com/DataDog/datadog-operator/test/e2e/tests/utils"
	"github.com/stretchr/testify/assert"
)

type gkeAutopilotSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestGKEAutopilotSuite(t *testing.T) {
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

	provisionerOptions := []provisioners.GKEProvisionerOption{
		provisioners.WithGKEName("operator-autopilot"),
		provisioners.WithGKETestName("e2e-operator-gke-autopilot"),
		provisioners.WithGKEOperatorOptions(operatorOptions...),
		provisioners.WithGKEAutopilot(),
		provisioners.WithoutGKEFakeIntake(),
	}

	e2eOpts := []e2e.SuiteOption{
		e2e.WithStackName("operator-gke-autopilot"),
		e2e.WithProvisioner(provisioners.GKEProvisioner(provisionerOptions...)),
	}

	e2e.Run(t, &gkeAutopilotSuite{}, e2eOpts...)
}

func (s *gkeAutopilotSuite) TestAutopilotOperator() {
	s.Run("Verify Operator", func() {
		s.Assert().EventuallyWithT(func(c *assert.CollectT) {
			utils.VerifyOperator(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client())
		}, 10*time.Minute, 15*time.Second, "could not validate operator pod in time")
	})
}
