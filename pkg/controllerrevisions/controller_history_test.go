// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controllerrevisions

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/stretchr/testify/assert"
)

func TestHashControllerRevision_Deterministic(t *testing.T) {
	rev := &appsv1.ControllerRevision{
		Data: runtime.RawExtension{Raw: []byte(`{"foo":"bar"}`)},
	}
	assert.Equal(t, HashControllerRevision(rev, nil), HashControllerRevision(rev, nil))
}

func TestHashControllerRevision_DifferentDataDifferentHash(t *testing.T) {
	rev1 := &appsv1.ControllerRevision{Data: runtime.RawExtension{Raw: []byte(`{"foo":"bar"}`)}}
	rev2 := &appsv1.ControllerRevision{Data: runtime.RawExtension{Raw: []byte(`{"foo":"baz"}`)}}
	assert.NotEqual(t, HashControllerRevision(rev1, nil), HashControllerRevision(rev2, nil))
}

func TestHashControllerRevision_CollisionCountChangesHash(t *testing.T) {
	rev := &appsv1.ControllerRevision{Data: runtime.RawExtension{Raw: []byte(`{"foo":"bar"}`)}}
	c0, c1 := int32(0), int32(1)
	assert.NotEqual(t, HashControllerRevision(rev, &c0), HashControllerRevision(rev, &c1))
}

func TestControllerRevisionName_Format(t *testing.T) {
	assert.Equal(t, "my-dda-abc123", ControllerRevisionName("my-dda", "abc123"))
}

func TestControllerRevisionName_TruncatesLongPrefix(t *testing.T) {
	longPrefix := string(make([]byte, 230))
	name := ControllerRevisionName(longPrefix, "hash")
	// 223 truncated prefix + "-" + 4-char hash
	assert.Equal(t, 223+1+4, len(name))
	assert.LessOrEqual(t, len(name), 253)
}
