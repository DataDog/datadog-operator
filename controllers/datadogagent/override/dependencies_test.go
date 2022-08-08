// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"testing"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/dependencies"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/stretchr/testify/assert"
)

func TestDependencies(t *testing.T) {
	// These tests are not exhaustive. There's only 1 that covers a bug fix.

	namespace := "test-namespace"

	tests := []struct {
		name          string
		dda           v2alpha1.DatadogAgent
		overrides     map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride
		expectsErrors bool
	}{
		{
			name: "override without errors",
			dda:  v2alpha1.DatadogAgent{},
			overrides: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
				v2alpha1.NodeAgentComponentName: {},
			},
			expectsErrors: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := dependencies.NewStore(&test.dda, nil)
			manager := feature.NewResourceManagers(store)

			errs := Dependencies(manager, test.overrides, namespace)

			if test.expectsErrors {
				assert.NotEmpty(t, errs)
			} else {
				assert.Empty(t, errs)
			}
		})
	}
}
