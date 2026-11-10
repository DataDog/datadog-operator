// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/stretchr/testify/assert"
)

func Test_ManagedInstallNoScheduleToleration(t *testing.T) {
	toleration := ManagedInstallNoScheduleToleration()

	// Keyless Exists: tolerates every NoSchedule taint, whatever its key —
	// customers choose taint keys, so the default must not match a fixed one.
	assert.Empty(t, toleration.Key)
	assert.Equal(t, corev1.TolerationOpExists, toleration.Operator)
	assert.Equal(t, corev1.TaintEffectNoSchedule, toleration.Effect)
	assert.Nil(t, toleration.TolerationSeconds)

	// A NoExecute taint must NOT be tolerated, so operational evictions
	// (e.g. CoreWeave SUNK) still drain the Agent.
	noExecuteTaint := corev1.Taint{Key: "sunk.coreweave.com/lock", Value: "true", Effect: corev1.TaintEffectNoExecute}
	assert.False(t, toleration.ToleratesTaint(klog.TODO(), &noExecuteTaint, false))

	// Any NoSchedule taint must be tolerated.
	noScheduleTaint := corev1.Taint{Key: "nvidia.com/gpu", Value: "true", Effect: corev1.TaintEffectNoSchedule}
	assert.True(t, toleration.ToleratesTaint(klog.TODO(), &noScheduleTaint, false))
}

func Test_IsFleetManagedDatadogAgent(t *testing.T) {
	assert.False(t, IsFleetManagedDatadogAgent(nil))

	dda := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Name: "foo"}}
	assert.False(t, IsFleetManagedDatadogAgent(dda))

	dda.Labels = map[string]string{fleetManagedByLabel: "something-else"}
	assert.False(t, IsFleetManagedDatadogAgent(dda))

	dda.Labels = map[string]string{fleetManagedByLabel: fleetManagedByValue}
	assert.True(t, IsFleetManagedDatadogAgent(dda))
}
