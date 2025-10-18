// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helmcheck

import (
	"fmt"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

const (
	helmCheckConfFileName = "helm.yaml"
	helmCheckFolderName   = "helm.d"
	helmCheckRBACPrefix   = "helm-check"

	helmCheckConfigVolumeName = "helm-check-config"

	// DefaultHelmCheckConf default Helm Check ConfigMap name
	defaultHelmCheckConf string = "helm-check-config"
)

var helmCheckRBACPolicyRules = []rbacv1.PolicyRule{
	{
		APIGroups: []string{rbac.CoreAPIGroup},
		Resources: []string{
			rbac.ConfigMapsResource,
			rbac.SecretsResource,
		},
		Verbs: []string{
			rbac.GetVerb,
			rbac.ListVerb,
			rbac.WatchVerb,
		},
	},
}

func getHelmCheckRBACResourceName(owner metav1.Object, rbacSuffix string) string {
	return fmt.Sprintf("%s-%s-%s-%s", owner.GetNamespace(), owner.GetName(), helmCheckRBACPrefix, rbacSuffix)
}
