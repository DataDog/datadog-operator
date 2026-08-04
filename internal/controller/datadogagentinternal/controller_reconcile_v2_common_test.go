package datadogagentinternal

import (
	"context"
	"fmt"
	"testing"

	assert "github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/condition"
)

func Test_ensureSelectorInPodTemplateLabels(t *testing.T) {

	tests := []struct {
		name              string
		selector          *metav1.LabelSelector
		podTemplateLabels map[string]string
		expectedLabels    map[string]string
	}{
		{
			name:     "Nil selector",
			selector: nil,
			podTemplateLabels: map[string]string{
				"foo": "bar",
			},
			expectedLabels: map[string]string{
				"foo": "bar",
			},
		},
		{
			name:     "Empty selector",
			selector: &metav1.LabelSelector{},
			podTemplateLabels: map[string]string{
				"foo": "bar",
			},
			expectedLabels: map[string]string{
				"foo": "bar",
			},
		},
		{
			name: "Selector in template labels",
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "bar",
				},
			},
			podTemplateLabels: map[string]string{
				"foo": "bar",
			},
			expectedLabels: map[string]string{
				"foo": "bar",
			},
		},
		{
			name: "Selector not in template labels",
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"bar": "foo",
				},
			},
			podTemplateLabels: map[string]string{
				"foo": "bar",
			},
			expectedLabels: map[string]string{
				"foo": "bar",
				"bar": "foo",
			},
		},
		{
			name: "Selector label value does not match template labels",
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "foo",
				},
			},
			podTemplateLabels: map[string]string{
				"foo": "bar",
			},
			expectedLabels: map[string]string{
				"foo": "foo",
			},
		},
		{
			name: "Nil pod template labels",
			selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"foo": "foo",
				},
			},
			podTemplateLabels: nil,
			expectedLabels: map[string]string{
				"foo": "foo",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			labels := ensureSelectorInPodTemplateLabels(context.Background(), tt.selector, tt.podTemplateLabels)
			assert.Equal(t, tt.expectedLabels, labels)
		})
	}
}

func Test_updateStatusIfNeededV2_ReconcileErrorCondition(t *testing.T) {
	sch := runtime.NewScheme()
	_ = scheme.AddToScheme(sch)
	_ = v1alpha1.AddToScheme(sch)

	now := metav1.NewTime(metav1.Now().Time)

	ddai := &v1alpha1.DatadogAgentInternal{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ddai",
			Namespace: "test-namespace",
		},
	}

	tests := []struct {
		name              string
		existingCondition *metav1.Condition
		currentError      error
		expectNoCondition bool
		expectStatus      metav1.ConditionStatus
		expectReason      string
		expectMessage     string
	}{
		{
			name:              "no error and no prior condition adds nothing",
			currentError:      nil,
			expectNoCondition: true,
		},
		{
			name: "no error clears a prior error condition",
			existingCondition: &metav1.Condition{
				Type:    common.DatadogAgentReconcileErrorConditionType,
				Status:  metav1.ConditionTrue,
				Reason:  "DatadogAgent_reconcile_error",
				Message: "some prior error",
			},
			currentError:  nil,
			expectStatus:  metav1.ConditionFalse,
			expectReason:  "DatadogAgent_reconcile_ok",
			expectMessage: "DatadogAgent reconcile ok",
		},
		{
			name:          "error surfaces its real message",
			currentError:  fmt.Errorf("rbac error: clusterroles.rbac.authorization.k8s.io is forbidden"),
			expectStatus:  metav1.ConditionTrue,
			expectReason:  "DatadogAgent_reconcile_error",
			expectMessage: "rbac error: clusterroles.rbac.authorization.k8s.io is forbidden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(sch).WithStatusSubresource(&v1alpha1.DatadogAgentInternal{}).WithObjects([]client.Object{ddai.DeepCopy()}...).Build()
			r := &Reconciler{client: fakeClient}

			currentDDAI := &v1alpha1.DatadogAgentInternal{}
			assert.NoError(t, fakeClient.Get(context.Background(), client.ObjectKeyFromObject(ddai), currentDDAI))

			newStatus := &v1alpha1.DatadogAgentInternalStatus{}
			if tt.existingCondition != nil {
				newStatus.Conditions = append(newStatus.Conditions, *tt.existingCondition)
			}

			_, err := r.updateStatusIfNeededV2(context.Background(), currentDDAI, newStatus, reconcile.Result{}, tt.currentError, now)
			assert.Equal(t, tt.currentError, err)

			cond := condition.GetDDAICondition(newStatus, common.DatadogAgentReconcileErrorConditionType)
			if tt.expectNoCondition {
				assert.Nil(t, cond)
				return
			}
			assert.NotNil(t, cond)
			assert.Equal(t, tt.expectStatus, cond.Status)
			assert.Equal(t, tt.expectReason, cond.Reason)
			assert.Equal(t, tt.expectMessage, cond.Message)
		})
	}
}
