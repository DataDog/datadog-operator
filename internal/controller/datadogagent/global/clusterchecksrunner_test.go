// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package global

import (
	"fmt"
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	ccr "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusterchecksrunner"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/fake"
	mergerfake "github.com/DataDog/datadog-operator/internal/controller/datadogagent/merger/fake"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestApplyClusterChecksRunnerResources_WithClusterName(t *testing.T) {
	clusterName := "my-cluster-name"
	ddaSpec := &v2alpha1.DatadogAgentSpec{
		Global: &v2alpha1.GlobalConfig{
			ClusterName: &clusterName,
		},
	}

	manager := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})
	applyClusterChecksRunnerResources(manager, ddaSpec)

	envVars := manager.EnvVarMgr.EnvVarsByC[mergerfake.AllContainers]
	assert.Len(t, envVars, 1)
	assert.Equal(t, constants.DDHostName, envVars[0].Name)
	assert.Equal(t, fmt.Sprintf("$(%s)-%s", ccr.DDCCRNodeName, clusterName), envVars[0].Value)
}

func TestApplyClusterChecksRunnerResources_WithoutClusterName(t *testing.T) {
	ddaSpec := &v2alpha1.DatadogAgentSpec{
		Global: &v2alpha1.GlobalConfig{},
	}

	manager := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})
	applyClusterChecksRunnerResources(manager, ddaSpec)

	envVars := manager.EnvVarMgr.EnvVarsByC[mergerfake.AllContainers]
	assert.Len(t, envVars, 1)
	assert.Equal(t, constants.DDHostName, envVars[0].Name)
	assert.Equal(t, fmt.Sprintf("$(%s)", ccr.DDCCRNodeName), envVars[0].Value)
}

func TestApplyClusterChecksRunnerResources_NilGlobal(t *testing.T) {
	ddaSpec := &v2alpha1.DatadogAgentSpec{}

	manager := fake.NewPodTemplateManagers(t, corev1.PodTemplateSpec{})
	applyClusterChecksRunnerResources(manager, ddaSpec)

	envVars := manager.EnvVarMgr.EnvVarsByC[mergerfake.AllContainers]
	assert.Len(t, envVars, 1)
	assert.Equal(t, constants.DDHostName, envVars[0].Name)
	assert.Equal(t, fmt.Sprintf("$(%s)", ccr.DDCCRNodeName), envVars[0].Value)
}
