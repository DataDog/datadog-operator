// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build e2e
// +build e2e

package e2e

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	awskubernetes "github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments/aws/kubernetes"
	"github.com/DataDog/datadog-operator/test/e2e/common"
	"github.com/DataDog/test-infra-definitions/components/datadog/operatorparams"
	"testing"
)

// not working yet
func TestAWSKindSuite(t *testing.T) {
	operatorOptions := []operatorparams.Option{
		operatorparams.WithNamespace(common.NamespaceName),
		operatorparams.WithOperatorFullImagePath(common.OperatorImageName),
		operatorparams.WithHelmValues("installCRDs: false"),
	}

	e2e.Run(t, &k8sSuite{}, e2e.WithProvisioner(awskubernetes.KindProvisioner(
		awskubernetes.WithOperatorOptions(operatorOptions...),
	)), e2e.WithSkipDeleteOnFailure(), e2e.WithDevMode())
}
