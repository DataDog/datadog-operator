// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusterchecksrunner

import (
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/stretchr/testify/assert"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func Test_getDefaultServiceAccountName(t *testing.T) {
	ddai := v1alpha1.DatadogAgentInternal{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-datadog-agent",
			Namespace: "some-namespace",
		},
	}

	assert.Equal(t, "my-datadog-agent-cluster-checks-runner", getDefaultServiceAccountName(&ddai))
}

func Test_getPodDisruptionBudget(t *testing.T) {
	ddai := v1alpha1.DatadogAgentInternal{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-datadog-agent",
			Namespace: "some-namespace",
		},
	}
	testpdb := GetClusterChecksRunnerPodDisruptionBudget(&ddai, false).(*policyv1.PodDisruptionBudget)
	assert.Equal(t, "my-datadog-agent-cluster-checks-runner-pdb", testpdb.Name)
	assert.Equal(t, intstr.FromInt(pdbMaxUnavailableInstances), *testpdb.Spec.MaxUnavailable)
	assert.Nil(t, testpdb.Spec.MinAvailable)
}
