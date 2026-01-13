package datadogagent

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	testutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/testutils"
	"github.com/stretchr/testify/assert"
)

func Test_ssaMergeCRD(t *testing.T) {
	sch := k8sruntime.NewScheme()
	_ = scheme.AddToScheme(sch)
	_ = v1alpha1.AddToScheme(sch)
	_ = v2alpha1.AddToScheme(sch)
	_ = corev1.AddToScheme(sch)
	_ = apiextensionsv1.AddToScheme(sch)
	ctx := context.Background()

	testCases := []struct {
		name    string
		ddai    v1alpha1.DatadogAgentInternal
		profile v1alpha1.DatadogAgentInternal
		want    v1alpha1.DatadogAgentInternal
	}{
		{
			name: "merge new env var",
			ddai: v1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
					ManagedFields: []metav1.ManagedFieldsEntry{
						{
							Manager:    "datadog-operator",
							Operation:  metav1.ManagedFieldsOperationApply,
							FieldsType: "FieldsV1",
							APIVersion: "datadoghq.com/v1alpha1",
						},
					},
				},
				TypeMeta: metav1.TypeMeta{
					APIVersion: "datadoghq.com/v1alpha1",
					Kind:       "DatadogAgentInternal",
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Features: &v2alpha1.DatadogFeatures{
						APM: &v2alpha1.APMFeatureConfig{
							Enabled: apiutils.NewBoolPointer(true),
						},
					},
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Env: []corev1.EnvVar{
								{
									Name:  "EXISTING",
									Value: "value",
								},
							},
						},
					},
				},
			},
			profile: v1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				TypeMeta: metav1.TypeMeta{
					APIVersion: "datadoghq.com/v1alpha1",
					Kind:       "DatadogAgentInternal",
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Env: []corev1.EnvVar{
								{
									Name:  "NEW",
									Value: "newvalue",
								},
							},
						},
					},
				},
			},
			want: v1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Features: &v2alpha1.DatadogFeatures{
						APM: &v2alpha1.APMFeatureConfig{
							Enabled: apiutils.NewBoolPointer(true),
						},
					},
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Env: []corev1.EnvVar{
								{
									Name:  "EXISTING",
									Value: "value",
								},
								{
									Name:  "NEW",
									Value: "newvalue",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "override existing env var",
			ddai: v1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				TypeMeta: metav1.TypeMeta{
					APIVersion: "datadoghq.com/v1alpha1",
					Kind:       "DatadogAgentInternal",
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Features: &v2alpha1.DatadogFeatures{
						APM: &v2alpha1.APMFeatureConfig{
							Enabled: apiutils.NewBoolPointer(true),
						},
					},
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Env: []corev1.EnvVar{
								{
									Name:  "EXISTING",
									Value: "value",
								},
							},
						},
					},
				},
			},
			profile: v1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				TypeMeta: metav1.TypeMeta{
					APIVersion: "datadoghq.com/v1alpha1",
					Kind:       "DatadogAgentInternal",
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Env: []corev1.EnvVar{
								{
									Name:  "EXISTING",
									Value: "newvalue",
								},
							},
						},
					},
				},
			},
			want: v1alpha1.DatadogAgentInternal{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "foo",
					Namespace: "bar",
				},
				Spec: v2alpha1.DatadogAgentSpec{
					Features: &v2alpha1.DatadogFeatures{
						APM: &v2alpha1.APMFeatureConfig{
							Enabled: apiutils.NewBoolPointer(true),
						},
					},
					Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
						v2alpha1.NodeAgentComponentName: {
							Env: []corev1.EnvVar{
								{
									Name:  "EXISTING",
									Value: "newvalue",
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range testCases {
		// Load CRD from config folder
		crd, err := getDDAICRDFromConfig(sch)
		assert.NoError(t, err)

		fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(&tt.ddai, crd).Build()
		logger := logf.Log.WithName("Test_ssaMergeCRD")
		eventBroadcaster := record.NewBroadcaster()
		recorder := eventBroadcaster.NewRecorder(sch, corev1.EventSource{Component: "Test_ssaMergeCRD"})
		fieldManager, err := newFieldManager(fakeClient, sch, v1alpha1.GroupVersion.WithKind("DatadogAgentInternal"))
		assert.NoError(t, err)

		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{
				client:       fakeClient,
				log:          logger,
				scheme:       sch,
				recorder:     recorder,
				fieldManager: fieldManager,
			}

			crd := &apiextensionsv1.CustomResourceDefinition{}
			err := r.client.Get(ctx,
				types.NamespacedName{
					Name: "datadogagentinternals.datadoghq.com",
				},
				crd)
			assert.NoError(t, err)

			ddai, err := r.ssaMergeCRD(&tt.ddai, &tt.profile)
			assert.NoError(t, err)
			obj, ok := ddai.(*v1alpha1.DatadogAgentInternal)
			assert.True(t, ok)
			assert.Equal(t, tt.want.Spec, obj.Spec)
		})
	}
}

func Test_ssaMergeCRD_OutdatedCRD_IgnoresUnknownFields(t *testing.T) {
	sch := k8sruntime.NewScheme()
	_ = scheme.AddToScheme(sch)
	_ = v1alpha1.AddToScheme(sch)
	_ = v2alpha1.AddToScheme(sch)
	_ = corev1.AddToScheme(sch)
	_ = apiextensionsv1.AddToScheme(sch)

	// Load CRD from config folder, then simulate an older CRD that doesn't have
	// .spec.features.cws.enforcement in its schema.
	crd, err := getDDAICRDFromConfig(sch)
	assert.NoError(t, err)
	removeCwsEnforcementFromDDAISchema(t, crd)

	ddai := v1alpha1.DatadogAgentInternal{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "bar",
			ManagedFields: []metav1.ManagedFieldsEntry{
				{
					Manager:    "datadog-operator",
					Operation:  metav1.ManagedFieldsOperationApply,
					FieldsType: "FieldsV1",
					APIVersion: "datadoghq.com/v1alpha1",
				},
			},
		},
		TypeMeta: metav1.TypeMeta{
			APIVersion: "datadoghq.com/v1alpha1",
			Kind:       "DatadogAgentInternal",
		},
		Spec: v2alpha1.DatadogAgentSpec{
			Features: &v2alpha1.DatadogFeatures{
				CWS: &v2alpha1.CWSFeatureConfig{
					Enabled: apiutils.NewBoolPointer(true),
					Enforcement: &v2alpha1.CWSEnforcementConfig{
						Enabled: apiutils.NewBoolPointer(true),
					},
				},
			},
			Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
				v2alpha1.NodeAgentComponentName: {
					Env: []corev1.EnvVar{
						{Name: "EXISTING", Value: "value"},
					},
				},
			},
		},
	}

	profile := v1alpha1.DatadogAgentInternal{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "foo",
			Namespace: "bar",
		},
		TypeMeta: metav1.TypeMeta{
			APIVersion: "datadoghq.com/v1alpha1",
			Kind:       "DatadogAgentInternal",
		},
		Spec: v2alpha1.DatadogAgentSpec{
			Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
				v2alpha1.NodeAgentComponentName: {
					Env: []corev1.EnvVar{
						{Name: "NEW", Value: "newvalue"},
					},
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(&ddai, crd).Build()
	logger := logf.Log.WithName("Test_ssaMergeCRD_OutdatedCRD_IgnoresUnknownFields")
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(sch, corev1.EventSource{Component: "Test_ssaMergeCRD_OutdatedCRD_IgnoresUnknownFields"})
	fieldManager, err := newFieldManager(fakeClient, sch, v1alpha1.GroupVersion.WithKind("DatadogAgentInternal"))
	assert.NoError(t, err)

	r := &Reconciler{
		client:       fakeClient,
		log:          logger,
		scheme:       sch,
		recorder:     recorder,
		fieldManager: fieldManager,
	}

	merged, err := r.ssaMergeCRD(&ddai, &profile)
	assert.NoError(t, err)
	obj, ok := merged.(*v1alpha1.DatadogAgentInternal)
	assert.True(t, ok)

	// Ensure we still merged other fields.
	assert.Equal(t, []corev1.EnvVar{
		{Name: "EXISTING", Value: "value"},
		{Name: "NEW", Value: "newvalue"},
	}, obj.Spec.Override[v2alpha1.NodeAgentComponentName].Env)

	// Because the CRD schema doesn't declare cws.enforcement, the merge should not fail;
	// the unknown field is stripped from the merge inputs.
	if obj.Spec.Features != nil && obj.Spec.Features.CWS != nil {
		assert.Nil(t, obj.Spec.Features.CWS.Enforcement)
	}
}

func Test_pruneObjectsAgainstSchema(t *testing.T) {
	sch := testutils.TestScheme()

	t.Run("prunes unknown fields from both objects", func(t *testing.T) {
		// Load CRD and remove cws.enforcement field to simulate older schema
		crd, err := getDDAICRDFromConfig(sch)
		assert.NoError(t, err)
		removeCwsEnforcementFromDDAISchema(t, crd)

		original := &v1alpha1.DatadogAgentInternal{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "datadoghq.com/v1alpha1",
				Kind:       "DatadogAgentInternal",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
			},
			Spec: v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(true),
						Network: &v2alpha1.CWSNetworkConfig{
							Enabled: apiutils.NewBoolPointer(true),
						},
						Enforcement: &v2alpha1.CWSEnforcementConfig{
							Enabled: apiutils.NewBoolPointer(true),
						},
					},
				},
			},
		}

		modified := &v1alpha1.DatadogAgentInternal{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "datadoghq.com/v1alpha1",
				Kind:       "DatadogAgentInternal",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "foo",
				Namespace: "bar",
			},
			Spec: v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					CWS: &v2alpha1.CWSFeatureConfig{
						Enforcement: &v2alpha1.CWSEnforcementConfig{
							Enabled: apiutils.NewBoolPointer(false),
						},
					},
				},
			},
		}

		prunedOrig, prunedMod, err := pruneObjectsAgainstSchema(original, modified, crd)
		assert.NoError(t, err)
		assert.NotNil(t, prunedOrig)
		assert.NotNil(t, prunedMod)

		// Check that enforcement was pruned from original
		origDDAI, ok := prunedOrig.(*v1alpha1.DatadogAgentInternal)
		assert.True(t, ok)
		assert.NotNil(t, origDDAI.Spec.Features)
		assert.NotNil(t, origDDAI.Spec.Features.CWS)
		assert.NotNil(t, origDDAI.Spec.Features.CWS.Network, "known fields should remain")
		assert.Nil(t, origDDAI.Spec.Features.CWS.Enforcement, "unknown field should be pruned")

		// Check that enforcement was pruned from modified
		modDDAI, ok := prunedMod.(*v1alpha1.DatadogAgentInternal)
		assert.True(t, ok)
		assert.NotNil(t, modDDAI.Spec.Features)
		assert.NotNil(t, modDDAI.Spec.Features.CWS)
		assert.Nil(t, modDDAI.Spec.Features.CWS.Enforcement, "unknown field should be pruned")
	})

	t.Run("returns error for nil original", func(t *testing.T) {
		crd, err := getDDAICRDFromConfig(sch)
		assert.NoError(t, err)

		modified := &v1alpha1.DatadogAgentInternal{}
		_, _, err = pruneObjectsAgainstSchema(nil, modified, crd)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must not be nil")
	})

	t.Run("returns error for nil modified", func(t *testing.T) {
		crd, err := getDDAICRDFromConfig(sch)
		assert.NoError(t, err)

		original := &v1alpha1.DatadogAgentInternal{}
		_, _, err = pruneObjectsAgainstSchema(original, nil, crd)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "must not be nil")
	})

	t.Run("returns error for invalid version", func(t *testing.T) {
		crd, err := getDDAICRDFromConfig(sch)
		assert.NoError(t, err)

		original := &v1alpha1.DatadogAgentInternal{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "datadoghq.com/v999alpha999",
				Kind:       "DatadogAgentInternal",
			},
		}
		modified := &v1alpha1.DatadogAgentInternal{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "datadoghq.com/v999alpha999",
				Kind:       "DatadogAgentInternal",
			},
		}

		_, _, err = pruneObjectsAgainstSchema(original, modified, crd)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "version v999alpha999 not found")
	})

	t.Run("preserves known fields while pruning unknown ones", func(t *testing.T) {
		crd, err := getDDAICRDFromConfig(sch)
		assert.NoError(t, err)
		removeCwsEnforcementFromDDAISchema(t, crd)

		original := &v1alpha1.DatadogAgentInternal{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "datadoghq.com/v1alpha1",
				Kind:       "DatadogAgentInternal",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
			},
			Spec: v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					APM: &v2alpha1.APMFeatureConfig{
						Enabled: apiutils.NewBoolPointer(true),
					},
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(true),
						Enforcement: &v2alpha1.CWSEnforcementConfig{
							Enabled: apiutils.NewBoolPointer(true),
						},
					},
				},
			},
		}

		modified := &v1alpha1.DatadogAgentInternal{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "datadoghq.com/v1alpha1",
				Kind:       "DatadogAgentInternal",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "default",
			},
			Spec: v2alpha1.DatadogAgentSpec{},
		}

		prunedOrig, prunedMod, err := pruneObjectsAgainstSchema(original, modified, crd)
		assert.NoError(t, err)

		origDDAI, ok := prunedOrig.(*v1alpha1.DatadogAgentInternal)
		assert.True(t, ok)
		assert.NotNil(t, origDDAI.Spec.Features)
		assert.NotNil(t, origDDAI.Spec.Features.APM, "known field should be preserved")
		assert.True(t, *origDDAI.Spec.Features.APM.Enabled)
		assert.NotNil(t, origDDAI.Spec.Features.CWS, "known field should be preserved")
		assert.True(t, *origDDAI.Spec.Features.CWS.Enabled)
		assert.Nil(t, origDDAI.Spec.Features.CWS.Enforcement, "unknown field should be pruned")

		modDDAI, ok := prunedMod.(*v1alpha1.DatadogAgentInternal)
		assert.True(t, ok)
		assert.Nil(t, modDDAI.Spec.Features)
	})
}

func removeCwsEnforcementFromDDAISchema(t *testing.T, crd *apiextensionsv1.CustomResourceDefinition) {
	t.Helper()
	found := false
	for i := range crd.Spec.Versions {
		v := &crd.Spec.Versions[i]
		if v.Schema == nil || v.Schema.OpenAPIV3Schema == nil {
			continue
		}

		s := v.Schema.OpenAPIV3Schema
		spec, ok := s.Properties["spec"]
		if !ok {
			continue
		}
		features, ok := spec.Properties["features"]
		if !ok {
			continue
		}
		cws, ok := features.Properties["cws"]
		if !ok {
			continue
		}
		if _, ok := cws.Properties["enforcement"]; ok {
			delete(cws.Properties, "enforcement")
			features.Properties["cws"] = cws
			spec.Properties["features"] = features
			s.Properties["spec"] = spec
			v.Schema.OpenAPIV3Schema = s
			found = true
		}
	}
	assert.True(t, found, "expected to find and delete cws.enforcement schema")
}
