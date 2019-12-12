// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package utils

import (
	"context"
	"testing"

	framework "github.com/operator-framework/operator-sdk/pkg/test"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	dynclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// FindPodsByLabels looks up pods in a namespace using a label selector
func FindPodsByLabels(t *testing.T, client framework.FrameworkClient, namespace string, labelSelector string) (*corev1.PodList, error) {
	s, err := labels.Parse(labelSelector)
	if err != nil {
		return nil, err
	}

	listOptions := &dynclient.ListOptions{
		Namespace:     namespace,
		LabelSelector: s,
	}
	podList := &corev1.PodList{}
	err = client.List(context.TODO(), podList, listOptions)
	return podList, err
}
