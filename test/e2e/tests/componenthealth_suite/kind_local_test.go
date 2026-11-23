// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package componenthealthsuite

import (
	"fmt"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"

	"github.com/DataDog/datadog-operator/test/e2e/common"
	"github.com/DataDog/datadog-operator/test/e2e/provisioners"
)

// TestComponentHealthLocalKind runs the component-health flow on a local kind
// cluster.
func TestComponentHealthLocalKind(t *testing.T) {
	e2e.Run(t, &componentHealthSuite{local: true},
		e2e.WithStackName(fmt.Sprintf("componenthealth-localkind-%s", strings.ReplaceAll(common.K8sVersion, ".", "-"))),
		e2e.WithProvisioner(provisioners.KubernetesProvisioner(buildProvisionerOptions(true)...)),
	)
}
