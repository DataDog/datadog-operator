// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package object

import (
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func TestGetDefaultLabels(t *testing.T) {
	dda := &metav1.ObjectMeta{
		Name:      "datadog",
		Namespace: "agents",
		Labels: map[string]string{
			"tags.datadoghq.com/env": "prod",
			"unrelated":              "ignored",
		},
	}

	got := GetDefaultLabels(dda, "datadog-agent", "7.78.0")

	require.Equal(t, map[string]string{
		kubernetes.AppKubernetesNameLabelKey:     "datadog-agent-deployment",
		kubernetes.AppKubernetesInstanceLabelKey: "datadog-agent",
		kubernetes.AppKubernetesPartOfLabelKey:   "agents-datadog",
		kubernetes.AppKubernetesVersionLabelKey:  "7.78.0",
		kubernetes.AppKubernetesManageByLabelKey: "datadog-operator",
		"tags.datadoghq.com/env":                 "prod",
	}, got)
}

func TestMergeAnnotationsLabels(t *testing.T) {
	previous := map[string]string{
		"keep.datadoghq.com/value": "kept-by-domain",
		"custom.keep":              "kept-by-filter",
		"drop":                     "removed",
		"overwrite":                "old",
	}
	newValues := map[string]string{
		"overwrite": "new",
		"new":       "value",
	}

	got := MergeAnnotationsLabels(logr.Discard(), previous, newValues, "custom.*")

	require.Equal(t, map[string]string{
		"keep.datadoghq.com/value": "kept-by-domain",
		"custom.keep":              "kept-by-filter",
		"overwrite":                "new",
		"new":                      "value",
	}, got)
}

func TestGetChecksumAnnotationKey(t *testing.T) {
	require.Empty(t, GetChecksumAnnotationKey(""))
	require.Equal(t, "checksum/datadog-custom-config", GetChecksumAnnotationKey("datadog"))
}
