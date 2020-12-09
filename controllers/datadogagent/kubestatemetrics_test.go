package datadogagent

import (
	"fmt"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	rbacv1 "k8s.io/api/rbac/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
)

func TestBuildKubeStateMetricsCoreRBAC(t *testing.T) {
	dda := &datadoghqv1alpha1.DatadogAgent{
		ObjectMeta: v1.ObjectMeta{
			Name: "test",
		},
	}
	// verify that default RBAC is sufficient
	rbac := buildKubeStateMetricsCoreRBAC(dda, kubeStateMetricsRBACName, "1.2.3")
	yamlFile, err := ioutil.ReadFile("./testdata/ksm_clusterrole.yaml")
	require.NoError(t, err)
	c := rbacv1.ClusterRole{}
	err = yaml.Unmarshal(yamlFile, &c)
	require.NoError(t, err)
	require.Equal(t, c.Rules, rbac.Rules)
}

func TestBuildKSMCoreConfigMap(t *testing.T) {
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
`
	dda := &datadoghqv1alpha1.DatadogAgent{
		ObjectMeta: v1.ObjectMeta{
			Name: "test",
		},
		Spec: datadoghqv1alpha1.DatadogAgentSpec{
			Features: &datadoghqv1alpha1.DatadogFeatures{
				KubeStateMetricsCore: &datadoghqv1alpha1.KubeStateMetricsCore{
					Enabled: &enabledBool,
				},
			},
		},
	}
	// default case, no override
	cm, err := buildKSMCoreConfigMap(dda)
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("%s-%s", dda.Name, datadoghqv1alpha1.DefaultKubeStateMetricsCoreConf), cm.Name)
	require.Equal(t, cm.Data[ksmCoreCheckName], defaultKSMCoreConfigMap)

	// override case configData
	dda.Spec.Features.KubeStateMetricsCore.Conf = &datadoghqv1alpha1.CustomConfigSpec{
		ConfigData: &overrideConf,
	}
	cm, err = buildKSMCoreConfigMap(dda)
	require.NoError(t, err)
	require.Equal(t, fmt.Sprintf("%s-%s", dda.Name, datadoghqv1alpha1.DefaultKubeStateMetricsCoreConf), cm.Name)
	require.Equal(t, overrideConf, cm.Data[ksmCoreCheckName])
}
