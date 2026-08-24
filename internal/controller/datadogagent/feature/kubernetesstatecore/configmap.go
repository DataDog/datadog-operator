// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetesstatecore

import (
	"bytes"

	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/configmap"
)

func (f *ksmFeature) buildKSMCoreConfigMap(collectorOpts collectorOptions) (*corev1.ConfigMap, error) {
	if f.customConfig != nil && f.customConfig.ConfigMap != nil {
		return nil, nil
	}
	if f.customConfig != nil && f.customConfig.ConfigData != nil {
		return configmap.BuildConfigMapConfigData(f.owner.GetNamespace(), f.customConfig.ConfigData, f.configConfigMapName, ksmCoreCheckName)
	}

	configMap := buildDefaultConfigMap(
		f.owner.GetNamespace(),
		f.configConfigMapName,
		ksmCheckConfig(f.runInClusterChecksRunner, f.podCollectionOnNode, collectorOpts),
	)
	return configMap, nil
}

// ksmCoreConfig is the kubernetes_state_core check config, shared by any
// agent running the check (cluster agent, cluster check runner, node agent).
type ksmCoreConfig struct {
	ClusterCheck *bool             `yaml:"cluster_check,omitempty"`
	InitConfig   any               `yaml:"init_config"`
	Instances    []ksmCoreInstance `yaml:"instances"`
}

type ksmCoreInstance struct {
	SkipLeaderElection       *bool                  `yaml:"skip_leader_election,omitempty"`
	UseAPIServerCache        bool                   `yaml:"use_apiserver_cache,omitempty"`
	PodCollectionMode        string                 `yaml:"pod_collection_mode,omitempty"`
	ClusterAggregatesEnabled bool                   `yaml:"cluster_aggregates_enabled,omitempty"`
	Collectors               []string               `yaml:"collectors,omitempty"`
	CustomResource           *ksmCoreCustomResource `yaml:"custom_resource,omitempty"`
}

type ksmCoreCustomResource struct {
	Spec ksmCoreCustomResourceSpec `yaml:"spec"`
}

type ksmCoreCustomResourceSpec struct {
	Resources []v2alpha1.Resource `yaml:"resources"`
}

// buildKSMCorePodsOnNodeConfigMap builds the ConfigMap mounted into every node
// agent when PodCollectionMode is set to node_kubelet. Each node agent then
// runs a pods-only kubernetes_state_core check that reads pods locally from
// the Kubelet via workloadmeta.
func (f *ksmFeature) buildKSMCorePodsOnNodeConfigMap() (*corev1.ConfigMap, error) {
	cfg := ksmCoreConfig{
		Instances: []ksmCoreInstance{
			{
				PodCollectionMode:        "node_kubelet",
				ClusterAggregatesEnabled: true,
				Collectors:               []string{"pods"},
			},
		},
	}

	content, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, err
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.nodeAgentConfigMapName,
			Namespace: f.owner.GetNamespace(),
		},
		Data: map[string]string{
			ksmCorePodsOnNodeCheckName: string(content),
		},
	}, nil
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
func ksmCheckConfig(clusterCheck, podCollectionOnNode bool, collectorOpts collectorOptions) string {
	collectors := []string{
		"pods",
		"replicationcontrollers",
		"statefulsets",
		"nodes",
		"cronjobs",
		"jobs",
		"replicasets",
		"deployments",
		"configmaps",
		"services",
		"endpoints",
		"endpointslices",
		"daemonsets",
		"horizontalpodautoscalers",
		"poddisruptionbudgets",
		"limitranges",
		"resourcequotas",
		"secrets",
		"namespaces",
		"persistentvolumeclaims",
		"persistentvolumes",
		"ingresses",
		"storageclasses",
		"volumeattachments",
	}

	if collectorOpts.enableVPA {
		collectors = append(collectors, "verticalpodautoscalers")
	}
	if collectorOpts.enableAPIService {
		collectors = append(collectors, "apiservices")
	}
	if collectorOpts.enableControllerRevisions {
		collectors = append(collectors, "controllerrevisions")
	}
	if collectorOpts.enableCRD {
		collectors = append(collectors, "customresourcedefinitions")
	}

	var instances []ksmCoreInstance
	instance := ksmCoreInstance{
		SkipLeaderElection:       &clusterCheck,
		UseAPIServerCache:        collectorOpts.useApiServerCache,
		Collectors:               collectors,
		ClusterAggregatesEnabled: podCollectionOnNode,
	}

	if podCollectionOnNode {
		// Cluster-side instance only collects pods that have not been
		// scheduled to a node yet; node agents collect the rest locally.
		instance.PodCollectionMode = "cluster_unassigned"
	}

	if collectorOpts.customResources != nil {
		instance.CustomResource = &ksmCoreCustomResource{
			Spec: ksmCoreCustomResourceSpec{Resources: collectorOpts.customResources},
		}
	}

	instances = append(instances, instance)
	if podCollectionOnNode {
		instances = append(instances, ksmCoreInstance{
			SkipLeaderElection: &clusterCheck,
			PodCollectionMode:  "cluster_aggregates_only",
		})
	}

	cfg := ksmCoreConfig{
		ClusterCheck: &clusterCheck,
		Instances:    instances,
	}

	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(cfg); err != nil {
		return ""
	}
	if err := encoder.Close(); err != nil {
		return ""
	}

	return "---\n" + buf.String()
}
