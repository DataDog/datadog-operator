// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package utils

import (
	"reflect"
	"testing"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestMergeResult(t *testing.T) {
	type args struct {
		r1 reconcile.Result
		r2 reconcile.Result
	}
	tests := []struct {
		name string
		args args
		want reconcile.Result
	}{
		{
			name: "both empty",
			args: args{
				r1: reconcile.Result{},
				r2: reconcile.Result{},
			},
			want: reconcile.Result{},
		},
		{
			name: "r1 Requeue == true",
			args: args{
				r1: reconcile.Result{Requeue: true},
				r2: reconcile.Result{},
			},
			want: reconcile.Result{Requeue: true},
		},
		{
			name: "r1 RequeueAfter == time.Second",
			args: args{
				r1: reconcile.Result{RequeueAfter: time.Second},
				r2: reconcile.Result{},
			},
			want: reconcile.Result{RequeueAfter: time.Second},
		},
		{
			name: "r2 RequeueAfter == time.Second",
			args: args{
				r1: reconcile.Result{},
				r2: reconcile.Result{RequeueAfter: time.Second},
			},
			want: reconcile.Result{RequeueAfter: time.Second},
		},
		{
			name: "r1 Requeue == true",
			args: args{
				r1: reconcile.Result{Requeue: true},
				r2: reconcile.Result{},
			},
			want: reconcile.Result{Requeue: true},
		},
		{
			name: "r1 RequeueAfter == time.Second, r2 RequeueAfter == time.Minute",
			args: args{
				r1: reconcile.Result{RequeueAfter: time.Second},
				r2: reconcile.Result{RequeueAfter: time.Minute},
			},
			want: reconcile.Result{RequeueAfter: time.Second},
		},
		{
			name: "r1 RequeueAfter == time.Minute, r2 RequeueAfter == time.Second",
			args: args{
				r1: reconcile.Result{RequeueAfter: time.Minute},
				r2: reconcile.Result{RequeueAfter: time.Second},
			},
			want: reconcile.Result{RequeueAfter: time.Second},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MergeResult(tt.args.r1, tt.args.r2); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MergeResult() = %v, want %v", got, tt.want)
			}
		})
	}
}
