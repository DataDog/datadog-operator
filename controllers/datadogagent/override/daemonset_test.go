// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"testing"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestDaemonSet(t *testing.T) {
	daemonSet := v1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: "current-name",
		},
		Spec: v1.DaemonSetSpec{
			UpdateStrategy: v1.DaemonSetUpdateStrategy{
				RollingUpdate: &v1.RollingUpdateDaemonSet{
					MaxUnavailable: &intstr.IntOrString{
						StrVal: "25%",
					},
					MaxSurge: &intstr.IntOrString{
						StrVal: "25%",
					},
				},
			},
		},
	}

	override := v2alpha1.DatadogAgentComponentOverride{
		Name: apiutils.NewStringPointer("new-name"),
		Strategy: &v2alpha1.UpdateStrategy{
			Type: "RollingUpdate",
			RollingUpdate: &v2alpha1.RollingUpdate{
				MaxUnavailable: &intstr.IntOrString{
					StrVal: "50%",
				},
				MaxSurge: &intstr.IntOrString{
					StrVal: "50%",
				},
			},
		},
	}

	DaemonSet(&daemonSet, &override)

	assert.Equal(t, "new-name", daemonSet.Name)
	assert.Equal(t, v1.RollingUpdateDaemonSetStrategyType, daemonSet.Spec.UpdateStrategy.Type)
	assert.Equal(t, "50%", daemonSet.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable.StrVal)
	assert.Equal(t, "50%", daemonSet.Spec.UpdateStrategy.RollingUpdate.MaxSurge.StrVal)

}
