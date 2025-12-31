// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetesstatecore

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	kubeStateMetricsRBACPrefix = "ksm-core"
	ksmCoreCheckName           = "kubernetes_state_core.yaml.default"
	ksmCoreCheckFolderName     = "kubernetes_state_core.d"

	ksmCoreVolumeName = "ksm-core-config"
	// DefaultKubeStateMetricsCoreConf default ksm core ConfigMap name
	defaultKubeStateMetricsCoreConf string = "kube-state-metrics-core-config"

	// legacy kubernetes_state auto_conf override
	legacyKSMAutoConfVolumeName = "legacy-ksm-autoconf-override"
	legacyKSMAutoConfMountPath  = "/etc/datadog-agent/conf.d/kubernetes_state.d"
)

// GetKubeStateMetricsRBACResourceName return the RBAC resources name
func GetKubeStateMetricsRBACResourceName(owner metav1.Object, suffix string) string {
	return fmt.Sprintf("%s-%s-%s-%s", owner.GetNamespace(), owner.GetName(), kubeStateMetricsRBACPrefix, suffix)
}
