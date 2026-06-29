// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDatadogAgentBuilder_WithKubernetesActionsEnabled(t *testing.T) {
	builder := NewDatadogAgentBuilder()
	require.Nil(t, builder.datadogAgent.Spec.Features.KubernetesActions)

	returnedBuilder := builder.WithKubernetesActionsEnabled(true)
	require.Same(t, builder, returnedBuilder)
	require.NotNil(t, builder.datadogAgent.Spec.Features.KubernetesActions)
	require.NotNil(t, builder.datadogAgent.Spec.Features.KubernetesActions.Enabled)
	assert.True(t, *builder.datadogAgent.Spec.Features.KubernetesActions.Enabled)

	initialKubernetesActionsConfig := builder.datadogAgent.Spec.Features.KubernetesActions
	builder.WithKubernetesActionsEnabled(false)

	assert.Same(t, initialKubernetesActionsConfig, builder.datadogAgent.Spec.Features.KubernetesActions)
	require.NotNil(t, builder.datadogAgent.Spec.Features.KubernetesActions.Enabled)
	assert.False(t, *builder.datadogAgent.Spec.Features.KubernetesActions.Enabled)
}
