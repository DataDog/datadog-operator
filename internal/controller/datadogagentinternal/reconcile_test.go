// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/defaults"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func TestUpdateStatusIfNeededEvaluatedConditions(t *testing.T) {
	for _, tt := range []struct {
		name      string
		evaluated bool
		messages  []string
		err       error
		wantClear bool
	}{
		{name: "early return without error preserves condition"},
		{name: "early error preserves condition", err: errors.New("dependency error")},
		{name: "completed checks clear condition", evaluated: true, wantClear: true},
		{name: "cleanup error still clears resolved condition", evaluated: true, err: errors.New("cleanup error"), wantClear: true},
		{name: "reported conflict survives cleanup error", evaluated: true, messages: []string{"current conflict"}, err: errors.New("cleanup error")},
		{name: "partially evaluated conflict is saved", messages: []string{"current conflict"}, err: errors.New("component error")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			sch := runtime.NewScheme()
			require.NoError(t, v1alpha1.AddToScheme(sch))
			previous := metav1.NewTime(time.Unix(100, 0))
			now := metav1.NewTime(time.Unix(200, 0))
			ddai := &v1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"}}
			condition.UpdateDatadogAgentInternalStatusConditions(&ddai.Status, previous, common.OverrideReconcileConflictConditionType, metav1.ConditionTrue, "OverrideConflict", "previous conflict", true)
			condition.UpdateDatadogAgentInternalStatusConditions(&ddai.Status, previous, "Unrelated", metav1.ConditionTrue, "Ready", "unchanged", true)
			c := fake.NewClientBuilder().WithScheme(sch).WithStatusSubresource(ddai).WithObjects(ddai).Build()
			require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(ddai), ddai))
			r := &Reconciler{client: c}
			newStatus := ddai.Status.DeepCopy()
			messages := make(map[string][]string)
			for _, message := range tt.messages {
				condition.AppendDatadogAgentInternalStatusConditions(newStatus, messages, now, common.OverrideReconcileConflictConditionType, metav1.ConditionTrue, "OverrideConflict", message, true)
			}
			evaluated := map[string]bool{common.OverrideReconcileConflictConditionType: tt.evaluated}
			result := reconcile.Result{RequeueAfter: time.Minute}
			got, err := r.updateStatusIfNeeded(ctx, ddai, newStatus, result, tt.err, now, messages, evaluated)
			require.Equal(t, tt.err, err)
			require.Equal(t, result, got)
			require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(ddai), ddai))
			require.NotNil(t, condition.GetDDAICondition(&ddai.Status, "Unrelated"))
			conflict := condition.GetDDAICondition(&ddai.Status, common.OverrideReconcileConflictConditionType)
			if tt.wantClear {
				require.Nil(t, conflict)
			} else {
				require.NotNil(t, conflict)
				wantMessage := "previous conflict"
				if len(tt.messages) > 0 {
					wantMessage = "current conflict"
				}
				require.Equal(t, wantMessage, conflict.Message)
				require.Equal(t, previous, conflict.LastTransitionTime)
			}
		})
	}
}

func TestReconcileInstanceWindowsOverrideConflict(t *testing.T) {
	for _, tt := range []struct {
		name       string
		global     v2alpha1.GlobalConfig
		disabled   bool
		fips       bool
		noPrevious bool
	}{
		{name: "FIPS Agent preserves conflict", disabled: true, fips: true, global: v2alpha1.GlobalConfig{UseFIPSAgent: ptr.To(true)}},
		{name: "FIPS proxy preserves conflict", disabled: true, fips: true, global: v2alpha1.GlobalConfig{FIPS: &v2alpha1.FIPSConfig{Enabled: ptr.To(true)}}},
		{name: "FIPS Agent records new conflict", disabled: true, fips: true, noPrevious: true, global: v2alpha1.GlobalConfig{UseFIPSAgent: ptr.To(true)}},
		{name: "FIPS proxy records new conflict", disabled: true, fips: true, noPrevious: true, global: v2alpha1.GlobalConfig{FIPS: &v2alpha1.FIPSConfig{Enabled: ptr.To(true)}}},
		{name: "FIPS Agent clears resolved conflict", fips: true, global: v2alpha1.GlobalConfig{UseFIPSAgent: ptr.To(true)}},
		{name: "FIPS proxy clears resolved conflict", fips: true, global: v2alpha1.GlobalConfig{FIPS: &v2alpha1.FIPSConfig{Enabled: ptr.To(true)}}},
		{name: "completed check preserves active conflict", disabled: true},
		{name: "completed check clears resolved conflict"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			ddai := newRolloutTestDDAI()
			ddai.Labels = map[string]string{constants.ProfileLabelKey: "windows"}
			ddai.Annotations = map[string]string{kubernetes.ProviderAnnotationKey: kubernetes.WindowsProvider}
			ddai.Spec.Global = tt.global.DeepCopy()
			ddai.Spec.Global.Credentials = &v2alpha1.DatadogCredentials{
				APISecret: &v2alpha1.SecretConfig{SecretName: "datadog-secret", KeyName: "api-key"},
			}
			ddai.Spec.Override = map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
				v2alpha1.NodeAgentComponentName: {Disabled: ptr.To(tt.disabled)},
			}
			defaults.DefaultDatadogAgentSpec(&ddai.Spec)
			previous := metav1.NewTime(time.Unix(100, 0))
			condition.UpdateDatadogAgentInternalStatusConditions(&ddai.Status, previous, common.OverrideReconcileConflictConditionType, metav1.ConditionTrue, "OverrideConflict", "Agent component is set to disabled", true)
			wantConflict := *condition.GetDDAICondition(&ddai.Status, common.OverrideReconcileConflictConditionType)
			if tt.noPrevious {
				condition.DeleteDatadogAgentInternalStatusCondition(&ddai.Status, common.OverrideReconcileConflictConditionType)
			}
			r := newRolloutTestReconciler(false)
			r.client = fake.NewClientBuilder().WithScheme(r.scheme).WithStatusSubresource(ddai).WithObjects(ddai).Build()
			require.NoError(t, r.client.Get(ctx, client.ObjectKeyFromObject(ddai), ddai))

			_, err := r.reconcileInstance(ctx, ddai)

			require.NoError(t, err)
			require.NoError(t, r.client.Get(ctx, client.ObjectKeyFromObject(ddai), ddai))
			windowsCondition := condition.GetDDAICondition(&ddai.Status, "WindowsAgentReconcile")
			require.NotNil(t, windowsCondition)
			if tt.fips {
				require.Equal(t, "WindowsFIPSUnsupported", windowsCondition.Reason)
			}
			conflict := condition.GetDDAICondition(&ddai.Status, common.OverrideReconcileConflictConditionType)
			if tt.disabled {
				require.NotNil(t, conflict)
				require.Equal(t, wantConflict.Message, conflict.Message)
				require.Equal(t, wantConflict.Status, conflict.Status)
				require.Equal(t, wantConflict.Reason, conflict.Reason)
				if !tt.noPrevious {
					require.Equal(t, wantConflict.LastTransitionTime, conflict.LastTransitionTime)
				}
			} else {
				require.Nil(t, conflict)
			}
		})
	}
}
