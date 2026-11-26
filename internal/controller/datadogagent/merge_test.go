package datadogagent

import (
	"context"
	"errors"
	"testing"

	"k8s.io/utils/ptr"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/stretchr/testify/assert"
)

// computeProfileMergeSingle merges a single profile, validating the source DDAI
// schema itself instead of reusing the per-batch check from applyProfilesToDDAISpec.
func (r *Reconciler) computeProfileMergeSingle(ctx context.Context, ddai *v1alpha1.DatadogAgentInternal, profile *v1alpha1.DatadogAgentProfile) (*v1alpha1.DatadogAgentInternal, error) {
	return r.computeProfileMerge(ctx, ddai, profile, r.isDDAISchemaValid(ddai), nil)
}

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
							Enabled: ptr.To(true),
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
							Enabled: ptr.To(true),
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
							Enabled: ptr.To(true),
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
							Enabled: ptr.To(true),
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
		eventBroadcaster := record.NewBroadcaster()
		recorder := eventBroadcaster.NewRecorder(sch, corev1.EventSource{Component: "Test_ssaMergeCRD"})
		fieldManager, err := newFieldManager(context.Background(), fakeClient, sch, v1alpha1.GroupVersion.WithKind("DatadogAgentInternal"))
		assert.NoError(t, err)

		t.Run(tt.name, func(t *testing.T) {
			r := &Reconciler{
				client:       fakeClient,
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

			ddai, err := r.ssaMergeCRD(context.Background(), &tt.ddai, &tt.profile)
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
					Enabled: ptr.To(true),
					Enforcement: &v2alpha1.CWSEnforcementConfig{
						Enabled: ptr.To(true),
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
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(sch, corev1.EventSource{Component: "Test_ssaMergeCRD_OutdatedCRD_IgnoresUnknownFields"})
	fieldManager, err := newFieldManager(context.Background(), fakeClient, sch, v1alpha1.GroupVersion.WithKind("DatadogAgentInternal"))
	assert.NoError(t, err)

	r := &Reconciler{
		client:       fakeClient,
		scheme:       sch,
		recorder:     recorder,
		fieldManager: fieldManager,
	}

	ctx, checkLogs := tracedLogContext(t)
	merged, err := r.ssaMergeCRD(ctx, &ddai, &profile)
	checkLogs()
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
						Enabled: ptr.To(true),
						Network: &v2alpha1.CWSNetworkConfig{
							Enabled: ptr.To(true),
						},
						Enforcement: &v2alpha1.CWSEnforcementConfig{
							Enabled: ptr.To(true),
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
						Enabled: ptr.To(true),
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

func Test_computeProfileMerge_EquivalentToFieldManagerApply(t *testing.T) {
	sch := newBenchScheme()
	crd, err := getDDAICRDFromConfig(sch)
	assert.NoError(t, err)

	baseDDAI := newBenchBaseDDAI()
	fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(baseDDAI, crd).Build()
	recorder := record.NewBroadcaster().NewRecorder(sch, corev1.EventSource{Component: "equiv"})
	fm, err := newFieldManager(context.Background(), fakeClient, sch, getDDAIGVK())
	assert.NoError(t, err)

	r := &Reconciler{
		client:       fakeClient,
		scheme:       sch,
		recorder:     recorder,
		fieldManager: fm,
	}

	profiles := newBenchProfiles(4)
	// Add a profile that overrides Global + Features + ClusterAgent as well.
	profiles = append(profiles, &v1alpha1.DatadogAgentProfile{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "profile-complex",
			Namespace: "datadog-agent",
			Annotations: map[string]string{
				"agent.datadoghq.com/provider": "gke-cos",
			},
		},
		Spec: v1alpha1.DatadogAgentProfileSpec{
			ProfileAffinity: &v1alpha1.ProfileAffinity{
				ProfileNodeAffinity: []corev1.NodeSelectorRequirement{
					{Key: "pool", Operator: corev1.NodeSelectorOpIn, Values: []string{"complex"}},
				},
			},
			Config: &v2alpha1.DatadogAgentSpec{
				Global: &v2alpha1.GlobalConfig{
					Tags: []string{"custom:tag"},
					CommonLabels: map[string]string{
						"team": "override-team",
					},
				},
				Features: &v2alpha1.DatadogFeatures{
					APM: &v2alpha1.APMFeatureConfig{Enabled: ptr.To(false)},
				},
				Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.NodeAgentComponentName: {
						Env: []corev1.EnvVar{
							{Name: "DD_CUSTOM_VAR_01", Value: "replaced"},
							{Name: "DD_NEW_VAR", Value: "added"},
						},
					},
				},
			},
		},
	})

	// Profiles that configure components which are auto-disabled for user
	// profiles: the subtree merge handles these separately from the nodeAgent.
	profiles = append(profiles, &v1alpha1.DatadogAgentProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "profile-auto-disabled-overrides", Namespace: "datadog-agent"},
		Spec: v1alpha1.DatadogAgentProfileSpec{
			ProfileAffinity: &v1alpha1.ProfileAffinity{
				ProfileNodeAffinity: []corev1.NodeSelectorRequirement{
					{Key: "pool", Operator: corev1.NodeSelectorOpIn, Values: []string{"auto-disabled"}},
				},
			},
			Config: &v2alpha1.DatadogAgentSpec{
				Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.ClusterAgentComponentName: {
						Replicas: ptr.To[int32](3),
						Env:      []corev1.EnvVar{{Name: "DD_DCA_VAR", Value: "profile"}},
					},
					v2alpha1.ClusterChecksRunnerComponentName: {
						Disabled: ptr.To(false),
					},
				},
			},
		},
	}, &v1alpha1.DatadogAgentProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "profile-features-only", Namespace: "datadog-agent"},
		Spec: v1alpha1.DatadogAgentProfileSpec{
			ProfileAffinity: &v1alpha1.ProfileAffinity{
				ProfileNodeAffinity: []corev1.NodeSelectorRequirement{
					{Key: "pool", Operator: corev1.NodeSelectorOpIn, Values: []string{"features-only"}},
				},
			},
			Config: &v2alpha1.DatadogAgentSpec{
				Features: &v2alpha1.DatadogFeatures{
					NPM: &v2alpha1.NPMFeatureConfig{Enabled: ptr.To(true)},
				},
			},
		},
	})

	ctx := context.Background()
	for _, p := range profiles {
		t.Run(p.Name, func(t *testing.T) {
			// Reference output using upstream FieldManager.Apply directly.
			refProfileDDAI := baseDDAI.DeepCopy()
			refBaseDDAI := baseDDAI.DeepCopy()
			pCopy := p.DeepCopy()
			setProfileSpec(refProfileDDAI, pCopy)
			assert.NoError(t, setProfileDDAIMeta(refProfileDDAI, pCopy))
			refBaseDDAI.GetObjectKind().SetGroupVersionKind(getDDAIGVK())
			refProfileDDAI.GetObjectKind().SetGroupVersionKind(getDDAIGVK())

			refRaw, err := fm.fieldManager.Apply(refBaseDDAI, refProfileDDAI, defaultOperatorManager, true)
			assert.NoError(t, err)
			refObj := refRaw.(*v1alpha1.DatadogAgentInternal)
			refObj.ManagedFields = nil
			refSpecMD5, err := comparison.GenerateMD5ForSpec(refObj.Spec)
			assert.NoError(t, err)
			refObj.ObjectMeta.Annotations[constants.MD5DDAIDeploymentAnnotationKey] = refSpecMD5

			gotObj, err := r.computeProfileMergeSingle(ctx, baseDDAI, p)
			assert.NoError(t, err)

			// Verify identical Name, Namespace, Labels, Finalizers, Annotations (including MD5 hash), and semantically identical Spec.
			assert.Equal(t, refObj.Name, gotObj.Name)
			assert.Equal(t, refObj.Namespace, gotObj.Namespace)
			assert.Equal(t, refObj.Labels, gotObj.Labels)
			assert.Equal(t, refObj.Finalizers, gotObj.Finalizers)
			assert.Equal(t, refObj.Annotations, gotObj.Annotations)
			assert.True(t, apiequality.Semantic.DeepEqual(refObj.Spec, gotObj.Spec), "merged spec mismatch for profile %s", p.Name)
		})
	}
}

func Test_applyProfilesToDDAISpec_CacheCorrectnessAndEviction(t *testing.T) {
	sch := newBenchScheme()
	crd, err := getDDAICRDFromConfig(sch)
	assert.NoError(t, err)

	baseDDAI := newBenchBaseDDAI()
	fakeClient := fake.NewClientBuilder().WithScheme(sch).WithObjects(baseDDAI, crd).Build()
	recorder := record.NewBroadcaster().NewRecorder(sch, corev1.EventSource{Component: "cache-test"})
	fm, err := newFieldManager(context.Background(), fakeClient, sch, getDDAIGVK())
	assert.NoError(t, err)

	r := &Reconciler{
		client:       fakeClient,
		scheme:       sch,
		recorder:     recorder,
		fieldManager: fm,
	}

	ctx := context.Background()
	profiles := newBenchProfiles(3) // default + 2 user profiles

	// 1. Cold call populates cache
	ddais1, err := r.applyProfilesToDDAISpec(ctx, baseDDAI, baseDDAI.DeepCopy(), profiles)
	assert.NoError(t, err)
	assert.Len(t, ddais1, 3)
	assert.Len(t, r.profileMergeCache, 3)

	// 2. Mutating returned objects must not corrupt cached entries
	origRV := ddais1[1].ResourceVersion
	ddais1[1].ResourceVersion = "mutated-rv"
	ddais1[1].Spec.Override[v2alpha1.NodeAgentComponentName].Env[0].Value = "mutated-val"

	// 3. Steady-state call hits cache and returns clean copy
	ddais2, err := r.applyProfilesToDDAISpec(ctx, baseDDAI, baseDDAI.DeepCopy(), profiles)
	assert.NoError(t, err)
	assert.Len(t, ddais2, 3)
	assert.Equal(t, origRV, ddais2[1].ResourceVersion)
	assert.Equal(t, "override-01", ddais2[1].Spec.Override[v2alpha1.NodeAgentComponentName].Env[0].Value)

	// 4. Updating profile ResourceVersion invalidates that profile's cache entry
	profilesUpdated := []*v1alpha1.DatadogAgentProfile{profiles[0], profiles[1].DeepCopy(), profiles[2]}
	profilesUpdated[1].ResourceVersion = "9999"
	profilesUpdated[1].Spec.Config.Override[v2alpha1.NodeAgentComponentName].Env[0].Value = "updated-override"
	ddais3, err := r.applyProfilesToDDAISpec(ctx, baseDDAI, baseDDAI.DeepCopy(), profilesUpdated)
	assert.NoError(t, err)
	assert.Equal(t, "updated-override", ddais3[1].Spec.Override[v2alpha1.NodeAgentComponentName].Env[0].Value)

	// 5. Updating baseDDAI spec invalidates all entries
	baseModified := baseDDAI.DeepCopy()
	baseModified.Spec.Global.ClusterName = ptr.To("new-cluster-name")
	ddais4, err := r.applyProfilesToDDAISpec(ctx, baseModified, baseModified.DeepCopy(), profilesUpdated)
	assert.NoError(t, err)
	assert.Equal(t, "new-cluster-name", *ddais4[0].Spec.Global.ClusterName)
	assert.Equal(t, "new-cluster-name", *ddais4[1].Spec.Global.ClusterName)

	// 6. Removing a profile evicts its cache entry
	ddais5, err := r.applyProfilesToDDAISpec(ctx, baseModified, baseModified.DeepCopy(), profilesUpdated[:2])
	assert.NoError(t, err)
	assert.Len(t, ddais5, 2)
	assert.Len(t, r.profileMergeCache, 2)
}
