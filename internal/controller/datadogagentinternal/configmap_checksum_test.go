// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/DataDog/datadog-operator/pkg/constants"
)

func newChecksumTestReconciler(objs ...client.Object) *Reconciler {
	sch := runtime.NewScheme()
	_ = scheme.AddToScheme(sch)
	return &Reconciler{client: fake.NewClientBuilder().WithScheme(sch).WithObjects(objs...).Build()}
}

func configMapVolume(name string) corev1.Volume {
	return corev1.Volume{
		Name: name,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: name},
			},
		},
	}
}

func Test_annotateConfigMapsChecksum_SetsAnnotation(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "my-config", Namespace: "ns-1"},
		Data:       map[string]string{"foo": "bar"},
	}
	r := newChecksumTestReconciler(cm)
	podTmpl := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{Volumes: []corev1.Volume{configMapVolume("my-config")}},
	}

	err := r.annotateConfigMapsChecksum(context.Background(), "ns-1", podTmpl)
	require.NoError(t, err)

	wantHash := hashConfigMapContents(map[string]configMapContent{
		"my-config": {Data: map[string]string{"foo": "bar"}},
	})
	assert.Equal(t, wantHash, podTmpl.Annotations[constants.ConfigMapsChecksumAnnotationKey])
}

func Test_annotateConfigMapsChecksum_NoConfigMapVolumes(t *testing.T) {
	r := newChecksumTestReconciler()
	podTmpl := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{Volumes: []corev1.Volume{{Name: "scratch", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}}},
	}

	err := r.annotateConfigMapsChecksum(context.Background(), "ns-1", podTmpl)
	require.NoError(t, err)

	_, ok := podTmpl.Annotations[constants.ConfigMapsChecksumAnnotationKey]
	assert.False(t, ok)
}

func Test_annotateConfigMapsChecksum_MissingConfigMap(t *testing.T) {
	r := newChecksumTestReconciler()
	podTmpl := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{Volumes: []corev1.Volume{configMapVolume("does-not-exist")}},
	}

	err := r.annotateConfigMapsChecksum(context.Background(), "ns-1", podTmpl)
	require.NoError(t, err)

	_, ok := podTmpl.Annotations[constants.ConfigMapsChecksumAnnotationKey]
	assert.False(t, ok)
}

func Test_annotateConfigMapsChecksum_DeterministicOrdering(t *testing.T) {
	cmA := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm-a", Namespace: "ns-1"}, Data: map[string]string{"a": "1"}}
	cmB := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "cm-b", Namespace: "ns-1"}, Data: map[string]string{"b": "2"}}
	r := newChecksumTestReconciler(cmA, cmB)

	podTmplAB := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{Volumes: []corev1.Volume{configMapVolume("cm-a"), configMapVolume("cm-b")}},
	}
	podTmplBA := &corev1.PodTemplateSpec{
		Spec: corev1.PodSpec{Volumes: []corev1.Volume{configMapVolume("cm-b"), configMapVolume("cm-a")}},
	}

	require.NoError(t, r.annotateConfigMapsChecksum(context.Background(), "ns-1", podTmplAB))
	require.NoError(t, r.annotateConfigMapsChecksum(context.Background(), "ns-1", podTmplBA))

	assert.Equal(t,
		podTmplAB.Annotations[constants.ConfigMapsChecksumAnnotationKey],
		podTmplBA.Annotations[constants.ConfigMapsChecksumAnnotationKey],
	)
}

func Test_hashConfigMapContents_Consistent(t *testing.T) {
	contents := map[string]configMapContent{
		"cm-a": {Data: map[string]string{"a": "1", "z": "26"}, BinaryData: map[string][]byte{"bin": {0, 1, 2}}},
		"cm-b": {Data: map[string]string{"foo": "bar"}},
		"cm-c": {BinaryData: map[string][]byte{"x": []byte("hello"), "y": []byte("world")}},
	}

	want := hashConfigMapContents(contents)
	assert.Equal(t, want, hashConfigMapContents(contents))

	// Rebuilding the same logical contents via different map insertion order must not change the hash.
	rebuilt := map[string]configMapContent{}
	rebuilt["cm-c"] = configMapContent{BinaryData: map[string][]byte{"y": []byte("world"), "x": []byte("hello")}}
	rebuilt["cm-b"] = configMapContent{Data: map[string]string{"foo": "bar"}}
	rebuilt["cm-a"] = configMapContent{Data: map[string]string{"z": "26", "a": "1"}, BinaryData: map[string][]byte{"bin": {0, 1, 2}}}

	assert.Equal(t, want, hashConfigMapContents(rebuilt))
}

func Test_annotateConfigMapsChecksum_ContentChangeChangesHash(t *testing.T) {
	makeReconcilerAndPodTmpl := func(data map[string]string) (*Reconciler, *corev1.PodTemplateSpec) {
		cm := &corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "my-config", Namespace: "ns-1"}, Data: data}
		r := newChecksumTestReconciler(cm)
		podTmpl := &corev1.PodTemplateSpec{Spec: corev1.PodSpec{Volumes: []corev1.Volume{configMapVolume("my-config")}}}
		return r, podTmpl
	}

	r1, podTmpl1 := makeReconcilerAndPodTmpl(map[string]string{"foo": "bar"})
	r2, podTmpl2 := makeReconcilerAndPodTmpl(map[string]string{"foo": "baz"})

	require.NoError(t, r1.annotateConfigMapsChecksum(context.Background(), "ns-1", podTmpl1))
	require.NoError(t, r2.annotateConfigMapsChecksum(context.Background(), "ns-1", podTmpl2))

	assert.NotEqual(t,
		podTmpl1.Annotations[constants.ConfigMapsChecksumAnnotationKey],
		podTmpl2.Annotations[constants.ConfigMapsChecksumAnnotationKey],
	)
}
