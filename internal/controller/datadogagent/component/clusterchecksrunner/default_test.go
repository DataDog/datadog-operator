// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusterchecksrunner

import (
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/stretchr/testify/assert"
	policyv1 "k8s.io/api/policy/v1"
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

func Test_defaultEnvVars_hasCCRNodeName(t *testing.T) {
	dda := v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-datadog-agent",
			Namespace: "some-namespace",
		},
	}

	envVars := defaultEnvVars(&dda)

	// DD_CCR_NODE_NAME should be set from spec.nodeName via downward API
	var foundCCRNodeName bool
	var foundDDHostname bool
	for _, env := range envVars {
		if env.Name == DDCCRNodeName {
			foundCCRNodeName = true
			assert.NotNil(t, env.ValueFrom)
			assert.NotNil(t, env.ValueFrom.FieldRef)
			assert.Equal(t, common.FieldPathSpecNodeName, env.ValueFrom.FieldRef.FieldPath)
		}
		if env.Name == constants.DDHostName {
			foundDDHostname = true
		}
	}
	assert.True(t, foundCCRNodeName, "DD_CCR_NODE_NAME should be present in default env vars")
	assert.False(t, foundDDHostname, "DD_HOSTNAME should not be directly set in default env vars")
}

func Test_getPodDisruptionBudget(t *testing.T) {
	dda := v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-datadog-agent",
			Namespace: "some-namespace",
		},
	}
	testpdb := GetClusterChecksRunnerPodDisruptionBudget(&dda, false).(*policyv1.PodDisruptionBudget)
	assert.Equal(t, "my-datadog-agent-cluster-checks-runner-pdb", testpdb.Name)
	assert.Equal(t, intstr.FromInt(pdbMaxUnavailableInstances), *testpdb.Spec.MaxUnavailable)
	assert.Nil(t, testpdb.Spec.MinAvailable)
}
