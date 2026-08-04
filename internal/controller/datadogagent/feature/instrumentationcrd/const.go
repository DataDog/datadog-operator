// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package instrumentationcrd

import (
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

const (
	instrumentationCRDRBACPrefix = "instrumentation-crd"

	// DDInstrumentationCRDControllerEnabled is the env var that enables the instrumentation CRD controller
	// in the Cluster Agent. Maps to instrumentation_crd_controller.enabled in datadog.yaml.
	DDInstrumentationCRDControllerEnabled = "DD_INSTRUMENTATION_CRD_CONTROLLER_ENABLED"
)

var instrumentationCRDRBACPolicyRules = []rbacv1.PolicyRule{
	{
		APIGroups: []string{rbac.DatadogAPIGroup},
		Resources: []string{
			rbac.DatadogInstrumentationsResource,
		},
		Verbs: []string{
			rbac.GetVerb,
			rbac.ListVerb,
			rbac.WatchVerb,
		},
	},
	{
		APIGroups: []string{rbac.DatadogAPIGroup},
		Resources: []string{
			rbac.DatadogInstrumentationsStatusResource,
		},
		Verbs: []string{
			rbac.UpdateVerb,
			rbac.PatchVerb,
		},
	},
}

func GetInstrumentationCRDRBACResourceName(owner metav1.Object, suffix string) string {
	return fmt.Sprintf("%s-%s-%s-%s", owner.GetNamespace(), owner.GetName(), instrumentationCRDRBACPrefix, suffix)
}
