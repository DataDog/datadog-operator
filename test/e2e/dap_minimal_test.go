// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build e2e
// +build e2e

package e2e

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/test/e2e/common"
	"github.com/DataDog/datadog-operator/test/e2e/provisioners"
	"github.com/DataDog/datadog-operator/test/e2e/tests/utils"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentwithoperatorparams"
	"github.com/DataDog/test-infra-definitions/components/datadog/operatorparams"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var (
	dapNameRegex  = "^datadog-agent-with-profile-[A-Za-z0-9-]+"
	dapHelmValues = `installCRDs: false
datadogAgentProfile:
  enabled: true`
)

type localKindDAPSuite struct {
	e2e.BaseSuite[environments.Kubernetes]
}

func TestLocalKindDAPSuite(t *testing.T) {
	operatorOptions := []operatorparams.Option{
		operatorparams.WithNamespace(common.NamespaceName),
		operatorparams.WithOperatorFullImagePath(common.OperatorImageName),
		operatorparams.WithHelmValues(dapHelmValues),
	}

	ddaConfigPath, err := common.GetAbsPath(filepath.Join(manifestsPath, "datadog-agent-resources.yaml"))
	assert.NoError(t, err)

	ddaOptions := []agentwithoperatorparams.Option{
		agentwithoperatorparams.WithNamespace(common.NamespaceName),
		agentwithoperatorparams.WithDDAConfig(agentwithoperatorparams.DDAConfig{
			Name:         "datadog-resources",
			YamlFilePath: ddaConfigPath,
		}),
	}

	provisionerOptions := []provisioners.KubernetesProvisionerOption{
		provisioners.WithK8sVersion(common.K8sVersion),
		provisioners.WithOperatorOptions(operatorOptions...),
		provisioners.WithDDAOptions(ddaOptions...),
		provisioners.WithYAMLWorkload(provisioners.YAMLWorkload{Name: "dap", Path: filepath.Join(manifestsPath, "dap.yaml")}),
		provisioners.WithoutFakeIntake(),
	}

	e2eOpts := []e2e.SuiteOption{
		e2e.WithProvisioner(provisioners.KubernetesProvisioner(provisionerOptions...)),
	}

	e2e.Run(t, &localKindDAPSuite{}, e2eOpts...)
}

func (s *localKindDAPSuite) TestDAP() {
	s.T().Run("Verify Operator", func(t *testing.T) {
		s.Assert().EventuallyWithT(func(c *assert.CollectT) {
			utils.VerifyOperator(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client())
		}, 300*time.Second, 15*time.Second, "Could not validate operator pod in time")
	})

	s.T().Run("Apply DAP to single node", func(t *testing.T) {
		s.EventuallyWithT(func(c *assert.CollectT) {
			utils.VerifyAgentPods(s.T(), c, common.NamespaceName, s.Env().KubernetesCluster.Client(), common.NodeAgentSelector+",agent.datadoghq.com/datadogagentprofile=dap")

			dsList, err := s.Env().KubernetesCluster.Client().AppsV1().DaemonSets(common.NamespaceName).List(context.TODO(), metav1.ListOptions{
				LabelSelector: "agent.datadoghq.com/datadogagentprofile=dap",
			})
			assert.NoError(s.T(), err)
			assert.Len(t, dsList.Items, 1)

			verifyDSConfig(t, dsList.Items[0])

			agentPodList, err := s.Env().KubernetesCluster.Client().CoreV1().Pods(common.NamespaceName).List(context.TODO(), metav1.ListOptions{LabelSelector: common.NodeAgentSelector + ",agent.datadoghq.com/datadogagentprofile=dap",
				FieldSelector: "status.phase=Running"})
			assert.NoError(s.T(), err)

			for _, agentPod := range agentPodList.Items {
				verifyPodConfig(t, agentPod)
			}

		}, 900*time.Second, 10*time.Second, "DAP Agent pods did not become ready in time.")
	})
}

func verifyDSConfig(t *testing.T, ds appsv1.DaemonSet) {
	// name
	assert.Regexp(t, dapNameRegex, ds.Name)
	// update strategy
	assert.Equal(t, appsv1.RollingUpdateDaemonSetStrategyType, ds.Spec.UpdateStrategy.Type)
	assert.Equal(t, intstr.IntOrString{IntVal: int32(2)}, *ds.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable)

	// affinity
	expectedAffinity := corev1.NodeSelectorTerm{
		MatchExpressions: []corev1.NodeSelectorRequirement{
			{
				Key:      "beta.kubernetes.io/os",
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{"linux"},
			},
			{
				Key:      "agent.datadoghq.com/datadogagentprofile",
				Operator: corev1.NodeSelectorOpIn,
				Values:   []string{"dap"},
			},
		},
	}
	assert.Contains(t, ds.Spec.Template.Spec.Affinity.NodeAffinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms, expectedAffinity)

}

func verifyPodConfig(t *testing.T, pod corev1.Pod) {
	// name
	assert.Regexp(t, dapNameRegex, pod.Name)
	// label
	expectedLabel := map[string]string{"foo": "bar"}
	assert.Subset(t, pod.Labels, expectedLabel)

	for _, cont := range pod.Spec.Containers {
		if cont.Name == string(apicommon.CoreAgentContainerName) {
			// resources
			expectedCPULimit := resource.MustParse("500m")
			assert.Equal(t, expectedCPULimit, cont.Resources.Limits[corev1.ResourceCPU])
			// env var
			expectedEnvVar := corev1.EnvVar{Name: "TEST", Value: "test"}
			assert.Contains(t, cont.Env, expectedEnvVar)
		}
	}
}
