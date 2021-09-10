// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"fmt"
	"io/ioutil"
	"testing"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	rbacv1 "k8s.io/api/rbac/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestBuildOrchestratorExplorerRBAC(t *testing.T) {
	dda := &datadoghqv1alpha1.DatadogAgent{
		ObjectMeta: v1.ObjectMeta{
			Name: "test",
		},
	}
	// verify that default RBAC is sufficient
	rbac := buildOrchestratorExplorerRBAC(dda, orchestratorExplorerRBACPrefix, "1.2.3")
	yamlFile, err := ioutil.ReadFile("./testdata/orchestrator_clusterrole.yaml")
	require.NoError(t, err)
	c := rbacv1.ClusterRole{}
	err = yaml.Unmarshal(yamlFile, &c)
	require.NoError(t, err)
	require.Equal(t, c.Rules, rbac.Rules)
}

func TestBuildOrchestratorExplorerConfigMap(t *testing.T) {
	// test on both ConfigData and ConfigMap field set for conf is dealt with in datadog_validation.go
	// test on mounting external ConfigMap with the field `CustomConfigSpec.ConfigMap` is tested in the clusteragent.go
	enabledBool := true
	overrideConf := `
---
cluster_check: true
init_config:
instances:
  - collectors:
      - pods
    skip_leader_election: true
`
	dda := &datadoghqv1alpha1.DatadogAgent{
		ObjectMeta: v1.ObjectMeta{
			Name: "test",
		},
		Spec: datadoghqv1alpha1.DatadogAgentSpec{
			Features: datadoghqv1alpha1.DatadogFeatures{
				OrchestratorExplorer: &datadoghqv1alpha1.OrchestratorExplorerConfig{
					Enabled:      &enabledBool,
					ClusterCheck: &enabledBool,
				},
			},
		},
	}

	// cluster check enabled
	dda.Spec.Features.OrchestratorExplorer.ClusterCheck = datadoghqv1alpha1.NewBoolPointer(true)
	cm, err := buildOrchestratorExplorerConfigMap(dda)
	require.NoError(t, err)
	require.NotNil(t, cm)
	require.Equal(t, fmt.Sprintf("%s-%s", dda.Name, datadoghqv1alpha1.DefaultOrchestratorExplorerConf), cm.Name)
	require.Equal(t, cm.Data[orchestratorExplorerCheckName], orchestratorExplorerCheckConfig(true))

	// override case configData
	dda.Spec.Features.OrchestratorExplorer.Conf = &datadoghqv1alpha1.CustomConfigSpec{
		ConfigData: &overrideConf,
	}
	cm, err = buildOrchestratorExplorerConfigMap(dda)
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("%s-%s", dda.Name, datadoghqv1alpha1.DefaultOrchestratorExplorerConf), cm.Name)
	require.Equal(t, overrideConf, cm.Data[orchestratorExplorerCheckName])
}
