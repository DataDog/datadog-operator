// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2023 Datadog, Inc.

package v1alpha1

import (
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
	"testing"

	"github.com/pkg/errors"
)

func TestIsValidDatadogSLO(t *testing.T) {

	validThresholds := []DatadogSLOThreshold{
		{
			Target:    resource.MustParse("99.99"),
			Timeframe: DatadogSLOTimeFrame30d,
		},
	}

	tests := []struct {
		name     string
		spec     *DatadogSLOSpec
		expected error
	}{
		{
			name: "Valid spec",
			spec: &DatadogSLOSpec{
				Name: "MySLO",
				Query: &DatadogSLOQuery{
					Numerator:   "good",
					Denominator: "total",
				},
				Type:       DatadogSLOTypeMetric,
				Thresholds: validThresholds,
			},
			expected: nil,
		},
		{
			name: "Missing Name",
			spec: &DatadogSLOSpec{
				Query: &DatadogSLOQuery{
					Numerator:   "good",
					Denominator: "total",
				},
				Type:       DatadogSLOTypeMetric,
				Thresholds: validThresholds,
			},
			expected: errors.New("spec.Name must be defined"),
		},
		{
			name: "Missing Query",
			spec: &DatadogSLOSpec{
				Name:       "SLO without Query",
				Type:       DatadogSLOTypeMetric,
				Thresholds: validThresholds,
			},
			expected: errors.New("spec.Query must be defined"),
		},
		{
			name: "Missing Type",
			spec: &DatadogSLOSpec{
				Name: "SLO without Type",
				Query: &DatadogSLOQuery{
					Numerator:   "good",
					Denominator: "total",
				},
				Thresholds: validThresholds,
			},
			expected: errors.New("spec.Type must be defined"),
		},
		{
			name: "Invalid Type",
			spec: &DatadogSLOSpec{
				Name: "MySLO",
				Query: &DatadogSLOQuery{
					Numerator:   "good",
					Denominator: "total",
				},
				Type:       "invalid",
				Thresholds: validThresholds,
			},
			expected: errors.New("spec.Type must be one of the values: monitor or metric"),
		},
		{
			name: "Missing Thresholds",
			spec: &DatadogSLOSpec{
				Name: "SLO without Thresholds",
				Query: &DatadogSLOQuery{
					Numerator:   "good",
					Denominator: "total",
				},
				Type: DatadogSLOTypeMetric,
			},
			expected: errors.New("spec.Thresholds must be defined"),
		},
		{
			name: "Missing MonitorIDs",
			spec: &DatadogSLOSpec{
				Name:       "MySLO",
				Query:      &DatadogSLOQuery{},
				Type:       DatadogSLOTypeMonitor,
				Thresholds: validThresholds,
				MonitorIDs: []int64{},
			},
			expected: errors.New("spec.MonitorIDs must be defined when spec.Type is monitor"),
		},
		{
			name: "Invalid Thresholds",
			spec: &DatadogSLOSpec{
				Name: "MySLO",
				Query: &DatadogSLOQuery{
					Numerator:   "good",
					Denominator: "total",
				},
				Type: DatadogSLOTypeMetric,
				Thresholds: []DatadogSLOThreshold{
					{
						Target:         resource.MustParse("0"),
						TargetDisplay:  "98.00",
						Timeframe:      DatadogSLOTimeFrame7d,
						Warning:        nil,
						WarningDisplay: "",
					},
				},
			},
			expected: errors.New("spec.Thresholds.Target must be defined and greater than 0"),
		},
		{
			name: "Invalid Thresholds Warning",
			spec: &DatadogSLOSpec{
				Name: "MySLO",
				Query: &DatadogSLOQuery{
					Numerator:   "good",
					Denominator: "total",
				},
				Type: DatadogSLOTypeMetric,
				Thresholds: []DatadogSLOThreshold{
					{
						Target:         resource.MustParse("98.00"),
						TargetDisplay:  "98.00",
						Timeframe:      DatadogSLOTimeFrame7d,
						Warning:        ptrResourceQuantity(resource.MustParse("0")),
						WarningDisplay: "",
					},
				},
			},
			expected: errors.New("spec.Thresholds.Warning must be greater than 0"),
		},
		{
			name: "Invalid Thresholds Timeframe",
			spec: &DatadogSLOSpec{
				Name: "MySLO",
				Query: &DatadogSLOQuery{
					Numerator:   "good",
					Denominator: "total",
				},
				Type: DatadogSLOTypeMetric,
				Thresholds: []DatadogSLOThreshold{
					{
						Target:         resource.MustParse("98.00"),
						TargetDisplay:  "98.00",
						Timeframe:      "invalid",
						Warning:        nil,
						WarningDisplay: "",
					},
				},
			},
			expected: errors.New("spec.Thresholds.Timeframe must be defined as one of the values: 7d, 30d, 90d, or custom"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidDatadogSLO(tt.spec)
			if tt.expected != nil {
				assert.EqualError(t, tt.expected, result.Error())
			} else {
				assert.Nil(t, result)
			}
		})
	}
}

func ptrResourceQuantity(n resource.Quantity) *resource.Quantity {
	return &n
}
