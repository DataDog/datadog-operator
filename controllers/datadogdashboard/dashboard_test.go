package datadogdashboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	v1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/stretchr/testify/assert"
)

const dateFormat = "2006-01-02 15:04:05.999999999 -0700 MST"

func TestBuildDashboard(t *testing.T) {
	templateVariables := []v1alpha1.DashboardTemplateVariable{
		{
			AvailableValues: &[]string{
				"foo",
				"bar",
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
							"value",
							"avg",
						},
						LegendLayout: datadogV1.TIMESERIESWIDGETLEGENDLAYOUT_HORIZONTAL.Ptr(),
						LegendSize:   apiutils.NewStringPointer("10"),
						Markers: []v1alpha1.WidgetMarker{
							{
								DisplayType: apiutils.NewStringPointer("warning"),
								Label:       apiutils.NewStringPointer("marker label"),
								Time:        apiutils.NewStringPointer("6:30"),
								Value:       "y = 15",
							},
						},
						Requests: []v1alpha1.TimeseriesWidgetRequest{
							{
								DisplayType: datadogV1.WidgetDisplayType("area").Ptr(),
								Formulas: []v1alpha1.WidgetFormula{
									{
										Alias:           apiutils.NewStringPointer("foo-bar"),
										CellDisplayMode: datadogV1.TableWidgetCellDisplayMode("number").Ptr(),
										ConditionalFormats: []v1alpha1.WidgetConditionalFormat{
											{
												Comparator:    "=",
												CustomBgColor: apiutils.NewStringPointer("classic"),
												CustomFgColor: apiutils.NewStringPointer("classic"),
												HideValue:     apiutils.NewBoolPointer(false),
												ImageUrl:      apiutils.NewStringPointer("https://app.datadog.com"),
												Metric:        apiutils.NewStringPointer("kube_namespace:system"),
												Palette:       "blue",
												Timeframe:     apiutils.NewStringPointer("foo bar"),
											},
										},
										Formula: "formula",
										Limit: &v1alpha1.WidgetFormulaLimit{
											Count: apiutils.NewInt64Pointer(1),
											Order: datadogV1.QuerySortOrder("asc").Ptr(),
										},
										Style: &v1alpha1.WidgetFormulaStyle{
											Palette:      apiutils.NewStringPointer("classic"),
											PaletteIndex: apiutils.NewInt64Pointer(0),
										},
									},
								},
								Metadata: []v1alpha1.TimeseriesWidgetExpressionAlias{
									{
										AliasName:  apiutils.NewStringPointer("foo bar"),
										Expression: "test expression",
									},
								},
								OnRightYaxis: apiutils.NewBoolPointer(true),
								Queries: []v1alpha1.FormulaAndFunctionQueryDefinition{
									{
										FormulaAndFunctionMetricQueryDefinition: &v1alpha1.FormulaAndFunctionMetricQueryDefinition{
											Aggregator: datadogV1.FORMULAANDFUNCTIONMETRICAGGREGATION_AVG.Ptr(),
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
											DataSource: *datadogV1.FORMULAANDFUNCTIONEVENTSDATASOURCE_LOGS.Ptr(),
											GroupBy:    []v1alpha1.FormulaAndFunctionEventQueryGroupBy{},
											Name:       "logs",
											Indexes:    []string{"*"},
											Search: &v1alpha1.FormulaAndFunctionEventQueryDefinitionSearch{
												Query: "kube_namespace:system",
											},
											// Storage is in private beta, not tested here
										},
									},
									{
										FormulaAndFunctionProcessQueryDefinition: &v1alpha1.FormulaAndFunctionProcessQueryDefinition{
											Aggregator: datadogV1.FORMULAANDFUNCTIONMETRICAGGREGATION_AREA.Ptr(),
											DataSource: "container",
											Limit:      apiutils.NewInt64Pointer(10),
											Metric:     "process.stat.cpu.total_pct",
											Name:       "query2",
											Sort:       datadogV1.QUERYSORTORDER_ASC.Ptr(),
											TagFilters: []string{"team:test"},
											TextFilter: apiutils.NewStringPointer("foo-bar"),
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
								ResponseFormat: datadogV1.FormulaAndFunctionResponseFormat("timeseries").Ptr(),
								Style: &v1alpha1.WidgetRequestStyle{
									LineType:  datadogV1.WidgetLineType("dashed").Ptr(),
									LineWidth: datadogV1.WidgetLineWidth("normal").Ptr(),
									Palette:   apiutils.NewStringPointer("classic"),
								},
							},
						},
						RightYaxis: &v1alpha1.WidgetAxis{},
						ShowLegend: apiutils.NewBoolPointer(true),
						Time: &v1alpha1.WidgetTime{
							LiveSpan: datadogV1.WIDGETLIVESPAN_PAST_THIRTY_MINUTES.Ptr(),
						},
						Title: apiutils.NewStringPointer("ts graph"),
						Type:  "timeseries",
					},
					Id: apiutils.NewInt64Pointer(123),
					Layout: v1alpha1.WidgetLayout{
						Height:        10,
						IsColumnBreak: apiutils.NewBoolPointer(true),
						Width:         10,
						X:             5,
						Y:             5,
					},
				},
				{
					QueryValueWidgetDefinition: &v1alpha1.QueryValueWidgetDefinition{
						Autoscale: apiutils.NewBoolPointer(true),
						CustomLinks: []v1alpha1.WidgetCustomLink{
							{
								IsHidden: apiutils.NewBoolPointer(true),
								Label:    apiutils.NewStringPointer("example"),
								Link:     apiutils.NewStringPointer("team:test"),
							},
						},
						CustomUnit: apiutils.NewStringPointer("foobar"),
						Precision:  apiutils.NewInt64Pointer(2),
						Requests: []v1alpha1.QueryValueWidgetRequest{
							{
								Aggregator: datadogV1.WidgetAggregator("avg").Ptr(),
								ConditionalFormats: []v1alpha1.WidgetConditionalFormat{
									{
										Comparator:    "=",
										CustomBgColor: apiutils.NewStringPointer("classic"),
										CustomFgColor: apiutils.NewStringPointer("classic"),
										HideValue:     apiutils.NewBoolPointer(false),
										ImageUrl:      apiutils.NewStringPointer("https://app.datadog.com"),
										Metric:        apiutils.NewStringPointer("kube_namespace:system"),
										Palette:       "blue",
										Timeframe:     apiutils.NewStringPointer("foo bar"),
									},
								},
								Formulas: []v1alpha1.WidgetFormula{
									{
										Alias:           apiutils.NewStringPointer("foo-bar"),
										CellDisplayMode: datadogV1.TableWidgetCellDisplayMode("number").Ptr(),
										ConditionalFormats: []v1alpha1.WidgetConditionalFormat{
											{
												Comparator:    "=",
												CustomBgColor: apiutils.NewStringPointer("classic"),
												CustomFgColor: apiutils.NewStringPointer("classic"),
												HideValue:     apiutils.NewBoolPointer(false),
												ImageUrl:      apiutils.NewStringPointer("https://app.datadog.com"),
												Metric:        apiutils.NewStringPointer("kube_namespace:system"),
												Palette:       "blue",
												Timeframe:     apiutils.NewStringPointer("foo bar"),
											},
										},
										Formula: "formula",
										Limit: &v1alpha1.WidgetFormulaLimit{
											Count: apiutils.NewInt64Pointer(1),
											Order: datadogV1.QuerySortOrder("asc").Ptr(),
										},
										Style: &v1alpha1.WidgetFormulaStyle{
											Palette:      apiutils.NewStringPointer("classic"),
											PaletteIndex: apiutils.NewInt64Pointer(0),
										},
									},
								},
								Queries: []v1alpha1.FormulaAndFunctionQueryDefinition{
									{
										FormulaAndFunctionMetricQueryDefinition: &v1alpha1.FormulaAndFunctionMetricQueryDefinition{
											Aggregator: datadogV1.FORMULAANDFUNCTIONMETRICAGGREGATION_AVG.Ptr(),
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
											DataSource: *datadogV1.FORMULAANDFUNCTIONEVENTSDATASOURCE_LOGS.Ptr(),
											GroupBy:    []v1alpha1.FormulaAndFunctionEventQueryGroupBy{},
											Name:       "logs",
											Indexes:    []string{"*"},
											Search: &v1alpha1.FormulaAndFunctionEventQueryDefinitionSearch{
												Query: "kube_namespace:system",
											},
										},
									},
									{
										FormulaAndFunctionProcessQueryDefinition: &v1alpha1.FormulaAndFunctionProcessQueryDefinition{
											Aggregator: datadogV1.FORMULAANDFUNCTIONMETRICAGGREGATION_AREA.Ptr(),
											DataSource: "container",
											Limit:      apiutils.NewInt64Pointer(10),
											Metric:     "process.stat.cpu.total_pct",
											Name:       "query2",
											Sort:       datadogV1.QUERYSORTORDER_ASC.Ptr(),
											TagFilters: []string{"team:test"},
											TextFilter: apiutils.NewStringPointer("foo-bar"),
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
								ResponseFormat: datadogV1.FormulaAndFunctionResponseFormat("timeseries").Ptr(),
							},
						},
						TextAlign: datadogV1.WidgetTextAlign("center").Ptr(),
						Time:      &v1alpha1.WidgetTime{},
						TimeseriesBackground: &v1alpha1.TimeseriesBackground{
							Type: "bars",
							Yaxis: &v1alpha1.WidgetAxis{
								IncludeZero: apiutils.NewBoolPointer(true),
								Label:       apiutils.NewStringPointer("label"),
								Max:         apiutils.NewStringPointer("auto"),
								Min:         apiutils.NewStringPointer("auto"),
								Scale:       apiutils.NewStringPointer("linear"),
							},
						},
						Title:     apiutils.NewStringPointer("query value"),
						TitleSize: apiutils.NewStringPointer("50"),
						Type:      "query_value",
					},
				},
			},
		},
	}

	dashboard := buildDashboard(testLogger, db)
	assert.Equal(t, datadogV1.DashboardLayoutType(db.Spec.LayoutType), dashboard.GetLayoutType(), "discrepancy found in parameter: Query")
	assert.Equal(t, db.Spec.NotifyList, dashboard.GetNotifyList(), "discrepancy found in parameter: NotifyList")
	assert.Equal(t, datadogV1.DashboardReflowType(*db.Spec.ReflowType), dashboard.GetReflowType(), "discrepancy found in parameter: ReflowType")
	assert.Equal(t, db.Spec.Tags, dashboard.GetTags(), "discrepancy found in parameter: Tags")
	assert.Equal(t, db.Spec.Title, dashboard.GetTitle(), "discrepancy found in parameter: Title")

	// Compare created dashboard from CRD to what it is expected to be
	assert.Equal(t, dashboard.Widgets[0].Definition.TimeseriesWidgetDefinition, expectedTimeSeries(), "discrepancy found in parameter: TimeseriesWidgetDefinition")
	assert.Equal(t, dashboard.Widgets[1].Definition.QueryValueWidgetDefinition, expectedQueryValue(), "discrepancy found in parameter: QueryValueWidgetDefinition")
}

func Test_getDashboard(t *testing.T) {
	dbID := "test_id"
	expectedDashboard := genericDashboard(dbID)

	jsonDashboard, _ := expectedDashboard.MarshalJSON()
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jsonDashboard)
	}))
	defer httpServer.Close()

	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	apiClient := datadogapi.NewAPIClient(testConfig)
	client := datadogV1.NewDashboardsApi(apiClient)
	testAuth := setupTestAuth(httpServer.URL)

	val, err := getDashboard(testAuth, client, dbID)
	assert.Nil(t, err)
	assert.Equal(t, expectedDashboard, val)
}

func Test_createDashboard(t *testing.T) {
	dbId := "test_id"
	expectedDashboard := genericDashboard(dbId)

	db := &v1alpha1.DatadogDashboard{
		Spec: v1alpha1.DatadogDashboardSpec{
			LayoutType: datadogV1.DASHBOARDLAYOUTTYPE_ORDERED,
			NotifyList: []string{
				"test@example.com",
				"test2@example.com"},
			Title: "Test dashboard",
			Tags: []string{
				"team:test", "team:test2",
			},
		},
	}

	jsonDashboard, _ := expectedDashboard.MarshalJSON()
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jsonDashboard)
	}))
	defer httpServer.Close()

	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	apiClient := datadogapi.NewAPIClient(testConfig)
	client := datadogV1.NewDashboardsApi(apiClient)
	testAuth := setupTestAuth(httpServer.URL)

	dashboard, err := createDashboard(testLogger, testAuth, client, db)
	assert.Nil(t, err)

	assert.Equal(t, datadogV1.DashboardLayoutType(db.Spec.LayoutType), dashboard.GetLayoutType(), "discrepancy found in parameter: LayoutType")
	assert.Equal(t, db.Spec.Title, dashboard.GetTitle(), "discrepancy found in parameter: Title")
	assert.Equal(t, db.Spec.Tags, dashboard.GetTags(), "discrepancy found in parameter: Tags")
}

func Test_deleteDashboard(t *testing.T) {
	dbId := "test_id"

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
	}))
	defer httpServer.Close()

	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	apiClient := datadogapi.NewAPIClient(testConfig)
	client := datadogV1.NewDashboardsApi(apiClient)
	testAuth := setupTestAuth(httpServer.URL)

	err := deleteDashboard(testAuth, client, dbId)
	assert.Nil(t, err)
}

func Test_updateDashboard(t *testing.T) {
	dbId := "test_id"
	expectedDashboard := genericDashboard(dbId)

	db := &v1alpha1.DatadogDashboard{
		Spec: v1alpha1.DatadogDashboardSpec{
			LayoutType: datadogV1.DASHBOARDLAYOUTTYPE_ORDERED,
			NotifyList: []string{
				"test@example.com",
				"test2@example.com"},
			Title: "Test dashboard",
			Tags: []string{
				"team:test", "team:test2",
			},
		},
	}

	jsonDashboard, _ := expectedDashboard.MarshalJSON()
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jsonDashboard)
	}))
	defer httpServer.Close()

	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	apiClient := datadogapi.NewAPIClient(testConfig)
	client := datadogV1.NewDashboardsApi(apiClient)
	testAuth := setupTestAuth(httpServer.URL)

	dashboard, err := updateDashboard(testLogger, testAuth, client, db)
	assert.Nil(t, err)

	assert.Equal(t, datadogV1.DashboardLayoutType(db.Spec.LayoutType), dashboard.GetLayoutType(), "discrepancy found in parameter: LayoutType")
	assert.Equal(t, db.Spec.Title, dashboard.GetTitle(), "discrepancy found in parameter: Title")
	assert.Equal(t, db.Spec.Tags, dashboard.GetTags(), "discrepancy found in parameter: Tags")
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
		Markers: []datadogV1.WidgetMarker{
			{
				DisplayType: apiutils.NewStringPointer("warning"),
				Label:       apiutils.NewStringPointer("marker label"),

				Time:  apiutils.NewStringPointer("6:30"),
				Value: "y = 15",
			},
		},
		Requests: []datadogV1.TimeseriesWidgetRequest{
			{
				DisplayType: datadogV1.WidgetDisplayType("area").Ptr(),
				Formulas: []datadogV1.WidgetFormula{
					{
						Alias:           apiutils.NewStringPointer("foo-bar"),
						CellDisplayMode: datadogV1.TableWidgetCellDisplayMode("number").Ptr(),
						ConditionalFormats: []datadogV1.WidgetConditionalFormat{
							{
								Comparator:    "=",
								CustomBgColor: apiutils.NewStringPointer("classic"),
								CustomFgColor: apiutils.NewStringPointer("classic"),
								HideValue:     apiutils.NewBoolPointer(false),
								ImageUrl:      apiutils.NewStringPointer("https://app.datadog.com"),
								Metric:        apiutils.NewStringPointer("kube_namespace:system"),
								Palette:       "blue",
								Timeframe:     apiutils.NewStringPointer("foo bar"),
							},
						},
						Formula: "formula",
						Limit: &datadogV1.WidgetFormulaLimit{
							Count: apiutils.NewInt64Pointer(1),
							Order: datadogV1.QuerySortOrder("asc").Ptr(),
						},
						Style: &datadogV1.WidgetFormulaStyle{
							Palette:      apiutils.NewStringPointer("classic"),
							PaletteIndex: apiutils.NewInt64Pointer(0),
						},
					},
				},
				Metadata: []datadogV1.TimeseriesWidgetExpressionAlias{
					{
						AliasName:  apiutils.NewStringPointer("foo bar"),
						Expression: "test expression",
					},
				},
				OnRightYaxis: apiutils.NewBoolPointer(true),
				Queries: []datadogV1.FormulaAndFunctionQueryDefinition{

					{
						FormulaAndFunctionMetricQueryDefinition: &datadogV1.FormulaAndFunctionMetricQueryDefinition{
							Aggregator: datadogV1.FORMULAANDFUNCTIONMETRICAGGREGATION_AVG.Ptr(),
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
							DataSource: *datadogV1.FORMULAANDFUNCTIONEVENTSDATASOURCE_LOGS.Ptr(),
							GroupBy:    []datadogV1.FormulaAndFunctionEventQueryGroupBy{},
							Name:       "logs",
							Indexes:    []string{"*"},
							Search: &datadogV1.FormulaAndFunctionEventQueryDefinitionSearch{
								Query: "kube_namespace:system",
							},
						},
					},
					{
						FormulaAndFunctionProcessQueryDefinition: &datadogV1.FormulaAndFunctionProcessQueryDefinition{
							Aggregator: datadogV1.FORMULAANDFUNCTIONMETRICAGGREGATION_AREA.Ptr(),
							DataSource: "container",

							Limit:      apiutils.NewInt64Pointer(10),
							Metric:     "process.stat.cpu.total_pct",
							Name:       "query2",
							Sort:       datadogV1.QUERYSORTORDER_ASC.Ptr(),
							TagFilters: []string{"team:test"},
							TextFilter: apiutils.NewStringPointer("foo-bar"),
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
				ResponseFormat: datadogV1.FormulaAndFunctionResponseFormat("timeseries").Ptr(),
				Style: &datadogV1.WidgetRequestStyle{
					LineType:  datadogV1.WidgetLineType("dashed").Ptr(),
					LineWidth: datadogV1.WidgetLineWidth("normal").Ptr(),
					Palette:   apiutils.NewStringPointer("classic"),
				},
			},
		},
		RightYaxis: &datadogV1.WidgetAxis{},
		ShowLegend: apiutils.NewBoolPointer(true),
		Time: &datadogV1.WidgetTime{
			LiveSpan: datadogV1.WIDGETLIVESPAN_PAST_THIRTY_MINUTES.Ptr(),
		},
		Title: apiutils.NewStringPointer("ts graph"),
		Type:  "timeseries",
	}

	return timeSeries
}

func expectedQueryValue() *datadogV1.QueryValueWidgetDefinition {
	queryValue := &datadogV1.QueryValueWidgetDefinition{
		Autoscale: apiutils.NewBoolPointer(true),
		CustomLinks: []datadogV1.WidgetCustomLink{
			{
				IsHidden: apiutils.NewBoolPointer(true),
				Label:    apiutils.NewStringPointer("example"),
				Link:     apiutils.NewStringPointer("team:test"),
			},
		},
		CustomUnit: apiutils.NewStringPointer("foobar"),
		Precision:  apiutils.NewInt64Pointer(2),
		Requests: []datadogV1.QueryValueWidgetRequest{
			{
				Aggregator: datadogV1.WidgetAggregator("avg").Ptr(),
				ConditionalFormats: []datadogV1.WidgetConditionalFormat{
					{
						Comparator:    "=",
						CustomBgColor: apiutils.NewStringPointer("classic"),
						CustomFgColor: apiutils.NewStringPointer("classic"),
						HideValue:     apiutils.NewBoolPointer(false),
						ImageUrl:      apiutils.NewStringPointer("https://app.datadog.com"),
						Metric:        apiutils.NewStringPointer("kube_namespace:system"),
						Palette:       "blue",
						Timeframe:     apiutils.NewStringPointer("foo bar"),
					},
				},
				Formulas: []datadogV1.WidgetFormula{
					{
						Alias:           apiutils.NewStringPointer("foo-bar"),
						CellDisplayMode: datadogV1.TableWidgetCellDisplayMode("number").Ptr(),
						ConditionalFormats: []datadogV1.WidgetConditionalFormat{
							{
								Comparator:    "=",
								CustomBgColor: apiutils.NewStringPointer("classic"),
								CustomFgColor: apiutils.NewStringPointer("classic"),
								HideValue:     apiutils.NewBoolPointer(false),
								ImageUrl:      apiutils.NewStringPointer("https://app.datadog.com"),
								Metric:        apiutils.NewStringPointer("kube_namespace:system"),
								Palette:       "blue",
								Timeframe:     apiutils.NewStringPointer("foo bar"),
							},
						},
						Formula: "formula",
						Limit: &datadogV1.WidgetFormulaLimit{
							Count: apiutils.NewInt64Pointer(1),
							Order: datadogV1.QuerySortOrder("asc").Ptr(),
						},
						Style: &datadogV1.WidgetFormulaStyle{
							Palette:      apiutils.NewStringPointer("classic"),
							PaletteIndex: apiutils.NewInt64Pointer(0),
						},
					},
				},
				Queries: []datadogV1.FormulaAndFunctionQueryDefinition{
					{
						FormulaAndFunctionMetricQueryDefinition: &datadogV1.FormulaAndFunctionMetricQueryDefinition{
							Aggregator: datadogV1.FORMULAANDFUNCTIONMETRICAGGREGATION_AVG.Ptr(),
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
							DataSource: *datadogV1.FORMULAANDFUNCTIONEVENTSDATASOURCE_LOGS.Ptr(),
							GroupBy:    []datadogV1.FormulaAndFunctionEventQueryGroupBy{},
							Name:       "logs",
							Indexes:    []string{"*"},
							Search: &datadogV1.FormulaAndFunctionEventQueryDefinitionSearch{
								Query: "kube_namespace:system",
							},
						},
					},
					{
						FormulaAndFunctionProcessQueryDefinition: &datadogV1.FormulaAndFunctionProcessQueryDefinition{
							Aggregator: datadogV1.FORMULAANDFUNCTIONMETRICAGGREGATION_AREA.Ptr(),
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
				ResponseFormat: datadogV1.FormulaAndFunctionResponseFormat("timeseries").Ptr(),
			},
		},
		TextAlign: datadogV1.WidgetTextAlign("center").Ptr(),
		Time:      &datadogV1.WidgetTime{},
		TimeseriesBackground: &datadogV1.TimeseriesBackground{
			Type: "bars",
			Yaxis: &datadogV1.WidgetAxis{
				IncludeZero: apiutils.NewBoolPointer(true),
				Label:       apiutils.NewStringPointer("label"),
				Max:         apiutils.NewStringPointer("auto"),
				Min:         apiutils.NewStringPointer("auto"),
				Scale:       apiutils.NewStringPointer("linear"),
			},
		},
		Title:     apiutils.NewStringPointer("query value"),
		TitleSize: apiutils.NewStringPointer("50"),
		Type:      "query_value",
	}

	return queryValue
}

func genericDashboard(dbID string) datadogV1.Dashboard {
	fakeRawNow := time.Unix(1612244495, 0)
	fakeNow, _ := time.Parse(dateFormat, strings.Split(fakeRawNow.String(), " db=")[0])
	layoutType := datadogV1.DashboardLayoutType("ordered")
	reflowType := datadogV1.DashboardReflowType("auto")
	notifyList := datadogapi.NewNullableList(&[]string{
		"test@example.com",
		"test2@example.com",
	})
	title := "Test dashboard"
	handle := "test_user"
	description := datadogapi.NewNullableString(apiutils.NewStringPointer("test description"))
	tags := datadogapi.NewNullableList(&[]string{
		"team:test", "team:test2",
	})

	return datadogV1.Dashboard{
		AuthorHandle: &handle,
		CreatedAt:    &fakeNow,
		Description:  *description,
		Tags:         *tags,
		Id:           &dbID,
		Title:        title,
		LayoutType:   layoutType,
		NotifyList:   *notifyList,
		ReflowType:   &reflowType,
		Widgets:      []datadogV1.Widget{},
	}
}

// Test a dashboard manifest with the data field set
func TestUnmarshalDashboard(t *testing.T) {
	dashboardJson := readFile("dashboard.json")
	v1alpha1Dashboard := v1alpha1.DatadogDashboard{}
	err := json.Unmarshal(dashboardJson, &v1alpha1Dashboard)
	dashboard := buildDashboard(testLogger, &v1alpha1Dashboard)

	templateVariables := []datadogV1.DashboardTemplateVariable{{
		Name:            "second",
		Prefix:          *datadogapi.NewNullableString(apiutils.NewStringPointer("override prefix")),
		AvailableValues: *datadogapi.NewNullableList(&[]string{"host1"}),
		Defaults:        []string{"*"},
	}}

	assert.Nil(t, err)
	// Sanity check. Check that dashboard has the number of widgets we expect it to have
	assert.Equal(t, 1, len(dashboard.GetWidgets()))
	// Check overridden fields
	assert.Equal(t, "Changed Title", dashboard.GetTitle())
	assert.Equal(t, []string{"test@datadoghq.com"}, dashboard.GetNotifyList())
	assert.Equal(t, templateVariables, dashboard.GetTemplateVariables())
}

func readFile(path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		panic(fmt.Sprintf("cannot open file %q: %s", path, err))
	}

	defer func() {
		if err = f.Close(); err != nil {
			panic(fmt.Sprintf("cannot close file: %s", err))
		}
	}()

	b, err := io.ReadAll(f)
	if err != nil {
		panic(fmt.Sprintf("cannot read file %q: %s", path, err))
	}

	return b
}

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
