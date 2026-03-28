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

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
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

func strPtr(s string) *string {
	return &s
}

func Test_buildSLO(t *testing.T) {
	tests := []struct {
		name              string
		crdSLO            *v1alpha1.DatadogSLO
		validateRequest   func(t *testing.T, req *datadogV1.ServiceLevelObjectiveRequest)
		validateUpdateObj func(t *testing.T, slo *datadogV1.ServiceLevelObjective)
	}{
		{
			name: "metric SLO sets query, no sli_specification",
			crdSLO: &v1alpha1.DatadogSLO{
				Spec: v1alpha1.DatadogSLOSpec{
					Name:        "metric-slo",
					Description: strPtr("a metric SLO"),
					Type:        v1alpha1.DatadogSLOTypeMetric,
					Query: &v1alpha1.DatadogSLOQuery{
						Numerator:   "sum:good.events{*}.as_count()",
						Denominator: "sum:total.events{*}.as_count()",
					},
					Tags:            []string{"env:prod"},
					TargetThreshold: resource.MustParse("99.9"),
					Timeframe:       v1alpha1.DatadogSLOTimeFrame7d,
				},
			},
			validateRequest: func(t *testing.T, req *datadogV1.ServiceLevelObjectiveRequest) {
				assert.Equal(t, datadogV1.SLOType("metric"), req.GetType())
				assert.Equal(t, "metric-slo", req.GetName())
				assert.Equal(t, "a metric SLO", req.GetDescription())
				assert.Equal(t, []string{"env:prod"}, req.GetTags())

				query := req.GetQuery()
				assert.Equal(t, "sum:good.events{*}.as_count()", query.Numerator)
				assert.Equal(t, "sum:total.events{*}.as_count()", query.Denominator)

				_, hasSliSpec := req.GetSliSpecificationOk()
				assert.False(t, hasSliSpec)
			},
			validateUpdateObj: func(t *testing.T, slo *datadogV1.ServiceLevelObjective) {
				assert.Equal(t, datadogV1.SLOType("metric"), slo.GetType())
				query := slo.GetQuery()
				assert.Equal(t, "sum:good.events{*}.as_count()", query.Numerator)
			},
		},
		{
			name: "monitor SLO sets monitor IDs and groups, no query or sli_specification",
			crdSLO: &v1alpha1.DatadogSLO{
				Spec: v1alpha1.DatadogSLOSpec{
					Name:            "monitor-slo",
					Type:            v1alpha1.DatadogSLOTypeMonitor,
					MonitorIDs:      []int64{12345, 67890},
					Groups:          []string{"env:prod"},
					Tags:            []string{"team:backend"},
					TargetThreshold: resource.MustParse("97"),
					Timeframe:       v1alpha1.DatadogSLOTimeFrame30d,
				},
			},
			validateRequest: func(t *testing.T, req *datadogV1.ServiceLevelObjectiveRequest) {
				assert.Equal(t, datadogV1.SLOType("monitor"), req.GetType())
				assert.Equal(t, []int64{12345, 67890}, req.GetMonitorIds())
				assert.Equal(t, []string{"env:prod"}, req.GetGroups())

				_, hasQuery := req.GetQueryOk()
				assert.False(t, hasQuery)

				_, hasSliSpec := req.GetSliSpecificationOk()
				assert.False(t, hasSliSpec)
			},
			validateUpdateObj: func(t *testing.T, slo *datadogV1.ServiceLevelObjective) {
				assert.Equal(t, datadogV1.SLOType("monitor"), slo.GetType())
				assert.Equal(t, []int64{12345, 67890}, slo.GetMonitorIds())
			},
		},
		{
			name: "time_slice SLO sets sli_specification with auto-generated formula and named query",
			crdSLO: &v1alpha1.DatadogSLO{
				Spec: v1alpha1.DatadogSLOSpec{
					Name:        "timeslice-slo",
					Description: strPtr("a time slice SLO"),
					Type:        v1alpha1.DatadogSLOTypeTimeSlice,
					TimeSlice: &v1alpha1.DatadogSLOTimeSlice{
						Query:      "trace.servlet.request{env:prod}",
						Comparator: v1alpha1.DatadogSLOTimeSliceComparatorGreater,
						Threshold:  resource.MustParse("5"),
					},
					Tags:            []string{"env:prod"},
					TargetThreshold: resource.MustParse("97"),
					Timeframe:       v1alpha1.DatadogSLOTimeFrame7d,
				},
			},
			validateRequest: func(t *testing.T, req *datadogV1.ServiceLevelObjectiveRequest) {
				assert.Equal(t, datadogV1.SLOType("time_slice"), req.GetType())
				assert.Equal(t, "timeslice-slo", req.GetName())
				assert.Equal(t, "a time slice SLO", req.GetDescription())

				_, hasQuery := req.GetQueryOk()
				assert.False(t, hasQuery)

				sliSpec, hasSliSpec := req.GetSliSpecificationOk()
				assert.True(t, hasSliSpec)

				timeSliceSpec := sliSpec.SLOTimeSliceSpec
				assert.NotNil(t, timeSliceSpec)

				condition := timeSliceSpec.TimeSlice
				assert.Equal(t, datadogV1.SLOTimeSliceComparator(">"), condition.Comparator)
				assert.Equal(t, float64(5), condition.Threshold)

				// Verify auto-generated formula references the auto-generated query name
				assert.Len(t, condition.Query.Formulas, 1)
				assert.Equal(t, "query1", condition.Query.Formulas[0].Formula)

				// Verify single auto-generated named query with hardcoded metrics data source
				assert.Len(t, condition.Query.Queries, 1)
				metricQuery := condition.Query.Queries[0].FormulaAndFunctionMetricQueryDefinition
				assert.NotNil(t, metricQuery)
				assert.Equal(t, datadogV1.FORMULAANDFUNCTIONMETRICDATASOURCE_METRICS, metricQuery.DataSource)
				assert.Equal(t, "query1", metricQuery.Name)
				assert.Equal(t, "trace.servlet.request{env:prod}", metricQuery.Query)
			},
			validateUpdateObj: func(t *testing.T, slo *datadogV1.ServiceLevelObjective) {
				assert.Equal(t, datadogV1.SLOType("time_slice"), slo.GetType())

				sliSpec, hasSliSpec := slo.GetSliSpecificationOk()
				assert.True(t, hasSliSpec)
				assert.NotNil(t, sliSpec.SLOTimeSliceSpec)

				condition := sliSpec.SLOTimeSliceSpec.TimeSlice
				assert.Equal(t, datadogV1.SLOTimeSliceComparator(">"), condition.Comparator)
				assert.Equal(t, float64(5), condition.Threshold)
				assert.Equal(t, "trace.servlet.request{env:prod}", condition.Query.Queries[0].FormulaAndFunctionMetricQueryDefinition.Query)
			},
		},
		{
			name: "time_slice SLO with less-equal comparator and fractional threshold",
			crdSLO: &v1alpha1.DatadogSLO{
				Spec: v1alpha1.DatadogSLOSpec{
					Name: "latency-timeslice",
					Type: v1alpha1.DatadogSLOTypeTimeSlice,
					TimeSlice: &v1alpha1.DatadogSLOTimeSlice{
						Query:      "avg:trace.servlet.request.duration{service:data-model-manager}",
						Comparator: v1alpha1.DatadogSLOTimeSliceComparatorLessEqual,
						Threshold:  resource.MustParse("0.5"),
					},
					TargetThreshold: resource.MustParse("95"),
					Timeframe:       v1alpha1.DatadogSLOTimeFrame30d,
				},
			},
			validateRequest: func(t *testing.T, req *datadogV1.ServiceLevelObjectiveRequest) {
				sliSpec := req.GetSliSpecification()
				condition := sliSpec.SLOTimeSliceSpec.TimeSlice

				assert.Equal(t, datadogV1.SLOTimeSliceComparator("<="), condition.Comparator)
				assert.Equal(t, 0.5, condition.Threshold)

				assert.Len(t, condition.Query.Formulas, 1)
				assert.Equal(t, "query1", condition.Query.Formulas[0].Formula)

				assert.Len(t, condition.Query.Queries, 1)
				assert.Equal(t, "avg:trace.servlet.request.duration{service:data-model-manager}",
					condition.Query.Queries[0].FormulaAndFunctionMetricQueryDefinition.Query)
			},
			validateUpdateObj: func(t *testing.T, slo *datadogV1.ServiceLevelObjective) {
				sliSpec := slo.GetSliSpecification()
				condition := sliSpec.SLOTimeSliceSpec.TimeSlice
				assert.Equal(t, datadogV1.SLOTimeSliceComparator("<="), condition.Comparator)
				assert.Equal(t, 0.5, condition.Threshold)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, slo := buildSLO(tt.crdSLO)

			assert.NotNil(t, req)
			assert.NotNil(t, slo)

			if tt.validateRequest != nil {
				tt.validateRequest(t, req)
			}
			if tt.validateUpdateObj != nil {
				tt.validateUpdateObj(t, slo)
			}
		})
	}
}
