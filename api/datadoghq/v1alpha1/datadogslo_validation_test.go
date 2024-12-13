// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/resource"
	utilserrors "k8s.io/apimachinery/pkg/util/errors"
)

func TestIsValidDatadogSLO(t *testing.T) {

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
				Type:            DatadogSLOTypeMetric,
				TargetThreshold: resource.MustParse("99.99"),
				Timeframe:       DatadogSLOTimeFrame30d,
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
				Type:            DatadogSLOTypeMetric,
				TargetThreshold: resource.MustParse("99.99"),
				Timeframe:       DatadogSLOTimeFrame30d,
			},
			expected: errors.New("spec.Name must be defined"),
		},
		{
			name: "Missing Query",
			spec: &DatadogSLOSpec{
				Name:            "SLO without Query",
				Type:            DatadogSLOTypeMetric,
				TargetThreshold: resource.MustParse("99.99"),
				Timeframe:       DatadogSLOTimeFrame30d,
			},
			expected: errors.New("spec.Query must be defined when spec.Type is metric"),
		},
		{
			name: "Missing Type",
			spec: &DatadogSLOSpec{
				Name: "SLO without Type",
				Query: &DatadogSLOQuery{
					Numerator:   "good",
					Denominator: "total",
				},
				TargetThreshold: resource.MustParse("99.99"),
				Timeframe:       DatadogSLOTimeFrame30d,
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
				Type:            "invalid",
				TargetThreshold: resource.MustParse("99.99"),
				Timeframe:       DatadogSLOTimeFrame30d,
			},
			expected: errors.New("spec.Type must be one of the values: monitor or metric"),
		},
		{
			name: "Missing Threshold and Timeframe",
			spec: &DatadogSLOSpec{
				Name: "SLO without Thresholds and Timeframe",
				Query: &DatadogSLOQuery{
					Numerator:   "good",
					Denominator: "total",
				},
				Type: DatadogSLOTypeMetric,
			},
			expected: utilserrors.NewAggregate(
				[]error{
					errors.New("spec.TargetThreshold must be greater than 0 and less than 100"),
					errors.New("spec.Timeframe must be defined as one of the values: 7d, 30d, or 90d"),
				},
			),
		},
		{
			name: "Missing MonitorIDs",
			spec: &DatadogSLOSpec{
				Name:            "MySLO",
				Query:           &DatadogSLOQuery{},
				Type:            DatadogSLOTypeMonitor,
				TargetThreshold: resource.MustParse("99.99"),
				Timeframe:       DatadogSLOTimeFrame30d,
				MonitorIDs:      []int64{},
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
				Type:            DatadogSLOTypeMetric,
				TargetThreshold: resource.MustParse("0"),
				Timeframe:       DatadogSLOTimeFrame30d,
			},
			expected: errors.New("spec.TargetThreshold must be greater than 0 and less than 100"),
		},
		{
			name: "Invalid Thresholds Warning",
			spec: &DatadogSLOSpec{
				Name: "MySLO",
				Query: &DatadogSLOQuery{
					Numerator:   "good",
					Denominator: "total",
				},
				Type:             DatadogSLOTypeMetric,
				TargetThreshold:  resource.MustParse("98.00"),
				Timeframe:        DatadogSLOTimeFrame30d,
				WarningThreshold: ptrResourceQuantity(resource.MustParse("0")),
			},
			expected: errors.New("spec.WarningThreshold must be greater than 0 and less than 100"),
		},
		{
			name: "Invalid Thresholds Timeframe",
			spec: &DatadogSLOSpec{
				Name: "MySLO",
				Query: &DatadogSLOQuery{
					Numerator:   "good",
					Denominator: "total",
				},
				Type:            DatadogSLOTypeMetric,
				TargetThreshold: resource.MustParse("98.00"),
				Timeframe:       "invalid",
			},
			expected: errors.New("spec.Timeframe must be defined as one of the values: 7d, 30d, or 90d"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsValidDatadogSLO(tt.spec)
			if tt.expected != nil {
				assert.EqualError(t, result, tt.expected.Error())
			} else {
				assert.Nil(t, result)
			}
		})
	}
}

func ptrResourceQuantity(n resource.Quantity) *resource.Quantity {
	return &n
}
