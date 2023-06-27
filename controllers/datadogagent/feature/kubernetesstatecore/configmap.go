// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetesstatecore

import (
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/configmap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (f *ksmFeature) buildKSMCoreConfigMap(cmOptions configMapOptions) (*corev1.ConfigMap, error) {
	if f.customConfig != nil && f.customConfig.ConfigMap != nil {
		return nil, nil
	}
	if f.customConfig != nil && f.customConfig.ConfigData != nil {
		return configmap.BuildConfigMapConfigData(f.owner.GetNamespace(), f.customConfig.ConfigData, f.configConfigMapName, ksmCoreCheckName)
	}

	configMap := buildDefaultConfigMap(f.owner.GetNamespace(), f.configConfigMapName, ksmCheckConfig(f.runInClusterChecksRunner, cmOptions))
	return configMap, nil
}

func buildDefaultConfigMap(namespace, cmName string, content string) *corev1.ConfigMap {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: namespace,
		},
		Data: map[string]string{
			ksmCoreCheckName: content,
		},
	}
	return configMap
}

// KSM should be configured as a cluster check only when there are Cluster Check
// Runners deployed.
// This check is not designed to work on the DaemonSet Agent. That's why when
// cluster checks are enabled but without Cluster Check Runners, we don't want
// to set this check as a cluster check, because then it would be scheduled in
// the DaemonSet agent instead of the DCA.
func ksmCheckConfig(clusterCheck bool, cmOptions configMapOptions) string {
	stringVal := strconv.FormatBool(clusterCheck)
	config := fmt.Sprintf(`---
cluster_check: %s
init_config:
instances:
  - telemetry: true
    skip_leader_election: %s
    collectors:
    - pods
    - replicationcontrollers
    - statefulsets
    - nodes
    - cronjobs
    - jobs
    - replicasets
    - deployments
    - configmaps
    - services
    - endpoints
    - daemonsets
    - horizontalpodautoscalers
    - limitranges
    - resourcequotas
    - secrets
    - namespaces
    - persistentvolumeclaims
    - persistentvolumes
    - ingresses
`, stringVal, stringVal)

	if cmOptions.withVPA {
		config += "    - verticalpodautoscalers\n"
	}

	if cmOptions.withAPIServices {
		config += "    - apiservices\n"
	}

	if cmOptions.withCRDs {
		config += "    - customresourcedefinitions\n"
	}

	return config
}
