// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package admissioncontroller

import (
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_getACServiceName(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
		},
	}
	result := getACServiceName(dda)
	if result != "foo-admission-controller" {
		t.Errorf("Expected %s, got %s", "test-admission-controller", result)
	}
}

func Test_getACWebhookName(t *testing.T) {
	dda := &v2alpha1.DatadogAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name: "foo",
		},
	}
	result := getACWebhookName(dda)
	if result != "foo-webhook" {
		t.Errorf("Expected %s, got %s", "test-webhook", result)
	}
}
