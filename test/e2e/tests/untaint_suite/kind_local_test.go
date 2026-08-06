// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package untaintsuite

import (
	"fmt"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"

	"github.com/DataDog/datadog-operator/test/e2e/common"
	"github.com/DataDog/datadog-operator/test/e2e/provisioners"
)

// TestUntaintAgentOnlyLocalKind runs the agent-only untaint flow on a local kind
// cluster (control-plane + one untainted worker + one pre-tainted worker).
func TestUntaintAgentOnlyLocalKind(t *testing.T) {
	const waitForCSI = false
	e2e.Run(t, &untaintSuite{local: true, waitForCSI: waitForCSI},
		e2e.WithStackName(fmt.Sprintf("untaint-agent-localkind-%s", strings.ReplaceAll(common.K8sVersion, ".", "-"))),
		e2e.WithProvisioner(provisioners.KubernetesProvisioner(buildProvisionerOptions(true, waitForCSI, false)...)),
	)
}

// TestUntaintWaitForCSILocalKind runs the dual-readiness (Agent + CSI) untaint
// flow on a local kind cluster.
func TestUntaintWaitForCSILocalKind(t *testing.T) {
	const waitForCSI = true
	e2e.Run(t, &untaintSuite{local: true, waitForCSI: waitForCSI},
		e2e.WithStackName(fmt.Sprintf("untaint-csi-localkind-%s", strings.ReplaceAll(common.K8sVersion, ".", "-"))),
		e2e.WithProvisioner(provisioners.KubernetesProvisioner(buildProvisionerOptions(true, waitForCSI, false)...)),
	)
}
