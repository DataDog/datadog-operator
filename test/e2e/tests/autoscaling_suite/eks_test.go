// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autoscalingsuite

import (
	"fmt"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"
	clustercommon "github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/cluster/common"
	"github.com/DataDog/datadog-operator/test/e2e/provisioners"
)

// longClusterStackName returns a stack name such that the EKS cluster name the
// e2e framework derives from it is exactly [clustercommon.MaxClusterNameLength] characters long.
func longClusterStackName() string {
	// The cluster name is the Pulumi stack name, <profile prefix>-<stack name>,
	// and the profile prefix depends on the environment (local username or CI
	// job), so the stack name is padded accordingly. extractClusterInfo asserts
	// the result.
	const stackNamePrefix = "eks-autoscaling"

	prefix := runner.GetProfile().NamePrefix()
	padding := clustercommon.MaxClusterNameLength - len(prefix) - len("-") - len(stackNamePrefix)
	if padding < 0 {
		panic(fmt.Sprintf("profile name prefix %q is too long to build a %d-char cluster name", prefix, clustercommon.MaxClusterNameLength))
	}
	return stackNamePrefix + strings.Repeat("x", padding)
}

// TestEKSAutoscalingSuite runs the autoscaling E2E tests on an EKS cluster.
func TestEKSAutoscalingSuite(t *testing.T) {
	provisionerOptions := []provisioners.EKSProvisionerOption{
		provisioners.WithEKSName("autoscaling-e2e"),
		provisioners.WithEKSK8sVersion("1.34"),
		provisioners.WithEKSLinuxNodeGroup(),
		provisioners.WithEKSLinuxARMNodeGroup(),
	}

	e2eOpts := []e2e.SuiteOption{
		// The cluster name (derived from the stack name) is interpolated into
		// IAM/SQS resource names in the Karpenter CloudFormation templates.
		// Use the longest supported name to exercise the worst case.
		e2e.WithStackName(longClusterStackName()),
		e2e.WithProvisioner(provisioners.EKSProvisioner(provisionerOptions...)),
	}

	e2e.Run(t, &autoscalingSuite{}, e2eOpts...)
}
