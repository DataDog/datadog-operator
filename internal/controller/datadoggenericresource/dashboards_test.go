// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func Test_DashboardHandler_getHandler(t *testing.T) {
	handler := getHandler(v1alpha1.Dashboard)
	assert.IsType(t, &DashboardHandler{}, handler)
}
