// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component"
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/composition"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
)

func newStandaloneDDAI() *v1alpha1.DatadogAgentInternal {
	return &v1alpha1.DatadogAgentInternal{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "dda-foo",
			Namespace:   "ns-1",
			Annotations: map[string]string{composition.ModeAnnotationKey: string(composition.ModeStandalone)},
		},
	}
}

func TestNodeAgentDaemonSetIdentityAcrossModes(t *testing.T) {
	ddai := newStandaloneDDAI()
	compositions := [][]apicommon.AgentContainerName{
		{apicommon.CoreAgentContainerName},
		{apicommon.CoreAgentContainerName, apicommon.OtelAgent},
		{apicommon.OtelAgent},
		{apicommon.HostProfiler, apicommon.OtelAgent},
	}

	var first *appsv1.DaemonSet
	for _, containers := range compositions {
		ds := componentagent.NewDefaultAgentDaemonset(ddai, feature.RequiredComponent{IsRequired: new(true), Containers: containers}, component.GetAgentName(ddai))
		if first == nil {
			first = ds
			continue
		}
		assert.Equal(t, first.Name, ds.Name, "containers %v", containers)
		assert.Equal(t, first.Spec.Selector, ds.Spec.Selector, "containers %v", containers)
	}
}

func TestReconcileV2AgentWithoutContainersDeletesDaemonSet(t *testing.T) {
	ddai := newStandaloneDDAI()
	existing := &appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: component.GetAgentName(ddai), Namespace: ddai.Namespace}}
	r := newRolloutTestReconciler(false, existing)
	status := &v1alpha1.DatadogAgentInternalStatus{Agent: &v2alpha1.DaemonSetStatus{}}

	_, err := r.reconcileV2Agent(context.Background(), feature.RequiredComponents{}, nil, ddai, nil, status, "")
	require.NoError(t, err)

	err = r.client.Get(context.Background(), types.NamespacedName{Name: existing.Name, Namespace: existing.Namespace}, &appsv1.DaemonSet{})
	assert.True(t, apierrors.IsNotFound(err), "node Agent DaemonSet should be deleted, got %v", err)
	assert.Nil(t, status.Agent)
}

func TestReportModeDroppedFeatures(t *testing.T) {
	r := &Reconciler{recorder: record.NewFakeRecorder(10)}
	ddai := newStandaloneDDAI()
	status := &v1alpha1.DatadogAgentInternalStatus{}
	now := metav1.Now()

	r.reportModeDroppedFeatures(logr.Discard(), ddai, nil, status, now)
	assert.Equal(t, metav1.ConditionStatus(""), conditionStatus(status, common.NodeAgentModeFeaturesDroppedConditionType), "no condition is written when nothing was ever dropped")

	r.reportModeDroppedFeatures(logr.Discard(), ddai, []feature.DroppedFeature{{ID: feature.APMIDType, Reasons: []string{`node Agent container "trace-agent"`}}}, status, now)
	assert.Equal(t, metav1.ConditionTrue, conditionStatus(status, common.NodeAgentModeFeaturesDroppedConditionType))

	r.reportModeDroppedFeatures(logr.Discard(), ddai, nil, status, now)
	assert.Equal(t, metav1.ConditionFalse, conditionStatus(status, common.NodeAgentModeFeaturesDroppedConditionType), "condition is cleared once nothing is dropped")
}
