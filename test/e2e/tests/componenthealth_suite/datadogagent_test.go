// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package componenthealthsuite

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/test/e2e/common"
)

func TestDatadogAgentFaultLifecycle(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, v2alpha1.AddToScheme(scheme))
	s := &componentHealthSuite{ddaClient: fake.NewClientBuilder().WithScheme(scheme).Build()}
	s.SetT(t)
	key := types.NamespacedName{Name: ddaName, Namespace: common.NamespaceName}

	s.applyDatadogAgent(clusterAgentImagePullFault())
	dda := &v2alpha1.DatadogAgent{}
	require.NoError(t, s.ddaClient.Get(t.Context(), key, dda))
	require.Equal(t, imagePullBadTag, dda.Spec.Override[v2alpha1.ClusterAgentComponentName].Image.Tag)

	// Simulate fields populated outside the test's fault injection.
	dda.Annotations = map[string]string{"other-owner": "preserve"}
	dda.Spec.Global.ClusterName = ptr.To("defaulted-cluster-name")
	dda.Spec.Override[v2alpha1.NodeAgentComponentName] = &v2alpha1.DatadogAgentComponentOverride{Replicas: ptr.To(int32(2))}
	require.NoError(t, s.ddaClient.Update(t.Context(), dda))

	s.applyDatadogAgent(clusterAgentUnschedulableFault())
	dda = &v2alpha1.DatadogAgent{}
	require.NoError(t, s.ddaClient.Get(t.Context(), key, dda))
	require.Nil(t, dda.Spec.Override[v2alpha1.ClusterAgentComponentName].Image, "switching faults must remove the old image override")
	require.Equal(t, clusterAgentUnschedulableFault().NodeSelector, dda.Spec.Override[v2alpha1.ClusterAgentComponentName].NodeSelector)

	s.applyDatadogAgent(nil)
	dda = &v2alpha1.DatadogAgent{}
	require.NoError(t, s.ddaClient.Get(t.Context(), key, dda))
	require.NotContains(t, dda.Spec.Override, v2alpha1.ClusterAgentComponentName, "remediation must remove the fault")
	require.Contains(t, dda.Spec.Override, v2alpha1.NodeAgentComponentName)
	require.Equal(t, "defaulted-cluster-name", *dda.Spec.Global.ClusterName)
	require.Equal(t, "preserve", dda.Annotations["other-owner"])
}
