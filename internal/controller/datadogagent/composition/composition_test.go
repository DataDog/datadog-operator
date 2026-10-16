// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package composition_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/composition"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/experimental"
)

func TestModeAnnotationKeyUsesExperimentalPrefix(t *testing.T) {
	assert.True(t, strings.HasPrefix(composition.ModeAnnotationKey, experimental.ExperimentalAnnotationPrefix+"/"))
}

func TestModeOf(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        composition.Mode
	}{
		{name: "no annotation", want: composition.ModeDefault},
		{name: "standalone", annotations: map[string]string{composition.ModeAnnotationKey: "standalone"}, want: composition.ModeStandalone},
		{name: "invalid value", annotations: map[string]string{composition.ModeAnnotationKey: "ddot"}, want: composition.ModeDefault},
		{name: "empty value", annotations: map[string]string{composition.ModeAnnotationKey: ""}, want: composition.ModeDefault},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &metav1.ObjectMeta{Annotations: tt.annotations}
			assert.Equal(t, tt.want, composition.ModeOf(obj))
			assert.Equal(t, tt.want, composition.PolicyOf(obj).Mode)
		})
	}
}

func TestPolicies(t *testing.T) {
	def := composition.PolicyFor(composition.ModeDefault)
	assert.False(t, def.Composed())
	assert.True(t, def.ClusterAgent)
	assert.True(t, def.ClusterChecksRunner)
	assert.True(t, def.OtelAgentGateway)
	assert.Equal(t,
		[]apicommon.AgentContainerName{apicommon.CoreAgentContainerName, apicommon.OtelAgent},
		def.NodeAgentContainers(apicommon.CoreAgentContainerName, apicommon.OtelAgent))

	standalone := composition.PolicyFor(composition.ModeStandalone)
	assert.True(t, standalone.Composed())
	assert.False(t, standalone.ClusterAgent)
	assert.False(t, standalone.ClusterChecksRunner)
	assert.True(t, standalone.OtelAgentGateway)
	assert.False(t, standalone.AllowsNodeAgentContainer(apicommon.CoreAgentContainerName))
	assert.False(t, standalone.AllowsNodeAgentContainer(apicommon.TraceAgentContainerName))
	assert.Equal(t,
		[]apicommon.AgentContainerName{apicommon.HostProfiler, apicommon.OtelAgent},
		standalone.NodeAgentContainers(apicommon.CoreAgentContainerName, apicommon.HostProfiler, apicommon.OtelAgent))
	assert.Empty(t, standalone.NodeAgentContainers(apicommon.CoreAgentContainerName))

	assert.Equal(t, def, composition.PolicyFor("unknown"))
}
