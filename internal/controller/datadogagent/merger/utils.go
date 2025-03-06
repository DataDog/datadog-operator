// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"fmt"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
)

// AllAgentContainers is a map of all agent containers
var AllAgentContainers = map[common.AgentContainerName]struct{}{
	// Node agent containers
	common.CoreAgentContainerName:      {},
	common.TraceAgentContainerName:     {},
	common.ProcessAgentContainerName:   {},
	common.SecurityAgentContainerName:  {},
	common.SystemProbeContainerName:    {},
	common.OtelAgent:                   {},
	common.AgentDataPlaneContainerName: {},
	// DCA containers
	common.ClusterAgentContainerName: {},
	// CCR container name is equivalent to core agent container name
	// Single Agent container
	common.UnprivilegedSingleAgentContainerName: {},
}

func AddChecksumAnnotation(logger logr.Logger, data interface{}, obj metav1.Object) {
	objName := obj.GetName()
	objNS := obj.GetNamespace()
	hash, err := comparison.GenerateMD5ForSpec(data)
	if err != nil {
		logger.Error(err, "couldn't generate hash", "name", objName, "namespace", objNS)
	} else {
		logger.V(2).Info("built hash", "hash", hash, "name", objName, "namespace", objNS)
	}
	extraAnnotations := map[string]string{
		object.GetChecksumAnnotationKey(fmt.Sprintf("%s-%s", objNS, objName)): hash,
	}
	obj.SetAnnotations(object.MergeAnnotationsLabels(logger, obj.GetAnnotations(), extraAnnotations, "*"))
}
