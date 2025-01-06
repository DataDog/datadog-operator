// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutils

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

var apiVersion = fmt.Sprintf("%s/%s", v2alpha1.GroupVersion.Group, v2alpha1.GroupVersion.Version)

// NewDatadogAgent returns an initialized and defaulted DatadogAgent for testing purpose
func NewDatadogAgent(ns, name string, globalOverride *v2alpha1.GlobalConfig) *v2alpha1.DatadogAgent {
	dda := &v2alpha1.DatadogAgent{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogAgent",
			APIVersion: apiVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace:  ns,
			Name:       name,
			Labels:     map[string]string{},
			Finalizers: []string{"finalizer.agent.datadoghq.com"},
		},
		Spec: v2alpha1.DatadogAgentSpec{
			Global: &v2alpha1.GlobalConfig{
				Credentials: defaultCredentials(),
			},
		},
	}

	if globalOverride != nil {
		dda.Spec.Global = globalOverride
	}
	return dda
}

func defaultCredentials() *v2alpha1.DatadogCredentials {
	defaultAPIKey := "0000000000000000000000"
	defaultAppKey := "0000000000000000000000"

	return &v2alpha1.DatadogCredentials{
		APIKey: &defaultAPIKey,
		AppKey: &defaultAppKey,
	}
}
