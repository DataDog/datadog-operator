// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build e2e
// +build e2e

package e2e

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-operator/test/e2e/common"
	"github.com/DataDog/datadog-operator/test/e2e/provisioners"
)

func TestRcLocalKindSuite(t *testing.T) {

	provisionerOpts := []provisioners.KubernetesProvisionerOption{
		provisioners.WithK8sVersion(common.K8sVersion),
		provisioners.WithoutOperator(),
		provisioners.WithoutFakeIntake(),
		provisioners.WithoutDDA(),
	}

	e2eParams := []e2e.SuiteOption{
		e2e.WithSkipDeleteOnFailure(),
		// Un-comment the following line to run the test in dev mode (keep stack after test)
		// e2e.WithDevMode(),
		e2e.WithProvisioner(provisioners.KubernetesProvisioner(provisioners.LocalKindRunFunc, provisionerOpts...)),
	}

	t.Parallel()

	e2e.Run(t, &updaterSuite{}, e2eParams...)
}
