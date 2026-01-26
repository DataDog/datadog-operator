package datadogagent

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/pkg/constants"
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

func Test_extractMissingSchemaPaths(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want []string
	}{
		{
			name: "nil error",
			err:  nil,
			want: nil,
		},
		{
			name: "no match",
			err:  errors.New("some other error"),
			want: nil,
		},
		{
			name: "single match",
			err:  errors.New(".spec.features.cws.enforcement: field not declared in schema"),
			want: []string{".spec.features.cws.enforcement"},
		},
		{
			name: "multiple matches with duplicates preserves first-seen order",
			err: errors.New(
				".spec.features.cws.enforcement: field not declared in schema; " +
					".spec.features.foo_bar-1: field not declared in schema; " +
					".spec.features.cws.enforcement: field not declared in schema",
			),
			want: []string{".spec.features.cws.enforcement", ".spec.features.foo_bar-1"},
		},
		{
			name: "does not match single-segment path",
			err:  errors.New(".spec: field not declared in schema"),
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractMissingSchemaPaths(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

type badRuntimeObject struct {
	metav1.TypeMeta `json:",inline"`
	Bad             func()
}

func (b *badRuntimeObject) DeepCopyObject() k8sruntime.Object {
	return &badRuntimeObject{TypeMeta: b.TypeMeta}
}

func Test_stripDottedFieldPath(t *testing.T) {
	t.Run("nil object", func(t *testing.T) {
		assert.NoError(t, stripDottedFieldPath(nil, ".spec.features.cws.enforcement"))
	})

	t.Run("empty path is no-op", func(t *testing.T) {
		o := &v1alpha1.DatadogAgentInternal{}
		assert.NoError(t, stripDottedFieldPath(o, ""))
	})

	t.Run("invalid path without leading dot", func(t *testing.T) {
		o := &v1alpha1.DatadogAgentInternal{}
		assert.Error(t, stripDottedFieldPath(o, "spec.features.cws.enforcement"))
	})

	t.Run("invalid path with empty segment", func(t *testing.T) {
		o := &v1alpha1.DatadogAgentInternal{}
		assert.Error(t, stripDottedFieldPath(o, ".spec..features"))
	})

	t.Run("conversion error bubbles up", func(t *testing.T) {
		o := &badRuntimeObject{Bad: func() {}}
		assert.Error(t, stripDottedFieldPath(o, ".spec.features.cws.enforcement"))
	})

	t.Run("removes only the targeted nested field", func(t *testing.T) {
		ddai := &v1alpha1.DatadogAgentInternal{
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

		err := stripDottedFieldPath(ddai, ".spec.features.cws.enforcement")
		assert.NoError(t, err)
		assert.NotNil(t, ddai.Spec.Features)
		assert.NotNil(t, ddai.Spec.Features.CWS)
		assert.NotNil(t, ddai.Spec.Features.CWS.Network, "sibling fields should remain")
		assert.Nil(t, ddai.Spec.Features.CWS.Enforcement, "targeted field should be removed")
	})

	t.Run("removing non-existent field is a no-op", func(t *testing.T) {
		ddai := &v1alpha1.DatadogAgentInternal{
			Spec: v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					CWS: &v2alpha1.CWSFeatureConfig{
						Enabled: apiutils.NewBoolPointer(true),
					},
				},
			},
		}

		assert.NoError(t, stripDottedFieldPath(ddai, ".spec.features.cws.doesNotExist"))
		assert.NotNil(t, ddai.Spec.Features)
		assert.NotNil(t, ddai.Spec.Features.CWS)
		assert.Equal(t, true, apiutils.BoolValue(ddai.Spec.Features.CWS.Enabled))
	})
}

func Test_objectIdentityKV(t *testing.T) {
	t.Run("nil object", func(t *testing.T) {
		kv := objectIdentityKV("original", nil)
		assert.Equal(t, []any{"original_nil", true}, kv)
	})

	t.Run("meta accessor error", func(t *testing.T) {
		// runtime.Object but not metav1.Object, so meta.Accessor should fail
		o := &badRuntimeObject{}
		_, accessorErr := meta.Accessor(o)
		assert.Error(t, accessorErr)

		kv := objectIdentityKV("obj", o)
		assert.Equal(t, []any{
			"obj_gvk", o.GetObjectKind().GroupVersionKind().String(),
			"obj_metaError", accessorErr.Error(),
		}, kv)
	})

	t.Run("includes name/namespace/uid and selected labels", func(t *testing.T) {
		ddai := &v1alpha1.DatadogAgentInternal{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "datadoghq.com/v1alpha1",
				Kind:       "DatadogAgentInternal",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "ddai-name",
				Namespace: "ddai-ns",
				UID:       types.UID("uid-123"),
				Labels: map[string]string{
					constants.ProfileLabelKey:          "profile-a",
					apicommon.DatadogAgentNameLabelKey: "dda-name",
				},
			},
		}
		ddai.GetObjectKind().SetGroupVersionKind(getDDAIGVK())

		kv := objectIdentityKV("ddai", ddai)
		assert.Equal(t, []any{
			"ddai_gvk", getDDAIGVK().String(),
			"ddai_namespace", "ddai-ns",
			"ddai_name", "ddai-name",
			"ddai_uid", "uid-123",
			"ddai_profile", "profile-a",
			"ddai_datadogAgent", "dda-name",
		}, kv)
	})
}
