package datadogagent

import (
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
