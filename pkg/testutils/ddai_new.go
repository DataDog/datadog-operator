// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutils

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

var apiDDAIVersion = fmt.Sprintf("%s/%s", v1alpha1.GroupVersion.Group, v1alpha1.GroupVersion.Version)

// NewDatadogAgentInternal returns an initialized and defaulted DatadogAgentInternal for testing purpose
// DDAI should always have credentials and cluster agent token secret when created by DDA controller
func NewDatadogAgentInternal(ns, name string, globalOverride *v2alpha1.GlobalConfig) *v1alpha1.DatadogAgentInternal {
	ddai := &v1alpha1.DatadogAgentInternal{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogAgentInternal",
			APIVersion: apiDDAIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: ns,
			Name:      name,
			Labels:    map[string]string{},
		},
		Spec: v2alpha1.DatadogAgentSpec{
			Global: &v2alpha1.GlobalConfig{
				Credentials: defaultCredentials(),
				ClusterAgentTokenSecret: &v2alpha1.SecretConfig{
					SecretName: "cluster-agent-token",
					KeyName:    "token",
				},
			},
		},
	}

	if globalOverride != nil {
		ddai.Spec.Global = globalOverride
	}
	return ddai
}
