// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatadogDashboardSpec defines the desired state of DatadogDashboard
// +k8s:openapi-gen=true
type DatadogDashboardSpec struct {
	// Description of the dashboard.
	Description string `json:"description,omitempty"`
	// Layout type of the dashboard.
	LayoutType DashboardLayoutType `json:"layoutType,omitempty"`
	// List of handles of users to notify when changes are made to this dashboard.
	// +listType=set
	NotifyList []string `json:"notifyList,omitempty"`
	// Reflow type for a **new dashboard layout** dashboard. Set this only when layout type is 'ordered'.
	// If set to 'fixed', the dashboard expects all widgets to have a layout, and if it's set to 'auto',
	// widgets should not have layouts.
	ReflowType *datadogV1.DashboardReflowType `json:"reflowType,omitempty"`
	// List of team names representing ownership of a dashboard.
	// +listType=set
	Tags []string `json:"tags,omitempty"`
	// Array of template variables saved views.
	// +listType=map
	// +listMapKey=name
	TemplateVariablePresets []DashboardTemplateVariablePreset `json:"templateVariablePresets,omitempty"`
	// List of template variables for this dashboard.
	// +listType=map
	// +listMapKey=name
	TemplateVariables []DashboardTemplateVariable `json:"templateVariables,omitempty"`
	// Title of the dashboard.
	Title string `json:"title,omitempty"`
	// List of widgets to display on the dashboard.
	// +listType=map
	// +listMapKey=id
	Widgets []Widget `json:"widgets,omitempty"`
}

// DashboardLayoutType Layout type of the dashboard.
type DashboardLayoutType string

// List of DashboardLayoutType.
const (
	DASHBOARDLAYOUTTYPE_ORDERED DashboardLayoutType = "ordered"
	DASHBOARDLAYOUTTYPE_FREE    DashboardLayoutType = "free"
)

func (t DashboardLayoutType) isValid() bool {
	switch t {
	case DASHBOARDLAYOUTTYPE_ORDERED, DASHBOARDLAYOUTTYPE_FREE:
		return true
	default:
		return false
	}
}

// DatadogDashboardStatus defines the observed state of DatadogDashboard
// +k8s:openapi-gen=true
type DatadogDashboardStatus struct {
	// Conditions represents the latest available observations of the state of a DatadogSLO.
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// ID is the SLO ID generated in Datadog.
	ID string `json:"id,omitempty"`
	// Creator is the identity of the SLO creator.
	Creator string `json:"creator,omitempty"`
	// Created is the time the SLO was created.
	Created *metav1.Time `json:"created,omitempty"`
	// SyncStatus shows the health of syncing the SLO state to Datadog.
	SyncStatus DatadogDashboardSyncStatus `json:"syncStatus,omitempty"`
	// CurrentHash tracks the hash of the current DatadogSLOSpec to know
	// if the Spec has changed and needs an update.
	CurrentHash string `json:"currentHash,omitempty"`
	// DashboardLastForceSyncTime is the last time the API dashboard was last force synced with the Datadogdashboard resource
	LastForceSyncTime *metav1.Time `json:"dashboardLastForceSyncTime,omitempty"`
}

type DatadogDashboardSyncStatus string

const (
	// DatadogDashboardSyncStatusOK means syncing is OK.
	DatadogDashboardSyncStatusOK DatadogDashboardSyncStatus = "OK"
	// DatadogDashboardSyncStatusValidateError means there is a dashboard validation error.
	DatadogDashboardSyncStatusValidateError DatadogDashboardSyncStatus = "error validating dashboard"
	// DatadogDashboardSyncStatusUpdateError means there is a dashboard update error.
	DatadogDashboardSyncStatusUpdateError DatadogDashboardSyncStatus = "error updating dashboard"
	// DatadogDashboardSyncStatusCreateError means there is an error getting the dashboard.
	DatadogDashboardSyncStatusCreateError DatadogDashboardSyncStatus = "error creating dashboard"
	// SyncStatusGetError means there is an error getting the monitor
	DatadoggDashboardSyncStatusGetError DatadogDashboardSyncStatus = "error getting dashboard"
)

// DatadogDashboard is the Schema for the datadogdashboards API
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=datadogdashboards,scope=Namespaced,shortName=dddashboard
// +kubebuilder:printcolumn:name="id",type="string",JSONPath=".status.id"
// +kubebuilder:printcolumn:name="sync status",type="string",JSONPath=".status.syncStatus"
// +kubebuilder:printcolumn:name="age",type="date",JSONPath=".metadata.creationTimestamp"
// +k8s:openapi-gen=true
// +genclient
type DatadogDashboard struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatadogDashboardSpec   `json:"spec,omitempty"`
	Status DatadogDashboardStatus `json:"status,omitempty"`
}

// DashboardTemplateVariablePresetValue Template variables saved views.
// +k8s:openapi-gen=true
type DashboardTemplateVariablePresetValue struct {
	// The name of the variable.
	Name *string `json:"name"`
	// (deprecated) The value of the template variable within the saved view. Cannot be used in conjunction with `values`.
	// Deprecated
	Value *string `json:"value,omitempty"`
	// One or many template variable values within the saved view, which will be unioned together using `OR` if more than one is specified. Cannot be used in conjunction with `value`.
	// +listType=set
	Values []string `json:"values,omitempty"`
}

// DashboardTemplateVariablePreset Template variables saved views.
// +k8s:openapi-gen=true
type DashboardTemplateVariablePreset struct {
	// The name of the variable.
	Name *string `json:"name"`
	// List of variables.
	// +listType=map
	// +listMapKey=name
	TemplateVariables []DashboardTemplateVariablePresetValue `json:"templateVariables,omitempty"`
}

// DashboardTemplateVariable Template variable.
// +k8s:openapi-gen=true
type DashboardTemplateVariable struct {
	// The list of values that the template variable drop-down is limited to.
	AvailableValues NullableList `json:"availableValues,omitempty"`
	// (deprecated) The default value for the template variable on dashboard load. Cannot be used in conjunction with `defaults`.
	// Deprecated
	Default NullableString `json:"default,omitempty"`
	// One or many default values for template variables on load. If more than one default is specified, they will be unioned together with `OR`. Cannot be used in conjunction with `default`.
	// +listType=set
	Defaults []string `json:"defaults,omitempty"`
	// The name of the variable.
	Name string `json:"name"`
	// The tag prefix associated with the variable. Only tags with this prefix appear in the variable drop-down.
	Prefix NullableString `json:"prefix,omitempty"`
}

// NullableString is a struct to hold a nullable string value.
// +k8s:openapi-gen=true
type NullableString struct {
	Value *string `json:"value,omitempty"`
	IsSet bool    `json:"isSet,omitempty"`
}

// NullableList struct to hold nullable list value.
// +k8s:openapi-gen=true
type NullableList struct {
	Value *[]string `json:"value,omitempty"`
	IsSet bool      `json:"isSet,omitempty"`
}

// Widget Information about widget.
//
// **Note**: The `layout` property is required for widgets in dashboards with `free` `layout_type`.
//
//	For the **new dashboard layout**, the `layout` property depends on the `reflow_type` of the dashboard.
//	- If `reflow_type` is `fixed`, `layout` is required.
//	- If `reflow_type` is `auto`, `layout` should not be set.
//
// +k8s:openapi-gen=true
type Widget struct {
	// [Definition of the widget](https://docs.datadoghq.com/dashboards/widgets/).
	Definition WidgetDefinition `json:"definition"`
	// ID of the widget.
	Id *int64 `json:"id"`
	// The layout for a widget on a `free` or **new dashboard layout** dashboard.
	Layout WidgetLayout `json:"layout,omitempty"`
}

// WidgetLayout The layout for a widget on a `free` or **new dashboard layout** dashboard.
// +k8s:openapi-gen=true
type WidgetLayout struct {
	// The height of the widget. Should be a non-negative integer.
	Height int64 `json:"height"`
	// Whether the widget should be the first one on the second column in high density or not.
	// **Note**: Only for the **new dashboard layout** and only one widget in the dashboard should have this property set to `true`.
	IsColumnBreak *bool `json:"isColumnBreak,omitempty"`
	// The width of the widget. Should be a non-negative integer.
	Width int64 `json:"width"`
	// The position of the widget on the x (horizontal) axis. Should be a non-negative integer.
	X int64 `json:"x"`
	// The position of the widget on the y (vertical) axis. Should be a non-negative integer.
	Y int64 `json:"y"`
}

// TimeseriesWidgetDefinition The timeseries visualization allows you to display the evolution of one or more metrics, log events, or Indexed Spans over time.
type TimeseriesWidgetDefinition struct {
	// List of custom links.
	CustomLinks []WidgetCustomLink `json:"customLinks,omitempty"`
	// List of widget events.
	Events []WidgetEvent `json:"events,omitempty"`
	// Columns displayed in the legend.
	LegendColumns []datadogV1.TimeseriesWidgetLegendColumn `json:"legendColumns,omitempty"`
	// Layout of the legend.
	LegendLayout *datadogV1.TimeseriesWidgetLegendLayout `json:"legendLayout,omitempty"`
	// Available legend sizes for a widget. Should be one of "0", "2", "4", "8", "16", or "auto".
	LegendSize *string `json:"legendSize,omitempty"`
	// List of markers.
	Markers []WidgetMarker `json:"markers,omitempty"`
	// NOTE: should this be required? we will see
	// List of timeseries widget requests.
	Requests []TimeseriesWidgetRequest `json:"requests"`
	// Axis controls for the widget.
	RightYaxis *WidgetAxis `json:"rightYaxis,omitempty"`
	// (screenboard only) Show the legend for this widget.
	ShowLegend *bool `json:"showLegend,omitempty"`
	// Time setting for the widget.
	Time *WidgetTime `json:"time,omitempty"`
	// Title of your widget.
	Title *string `json:"title,omitempty"`
	// How to align the text on the widget.
	TitleAlign *datadogV1.WidgetTextAlign `json:"titleAlign,omitempty"`
	// Size of the title.
	TitleSize *string `json:"titleSize,omitempty"`
	// Type of the timeseries widget.
	Type datadogV1.TimeseriesWidgetDefinitionType `json:"type,omitempty"`
	// Axis controls for the widget.
	Yaxis *WidgetAxis `json:"yaxis,omitempty"`
}

// WidgetCustomLink Custom links help you connect a data value to a URL, like a Datadog page or your AWS console.
type WidgetCustomLink struct {
	// The flag for toggling context menu link visibility.
	IsHidden *bool `json:"isHidden,omitempty"`
	// The label for the custom link URL. Keep the label short and descriptive. Use metrics and tags as variables.
	Label *string `json:"label,omitempty"`
	// The URL of the custom link. URL must include `http` or `https`. A relative URL must start with `/`.
	Link *string `json:"link,omitempty"`
	// The label ID that refers to a context menu link. Can be `logs`, `hosts`, `traces`, `profiles`, `processes`, `containers`, or `rum`.
	OverrideLabel *string `json:"overrideLabel,omitempty"`
}

// WidgetEvent Event overlay control options.
//
// See the dedicated [Events JSON schema documentation](https://docs.datadoghq.com/dashboards/graphing_json/widget_json/#events-schema)
// to learn how to build the `<EVENTS_SCHEMA>`.
type WidgetEvent struct {
	// Query definition.
	Q string `json:"q"`
	// The execution method for multi-value filters.
	TagsExecution *string `json:"tagsExecution,omitempty"`
}

// Interface -->
// NOTE: this didn't have tags. How would I even assign it?
// WidgetDefinition - [Definition of the widget](https://docs.datadoghq.com/dashboards/widgets/).
// +k8s:openapi-gen=true
type WidgetDefinition struct {
	TimeseriesWidgetDefinition *TimeseriesWidgetDefinition `json:"timeseries,omitempty"`
	QueryValueWidgetDefinition *QueryValueWidgetDefinition `json:"queryValue,omitempty"`
}

// WidgetMarker Markers allow you to add visual conditional formatting for your graphs.
type WidgetMarker struct {
	// Combination of:
	//   - A severity error, warning, ok, or info
	//   - A line type: dashed, solid, or bold
	// In this case of a Distribution widget, this can be set to be `x_axis_percentile`.
	//
	DisplayType *string `json:"displayType,omitempty"`
	// Label to display over the marker.
	Label *string `json:"label,omitempty"`
	// Timestamp for the widget.
	Time *string `json:"time,omitempty"`
	// Value to apply. Can be a single value y = 15 or a range of values 0 < y < 10.
	Value string `json:"value"`
}

// TimeseriesWidgetRequest Updated timeseries widget.
type TimeseriesWidgetRequest struct {
	// The log query.
	ApmQuery *LogQueryDefinition `json:"apmQuery,omitempty"`
	// The log query.
	AuditQuery *LogQueryDefinition `json:"auditQuery,omitempty"`
	// Type of display to use for the request.
	DisplayType *datadogV1.WidgetDisplayType `json:"displayType,omitempty"`
	// The log query.
	EventQuery *LogQueryDefinition `json:"eventQuery,omitempty"`
	// List of formulas that operate on queries.
	Formulas []WidgetFormula `json:"formulas,omitempty"`
	// The log query.
	LogQuery *LogQueryDefinition `json:"logQuery,omitempty"`
	// Used to define expression aliases.
	Metadata []TimeseriesWidgetExpressionAlias `json:"metadata,omitempty"`
	// The log query.
	NetworkQuery *LogQueryDefinition `json:"networkQuery,omitempty"`
	// Whether or not to display a second y-axis on the right.
	OnRightYaxis *bool `json:"onRightYaxis,omitempty"`
	// The process query to use in the widget.
	ProcessQuery *ProcessQueryDefinition `json:"processQuery,omitempty"`
	// The log query.
	ProfileMetricsQuery *LogQueryDefinition `json:"profileMetricsQuery,omitempty"`
	// Widget query.
	Q *string `json:"q,omitempty"`
	// List of queries that can be returned directly or used in formulas.
	Queries []FormulaAndFunctionQueryDefinition `json:"queries,omitempty"`
	// Timeseries, scalar, or event list response. Event list response formats are supported by Geomap widgets.
	ResponseFormat *datadogV1.FormulaAndFunctionResponseFormat `json:"responseFormat,omitempty"`
	// The log query.
	RumQuery *LogQueryDefinition `json:"rumQuery,omitempty"`
	// The log query.
	SecurityQuery *LogQueryDefinition `json:"securityQuery,omitempty"`
	// Define request widget style.
	Style *WidgetRequestStyle `json:"style,omitempty"`
}

// TimeseriesWidgetExpressionAlias Define an expression alias.
type TimeseriesWidgetExpressionAlias struct {
	// Expression alias.
	AliasName *string `json:"aliasName,omitempty"`
	// Expression name.
	Expression string `json:"expression"`
}

// LogQueryDefinition The log query.
type LogQueryDefinition struct {
	// Define computation for a log query.
	Compute *LogsQueryCompute `json:"compute,omitempty"`
	// List of tag prefixes to group by in the case of a cluster check.
	GroupBy []LogQueryDefinitionGroupBy `json:"groupBy,omitempty"`
	// A coma separated-list of index names. Use "*" query all indexes at once. [Multiple Indexes](https://docs.datadoghq.com/logs/indexes/#multiple-indexes)
	Index *string `json:"index,omitempty"`
	// This field is mutually exclusive with `compute`.
	MultiCompute []LogsQueryCompute `json:"multiCompute,omitempty"`
	// The query being made on the logs.
	Search *LogQueryDefinitionSearch `json:"search,omitempty"`
}

// LogsQueryCompute Define computation for a log query.
type LogsQueryCompute struct {
	// The aggregation method.
	Aggregation string `json:"aggregation"`
	// Facet name.
	Facet *string `json:"facet,omitempty"`
	// Define a time interval in seconds.
	Interval *int64 `json:"interval,omitempty"`
}

// LogQueryDefinitionGroupBy Defined items in the group.
type LogQueryDefinitionGroupBy struct {
	// Facet name.
	Facet string `json:"facet"`
	// Maximum number of items in the group.
	Limit *int64 `json:"limit,omitempty"`
	// Define a sorting method.
	Sort *LogQueryDefinitionGroupBySort `json:"sort,omitempty"`
}

// LogQueryDefinitionGroupBySort Define a sorting method.
type LogQueryDefinitionGroupBySort struct {
	// The aggregation method.
	Aggregation string `json:"aggregation"`
	// Facet name.
	Facet *string `json:"facet,omitempty"`
	// Widget sorting methods.
	Order datadogV1.WidgetSort `json:"order"`
}

// LogQueryDefinitionSearch The query being made on the logs.
type LogQueryDefinitionSearch struct {
	// Search value to apply.
	Query string `json:"query"`
}

// ProcessQueryDefinition The process query to use in the widget.
type ProcessQueryDefinition struct {
	// List of processes.
	FilterBy []string `json:"filterBy,omitempty"`
	// Max number of items in the filter list.
	Limit *int64 `json:"limit,omitempty"`
	// Your chosen metric.
	Metric string `json:"metric"`
	// Your chosen search term.
	SearchBy *string `json:"searchBy,omitempty"`
}

// NOTE: this struct does/did not have json tags..., might need to be an interface
// FormulaAndFunctionQueryDefinition - A formula and function query.
type FormulaAndFunctionQueryDefinition struct {
	FormulaAndFunctionMetricQueryDefinition             *FormulaAndFunctionMetricQueryDefinition             `json:"metricQuery,omitempty"`
	FormulaAndFunctionEventQueryDefinition              *FormulaAndFunctionEventQueryDefinition              `json:"eventQuery,omitempty"`
	FormulaAndFunctionProcessQueryDefinition            *FormulaAndFunctionProcessQueryDefinition            `json:"processQuery,omitempty"`
	FormulaAndFunctionApmDependencyStatsQueryDefinition *FormulaAndFunctionApmDependencyStatsQueryDefinition `json:"apmDependencyQuery,omitempty"`
	FormulaAndFunctionApmResourceStatsQueryDefinition   *FormulaAndFunctionApmResourceStatsQueryDefinition   `json:"apmResourceQuery,omitempty"`
	FormulaAndFunctionSLOQueryDefinition                *FormulaAndFunctionSLOQueryDefinition                `json:"sloQuery,omitempty"`
	FormulaAndFunctionCloudCostQueryDefinition          *FormulaAndFunctionCloudCostQueryDefinition          `json:"cloudCostQuery,omitempty"`
}

// FormulaAndFunctionMetricQueryDefinition A formula and functions metrics query.
type FormulaAndFunctionMetricQueryDefinition struct {
	// The aggregation methods available for metrics queries.
	Aggregator *datadogV1.FormulaAndFunctionMetricAggregation `json:"aggregator,omitempty"`
	// NOTE: do we need to support this?
	// The source organization UUID for cross organization queries. Feature in Private Beta.
	CrossOrgUuids []string `json:"crossOrgUuids,omitempty"`
	// Data source for metrics queries.
	DataSource datadogV1.FormulaAndFunctionMetricDataSource `json:"dataSource"`
	// Name of the query for use in formulas.
	Name string `json:"name"`
	// Metrics query definition.
	Query string `json:"query"`
}

// FormulaAndFunctionEventQueryDefinition A formula and functions events query.
type FormulaAndFunctionEventQueryDefinition struct {
	// Compute options.
	Compute *FormulaAndFunctionEventQueryDefinitionCompute `json:"compute"`
	// The source organization UUID for cross organization queries. Feature in Private Beta.
	CrossOrgUuids []string `json:"crossOrgUuids,omitempty"`
	// Data source for event platform-based queries.
	DataSource datadogV1.FormulaAndFunctionEventsDataSource `json:"dataSource"`
	// Group by options.
	GroupBy []FormulaAndFunctionEventQueryGroupBy `json:"groupBy,omitempty"`
	// An array of index names to query in the stream. Omit or use `[]` to query all indexes at once.
	Indexes []string `json:"indexes,omitempty"`
	// Name of the query for use in formulas.
	Name string `json:"name"`
	// Search options.
	Search *FormulaAndFunctionEventQueryDefinitionSearch `json:"search,omitempty"`
	// Option for storage location. Feature in Private Beta.
	Storage *string `json:"storage,omitempty"`
}

// FormulaAndFunctionEventQueryDefinitionCompute Compute options.
type FormulaAndFunctionEventQueryDefinitionCompute struct {
	// Aggregation methods for event platform queries.
	Aggregation datadogV1.FormulaAndFunctionEventAggregation `json:"aggregation"`
	// A time interval in milliseconds.
	Interval *int64 `json:"interval,omitempty"`
	// Measurable attribute to compute.
	Metric *string `json:"metric,omitempty"`
}

// FormulaAndFunctionEventQueryGroupBy List of objects used to group by.
type FormulaAndFunctionEventQueryGroupBy struct {
	// Event facet.
	Facet string `json:"facet"`
	// Number of groups to return.
	Limit *int64 `json:"limit,omitempty"`
	// Options for sorting group by results.
	Sort *FormulaAndFunctionEventQueryGroupBySort `json:"sort,omitempty"`
}

// FormulaAndFunctionEventQueryGroupBySort Options for sorting group by results.
type FormulaAndFunctionEventQueryGroupBySort struct {
	// Aggregation methods for event platform queries.
	Aggregation datadogV1.FormulaAndFunctionEventAggregation `json:"aggregation"`
	// Metric used for sorting group by results.
	Metric *string `json:"metric,omitempty"`
	// Direction of sort.
	Order *datadogV1.QuerySortOrder `json:"order,omitempty"`
}

// FormulaAndFunctionEventQueryDefinitionSearch Search options.
type FormulaAndFunctionEventQueryDefinitionSearch struct {
	// Events search string.
	Query string `json:"query"`
}

// FormulaAndFunctionProcessQueryDefinition Process query using formulas and functions.
type FormulaAndFunctionProcessQueryDefinition struct {
	// The aggregation methods available for metrics queries.
	Aggregator *datadogV1.FormulaAndFunctionMetricAggregation `json:"aggregator,omitempty"`
	// The source organization UUID for cross organization queries. Feature in Private Beta.
	CrossOrgUuids []string `json:"crossOrgUuids,omitempty"`
	// Data sources that rely on the process backend.
	DataSource datadogV1.FormulaAndFunctionProcessQueryDataSource `json:"dataSource"`
	// Whether to normalize the CPU percentages.
	IsNormalizedCpu *bool `json:"isNormalizedCpu,omitempty"`
	// Number of hits to return.
	Limit *int64 `json:"limit,omitempty"`
	// Process metric name.
	Metric string `json:"metric"`
	// Name of query for use in formulas.
	Name string `json:"name"`
	// Direction of sort.
	Sort *datadogV1.QuerySortOrder `json:"sort,omitempty"`
	// An array of tags to filter by.
	TagFilters []string `json:"tagFilters,omitempty"`
	// Text to use as filter.
	TextFilter *string `json:"textFilter,omitempty"`
}

// FormulaAndFunctionApmDependencyStatsQueryDefinition A formula and functions APM dependency stats query.
type FormulaAndFunctionApmDependencyStatsQueryDefinition struct {
	// The source organization UUID for cross organization queries. Feature in Private Beta.
	CrossOrgUuids []string `json:"crossOrgUuids,omitempty"`
	// Data source for APM dependency stats queries.
	DataSource datadogV1.FormulaAndFunctionApmDependencyStatsDataSource `json:"dataSource"`
	// APM environment.
	Env string `json:"env"`
	// Determines whether stats for upstream or downstream dependencies should be queried.
	IsUpstream *bool `json:"isUpstream,omitempty"`
	// Name of query to use in formulas.
	Name string `json:"name"`
	// Name of operation on service.
	OperationName string `json:"operationName"`
	// The name of the second primary tag used within APM; required when `primary_tag_value` is specified. See https://docs.datadoghq.com/tracing/guide/setting_primary_tags_to_scope/#add-a-second-primary-tag-in-datadog.
	PrimaryTagName *string `json:"primaryTagName,omitempty"`
	// Filter APM data by the second primary tag. `primary_tag_name` must also be specified.
	PrimaryTagValue *string `json:"primaryTagValue,omitempty"`
	// APM resource.
	ResourceName string `json:"resourceName"`
	// APM service.
	Service string `json:"service"`
	// APM statistic.
	Stat datadogV1.FormulaAndFunctionApmDependencyStatName `json:"stat"`
}

// FormulaAndFunctionApmResourceStatsQueryDefinition APM resource stats query using formulas and functions.
type FormulaAndFunctionApmResourceStatsQueryDefinition struct {
	// The source organization UUID for cross organization queries. Feature in Private Beta.
	CrossOrgUuids []string `json:"crossOrgUuids,omitempty"`
	// Data source for APM resource stats queries.
	DataSource datadogV1.FormulaAndFunctionApmResourceStatsDataSource `json:"dataSource"`
	// APM environment.
	Env string `json:"env"`
	// Array of fields to group results by.
	GroupBy []string `json:"groupBy,omitempty"`
	// Name of this query to use in formulas.
	Name string `json:"name"`
	// Name of operation on service.
	OperationName *string `json:"operationName,omitempty"`
	// Name of the second primary tag used within APM. Required when `primary_tag_value` is specified. See https://docs.datadoghq.com/tracing/guide/setting_primary_tags_to_scope/#add-a-second-primary-tag-in-datadog
	PrimaryTagName *string `json:"primaryTagName,omitempty"`
	// Value of the second primary tag by which to filter APM data. `primary_tag_name` must also be specified.
	PrimaryTagValue *string `json:"primaryTagValue,omitempty"`
	// APM resource name.
	ResourceName *string `json:"resourceName,omitempty"`
	// APM service name.
	Service string `json:"service"`
	// APM resource stat name.
	Stat datadogV1.FormulaAndFunctionApmResourceStatName `json:"stat"`
}

// FormulaAndFunctionSLOQueryDefinition A formula and functions metrics query.
type FormulaAndFunctionSLOQueryDefinition struct {
	// Additional filters applied to the SLO query.
	AdditionalQueryFilters *string `json:"additionalQueryFilters,omitempty"`
	// The source organization UUID for cross organization queries. Feature in Private Beta.
	CrossOrgUuids []string `json:"crossOrgUuids,omitempty"`
	// Data source for SLO measures queries.
	DataSource datadogV1.FormulaAndFunctionSLODataSource `json:"dataSource"`
	// Group mode to query measures.
	GroupMode *datadogV1.FormulaAndFunctionSLOGroupMode `json:"groupMode,omitempty"`
	// SLO measures queries.
	Measure datadogV1.FormulaAndFunctionSLOMeasure `json:"measure"`
	// Name of the query for use in formulas.
	Name *string `json:"name,omitempty"`
	// ID of an SLO to query measures.
	SloId string `json:"sloId"`
	// Name of the query for use in formulas.
	SloQueryType *datadogV1.FormulaAndFunctionSLOQueryType `json:"sloQueryType,omitempty"`
}

// FormulaAndFunctionCloudCostQueryDefinition A formula and functions Cloud Cost query.
type FormulaAndFunctionCloudCostQueryDefinition struct {
	// Aggregator used for the request.
	Aggregator *datadogV1.WidgetAggregator `json:"aggregator,omitempty"`
	// The source organization UUID for cross organization queries. Feature in Private Beta.
	CrossOrgUuids []string `json:"crossOrgUuids,omitempty"`
	// Data source for Cloud Cost queries.
	DataSource datadogV1.FormulaAndFunctionCloudCostDataSource `json:"dataSource"`
	// Name of the query for use in formulas.
	Name string `json:"name"`
	// Query for Cloud Cost data.
	Query string `json:"query"`
}

// WidgetRequestStyle Define request widget style.
type WidgetRequestStyle struct {
	// Type of lines displayed.
	LineType *datadogV1.WidgetLineType `json:"lineType,omitempty"`
	// Width of line displayed.
	LineWidth *datadogV1.WidgetLineWidth `json:"lineWidth,omitempty"`
	// Color palette to apply to the widget.
	Palette *string `json:"palette,omitempty"`
}

// WidgetAxis Axis controls for the widget.
type WidgetAxis struct {
	// Set to `true` to include zero.
	IncludeZero *bool `json:"includeZero,omitempty"`
	// The label of the axis to display on the graph. Only usable on Scatterplot Widgets.
	Label *string `json:"label,omitempty"`
	// Specifies maximum numeric value to show on the axis. Defaults to `auto`.
	Max *string `json:"max,omitempty"`
	// Specifies minimum numeric value to show on the axis. Defaults to `auto`.
	Min *string `json:"min,omitempty"`
	// Specifies the scale type. Possible values are `linear`, `log`, `sqrt`, and `pow##` (for example `pow2` or `pow0.5`).
	Scale *string `json:"scale,omitempty"`
}

// WidgetTime Time setting for the widget.
type WidgetTime struct {
	// The available timeframes depend on the widget you are using.
	LiveSpan *datadogV1.WidgetLiveSpan `json:"liveSpan,omitempty"`
}

// WidgetFormula Formula to be used in a widget query.
type WidgetFormula struct {
	// Expression alias.
	Alias *string `json:"alias,omitempty"`
	// Define a display mode for the table cell.
	CellDisplayMode *datadogV1.TableWidgetCellDisplayMode `json:"cellDisplayMode,omitempty"`
	// List of conditional formats.
	ConditionalFormats []WidgetConditionalFormat `json:"conditionalFormats,omitempty"`
	// String expression built from queries, formulas, and functions.
	Formula string `json:"formula"`
	// Options for limiting results returned.
	Limit *WidgetFormulaLimit `json:"limit,omitempty"`
	// Styling options for widget formulas.
	Style *WidgetFormulaStyle `json:"style,omitempty"`
}

// WidgetConditionalFormat Define a conditional format for the widget.
type WidgetConditionalFormat struct {
	// Comparator to apply.
	Comparator datadogV1.WidgetComparator `json:"comparator"`
	// Color palette to apply to the background, same values available as palette.
	CustomBgColor *string `json:"customBgColor,omitempty"`
	// Color palette to apply to the foreground, same values available as palette.
	CustomFgColor *string `json:"customFgColor,omitempty"`
	// True hides values.
	HideValue *bool `json:"hideValue,omitempty"`
	// Displays an image as the background.
	ImageUrl *string `json:"imageUrl,omitempty"`
	// Metric from the request to correlate this conditional format with.
	Metric *string `json:"metric,omitempty"`
	// Color palette to apply.
	Palette datadogV1.WidgetPalette `json:"palette"`
	// Defines the displayed timeframe.
	Timeframe *string `json:"timeframe,omitempty"`
	// NOTE: turn into float
	// Value for the comparator.
	Value *resource.Quantity `json:"value"`
}

// WidgetFormulaLimit Options for limiting results returned.
type WidgetFormulaLimit struct {
	// Number of results to return.
	Count *int64 `json:"count,omitempty"`
	// Direction of sort.
	Order *datadogV1.QuerySortOrder `json:"order,omitempty"`
}

// WidgetFormulaStyle Styling options for widget formulas.
type WidgetFormulaStyle struct {
	// The color palette used to display the formula. A guide to the available color palettes can be found at https://docs.datadoghq.com/dashboards/guide/widget_colors
	Palette *string `json:"palette,omitempty"`
	// Index specifying which color to use within the palette.
	PaletteIndex *int64 `json:"paletteIndex,omitempty"`
}

// QueryValueWidgetDefinition Query values display the current value of a given metric, APM, or log query.
type QueryValueWidgetDefinition struct {
	// Whether to use auto-scaling or not.
	Autoscale *bool `json:"autoscale,omitempty"`
	// List of custom links.
	CustomLinks []WidgetCustomLink `json:"customLinks,omitempty"`
	// Display a unit of your choice on the widget.
	CustomUnit *string `json:"customUnit,omitempty"`
	// Number of decimals to show. If not defined, the widget uses the raw value.
	Precision *int64 `json:"precision,omitempty"`
	// Widget definition.
	Requests []QueryValueWidgetRequest `json:"requests"`
	// How to align the text on the widget.
	TextAlign *datadogV1.WidgetTextAlign `json:"textAlign,omitempty"`
	// Time setting for the widget.
	Time *WidgetTime `json:"time,omitempty"`
	// Set a timeseries on the widget background.
	TimeseriesBackground *TimeseriesBackground `json:"timeseriesBackground,omitempty"`
	// Title of your widget.
	Title *string `json:"title,omitempty"`
	// How to align the text on the widget.
	TitleAlign *datadogV1.WidgetTextAlign `json:"titleAlign,omitempty"`
	// Size of the title.
	TitleSize *string `json:"titleSize,omitempty"`
	// Type of the query value widget.
	Type datadogV1.QueryValueWidgetDefinitionType `json:"type"`
}

// TimeseriesBackground Set a timeseries on the widget background.
type TimeseriesBackground struct {
	// Timeseries is made using an area or bars.
	Type datadogV1.TimeseriesBackgroundType `json:"type"`
	// Axis controls for the widget.
	Yaxis *WidgetAxis `json:"yaxis,omitempty"`
}

// QueryValueWidgetRequest Updated query value widget.
type QueryValueWidgetRequest struct {
	// Aggregator used for the request.
	Aggregator *datadogV1.WidgetAggregator `json:"aggregator,omitempty"`
	// The log query.
	ApmQuery *LogQueryDefinition `json:"apmQuery,omitempty"`
	// The log query.
	AuditQuery *LogQueryDefinition `json:"auditQuery,omitempty"`
	// List of conditional formats.
	ConditionalFormats []WidgetConditionalFormat `json:"conditionalFormats,omitempty"`
	// The log query.
	EventQuery *LogQueryDefinition `json:"eventQuery,omitempty"`
	// List of formulas that operate on queries.
	Formulas []WidgetFormula `json:"formulas,omitempty"`
	// The log query.
	LogQuery *LogQueryDefinition `json:"logQuery,omitempty"`
	// The log query.
	NetworkQuery *LogQueryDefinition `json:"networkQuery,omitempty"`
	// The process query to use in the widget.
	ProcessQuery *ProcessQueryDefinition `json:"processQuery,omitempty"`
	// The log query.
	ProfileMetricsQuery *LogQueryDefinition `json:"profileMetricsQuery,omitempty"`
	// TODO.
	Q *string `json:"q,omitempty"`
	// List of queries that can be returned directly or used in formulas.
	Queries []FormulaAndFunctionQueryDefinition `json:"queries,omitempty"`
	// Timeseries, scalar, or event list response. Event list response formats are supported by Geomap widgets.
	ResponseFormat *datadogV1.FormulaAndFunctionResponseFormat `json:"responseFormat,omitempty"`
	// The log query.
	RumQuery *LogQueryDefinition `json:"rumQuery,omitempty"`
	// The log query.
	SecurityQuery *LogQueryDefinition `json:"securityQuery,omitempty"`
}

// DatadogDashboardList contains a list of DatadogDashboard
// +kubebuilder:object:root=true
type DatadogDashboardList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatadogDashboard `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatadogDashboard{}, &DatadogDashboardList{})
}
