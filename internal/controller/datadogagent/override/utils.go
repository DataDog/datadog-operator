// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

func getDefaultConfigMapName(ddaName, fileName string) string {
	return fmt.Sprintf("%s-%s-yaml", ddaName, strings.Split(fileName, ".")[0])
}

func hasProbeHandler(probe *corev1.Probe) bool {
	handler := &probe.ProbeHandler
	if handler.Exec != nil || handler.HTTPGet != nil || handler.TCPSocket != nil || handler.GRPC != nil {
		return true
	}
	return false
}
