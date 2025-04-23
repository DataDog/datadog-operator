package datadogagent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

const testNamespace = "foo"

func Test_profilesToApply(t *testing.T) {
	t1 := time.Now()
	t2 := t1.Add(time.Minute)
	t3 := t2.Add(time.Minute)
	now := metav1.NewTime(t1)

	sch := runtime.NewScheme()
	_ = scheme.AddToScheme(sch)
	_ = v1alpha1.AddToScheme(sch)
	ctx := context.Background()

	testCases := []struct {
		name                     string
		nodeList                 []corev1.Node
		profileList              []client.Object
		wantProfilesToApply      func() []v1alpha1.DatadogAgentProfile
		wantProfileAppliedByNode map[string]types.NamespacedName
		wantError                error
	}{
		{
			name: "no user-created profiles to apply",
			nodeList: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							"1": "1",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							"2": "1",
						},
					},
				},
			},
			profileList: []client.Object{},
			wantProfilesToApply: func() []v1alpha1.DatadogAgentProfile {
				return []v1alpha1.DatadogAgentProfile{defaultProfile()}
			},
			wantProfileAppliedByNode: map[string]types.NamespacedName{
				"node1": {
					Namespace: "",
					Name:      "default",
				},
				"node2": {
					Namespace: "",
					Name:      "default",
				},
			},
		},
		{
			name:        "no nodes, no profiles",
			nodeList:    []corev1.Node{},
			profileList: []client.Object{},
			wantProfilesToApply: func() []v1alpha1.DatadogAgentProfile {
				return []v1alpha1.DatadogAgentProfile{defaultProfile()}
			}, wantProfileAppliedByNode: map[string]types.NamespacedName{},
		},
		{
			name:        "no nodes",
			nodeList:    []corev1.Node{},
			profileList: generateObjectList([]string{"1"}, []time.Time{t1}),
			wantProfilesToApply: func() []v1alpha1.DatadogAgentProfile {
				profileList := generateProfileList([]string{"1"}, []time.Time{t1})
				profileList[0].Status = v1alpha1.DatadogAgentProfileStatus{
					LastUpdate:  &now,
					CurrentHash: "36a4d655a44a0ca07780fff47dd96c6a",
					Conditions:  nil,
					Valid:       "Unknown",
					Applied:     "Unknown",
				}
				profileList[0].ResourceVersion = "1000"
				return profileList
			},
			wantProfileAppliedByNode: map[string]types.NamespacedName{},
		},
		{
			name: "one profile",
			nodeList: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							"1": "1",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							"2": "1",
						},
					},
				},
			},
			profileList: generateObjectList([]string{"1"}, []time.Time{t1}),
			wantProfilesToApply: func() []v1alpha1.DatadogAgentProfile {
				profileList := generateProfileList([]string{"1"}, []time.Time{t1})
				profileList[0].Status = v1alpha1.DatadogAgentProfileStatus{
					LastUpdate:  &now,
					CurrentHash: "36a4d655a44a0ca07780fff47dd96c6a",
					Conditions: []metav1.Condition{
						{
							Type:               "Valid",
							Status:             "True",
							LastTransitionTime: now,
							Reason:             "Valid",
							Message:            "Valid manifest",
						},
						{
							Type:               "Applied",
							Status:             "True",
							LastTransitionTime: now,
							Reason:             "Applied",
							Message:            "Profile applied",
						},
					},
					Valid:   "True",
					Applied: "True",
				}
				profileList[0].ResourceVersion = "1000"
				return profileList
			},
			wantProfileAppliedByNode: map[string]types.NamespacedName{
				"node1": {
					Namespace: testNamespace,
					Name:      "1",
				},
				"node2": {
					Namespace: "",
					Name:      "default",
				},
			},
		},
		{
			name: "several non-conflicting profiles",
			nodeList: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							"1": "1",
						},
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							"2": "1",
						},
					},
				},
			},
			profileList: generateObjectList([]string{"1", "2"}, []time.Time{t1, t2}),
			wantProfilesToApply: func() []v1alpha1.DatadogAgentProfile {
				profileList := generateProfileList([]string{"1", "2"}, []time.Time{t1, t2})
				profileList[0].Status = v1alpha1.DatadogAgentProfileStatus{
					LastUpdate:  &now,
					CurrentHash: "36a4d655a44a0ca07780fff47dd96c6a",
					Conditions: []metav1.Condition{
						{
							Type:               "Valid",
							Status:             "True",
							LastTransitionTime: now,
							Reason:             "Valid",
							Message:            "Valid manifest",
						},
						{
							Type:               "Applied",
							Status:             "True",
							LastTransitionTime: now,
							Reason:             "Applied",
							Message:            "Profile applied",
						},
					},
					Valid:   "True",
					Applied: "True",
				}
				profileList[0].ResourceVersion = "1000"
				profileList[1].Status = v1alpha1.DatadogAgentProfileStatus{
					LastUpdate:  &now,
					CurrentHash: "e7eda6755e8a98d127140e2169204312",
					Conditions: []metav1.Condition{
						{
							Type:               "Valid",
							Status:             "True",
							LastTransitionTime: now,
							Reason:             "Valid",
							Message:            "Valid manifest",
						},
						{
							Type:               "Applied",
							Status:             "True",
							LastTransitionTime: now,
							Reason:             "Applied",
							Message:            "Profile applied",
						},
					},
					Valid:   "True",
					Applied: "True",
				}
				profileList[1].ResourceVersion = "1000"
				return profileList
			},
			wantProfileAppliedByNode: map[string]types.NamespacedName{
				"node1": {
					Namespace: testNamespace,
					Name:      "1",
				},
				"node2": {
					Namespace: testNamespace,
					Name:      "2",
				},
			},
		},
		{
			// This test defines 3 profiles created in this order: profile-2,
			// profile-1, profile-3 (not sorted here to make sure that the code does).
			// - profile-1 and profile-2 conflict, but profile-2 is the oldest,
			// so it wins.
			// - profile-1 and profile-3 conflict, but profile-1 is not applied
			// because of the conflict with profile-2, so profile-3 should be.
			// So in this case, the returned profiles should be profile-2,
			// profile-3 and a default one.
			name: "several conflicting profiles with different creation timestamps",
			nodeList: []corev1.Node{
				// node1 matches profile-1 and profile-3
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							"1": "1",
							"3": "1",
						},
					},
				},
				// node2 matches profile-2
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node2",
						Labels: map[string]string{
							"2": "1",
						},
					},
				},
				// node3 matches profile-1 and profile-2
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node3",
						Labels: map[string]string{
							"1": "1",
							"2": "1",
						},
					},
				},
			},
			profileList: generateObjectList([]string{"1", "2", "3"}, []time.Time{t2, t1, t3}),
			wantProfilesToApply: func() []v1alpha1.DatadogAgentProfile {
				profileList := generateProfileList([]string{"2", "3"}, []time.Time{t1, t3})
				profileList[0].Status = v1alpha1.DatadogAgentProfileStatus{
					LastUpdate:  &now,
					CurrentHash: "e7eda6755e8a98d127140e2169204312",
					Conditions: []metav1.Condition{
						{
							Type:               "Valid",
							Status:             "True",
							LastTransitionTime: now,
							Reason:             "Valid",
							Message:            "Valid manifest",
						},
						{
							Type:               "Applied",
							Status:             "True",
							LastTransitionTime: now,
							Reason:             "Applied",
							Message:            "Profile applied",
						},
					},
					Valid:   "True",
					Applied: "True",
				}
				profileList[0].ResourceVersion = "1000"
				profileList[1].Status = v1alpha1.DatadogAgentProfileStatus{
					LastUpdate:  &now,
					CurrentHash: "6cc0746a51b8e52da6e4e625d3181686",
					Conditions: []metav1.Condition{
						{
							Type:               "Valid",
							Status:             "True",
							LastTransitionTime: now,
							Reason:             "Valid",
							Message:            "Valid manifest",
						},
						{
							Type:               "Applied",
							Status:             "True",
							LastTransitionTime: now,
							Reason:             "Applied",
							Message:            "Profile applied",
						},
					},
					Valid:   "True",
					Applied: "True",
				}
				profileList[1].ResourceVersion = "1000"
				return profileList
			},
			wantProfileAppliedByNode: map[string]types.NamespacedName{
				"node1": {
					Namespace: testNamespace,
					Name:      "3",
				},
				"node2": {
					Namespace: testNamespace,
					Name:      "2",
				},
				"node3": {
					Namespace: testNamespace,
					Name:      "2",
				},
			},
		},
		{
			// This test defines 3 profiles with the same creation timestamp:
			// profile-2, profile-1, profile-3 (not sorted alphabetically here
			// to make sure that the code does).
			// The 3 profiles conflict and only profile-1 should apply because
			// it's the first one alphabetically.
			name: "conflicting profiles with the same creation timestamp",
			nodeList: []corev1.Node{
				// matches all profiles
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							"1": "1",
							"2": "1",
							"3": "1",
						},
					},
				},
			},
			profileList: generateObjectList([]string{"2", "1", "3"}, []time.Time{t1, t1, t1}),
			wantProfilesToApply: func() []v1alpha1.DatadogAgentProfile {
				profileList := generateProfileList([]string{"1"}, []time.Time{t1})
				profileList[0].Status = v1alpha1.DatadogAgentProfileStatus{
					LastUpdate:  &now,
					CurrentHash: "36a4d655a44a0ca07780fff47dd96c6a",
					Conditions: []metav1.Condition{
						{
							Type:               "Valid",
							Status:             "True",
							LastTransitionTime: now,
							Reason:             "Valid",
							Message:            "Valid manifest",
						},
						{
							Type:               "Applied",
							Status:             "True",
							LastTransitionTime: now,
							Reason:             "Applied",
							Message:            "Profile applied",
						},
					},
					Valid:   "True",
					Applied: "True",
				}
				profileList[0].ResourceVersion = "1000"
				return profileList
			},
			wantProfileAppliedByNode: map[string]types.NamespacedName{
				"node1": {
					Namespace: testNamespace,
					Name:      "1",
				},
			},
		},
		{
			name: "invalid profile",
			nodeList: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							"1": "1",
						},
					},
				},
			},
			profileList: []client.Object{
				&v1alpha1.DatadogAgentProfile{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNamespace,
						Name:      "invalid",
					},
					Spec: v1alpha1.DatadogAgentProfileSpec{},
				},
			},
			wantProfilesToApply: func() []v1alpha1.DatadogAgentProfile {
				return generateProfileList([]string{}, []time.Time{})
			},
			wantProfileAppliedByNode: map[string]types.NamespacedName{
				"node1": {
					Namespace: "",
					Name:      "default",
				},
			},
		},
		{
			// Profile 1 matches node1 and should be applied.
			// Profile 2 doesn't conflict with Profile 1 but doesn't apply
			// to any nodes since there are no matching nodes.
			name: "invalid profiles + valid profiles",
			nodeList: []corev1.Node{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "node1",
						Labels: map[string]string{
							"1": "1",
						},
					},
				},
			},
			profileList: append(generateObjectList([]string{"1", "2"}, []time.Time{t1, t2}), []client.Object{
				&v1alpha1.DatadogAgentProfile{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNamespace,
						Name:      "invalid-no-affinity",
					},
					Spec: v1alpha1.DatadogAgentProfileSpec{
						Config: &v1alpha1.Config{
							Override: map[v1alpha1.ComponentName]*v1alpha1.Override{
								v1alpha1.NodeAgentComponentName: {
									Containers: map[common.AgentContainerName]*v1alpha1.Container{
										common.CoreAgentContainerName: {
											Resources: &corev1.ResourceRequirements{
												Requests: corev1.ResourceList{
													corev1.ResourceCPU: resource.MustParse("100m"),
												},
											},
										},
									},
								},
							},
						},
					},
				},
				&v1alpha1.DatadogAgentProfile{
					ObjectMeta: metav1.ObjectMeta{
						Namespace: testNamespace,
						Name:      "invalid-no-config",
					},
					Spec: v1alpha1.DatadogAgentProfileSpec{
						ProfileAffinity: &v1alpha1.ProfileAffinity{
							ProfileNodeAffinity: []corev1.NodeSelectorRequirement{
								{
									Key:      "os",
									Operator: corev1.NodeSelectorOpIn,
									Values:   []string{"linux"},
								},
							},
						},
					},
				},
			}...),
			wantProfilesToApply: func() []v1alpha1.DatadogAgentProfile {
				profileList := generateProfileList([]string{"1", "2"}, []time.Time{t1, t2})
				profileList[0].Status = v1alpha1.DatadogAgentProfileStatus{
					LastUpdate:  &now,
					CurrentHash: "36a4d655a44a0ca07780fff47dd96c6a",
					Conditions: []metav1.Condition{
						{
							Type:               "Valid",
							Status:             "True",
							LastTransitionTime: now,
							Reason:             "Valid",
							Message:            "Valid manifest",
						},
						{
							Type:               "Applied",
							Status:             "True",
							LastTransitionTime: now,
							Reason:             "Applied",
							Message:            "Profile applied",
						},
					},
					Valid:   "True",
					Applied: "True",
				}
				profileList[0].ResourceVersion = "1000"
				profileList[1].Status = v1alpha1.DatadogAgentProfileStatus{
					LastUpdate:  &now,
					CurrentHash: "e7eda6755e8a98d127140e2169204312",
					Conditions: []metav1.Condition{
						{
							Type:               "Valid",
							Status:             "True",
							LastTransitionTime: now,
							Reason:             "Valid",
							Message:            "Valid manifest",
						},
					},
					Valid:   "True",
					Applied: "Unknown",
				}
				profileList[1].ResourceVersion = "1000"
				return profileList
			},
			wantProfileAppliedByNode: map[string]types.NamespacedName{
				"node1": {
					Namespace: testNamespace,
					Name:      "1",
				},
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithScheme(sch).WithStatusSubresource(&v1alpha1.DatadogAgentProfile{}).WithObjects(tt.profileList...).Build()
			logger := logf.Log.WithName("Test_profilesToApply")
			eventBroadcaster := record.NewBroadcaster()
			recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "Test_profilesToApply"})

			r := &Reconciler{
				client:   fakeClient,
				log:      logger,
				recorder: recorder,
				options: ReconcilerOptions{
					DatadogAgentProfileEnabled: true,
				},
			}

			profilesToApply, profileAppliedByNode, err := r.profilesToApply(ctx, logger, tt.nodeList, metav1.NewTime(t1), &v2alpha1.DatadogAgent{})
			require.NoError(t, err)

			wantProfilesToApply := tt.wantProfilesToApply()

			for i := range wantProfilesToApply {
				// After version update need to truncate times set by fake to Seconds
				wantProfilesToApply[i].CreationTimestamp.Time = wantProfilesToApply[i].CreationTimestamp.Time.Truncate(time.Second)
				profilesToApply[i].CreationTimestamp.Time = profilesToApply[i].CreationTimestamp.Time.Truncate(time.Second)
				if wantProfilesToApply[i].Status.LastUpdate != nil {
					wantProfilesToApply[i].Status.LastUpdate.Time = wantProfilesToApply[i].Status.LastUpdate.Time.Truncate(time.Second)
					profilesToApply[i].Status.LastUpdate.Time = profilesToApply[i].Status.LastUpdate.Time.Truncate(time.Second)
				}

				assert.Equal(t, wantProfilesToApply[i], profilesToApply[i])
			}
			assert.Equal(t, wantProfilesToApply, profilesToApply)
			// assert.ElementsMatch(t, wantProfilesToApply, profilesToApply)
			assert.Equal(t, tt.wantProfileAppliedByNode, profileAppliedByNode)
		})
	}
}

func generateObjectList(profileIdentifiers []string, creationTimes []time.Time) []client.Object {
	objectList := []client.Object{}
	for i, j := range profileIdentifiers {
		profile := exampleProfile(j, creationTimes[i])
		objectList = append(objectList, &profile)
	}
	return objectList
}

func generateProfileList(profileIdentifiers []string, creationTimes []time.Time) []v1alpha1.DatadogAgentProfile {
	profileList := []v1alpha1.DatadogAgentProfile{}
	for i, j := range profileIdentifiers {
		profileList = append(profileList, exampleProfile(j, creationTimes[i]))
	}
	profileList = append(profileList, defaultProfile())
	return profileList
}

func exampleProfile(i string, creationTime time.Time) v1alpha1.DatadogAgentProfile {
	return v1alpha1.DatadogAgentProfile{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:         testNamespace,
			Name:              i,
			CreationTimestamp: metav1.NewTime(creationTime.Truncate(time.Second)),
		},
		Spec: v1alpha1.DatadogAgentProfileSpec{
			ProfileAffinity: &v1alpha1.ProfileAffinity{
				ProfileNodeAffinity: []corev1.NodeSelectorRequirement{
					{
						Key:      i,
						Operator: corev1.NodeSelectorOpIn,
						Values:   []string{"1"},
					},
				},
			},
			Config: &v1alpha1.Config{
				Override: map[v1alpha1.ComponentName]*v1alpha1.Override{
					v1alpha1.NodeAgentComponentName: {
						Containers: map[common.AgentContainerName]*v1alpha1.Container{
							common.CoreAgentContainerName: {
								Resources: &corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceCPU: resource.MustParse(fmt.Sprintf("%s00m", i)),
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func defaultProfile() v1alpha1.DatadogAgentProfile {
	return v1alpha1.DatadogAgentProfile{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "",
			Name:      "default",
		},
	}
}

func Test_updateSecretHash(t *testing.T) {
	sch := runtime.NewScheme()
	_ = scheme.AddToScheme(sch)
	_ = v1alpha1.AddToScheme(sch)
	_ = v2alpha1.AddToScheme(sch)
	const agentName = "test-dda"
	const secretName = "test-secret"

	tests := []struct {
		name          string
		dda           *v2alpha1.DatadogAgent
		secret        *corev1.Secret
		expectedEnv   string
		expectedValue string
	}{
		{
			name: "API key secret present",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentName,
					Namespace: "default",
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Global: &v2alpha1.GlobalConfig{
						Credentials: &v2alpha1.DatadogCredentials{
							APISecret: &v2alpha1.SecretConfig{
								SecretName: secretName,
							},
						},
					},
				},
			},
			secret: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: "default",
				},
				Data: map[string][]byte{
					"api-key": []byte("test-api-key"),
				},
			},
			expectedEnv:   "API_SECRET_HASH",
			expectedValue: "test-api-key",
		},
		{
			name: "API key secret not present",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentName,
					Namespace: "default",
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Global: &v2alpha1.GlobalConfig{
						Credentials: &v2alpha1.DatadogCredentials{},
					},
				},
			},
			secret:        nil,
			expectedEnv:   "",
			expectedValue: "",
		},
		{
			name: "API key secret present but Secret does not exist",
			dda: &v2alpha1.DatadogAgent{
				ObjectMeta: metav1.ObjectMeta{
					Name:      agentName,
					Namespace: "default",
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Global: &v2alpha1.GlobalConfig{
						Credentials: &v2alpha1.DatadogCredentials{
							APISecret: &v2alpha1.SecretConfig{
								SecretName: "test-secret",
							},
						},
					},
				},
			},
			secret:        nil,
			expectedEnv:   "",
			expectedValue: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objs []client.Object
			objs = append(objs, tt.dda)
			if tt.secret != nil {
				objs = append(objs, tt.secret)
			}

			client := fake.NewClientBuilder().WithScheme(sch).WithObjects(objs...).Build()
			reconciler := &Reconciler{
				client: client,
				log:    logf.Log.WithName("Test_updateSecretHash"),
				options: ReconcilerOptions{
					DatadogAgentProfileEnabled: true,
				},
			}

			// Call the updateSecretHash function
			reconciler.updateSecretHash(context.Background(), tt.dda)

			// Verify that the secret hash was appended to spec.global.env if secret is present
			if tt.secret != nil {
				expectedHash := sha256.New()
				expectedHash.Write([]byte(tt.expectedValue))
				secretHash := hex.EncodeToString(expectedHash.Sum(nil))

				found := false
				for _, envVar := range tt.dda.Spec.Global.Env {
					if envVar.Name == tt.expectedEnv && envVar.Value == secretHash {
						found = true
						break
					}
				}
				assert.True(t, found, fmt.Sprintf("%s not found in spec.global.env", tt.expectedEnv))
			} else {
				found := false
				for _, envVar := range tt.dda.Spec.Global.Env {
					if envVar.Name == "API_SECRET_HASH" {
						found = true
						break
					}
				}
				assert.False(t, found, "API_SECRET_HASH should not be present in spec.global.env")
			}
		})
	}
}
