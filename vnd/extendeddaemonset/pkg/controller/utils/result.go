// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package utils

import (
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// MergeResult use to merge two reconcile.Result struct.
func MergeResult(r1, r2 reconcile.Result) reconcile.Result {
	r := reconcile.Result{}
	if r1.Requeue || r2.Requeue {
		r.Requeue = true
	}
	if r1.RequeueAfter+r2.RequeueAfter > 0 {
		switch {
		case r2.RequeueAfter == 0:
			r.RequeueAfter = r1.RequeueAfter
		case r1.RequeueAfter == 0:
			r.RequeueAfter = r2.RequeueAfter
		case r1.RequeueAfter > r2.RequeueAfter:
			r.RequeueAfter = r2.RequeueAfter
		default:
			r.RequeueAfter = r1.RequeueAfter
		}
	}

	return r
}
