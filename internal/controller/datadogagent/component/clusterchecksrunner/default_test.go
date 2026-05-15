// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusterchecksrunner

import (
	"testing"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	policyv1 "k8s.io/api/policy/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
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
	testpdb := GetClusterChecksRunnerPodDisruptionBudget(&dda, false).(*policyv1.PodDisruptionBudget)
	assert.Equal(t, "my-datadog-agent-cluster-checks-runner-pdb", testpdb.Name)
	assert.Equal(t, intstr.FromInt(pdbMaxUnavailableInstances), *testpdb.Spec.MaxUnavailable)
	assert.Nil(t, testpdb.Spec.MinAvailable)
}

func Test_getPodDisruptionBudget_v1beta1(t *testing.T) {
	dda := v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-datadog-agent",
			Namespace: "some-namespace",
		},
	}

	testpdb := GetClusterChecksRunnerPodDisruptionBudget(&dda, true).(*policyv1beta1.PodDisruptionBudget)

	require.Equal(t, "my-datadog-agent-cluster-checks-runner-pdb", testpdb.Name)
	require.Equal(t, "some-namespace", testpdb.Namespace)
	require.Equal(t, intstr.FromInt(pdbMaxUnavailableInstances), *testpdb.Spec.MaxUnavailable)
	require.Equal(t, map[string]string{
		apicommon.AgentDeploymentNameLabelKey:      "my-datadog-agent",
		apicommon.AgentDeploymentComponentLabelKey: constants.DefaultClusterChecksRunnerResourceSuffix,
	}, testpdb.Spec.Selector.MatchLabels)
}

func TestClusterChecksRunnerDefaultDeployment(t *testing.T) {
	dda := v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-datadog-agent",
			Namespace: "some-namespace",
		},
	}

	deployment := NewDefaultClusterChecksRunnerDeployment(&dda, &dda.Spec)

	require.Equal(t, "my-datadog-agent-cluster-checks-runner", deployment.Name)
	require.Equal(t, "some-namespace", deployment.Namespace)
	require.NotNil(t, deployment.Spec.Replicas)
	require.Equal(t, int32(defaultClusterChecksRunnerReplicas), *deployment.Spec.Replicas)
	require.Equal(t, deployment.Labels, deployment.Spec.Template.Labels)
	require.Equal(t, deployment.Annotations, deployment.Spec.Template.Annotations)
	require.Equal(t, getDefaultServiceAccountName(&dda), deployment.Spec.Template.Spec.ServiceAccountName)
	require.Len(t, deployment.Spec.Template.Spec.Containers, 1)
	require.Equal(t, string(apicommon.ClusterChecksRunnersContainerName), deployment.Spec.Template.Spec.Containers[0].Name)
}
