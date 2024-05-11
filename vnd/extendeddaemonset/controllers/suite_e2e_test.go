// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

//go:build e2e
// +build e2e

package controllers

import (
	"fmt"
	"time"
)

func initTestConfig() *testConfigOptions {
	return &testConfigOptions{
		useExistingCluster: true,
		crdVersion:         "v1",
		namespace:          fmt.Sprintf("eds-%d", time.Now().Unix()),
	}
}
