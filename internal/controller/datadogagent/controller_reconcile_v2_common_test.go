package datadogagent

import (
	"testing"
	"time"

	"k8s.io/utils/ptr"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/constants"

	assert "github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func Test_addDDAIStatusToDDAStatus(t *testing.T) {
	sch := runtime.NewScheme()
	_ = scheme.AddToScheme(sch)
	_ = v1alpha1.AddToScheme(sch)
	_ = v2alpha1.AddToScheme(sch)

	now := metav1.NewTime(time.Now())

	tests := []struct {
		name           string
		status         v2alpha1.DatadogAgentStatus
		existingDDAI   v1alpha1.DatadogAgentInternal
		expectedStatus v2alpha1.DatadogAgentStatus
	}{
		{
			name: "No DDAI exists",
			status: v2alpha1.DatadogAgentStatus{
				Agent: &v2alpha1.DaemonSetStatus{
					Desired: int32(1),
				},
			},
			existingDDAI: v1alpha1.DatadogAgentInternal{},
			expectedStatus: v2alpha1.DatadogAgentStatus{
				Agent: &v2alpha1.DaemonSetStatus{
					Desired: int32(1),
				},
			},
		},
		{
			name: "Copy status from DDAI to DDA (except for remote config)",
			status: v2alpha1.DatadogAgentStatus{
				Agent: &v2alpha1.DaemonSetStatus{
					Desired: int32(1),
				},
			},
			existingDDAI: v1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ddai",
					Namespace: "test-namespace",
				},
				Status: v1alpha1.DatadogAgentInternalStatus{
					Agent: &v2alpha1.DaemonSetStatus{
						Desired: int32(3),
						Ready:   int32(2),
					},
					ClusterAgent: &v2alpha1.DeploymentStatus{
						Replicas:      int32(1),
						ReadyReplicas: int32(1),
					},
					ClusterChecksRunner: &v2alpha1.DeploymentStatus{
						CurrentHash: "foo",
					},
					RemoteConfigConfiguration: &v2alpha1.RemoteConfigConfiguration{
						Features: &v2alpha1.DatadogFeatures{
							CWS: &v2alpha1.CWSFeatureConfig{
								Enabled: ptr.To(true),
							},
						},
					},
				},
			},
			expectedStatus: v2alpha1.DatadogAgentStatus{
				Agent: &v2alpha1.DaemonSetStatus{
					Desired: int32(4),
					Ready:   int32(2),
					Status:  " (4/2/0)",
				},
				ClusterAgent: &v2alpha1.DeploymentStatus{
					Replicas:      int32(1),
					ReadyReplicas: int32(1),
				},
				ClusterChecksRunner: &v2alpha1.DeploymentStatus{
					CurrentHash: "foo",
				},
			},
		},
		{
			name:   "Default DDAI reconcile error is surfaced on the DDA",
			status: v2alpha1.DatadogAgentStatus{},
			existingDDAI: v1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-ddai",
					Namespace: "test-namespace",
				},
				Status: v1alpha1.DatadogAgentInternalStatus{
					Conditions: []metav1.Condition{
						{
							Type:    common.DatadogAgentReconcileErrorConditionType,
							Status:  metav1.ConditionTrue,
							Reason:  "DatadogAgent_reconcile_error",
							Message: "rbac error: clusterroles.rbac.authorization.k8s.io is forbidden",
						},
					},
				},
			},
			expectedStatus: v2alpha1.DatadogAgentStatus{
				Conditions: []metav1.Condition{
					{
						Type:               common.DatadogAgentInternalReconcileErrorConditionType,
						Status:             metav1.ConditionTrue,
						LastTransitionTime: now,
						Reason:             "DatadogAgentInternal_reconcile_error",
						Message:            "test-ddai: rbac error: clusterroles.rbac.authorization.k8s.io is forbidden",
					},
				},
			},
		},
		{
			name:   "Profile DDAI reconcile error is NOT surfaced on the DDA",
			status: v2alpha1.DatadogAgentStatus{},
			existingDDAI: v1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-profile-ddai",
					Namespace: "test-namespace",
					Labels: map[string]string{
						constants.ProfileLabelKey: "test-profile",
					},
				},
				Status: v1alpha1.DatadogAgentInternalStatus{
					Conditions: []metav1.Condition{
						{
							Type:    common.DatadogAgentReconcileErrorConditionType,
							Status:  metav1.ConditionTrue,
							Reason:  "DatadogAgent_reconcile_error",
							Message: "some daemonset error",
						},
					},
				},
			},
			expectedStatus: v2alpha1.DatadogAgentStatus{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects([]client.Object{&tt.existingDDAI}...).Build()

			r := &Reconciler{
				client: fakeClient,
				log:    logf.Log.WithName(tt.name),
			}

			err := r.addDDAIStatusToDDAStatus(&tt.status, tt.existingDDAI.ObjectMeta, now)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, tt.status)
		})
	}
}
