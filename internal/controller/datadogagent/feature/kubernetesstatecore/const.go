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

	// Filename for the pods-only check shipped to node agents when
	// PodCollectionMode is set to node_kubelet.
	ksmCorePodsOnNodeCheckName  = "kubernetes_state_core.yaml"
	ksmCorePodsOnNodeVolumeName = "ksm-core-pods-on-node-config"
	// defaultKSMPodsOnNodeConf is the default ConfigMap name suffix for
	// the node-side KSM check (each pod-collection-on-node deployment owns
	// one of these alongside the cluster-side ConfigMap).
	defaultKSMPodsOnNodeConf string = "kube-state-metrics-core-pods-on-node-config"

	// Minimum agent / cluster-agent / node-agent version supporting the
	// pod_collection_mode field used by PodCollectionMode=node_kubelet.
	// node_kubelet shipped in 7.58; the startup fix landed in 7.60, so 7.60
	// is the supported floor across all components that load the check.
	podCollectionOnNodeMinVersion = "7.60.0-0"
)

// GetKubeStateMetricsRBACResourceName return the RBAC resources name
func GetKubeStateMetricsRBACResourceName(owner metav1.Object, suffix string) string {
	return fmt.Sprintf("%s-%s-%s-%s", owner.GetNamespace(), owner.GetName(), kubeStateMetricsRBACPrefix, suffix)
}
