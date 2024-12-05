// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build e2e
// +build e2e

package e2e

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-operator/test/new-e2e/common"
	"github.com/DataDog/datadog-operator/test/new-e2e/provisioners"
	"github.com/DataDog/test-infra-definitions/components/datadog/operatorparams"
	"testing"
)

type localKindSuite struct {
	k8sSuite
}

func TestLocalKindSuite(t *testing.T) {
	operatorOptions := []operatorparams.Option{
		operatorparams.WithNamespace(common.NamespaceName),
		operatorparams.WithOperatorFullImagePath(common.OperatorImageName),
		operatorparams.WithHelmValues("installCRDs: false"),
	}

	provisionerOptions := []provisioners.KubernetesProvisionerOption{
		provisioners.WithK8sVersion(common.K8sVersion),
		provisioners.WithOperatorOptions(operatorOptions...),
		provisioners.WithoutDDA(),
	}

	t.Parallel()

	e2e.Run(t, &localKindSuite{}, e2e.WithProvisioner(provisioners.KubernetesProvisioner(provisioners.LocalKindRunFunc, provisionerOptions...)), e2e.WithSkipDeleteOnFailure(), e2e.WithDevMode())
}
