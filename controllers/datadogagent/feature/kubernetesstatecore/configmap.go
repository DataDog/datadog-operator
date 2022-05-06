// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetesstatecore

import (
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-operator/controllers/datadogagent/object"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/configmap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (f *ksmFeature) buildKSMCoreConfigMap() (*corev1.ConfigMap, error) {
	if f.customConfig != nil && f.customConfig.ConfigMap != nil {
		return nil, nil
	}
	if f.customConfig != nil && f.customConfig.ConfigData != nil {
		return configmap.BuildConfiguration(f.owner, f.customConfig.ConfigData, f.configConfigMapName, ksmCoreCheckName)
	}

	configMap := buildDefaultConfigMap(f.owner, f.configConfigMapName, ksmCheckConfig(f.clusterChecksEnabled))
	return configMap, nil
}

func buildDefaultConfigMap(owner metav1.Object, cmName string, content string) *corev1.ConfigMap {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        cmName,
			Namespace:   owner.GetNamespace(),
			Labels:      object.GetDefaultLabels(owner, owner.GetName(), ""),
			Annotations: object.GetDefaultAnnotations(owner),
		},
		Data: map[string]string{
			ksmCoreCheckName: content,
		},
	}
	return configMap
}

func ksmCheckConfig(clusteCheck bool) string {
	stringVal := strconv.FormatBool(clusteCheck)
	return fmt.Sprintf(`---
cluster_check: %s
init_config:
instances:
  - collectors:
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
    telemetry: true
    skip_leader_election: %s
`, stringVal, stringVal)
}
