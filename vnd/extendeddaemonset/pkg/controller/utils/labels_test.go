// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package utils

import (
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildInfoLabels(t *testing.T) {
	type args struct {
		obj *metav1.ObjectMeta
	}
	tests := []struct {
		name  string
		args  args
		want  []string
		want1 []string
	}{
		{
			name: "empty labels map",
			args: args{
				obj: &metav1.ObjectMeta{
					Labels: nil,
				},
			},
			want:  []string{},
			want1: []string{},
		},
		{
			name: "2 labels in map",
			args: args{
				obj: &metav1.ObjectMeta{
					Labels: map[string]string{
						"foo": "bar",
						"tic": "tac",
					},
				},
			},
			want:  []string{"foo", "tic"},
			want1: []string{"bar", "tac"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, got1 := BuildInfoLabels(tt.args.obj)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("BuildInfoLabels() got = %#v, want %#v", got, tt.want)
			}
			if !reflect.DeepEqual(got1, tt.want1) {
				t.Errorf("BuildInfoLabels() got1 = %#v, want %#v", got1, tt.want1)
			}
		})
	}
}

func Test_sanitizeLabelName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "no change",
			input: "hello",
			want:  "hello",
		},
		{
			name:  "one invalid character",
			input: "hello!",
			want:  "hello_",
		},
		{
			name:  "two invalid characters",
			input: "h*ello!",
			want:  "h_ello_",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeLabelName(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeLabelName() got = %#v, want %#v", got, tt.want)
			}
		})
	}
}
