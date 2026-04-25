// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetesstatecore

import (
	"bytes"
	"strconv"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/configmap"
)

func (f *ksmFeature) buildKSMCoreConfigMap(collectorOpts collectorOptions) (*corev1.ConfigMap, error) {
	if f.customConfig != nil && f.customConfig.ConfigMap != nil {
		return nil, nil
	}
	if f.customConfig != nil && f.customConfig.ConfigData != nil {
		return configmap.BuildConfigMapConfigData(f.owner.GetNamespace(), f.customConfig.ConfigData, f.configConfigMapName, ksmCoreCheckName)
	}

	configMap := buildDefaultConfigMap(f.owner.GetNamespace(), f.configConfigMapName, ksmCheckConfig(f.runInClusterChecksRunner, collectorOpts))
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

// writeYAMLBlock writes `    <key>:\n` followed by a YAML-encoded value,
// indented to sit under the key inside the KSM check instance.
// It ignores encode errors because bytes.Buffer.Write is infallible and
// the callers pass well-typed maps/slices that cannot fail to marshal.
func writeYAMLBlock(config *bytes.Buffer, key string, indent int, value any) {
	config.WriteString("    " + key + ":\n")
	iw := newIndentWriter(config, indent)
	enc := yaml.NewEncoder(iw)
	enc.SetIndent(2)
	_ = enc.Encode(value)
	enc.Close()
}

// KSM should be configured as a cluster check only when there are Cluster Check
// Runners deployed.
// This check is not designed to work on the DaemonSet Agent. That's why when
// cluster checks are enabled but without Cluster Check Runners, we don't want
// to set this check as a cluster check, because then it would be scheduled in
// the DaemonSet agent instead of the DCA.
func ksmCheckConfig(clusterCheck bool, collectorOpts collectorOptions) string {
	stringVal := strconv.FormatBool(clusterCheck)
	config := bytes.NewBufferString(`---
cluster_check: `)
	config.WriteString(stringVal)
	config.WriteString(`
init_config:
instances:
  - skip_leader_election: `)
	config.WriteString(stringVal)
	config.WriteString(`
    collectors:
    - pods
    - replicationcontrollers
    - statefulsets
    - nodes
    - cronjobs
    - jobs
    - replicasets
    - deployments
    - services
    - endpoints
    - daemonsets
    - horizontalpodautoscalers
    - poddisruptionbudgets
    - limitranges
    - resourcequotas
    - namespaces
    - persistentvolumeclaims
    - persistentvolumes
    - ingresses
    - storageclasses
    - volumeattachments
`)

	if collectorOpts.collectSecrets {
		config.WriteString("    - secrets\n")
	}
	if collectorOpts.collectConfigMaps {
		config.WriteString("    - configmaps\n")
	}

	if collectorOpts.enableVPA {
		config.WriteString("    - verticalpodautoscalers\n")
	}

	if collectorOpts.enableAPIService {
		config.WriteString("    - apiservices\n")
	}

	if collectorOpts.enableControllerRevisions {
		config.WriteString("    - controllerrevisions\n")
	}

	if collectorOpts.enableCRD {
		config.WriteString("    - customresourcedefinitions\n")
	}

	if collectorOpts.customResources != nil {
		config.WriteString(`    custom_resource:
      spec:
        resources:
`)
		// Use indentWriter to add proper indentation (10 spaces for YAML nesting)
		indentedWriter := newIndentWriter(config, 10)
		encoder := yaml.NewEncoder(indentedWriter)
		encoder.SetIndent(2) // Keep YAML's internal indentation
		encoder.Encode(collectorOpts.customResources)
		encoder.Close()
	}

	if len(collectorOpts.labelsAsTags) > 0 {
		writeYAMLBlock(config, "labels_as_tags", 6, collectorOpts.labelsAsTags)
	}

	if len(collectorOpts.annotationsAsTags) > 0 {
		writeYAMLBlock(config, "annotations_as_tags", 6, collectorOpts.annotationsAsTags)
	}

	if len(collectorOpts.tags) > 0 {
		writeYAMLBlock(config, "tags", 4, collectorOpts.tags)
	}

	return config.String()
}
