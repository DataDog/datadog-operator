// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package global

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	ccr "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusterchecksrunner"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

func applyClusterChecksRunnerResources(manager feature.PodTemplateManagers, ddaSpec *v2alpha1.DatadogAgentSpec) {
	// Construct DD_HOSTNAME from the intermediate DD_CCR_NODE_NAME env var.
	// If a cluster name is configured, append it as a suffix to match the
	// hostname format used by the node agent, preventing ghost hosts in
	// Remote Configuration.
	hostnameValue := fmt.Sprintf("$(%s)", ccr.DDCCRNodeName)
	if ddaSpec.Global != nil && ddaSpec.Global.ClusterName != nil && *ddaSpec.Global.ClusterName != "" {
		hostnameValue = fmt.Sprintf("$(%s)-%s", ccr.DDCCRNodeName, *ddaSpec.Global.ClusterName)
	}

	manager.EnvVar().AddEnvVar(&corev1.EnvVar{
		Name:  constants.DDHostName,
		Value: hostnameValue,
	})
}
