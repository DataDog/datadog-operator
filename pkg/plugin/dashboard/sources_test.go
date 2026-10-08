// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dashboard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestSources(t *testing.T) {
	keys := map[string]bool{}
	for _, src := range Sources() {
		assert.False(t, keys[src.Key], "duplicate key %s", src.Key)
		keys[src.Key] = true
		assert.NotNil(t, src.Transform, src.Key)
		assert.NotEmpty(t, src.GVR.Resource, src.Key)
	}
	assert.Len(t, keys, 10+len(SummaryKinds))
	assert.Len(t, SummaryKinds, 8)
	for _, k := range []string{SourceDDA, SourceDDAI, SourceDAP} {
		assert.True(t, keys[k])
	}
}

func TestStripObject(t *testing.T) {
	u := &unstructured.Unstructured{}
	u.SetManagedFields([]metav1.ManagedFieldsEntry{{Manager: "kubectl"}})
	u.SetAnnotations(map[string]string{lastAppliedAnnotation: "{}", "keep": "me"})
	out, err := StripObject(u)
	require.NoError(t, err)
	assert.Empty(t, out.(*unstructured.Unstructured).GetManagedFields())
	assert.Equal(t, map[string]string{"keep": "me"}, out.(*unstructured.Unstructured).GetAnnotations())

	other, err := StripObject("not an object")
	require.NoError(t, err)
	assert.Equal(t, "not an object", other)

	crev := &unstructured.Unstructured{Object: map[string]any{"revision": int64(1), "data": map[string]any{"spec": "x"}}}
	out, err = StripControllerRevision(crev)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"revision": int64(1)}, out.(*unstructured.Unstructured).Object)
}

func TestStripPod(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: "p", Namespace: "ns", Labels: map[string]string{"a": "b"},
			Annotations:   map[string]string{"big": "annotation"},
			ManagedFields: []metav1.ManagedFieldsEntry{{Manager: "x"}},
		},
		Spec: corev1.PodSpec{NodeName: "n1", Containers: []corev1.Container{{Name: "c", Image: "i"}}},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue}},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "c", Image: "i", ImageID: "sha", RestartCount: 3,
				State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
			}},
		},
	}
	out, err := StripPod(pod)
	require.NoError(t, err)
	got := out.(*corev1.Pod)
	assert.Equal(t, "n1", got.Spec.NodeName)
	assert.Empty(t, got.Spec.Containers)
	assert.Nil(t, got.Annotations)
	assert.Nil(t, got.ManagedFields)
	assert.Equal(t, map[string]string{"a": "b"}, got.Labels)
	assert.Equal(t, pod.Status.Conditions, got.Status.Conditions)
	assert.Equal(t, []corev1.ContainerStatus{{Name: "c", RestartCount: 3, State: pod.Status.ContainerStatuses[0].State}}, got.Status.ContainerStatuses)
}

func TestLoadFixture(t *testing.T) {
	s := loadScenario(t, "steady")
	assert.Equal(t, testNow, s.Taken)
	assert.Len(t, s.Objects[SourceDDA], 1)
	assert.Len(t, s.Objects[SourceDeployments], 2)
	assert.Len(t, s.Objects[SourceOperator], 1, "operator Deployment routed by label")
	assert.Len(t, s.Objects[SourceControllerRevisions], 2)
	assert.Nil(t, s.Pods)
	assert.NotContains(t, s.Sources, SourcePods)
	assert.Equal(t, SourceNotInstalled, s.Sources["sum/DatadogMonitor"].State)
	assert.Equal(t, SourceInfo{State: SourceOK, Mode: RefreshOnce, LastOK: testNow}, s.Sources[SourceDDA])

	// Transforms are applied.
	dda := s.Objects[SourceDDA][0]
	assert.Empty(t, dda.GetManagedFields())
	assert.NotContains(t, dda.GetAnnotations(), lastAppliedAnnotation)
	_, found := s.Objects[SourceControllerRevisions][0].Object["data"]
	assert.False(t, found)
	rev, _, _ := unstructured.NestedInt64(s.Objects[SourceControllerRevisions][0].Object, "revision")
	assert.NotZero(t, rev, "integers decoded as int64")

	s = loadScenario(t, "stalled")
	require.Len(t, s.Pods, 4)
	assert.Equal(t, SourceOK, s.Sources[SourcePods].State)
	assert.Empty(t, s.Pods[0].Status.ContainerStatuses[0].ImageID, "pods stripped")

	s = loadScenario(t, "summary")
	assert.Equal(t, SourceForbidden, s.Sources["sum/DatadogSLO"].State)
	assert.True(t, s.Sources["sum/DatadogSLO"].LastOK.IsZero())
	assert.Equal(t, RefreshPoll, s.Sources["sum/DatadogMonitor"].Mode)

	_, err := LoadFixture(t.TempDir())
	require.Error(t, err)
}

func TestHelpers(t *testing.T) {
	for image, want := range map[string]string{
		"gcr.io/datadoghq/operator:1.22.0":               "1.22.0",
		"localhost:5000/operator":                        "",
		"localhost:5000/operator:dev":                    "dev",
		"operator:1.2@sha256:abc":                        "1.2",
		"gcr.io/datadoghq/operator@sha256:0123456789abc": "",
	} {
		assert.Equal(t, want, imageTag(image), image)
	}
	for label, want := range map[string][2]string{
		"datadog-3.70.0":              {"datadog", "3.70.0"},
		"datadog-operator-2.1.0-rc.1": {"datadog-operator", "2.1.0-rc.1"},
		"datadog-crds-1.0.0_build.5":  {"datadog-crds", "1.0.0+build.5"},
		"nochart":                     {"nochart", ""},
		"my-chart-v2-3.1.0":           {"my-chart-v2", "3.1.0"},
	} {
		chart, version := parseChartLabel(label)
		assert.Equal(t, want, [2]string{chart, version}, label)
	}
	ref, ok := HelmRelease(&metav1.ObjectMeta{Namespace: "ns", Annotations: map[string]string{helmReleaseNameAnnotation: "rel"}})
	assert.True(t, ok)
	assert.Equal(t, ReleaseRef{Namespace: "ns", Name: "rel"}, ref)
	assert.Equal(t, "ns/rel", ref.Key())
}
