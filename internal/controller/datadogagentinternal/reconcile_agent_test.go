package datadogagentinternal

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/defaults"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const defaultProvider = kubernetes.DefaultProvider
const gkeCosProvider = kubernetes.GKECloudProvider + "-" + kubernetes.GKECosType

func Test_getDaemonSetNameFromDatadogAgent(t *testing.T) {
	testCases := []struct {
		name       string
		ddai       *datadoghqv1alpha1.DatadogAgentInternal
		wantDSName string
	}{
		{
			name: "no node override",
			ddai: &datadoghqv1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
			},
			wantDSName: "foo-agent",
		},
		{
			name: "node override with no name override",
			ddai: &datadoghqv1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: datadoghqv2alpha1.DatadogAgentSpec{
					Override: map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{
						datadoghqv2alpha1.NodeAgentComponentName: {
							Replicas: ptr.To[int32](10),
						},
					},
				},
			},
			wantDSName: "foo-agent",
		},
		{
			name: "node override with name override",
			ddai: &datadoghqv1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: datadoghqv2alpha1.DatadogAgentSpec{
					Override: map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{
						datadoghqv2alpha1.NodeAgentComponentName: {
							Name:     ptr.To("bar"),
							Replicas: ptr.To[int32](10),
						},
					},
				},
			},
			wantDSName: "bar",
		},
		{
			name: "dca override with name override",
			ddai: &datadoghqv1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
				Spec: datadoghqv2alpha1.DatadogAgentSpec{
					Override: map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{
						datadoghqv2alpha1.ClusterAgentComponentName: {
							Name:     ptr.To("bar"),
							Replicas: ptr.To[int32](10),
						},
					},
				},
			},
			wantDSName: "foo-agent",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			dsName := component.GetDaemonSetNameFromDatadogAgent(tt.ddai.GetObjectMeta(), &tt.ddai.Spec)
			assert.Equal(t, tt.wantDSName, dsName)
		})
	}
}

func Test_isDDAILabeledWithProfile(t *testing.T) {
	testCases := []struct {
		name string
		ddai *datadoghqv1alpha1.DatadogAgentInternal
		want bool
	}{
		{
			name: "no profile label",
			ddai: &datadoghqv1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
				},
			},
			want: false,
		},
		{
			name: "profile label",
			ddai: &datadoghqv1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name: "foo",
					Labels: map[string]string{
						constants.ProfileLabelKey: "foo",
					},
				},
			},
			want: true,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			got := isDDAILabeledWithProfile(tt.ddai)
			assert.Equal(t, tt.want, got)
		})
	}
}

func Test_updateDSStatusV2WithAgent(t *testing.T) {
	liveDaemonset := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "datadog-agent"},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 3,
			CurrentNumberScheduled: 3,
			NumberReady:            3,
			NumberAvailable:        3,
			UpdatedNumberScheduled: 3,
		},
	}

	tests := []struct {
		name              string
		ds                *appsv1.DaemonSet
		conditionStatus   metav1.ConditionStatus
		expectedAgentText string
	}{
		{name: "False condition marks the agent Failed with live counts", ds: liveDaemonset, conditionStatus: metav1.ConditionFalse, expectedAgentText: "Failed (3/3/3)"},
		{name: "False condition with no DaemonSet marks the agent Failed", ds: nil, conditionStatus: metav1.ConditionFalse, expectedAgentText: "Failed"},
		{name: "True condition keeps the computed state", ds: liveDaemonset, conditionStatus: metav1.ConditionTrue, expectedAgentText: "Running (3/3/3)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := &datadoghqv1alpha1.DatadogAgentInternalStatus{}
			updateDSStatusV2WithAgent("datadog-agent", tt.ds, status, metav1.Now(), tt.conditionStatus, "reason", "message")

			assert.Equal(t, tt.expectedAgentText, status.Agent.Status)
			var found bool
			for _, c := range status.Conditions {
				if c.Type == common.AgentReconcileConditionType {
					found = true
					assert.Equal(t, tt.conditionStatus, c.Status)
				}
			}
			assert.True(t, found, "AgentReconcile condition not set")
		})
	}
}

// TestReconcileV2AgentFeatureError checks the agent status when a node agent feature returns an error.
func TestReconcileV2AgentFeatureError(t *testing.T) {
	featErr := errors.New("patchCgroupPermissions is only supported in privileged mode")

	tests := []struct {
		name           string
		nameOverride   *string
		liveDaemonset  string // name of the DaemonSet that exists in the cluster, empty for none
		expectedName   string
		expectedStatus string
	}{
		{
			name:           "default name, DaemonSet exists",
			liveDaemonset:  "dda-foo-agent",
			expectedName:   "dda-foo-agent",
			expectedStatus: "Failed (3/2/0)",
		},
		{
			name:           "name override, DaemonSet exists under the override name",
			nameOverride:   new("custom-agent"),
			liveDaemonset:  "custom-agent",
			expectedName:   "custom-agent",
			expectedStatus: "Failed (3/2/0)",
		},
		{
			name:           "name override, no DaemonSet",
			nameOverride:   new("custom-agent"),
			expectedName:   "custom-agent",
			expectedStatus: "Failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			ddai := newRolloutTestDDAI()
			ddai.Spec.Global = &datadoghqv2alpha1.GlobalConfig{Credentials: &datadoghqv2alpha1.DatadogCredentials{
				APISecret: &datadoghqv2alpha1.SecretConfig{SecretName: "datadog-secret", KeyName: "api-key"},
			}}
			if tt.nameOverride != nil {
				ddai.Spec.Override = map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{
					datadoghqv2alpha1.NodeAgentComponentName: {Name: tt.nameOverride},
				}
			}
			defaults.DefaultDatadogAgentSpec(&ddai.Spec)

			r := newRolloutTestReconciler(false)
			if tt.liveDaemonset != "" {
				require.NoError(t, r.client.Create(ctx, &appsv1.DaemonSet{
					ObjectMeta: metav1.ObjectMeta{Name: tt.liveDaemonset, Namespace: ddai.Namespace},
					Status: appsv1.DaemonSetStatus{
						DesiredNumberScheduled: 3,
						CurrentNumberScheduled: 2,
						NumberReady:            2,
						NumberAvailable:        2,
						UpdatedNumberScheduled: 0,
					},
				}))
			}
			_, managers := r.setupDependencies(ctx, ddai)

			status := &datadoghqv1alpha1.DatadogAgentInternalStatus{}
			features := []feature.Feature{&dummyFeature{IDValue: "failing", ManageNodeAgentError: featErr}}
			_, err := r.reconcileV2Agent(ctx, feature.RequiredComponents{}, features, ddai, managers, status, "", make(map[string][]string))

			require.ErrorContains(t, err, featErr.Error())
			require.NotNil(t, status.Agent)
			assert.Equal(t, tt.expectedName, status.Agent.DaemonsetName)
			assert.Equal(t, "Failed", status.Agent.State)
			assert.Equal(t, tt.expectedStatus, status.Agent.Status)

			var agentCondition *metav1.Condition
			for i := range status.Conditions {
				if status.Conditions[i].Type == common.AgentReconcileConditionType {
					agentCondition = &status.Conditions[i]
				}
			}
			require.NotNil(t, agentCondition, "AgentReconcile condition not set")
			assert.Equal(t, metav1.ConditionFalse, agentCondition.Status)
			assert.Equal(t, "AgentFeatureError", agentCondition.Reason)
			assert.Contains(t, agentCondition.Message, featErr.Error())
		})
	}
}

// TestReconcileV2AgentOrderedShutdown checks the ordered-shutdown experimental
// annotation end to end on a DDAI with NPM (so a system-probe container exists).
func TestReconcileV2AgentOrderedShutdown(t *testing.T) {
	const annotation = "experimental.agent.datadoghq.com/ordered-shutdown"

	reconcileDS := func(t *testing.T, annotations map[string]string, singleContainer, npm bool) *appsv1.DaemonSet {
		t.Helper()
		ctx := context.Background()
		r := newRolloutTestReconciler(false)
		ddai := newRolloutTestDDAI()
		ddai.Annotations = annotations
		ddai.Spec.Global = &datadoghqv2alpha1.GlobalConfig{Credentials: &datadoghqv2alpha1.DatadogCredentials{
			APISecret: &datadoghqv2alpha1.SecretConfig{SecretName: "datadog-secret", KeyName: "api-key"},
		}}
		if singleContainer {
			ddai.Spec.Global.ContainerStrategy = ptr.To(datadoghqv2alpha1.SingleContainerStrategy)
		}
		// Service discovery is on by default and also needs system-probe; turn it
		// off with NPM so the single-container strategy can apply.
		ddai.Spec.Features = &datadoghqv2alpha1.DatadogFeatures{
			NPM:              &datadoghqv2alpha1.NPMFeatureConfig{Enabled: ptr.To(npm)},
			ServiceDiscovery: &datadoghqv2alpha1.ServiceDiscoveryFeatureConfig{Enabled: ptr.To(npm)},
		}
		defaults.DefaultDatadogAgentSpec(&ddai.Spec)

		configured, enabled, required, _ := feature.BuildFeatures(ddai, &ddai.Spec, nil, &feature.Options{Logger: logr.Discard()})
		_, managers := r.setupDependencies(ctx, ddai)
		status := &datadoghqv1alpha1.DatadogAgentInternalStatus{}
		_, err := r.reconcileV2Agent(ctx, required, append(configured, enabled...), ddai, managers, status, defaultProvider, make(map[string][]string))
		require.NoError(t, err)

		list := &appsv1.DaemonSetList{}
		require.NoError(t, r.client.List(ctx, list, client.InNamespace(ddai.Namespace)))
		require.Len(t, list.Items, 1)
		return &list.Items[0]
	}

	assertOrdered := func(t *testing.T, spec corev1.PodSpec) {
		t.Helper()
		require.NotNil(t, spec.TerminationGracePeriodSeconds)
		assert.Equal(t, int64(75), *spec.TerminationGracePeriodSeconds)

		var sawSystemProbe, sawWaiter bool
		for _, c := range spec.Containers {
			if c.Name == string(apicommon.SystemProbeContainerName) {
				sawSystemProbe = true
				assert.Equal(t, []string{"sh", "-c", "exec 9>/var/run/sysprobe/.running.lock && flock -x 9 && exec system-probe --config=/etc/datadog-agent/system-probe.yaml"}, c.Command)
				assert.Nil(t, c.Lifecycle)
				continue
			}
			sawWaiter = true
			require.NotNil(t, c.Lifecycle, c.Name)
			require.NotNil(t, c.Lifecycle.PreStop, c.Name)
			assert.Equal(t, []string{"sh", "-c", "flock -s -w 60 /var/run/sysprobe/.running.lock true || true"}, c.Lifecycle.PreStop.Exec.Command, c.Name)
			var mounts []corev1.VolumeMount
			for _, m := range c.VolumeMounts {
				if m.MountPath == common.SystemProbeSocketVolumePath {
					mounts = append(mounts, m)
				}
			}
			require.Len(t, mounts, 1, c.Name)
			assert.Equal(t, common.SystemProbeSocketVolumeName, mounts[0].Name, c.Name)
		}
		assert.True(t, sawSystemProbe, "NPM must add a system-probe container")
		assert.True(t, sawWaiter)
		for _, ic := range spec.InitContainers {
			assert.Nil(t, ic.Lifecycle, ic.Name)
		}
	}

	t.Run("multi-container with annotation", func(t *testing.T) {
		assertOrdered(t, reconcileDS(t, map[string]string{annotation: "true"}, false, true).Spec.Template.Spec)
	})

	// NPM needs system-probe, so the single-container strategy falls back to
	// multiple containers and ordered shutdown applies.
	t.Run("single-container requested with NPM falls back and is ordered", func(t *testing.T) {
		assertOrdered(t, reconcileDS(t, map[string]string{annotation: "true"}, true, true).Spec.Template.Spec)
	})

	t.Run("multi-container without annotation is unchanged", func(t *testing.T) {
		without := reconcileDS(t, nil, false, true).Spec.Template
		disabled := reconcileDS(t, map[string]string{annotation: "false"}, false, true).Spec.Template
		assert.Equal(t, without, disabled)
		assert.Nil(t, without.Spec.TerminationGracePeriodSeconds)
		for _, c := range without.Spec.Containers {
			assert.Nil(t, c.Lifecycle, c.Name)
			if c.Name == string(apicommon.SystemProbeContainerName) {
				assert.Equal(t, common.DefaultSystemProbeCommand(), c.Command)
			}
		}
	})

	t.Run("GKE Autopilot with annotation is unchanged", func(t *testing.T) {
		const autopilot = "experimental.agent.datadoghq.com/autopilot"
		with := reconcileDS(t, map[string]string{annotation: "true", autopilot: "true"}, false, true).Spec.Template
		without := reconcileDS(t, map[string]string{autopilot: "true"}, false, true).Spec.Template
		assert.Equal(t, without, with)
	})

	t.Run("single-container with annotation is unchanged", func(t *testing.T) {
		with := reconcileDS(t, map[string]string{annotation: "true"}, true, false).Spec.Template
		without := reconcileDS(t, nil, true, false).Spec.Template
		require.Len(t, with.Spec.Containers, 1, "expected the single-container strategy to apply")
		assert.Equal(t, without, with)
	})
}
