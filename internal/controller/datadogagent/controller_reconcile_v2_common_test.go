package datadogagent

import (
	"errors"
	"testing"
	"time"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	edsv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"

	assert "github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

func Test_ensureSelectorInPodTemplateLabels(t *testing.T) {
	logger := logf.Log.WithName("Test_ensureSelectorInPodTemplateLabels")

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
			labels := ensureSelectorInPodTemplateLabels(logger, tt.selector, tt.podTemplateLabels)
			assert.Equal(t, tt.expectedLabels, labels)
		})
	}
}

func Test_shouldCheckCreateStrategyStatus(t *testing.T) {
	tests := []struct {
		name           string
		profile        *v1alpha1.DatadogAgentProfile
		CreateStrategy string
		expected       bool
	}{
		{
			name:           "nil profile",
			profile:        nil,
			CreateStrategy: "true",
			expected:       false,
		},
		{
			name:           "create strategy false",
			profile:        nil,
			CreateStrategy: "false",
			expected:       false,
		},
		{
			name:           "empty profile",
			profile:        &v1alpha1.DatadogAgentProfile{},
			CreateStrategy: "true",
			expected:       false,
		},
		{
			name: "default profile",
			profile: &v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "default",
				},
			},
			CreateStrategy: "true",
			expected:       false,
		},
		{
			name: "empty profile status",
			profile: &v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Status: v1alpha1.DatadogAgentProfileStatus{},
			},
			CreateStrategy: "true",
			expected:       false,
		},
		{
			name: "completed create strategy status",
			profile: &v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Status: v1alpha1.DatadogAgentProfileStatus{
					CreateStrategy: &v1alpha1.CreateStrategy{
						Status: v1alpha1.CompletedStatus,
					},
				},
			},
			CreateStrategy: "true",
			expected:       false,
		},
		{
			name: "in progress create strategy status",
			profile: &v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Status: v1alpha1.DatadogAgentProfileStatus{
					CreateStrategy: &v1alpha1.CreateStrategy{
						Status: v1alpha1.InProgressStatus,
					},
				},
			},
			CreateStrategy: "true",
			expected:       true,
		},
		{
			name: "waiting create strategy status",
			profile: &v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Status: v1alpha1.DatadogAgentProfileStatus{
					CreateStrategy: &v1alpha1.CreateStrategy{
						Status: v1alpha1.WaitingStatus,
					},
				},
			},
			CreateStrategy: "true",
			expected:       true,
		},
		{
			name: "empty status in create strategy status",
			profile: &v1alpha1.DatadogAgentProfile{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Status: v1alpha1.DatadogAgentProfileStatus{
					CreateStrategy: &v1alpha1.CreateStrategy{
						Status: "",
					},
				},
			},
			CreateStrategy: "true",
			expected:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(common.CreateStrategyEnabled, tt.CreateStrategy)
			actual := shouldCheckCreateStrategyStatus(tt.profile)
			assert.Equal(t, tt.expected, actual)
		})
	}
}

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
		edsv1alpha1.ExtendedDaemonSetNameLabelKey: "test-eds",
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
							edsv1alpha1.ExtendedDaemonSetCanaryPausedAnnotationKey: "true",
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
					Status: edsv1alpha1.ExtendedDaemonSetStatus{
						Canary: &edsv1alpha1.ExtendedDaemonSetStatusCanary{},
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
					Status: edsv1alpha1.ExtendedDaemonSetStatus{
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
					Status: edsv1alpha1.ExtendedDaemonSetStatus{
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
			name: "Canary validated annotation present but name doesn't match",
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
							edsv1alpha1.ExtendedDaemonSetCanaryValidAnnotationKey: "test-ers-2",
						},
						Labels: testEDSLabels,
					},
					Status: edsv1alpha1.ExtendedDaemonSetStatus{
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
							edsv1alpha1.ExtendedDaemonSetCanaryValidAnnotationKey: "test-ers-1",
						},
						Labels: testEDSLabels,
					},
					Status: edsv1alpha1.ExtendedDaemonSetStatus{
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
					Status: edsv1alpha1.ExtendedDaemonSetStatus{
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
							v2alpha1.MD5AgentDeploymentAnnotationKey: "12345",
						},
					},
					Status: edsv1alpha1.ExtendedDaemonSetStatus{
						ActiveReplicaSet: "test-ers-1",
					},
				},
				&edsdatadoghqv1alpha1.ExtendedDaemonSetReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-ers-1",
						Namespace: testNS,
						Labels:    testERSLabels,
						Annotations: map[string]string{
							v2alpha1.MD5AgentDeploymentAnnotationKey: "67890",
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
							v2alpha1.MD5AgentDeploymentAnnotationKey: "12345",
						},
					},
					Status: edsv1alpha1.ExtendedDaemonSetStatus{
						ActiveReplicaSet: "test-ers-1",
					},
				},
				&edsdatadoghqv1alpha1.ExtendedDaemonSetReplicaSet{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-ers-1",
						Namespace: testNS,
						Labels:    testERSLabels,
						Annotations: map[string]string{
							v2alpha1.MD5AgentDeploymentAnnotationKey: "12345",
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
