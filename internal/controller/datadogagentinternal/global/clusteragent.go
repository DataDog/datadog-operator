// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package global

import (
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature"
)

func applyClusterAgentResources(manager feature.PodTemplateManagers, dda *v2alpha1.DatadogAgent) {
	// Registry is the image registry to use for all Agent images.
	setImageRegistry(manager, dda, v2alpha1.ClusterAgentComponentName)

}
