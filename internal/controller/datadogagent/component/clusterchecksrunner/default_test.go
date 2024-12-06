// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusterchecksrunner

import (
	"testing"

	"github.com/DataDog/datadog-operator/api/crds/datadoghq/v2alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func Test_getDefaultServiceAccountName(t *testing.T) {
	dda := v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-datadog-agent",
			Namespace: "some-namespace",
		},
	}

	assert.Equal(t, "my-datadog-agent-cluster-checks-runner", getDefaultServiceAccountName(&dda))
}

func Test_getPodDisruptionBudget(t *testing.T) {
	dda := v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-datadog-agent",
			Namespace: "some-namespace",
		},
	}
	testpdb := GetClusterChecksRunnerPodDisruptionBudget(&dda)
	assert.Equal(t, "my-datadog-agent-cluster-checks-runner-pdb", testpdb.Name)
	assert.Equal(t, intstr.FromInt(pdbMaxUnavailableInstances), *testpdb.Spec.MaxUnavailable)
	assert.Nil(t, testpdb.Spec.MinAvailable)
}
