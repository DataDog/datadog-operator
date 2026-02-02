// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autoscalingsuite

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-operator/test/e2e/provisioners"
)

// TestEKSAutoscalingSuite runs the autoscaling E2E tests on an EKS cluster.
// The EKS cluster is provisioned once and shared across all tests in the suite.
func TestEKSAutoscalingSuite(t *testing.T) {
	provisionerOptions := []provisioners.EKSProvisionerOption{
		provisioners.WithEKSName("autoscaling-e2e"),
		provisioners.WithEKSK8sVersion("1.32"),
		provisioners.WithEKSLinuxNodeGroup(),
	}

	e2eOpts := []e2e.SuiteOption{
		// Keep the stack name short to avoid exceeding IAM policy size limits.
		// The cluster name (derived from stack name) appears 20+ times in the
		// KarpenterControllerPolicy, which has a 6144 char limit.
		e2e.WithStackName("eks-as"),
		e2e.WithProvisioner(provisioners.EKSProvisioner(provisionerOptions...)),
	}

	e2e.Run(t, &autoscalingSuite{}, e2eOpts...)
}
