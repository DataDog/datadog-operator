// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2023 Datadog, Inc.

package datadogslo

import (
	datadogapiclientv1 "github.com/DataDog/datadog-api-client-go/api/v1/datadog"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
	"testing"
)

func TestBuildThreshold(t *testing.T) {
	tests := []struct {
		name           string
		thresholds     []datadoghqv2alpha1.DatadogSLOThreshold
		expectedResult []datadogapiclientv1.SLOThreshold
	}{
		{
			name: "Single threshold",
			thresholds: []datadoghqv2alpha1.DatadogSLOThreshold{
				{
					Target:         resource.MustParse("99.9"),
					TargetDisplay:  "99.90",
					Timeframe:      "7d",
					Warning:        ptrResourceQuantity(resource.MustParse("98.0")),
					WarningDisplay: "98.00",
				},
			},
			expectedResult: []datadogapiclientv1.SLOThreshold{
				{
					Target:         99.9,
					TargetDisplay:  stringPtr("99.90"),
					Timeframe:      datadogapiclientv1.SLOTimeframe("7d"),
					Warning:        float64Ptr(98.0),
					WarningDisplay: stringPtr("98.00"),
				},
			},
		},
		{
			name: "Multiple thresholds",
			thresholds: []datadoghqv2alpha1.DatadogSLOThreshold{
				{
					Target:         resource.MustParse("99.9"),
					TargetDisplay:  "99.90",
					Timeframe:      "7d",
					Warning:        ptrResourceQuantity(resource.MustParse("98.0")),
					WarningDisplay: "98.00",
				},
				{
					Target:        resource.MustParse("99.5"),
					TargetDisplay: "99.50",
					Timeframe:     "30d",
				},
			},
			expectedResult: []datadogapiclientv1.SLOThreshold{
				{
					Target:         99.9,
					TargetDisplay:  stringPtr("99.90"),
					Timeframe:      datadogapiclientv1.SLOTimeframe("7d"),
					Warning:        float64Ptr(98.0),
					WarningDisplay: stringPtr("98.00"),
				},
				{
					Target:        99.5,
					TargetDisplay: stringPtr("99.50"),
					Timeframe:     datadogapiclientv1.SLOTimeframe("30d"),
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildThreshold(tt.thresholds)
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
