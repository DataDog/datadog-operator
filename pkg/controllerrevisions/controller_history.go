// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Ported from https://github.com/kubernetes/kubernetes/blob/master/pkg/controller/history/controller_history.go
//
// Why copied instead of imported: k8s.io/kubernetes declares internal replace
// directives in its go.mod that make it unimportable as a Go module dependency
// in external projects. The three functions below are the only pieces we need.
//
// Differences from upstream and rationale:
//
//  1. HashControllerRevision — Data.Object branch removed.
//     Upstream supports two storage formats: Data.Raw (JSON bytes) and
//     Data.Object (a runtime.Object hashed via k8s.io/apimachinery/pkg/util/hash).
//     We always marshal our snapshot struct to JSON and store it in Data.Raw, so
//     the Data.Object path is dead code for us. Removing it also avoids pulling
//     in the spew-based reflect hash from util/hash.

package controllerrevisions

import (
	"fmt"
	"hash/fnv"
	"maps"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
)

// ControllerRevisionHashLabel is the label key used to store the revision hash.
// Matches the upstream Kubernetes constant.
const ControllerRevisionHashLabel = "controller.kubernetes.io/hash"

// HashControllerRevision hashes the contents of revision's Data.Raw using FNV-32.
// If collisionCount is not nil, its value is also mixed into the hash to produce a
// distinct name on collision.
//
// Upstream: https://github.com/kubernetes/kubernetes/blob/master/pkg/controller/history/controller_history.go
func HashControllerRevision(revision *appsv1.ControllerRevision, collisionCount *int32) string {
	hf := fnv.New32()
	if len(revision.Data.Raw) > 0 {
		hf.Write(revision.Data.Raw)
	}
	if collisionCount != nil {
		hf.Write([]byte(strconv.FormatInt(int64(*collisionCount), 10)))
	}
	return utilrand.SafeEncodeString(fmt.Sprint(hf.Sum32()))
}

// ControllerRevisionName returns the name for a ControllerRevision in the form
// "{prefix}-{hash}". The prefix is truncated to 223 characters so the full name
// stays within Kubernetes' 253-character limit.
//
// Upstream: https://github.com/kubernetes/kubernetes/blob/master/pkg/controller/history/controller_history.go
func ControllerRevisionName(prefix, hash string) string {
	if len(prefix) > 223 {
		prefix = prefix[:223]
	}
	return fmt.Sprintf("%s-%s", prefix, hash)
}

// NewControllerRevision returns a ControllerRevision with a ControllerRef pointing
// to owner. The revision has labels matching the provided labels map (plus the hash
// label), Data set to data, and Revision set to revision.
//
// Upstream: https://github.com/kubernetes/kubernetes/blob/master/pkg/controller/history/controller_history.go
func NewControllerRevision(
	owner metav1.Object,
	gvk schema.GroupVersionKind,
	labels map[string]string,
	data runtime.RawExtension,
	revision int64,
	collisionCount *int32,
) *appsv1.ControllerRevision {
	labelMap := make(map[string]string, len(labels)+1)
	maps.Copy(labelMap, labels)
	cr := &appsv1.ControllerRevision{
		ObjectMeta: metav1.ObjectMeta{
			Namespace:       owner.GetNamespace(),
			Labels:          labelMap,
			OwnerReferences: []metav1.OwnerReference{*metav1.NewControllerRef(owner, gvk)},
		},
		Data:     data,
		Revision: revision,
	}
	hash := HashControllerRevision(cr, collisionCount)
	cr.Name = ControllerRevisionName(owner.GetName(), hash)
	cr.Labels[ControllerRevisionHashLabel] = hash
	return cr
}
