// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helmcheck

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	helmCheckConfFileName = "helm.yaml"
	helmCheckFolderName   = "helm.d"
	helmCheckRBACPrefix   = "helm-check"
)

func getHelmCheckRBACResourceName(owner metav1.Object, rbacSuffix string) string {
	return fmt.Sprintf("%s-%s-%s-%s", owner.GetNamespace(), owner.GetName(), helmCheckRBACPrefix, rbacSuffix)
}
