// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestVolumeBuilder(t *testing.T) {
	volumeFoo := corev1.Volume{
		Name: "foo",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	volumeFooOverride := corev1.Volume{
		Name: "foo",
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{},
		},
	}
	volumeBar := corev1.Volume{
		Name: "bar",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	type fields struct {
		options BuilderOptions
		volumes []corev1.Volume
	}
	tests := []struct {
		name    string
		fields  fields
		actions func(builder *VolumeBuilder)
		want    []corev1.Volume
	}{
		{
			name: "Add volume to empty list",
			fields: fields{
				options: DefaultBuilderOptions(),
			},
			actions: func(builder *VolumeBuilder) {
				builder.Add(&volumeFoo)
			},
			want: []corev1.Volume{
				volumeFoo,
			},
		},
		{
			name: "Add volume",
			fields: fields{
				options: DefaultBuilderOptions(),
				volumes: []corev1.Volume{
					volumeBar,
				},
			},
			actions: func(builder *VolumeBuilder) {
				builder.Add(&volumeFoo)
			},
			want: []corev1.Volume{
				volumeBar,
				volumeFoo,
			},
		},
		{
			name: "Add volume already exist",
			fields: fields{
				options: DefaultBuilderOptions(),
				volumes: []corev1.Volume{
					volumeBar,
					volumeFoo,
				},
			},
			actions: func(builder *VolumeBuilder) {
				builder.Add(&volumeFoo)
			},
			want: []corev1.Volume{
				volumeBar,
				volumeFoo,
			},
		},
		{
			name: "Add volume, allow override",
			fields: fields{
				options: DefaultBuilderOptions(),
				volumes: []corev1.Volume{
					volumeBar,
					volumeFoo,
				},
			},
			actions: func(builder *VolumeBuilder) {
				builder.Add(&volumeFooOverride)
			},
			want: []corev1.Volume{
				volumeBar,
				volumeFooOverride,
			},
		},
		{
			name: "Add volume, disallow override",
			fields: fields{
				options: BuilderOptions{
					AllowOverride: false,
				},
				volumes: []corev1.Volume{
					volumeBar,
					volumeFoo,
				},
			},
			actions: func(builder *VolumeBuilder) {
				builder.Add(&volumeFooOverride)
			},
			want: []corev1.Volume{
				volumeBar,
				volumeFoo,
			},
		},
		{
			name: "Remove volume",
			fields: fields{
				options: DefaultBuilderOptions(),
				volumes: []corev1.Volume{
					volumeFoo,
					volumeBar,
				},
			},
			actions: func(builder *VolumeBuilder) {
				builder.Remove(volumeFoo.Name)
			},
			want: []corev1.Volume{
				volumeBar,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &VolumeBuilder{
				options: tt.fields.options,
				volumes: tt.fields.volumes,
			}

			tt.actions(b)

			if got := b.Build(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("VolumeBuilder.Build() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewVolumeBuilder(t *testing.T) {
	fooVolume := corev1.Volume{
		Name: "foo",
	}

	tests := []struct {
		name     string
		iVolumes []corev1.Volume
		iOpts    *BuilderOptions
		want     *VolumeBuilder
	}{
		{
			name:  "default config, with nil options",
			iOpts: nil,
			want: &VolumeBuilder{
				options: DefaultBuilderOptions(),
			},
		},
		{
			name: "with Options",
			iOpts: &BuilderOptions{
				AllowOverride: false,
			},
			want: &VolumeBuilder{
				options: BuilderOptions{
					AllowOverride: false,
				},
			},
		},
		{
			name:  "default config, with input volumes",
			iOpts: nil,
			iVolumes: []corev1.Volume{
				fooVolume,
			},
			want: &VolumeBuilder{
				options: DefaultBuilderOptions(),
				volumes: []corev1.Volume{
					fooVolume,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewVolumeBuilder(tt.iVolumes, tt.iOpts); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewVolumeBuilder() = %v, want %v", got, tt.want)
			}
		})
	}
}
