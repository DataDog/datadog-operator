package datadogagent

import (
	"errors"
	"testing"
	"time"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"

	assert "github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func Test_shouldUpdateProfileDaemonSet(t *testing.T) {
	sch := runtime.NewScheme()
	_ = scheme.AddToScheme(sch)
	_ = edsdatadoghqv1alpha1.AddToScheme(sch)

	testNS := "test"
	now := metav1.Now()
	now5MinBefore := metav1.NewTime(now.Add(-5 * time.Minute))
	now15MinBefore := metav1.NewTime(now.Add(-15 * time.Minute))
	testProfile := v1alpha1.DatadogAgentProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-profile",
			Namespace: testNS,
		},
	}
	testEDSLabels := map[string]string{
		apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
		kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
	}
	testERSLabels := map[string]string{
		edsdatadoghqv1alpha1.ExtendedDaemonSetNameLabelKey: "test-eds",
	}

	tests := []struct {
		name                 string
		reconcilerOptions    ReconcilerOptions
		profile              *v1alpha1.DatadogAgentProfile
		ddaLastUpdateTime    metav1.Time
		now                  metav1.Time
		existingObjects      []client.Object
		expectedShouldUpdate bool
		errorMessage         error
	}{
		{
			name: "EDS not enabled",
			reconcilerOptions: ReconcilerOptions{
				ExtendedDaemonsetOptions: agent.ExtendedDaemonsetOptions{
					Enabled: false,
				},
			},
			profile:              &testProfile,
			ddaLastUpdateTime:    now,
			now:                  now,
			existingObjects:      nil,
			expectedShouldUpdate: true,
			errorMessage:         nil,
		},
		{
			name: "Profiles not enabled",
			reconcilerOptions: ReconcilerOptions{
				ExtendedDaemonsetOptions: agent.ExtendedDaemonsetOptions{
					Enabled: true,
				},
				DatadogAgentProfileEnabled: false,
			},
			profile:              &testProfile,
			ddaLastUpdateTime:    now,
			now:                  now,
			existingObjects:      nil,
			expectedShouldUpdate: true,
			errorMessage:         nil,
		},
		{
			name: "Profiles nil",
			reconcilerOptions: ReconcilerOptions{
				ExtendedDaemonsetOptions: agent.ExtendedDaemonsetOptions{
					Enabled: true,
				},
				DatadogAgentProfileEnabled: true,
			},
			profile:              nil,
			ddaLastUpdateTime:    now,
			now:                  now,
			existingObjects:      nil,
			expectedShouldUpdate: true,
			errorMessage:         nil,
		},
		{
			name: "Default profile",
			reconcilerOptions: ReconcilerOptions{
				ExtendedDaemonsetOptions: agent.ExtendedDaemonsetOptions{
					Enabled: true,
				},
				DatadogAgentProfileEnabled: true,
			},
			profile: &v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default",
					Namespace: "",
				},
			},
			ddaLastUpdateTime:    now,
			now:                  now,
			existingObjects:      nil,
			expectedShouldUpdate: true,
			errorMessage:         nil,
		},
		{
			name: "No EDSs exist",
			reconcilerOptions: ReconcilerOptions{
				ExtendedDaemonsetOptions: agent.ExtendedDaemonsetOptions{
					Enabled: true,
				},
				DatadogAgentProfileEnabled: true,
			},
			profile:              &testProfile,
			ddaLastUpdateTime:    now,
			now:                  now,
			existingObjects:      []client.Object{},
			expectedShouldUpdate: false,
			errorMessage:         nil,
		},
		{
			name: "Canary paused",
			reconcilerOptions: ReconcilerOptions{
				ExtendedDaemonsetOptions: agent.ExtendedDaemonsetOptions{
					Enabled: true,
				},
				DatadogAgentProfileEnabled: true,
			},
			profile:           &testProfile,
			ddaLastUpdateTime: now,
			now:               now,
			existingObjects: []client.Object{
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-eds",
						Namespace: testNS,
						Annotations: map[string]string{
							edsdatadoghqv1alpha1.ExtendedDaemonSetCanaryPausedAnnotationKey: "true",
						},
						Labels: testEDSLabels,
					},
				},
			},
			expectedShouldUpdate: false,
			errorMessage:         nil,
		},
		{
			name: "Active canary",
			reconcilerOptions: ReconcilerOptions{
				ExtendedDaemonsetOptions: agent.ExtendedDaemonsetOptions{
					Enabled: true,
				},
				DatadogAgentProfileEnabled: true,
			},
			profile:           &testProfile,
			ddaLastUpdateTime: now,
			now:               now,
			existingObjects: []client.Object{
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-eds",
						Namespace: testNS,
						Labels:    testEDSLabels,
					},
					Status: edsdatadoghqv1alpha1.ExtendedDaemonSetStatus{
						Canary: &edsdatadoghqv1alpha1.ExtendedDaemonSetStatusCanary{
							ReplicaSet: "test-ers-2",
							Nodes:      []string{"node-foo"},
						},
					},
				},
			},
			expectedShouldUpdate: false,
			errorMessage:         nil,
		},
		{
			name: "No ERS",
			reconcilerOptions: ReconcilerOptions{
				ExtendedDaemonsetOptions: agent.ExtendedDaemonsetOptions{
					Enabled: true,
				},
				DatadogAgentProfileEnabled: true,
			},
			profile:           &testProfile,
			ddaLastUpdateTime: now,
			now:               now,
			existingObjects: []client.Object{
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-eds",
						Namespace: testNS,
						Labels:    testEDSLabels,
					},
				},
			},
			expectedShouldUpdate: false,
			errorMessage:         errors.New("there must exist at least 1 ExtendedDaemonSetReplicaSet"),
		},
		{
			name: "More than 1 ERS",
			reconcilerOptions: ReconcilerOptions{
				ExtendedDaemonsetOptions: agent.ExtendedDaemonsetOptions{
					Enabled: true,
				},
				DatadogAgentProfileEnabled: true,
			},
			profile:           &testProfile,
			ddaLastUpdateTime: now,
			now:               now,
			existingObjects: []client.Object{
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-eds",
						Namespace: testNS,
						Labels:    testEDSLabels,
					},
				},
				&edsdatadoghqv1alpha1.ExtendedDaemonSetReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-ers-1",
						Namespace: testNS,
						Labels:    testERSLabels,
					},
				},
				&edsdatadoghqv1alpha1.ExtendedDaemonSetReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-ers-2",
						Namespace: testNS,
						Labels:    testERSLabels,
					},
				},
			},
			expectedShouldUpdate: false,
			errorMessage:         nil,
		},
		{
			name: "ERS name and active replicaSet name don't match",
			reconcilerOptions: ReconcilerOptions{
				ExtendedDaemonsetOptions: agent.ExtendedDaemonsetOptions{
					Enabled: true,
				},
				DatadogAgentProfileEnabled: true,
			},
			profile:           &testProfile,
			ddaLastUpdateTime: now,
			now:               now,
			existingObjects: []client.Object{
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-eds",
						Namespace: testNS,
						Labels:    testEDSLabels,
					},
					Status: edsdatadoghqv1alpha1.ExtendedDaemonSetStatus{
						ActiveReplicaSet: "",
					},
				},
				&edsdatadoghqv1alpha1.ExtendedDaemonSetReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-ers-1",
						Namespace: testNS,
						Labels:    testERSLabels,
					},
				},
			},
			expectedShouldUpdate: false,
			errorMessage:         errors.New("ExtendedDaemonSetReplicaSet name does not match ExtendedDaemonSet's active replicaset"),
		},
		{
			name: "DDA was just updated",
			reconcilerOptions: ReconcilerOptions{
				ExtendedDaemonsetOptions: agent.ExtendedDaemonsetOptions{
					Enabled: true,
				},
				DatadogAgentProfileEnabled: true,
			},
			profile:           &testProfile,
			ddaLastUpdateTime: now,
			now:               now,
			existingObjects: []client.Object{
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-eds",
						Namespace: testNS,
						Labels:    testEDSLabels,
					},
					Status: edsdatadoghqv1alpha1.ExtendedDaemonSetStatus{
						ActiveReplicaSet: "test-ers-1",
					},
				},
				&edsdatadoghqv1alpha1.ExtendedDaemonSetReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-ers-1",
						Namespace: testNS,
						Labels:    testERSLabels,
					},
				},
			},
			expectedShouldUpdate: false,
			errorMessage:         nil,
		},
		{
			name: "Canary validated annotation present but agent spec hashes doesn't match",
			reconcilerOptions: ReconcilerOptions{
				ExtendedDaemonsetOptions: agent.ExtendedDaemonsetOptions{
					Enabled: true,
				},
				DatadogAgentProfileEnabled: true,
			},
			profile:           &testProfile,
			ddaLastUpdateTime: now5MinBefore,
			now:               now,
			existingObjects: []client.Object{
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-eds",
						Namespace: testNS,
						Annotations: map[string]string{
							edsdatadoghqv1alpha1.ExtendedDaemonSetCanaryValidAnnotationKey: "test-ers-2",
							constants.MD5AgentDeploymentAnnotationKey:                      "12345",
						},
						Labels: testEDSLabels,
					},
					Status: edsdatadoghqv1alpha1.ExtendedDaemonSetStatus{
						ActiveReplicaSet: "test-ers-1",
					},
				},
				&edsdatadoghqv1alpha1.ExtendedDaemonSetReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-ers-1",
						Namespace: testNS,
						Labels:    testERSLabels,
						Annotations: map[string]string{
							constants.MD5AgentDeploymentAnnotationKey: "67890",
						},
					},
				},
			},
			expectedShouldUpdate: false,
			errorMessage:         nil,
		},
		{
			name: "Canary validated",
			reconcilerOptions: ReconcilerOptions{
				ExtendedDaemonsetOptions: agent.ExtendedDaemonsetOptions{
					Enabled: true,
				},
				DatadogAgentProfileEnabled: true,
			},
			profile:           &testProfile,
			ddaLastUpdateTime: now5MinBefore,
			now:               now,
			existingObjects: []client.Object{
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-eds",
						Namespace: testNS,
						Annotations: map[string]string{
							edsdatadoghqv1alpha1.ExtendedDaemonSetCanaryValidAnnotationKey: "test-ers-1",
						},
						Labels: testEDSLabels,
					},
					Status: edsdatadoghqv1alpha1.ExtendedDaemonSetStatus{
						ActiveReplicaSet: "test-ers-1",
					},
				},
				&edsdatadoghqv1alpha1.ExtendedDaemonSetReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-ers-1",
						Namespace: testNS,
						Labels:    testERSLabels,
					},
				},
			},
			expectedShouldUpdate: true,
			errorMessage:         nil,
		},
		{
			name: "Canary duration hasn't passed",
			reconcilerOptions: ReconcilerOptions{
				ExtendedDaemonsetOptions: agent.ExtendedDaemonsetOptions{
					Enabled:        true,
					CanaryDuration: 10 * time.Minute,
				},
				DatadogAgentProfileEnabled: true,
			},
			profile:           &testProfile,
			ddaLastUpdateTime: now5MinBefore,
			now:               now,
			existingObjects: []client.Object{
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-eds",
						Namespace: testNS,
						Labels:    testEDSLabels,
					},
					Status: edsdatadoghqv1alpha1.ExtendedDaemonSetStatus{
						ActiveReplicaSet: "test-ers-1",
					},
				},
				&edsdatadoghqv1alpha1.ExtendedDaemonSetReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-ers-1",
						Namespace: testNS,
						Labels:    testERSLabels,
					},
				},
			},
			expectedShouldUpdate: false,
			errorMessage:         nil,
		},
		{
			name: "Agent spec hash for EDS and ERS doesn't match",
			reconcilerOptions: ReconcilerOptions{
				ExtendedDaemonsetOptions: agent.ExtendedDaemonsetOptions{
					Enabled:        true,
					CanaryDuration: 10 * time.Minute,
				},
				DatadogAgentProfileEnabled: true,
			},
			profile:           &testProfile,
			ddaLastUpdateTime: now15MinBefore,
			now:               now,
			existingObjects: []client.Object{
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-eds",
						Namespace: testNS,
						Labels:    testEDSLabels,
						Annotations: map[string]string{
							constants.MD5AgentDeploymentAnnotationKey: "12345",
						},
					},
					Status: edsdatadoghqv1alpha1.ExtendedDaemonSetStatus{
						ActiveReplicaSet: "test-ers-1",
					},
				},
				&edsdatadoghqv1alpha1.ExtendedDaemonSetReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-ers-1",
						Namespace: testNS,
						Labels:    testERSLabels,
						Annotations: map[string]string{
							constants.MD5AgentDeploymentAnnotationKey: "67890",
						},
					},
				},
			},
			expectedShouldUpdate: false,
			errorMessage:         nil,
		},
		{
			name: "Agent spec hash for EDS and ERS match",
			reconcilerOptions: ReconcilerOptions{
				ExtendedDaemonsetOptions: agent.ExtendedDaemonsetOptions{
					Enabled:        true,
					CanaryDuration: 10 * time.Minute,
				},
				DatadogAgentProfileEnabled: true,
			},
			profile:           &testProfile,
			ddaLastUpdateTime: now15MinBefore,
			now:               now,
			existingObjects: []client.Object{
				&edsdatadoghqv1alpha1.ExtendedDaemonSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-eds",
						Namespace: testNS,
						Labels:    testEDSLabels,
						Annotations: map[string]string{
							constants.MD5AgentDeploymentAnnotationKey: "12345",
						},
					},
					Status: edsdatadoghqv1alpha1.ExtendedDaemonSetStatus{
						ActiveReplicaSet: "test-ers-1",
					},
				},
				&edsdatadoghqv1alpha1.ExtendedDaemonSetReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-ers-1",
						Namespace: testNS,
						Labels:    testERSLabels,
						Annotations: map[string]string{
							constants.MD5AgentDeploymentAnnotationKey: "12345",
						},
					},
				},
			},
			expectedShouldUpdate: true,
			errorMessage:         nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(tt.existingObjects...).Build()

			r := &Reconciler{
				client:  fakeClient,
				log:     logf.Log.WithName(tt.name),
				options: tt.reconcilerOptions,
			}

			actual, err := r.shouldUpdateProfileDaemonSet(tt.profile, tt.ddaLastUpdateTime, tt.now)
			assert.Equal(t, tt.expectedShouldUpdate, actual)
			assert.Equal(t, tt.errorMessage, err)
		})
	}
}

func Test_getDDALastUpdatedTime(t *testing.T) {
	now := metav1.Now()
	now5MinLater := metav1.NewTime(now.Add(5 * time.Minute))
	now15MinLater := metav1.NewTime(now.Add(15 * time.Minute))
	tests := []struct {
		name              string
		managedFields     []metav1.ManagedFieldsEntry
		creationTimestamp metav1.Time
		expected          metav1.Time
	}{
		{
			name:              "no managed field entries",
			managedFields:     []metav1.ManagedFieldsEntry{},
			creationTimestamp: now,
			expected:          now,
		},
		{
			name: "one new entry",
			managedFields: []metav1.ManagedFieldsEntry{
				{
					Time: &now15MinLater,
				},
			},
			creationTimestamp: now,
			expected:          now15MinLater,
		},
		{
			name: "multiple entries",
			managedFields: []metav1.ManagedFieldsEntry{
				{
					Time: &now15MinLater,
				},
				{
					Time: &now5MinLater,
				},
				{
					Time: &now,
				},
			},
			creationTimestamp: now,
			expected:          now15MinLater,
		},
		{
			name: "ignore status entry",
			managedFields: []metav1.ManagedFieldsEntry{
				{
					Subresource: "status",
					Time:        &now15MinLater,
				},
				{
					Time: &now5MinLater,
				},
				{
					Time: &now,
				},
			},
			creationTimestamp: now,
			expected:          now5MinLater,
		},
		{
			name: "nil time entry",
			managedFields: []metav1.ManagedFieldsEntry{
				{
					Time: &now15MinLater,
				},
				{
					Manager: "test",
				},
				{
					Time: &now,
				},
			},
			creationTimestamp: now,
			expected:          now15MinLater,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := getDDALastUpdatedTime(tt.managedFields, tt.creationTimestamp)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func Test_shouldProfileWaitForCanary(t *testing.T) {
	logger := logf.Log.WithName("Test_shouldProfileWaitForCanary")

	tests := []struct {
		name        string
		annotations map[string]string
		expected    bool
	}{
		{
			name:        "Nil annotations",
			annotations: nil,
			expected:    false,
		},
		{
			name:        "Empty annotations",
			annotations: map[string]string{},
			expected:    false,
		},
		{
			name: "No relevant annotations",
			annotations: map[string]string{
				"foo": "bar",
			},
			expected: false,
		},
		{
			name: "Relevant annotation exists",
			annotations: map[string]string{
				"foo":                   "bar",
				profileWaitForCanaryKey: "true",
			},
			expected: true,
		},
		{
			name: "Relevant annotation exists and is false",
			annotations: map[string]string{
				"foo":                   "bar",
				profileWaitForCanaryKey: "false",
			},
			expected: false,
		},
		{
			name: "Relevant annotation exists, but value is not bool",
			annotations: map[string]string{
				"foo":                   "bar",
				profileWaitForCanaryKey: "yes",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shouldWait := shouldProfileWaitForCanary(logger, tt.annotations)
			assert.Equal(t, tt.expected, shouldWait)
		})
	}
}

func Test_addDDAIStatusToDDAStatus(t *testing.T) {
	sch := runtime.NewScheme()
	_ = scheme.AddToScheme(sch)
	_ = v1alpha1.AddToScheme(sch)
	_ = v2alpha1.AddToScheme(sch)

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
								Enabled: apiutils.NewBoolPointer(true),
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects([]client.Object{&tt.existingDDAI}...).Build()

			r := &Reconciler{
				client: fakeClient,
				log:    logf.Log.WithName(tt.name),
			}

			err := r.addDDAIStatusToDDAStatus(&tt.status, tt.existingDDAI.ObjectMeta)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectedStatus, tt.status)
		})
	}
}
