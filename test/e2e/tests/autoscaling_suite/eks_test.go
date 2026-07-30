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
func TestEKSAutoscalingSuite(t *testing.T) {
	provisionerOptions := []provisioners.EKSProvisionerOption{
		provisioners.WithEKSName("autoscaling-e2e"),
		provisioners.WithEKSK8sVersion("1.34"),
		provisioners.WithEKSLinuxNodeGroup(),
		provisioners.WithEKSLinuxARMNodeGroup(),
	}

	e2eOpts := []e2e.SuiteOption{
		// Keep the stack name short: the cluster name (derived from the stack
		// name) is interpolated into several IAM/SQS resource names in the
		// Karpenter CloudFormation templates. The managed-policy size limit
		// (CASCL-1645) now supports cluster names up to EKS's own 100-char
		// maximum, but the KarpenterNodeRole-${ClusterName} IAM role name
		// (64-char limit) still caps supported names at ~46 chars — tracked
		// in CASCL-1646.
		e2e.WithStackName("eks-as"),
		e2e.WithProvisioner(provisioners.EKSProvisioner(provisionerOptions...)),
	}

	e2e.Run(t, &autoscalingSuite{}, e2eOpts...)
}
