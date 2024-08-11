package datadogdashboard

// build dashboards - build dashoard spec
// get dashboards (why? idk, but perhaps to update status if it's not reachable. Tells you something)
// validate if the fields of a dashboard are correctly written
// create dashboard -- use dashboard spec to create dashbaord
// update dashboard -- based on CRD change, I'm assuming?
// delete dashboard -- delete dashboard
// kubernetes automatically detects changes? how to deleting/updating

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"

	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/go-logr/logr"
)

// Dashboard
func buildDashboard(logger logr.Logger, ddb *v1alpha1.DatadogDashboard) *datadogV1.Dashboard {
	// create a dashboard
	layoutType := datadogV1.DashboardLayoutType(ddb.Spec.LayoutType)
	dbWidgets := buildWidgets(logger, ddb.Spec.Widgets)

	// NOTE: for now, pass in empty widgetlist
	dashboard := datadogV1.NewDashboard(layoutType, ddb.Spec.Title, dbWidgets)

	if ddb.Spec.Description != "" {
		dashboard.SetDescription(ddb.Spec.Description)
	} else {
		dashboard.SetDescriptionNil()
	}
	if ddb.Spec.NotifyList != nil {
		dashboard.SetNotifyList(ddb.Spec.NotifyList)
	}
	if ddb.Spec.ReflowType != nil {
		dashboard.SetReflowType(*ddb.Spec.ReflowType)
	}

	if ddb.Spec.TemplateVariablePresets != nil {
		dbTemplateVariablePresets := []datadogV1.DashboardTemplateVariablePreset{}
		for _, variablePreset := range ddb.Spec.TemplateVariablePresets {
			dbTemplateVariablePreset := datadogV1.DashboardTemplateVariablePreset{}
			// Note: Name is required. It can't be nil.
			dbTemplateVariablePreset.SetName(*variablePreset.Name)
			dbTemplateVariablePresetValues := []datadogV1.DashboardTemplateVariablePresetValue{}
			for _, presetValue := range variablePreset.TemplateVariables {
				dbTemplateVariablePresetValue := datadogV1.DashboardTemplateVariablePresetValue{}
				// Name is required
				dbTemplateVariablePresetValue.SetName(*presetValue.Name)
				if presetValue.Values != nil {
					dbTemplateVariablePresetValue.SetValues(presetValue.Values)
				}
				dbTemplateVariablePresetValues = append(dbTemplateVariablePresetValues, dbTemplateVariablePresetValue)
			}
			dbTemplateVariablePreset.SetTemplateVariables(dbTemplateVariablePresetValues)
			dbTemplateVariablePresets = append(dbTemplateVariablePresets, dbTemplateVariablePreset)
		}
		dashboard.SetTemplateVariablePresets(dbTemplateVariablePresets)
	}

	if ddb.Spec.TemplateVariables != nil {
		dbTemplateVariables := []datadogV1.DashboardTemplateVariable{}
		for _, templateVariable := range ddb.Spec.TemplateVariables {
			dbTemplateVariable := datadogV1.DashboardTemplateVariable{}
			dbTemplateVariable.SetName(templateVariable.Name)

			if dbTemplateVariable.Defaults != nil {
				dbTemplateVariable.SetDefaults(templateVariable.Defaults)
			}
			if templateVariable.AvailableValues.Value != nil {
				dbTemplateVariable.SetAvailableValues(*templateVariable.AvailableValues.Value)
			}
			// NOTE: since we can just set nullableString/List like so, perhaps change types to just make it a regular string/list?
			if templateVariable.Prefix.Value != nil {
				dbTemplateVariable.SetPrefix(*templateVariable.Prefix.Value)
			}
			dbTemplateVariables = append(dbTemplateVariables, dbTemplateVariable)

		}
		dashboard.SetTemplateVariables(dbTemplateVariables)
	}

	dashboard.SetWidgets(dbWidgets)

	tags := ddb.Spec.Tags
	sort.Strings(tags)
	dashboard.SetTags(tags)

	return dashboard
}

func getDashboard(auth context.Context, client *datadogV1.DashboardsApi, dashboardID string) (datadogV1.Dashboard, error) {
	dashboard, _, err := client.GetDashboard(auth, dashboardID)
	if err != nil {
		return datadogV1.Dashboard{}, translateClientError(err, "error creating Dashboard")
	}
	return dashboard, nil
}

func createDashboard(logger logr.Logger, auth context.Context, client *datadogV1.DashboardsApi, ddb *v1alpha1.DatadogDashboard) (datadogV1.Dashboard, error) {
	db := buildDashboard(logger, ddb)
	dbCreated, _, err := client.CreateDashboard(auth, *db)
	if err != nil {
		return datadogV1.Dashboard{}, translateClientError(err, "error creating dashboard")
	}

	return dbCreated, nil
}

func updateDashboard(logger logr.Logger, auth context.Context, client *datadogV1.DashboardsApi, ddb *v1alpha1.DatadogDashboard) (datadogV1.Dashboard, error) {
	dashboard := buildDashboard(logger, ddb)
	dbUpdated, _, err := client.UpdateDashboard(auth, ddb.Status.ID, *dashboard)
	if err != nil {
		return datadogV1.Dashboard{}, translateClientError(err, "error updating SLO")
	}

	return dbUpdated, nil
}

func deleteDashboard(auth context.Context, client *datadogV1.DashboardsApi, dashboardID string) error {
	if _, _, err := client.DeleteDashboard(auth, dashboardID); err != nil {
		return translateClientError(err, "error deleting Dashboard")
	}

	return nil
}

func translateClientError(err error, msg string) error {
	if msg == "" {
		msg = "an error occurred"
	}

	var apiErr datadogapi.GenericOpenAPIError
	var errURL *url.Error
	if errors.As(err, &apiErr) {
		return fmt.Errorf(msg+": %w: %s", err, apiErr.Body())
	}

	if errors.As(err, &errURL) {
		return fmt.Errorf(msg+" (url.Error): %s", errURL)
	}

	return fmt.Errorf(msg+": %w", err)
}

func buildWidgets(logger logr.Logger, widgets []v1alpha1.Widget) []datadogV1.Widget {
	dbWidgets := []datadogV1.Widget{}
	for _, widget := range widgets {
		dbWidget := datadogV1.Widget{}
		if widget.Id != nil {
			dbWidget.SetId(*widget.Id)
		}
		if widget.TimeseriesWidgetDefinition != nil {
			timeSeriesDefinition := buildTimeSeries(logger, widget.TimeseriesWidgetDefinition)
			definition := datadogV1.WidgetDefinition{
				TimeseriesWidgetDefinition: timeSeriesDefinition,
			}
			dbWidget.SetDefinition(definition)
		}
		if widget.QueryValueWidgetDefinition != nil {
			queryValueDefinition := buildQueryValue(logger, widget.QueryValueWidgetDefinition)
			definition := datadogV1.WidgetDefinition{
				QueryValueWidgetDefinition: queryValueDefinition,
			}
			dbWidget.SetDefinition(definition)
		}

		dbWidgets = append(dbWidgets, dbWidget)
	}
	return dbWidgets
}

func buildQueryValue(logger logr.Logger, qv *v1alpha1.QueryValueWidgetDefinition) *datadogV1.QueryValueWidgetDefinition {
	dbQueryValue := datadogV1.QueryValueWidgetDefinition{}
	if qv.Autoscale != nil {
		dbQueryValue.SetAutoscale(*qv.Autoscale)
	}
	if qv.CustomLinks != nil {
		dbLinks := convertCustomLinks(qv.CustomLinks)
		dbQueryValue.SetCustomLinks(dbLinks)
	}
	if qv.CustomUnit != nil {
		dbQueryValue.SetCustomUnit(*qv.CustomUnit)
	}
	if qv.Precision != nil {
		dbQueryValue.SetPrecision(*qv.Precision)
	}
	if qv.Requests != nil {
		dbRequests := convertQvRequests(qv.Requests)
		dbQueryValue.SetRequests(dbRequests)
	}
	if qv.TextAlign != nil {
		dbQueryValue.SetTextAlign(*qv.TextAlign)
	}
	if qv.Time != nil {
		dbTime := convertWidgetTime(*qv.Time)
		dbQueryValue.SetTime(dbTime)
	}
	if qv.TimeseriesBackground != nil {
		dbTsBg := convertTsBackground(qv.TimeseriesBackground)
		dbQueryValue.SetTimeseriesBackground(*dbTsBg)
	}
	if qv.Title != nil {
		dbQueryValue.SetTitle(*qv.Title)
	}
	if qv.TitleAlign != nil {
		dbQueryValue.SetTitleAlign(*qv.TitleAlign)
	}
	if qv.Type != "" {
		dbQueryValue.SetType(qv.Type)
	}
	return &dbQueryValue
}

func convertTsBackground(bg *v1alpha1.TimeseriesBackground) *datadogV1.TimeseriesBackground {
	dbBg := datadogV1.TimeseriesBackground{}
	if bg.Type != "" {
		dbBg.SetType(bg.Type)
	}
	if bg.Yaxis != nil {
		dbYaxis := convertWidgetAxis(*bg.Yaxis)
		dbBg.SetYaxis(dbYaxis)
	}
	return &dbBg
}

func convertQvRequests(requests []v1alpha1.QueryValueWidgetRequest) []datadogV1.QueryValueWidgetRequest {
	dbRequests := []datadogV1.QueryValueWidgetRequest{}
	for _, request := range requests {
		dbRequest := datadogV1.QueryValueWidgetRequest{}
		if request.ApmQuery != nil {
			dbLogDef := convertLogDefinition(request.ApmQuery)
			dbRequest.SetApmQuery(*dbLogDef)
		}
		if request.AuditQuery != nil {
			dbLogDef := convertLogDefinition(request.AuditQuery)
			dbRequest.SetAuditQuery(*dbLogDef)
		}
		if request.EventQuery != nil {
			dbLogDef := convertLogDefinition(request.EventQuery)
			dbRequest.SetEventQuery(*dbLogDef)
		}
		if request.LogQuery != nil {
			dbLogDef := convertLogDefinition(request.LogQuery)
			dbRequest.SetLogQuery(*dbLogDef)
		}
		if request.NetworkQuery != nil {
			dbLogDef := convertLogDefinition(request.NetworkQuery)
			dbRequest.SetNetworkQuery(*dbLogDef)
		}
		if request.ProfileMetricsQuery != nil {
			dbLogDef := convertLogDefinition(request.ProfileMetricsQuery)
			dbRequest.SetProfileMetricsQuery(*dbLogDef)
		}
		if request.RumQuery != nil {
			dbLogDef := convertLogDefinition(request.RumQuery)
			dbRequest.SetRumQuery(*dbLogDef)
		}
		if request.SecurityQuery != nil {
			dbLogDef := convertLogDefinition(request.SecurityQuery)
			dbRequest.SetSecurityQuery(*dbLogDef)
		}
		if request.RumQuery != nil {
			dbLogDef := convertLogDefinition(request.RumQuery)
			dbRequest.SetRumQuery(*dbLogDef)
		}
		if request.ProcessQuery != nil {
			dbProcessQuery := convertProcessQuery(*request.ProcessQuery)
			dbRequest.SetProcessQuery(dbProcessQuery)
		}
		if request.Formulas != nil {
			dbFormulas := convertFormulas(request.Formulas)
			dbRequest.SetFormulas(dbFormulas)
		}
		if request.Q != nil {
			dbRequest.SetQ(*request.Q)
		}
		if request.Queries != nil {
			dbQueries := convertFormulaQueries(request.Queries)
			dbRequest.SetQueries(dbQueries)
		}
		if request.ResponseFormat != nil {
			dbRequest.SetResponseFormat(*request.ResponseFormat)
		}
		dbRequests = append(dbRequests, dbRequest)
	}
	return dbRequests
}

func buildTimeSeries(logger logr.Logger, ts *v1alpha1.TimeseriesWidgetDefinition) *datadogV1.TimeseriesWidgetDefinition {
	dbTimeseries := datadogV1.TimeseriesWidgetDefinition{}
	// Custom links -- possibly make into function
	if ts.CustomLinks != nil {
		dbCustomLinks := convertCustomLinks(ts.CustomLinks)
		dbTimeseries.SetCustomLinks(dbCustomLinks)
	}
	// Widget Events
	if ts.Events != nil {
		dbEvents := convertWidgetEvents(ts.Events)
		dbTimeseries.SetEvents(dbEvents)
	}
	// Legend Columns
	if ts.LegendColumns != nil {
		dbTimeseries.SetLegendColumns(ts.LegendColumns)
	}
	// Legend Layout
	if ts.LegendLayout != nil {
		dbTimeseries.SetLegendLayout(*ts.LegendLayout)
	}
	// Legend Size
	if ts.LegendSize != nil {
		dbTimeseries.SetLegendSize(*ts.LegendSize)
	}
	// Markers
	if ts.Markers != nil {
		dbMarkers := convertWidgetMarkers(ts.Markers)
		dbTimeseries.SetMarkers(dbMarkers)
	}
	// Requests
	if ts.Requests != nil {
		dbRequests := convertTsRequests(ts.Requests)
		dbTimeseries.SetRequests(dbRequests)
	}
	// RightYAxis
	if ts.RightYaxis != nil {
		dbRightYaxis := convertWidgetAxis(*ts.RightYaxis)
		dbTimeseries.SetRightYaxis(dbRightYaxis)
	}
	// Show Legend
	if ts.ShowLegend != nil {
		dbTimeseries.SetShowLegend(*ts.ShowLegend)
	}
	// Time
	if ts.Time != nil {
		dbTime := convertWidgetTime(*ts.Time)
		dbTimeseries.SetTime(dbTime)
	}
	// Title
	if ts.Title != nil {
		dbTimeseries.SetTitle(*ts.Title)
	}
	// TitleAlign
	if ts.TitleAlign != nil {
		dbTimeseries.SetTitleAlign(*ts.TitleAlign)
	}
	// TitleSize
	if ts.TitleSize != nil {
		dbTimeseries.SetTitleSize(*ts.TitleSize)
	}
	// Yaxis
	if ts.Yaxis != nil {
		dbYaxis := convertWidgetAxis(*ts.Yaxis)
		dbTimeseries.SetYaxis(dbYaxis)
	}
	dbTimeseries.SetType("timeseries")
	return &dbTimeseries
}

func convertTsRequests(requests []v1alpha1.TimeseriesWidgetRequest) []datadogV1.TimeseriesWidgetRequest {
	dbRequests := []datadogV1.TimeseriesWidgetRequest{}
	for _, request := range requests {
		dbRequest := datadogV1.TimeseriesWidgetRequest{}
		// are these mutually exclusive
		if request.DisplayType != nil {
			dbRequest.SetDisplayType(*request.DisplayType)
		}
		if request.ApmQuery != nil {
			dbLogDef := convertLogDefinition(request.ApmQuery)
			dbRequest.SetApmQuery(*dbLogDef)
		}
		if request.AuditQuery != nil {
			dbLogDef := convertLogDefinition(request.AuditQuery)
			dbRequest.SetAuditQuery(*dbLogDef)
		}
		if request.EventQuery != nil {
			dbLogDef := convertLogDefinition(request.EventQuery)
			dbRequest.SetEventQuery(*dbLogDef)
		}
		if request.LogQuery != nil {
			dbLogDef := convertLogDefinition(request.LogQuery)
			dbRequest.SetLogQuery(*dbLogDef)
		}
		if request.NetworkQuery != nil {
			dbLogDef := convertLogDefinition(request.NetworkQuery)
			dbRequest.SetNetworkQuery(*dbLogDef)
		}
		if request.ProfileMetricsQuery != nil {
			dbLogDef := convertLogDefinition(request.ProfileMetricsQuery)
			dbRequest.SetProfileMetricsQuery(*dbLogDef)
		}
		if request.RumQuery != nil {
			dbLogDef := convertLogDefinition(request.RumQuery)
			dbRequest.SetRumQuery(*dbLogDef)
		}
		if request.SecurityQuery != nil {
			dbLogDef := convertLogDefinition(request.SecurityQuery)
			dbRequest.SetSecurityQuery(*dbLogDef)
		}
		if request.RumQuery != nil {
			dbLogDef := convertLogDefinition(request.RumQuery)
			dbRequest.SetRumQuery(*dbLogDef)
		}
		if request.ProcessQuery != nil {
			dbProcessQuery := convertProcessQuery(*request.ProcessQuery)
			dbRequest.SetProcessQuery(dbProcessQuery)
		}
		if request.Formulas != nil {
			dbFormulas := convertFormulas(request.Formulas)
			dbRequest.SetFormulas(dbFormulas)
		}
		if request.Metadata != nil {
			dbMetadata := convertTsMetadata(request.Metadata)
			dbRequest.SetMetadata(dbMetadata)
		}

		if request.OnRightYaxis != nil {
			dbRequest.SetOnRightYaxis(*request.OnRightYaxis)
		}
		if request.Q != nil {
			dbRequest.SetQ(*request.Q)
		}
		if request.Queries != nil {
			dbQueries := convertFormulaQueries(request.Queries)
			dbRequest.SetQueries(dbQueries)
		}
		if request.ResponseFormat != nil {
			dbRequest.SetResponseFormat(*request.ResponseFormat)
		}
		if request.Style != nil {
			dbStyle := convertRequestStyle(*request.Style)
			dbRequest.SetStyle(dbStyle)
		}
		dbRequests = append(dbRequests, dbRequest)
	}
	return dbRequests
}

func convertFormulas(formulas []v1alpha1.WidgetFormula) []datadogV1.WidgetFormula {
	dbFormulas := []datadogV1.WidgetFormula{}
	for _, formula := range formulas {
		dbFormula := datadogV1.WidgetFormula{}
		if formula.Alias != nil {
			dbFormula.SetAlias(*formula.Alias)
		}
		if formula.CellDisplayMode != nil {
			dbFormula.SetCellDisplayMode(*formula.CellDisplayMode)
		}
		if formula.ConditionalFormats != nil {
			dbConditionalFormats := convertConditionalFormats(formula.ConditionalFormats)
			dbFormula.SetConditionalFormats(dbConditionalFormats)
		}
		if formula.Formula != "" {
			dbFormula.SetFormula(formula.Formula)
		}
		if formula.Limit != nil {
			dbLimit := convertFormulaLimit(*formula.Limit)
			dbFormula.SetLimit(dbLimit)
		}
		if formula.Style != nil {
			dbStyle := convertFormulaStyle(*formula.Style)
			dbFormula.SetStyle(dbStyle)
		}
	}
	return dbFormulas
}

func convertCustomLinks(customLinks []v1alpha1.WidgetCustomLink) []datadogV1.WidgetCustomLink {
	dbCustomLinks := []datadogV1.WidgetCustomLink{}
	for _, link := range customLinks {
		dbCustomLink := datadogV1.WidgetCustomLink{}
		if link.IsHidden != nil {
			dbCustomLink.SetIsHidden(*link.IsHidden)
		}
		if link.Label != nil {
			dbCustomLink.SetLabel(*link.Label)
		}
		if link.OverrideLabel != nil {
			dbCustomLink.SetOverrideLabel(*link.OverrideLabel)
		}
		if link.Link != nil {
			dbCustomLink.SetLink(*link.Link)
		}
		dbCustomLinks = append(dbCustomLinks, dbCustomLink)
	}
	return dbCustomLinks
}

func convertLogDefinition(logQuery *v1alpha1.LogQueryDefinition) *datadogV1.LogQueryDefinition {
	dbLogQuery := datadogV1.LogQueryDefinition{}

	if logQuery.Compute != nil {
		dbLogCompute := datadogV1.LogsQueryCompute{}
		dbLogQuery.Compute = &dbLogCompute
		if logQuery.Compute.Aggregation != "" {
			dbLogQuery.Compute.Aggregation = logQuery.Compute.Aggregation
		}
		if dbLogQuery.Compute.Facet != nil {
			dbLogQuery.Compute.Facet = logQuery.Compute.Facet
		}
		if dbLogQuery.Compute.Interval != nil {
			dbLogQuery.Compute.Interval = logQuery.Compute.Interval
		}
	}

	if logQuery.GroupBy != nil {
		dbGroupBys := []datadogV1.LogQueryDefinitionGroupBy{}
		for _, groupBy := range logQuery.GroupBy {
			// NOTE: style? declare in struct or outside
			dbGroupBy := datadogV1.LogQueryDefinitionGroupBy{}
			if groupBy.Facet != "" {
				dbGroupBy.Facet = groupBy.Facet
			}
			if groupBy.Limit != nil {
				dbGroupBy.Limit = groupBy.Limit
			}
			if groupBy.Sort != nil {
				dbLogSort := datadogV1.LogQueryDefinitionGroupBySort{}
				dbGroupBy.Sort = &dbLogSort
				if groupBy.Sort.Aggregation != "" {
					dbGroupBy.Sort.Aggregation = groupBy.Sort.Aggregation
				}
				if groupBy.Sort.Facet != nil {
					dbGroupBy.Sort.Facet = groupBy.Sort.Facet
				}
				if groupBy.Sort.Order != "" {
					dbGroupBy.Sort.Order = groupBy.Sort.Order
				}
			}
			dbGroupBys = append(dbGroupBys, dbGroupBy)
		}
		dbLogQuery.GroupBy = dbGroupBys
	}

	if logQuery.Index != nil {
		dbLogQuery.Index = logQuery.Index
	}

	if logQuery.MultiCompute != nil {
		dbMultiCompute := []datadogV1.LogsQueryCompute{}
		for _, compute := range logQuery.MultiCompute {
			dbCompute := datadogV1.LogsQueryCompute{}
			if compute.Aggregation != "" {
				dbCompute.Aggregation = compute.Aggregation
			}
			if compute.Facet != nil {
				dbCompute.Facet = compute.Facet
			}
			if compute.Interval != nil {
				dbCompute.Interval = compute.Interval
			}
			dbMultiCompute = append(dbMultiCompute, dbCompute)
		}
		dbLogQuery.MultiCompute = dbMultiCompute
	}

	if logQuery.Search != nil {
		dbSearch := datadogV1.LogQueryDefinitionSearch{}
		// NOTE: is there a need to check
		if logQuery.Search.Query != "" {
			dbSearch.Query = logQuery.Search.Query
		}
		// NOTE: will this lose its reference
		dbLogQuery.Search = &dbSearch
	}

	return &dbLogQuery
}

func convertProcessQuery(processQuery v1alpha1.ProcessQueryDefinition) datadogV1.ProcessQueryDefinition {
	dbProcessQuery := datadogV1.ProcessQueryDefinition{}
	if processQuery.FilterBy != nil {
		dbProcessQuery.SetFilterBy(processQuery.FilterBy)
	}
	if processQuery.Limit != nil {
		dbProcessQuery.SetLimit(*processQuery.Limit)
	}
	if processQuery.Metric != "" {
		dbProcessQuery.SetMetric(processQuery.Metric)
	}
	if processQuery.SearchBy != nil {
		dbProcessQuery.SetSearchBy(*processQuery.SearchBy)
	}
	return dbProcessQuery
}

func convertConditionalFormats(conFormats []v1alpha1.WidgetConditionalFormat) []datadogV1.WidgetConditionalFormat {
	dbConFormats := []datadogV1.WidgetConditionalFormat{}
	for _, conFormat := range conFormats {
		dbConFormat := datadogV1.WidgetConditionalFormat{}
		if conFormat.Comparator != "" {
			dbConFormat.SetComparator(conFormat.Comparator)
		}
		if conFormat.CustomBgColor != nil {
			dbConFormat.SetCustomBgColor(*conFormat.CustomBgColor)
		}
		if conFormat.CustomFgColor != nil {
			dbConFormat.SetCustomFgColor(*conFormat.CustomFgColor)
		}
		if conFormat.HideValue != nil {
			dbConFormat.SetHideValue(*conFormat.HideValue)
		}
		if conFormat.ImageUrl != nil {
			dbConFormat.SetImageUrl(*conFormat.ImageUrl)
		}
		if conFormat.Metric != nil {
			dbConFormat.SetMetric(*conFormat.Metric)
		}
		if conFormat.Palette != "" {
			dbConFormat.SetPalette(conFormat.Palette)
		}
		if conFormat.Timeframe != nil {
			dbConFormat.SetTimeframe(*conFormat.Timeframe)
		}
		if conFormat.Value != nil {
			convertedFloat, _ := strconv.ParseFloat(conFormat.Value.AsDec().String(), 64)
			dbConFormat.SetValue(convertedFloat)
		}
	}
	return dbConFormats
}

func convertFormulaLimit(limit v1alpha1.WidgetFormulaLimit) datadogV1.WidgetFormulaLimit {
	dbLimit := datadogV1.WidgetFormulaLimit{}
	if limit.Count != nil {
		dbLimit.SetCount(*limit.Count)
	}
	if limit.Order != nil {
		dbLimit.SetOrder(*limit.Order)
	}
	return dbLimit
}

func convertFormulaStyle(style v1alpha1.WidgetFormulaStyle) datadogV1.WidgetFormulaStyle {
	dbStyle := datadogV1.WidgetFormulaStyle{}
	if style.Palette != nil {
		dbStyle.SetPalette(*style.Palette)
	}
	if style.PaletteIndex != nil {
		dbStyle.SetPaletteIndex(*style.PaletteIndex)
	}
	return dbStyle
}

func convertFormulaQueries(formulas []v1alpha1.FormulaAndFunctionQueryDefinition) []datadogV1.FormulaAndFunctionQueryDefinition {
	dbFormulas := []datadogV1.FormulaAndFunctionQueryDefinition{}

	for _, formula := range formulas {
		// NOTE: get rid of this with interface?
		if formula.FormulaAndFunctionMetricQueryDefinition != nil {
			dbMetricQuery := convertFormulaMetric(formula.FormulaAndFunctionMetricQueryDefinition)
			dbFormulaDef := datadogV1.FormulaAndFunctionQueryDefinition{
				FormulaAndFunctionMetricQueryDefinition: dbMetricQuery,
			}
			dbFormulas = append(dbFormulas, dbFormulaDef)
		} else if formula.FormulaAndFunctionEventQueryDefinition != nil {
			dbEventQuery := convertFormulaEvent(formula.FormulaAndFunctionEventQueryDefinition)
			dbFormulaDef := datadogV1.FormulaAndFunctionQueryDefinition{
				FormulaAndFunctionEventQueryDefinition: dbEventQuery,
			}
			dbFormulas = append(dbFormulas, dbFormulaDef)
		} else if formula.FormulaAndFunctionProcessQueryDefinition != nil {
			dbProcessQuery := convertFormulaProcess(formula.FormulaAndFunctionProcessQueryDefinition)
			dbFormulaDef := datadogV1.FormulaAndFunctionQueryDefinition{
				FormulaAndFunctionProcessQueryDefinition: dbProcessQuery,
			}
			dbFormulas = append(dbFormulas, dbFormulaDef)
		} else if formula.FormulaAndFunctionApmDependencyStatsQueryDefinition != nil {
			dbApmQuery := convertFormulaApm(formula.FormulaAndFunctionApmDependencyStatsQueryDefinition)
			dbFormulaDef := datadogV1.FormulaAndFunctionQueryDefinition{
				FormulaAndFunctionApmDependencyStatsQueryDefinition: dbApmQuery,
			}
			dbFormulas = append(dbFormulas, dbFormulaDef)
		} else if formula.FormulaAndFunctionSLOQueryDefinition != nil {
			dbSloQuery := convertFormulaSlo(formula.FormulaAndFunctionSLOQueryDefinition)
			dbFormulaDef := datadogV1.FormulaAndFunctionQueryDefinition{
				FormulaAndFunctionSLOQueryDefinition: dbSloQuery,
			}
			dbFormulas = append(dbFormulas, dbFormulaDef)
		} else if formula.FormulaAndFunctionCloudCostQueryDefinition != nil {
			dbCcQuery := convertFormulaCloudCost(formula.FormulaAndFunctionCloudCostQueryDefinition)

			dbFormulaDef := datadogV1.FormulaAndFunctionQueryDefinition{
				FormulaAndFunctionCloudCostQueryDefinition: dbCcQuery,
			}
			dbFormulas = append(dbFormulas, dbFormulaDef)
		}
	}
	return dbFormulas
}

// NOTE: change name to convertFormulaMetricQuery for consistency after touching base with dashboards team
func convertFormulaMetric(metricQuery *v1alpha1.FormulaAndFunctionMetricQueryDefinition) *datadogV1.FormulaAndFunctionMetricQueryDefinition {
	dbMetricQuery := datadogV1.FormulaAndFunctionMetricQueryDefinition{}
	if metricQuery.Aggregator != nil {
		dbMetricQuery.SetAggregator(*metricQuery.Aggregator)
	}
	// NOTE: can't set crossOrgUuids...
	// if metricQuery.CrossOrgUuids != nil {
	// 	dbMetricQuery.Set
	// }
	if metricQuery.DataSource != "" {
		dbMetricQuery.SetDataSource(metricQuery.DataSource)
	}
	if metricQuery.Name != "" {
		dbMetricQuery.SetName(metricQuery.Name)
	}
	if metricQuery.Query != "" {
		dbMetricQuery.SetQuery(metricQuery.Query)
	}
	return &dbMetricQuery
}

func convertFormulaEvent(eventQuery *v1alpha1.FormulaAndFunctionEventQueryDefinition) *datadogV1.FormulaAndFunctionEventQueryDefinition {
	dbEventQuery := datadogV1.FormulaAndFunctionEventQueryDefinition{}

	if eventQuery.Compute != nil {
		dbCompute := datadogV1.FormulaAndFunctionEventQueryDefinitionCompute{}
		if eventQuery.Compute.Aggregation != "" {
			dbCompute.SetAggregation(eventQuery.Compute.Aggregation)
		}
		if eventQuery.Compute.Interval != nil {
			dbCompute.SetInterval(*eventQuery.Compute.Interval)
		}
		if eventQuery.Compute.Metric != nil {
			dbCompute.SetMetric(*eventQuery.Compute.Metric)
		}
		dbEventQuery.SetCompute(dbCompute)
	}
	if eventQuery.DataSource != "" {
		dbEventQuery.SetDataSource(eventQuery.DataSource)
	}
	if eventQuery.GroupBy != nil {
		dbGroubys := []datadogV1.FormulaAndFunctionEventQueryGroupBy{}
		for _, groupBy := range eventQuery.GroupBy {
			dbGroupBy := datadogV1.FormulaAndFunctionEventQueryGroupBy{}
			if groupBy.Facet != "" {
				dbGroupBy.SetFacet(groupBy.Facet)
			}
			if groupBy.Limit != nil {
				dbGroupBy.SetLimit(*groupBy.Limit)
			}
			if groupBy.Sort != nil {
				dbSort := datadogV1.FormulaAndFunctionEventQueryGroupBySort{}
				if groupBy.Sort.Aggregation != "" {
					dbSort.SetAggregation(groupBy.Sort.Aggregation)
				}
				if groupBy.Sort.Metric != nil {
					dbSort.SetMetric(*groupBy.Sort.Metric)
				}
				if groupBy.Sort.Order != nil {
					dbSort.SetOrder(*groupBy.Sort.Order)
				}
				dbGroupBy.SetSort(dbSort)
			}
			dbGroubys = append(dbGroubys, dbGroupBy)
		}
		dbEventQuery.SetGroupBy(dbGroubys)
	}
	if eventQuery.Indexes != nil {
		dbEventQuery.SetIndexes(eventQuery.Indexes)
	}
	if eventQuery.Name != "" {
		dbEventQuery.SetName(eventQuery.Name)
	}
	if eventQuery.Search != nil {
		dbQuerySearch := datadogV1.FormulaAndFunctionEventQueryDefinitionSearch{}
		if eventQuery.Search.Query != "" {
			dbQuerySearch.SetQuery(eventQuery.Search.Query)
		}
		dbEventQuery.SetSearch(dbQuerySearch)
	}
	if eventQuery.Storage != nil {
		dbEventQuery.SetStorage(*eventQuery.Storage)
	}
	return &dbEventQuery
}

func convertFormulaProcess(processQuery *v1alpha1.FormulaAndFunctionProcessQueryDefinition) *datadogV1.FormulaAndFunctionProcessQueryDefinition {
	dbProcessQuery := datadogV1.FormulaAndFunctionProcessQueryDefinition{}

	if processQuery.Aggregator != nil {
		dbProcessQuery.SetAggregator(*processQuery.Aggregator)
	}
	// NOTE: INSERT CROSSORGUIDS
	if processQuery.DataSource != "" {
		dbProcessQuery.SetDataSource(processQuery.DataSource)
	}
	if processQuery.IsNormalizedCpu != nil {
		dbProcessQuery.SetIsNormalizedCpu(*processQuery.IsNormalizedCpu)
	}
	if processQuery.Limit != nil {
		dbProcessQuery.SetLimit(*processQuery.Limit)
	}
	if processQuery.Metric != "" {
		dbProcessQuery.SetMetric(processQuery.Metric)
	}
	if processQuery.Name != "" {
		dbProcessQuery.SetName(processQuery.Name)
	}
	if processQuery.Sort != nil {
		dbProcessQuery.SetSort(*processQuery.Sort)
	}
	if processQuery.TagFilters != nil {
		dbProcessQuery.SetTagFilters(processQuery.TagFilters)
	}
	if processQuery.TextFilter != nil {
		dbProcessQuery.SetTextFilter(*processQuery.TextFilter)
	}
	return &dbProcessQuery
}

func convertFormulaApm(apmQuery *v1alpha1.FormulaAndFunctionApmDependencyStatsQueryDefinition) *datadogV1.FormulaAndFunctionApmDependencyStatsQueryDefinition {
	dbApmQuery := datadogV1.FormulaAndFunctionApmDependencyStatsQueryDefinition{}
	if apmQuery.DataSource != "" {
		dbApmQuery.SetDataSource(apmQuery.DataSource)
	}
	if apmQuery.Env != "" {
		dbApmQuery.SetEnv(apmQuery.Env)
	}
	if apmQuery.IsUpstream != nil {
		dbApmQuery.SetIsUpstream(*apmQuery.IsUpstream)
	}
	if apmQuery.Name != "" {
		dbApmQuery.SetName(apmQuery.Name)
	}
	if apmQuery.OperationName != "" {
		dbApmQuery.SetOperationName(apmQuery.OperationName)
	}
	if apmQuery.PrimaryTagName != nil {
		dbApmQuery.SetPrimaryTagName(*apmQuery.PrimaryTagName)
	}
	if apmQuery.PrimaryTagValue != nil {
		dbApmQuery.SetPrimaryTagValue(*apmQuery.PrimaryTagName)
	}
	if apmQuery.ResourceName != "" {
		dbApmQuery.SetResourceName(apmQuery.ResourceName)
	}
	if apmQuery.Service != "" {
		dbApmQuery.SetService(apmQuery.Service)
	}
	if apmQuery.Stat != "" {
		dbApmQuery.SetStat(apmQuery.Stat)
	}
	return &dbApmQuery
}

func convertFormulaSlo(sloQuery *v1alpha1.FormulaAndFunctionSLOQueryDefinition) *datadogV1.FormulaAndFunctionSLOQueryDefinition {
	dbSloQuery := datadogV1.FormulaAndFunctionSLOQueryDefinition{}

	if sloQuery.AdditionalQueryFilters != nil {
		dbSloQuery.SetAdditionalQueryFilters(*sloQuery.AdditionalQueryFilters)
	}
	if sloQuery.DataSource != "" {
		dbSloQuery.SetDataSource(sloQuery.DataSource)
	}
	if sloQuery.GroupMode != nil {
		dbSloQuery.SetGroupMode(*sloQuery.GroupMode)
	}
	if sloQuery.Measure != "" {
		dbSloQuery.SetMeasure(sloQuery.Measure)
	}
	if sloQuery.Name != nil {
		dbSloQuery.SetName(*sloQuery.Name)
	}
	if sloQuery.SloId != "" {
		dbSloQuery.SetSloId(sloQuery.SloId)
	}
	if sloQuery.SloQueryType != nil {
		dbSloQuery.SetSloQueryType(*sloQuery.SloQueryType)
	}
	return &dbSloQuery
}

func convertFormulaCloudCost(ccQuery *v1alpha1.FormulaAndFunctionCloudCostQueryDefinition) *datadogV1.FormulaAndFunctionCloudCostQueryDefinition {
	dbCcQuery := datadogV1.FormulaAndFunctionCloudCostQueryDefinition{}

	if ccQuery.Aggregator != nil {
		dbCcQuery.SetAggregator(*ccQuery.Aggregator)
	}
	if ccQuery.DataSource != "" {
		dbCcQuery.SetDataSource(ccQuery.DataSource)
	}
	if ccQuery.Name != "" {
		dbCcQuery.SetName(ccQuery.Name)
	}
	if ccQuery.Query != "" {
		dbCcQuery.SetQuery(ccQuery.Query)
	}

	return &dbCcQuery
}

func convertTsMetadata(metadata []v1alpha1.TimeseriesWidgetExpressionAlias) []datadogV1.TimeseriesWidgetExpressionAlias {
	dbMetadata := []datadogV1.TimeseriesWidgetExpressionAlias{}
	// NOTE: name change?
	for _, alias := range metadata {
		dbAlias := datadogV1.TimeseriesWidgetExpressionAlias{}
		if alias.AliasName != nil {
			dbAlias.SetAliasName(*alias.AliasName)
		}
		if alias.Expression != "" {
			dbAlias.SetExpression(alias.Expression)
		}
		dbMetadata = append(dbMetadata, dbAlias)
	}
	return dbMetadata
}

func convertWidgetAxis(axis v1alpha1.WidgetAxis) datadogV1.WidgetAxis {
	dbAxis := datadogV1.WidgetAxis{}
	if axis.IncludeZero != nil {
		dbAxis.SetIncludeZero(*axis.IncludeZero)
	}
	if axis.Label != nil {
		dbAxis.SetLabel(*axis.Label)
	}
	if axis.Max != nil {
		dbAxis.SetMax(*axis.Max)
	}
	if axis.Min != nil {
		dbAxis.SetMin(*axis.Min)
	}
	if axis.Scale != nil {
		dbAxis.SetScale(*axis.Scale)
	}
	return dbAxis
}

func convertWidgetTime(time v1alpha1.WidgetTime) datadogV1.WidgetTime {
	dbTime := datadogV1.WidgetTime{}
	if time.LiveSpan != nil {
		dbTime.SetLiveSpan(*time.LiveSpan)
	}
	return dbTime
}

func convertRequestStyle(style v1alpha1.WidgetRequestStyle) datadogV1.WidgetRequestStyle {
	dbStyle := datadogV1.WidgetRequestStyle{}
	if style.LineType != nil {
		dbStyle.SetLineType(*style.LineType)
	}
	if style.LineWidth != nil {
		dbStyle.SetLineWidth(*style.LineWidth)
	}
	if style.Palette != nil {
		dbStyle.SetPalette(*style.Palette)
	}
	return dbStyle
}

func convertWidgetEvents(events []v1alpha1.WidgetEvent) []datadogV1.WidgetEvent {
	dbEvents := []datadogV1.WidgetEvent{}

	for _, event := range events {
		dbEvent := datadogV1.WidgetEvent{}
		if event.Q != "" {
			dbEvent.SetQ(event.Q)
		}
		if event.TagsExecution != nil {
			dbEvent.SetTagsExecution(*event.TagsExecution)
		}
		dbEvents = append(dbEvents, dbEvent)
	}
	return dbEvents
}

func convertWidgetMarkers(markers []v1alpha1.WidgetMarker) []datadogV1.WidgetMarker {
	dbMarkers := []datadogV1.WidgetMarker{}

	for _, marker := range markers {
		dbMarker := datadogV1.WidgetMarker{}
		if marker.DisplayType != nil {
			dbMarker.SetDisplayType(*marker.DisplayType)
		}
		if marker.Label != nil {
			dbMarker.SetLabel(*marker.Label)
		}
		if marker.Time != nil {
			dbMarker.SetTime(*marker.Time)
		}
		if marker.Value != "" {
			dbMarker.SetValue(marker.Value)
		}
		dbMarkers = append(dbMarkers, dbMarker)
	}
	return dbMarkers
}
