// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/providers"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func node(name string, labels map[string]string) corev1.Node {
	return corev1.Node{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels}}
}

func gkeCosLabels() map[string]string {
	return map[string]string{kubernetes.GKEProviderLabel: kubernetes.GKECosType}
}

func defaultProfile() types.NamespacedName {
	return types.NamespacedName{Namespace: "", Name: "default"}
}

func TestComputeProfileProviders(t *testing.T) {
	tests := []struct {
		name          string
		nodeList      []corev1.Node
		nodeToProfile map[string]types.NamespacedName
		want          map[string][]string
	}{
		{
			name:          "no nodes",
			nodeList:      nil,
			nodeToProfile: map[string]types.NamespacedName{},
			want:          map[string][]string{},
		},
		{
			name:     "single node, no provider",
			nodeList: []corev1.Node{node("n1", nil)},
			nodeToProfile: map[string]types.NamespacedName{
				"n1": defaultProfile(),
			},
			want: map[string][]string{"default": {""}},
		},
		{
			name:     "single node, gke-cos provider",
			nodeList: []corev1.Node{node("n1", gkeCosLabels())},
			nodeToProfile: map[string]types.NamespacedName{
				"n1": defaultProfile(),
			},
			// "" is always included, even with no unmatched nodes.
			want: map[string][]string{"default": {"", kubernetes.GKECosProvider}},
		},
		{
			name: "same profile, mixed providers",
			nodeList: []corev1.Node{
				node("n1", gkeCosLabels()),
				node("n2", nil),
			},
			nodeToProfile: map[string]types.NamespacedName{
				"n1": defaultProfile(),
				"n2": defaultProfile(),
			},
			want: map[string][]string{"default": {"", kubernetes.GKECosProvider}},
		},
		{
			name: "different profiles, independent provider lists",
			nodeList: []corev1.Node{
				node("n1", gkeCosLabels()),
				node("n2", nil),
			},
			nodeToProfile: map[string]types.NamespacedName{
				"n1": {Namespace: "", Name: "profile-a"},
				"n2": {Namespace: "", Name: "profile-b"},
			},
			want: map[string][]string{
				"profile-a": {"", kubernetes.GKECosProvider},
				"profile-b": {""},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := computeProfileProviders(tt.nodeList, tt.nodeToProfile)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestApplyDDAIProvider(t *testing.T) {
	t.Run("no provider applies: ddai left untouched", func(t *testing.T) {
		ddai := &v1alpha1.DatadogAgentInternal{
			ObjectMeta: metav1.ObjectMeta{Name: "foo"},
		}
		before := ddai.DeepCopy()

		applyDDAIProvider(ddai, "", []string{""})

		assert.Equal(t, before, ddai)
	})

	t.Run("single provider, no split: annotation set, no affinity or name change", func(t *testing.T) {
		ddai := &v1alpha1.DatadogAgentInternal{
			ObjectMeta: metav1.ObjectMeta{Name: "foo"},
		}

		applyDDAIProvider(ddai, kubernetes.GKECosProvider, []string{kubernetes.GKECosProvider})

		assert.Equal(t, "foo", ddai.Name)
		assert.Equal(t, kubernetes.GKECosProvider, ddai.Annotations[kubernetes.NodeProviderAnnotationKey])
		assert.Nil(t, ddai.Spec.Override)
	})

	t.Run("multiple providers, provider branch: name suffixed, affinity and annotation set", func(t *testing.T) {
		dsName := "foo-agent"
		ddai := &v1alpha1.DatadogAgentInternal{
			ObjectMeta: metav1.ObjectMeta{Name: "foo"},
			Spec: v2alpha1.DatadogAgentSpec{
				Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
					v2alpha1.NodeAgentComponentName: {Name: &dsName},
				},
			},
		}
		providers := []string{"", kubernetes.GKECosProvider}

		applyDDAIProvider(ddai, kubernetes.GKECosProvider, providers)

		assert.Equal(t, "foo-"+kubernetes.GKECosProvider, ddai.Name)
		assert.Equal(t, "foo-agent-"+kubernetes.GKECosProvider, *ddai.Spec.Override[v2alpha1.NodeAgentComponentName].Name)
		assert.Equal(t, kubernetes.GKECosProvider, ddai.Annotations[kubernetes.NodeProviderAnnotationKey])
		require.NotNil(t, ddai.Spec.Override[v2alpha1.NodeAgentComponentName].Affinity)
	})

	t.Run("multiple providers, remainder branch: no name change or annotation, affinity still set", func(t *testing.T) {
		ddai := &v1alpha1.DatadogAgentInternal{
			ObjectMeta: metav1.ObjectMeta{Name: "foo"},
		}
		providers := []string{"", kubernetes.GKECosProvider}

		applyDDAIProvider(ddai, "", providers)

		assert.Equal(t, "foo", ddai.Name)
		assert.Empty(t, ddai.Annotations[kubernetes.NodeProviderAnnotationKey])
		require.NotNil(t, ddai.Spec.Override[v2alpha1.NodeAgentComponentName].Affinity)
	})
}

func TestProviderAffinity(t *testing.T) {
	t.Run("specific provider: In rule matching its label", func(t *testing.T) {
		affinity := providerAffinity(kubernetes.GKECosProvider, []string{"", kubernetes.GKECosProvider})
		require.NotNil(t, affinity)
		terms := affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
		require.Len(t, terms, 1)
		require.Len(t, terms[0].MatchExpressions, 1)
		expr := terms[0].MatchExpressions[0]
		assert.Equal(t, kubernetes.GKEProviderLabel, expr.Key)
		assert.Equal(t, corev1.NodeSelectorOpIn, expr.Operator)
		assert.Equal(t, []string{kubernetes.GKECosType}, expr.Values)
	})

	t.Run("unknown provider: no rule, nil affinity", func(t *testing.T) {
		affinity := providerAffinity("not-a-real-provider", []string{"not-a-real-provider"})
		assert.Nil(t, affinity)
	})

	t.Run("empty provider: NotIn rule excluding every other provider", func(t *testing.T) {
		affinity := providerAffinity("", []string{"", kubernetes.GKECosProvider})
		require.NotNil(t, affinity)
		terms := affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms
		require.Len(t, terms, 1)
		require.Len(t, terms[0].MatchExpressions, 1)
		expr := terms[0].MatchExpressions[0]
		assert.Equal(t, kubernetes.GKEProviderLabel, expr.Key)
		assert.Equal(t, corev1.NodeSelectorOpNotIn, expr.Operator)
		assert.Equal(t, []string{kubernetes.GKECosType}, expr.Values)
	})

	t.Run("empty provider, no other real providers: no match expressions, nil affinity", func(t *testing.T) {
		affinity := providerAffinity("", []string{""})
		assert.Nil(t, affinity)
	})
}

func TestApplyIntrospection(t *testing.T) {
	t.Run("no node-scope provider anywhere: ddais unchanged", func(t *testing.T) {
		nodeList := []corev1.Node{node("n1", nil)}
		nodeToProfile := map[string]types.NamespacedName{"n1": defaultProfile()}
		ddais := []*v1alpha1.DatadogAgentInternal{
			{ObjectMeta: metav1.ObjectMeta{Name: "foo"}},
		}

		r := &Reconciler{}
		got := r.applyIntrospection(nodeList, nodeToProfile, ddais)

		require.Len(t, got, 1)
		assert.Equal(t, "foo", got[0].Name)
		assert.Empty(t, got[0].Annotations[kubernetes.NodeProviderAnnotationKey])
	})

	t.Run("profile's nodes span two providers: ddai splits in two", func(t *testing.T) {
		nodeList := []corev1.Node{
			node("n1", gkeCosLabels()),
			node("n2", nil),
		}
		nodeToProfile := map[string]types.NamespacedName{
			"n1": defaultProfile(),
			"n2": defaultProfile(),
		}
		ddais := []*v1alpha1.DatadogAgentInternal{
			{ObjectMeta: metav1.ObjectMeta{Name: "foo"}},
		}

		r := &Reconciler{}
		got := r.applyIntrospection(nodeList, nodeToProfile, ddais)

		require.Len(t, got, 2)
		names := map[string]bool{}
		for _, ddai := range got {
			names[ddai.Name] = true
		}
		assert.True(t, names["foo"])
		assert.True(t, names["foo-"+kubernetes.GKECosProvider])
	})

	t.Run("splits independently per profile via ProfileLabelKey", func(t *testing.T) {
		nodeList := []corev1.Node{
			node("n1", gkeCosLabels()),
			node("n2", nil),
			node("n3", nil),
		}
		nodeToProfile := map[string]types.NamespacedName{
			"n1": {Namespace: "", Name: "profile-a"},
			"n2": {Namespace: "", Name: "profile-a"},
			"n3": defaultProfile(),
		}
		ddais := []*v1alpha1.DatadogAgentInternal{
			{ObjectMeta: metav1.ObjectMeta{Name: "foo"}},
			{ObjectMeta: metav1.ObjectMeta{
				Name:   "foo-profile-a",
				Labels: map[string]string{constants.ProfileLabelKey: "profile-a"},
			}},
		}

		r := &Reconciler{}
		got := r.applyIntrospection(nodeList, nodeToProfile, ddais)

		// "foo" (default profile, only n3, no provider) stays a single DDAI.
		// "foo-profile-a" (profile-a, spans gke-cos and no-provider) splits in two.
		require.Len(t, got, 3)
		names := map[string]bool{}
		for _, ddai := range got {
			names[ddai.Name] = true
		}
		assert.True(t, names["foo"])
		assert.True(t, names["foo-profile-a"])
		assert.True(t, names["foo-profile-a-"+kubernetes.GKECosProvider])
	})

	t.Run("splits carry distinct, correct affinity: no aliasing between splits", func(t *testing.T) {
		nodeList := []corev1.Node{
			node("n1", gkeCosLabels()),
			node("n2", nil),
		}
		nodeToProfile := map[string]types.NamespacedName{
			"n1": defaultProfile(),
			"n2": defaultProfile(),
		}
		ddais := []*v1alpha1.DatadogAgentInternal{
			{ObjectMeta: metav1.ObjectMeta{Name: "foo"}},
		}

		r := &Reconciler{}
		got := r.applyIntrospection(nodeList, nodeToProfile, ddais)

		require.Len(t, got, 2)
		byName := make(map[string]*v1alpha1.DatadogAgentInternal, len(got))
		for _, ddai := range got {
			byName[ddai.Name] = ddai
		}

		base := byName["foo"]
		require.NotNil(t, base)
		baseAffinity := base.Spec.Override[v2alpha1.NodeAgentComponentName].Affinity
		require.NotNil(t, baseAffinity)
		baseExprs := baseAffinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions
		require.Len(t, baseExprs, 1)
		assert.Equal(t, kubernetes.GKEProviderLabel, baseExprs[0].Key)
		assert.Equal(t, corev1.NodeSelectorOpNotIn, baseExprs[0].Operator)
		assert.Equal(t, []string{kubernetes.GKECosType}, baseExprs[0].Values)

		cos := byName["foo-"+kubernetes.GKECosProvider]
		require.NotNil(t, cos)
		cosAffinity := cos.Spec.Override[v2alpha1.NodeAgentComponentName].Affinity
		require.NotNil(t, cosAffinity)
		cosExprs := cosAffinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms[0].MatchExpressions
		require.Len(t, cosExprs, 1)
		assert.Equal(t, kubernetes.GKEProviderLabel, cosExprs[0].Key)
		assert.Equal(t, corev1.NodeSelectorOpIn, cosExprs[0].Operator)
		assert.Equal(t, []string{kubernetes.GKECosType}, cosExprs[0].Values)

		// Splits must not share the same Affinity object.
		assert.NotSame(t, baseAffinity, cosAffinity)
	})

	t.Run("only the first output for a ddai reuses the original object; the rest are deep copies", func(t *testing.T) {
		nodeList := []corev1.Node{
			node("n1", gkeCosLabels()),
			node("n2", nil),
		}
		nodeToProfile := map[string]types.NamespacedName{
			"n1": defaultProfile(),
			"n2": defaultProfile(),
		}
		original := &v1alpha1.DatadogAgentInternal{ObjectMeta: metav1.ObjectMeta{Name: "foo"}}
		ddais := []*v1alpha1.DatadogAgentInternal{original}

		r := &Reconciler{}
		got := r.applyIntrospection(nodeList, nodeToProfile, ddais)

		require.Len(t, got, 2)
		var sawOriginal bool
		for _, ddai := range got {
			if ddai == original {
				sawOriginal = true
			}
		}
		assert.True(t, sawOriginal, "expected one of the results to be the original ddai pointer")
	})
}

func TestProviderFeatureRulesHaveSetters(t *testing.T) {
	for name, provider := range providers.All() {
		for id := range provider.Features() {
			t.Run(string(name)+"/"+id, func(t *testing.T) {
				assert.NoError(t, feature.SetEnabled(&v2alpha1.DatadogAgentSpec{}, feature.IDType(id), false),
					"provider %s has a rule for %s, so that feature must register an enabled setter in its own package", name, id)
			})
		}
	}
}

func TestSetEnabled(t *testing.T) {
	tests := []struct {
		name    string
		id      feature.IDType
		enabled func(*v2alpha1.DatadogFeatures) *bool
		wantErr bool
	}{
		{
			name:    "oom kill",
			id:      feature.OOMKillIDType,
			enabled: func(f *v2alpha1.DatadogFeatures) *bool { return f.OOMKill.Enabled },
		},
		{
			name:    "tcp queue length",
			id:      feature.TCPQueueLengthIDType,
			enabled: func(f *v2alpha1.DatadogFeatures) *bool { return f.TCPQueueLength.Enabled },
		},
		{
			name:    "ebpf check",
			id:      feature.EBPFCheckIDType,
			enabled: func(f *v2alpha1.DatadogFeatures) *bool { return f.EBPFCheck.Enabled },
		},
		{
			name:    "a feature with no setter fails",
			id:      feature.NPMIDType,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := &v2alpha1.DatadogAgentSpec{}
			err := feature.SetEnabled(spec, tt.id, false)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, spec.Features, "a failed set leaves the spec alone")
				return
			}
			require.NoError(t, err)
			require.NotNil(t, spec.Features)
			assert.Equal(t, new(false), tt.enabled(spec.Features))
		})
	}
}
