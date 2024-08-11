package datadogdashboard

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	v1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// NOTE: move to types test
var (
	testLogger logr.Logger = zap.New(zap.UseDevMode(true))
)

const dateFormat = "2006-01-02 15:04:05.999999999 -0700 MST"

func TestBuildDashboard(t *testing.T) {
	templateVariables := []v1alpha1.DashboardTemplateVariable{
		{
			AvailableValues: v1alpha1.NullableList{
				Value: &[]string{
					"foo",
					"bar",
				},
			},
			Name: "test variable",
		},
	}
	templateVariablePresets := []v1alpha1.DashboardTemplateVariablePreset{
		{
			Name: apiutils.NewStringPointer("test preset"),
			TemplateVariables: []v1alpha1.DashboardTemplateVariablePresetValue{
				{
					Name: apiutils.NewStringPointer("foo-bar"),
					Values: []string{
						"foo",
						"bar",
					},
				},
			},
		},
	}

	db := &v1alpha1.DatadogDashboard{
		Spec: v1alpha1.DatadogDashboardSpec{
			LayoutType: "ordered",
			NotifyList: []string{
				"foo",
				"bar",
			},
			ReflowType: datadogV1.DASHBOARDREFLOWTYPE_AUTO.Ptr(),
			Tags: []string{
				"team:test",
				"team:foo-bar",
			},
			TemplateVariablePresets: templateVariablePresets,
			TemplateVariables:       templateVariables,
			Title:                   "test dashboard",
			Widgets: []v1alpha1.Widget{
				{
					TimeseriesWidgetDefinition: &v1alpha1.TimeseriesWidgetDefinition{
						CustomLinks: []v1alpha1.WidgetCustomLink{
							{
								IsHidden: apiutils.NewBoolPointer(true),
								Label:    apiutils.NewStringPointer("example"),
								Link:     apiutils.NewStringPointer("team:test"),
							},
						},
						Events: []v1alpha1.WidgetEvent{
							{
								Q:             "foo-bar",
								TagsExecution: apiutils.NewStringPointer("foo-bar"),
							},
						},
						LegendColumns: []datadogV1.TimeseriesWidgetLegendColumn{
							// string types
							"value",
							"avg",
						},
						LegendLayout: datadogV1.TIMESERIESWIDGETLEGENDLAYOUT_HORIZONTAL.Ptr(),
						LegendSize:   apiutils.NewStringPointer("10"),
						// NOTE: test this out (by setting it )
						Markers: []v1alpha1.WidgetMarker{
							{
								DisplayType: apiutils.NewStringPointer("warning"),
								Label:       apiutils.NewStringPointer("marker label"),
								// NOTE: may cause issues
								Time:  apiutils.NewStringPointer("6:30"),
								Value: "y = 15",
							},
						},
						Requests: []v1alpha1.TimeseriesWidgetRequest{
							{
								// LogQuery: &v1alpha1.LogQueryDefinition{
								// 	Compute: &v1alpha1.LogsQueryCompute{
								// 		Aggregation: "count",
								// 		Facet:       apiutils.NewStringPointer("source"),
								// 		Interval:    apiutils.NewInt64Pointer(5000),
								// 	},
								// 	GroupBy: []v1alpha1.LogQueryDefinitionGroupBy{
								// 		v1alpha1.LogQueryDefinitionGroupBy{
								// 			Facet: "source",
								// 			Limit: apiutils.NewInt64Pointer(10),
								// 			Sort: &v1alpha1.LogQueryDefinitionGroupBySort{
								// 				Aggregation: "count",
								// 				Facet:       apiutils.NewStringPointer("source"),
								// 				Order:       "asc",
								// 			},
								// 		},
								// 	},
								// },
								Queries: []v1alpha1.FormulaAndFunctionQueryDefinition{
									{
										FormulaAndFunctionMetricQueryDefinition: &v1alpha1.FormulaAndFunctionMetricQueryDefinition{
											Aggregator: datadogV1.FORMULAANDFUNCTIONMETRICAGGREGATION_AVG.Ptr(),
											// NOTE: CrossOrg UIDs in private beta
											DataSource: "metrics",
											Name:       "query1",
											Query:      "avg:system.cpu.user{*}",
										},
									},
									{
										FormulaAndFunctionEventQueryDefinition: &v1alpha1.FormulaAndFunctionEventQueryDefinition{
											Compute: &v1alpha1.FormulaAndFunctionEventQueryDefinitionCompute{
												Aggregation: "count",
												Interval:    apiutils.NewInt64Pointer(5000),
											},
											// NOTE: Crossuids is in private beta
											DataSource: *datadogV1.FORMULAANDFUNCTIONEVENTSDATASOURCE_LOGS.Ptr(),
											GroupBy:    []v1alpha1.FormulaAndFunctionEventQueryGroupBy{},
											Name:       "logs",
											Indexes:    []string{"*"},
											Search: &v1alpha1.FormulaAndFunctionEventQueryDefinitionSearch{
												Query: "kube_namespace:system",
											},
											// NOTE: Strorage is in private beta
										},
									},
									{
										FormulaAndFunctionProcessQueryDefinition: &v1alpha1.FormulaAndFunctionProcessQueryDefinition{
											Aggregator: datadogV1.FORMULAANDFUNCTIONMETRICAGGREGATION_AREA.Ptr(),
											// NOTE: CrossOrgUids
											DataSource: "container",
											// IsNormalizedCpu: apiutils.NewBoolPointer(true),
											Limit:      apiutils.NewInt64Pointer(10),
											Metric:     "process.stat.cpu.total_pct",
											Name:       "query2",
											Sort:       datadogV1.QUERYSORTORDER_ASC.Ptr(),
											TagFilters: []string{"team:test"},
											TextFilter: apiutils.NewStringPointer("foo-bar"),
										},
									},
									// v1alpha1.FormulaAndFunctionQueryDefinition{
									// 	FormulaAndFunctionApmDependencyStatsQueryDefinition: &v1alpha1.FormulaAndFunctionApmDependencyStatsQueryDefinition{

									// 	},
									// },
									{
										FormulaAndFunctionSLOQueryDefinition: &v1alpha1.FormulaAndFunctionSLOQueryDefinition{
											AdditionalQueryFilters: apiutils.NewStringPointer(""),
											DataSource:             datadogV1.FORMULAANDFUNCTIONSLODATASOURCE_SLO,
											GroupMode:              datadogV1.FORMULAANDFUNCTIONSLOGROUPMODE_OVERALL.Ptr(),
											Measure:                datadogV1.FORMULAANDFUNCTIONSLOMEASURE_SLO_STATUS,
											Name:                   apiutils.NewStringPointer("query3"),
											SloId:                  "foobar123",
											SloQueryType:           datadogV1.FORMULAANDFUNCTIONSLOQUERYTYPE_METRIC.Ptr(),
										},
									},
									{
										FormulaAndFunctionSLOQueryDefinition: &v1alpha1.FormulaAndFunctionSLOQueryDefinition{
											AdditionalQueryFilters: apiutils.NewStringPointer(""),
											DataSource:             datadogV1.FORMULAANDFUNCTIONSLODATASOURCE_SLO,
											GroupMode:              datadogV1.FORMULAANDFUNCTIONSLOGROUPMODE_OVERALL.Ptr(),
											Measure:                datadogV1.FORMULAANDFUNCTIONSLOMEASURE_SLO_STATUS,
											Name:                   apiutils.NewStringPointer("query3"),
											SloId:                  "foobar123",
											SloQueryType:           datadogV1.FORMULAANDFUNCTIONSLOQUERYTYPE_METRIC.Ptr(),
										},
									},
									{
										FormulaAndFunctionCloudCostQueryDefinition: &v1alpha1.FormulaAndFunctionCloudCostQueryDefinition{
											Aggregator: datadogV1.WIDGETAGGREGATOR_AVERAGE.Ptr(),
											DataSource: "cloud_cost",
											Name:       "query1",
											Query:      "sum:aws.cost.amortized{*}.rollup(sum, 86400)",
										},
									},
								},
							},
						},
						RightYaxis: &v1alpha1.WidgetAxis{},
						ShowLegend: apiutils.NewBoolPointer(true),
						Time: &v1alpha1.WidgetTime{
							LiveSpan: datadogV1.WIDGETLIVESPAN_PAST_THIRTY_MINUTES.Ptr(),
						},
						Title: apiutils.NewStringPointer("ts graph"),
						// NOTE: Title align, Title Size don't have an effect
						Type: "timeseries",
					},
					Id: apiutils.NewInt64Pointer(123),
				},
				// v1alpha1.Widget{
				// 	QueryValueWidgetDefinition: &v1alpha1.QueryValueWidgetDefinition{
				// 		Autoscale: apiutils.NewBoolPointer(true),
				// 		CustomLinks: []v1alpha1.WidgetCustomLink{
				// 			v1alpha1.WidgetCustomLink{
				// 				IsHidden: apiutils.NewBoolPointer(true),
				// 				Label:    apiutils.NewStringPointer("example"),
				// 				Link:     apiutils.NewStringPointer("team:test"),
				// 			},
				// 		},
				// 		Precision: apiutils.NewInt64Pointer(2),
				// 		Requests:  apiutils,
				// 	},
				// },
			},
		},
	}

	dashboard := buildDashboard(testLogger, db)

	assert.Equal(t, datadogV1.DashboardLayoutType(db.Spec.LayoutType), dashboard.GetLayoutType(), "discrepancy found in parameter: Query")
	assert.Equal(t, db.Spec.NotifyList, dashboard.GetNotifyList(), "discrepancy found in parameter: NotifyList")
	assert.Equal(t, datadogV1.DashboardReflowType(*db.Spec.ReflowType), dashboard.GetReflowType(), "discrepancy found in parameter: ReflowType")
	assert.Equal(t, db.Spec.Tags, dashboard.GetTags(), "discrepancy found in parameter: Tags")
	assert.Equal(t, db.Spec.Title, dashboard.GetTitle(), "discrepancy found in parameter: Title")

	// assert.Equal(t, dashboard, expectedDashboard())
	// Compare created dashboard from CRD to what it is expected to be
	assert.Equal(t, dashboard.Widgets[0].Definition.TimeseriesWidgetDefinition, expectedTimeSeries(), "discrepancy found in parameter: TimeseriesWidgetDefinition")
	// assert.Equal(t, db.Spec.Widgets[)

	// Test timeseries (use conversion functions)
}

// func Test_getDashboard(t *testing.T) {
// 	dbID := 12345

// 	dm
// }

// func Test_validateDashboard(t *testing.T) {

// }

// func Test_createDashboard(t *testing.T) {
// 	dmId := 12345
// 	expectedDashboard := genericDashboard(dmId)

// }

// func Test_updateDashboard(t *testing.T) {

// }

// func Test_deleteDashboard(t *testing.T) {

// }

func expectedDashboard() datadogV1.Dashboard {
	notifyList := datadogapi.NullableList[string]{}
	notifyList.Set(&[]string{
		"foo",
		"bar",
	})
	tags := datadogapi.NullableList[string]{}
	tags.Set(&[]string{
		"team:test",
		"team:foo-bar",
	})
	availableValues := datadogapi.NullableList[string]{}
	notifyList.Set(&[]string{
		"foo",
		"bar",
	})

	dashboard := datadogV1.Dashboard{
		LayoutType: "ordered",
		NotifyList: notifyList,
		ReflowType: datadogV1.DASHBOARDREFLOWTYPE_AUTO.Ptr(),
		Tags:       tags,
		TemplateVariablePresets: []datadogV1.DashboardTemplateVariablePreset{
			{
				Name: apiutils.NewStringPointer("test preset"),
				TemplateVariables: []datadogV1.DashboardTemplateVariablePresetValue{
					{
						Name: apiutils.NewStringPointer("foo-bar"),
						Values: []string{
							"foo",
							"bar",
						},
					},
				},
			},
		},
		TemplateVariables: []datadogV1.DashboardTemplateVariable{
			{
				AvailableValues: availableValues,
				Name:            "test variable",
			},
		},
		Title: "test dashboard",
		Widgets: []datadogV1.Widget{
			{
				Definition: datadogV1.WidgetDefinition{
					TimeseriesWidgetDefinition: &datadogV1.TimeseriesWidgetDefinition{
						CustomLinks: []datadogV1.WidgetCustomLink{
							{
								IsHidden: apiutils.NewBoolPointer(true),
								Label:    apiutils.NewStringPointer("example"),
								Link:     apiutils.NewStringPointer("team:test"),
							},
						},
						Events: []datadogV1.WidgetEvent{
							{
								Q:             "foo-bar",
								TagsExecution: apiutils.NewStringPointer("foo-bar"),
							},
						},
						LegendColumns: []datadogV1.TimeseriesWidgetLegendColumn{
							// string types
							"value",
							"avg",
						},
						LegendLayout: datadogV1.TIMESERIESWIDGETLEGENDLAYOUT_HORIZONTAL.Ptr(),
						LegendSize:   apiutils.NewStringPointer("10"),
						// NOTE: test this out (by setting it )
						Markers: []datadogV1.WidgetMarker{
							{
								DisplayType: apiutils.NewStringPointer("warning"),
								Label:       apiutils.NewStringPointer("marker label"),
								// NOTE: may cause issues
								Time:  apiutils.NewStringPointer("6:30"),
								Value: "y = 15",
							},
						},
						Requests: []datadogV1.TimeseriesWidgetRequest{
							{
								// LogQuery: &datadogV1.LogQueryDefinition{
								// 	Compute: &datadogV1.LogsQueryCompute{
								// 		Aggregation: "count",
								// 		Facet:       apiutils.NewStringPointer("source"),
								// 		Interval:    apiutils.NewInt64Pointer(5000),
								// 	},
								// 	GroupBy: []datadogV1.LogQueryDefinitionGroupBy{
								// 		datadogV1.LogQueryDefinitionGroupBy{
								// 			Facet: "source",
								// 			Limit: apiutils.NewInt64Pointer(10),
								// 			Sort: &datadogV1.LogQueryDefinitionGroupBySort{
								// 				Aggregation: "count",
								// 				Facet:       apiutils.NewStringPointer("source"),
								// 				Order:       "asc",
								// 			},
								// 		},
								// 	},
								// },
								Queries: []datadogV1.FormulaAndFunctionQueryDefinition{
									{
										FormulaAndFunctionMetricQueryDefinition: &datadogV1.FormulaAndFunctionMetricQueryDefinition{
											Aggregator: datadogV1.FORMULAANDFUNCTIONMETRICAGGREGATION_AVG.Ptr(),
											// NOTE: CrossOrg UIDs in private beta
											DataSource: "metrics",
											Name:       "query1",
											Query:      "avg:system.cpu.user{*}",
										},
									},
									{
										FormulaAndFunctionEventQueryDefinition: &datadogV1.FormulaAndFunctionEventQueryDefinition{
											Compute: datadogV1.FormulaAndFunctionEventQueryDefinitionCompute{
												Aggregation: "count",
												Interval:    apiutils.NewInt64Pointer(5000),
											},
											// NOTE: Crossuids is in private beta
											DataSource: *datadogV1.FORMULAANDFUNCTIONEVENTSDATASOURCE_LOGS.Ptr(),
											GroupBy:    []datadogV1.FormulaAndFunctionEventQueryGroupBy{},
											Name:       "logs",
											Indexes:    []string{"*"},
											Search: &datadogV1.FormulaAndFunctionEventQueryDefinitionSearch{
												Query: "kube_namespace:system",
											},
											// NOTE: Strorage is in private beta
										},
									},
									{
										FormulaAndFunctionProcessQueryDefinition: &datadogV1.FormulaAndFunctionProcessQueryDefinition{
											Aggregator: datadogV1.FORMULAANDFUNCTIONMETRICAGGREGATION_AREA.Ptr(),
											// NOTE: CrossOrgUids
											DataSource: "container",
											// IsNormalizedCpu: apiutils.NewBoolPointer(true),
											Limit:      apiutils.NewInt64Pointer(10),
											Metric:     "process.stat.cpu.total_pct",
											Name:       "query2",
											Sort:       datadogV1.QUERYSORTORDER_ASC.Ptr(),
											TagFilters: []string{"team:test"},
											TextFilter: apiutils.NewStringPointer("foo-bar"),
										},
									},
									// datadogV1.FormulaAndFunctionQueryDefinition{
									// 	FormulaAndFunctionApmDependencyStatsQueryDefinition: &datadogV1.FormulaAndFunctionApmDependencyStatsQueryDefinition{

									// 	},
									// },
									{
										FormulaAndFunctionSLOQueryDefinition: &datadogV1.FormulaAndFunctionSLOQueryDefinition{
											AdditionalQueryFilters: apiutils.NewStringPointer(""),
											DataSource:             datadogV1.FORMULAANDFUNCTIONSLODATASOURCE_SLO,
											GroupMode:              datadogV1.FORMULAANDFUNCTIONSLOGROUPMODE_OVERALL.Ptr(),
											Measure:                datadogV1.FORMULAANDFUNCTIONSLOMEASURE_SLO_STATUS,
											Name:                   apiutils.NewStringPointer("query3"),
											SloId:                  "foobar123",
											SloQueryType:           datadogV1.FORMULAANDFUNCTIONSLOQUERYTYPE_METRIC.Ptr(),
										},
									},
									{
										FormulaAndFunctionSLOQueryDefinition: &datadogV1.FormulaAndFunctionSLOQueryDefinition{
											AdditionalQueryFilters: apiutils.NewStringPointer(""),
											DataSource:             datadogV1.FORMULAANDFUNCTIONSLODATASOURCE_SLO,
											GroupMode:              datadogV1.FORMULAANDFUNCTIONSLOGROUPMODE_OVERALL.Ptr(),
											Measure:                datadogV1.FORMULAANDFUNCTIONSLOMEASURE_SLO_STATUS,
											Name:                   apiutils.NewStringPointer("query3"),
											SloId:                  "foobar123",
											SloQueryType:           datadogV1.FORMULAANDFUNCTIONSLOQUERYTYPE_METRIC.Ptr(),
										},
									},
									{
										FormulaAndFunctionCloudCostQueryDefinition: &datadogV1.FormulaAndFunctionCloudCostQueryDefinition{
											Aggregator: datadogV1.WIDGETAGGREGATOR_AVERAGE.Ptr(),
											DataSource: "cloud_cost",
											Name:       "query1",
											Query:      "sum:aws.cost.amortized{*}.rollup(sum, 86400)",
										},
									},
								},
							},
						},
						RightYaxis: &datadogV1.WidgetAxis{},
						ShowLegend: apiutils.NewBoolPointer(true),
						Time: &datadogV1.WidgetTime{
							LiveSpan: datadogV1.WIDGETLIVESPAN_PAST_THIRTY_MINUTES.Ptr(),
						},
						Title: apiutils.NewStringPointer("ts graph"),
						// NOTE: Title align, Title Size don't have an effect
						Type: "timeseries",
					},
				},

				Id: apiutils.NewInt64Pointer(123),
			},
			// v1alpha1.Widget{
			// 	QueryValueWidgetDefinition: &v1alpha1.QueryValueWidgetDefinition{
			// 		Autoscale: apiutils.NewBoolPointer(true),
			// 		CustomLinks: []v1alpha1.WidgetCustomLink{
			// 			v1alpha1.WidgetCustomLink{
			// 				IsHidden: apiutils.NewBoolPointer(true),
			// 				Label:    apiutils.NewStringPointer("example"),
			// 				Link:     apiutils.NewStringPointer("team:test"),
			// 			},
			// 		},
			// 		Precision: apiutils.NewInt64Pointer(2),
			// 		Requests:  apiutils,
			// 	},
			// },
		},
	}

	return dashboard
}

func expectedTimeSeries() *datadogV1.TimeseriesWidgetDefinition {
	timeSeries := &datadogV1.TimeseriesWidgetDefinition{
		CustomLinks: []datadogV1.WidgetCustomLink{
			{
				IsHidden: apiutils.NewBoolPointer(true),
				Label:    apiutils.NewStringPointer("example"),
				Link:     apiutils.NewStringPointer("team:test"),
			},
		},
		Events: []datadogV1.WidgetEvent{
			{
				Q:             "foo-bar",
				TagsExecution: apiutils.NewStringPointer("foo-bar"),
			},
		},
		LegendColumns: []datadogV1.TimeseriesWidgetLegendColumn{
			// string types
			"value",
			"avg",
		},
		LegendLayout: datadogV1.TIMESERIESWIDGETLEGENDLAYOUT_HORIZONTAL.Ptr(),
		LegendSize:   apiutils.NewStringPointer("10"),
		// NOTE: test this out (by setting it )
		Markers: []datadogV1.WidgetMarker{
			{
				DisplayType: apiutils.NewStringPointer("warning"),
				Label:       apiutils.NewStringPointer("marker label"),
				// NOTE: may cause issues
				Time:  apiutils.NewStringPointer("6:30"),
				Value: "y = 15",
			},
		},
		Requests: []datadogV1.TimeseriesWidgetRequest{
			{
				// LogQuery: &datadogV1.LogQueryDefinition{
				// 	Compute: &datadogV1.LogsQueryCompute{
				// 		Aggregation: "count",
				// 		Facet:       apiutils.NewStringPointer("source"),
				// 		Interval:    apiutils.NewInt64Pointer(5000),
				// 	},
				// 	GroupBy: []datadogV1.LogQueryDefinitionGroupBy{
				// 		datadogV1.LogQueryDefinitionGroupBy{
				// 			Facet: "source",
				// 			Limit: apiutils.NewInt64Pointer(10),
				// 			Sort: &datadogV1.LogQueryDefinitionGroupBySort{
				// 				Aggregation: "count",
				// 				Facet:       apiutils.NewStringPointer("source"),
				// 				Order:       "asc",
				// 			},
				// 		},
				// 	},
				// },
				Queries: []datadogV1.FormulaAndFunctionQueryDefinition{
					{
						FormulaAndFunctionMetricQueryDefinition: &datadogV1.FormulaAndFunctionMetricQueryDefinition{
							Aggregator: datadogV1.FORMULAANDFUNCTIONMETRICAGGREGATION_AVG.Ptr(),
							// NOTE: CrossOrg UIDs in private beta
							DataSource: "metrics",
							Name:       "query1",
							Query:      "avg:system.cpu.user{*}",
						},
					},
					{
						FormulaAndFunctionEventQueryDefinition: &datadogV1.FormulaAndFunctionEventQueryDefinition{
							Compute: datadogV1.FormulaAndFunctionEventQueryDefinitionCompute{
								Aggregation: "count",
								Interval:    apiutils.NewInt64Pointer(5000),
							},
							// NOTE: Crossuids is in private beta
							DataSource: *datadogV1.FORMULAANDFUNCTIONEVENTSDATASOURCE_LOGS.Ptr(),
							GroupBy:    []datadogV1.FormulaAndFunctionEventQueryGroupBy{},
							Name:       "logs",
							Indexes:    []string{"*"},
							Search: &datadogV1.FormulaAndFunctionEventQueryDefinitionSearch{
								Query: "kube_namespace:system",
							},
							// NOTE: Strorage is in private beta
						},
					},
					{
						FormulaAndFunctionProcessQueryDefinition: &datadogV1.FormulaAndFunctionProcessQueryDefinition{
							Aggregator: datadogV1.FORMULAANDFUNCTIONMETRICAGGREGATION_AREA.Ptr(),
							// NOTE: CrossOrgUids
							DataSource: "container",
							// IsNormalizedCpu: apiutils.NewBoolPointer(true),
							Limit:      apiutils.NewInt64Pointer(10),
							Metric:     "process.stat.cpu.total_pct",
							Name:       "query2",
							Sort:       datadogV1.QUERYSORTORDER_ASC.Ptr(),
							TagFilters: []string{"team:test"},
							TextFilter: apiutils.NewStringPointer("foo-bar"),
						},
					},
					// datadogV1.FormulaAndFunctionQueryDefinition{
					// 	FormulaAndFunctionApmDependencyStatsQueryDefinition: &datadogV1.FormulaAndFunctionApmDependencyStatsQueryDefinition{

					// 	},
					// },
					{
						FormulaAndFunctionSLOQueryDefinition: &datadogV1.FormulaAndFunctionSLOQueryDefinition{
							AdditionalQueryFilters: apiutils.NewStringPointer(""),
							DataSource:             datadogV1.FORMULAANDFUNCTIONSLODATASOURCE_SLO,
							GroupMode:              datadogV1.FORMULAANDFUNCTIONSLOGROUPMODE_OVERALL.Ptr(),
							Measure:                datadogV1.FORMULAANDFUNCTIONSLOMEASURE_SLO_STATUS,
							Name:                   apiutils.NewStringPointer("query3"),
							SloId:                  "foobar123",
							SloQueryType:           datadogV1.FORMULAANDFUNCTIONSLOQUERYTYPE_METRIC.Ptr(),
						},
					},
					{
						FormulaAndFunctionSLOQueryDefinition: &datadogV1.FormulaAndFunctionSLOQueryDefinition{
							AdditionalQueryFilters: apiutils.NewStringPointer(""),
							DataSource:             datadogV1.FORMULAANDFUNCTIONSLODATASOURCE_SLO,
							GroupMode:              datadogV1.FORMULAANDFUNCTIONSLOGROUPMODE_OVERALL.Ptr(),
							Measure:                datadogV1.FORMULAANDFUNCTIONSLOMEASURE_SLO_STATUS,
							Name:                   apiutils.NewStringPointer("query3"),
							SloId:                  "foobar123",
							SloQueryType:           datadogV1.FORMULAANDFUNCTIONSLOQUERYTYPE_METRIC.Ptr(),
						},
					},
					{
						FormulaAndFunctionCloudCostQueryDefinition: &datadogV1.FormulaAndFunctionCloudCostQueryDefinition{
							Aggregator: datadogV1.WIDGETAGGREGATOR_AVERAGE.Ptr(),
							DataSource: "cloud_cost",
							Name:       "query1",
							Query:      "sum:aws.cost.amortized{*}.rollup(sum, 86400)",
						},
					},
				},
			},
		},
		RightYaxis: &datadogV1.WidgetAxis{},
		ShowLegend: apiutils.NewBoolPointer(true),
		Time: &datadogV1.WidgetTime{
			LiveSpan: datadogV1.WIDGETLIVESPAN_PAST_THIRTY_MINUTES.Ptr(),
		},
		Title: apiutils.NewStringPointer("ts graph"),
		// NOTE: Title align, Title Size don't have an effect
		Type: "timeseries",
	}

	return timeSeries
}

// func Test_convertTsBackground(t *testing.T) {
// 	tsBg := v1alpha1.TimeseriesBackground{
// 		Type: "bars",
// 	}

// 	assert.Equal(t, tsBg, "")
// }

// func genericDashboard(dbID int) datadogV1.Dashboard {
// 	fakeRawNow := time.Unix(1612244495, 0)
// 	// fakeNow, _ := time.Parse(dateFormat, strings.Split(fakeRawNow.String(), " db=")[0])
// 	// dbID64 := int64(dbID)
// 	// msg := "Something went wrong"
// 	// name := "Test monitor"
// 	// handle := "test_user"

// 	// tags := []string{
// 	// 	"team:test",
// 	// }

// 	return datadogV1.Dashboard{}
// }

// NOTE: move this into dashboar_test.go
func setupTestAuth(apiURL string) context.Context {
	testAuth := context.WithValue(
		context.Background(),
		datadogapi.ContextAPIKeys,
		map[string]datadogapi.APIKey{
			"apiKeyAuth": {
				Key: "DUMMY_API_KEY",
			},
			"appKeyAuth": {
				Key: "DUMMY_APP_KEY",
			},
		},
	)
	parsedAPIURL, _ := url.Parse(apiURL)
	testAuth = context.WithValue(testAuth, datadogapi.ContextServerIndex, 1)
	testAuth = context.WithValue(testAuth, datadogapi.ContextServerVariables, map[string]string{
		"name":     parsedAPIURL.Host,
		"protocol": parsedAPIURL.Scheme,
	})

	return testAuth
}

func Test_translateClientError(t *testing.T) {
	var ErrGeneric = errors.New("generic error")

	testCases := []struct {
		name                   string
		error                  error
		message                string
		expectedErrorType      error
		expectedError          error
		expectedErrorInterface interface{}
	}{
		{
			name:              "no message, generic error",
			error:             ErrGeneric,
			message:           "",
			expectedErrorType: ErrGeneric,
		},
		{
			name:              "generic message, generic error",
			error:             ErrGeneric,
			message:           "generic message",
			expectedErrorType: ErrGeneric,
		},
		{
			name:                   "generic message, error type datadogV1.GenericOpenAPIError",
			error:                  datadogapi.GenericOpenAPIError{},
			message:                "generic message",
			expectedErrorInterface: &datadogapi.GenericOpenAPIError{},
		},
		{
			name:          "generic message, error type *url.Error",
			error:         &url.Error{Err: fmt.Errorf("generic url error")},
			message:       "generic message",
			expectedError: fmt.Errorf("generic message (url.Error):  \"\": generic url error"),
		},
	}
	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			result := translateClientError(test.error, test.message)

			if test.expectedErrorType != nil {
				assert.True(t, errors.Is(result, test.expectedErrorType))
			}

			if test.expectedErrorInterface != nil {
				assert.True(t, errors.As(result, test.expectedErrorInterface))
			}

			if test.expectedError != nil {
				assert.Equal(t, test.expectedError, result)
			}
		})
	}
}
