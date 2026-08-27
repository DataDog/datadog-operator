// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func init() {
	err := register(kubernetes.GKECosProvider, gkeCos)
	if err != nil {
		panic(err)
	}
}

// gkeCos is GKE with Container-Optimized OS nodes, which have no /usr/src.
var gkeCos = Provider{
	volumes: map[string]Rule[corev1.Volume]{
		common.SrcVolumeName: {Verb: Deny},
	},
	volumeMounts: map[string]Rule[corev1.VolumeMount]{
		common.SrcVolumeName: {Verb: Deny},
	},
}
