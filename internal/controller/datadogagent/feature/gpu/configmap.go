// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gpu

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// xidKernelLogsCheckConfig is the journald integration config that collects the kernel ring
// buffer, where the NVIDIA driver reports Xid errors.
//
// `include_matches` is applied as a journal-level filter: kernel messages carry
// `_TRANSPORT=kernel` and no `_SYSTEMD_UNIT`, so unit-based filtering cannot select them.
const xidKernelLogsCheckConfig = `logs:
  - type: journald
    config_id: ` + xidKernelLogsConfigID + `
    start_position: beginning
    include_matches:
      - _TRANSPORT=kernel
    source: kernel
    service: kernel
`

// getXidKernelLogsConfigMapName returns the name of the ConfigMap holding the journald config.
func getXidKernelLogsConfigMapName(ddaName string) string {
	return fmt.Sprintf("%s-gpu-xid-kernel-logs", ddaName)
}

// buildXidKernelLogsConfigMap builds the journald integration ConfigMap mounted into conf.d.
func buildXidKernelLogsConfigMap(namespace, ddaName string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       string(kubernetes.ConfigMapKind),
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      getXidKernelLogsConfigMapName(ddaName),
			Namespace: namespace,
		},
		Data: map[string]string{
			"conf.yaml": xidKernelLogsCheckConfig,
		},
	}
}
