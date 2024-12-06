// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogslo

import (
	"testing"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/DataDog/datadog-operator/api/crds/datadoghq/v1alpha1"
)

func Test_buildThreshold(t *testing.T) {
	tests := []struct {
		name           string
		mockSpec       v1alpha1.DatadogSLOSpec
		expectedResult []datadogV1.SLOThreshold
	}{
		{
			name: "Target threshold only",
			mockSpec: v1alpha1.DatadogSLOSpec{
				Name:            "test",
				Timeframe:       "7d",
				TargetThreshold: resource.MustParse("99990m"),
			},
			expectedResult: []datadogV1.SLOThreshold{
				{
					Target:    99.99,
					Timeframe: datadogV1.SLOTimeframe("7d"),
				},
			},
		},
		{
			name: "Target and warning threshold",
			mockSpec: v1alpha1.DatadogSLOSpec{
				Name:             "test",
				Timeframe:        "30d",
				TargetThreshold:  resource.MustParse("99.999"),
				WarningThreshold: ptrResourceQuantity(resource.MustParse("95.010001")),
			},
			expectedResult: []datadogV1.SLOThreshold{
				{
					Target:    99.999,
					Timeframe: datadogV1.SLOTimeframe("30d"),
					Warning:   float64Ptr(95.010001),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildThreshold(tt.mockSpec)
			if len(result) != len(tt.expectedResult) {
				t.Errorf("Expected %d thresholds, got %d", len(tt.expectedResult), len(result))
				return
			}
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func float64Ptr(f float64) *float64 {
	return &f
}

func ptrResourceQuantity(n resource.Quantity) *resource.Quantity {
	return &n
}
