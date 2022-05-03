// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestAddVolumeToPod(t *testing.T) {
	volumeFoo := &corev1.Volume{
		Name: "foo",
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: "/var/logs/",
			},
		},
	}

	volumeFoo2 := &corev1.Volume{
		Name: "foo",
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: "/tmp/",
			},
		},
	}

	volumeBar := &corev1.Volume{
		Name: "bar",
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: "/etc/",
			},
		},
	}

	volumeCM1 := &corev1.Volume{
		Name: "cm",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "cm",
				},
				Items: []corev1.KeyToPath{
					{
						Key:  "key1",
						Path: "/etc/values1.yaml",
					},
				},
			},
		},
	}
	volumeCM2 := &corev1.Volume{
		Name: "cm",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "cm",
				},
				Items: []corev1.KeyToPath{
					{
						Key:  "key2",
						Path: "/etc/values2.yaml",
					},
				},
			},
		},
	}
	volumeCM3 := &corev1.Volume{
		Name: "cm",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "cm",
				},
				Items: []corev1.KeyToPath{
					{
						Key:  "key2",
						Path: "/etc/values1.yaml",
					},
				},
			},
		},
	}
	mode := int32(007)
	volumeCM4 := &corev1.Volume{
		Name: "cm",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "cm",
				},
				Items: []corev1.KeyToPath{
					{
						Key:  "key1",
						Path: "/etc/values1.yaml",
						Mode: &mode,
					},
				},
			},
		},
	}

	volumeCMMerged := &corev1.Volume{
		Name: "cm",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "cm",
				},
				Items: []corev1.KeyToPath{
					{
						Key:  "key1",
						Path: "/etc/values1.yaml",
					},
					{
						Key:  "key2",
						Path: "/etc/values2.yaml",
					},
				},
			},
		},
	}

	type args struct {
		podSpec     *corev1.PodSpec
		volumeMount *corev1.Volume
		mergeFunc   VolumeMergeFunction
	}
	tests := []struct {
		name    string
		args    args
		want    []corev1.Volume
		wantErr bool
	}{
		{
			name: "empty volumes list",
			args: args{
				podSpec: &corev1.PodSpec{
					Volumes: nil,
				},
				volumeMount: volumeFoo.DeepCopy(),
			},
			want: []corev1.Volume{*volumeFoo},
		},
		{
			name: "volume name already exist",
			args: args{
				podSpec: &corev1.PodSpec{
					Volumes: []corev1.Volume{*volumeFoo},
				},
				volumeMount: volumeFoo2.DeepCopy(),
			},
			want: []corev1.Volume{*volumeFoo2},
		},
		{
			name: "add a second volume name",
			args: args{
				podSpec: &corev1.PodSpec{
					Volumes: []corev1.Volume{*volumeFoo},
				},
				volumeMount: volumeBar.DeepCopy(),
			},
			want: []corev1.Volume{*volumeFoo, *volumeBar},
		},
		{
			name: "volume already set, avoid merge",
			args: args{
				podSpec: &corev1.PodSpec{
					Volumes: []corev1.Volume{*volumeFoo},
				},
				volumeMount: volumeFoo2.DeepCopy(),
				mergeFunc:   ErrorOnMergeAttemptdVolumeMergeFunction,
			},
			want:    nil,
			wantErr: true,
		},
		{
			name: "volume configmap merge ok",
			args: args{
				podSpec: &corev1.PodSpec{
					Volumes: []corev1.Volume{*volumeCM1},
				},
				volumeMount: volumeCM2.DeepCopy(),
				mergeFunc:   MergeConfigMapItemsVolumeMergeFunction,
			},
			want: []corev1.Volume{*volumeCMMerged},
		},
		{
			name: "volume configmap merge error",
			args: args{
				podSpec: &corev1.PodSpec{
					Volumes: []corev1.Volume{*volumeCM1},
				},
				volumeMount: volumeCM3.DeepCopy(),
				mergeFunc:   MergeConfigMapItemsVolumeMergeFunction,
			},
			wantErr: true,
		},
		{
			name: "volume configmap merge mode",
			args: args{
				podSpec: &corev1.PodSpec{
					Volumes: []corev1.Volume{*volumeCM1},
				},
				volumeMount: volumeCM4.DeepCopy(),
				mergeFunc:   MergeConfigMapItemsVolumeMergeFunction,
			},
			want: []corev1.Volume{*volumeCM4},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AddVolumeToPod(tt.args.podSpec, tt.args.volumeMount, tt.args.mergeFunc)
			if (err != nil) != tt.wantErr {
				t.Errorf("AddVolumeToPod() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AddVolumeToPod() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_mergeMode(t *testing.T) {
	val7 := int32(7)
	val77 := int32(77)

	type args struct {
		a *int32
		b *int32
	}
	tests := []struct {
		name string
		args args
		want *int32
	}{
		{
			name: "bot nil",
			args: args{
				a: nil,
				b: nil,
			},
			want: nil,
		},
		{
			name: "a nil",
			args: args{
				a: nil,
				b: &val7,
			},
			want: &val7,
		},
		{
			name: "b nil",
			args: args{
				a: &val7,
				b: nil,
			},
			want: &val7,
		},
		{
			name: "a < b",
			args: args{
				a: &val7,
				b: &val77,
			},
			want: &val77,
		},
		{
			name: "a > b",
			args: args{
				a: &val77,
				b: &val7,
			},
			want: &val77,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mergeMode(tt.args.a, tt.args.b); got != tt.want {
				t.Errorf("mergeMode() = %v, want %v", got, tt.want)
			}
		})
	}
}
