package datadogagentinternal

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	agentcommon "github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/defaults"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
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
			require.Equal(t, tt.expectedLabels, labels)
		})
	}
}

func Test_orphanLegacyProviderDaemonSetsBeforeCreate(t *testing.T) {
	ctx := context.Background()
	legacyGKECOSDSName := kubernetes.GetAgentNameWithProvider("foo-agent", kubernetes.GKECosProvider)
	legacyDefaultDSName := kubernetes.GetAgentNameWithProvider("foo-agent", kubernetes.DefaultProvider)

	tests := []struct {
		name              string
		desiredDS         *appsv1.DaemonSet
		legacyObjects     []client.Object
		extraObjects      []client.Object
		wantOrphaned      bool
		wantDeletedNames  []string
		wantExistingNames []string
	}{
		{
			name:      "orphans legacy GKE COS DaemonSet",
			desiredDS: testDesiredDaemonSet(true),
			legacyObjects: []client.Object{
				testLegacyGKECOSDaemonSet(),
			},
			wantOrphaned:     true,
			wantDeletedNames: []string{legacyGKECOSDSName},
		},
		{
			name:      "orphans legacy default and GKE COS DaemonSets",
			desiredDS: testDesiredDaemonSet(true),
			legacyObjects: []client.Object{
				testLegacyDefaultDaemonSet(),
				testLegacyGKECOSDaemonSet(),
			},
			wantOrphaned:     true,
			wantDeletedNames: []string{legacyDefaultDSName, legacyGKECOSDSName},
		},
		{
			name:      "skips when replacement still has src hostPath",
			desiredDS: testDesiredDaemonSet(false),
			legacyObjects: []client.Object{
				testLegacyGKECOSDaemonSet(),
			},
			wantOrphaned:      false,
			wantExistingNames: []string{legacyGKECOSDSName},
		},
		{
			name:      "skips DDAI-owned future DaemonSet",
			desiredDS: testDesiredDaemonSet(true),
			legacyObjects: []client.Object{
				testLegacyGKECOSDaemonSet(func(ds *appsv1.DaemonSet) {
					ds.OwnerReferences = []metav1.OwnerReference{
						{
							APIVersion: "datadoghq.com/v1alpha1",
							Kind:       "DatadogAgentInternal",
							Name:       "foo",
							UID:        types.UID("ddai-uid"),
							Controller: ptr.To(true),
						},
					}
				}),
			},
			wantOrphaned:      false,
			wantExistingNames: []string{legacyGKECOSDSName},
		},
		{
			name:      "skips selector mismatch",
			desiredDS: testDesiredDaemonSet(true),
			legacyObjects: []client.Object{
				testLegacyGKECOSDaemonSet(func(ds *appsv1.DaemonSet) {
					ds.Spec.Template.Labels[kubernetes.AppKubernetesInstanceLabelKey] = "other-agent"
				}),
			},
			wantOrphaned:      false,
			wantExistingNames: []string{legacyGKECOSDSName},
		},
		{
			name:      "skips when orphan selector matches additional DaemonSets",
			desiredDS: testDesiredDaemonSet(true),
			legacyObjects: []client.Object{
				testLegacyGKECOSDaemonSet(),
			},
			extraObjects: []client.Object{
				testMatchingDaemonSet("foo-agent-default"),
			},
			wantOrphaned:      false,
			wantExistingNames: []string{legacyGKECOSDSName, "foo-agent-default"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objects := append([]client.Object{}, tt.legacyObjects...)
			objects = append(objects, tt.extraObjects...)
			fakeClient := fake.NewClientBuilder().WithScheme(testDatadogAgentInternalScheme(t)).WithObjects(objects...).Build()
			r := &Reconciler{client: fakeClient}

			orphaned, err := r.orphanLegacyProviderDaemonSetsBeforeCreate(ctx, testDDAI(), tt.desiredDS)
			require.NoError(t, err)
			require.Equal(t, tt.wantOrphaned, orphaned)

			for _, name := range tt.wantDeletedNames {
				gotDS := &appsv1.DaemonSet{}
				err = fakeClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, gotDS)
				require.True(t, apierrors.IsNotFound(err))
			}
			for _, name := range tt.wantExistingNames {
				gotDS := &appsv1.DaemonSet{}
				err = fakeClient.Get(ctx, types.NamespacedName{Name: name, Namespace: "default"}, gotDS)
				require.NoError(t, err)
			}
		})
	}
}

func Test_createOrUpdateDaemonset_OrphansLegacyGKECOSDaemonSetBeforeCreate(t *testing.T) {
	ctx := context.Background()
	sch := testDatadogAgentInternalScheme(t)
	fakeClient := fake.NewClientBuilder().
		WithScheme(sch).
		WithObjects(testLegacyDefaultDaemonSet(), testLegacyGKECOSDaemonSet()).
		Build()
	r := &Reconciler{
		client:   fakeClient,
		scheme:   sch,
		recorder: record.NewFakeRecorder(10),
	}

	desiredDS := testDesiredDaemonSet(true)
	result, err := r.createOrUpdateDaemonset(ctx, testDDAI(), desiredDS, &datadoghqv1alpha1.DatadogAgentInternalStatus{}, updateDSStatusV2WithAgent)
	require.NoError(t, err)
	require.True(t, result.Requeue)

	gotDesiredDS := &appsv1.DaemonSet{}
	err = fakeClient.Get(ctx, types.NamespacedName{Name: desiredDS.Name, Namespace: desiredDS.Namespace}, gotDesiredDS)
	require.True(t, apierrors.IsNotFound(err))

	gotLegacyDS := &appsv1.DaemonSet{}
	err = fakeClient.Get(ctx, types.NamespacedName{Name: kubernetes.GetAgentNameWithProvider(desiredDS.Name, kubernetes.GKECosProvider), Namespace: desiredDS.Namespace}, gotLegacyDS)
	require.True(t, apierrors.IsNotFound(err))

	gotLegacyDefaultDS := &appsv1.DaemonSet{}
	err = fakeClient.Get(ctx, types.NamespacedName{Name: kubernetes.GetAgentNameWithProvider(desiredDS.Name, kubernetes.DefaultProvider), Namespace: desiredDS.Namespace}, gotLegacyDefaultDS)
	require.True(t, apierrors.IsNotFound(err))
}

func Test_reconcileV2Agent_MigratesLegacyGKECOSDaemonSetWithOOMKillTCPQueueLengthOverride(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name     string
		legacyDS *appsv1.DaemonSet
	}{
		{
			name:     "v1.26.0 to main",
			legacyDS: testLegacyGKECOSDaemonSetFromVersion("v1.26.0"),
		},
		{
			name:     "v1.23.1 to main",
			legacyDS: testLegacyGKECOSDaemonSetFromVersion("v1.23.1"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sch := testDatadogAgentInternalScheme(t)
			legacyDefaultDS := testLegacyDefaultDaemonSet()
			fakeClient := fake.NewClientBuilder().
				WithScheme(sch).
				WithObjects(legacyDefaultDS, tt.legacyDS).
				Build()
			r := &Reconciler{
				client:   fakeClient,
				scheme:   sch,
				recorder: record.NewFakeRecorder(10),
			}

			ddai := testDDAIWithOOMKillTCPQueueLengthOverride()
			configuredFeatures, enabledFeatures, requiredComponents := testBuildFeatures(ctx, fakeClient, ddai)
			_, resourceManagers := r.setupDependencies(ctx, ddai)

			result, err := r.reconcileV2Agent(
				ctx,
				requiredComponents,
				append(configuredFeatures, enabledFeatures...),
				ddai,
				resourceManagers,
				&datadoghqv1alpha1.DatadogAgentInternalStatus{},
				"",
			)
			require.NoError(t, err)
			require.True(t, result.Requeue)

			gotLegacyDS := &appsv1.DaemonSet{}
			err = fakeClient.Get(ctx, types.NamespacedName{Name: tt.legacyDS.Name, Namespace: tt.legacyDS.Namespace}, gotLegacyDS)
			require.True(t, apierrors.IsNotFound(err))

			gotLegacyDefaultDS := &appsv1.DaemonSet{}
			err = fakeClient.Get(ctx, types.NamespacedName{Name: legacyDefaultDS.Name, Namespace: legacyDefaultDS.Namespace}, gotLegacyDefaultDS)
			require.True(t, apierrors.IsNotFound(err))

			gotDesiredDS := &appsv1.DaemonSet{}
			err = fakeClient.Get(ctx, types.NamespacedName{Name: "foo-agent", Namespace: "default"}, gotDesiredDS)
			require.True(t, apierrors.IsNotFound(err))

			_, resourceManagers = r.setupDependencies(ctx, ddai)
			result, err = r.reconcileV2Agent(
				ctx,
				requiredComponents,
				append(configuredFeatures, enabledFeatures...),
				ddai,
				resourceManagers,
				&datadoghqv1alpha1.DatadogAgentInternalStatus{},
				"",
			)
			require.NoError(t, err)
			require.Zero(t, result)

			err = fakeClient.Get(ctx, types.NamespacedName{Name: "foo-agent", Namespace: "default"}, gotDesiredDS)
			require.NoError(t, err)
			require.True(t, daemonSetTemplateIsGKECOSMigrationSafe(gotDesiredDS))
			require.True(t, daemonSetSelectorMatchesPodLabels(gotDesiredDS.Spec.Selector, tt.legacyDS.Spec.Template.Labels))
			require.True(t, daemonSetSelectorMatchesPodLabels(gotDesiredDS.Spec.Selector, legacyDefaultDS.Spec.Template.Labels))
			requireSrcEmptyDirVolume(t, gotDesiredDS)
			requireSystemProbeSrcVolumeMount(t, gotDesiredDS)
		})
	}
}

func testDatadogAgentInternalScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	sch := runtime.NewScheme()
	require.NoError(t, scheme.AddToScheme(sch))
	require.NoError(t, datadoghqv1alpha1.AddToScheme(sch))
	return sch
}

func testDDAI() *datadoghqv1alpha1.DatadogAgentInternal {
	return &datadoghqv1alpha1.DatadogAgentInternal{
		TypeMeta: metav1.TypeMeta{
			APIVersion: datadoghqv1alpha1.GroupVersion.String(),
			Kind:       "DatadogAgentInternal",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "default",
			UID:       types.UID("ddai-uid"),
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "datadoghq.com/v2alpha1",
					Kind:       "DatadogAgent",
					Name:       "foo",
					UID:        types.UID("dda-uid"),
					Controller: ptr.To(true),
				},
			},
		},
	}
}

func testDDAIWithOOMKillTCPQueueLengthOverride() *datadoghqv1alpha1.DatadogAgentInternal {
	ddai := testDDAI()
	ddai.Spec.Features = &datadoghqv2alpha1.DatadogFeatures{
		OOMKill: &datadoghqv2alpha1.OOMKillFeatureConfig{
			Enabled: ptr.To(true),
		},
		TCPQueueLength: &datadoghqv2alpha1.TCPQueueLengthFeatureConfig{
			Enabled: ptr.To(true),
		},
	}
	ddai.Spec.Override = map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride{
		datadoghqv2alpha1.NodeAgentComponentName: {
			Volumes: []corev1.Volume{
				{
					Name: agentcommon.SrcVolumeName,
					VolumeSource: corev1.VolumeSource{
						EmptyDir: &corev1.EmptyDirVolumeSource{},
					},
				},
			},
			Containers: map[apicommon.AgentContainerName]*datadoghqv2alpha1.DatadogAgentGenericContainer{
				apicommon.SystemProbeContainerName: {
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      agentcommon.SrcVolumeName,
							MountPath: agentcommon.SrcVolumePath,
						},
					},
				},
			},
		},
	}
	defaults.DefaultDatadogAgentSpec(&ddai.Spec)
	ddai.Spec.Global.Credentials = &datadoghqv2alpha1.DatadogCredentials{}
	return ddai
}

func testBuildFeatures(ctx context.Context, fakeClient client.Client, ddai *datadoghqv1alpha1.DatadogAgentInternal) ([]feature.Feature, []feature.Feature, feature.RequiredComponents) {
	return feature.BuildFeatures(ddai, &ddai.Spec, ddai.Status.RemoteConfigConfiguration, reconcilerOptionsToFeatureOptions(ctx, fakeClient))
}

func testDesiredDaemonSet(cosSafe bool) *appsv1.DaemonSet {
	ds := testMatchingDaemonSet("foo-agent")
	ds.Spec.Template.Spec.Volumes = []corev1.Volume{
		{
			Name: agentcommon.SrcVolumeName,
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}
	if !cosSafe {
		ds.Spec.Template.Spec.Volumes[0].VolumeSource = corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{Path: agentcommon.SrcVolumePath},
		}
	}
	return ds
}

func testLegacyGKECOSDaemonSetFromVersion(version string) *appsv1.DaemonSet {
	ds := testLegacyGKECOSDaemonSet()
	ds.Annotations = map[string]string{
		"test.datadoghq.com/legacy-version": version,
	}
	return ds
}

func testLegacyDefaultDaemonSet(opts ...func(*appsv1.DaemonSet)) *appsv1.DaemonSet {
	return testLegacyProviderDaemonSet(kubernetes.DefaultProvider, opts...)
}

func testLegacyGKECOSDaemonSet(opts ...func(*appsv1.DaemonSet)) *appsv1.DaemonSet {
	return testLegacyProviderDaemonSet(kubernetes.GKECosProvider, opts...)
}

func testLegacyProviderDaemonSet(provider string, opts ...func(*appsv1.DaemonSet)) *appsv1.DaemonSet {
	ds := testMatchingDaemonSet(kubernetes.GetAgentNameWithProvider("foo-agent", provider))
	ds.Labels[constants.MD5AgentDeploymentProviderLabelKey] = provider
	ds.Spec.Template.Labels[constants.MD5AgentDeploymentProviderLabelKey] = provider
	ds.OwnerReferences = []metav1.OwnerReference{
		{
			APIVersion: "datadoghq.com/v2alpha1",
			Kind:       "DatadogAgent",
			Name:       "foo",
			UID:        types.UID("dda-uid"),
			Controller: ptr.To(true),
		},
	}
	for _, opt := range opts {
		opt(ds)
	}
	return ds
}

func testMatchingDaemonSet(name string) *appsv1.DaemonSet {
	selectorLabels := map[string]string{
		kubernetes.AppKubernetesInstanceLabelKey:   "foo-agent",
		apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
	}
	objectLabels := map[string]string{
		kubernetes.AppKubernetesInstanceLabelKey:   "foo-agent",
		kubernetes.AppKubernetesNameLabelKey:       "datadog-agent-deployment",
		apicommon.AgentDeploymentComponentLabelKey: constants.DefaultAgentResourceSuffix,
		apicommon.AgentDeploymentNameLabelKey:      "foo",
		kubernetes.AppKubernetesComponentLabelKey:  constants.DefaultAgentResourceSuffix,
		kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
		kubernetes.AppKubernetesPartOfLabelKey:     "default-foo",
		kubernetes.AppKubernetesVersionLabelKey:    "",
	}

	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
			Labels:    objectLabels,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: selectorLabels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: objectLabels,
				},
			},
		},
	}
}

func requireSrcEmptyDirVolume(t *testing.T, ds *appsv1.DaemonSet) {
	t.Helper()
	for _, volume := range ds.Spec.Template.Spec.Volumes {
		if volume.Name == agentcommon.SrcVolumeName {
			require.NotNil(t, volume.EmptyDir)
			require.Nil(t, volume.HostPath)
			return
		}
	}
	t.Fatalf("volume %q not found", agentcommon.SrcVolumeName)
}

func requireSystemProbeSrcVolumeMount(t *testing.T, ds *appsv1.DaemonSet) {
	t.Helper()
	for _, container := range ds.Spec.Template.Spec.Containers {
		if container.Name != string(apicommon.SystemProbeContainerName) {
			continue
		}
		for _, volumeMount := range container.VolumeMounts {
			if volumeMount.Name == agentcommon.SrcVolumeName {
				require.Equal(t, agentcommon.SrcVolumePath, volumeMount.MountPath)
				return
			}
		}
		t.Fatalf("volume mount %q not found on %q container", agentcommon.SrcVolumeName, apicommon.SystemProbeContainerName)
	}
	t.Fatalf("container %q not found", apicommon.SystemProbeContainerName)
}
