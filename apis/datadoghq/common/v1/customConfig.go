// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GetConfName get the name of the Configmap for a CustomConfigSpec
func GetConfName(owner metav1.Object, conf *CustomConfig, defaultName string) string {
	// `configData` and `configMap` can't be set together.
	// Return the default if the conf is not overridden or if it is just overridden with the ConfigData.
	if conf != nil && conf.ConfigMap != nil {
		return conf.ConfigMap.Name
	}
	return fmt.Sprintf("%s-%s", owner.GetName(), defaultName)
}
