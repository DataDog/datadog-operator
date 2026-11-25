// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentprofile

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func testDaemonSet(name string, managed bool, affinity *corev1.Affinity) *appsv1.DaemonSet {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Namespace: "foo", Name: name},
		Spec: appsv1.DaemonSetSpec{Template: corev1.PodTemplateSpec{Spec: corev1.PodSpec{
			Affinity: affinity,
		}}},
	}
	if managed {
		ds.Labels = map[string]string{kubernetes.AppKubernetesManageByLabelKey: "datadog-operator"}
	}
	return ds
}

func requiredAffinity(key string, op corev1.NodeSelectorOperator, values ...string) *corev1.Affinity {
	return &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{{MatchExpressions: []corev1.NodeSelectorRequirement{
				{Key: "os", Operator: corev1.NodeSelectorOpIn, Values: []string{"linux"}},
				{Key: key, Operator: op, Values: values},
			}}},
		},
	}}
}

func TestLegacyProfileLabelInUse(t *testing.T) {
	sch := runtime.NewScheme()
	require.NoError(t, clientgoscheme.AddToScheme(sch))

	tests := []struct {
		name    string
		objects []client.Object
		want    bool
	}{
		{
			name: "no DaemonSets",
			want: false,
		},
		{
			name:    "old profile DaemonSet selects with the legacy label",
			objects: []client.Object{testDaemonSet("profile-agent", true, requiredAffinity(constants.ProfileLabelKey, corev1.NodeSelectorOpIn, "profile"))},
			want:    true,
		},
		{
			name:    "old default DaemonSet excludes the legacy label",
			objects: []client.Object{testDaemonSet("agent", true, requiredAffinity(constants.ProfileLabelKey, corev1.NodeSelectorOpDoesNotExist))},
			want:    true,
		},
		{
			name: "legacy label in a preferred term",
			objects: []client.Object{testDaemonSet("agent", true, &corev1.Affinity{NodeAffinity: &corev1.NodeAffinity{
				PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{{
					Weight: 1,
					Preference: corev1.NodeSelectorTerm{MatchExpressions: []corev1.NodeSelectorRequirement{
						{Key: constants.ProfileLabelKey, Operator: corev1.NodeSelectorOpExists},
					}},
				}},
			}})},
			want: true,
		},
		{
			name: "migrated DaemonSets use the namespace-scoped label",
			objects: []client.Object{
				testDaemonSet("profile-agent", true, requiredAffinity(NodeLabelKey("foo"), corev1.NodeSelectorOpIn, "profile")),
				testDaemonSet("agent", true, requiredAffinity(NodeLabelKey("foo"), corev1.NodeSelectorOpDoesNotExist)),
				testDaemonSet("no-affinity", true, nil),
			},
			want: false,
		},
		{
			name:    "DaemonSet not managed by the operator is ignored",
			objects: []client.Object{testDaemonSet("user-ds", false, requiredAffinity(constants.ProfileLabelKey, corev1.NodeSelectorOpIn, "profile"))},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fake.NewClientBuilder().WithScheme(sch).WithObjects(tt.objects...).Build()
			got, err := LegacyProfileLabelInUse(context.Background(), c)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
