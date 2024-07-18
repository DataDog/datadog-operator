// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DatadogDashboardSpec defines the desired state of DatadogDashboard
// +k8s:openapi-gen=true
type DatadogDashboardSpec struct {
	// Description of the dashboard.
	Description string `json:"description,omitempty"`
	// Layout type of the dashboard.
	LayoutType DashboardLayoutType `json:"layout_type"`
	// List of handles of users to notify when changes are made to this dashboard.
	NotifyList []string `json:"notify_list,omitempty"`
	// Reflow type for a **new dashboard layout** dashboard. Set this only when layout type is 'ordered'.
	// If set to 'fixed', the dashboard expects all widgets to have a layout, and if it's set to 'auto',
	// widgets should not have layouts.
	ReflowType *datadogV1.DashboardReflowType `json:"reflow_type,omitempty"`
	// List of team names representing ownership of a dashboard.
	Tags []string `json:"tags,omitempty"`
	// Array of template variables saved views.
	TemplateVariablePresets []DashboardTemplateVariablePreset `json:"template_variable_presets,omitempty"`
	// List of template variables for this dashboard.
	TemplateVariables []DashboardTemplateVariable `json:"template_variables,omitempty"`
	// Title of the dashboard.
	Title string `json:"title"`
	// List of widgets to display on the dashboard.
	Widgets []Widget `json:"widgets"`
}

// NOTE: Possibly move this since it doesn't fit the structure

// DashboardLayoutType Layout type of the dashboard.
type DashboardLayoutType string

// List of DashboardLayoutType.
const (
	DASHBOARDLAYOUTTYPE_ORDERED DashboardLayoutType = "ordered"
	DASHBOARDLAYOUTTYPE_FREE    DashboardLayoutType = "free"
)

var allowedDashboardLayoutTypeEnumValues = []DashboardLayoutType{
	DASHBOARDLAYOUTTYPE_ORDERED,
	DASHBOARDLAYOUTTYPE_FREE,
}

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
	Name *string `json:"name,omitempty"`
	// (deprecated) The value of the template variable within the saved view. Cannot be used in conjunction with `values`.
	// Deprecated
	Value *string `json:"value,omitempty"`
	// One or many template variable values within the saved view, which will be unioned together using `OR` if more than one is specified. Cannot be used in conjunction with `value`.
	Values []string `json:"values,omitempty"`
}

// DashboardTemplateVariablePreset Template variables saved views.
// +k8s:openapi-gen=true
type DashboardTemplateVariablePreset struct {
	// The name of the variable.
	Name *string `json:"name,omitempty"`
	// List of variables.
	TemplateVariables []DashboardTemplateVariablePresetValue `json:"template_variables,omitempty"`
}

// DashboardTemplateVariable Template variable.
// +k8s:openapi-gen=true
type DashboardTemplateVariable struct {
	// The list of values that the template variable drop-down is limited to.
	AvailableValues NullableList `json:"available_values,omitempty"`
	// (deprecated) The default value for the template variable on dashboard load. Cannot be used in conjunction with `defaults`.
	// Deprecated
	Default NullableString `json:"default,omitempty"`
	// One or many default values for template variables on load. If more than one default is specified, they will be unioned together with `OR`. Cannot be used in conjunction with `default`.
	Defaults []string `json:"defaults,omitempty"`
	// The name of the variable.
	Name string `json:"name"`
	// The tag prefix associated with the variable. Only tags with this prefix appear in the variable drop-down.
	Prefix NullableString `json:"prefix,omitempty"`
}

// NOTE: had to uppercase to deal with "tagged but not exported" error (paraphrasing)
// NullableString is a struct to hold a nullable string value.
// +k8s:openapi-gen=true
type NullableString struct {
	Value *string `json:"value,omitempty"`
	IsSet bool    `json:"isSet,omitempty"`
}

// NullableList struct to hold nullable list value.
// NOTE: cast this to the nullable anything type later
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
	Id *int64 `json:"id,omitempty"`
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
	IsColumnBreak *bool `json:"is_column_break,omitempty"`
	// The width of the widget. Should be a non-negative integer.
	Width int64 `json:"width"`
	// The position of the widget on the x (horizontal) axis. Should be a non-negative integer.
	X int64 `json:"x"`
	// The position of the widget on the y (vertical) axis. Should be a non-negative integer.
	Y int64 `json:"y"`
}

// // WidgetTime Time setting for the widget.
// type WidgetTime struct {
// 	// The available timeframes depend on the widget you are using.
// 	LiveSpan *datadogV1.WidgetLiveSpan `json:"live_span,omitempty"`
// }

// // WidgetCustomLink Custom links help you connect a data value to a URL, like a Datadog page or your AWS console.
// type WidgetCustomLink struct {
// 	// The flag for toggling context menu link visibility.
// 	IsHidden *bool `json:"is_hidden,omitempty"`
// 	// The label for the custom link URL. Keep the label short and descriptive. Use metrics and tags as variables.
// 	Label *string `json:"label,omitempty"`
// 	// The URL of the custom link. URL must include `http` or `https`. A relative URL must start with `/`.
// 	Link *string `json:"link,omitempty"`
// 	// The label ID that refers to a context menu link. Can be `logs`, `hosts`, `traces`, `profiles`, `processes`, `containers`, or `rum`.
// 	OverrideLabel *string `json:"override_label,omitempty"`
// }

// // ChangeWidgetRequest Updated change widget.
// type ChangeWidgetRequest struct {
// 	// The log query.
// 	ApmQuery *LogQueryDefinition `json:"apm_query,omitempty"`
// 	// Show the absolute or the relative change.
// 	ChangeType *datadogV1.WidgetChangeType `json:"change_type,omitempty"`
// 	// Timeframe used for the change comparison.
// 	CompareTo *datadogV1.WidgetCompareTo `json:"compare_to,omitempty"`
// 	// The log query.
// 	EventQuery *LogQueryDefinition `json:"event_query,omitempty"`
// 	// List of formulas that operate on queries.
// 	Formulas []WidgetFormula `json:"formulas,omitempty"`
// 	// Whether to show increase as good.
// 	IncreaseGood *bool `json:"increase_good,omitempty"`
// 	// The log query.
// 	LogQuery *LogQueryDefinition `json:"log_query,omitempty"`
// 	// The log query.
// 	NetworkQuery *LogQueryDefinition `json:"network_query,omitempty"`
// 	// What to order by.
// 	OrderBy *datadogV1.WidgetOrderBy `json:"order_by,omitempty"`
// 	// Widget sorting methods.
// 	OrderDir *datadogV1.WidgetSort `json:"order_dir,omitempty"`
// 	// The process query to use in the widget.
// 	ProcessQuery *ProcessQueryDefinition `json:"process_query,omitempty"`
// 	// The log query.
// 	ProfileMetricsQuery *LogQueryDefinition `json:"profile_metrics_query,omitempty"`
// 	// Query definition.
// 	Q *string `json:"q,omitempty"`
// 	// List of queries that can be returned directly or used in formulas.
// 	Queries []FormulaAndFunctionQueryDefinition `json:"queries,omitempty"`
// 	// Timeseries, scalar, or event list response. Event list response formats are supported by Geomap widgets.
// 	ResponseFormat *datadogV1.FormulaAndFunctionResponseFormat `json:"response_format,omitempty"`
// 	// The log query.
// 	RumQuery *LogQueryDefinition `json:"rum_query,omitempty"`
// 	// The log query.
// 	SecurityQuery *LogQueryDefinition `json:"security_query,omitempty"`
// 	// Whether to show the present value.
// 	ShowPresent *bool `json:"show_present,omitempty"`
// }

// // ProcessQueryDefinition The process query to use in the widget.
// type ProcessQueryDefinition struct {
// 	// List of processes.
// 	FilterBy []string `json:"filter_by,omitempty"`
// 	// Max number of items in the filter list.
// 	Limit *int64 `json:"limit,omitempty"`
// 	// Your chosen metric.
// 	Metric string `json:"metric"`
// 	// Your chosen search term.
// 	SearchBy *string `json:"search_by,omitempty"`
// }

// // LogQueryDefinition The log query.
// type LogQueryDefinition struct {
// 	// Define computation for a log query.
// 	Compute *LogsQueryCompute `json:"compute,omitempty"`
// 	// List of tag prefixes to group by in the case of a cluster check.
// 	GroupBy []LogQueryDefinitionGroupBy `json:"group_by,omitempty"`
// 	// A coma separated-list of index names. Use "*" query all indexes at once. [Multiple Indexes](https://docs.datadoghq.com/logs/indexes/#multiple-indexes)
// 	Index *string `json:"index,omitempty"`
// 	// This field is mutually exclusive with `compute`.
// 	MultiCompute []LogsQueryCompute `json:"multi_compute,omitempty"`
// 	// The query being made on the logs.
// 	Search *LogQueryDefinitionSearch `json:"search,omitempty"`
// }

// // LogsQueryCompute Define computation for a log query.
// type LogsQueryCompute struct {
// 	// The aggregation method.
// 	Aggregation string `json:"aggregation"`
// 	// Facet name.
// 	Facet *string `json:"facet,omitempty"`
// 	// Define a time interval in seconds.
// 	Interval *int64 `json:"interval,omitempty"`
// }

// // LogQueryDefinitionGroupBy Defined items in the group.
// type LogQueryDefinitionGroupBy struct {
// 	// Facet name.
// 	Facet string `json:"facet"`
// 	// Maximum number of items in the group.
// 	Limit *int64 `json:"limit,omitempty"`
// 	// Define a sorting method.
// 	Sort *LogQueryDefinitionGroupBySort `json:"sort,omitempty"`
// }

// // LogQueryDefinitionGroupBySort Define a sorting method.
// type LogQueryDefinitionGroupBySort struct {
// 	// The aggregation method.
// 	Aggregation string `json:"aggregation"`
// 	// Facet name.
// 	Facet *string `json:"facet,omitempty"`
// 	// Widget sorting methods.
// 	Order datadogV1.WidgetSort `json:"order"`
// }

// // LogQueryDefinitionSearch The query being made on the logs.
// type LogQueryDefinitionSearch struct {
// 	// Search value to apply.
// 	Query string `json:"query"`
// }

// // FormulaAndFunctionQueryDefinition - A formula and function query.
// type FormulaAndFunctionQueryDefinition struct {
// 	FormulaAndFunctionMetricQueryDefinition             *FormulaAndFunctionMetricQueryDefinition
// 	FormulaAndFunctionEventQueryDefinition              *FormulaAndFunctionEventQueryDefinition
// 	FormulaAndFunctionProcessQueryDefinition            *FormulaAndFunctionProcessQueryDefinition
// 	FormulaAndFunctionApmDependencyStatsQueryDefinition *FormulaAndFunctionApmDependencyStatsQueryDefinition
// 	FormulaAndFunctionApmResourceStatsQueryDefinition   *FormulaAndFunctionApmResourceStatsQueryDefinition
// 	FormulaAndFunctionSLOQueryDefinition                *FormulaAndFunctionSLOQueryDefinition
// 	FormulaAndFunctionCloudCostQueryDefinition          *FormulaAndFunctionCloudCostQueryDefinition

// 	// UnparsedObject contains the raw value of the object if there was an error when deserializing into the struct
// 	UnparsedObject interface{}
// }

// // FormulaAndFunctionCloudCostQueryDefinition A formula and functions Cloud Cost query.
// type FormulaAndFunctionCloudCostQueryDefinition struct {
// 	// Aggregator used for the request.
// 	Aggregator *datadogV1.WidgetAggregator `json:"aggregator,omitempty"`
// 	// The source organization UUID for cross organization queries. Feature in Private Beta.
// 	CrossOrgUuids []string `json:"cross_org_uuids,omitempty"`
// 	// Data source for Cloud Cost queries.
// 	DataSource datadogV1.FormulaAndFunctionCloudCostDataSource `json:"data_source"`
// 	// Name of the query for use in formulas.
// 	Name string `json:"name"`
// 	// Query for Cloud Cost data.
// 	Query string `json:"query"`
// }

// // FormulaAndFunctionApmResourceStatsQueryDefinition APM resource stats query using formulas and functions.
// type FormulaAndFunctionApmResourceStatsQueryDefinition struct {
// 	// The source organization UUID for cross organization queries. Feature in Private Beta.
// 	CrossOrgUuids []string `json:"cross_org_uuids,omitempty"`
// 	// Data source for APM resource stats queries.
// 	DataSource datadogV1.FormulaAndFunctionApmResourceStatsDataSource `json:"data_source"`
// 	// APM environment.
// 	Env string `json:"env"`
// 	// Array of fields to group results by.
// 	GroupBy []string `json:"group_by,omitempty"`
// 	// Name of this query to use in formulas.
// 	Name string `json:"name"`
// 	// Name of operation on service.
// 	OperationName *string `json:"operation_name,omitempty"`
// 	// Name of the second primary tag used within APM. Required when `primary_tag_value` is specified. See https://docs.datadoghq.com/tracing/guide/setting_primary_tags_to_scope/#add-a-second-primary-tag-in-datadog
// 	PrimaryTagName *string `json:"primary_tag_name,omitempty"`
// 	// Value of the second primary tag by which to filter APM data. `primary_tag_name` must also be specified.
// 	PrimaryTagValue *string `json:"primary_tag_value,omitempty"`
// 	// APM resource name.
// 	ResourceName *string `json:"resource_name,omitempty"`
// 	// APM service name.
// 	Service string `json:"service"`
// 	// APM resource stat name.
// 	Stat datadogV1.FormulaAndFunctionApmResourceStatName `json:"stat"`
// }

// // FormulaAndFunctionSLOQueryDefinition A formula and functions metrics query.
// type FormulaAndFunctionSLOQueryDefinition struct {
// 	// Additional filters applied to the SLO query.
// 	AdditionalQueryFilters *string `json:"additional_query_filters,omitempty"`
// 	// The source organization UUID for cross organization queries. Feature in Private Beta.
// 	CrossOrgUuids []string `json:"cross_org_uuids,omitempty"`
// 	// Data source for SLO measures queries.
// 	DataSource datadogV1.FormulaAndFunctionSLODataSource `json:"data_source"`
// 	// Group mode to query measures.
// 	GroupMode *datadogV1.FormulaAndFunctionSLOGroupMode `json:"group_mode,omitempty"`
// 	// SLO measures queries.
// 	Measure datadogV1.FormulaAndFunctionSLOMeasure `json:"measure"`
// 	// Name of the query for use in formulas.
// 	Name *string `json:"name,omitempty"`
// 	// ID of an SLO to query measures.
// 	SloId string `json:"slo_id"`
// 	// Name of the query for use in formulas.
// 	SloQueryType *datadogV1.FormulaAndFunctionSLOQueryType `json:"slo_query_type,omitempty"`
// }

// // FormulaAndFunctionProcessQueryDefinition Process query using formulas and functions.
// type FormulaAndFunctionProcessQueryDefinition struct {
// 	// The aggregation methods available for metrics queries.
// 	Aggregator *datadogV1.FormulaAndFunctionMetricAggregation `json:"aggregator,omitempty"`
// 	// The source organization UUID for cross organization queries. Feature in Private Beta.
// 	CrossOrgUuids []string `json:"cross_org_uuids,omitempty"`
// 	// Data sources that rely on the process backend.
// 	DataSource datadogV1.FormulaAndFunctionProcessQueryDataSource `json:"data_source"`
// 	// Whether to normalize the CPU percentages.
// 	IsNormalizedCpu *bool `json:"is_normalized_cpu,omitempty"`
// 	// Number of hits to return.
// 	Limit *int64 `json:"limit,omitempty"`
// 	// Process metric name.
// 	Metric string `json:"metric"`
// 	// Name of query for use in formulas.
// 	Name string `json:"name"`
// 	// Direction of sort.
// 	Sort *datadogV1.QuerySortOrder `json:"sort,omitempty"`
// 	// An array of tags to filter by.
// 	TagFilters []string `json:"tag_filters,omitempty"`
// 	// Text to use as filter.
// 	TextFilter *string `json:"text_filter,omitempty"`
// }

// // FormulaAndFunctionApmDependencyStatsQueryDefinition A formula and functions APM dependency stats query.
// type FormulaAndFunctionApmDependencyStatsQueryDefinition struct {
// 	// The source organization UUID for cross organization queries. Feature in Private Beta.
// 	CrossOrgUuids []string `json:"cross_org_uuids,omitempty"`
// 	// Data source for APM dependency stats queries.
// 	DataSource datadogV1.FormulaAndFunctionApmDependencyStatsDataSource `json:"data_source"`
// 	// APM environment.
// 	Env string `json:"env"`
// 	// Determines whether stats for upstream or downstream dependencies should be queried.
// 	IsUpstream *bool `json:"is_upstream,omitempty"`
// 	// Name of query to use in formulas.
// 	Name string `json:"name"`
// 	// Name of operation on service.
// 	OperationName string `json:"operation_name"`
// 	// The name of the second primary tag used within APM; required when `primary_tag_value` is specified. See https://docs.datadoghq.com/tracing/guide/setting_primary_tags_to_scope/#add-a-second-primary-tag-in-datadog.
// 	PrimaryTagName *string `json:"primary_tag_name,omitempty"`
// 	// Filter APM data by the second primary tag. `primary_tag_name` must also be specified.
// 	PrimaryTagValue *string `json:"primary_tag_value,omitempty"`
// 	// APM resource.
// 	ResourceName string `json:"resource_name"`
// 	// APM service.
// 	Service string `json:"service"`
// 	// APM statistic.
// 	Stat datadogV1.FormulaAndFunctionApmDependencyStatName `json:"stat"`
// }

// // FormulaAndFunctionMetricQueryDefinition A formula and functions metrics query.
// type FormulaAndFunctionMetricQueryDefinition struct {
// 	// The aggregation methods available for metrics queries.
// 	Aggregator *datadogV1.FormulaAndFunctionMetricAggregation `json:"aggregator,omitempty"`
// 	// The source organization UUID for cross organization queries. Feature in Private Beta.
// 	CrossOrgUuids []string `json:"cross_org_uuids,omitempty"`
// 	// Data source for metrics queries.
// 	DataSource datadogV1.FormulaAndFunctionMetricDataSource `json:"data_source"`
// 	// Name of the query for use in formulas.
// 	Name string `json:"name"`
// 	// Metrics query definition.
// 	Query string `json:"query"`
// }

// // FormulaAndFunctionEventQueryDefinition A formula and functions events query.
// type FormulaAndFunctionEventQueryDefinition struct {
// 	// Compute options.
// 	Compute FormulaAndFunctionEventQueryDefinitionCompute `json:"compute"`
// 	// The source organization UUID for cross organization queries. Feature in Private Beta.
// 	CrossOrgUuids []string `json:"cross_org_uuids,omitempty"`
// 	// Data source for event platform-based queries.
// 	DataSource datadogV1.FormulaAndFunctionEventsDataSource `json:"data_source"`
// 	// Group by options.
// 	GroupBy []FormulaAndFunctionEventQueryGroupBy `json:"group_by,omitempty"`
// 	// An array of index names to query in the stream. Omit or use `[]` to query all indexes at once.
// 	Indexes []string `json:"indexes,omitempty"`
// 	// Name of the query for use in formulas.
// 	Name string `json:"name"`
// 	// Search options.
// 	Search *FormulaAndFunctionEventQueryDefinitionSearch `json:"search,omitempty"`
// 	// Option for storage location. Feature in Private Beta.
// 	Storage *string `json:"storage,omitempty"`
// }

// // FormulaAndFunctionEventQueryDefinitionSearch Search options.
// type FormulaAndFunctionEventQueryDefinitionSearch struct {
// 	// Events search string.
// 	Query string `json:"query"`
// }

// // FormulaAndFunctionEventQueryGroupBy List of objects used to group by.
// type FormulaAndFunctionEventQueryGroupBy struct {
// 	// Event facet.
// 	Facet string `json:"facet"`
// 	// Number of groups to return.
// 	Limit *int64 `json:"limit,omitempty"`
// 	// Options for sorting group by results.
// 	Sort *FormulaAndFunctionEventQueryGroupBySort `json:"sort,omitempty"`
// }

// // FormulaAndFunctionEventQueryGroupBySort Options for sorting group by results.
// type FormulaAndFunctionEventQueryGroupBySort struct {
// 	// Aggregation methods for event platform queries.
// 	Aggregation datadogV1.FormulaAndFunctionEventAggregation `json:"aggregation"`
// 	// Metric used for sorting group by results.
// 	Metric *string `json:"metric,omitempty"`
// 	// Direction of sort.
// 	Order *datadogV1.QuerySortOrder `json:"order,omitempty"`
// }

// // FormulaAndFunctionEventQueryDefinitionCompute Compute options.
// type FormulaAndFunctionEventQueryDefinitionCompute struct {
// 	// Aggregation methods for event platform queries.
// 	Aggregation datadogV1.FormulaAndFunctionEventAggregation `json:"aggregation"`
// 	// A time interval in milliseconds.
// 	Interval *int64 `json:"interval,omitempty"`
// 	// Measurable attribute to compute.
// 	Metric *string `json:"metric,omitempty"`
// }

// // WidgetFormula Formula to be used in a widget query.
// type WidgetFormula struct {
// 	// Expression alias.
// 	Alias *string `json:"alias,omitempty"`
// 	// Define a display mode for the table cell.
// 	CellDisplayMode *datadogV1.TableWidgetCellDisplayMode `json:"cell_display_mode,omitempty"`
// 	// List of conditional formats.
// 	ConditionalFormats []WidgetConditionalFormat `json:"conditional_formats,omitempty"`
// 	// String expression built from queries, formulas, and functions.
// 	Formula string `json:"formula"`
// 	// Options for limiting results returned.
// 	Limit *WidgetFormulaLimit `json:"limit,omitempty"`
// 	// Styling options for widget formulas.
// 	Style *WidgetFormulaStyle `json:"style,omitempty"`
// }

// // WidgetFormulaStyle Styling options for widget formulas.
// type WidgetFormulaStyle struct {
// 	// The color palette used to display the formula. A guide to the available color palettes can be found at https://docs.datadoghq.com/dashboards/guide/widget_colors
// 	Palette *string `json:"palette,omitempty"`
// 	// Index specifying which color to use within the palette.
// 	PaletteIndex *int64 `json:"palette_index,omitempty"`
// }

// // WidgetFormulaLimit Options for limiting results returned.
// type WidgetFormulaLimit struct {
// 	// Number of results to return.
// 	Count *int64 `json:"count,omitempty"`
// 	// Direction of sort.
// 	Order *datadogV1.QuerySortOrder `json:"order,omitempty"`
// }

// // WidgetConditionalFormat Define a conditional format for the widget.
// type WidgetConditionalFormat struct {
// 	// Comparator to apply.
// 	Comparator datadogV1.WidgetComparator `json:"comparator"`
// 	// Color palette to apply to the background, same values available as palette.
// 	CustomBgColor *string `json:"custom_bg_color,omitempty"`
// 	// Color palette to apply to the foreground, same values available as palette.
// 	CustomFgColor *string `json:"custom_fg_color,omitempty"`
// 	// True hides values.
// 	HideValue *bool `json:"hide_value,omitempty"`
// 	// Displays an image as the background.
// 	ImageUrl *string `json:"image_url,omitempty"`
// 	// Metric from the request to correlate this conditional format with.
// 	Metric *string `json:"metric,omitempty"`
// 	// Color palette to apply.
// 	Palette datadogV1.WidgetPalette `json:"palette"`
// 	// Defines the displayed timeframe.
// 	Timeframe *string `json:"timeframe,omitempty"`
// 	// Value for the comparator.
// 	Value float64 `json:"value"`
// }

// // CheckStatusWidgetDefinition Check status shows the current status or number of results for any check performed.
// type CheckStatusWidgetDefinition struct {
// 	// Name of the check to use in the widget.
// 	Check string `json:"check"`
// 	// Group reporting a single check.
// 	Group *string `json:"group,omitempty"`
// 	// List of tag prefixes to group by in the case of a cluster check.
// 	GroupBy []string `json:"group_by,omitempty"`
// 	// The kind of grouping to use.
// 	Grouping datadogV1.WidgetGrouping `json:"grouping"`
// 	// List of tags used to filter the groups reporting a cluster check.
// 	Tags []string `json:"tags,omitempty"`
// 	// Time setting for the widget.
// 	Time *WidgetTime `json:"time,omitempty"`
// 	// Title of the widget.
// 	Title *string `json:"title,omitempty"`
// 	// How to align the text on the widget.
// 	TitleAlign *datadogV1.WidgetTextAlign `json:"title_align,omitempty"`
// 	// Size of the title.
// 	TitleSize *string `json:"title_size,omitempty"`
// 	// Type of the check status widget.
// 	Type datadogV1.CheckStatusWidgetDefinitionType `json:"type"`
// }

// // DistributionWidgetDefinition The Distribution visualization is another way of showing metrics
// // aggregated across one or several tags, such as hosts.
// // Unlike the heat map, a distribution graph’s x-axis is quantity rather than time.
// type DistributionWidgetDefinition struct {
// 	// A list of custom links.
// 	CustomLinks []WidgetCustomLink `json:"custom_links,omitempty"`
// 	// (Deprecated) The widget legend was replaced by a tooltip and sidebar.
// 	// Deprecated
// 	LegendSize *string `json:"legend_size,omitempty"`
// 	// List of markers.
// 	Markers []WidgetMarker `json:"markers,omitempty"`
// 	// Array of one request object to display in the widget.
// 	//
// 	// See the dedicated [Request JSON schema documentation](https://docs.datadoghq.com/dashboards/graphing_json/request_json)
// 	//  to learn how to build the `REQUEST_SCHEMA`.
// 	Requests []DistributionWidgetRequest `json:"requests"`
// 	// (Deprecated) The widget legend was replaced by a tooltip and sidebar.
// 	// Deprecated
// 	ShowLegend *bool `json:"show_legend,omitempty"`
// 	// Time setting for the widget.
// 	Time *WidgetTime `json:"time,omitempty"`
// 	// Title of the widget.
// 	Title *string `json:"title,omitempty"`
// 	// How to align the text on the widget.
// 	TitleAlign *datadogV1.WidgetTextAlign `json:"title_align,omitempty"`
// 	// Size of the title.
// 	TitleSize *string `json:"title_size,omitempty"`
// 	// Type of the distribution widget.
// 	Type datadogV1.DistributionWidgetDefinitionType `json:"type"`
// 	// X Axis controls for the distribution widget.
// 	Xaxis *DistributionWidgetXAxis `json:"xaxis,omitempty"`
// 	// Y Axis controls for the distribution widget.
// 	Yaxis *DistributionWidgetYAxis `json:"yaxis,omitempty"`
// }

// // WidgetMarker Markers allow you to add visual conditional formatting for your graphs.
// type WidgetMarker struct {
// 	// Combination of:
// 	//   - A severity error, warning, ok, or info
// 	//   - A line type: dashed, solid, or bold
// 	// In this case of a Distribution widget, this can be set to be `x_axis_percentile`.
// 	//
// 	DisplayType *string `json:"display_type,omitempty"`
// 	// Label to display over the marker.
// 	Label *string `json:"label,omitempty"`
// 	// Timestamp for the widget.
// 	Time *string `json:"time,omitempty"`
// 	// Value to apply. Can be a single value y = 15 or a range of values 0 < y < 10.
// 	Value string `json:"value"`
// }

// // DistributionWidgetRequest Updated distribution widget.
// type DistributionWidgetRequest struct {
// 	// The log query.
// 	ApmQuery *LogQueryDefinition `json:"apm_query,omitempty"`
// 	// The APM stats query for table and distributions widgets.
// 	ApmStatsQuery *ApmStatsQueryDefinition `json:"apm_stats_query,omitempty"`
// 	// The log query.
// 	EventQuery *LogQueryDefinition `json:"event_query,omitempty"`
// 	// The log query.
// 	LogQuery *LogQueryDefinition `json:"log_query,omitempty"`
// 	// The log query.
// 	NetworkQuery *LogQueryDefinition `json:"network_query,omitempty"`
// 	// The process query to use in the widget.
// 	ProcessQuery *ProcessQueryDefinition `json:"process_query,omitempty"`
// 	// The log query.
// 	ProfileMetricsQuery *LogQueryDefinition `json:"profile_metrics_query,omitempty"`
// 	// Widget query.
// 	Q *string `json:"q,omitempty"`
// 	// Query definition for Distribution Widget Histogram Request
// 	Query *DistributionWidgetHistogramRequestQuery `json:"query,omitempty"`
// 	// Request type for the histogram request.
// 	RequestType *datadogV1.DistributionWidgetHistogramRequestType `json:"request_type,omitempty"`
// 	// The log query.
// 	RumQuery *LogQueryDefinition `json:"rum_query,omitempty"`
// 	// The log query.
// 	SecurityQuery *LogQueryDefinition `json:"security_query,omitempty"`
// 	// Widget style definition.
// 	Style *WidgetStyle `json:"style,omitempty"`
// }

// // ApmStatsQueryDefinition The APM stats query for table and distributions widgets.
// type ApmStatsQueryDefinition struct {
// 	// Column properties used by the front end for display.
// 	Columns []datadogV1.ApmStatsQueryColumnType `json:"columns,omitempty"`
// 	// Environment name.
// 	Env string `json:"env"`
// 	// Operation name associated with service.
// 	Name string `json:"name"`
// 	// The organization's host group name and value.
// 	PrimaryTag string `json:"primary_tag"`
// 	// Resource name.
// 	Resource *string `json:"resource,omitempty"`
// 	// The level of detail for the request.
// 	RowType datadogV1.ApmStatsQueryRowType `json:"row_type"`
// 	// Service name.
// 	Service string `json:"service"`
// }

// // DistributionWidgetHistogramRequestQuery - Query definition for Distribution Widget Histogram Request
// type DistributionWidgetHistogramRequestQuery struct {
// 	FormulaAndFunctionMetricQueryDefinition           *FormulaAndFunctionMetricQueryDefinition
// 	FormulaAndFunctionEventQueryDefinition            *FormulaAndFunctionEventQueryDefinition
// 	FormulaAndFunctionApmResourceStatsQueryDefinition *FormulaAndFunctionApmResourceStatsQueryDefinition

// 	// UnparsedObject contains the raw value of the object if there was an error when deserializing into the struct
// 	UnparsedObject interface{}
// }

// // WidgetStyle Widget style definition.
// type WidgetStyle struct {
// 	// Color palette to apply to the widget.
// 	Palette *string `json:"palette,omitempty"`
// }

// // DistributionWidgetXAxis X Axis controls for the distribution widget.
// type DistributionWidgetXAxis struct {
// 	// True includes zero.
// 	IncludeZero *bool `json:"include_zero,omitempty"`
// 	// Specifies maximum value to show on the x-axis. It takes a number, percentile (p90 === 90th percentile), or auto for default behavior.
// 	Max *string `json:"max,omitempty"`
// 	// Specifies minimum value to show on the x-axis. It takes a number, percentile (p90 === 90th percentile), or auto for default behavior.
// 	Min *string `json:"min,omitempty"`
// 	// Specifies the scale type. Possible values are `linear`.
// 	Scale *string `json:"scale,omitempty"`
// }

// // DistributionWidgetYAxis Y Axis controls for the distribution widget.
// type DistributionWidgetYAxis struct {
// 	// True includes zero.
// 	IncludeZero *bool `json:"include_zero,omitempty"`
// 	// The label of the axis to display on the graph.
// 	Label *string `json:"label,omitempty"`
// 	// Specifies the maximum value to show on the y-axis. It takes a number, or auto for default behavior.
// 	Max *string `json:"max,omitempty"`
// 	// Specifies minimum value to show on the y-axis. It takes a number, or auto for default behavior.
// 	Min *string `json:"min,omitempty"`
// 	// Specifies the scale type. Possible values are `linear` or `log`.
// 	Scale *string `json:"scale,omitempty"`
// }

// // EventStreamWidgetDefinition The event stream is a widget version of the stream of events
// // on the Event Stream view. Only available on FREE layout dashboards.
// type EventStreamWidgetDefinition struct {
// 	// Size to use to display an event.
// 	EventSize *datadogV1.WidgetEventSize `json:"event_size,omitempty"`
// 	// Query to filter the event stream with.
// 	Query string `json:"query"`
// 	// The execution method for multi-value filters. Can be either and or or.
// 	TagsExecution *string `json:"tags_execution,omitempty"`
// 	// Time setting for the widget.
// 	Time *WidgetTime `json:"time,omitempty"`
// 	// Title of the widget.
// 	Title *string `json:"title,omitempty"`
// 	// How to align the text on the widget.
// 	TitleAlign *datadogV1.WidgetTextAlign `json:"title_align,omitempty"`
// 	// Size of the title.
// 	TitleSize *string `json:"title_size,omitempty"`
// 	// Type of the event stream widget.
// 	Type datadogV1.EventStreamWidgetDefinitionType `json:"type"`
// }

// // EventTimelineWidgetDefinition The event timeline is a widget version of the timeline that appears at the top of the Event Stream view. Only available on FREE layout dashboards.
// type EventTimelineWidgetDefinition struct {
// 	// Query to filter the event timeline with.
// 	Query string `json:"query"`
// 	// The execution method for multi-value filters. Can be either and or or.
// 	TagsExecution *string `json:"tags_execution,omitempty"`
// 	// Time setting for the widget.
// 	Time *WidgetTime `json:"time,omitempty"`
// 	// Title of the widget.
// 	Title *string `json:"title,omitempty"`
// 	// How to align the text on the widget.
// 	TitleAlign *datadogV1.WidgetTextAlign `json:"title_align,omitempty"`
// 	// Size of the title.
// 	TitleSize *string `json:"title_size,omitempty"`
// 	// Type of the event timeline widget.
// 	Type datadogV1.EventTimelineWidgetDefinitionType `json:"type"`
// }

// // FreeTextWidgetDefinition Free text is a widget that allows you to add headings to your screenboard. Commonly used to state the overall purpose of the dashboard. Only available on FREE layout dashboards.
// type FreeTextWidgetDefinition struct {
// 	// Color of the text.
// 	Color *string `json:"color,omitempty"`
// 	// Size of the text.
// 	FontSize *string `json:"font_size,omitempty"`
// 	// Text to display.
// 	Text string `json:"text"`
// 	// How to align the text on the widget.
// 	TextAlign *datadogV1.WidgetTextAlign `json:"text_align,omitempty"`
// 	// Type of the free text widget.
// 	Type datadogV1.FreeTextWidgetDefinitionType `json:"type"`
// }

// // FunnelWidgetDefinition The funnel visualization displays a funnel of user sessions that maps a sequence of view navigation and user interaction in your application.
// type FunnelWidgetDefinition struct {
// 	// Request payload used to query items.
// 	Requests []FunnelWidgetRequest `json:"requests"`
// 	// Time setting for the widget.
// 	Time *WidgetTime `json:"time,omitempty"`
// 	// The title of the widget.
// 	Title *string `json:"title,omitempty"`
// 	// How to align the text on the widget.
// 	TitleAlign *datadogV1.WidgetTextAlign `json:"title_align,omitempty"`
// 	// The size of the title.
// 	TitleSize *string `json:"title_size,omitempty"`
// 	// Type of funnel widget.
// 	Type datadogV1.FunnelWidgetDefinitionType `json:"type"`
// }

// // FunnelWidgetRequest Updated funnel widget.
// type FunnelWidgetRequest struct {
// 	// Updated funnel widget.
// 	Query FunnelQuery `json:"query"`
// 	// Widget request type.
// 	RequestType datadogV1.FunnelRequestType `json:"request_type"`
// }

// // FunnelQuery Updated funnel widget.
// type FunnelQuery struct {
// 	// Source from which to query items to display in the funnel.
// 	DataSource datadogV1.FunnelSource `json:"data_source"`
// 	// The widget query.
// 	QueryString string `json:"query_string"`
// 	// List of funnel steps.
// 	Steps []FunnelStep `json:"steps"`
// }

// // FunnelStep The funnel step.
// type FunnelStep struct {
// 	// The facet of the step.
// 	Facet string `json:"facet"`
// 	// The value of the step.
// 	Value string `json:"value"`
// }

// // GeomapWidgetDefinition This visualization displays a series of values by country on a world map.
// type GeomapWidgetDefinition struct {
// 	// A list of custom links.
// 	CustomLinks []WidgetCustomLink `json:"custom_links,omitempty"`
// 	// Array of one request object to display in the widget. The request must contain a `group-by` tag whose value is a country ISO code.
// 	//
// 	// See the [Request JSON schema documentation](https://docs.datadoghq.com/dashboards/graphing_json/request_json)
// 	// for information about building the `REQUEST_SCHEMA`.
// 	Requests []GeomapWidgetRequest `json:"requests"`
// 	// The style to apply to the widget.
// 	Style GeomapWidgetDefinitionStyle `json:"style"`
// 	// Time setting for the widget.
// 	Time *WidgetTime `json:"time,omitempty"`
// 	// The title of your widget.
// 	Title *string `json:"title,omitempty"`
// 	// How to align the text on the widget.
// 	TitleAlign *datadogV1.WidgetTextAlign `json:"title_align,omitempty"`
// 	// The size of the title.
// 	TitleSize *string `json:"title_size,omitempty"`
// 	// Type of the geomap widget.
// 	Type datadogV1.GeomapWidgetDefinitionType `json:"type"`
// 	// The view of the world that the map should render.
// 	View GeomapWidgetDefinitionView `json:"view"`
// }

// // GeomapWidgetRequest An updated geomap widget.
// type GeomapWidgetRequest struct {
// 	// Widget columns.
// 	Columns []ListStreamColumn `json:"columns,omitempty"`
// 	// List of formulas that operate on queries.
// 	Formulas []WidgetFormula `json:"formulas,omitempty"`
// 	// The log query.
// 	LogQuery *LogQueryDefinition `json:"log_query,omitempty"`
// 	// The widget metrics query.
// 	Q *string `json:"q,omitempty"`
// 	// List of queries that can be returned directly or used in formulas.
// 	Queries []FormulaAndFunctionQueryDefinition `json:"queries,omitempty"`
// 	// Updated list stream widget.
// 	Query *ListStreamQuery `json:"query,omitempty"`
// 	// Timeseries, scalar, or event list response. Event list response formats are supported by Geomap widgets.
// 	ResponseFormat *datadogV1.FormulaAndFunctionResponseFormat `json:"response_format,omitempty"`
// 	// The log query.
// 	RumQuery *LogQueryDefinition `json:"rum_query,omitempty"`
// 	// The log query.
// 	SecurityQuery *LogQueryDefinition `json:"security_query,omitempty"`
// 	// The controls for sorting the widget.
// 	Sort *WidgetSortBy `json:"sort,omitempty"`
// }

// // ListStreamColumn Widget column.
// type ListStreamColumn struct {
// 	// Widget column field.
// 	Field string `json:"field"`
// 	// Widget column width.
// 	Width datadogV1.ListStreamColumnWidth `json:"width"`
// }

// // ListStreamQuery Updated list stream widget.
// type ListStreamQuery struct {
// 	// Compute configuration for the List Stream Widget. Compute can be used only with the logs_transaction_stream (from 1 to 5 items) list stream source.
// 	Compute []ListStreamComputeItems `json:"compute,omitempty"`
// 	// Source from which to query items to display in the stream.
// 	DataSource datadogV1.ListStreamSource `json:"data_source"`
// 	// Size to use to display an event.
// 	EventSize *datadogV1.WidgetEventSize `json:"event_size,omitempty"`
// 	// Group by configuration for the List Stream Widget. Group by can be used only with logs_pattern_stream (up to 3 items) or logs_transaction_stream (one group by item is required) list stream source.
// 	GroupBy []ListStreamGroupByItems `json:"group_by,omitempty"`
// 	// List of indexes.
// 	Indexes []string `json:"indexes,omitempty"`
// 	// Widget query.
// 	QueryString string `json:"query_string"`
// 	// Which column and order to sort by
// 	Sort *WidgetFieldSort `json:"sort,omitempty"`
// 	// Option for storage location. Feature in Private Beta.
// 	Storage *string `json:"storage,omitempty"`
// }

// // ListStreamComputeItems List of facets and aggregations which to compute.
// type ListStreamComputeItems struct {
// 	// Aggregation value.
// 	Aggregation datadogV1.ListStreamComputeAggregation `json:"aggregation"`
// 	// Facet name.
// 	Facet *string `json:"facet,omitempty"`
// }

// // ListStreamGroupByItems List of facets on which to group.
// type ListStreamGroupByItems struct {
// 	// Facet name.
// 	Facet string `json:"facet"`
// }

// // WidgetFieldSort Which column and order to sort by
// type WidgetFieldSort struct {
// 	// Facet path for the column
// 	Column string `json:"column"`
// 	// Widget sorting methods.
// 	Order datadogV1.WidgetSort `json:"order"`
// }

// // WidgetSortBy The controls for sorting the widget.
// type WidgetSortBy struct {
// 	// The number of items to limit the widget to.
// 	Count *int64 `json:"count,omitempty"`
// 	// The array of items to sort the widget by in order.
// 	OrderBy []WidgetSortOrderBy `json:"order_by,omitempty"`
// }

// // WidgetSortOrderBy - The item to sort the widget by.
// type WidgetSortOrderBy struct {
// 	WidgetFormulaSort *WidgetFormulaSort
// 	WidgetGroupSort   *WidgetGroupSort

// 	// UnparsedObject contains the raw value of the object if there was an error when deserializing into the struct
// 	UnparsedObject interface{}
// }

// // WidgetFormulaSort The formula to sort the widget by.
// type WidgetFormulaSort struct {
// 	// The index of the formula to sort by.
// 	Index int64 `json:"index"`
// 	// Widget sorting methods.
// 	Order datadogV1.WidgetSort `json:"order"`
// 	// Set the sort type to formula.
// 	// NOTE: It Should exist...
// 	Type datadogV1.FormulaType `json:"type"`
// }

// // WidgetGroupSort The group to sort the widget by.
// type WidgetGroupSort struct {
// 	// The name of the group.
// 	Name string `json:"name"`
// 	// Widget sorting methods.
// 	Order datadogV1.WidgetSort `json:"order"`
// 	// Set the sort type to group.
// 	Type datadogV1.GroupType `json:"type"`
// }

// // GeomapWidgetDefinitionStyle The style to apply to the widget.
// type GeomapWidgetDefinitionStyle struct {
// 	// The color palette to apply to the widget.
// 	Palette string `json:"palette"`
// 	// Whether to flip the palette tones.
// 	PaletteFlip bool `json:"palette_flip"`
// }

// // GeomapWidgetDefinitionView The view of the world that the map should render.
// type GeomapWidgetDefinitionView struct {
// 	// The 2-letter ISO code of a country to focus the map on. Or `WORLD`.
// 	Focus string `json:"focus"`
// }

// // GroupWidgetDefinition The groups widget allows you to keep similar graphs together on your timeboard. Each group has a custom header, can hold one to many graphs, and is collapsible.
// type GroupWidgetDefinition struct {
// 	// Background color of the group title.
// 	BackgroundColor *string `json:"background_color,omitempty"`
// 	// URL of image to display as a banner for the group.
// 	BannerImg *string `json:"banner_img,omitempty"`
// 	// Layout type of the group.
// 	LayoutType datadogV1.WidgetLayoutType `json:"layout_type"`
// 	// Whether to show the title or not.
// 	ShowTitle *bool `json:"show_title,omitempty"`
// 	// Title of the widget.
// 	Title *string `json:"title,omitempty"`
// 	// How to align the text on the widget.
// 	TitleAlign *datadogV1.WidgetTextAlign `json:"title_align,omitempty"`
// 	// Type of the group widget.
// 	Type datadogV1.GroupWidgetDefinitionType `json:"type"`
// 	// List of widget groups.
// 	Widgets []Widget `json:"widgets"`
// }

// // HeatMapWidgetDefinition The heat map visualization shows metrics aggregated across many tags, such as hosts. The more hosts that have a particular value, the darker that square is.
// type HeatMapWidgetDefinition struct {
// 	// List of custom links.
// 	CustomLinks []WidgetCustomLink `json:"custom_links,omitempty"`
// 	// List of widget events.
// 	Events []WidgetEvent `json:"events,omitempty"`
// 	// Available legend sizes for a widget. Should be one of "0", "2", "4", "8", "16", or "auto".
// 	LegendSize *string `json:"legend_size,omitempty"`
// 	// List of widget types.
// 	Requests []HeatMapWidgetRequest `json:"requests"`
// 	// Whether or not to display the legend on this widget.
// 	ShowLegend *bool `json:"show_legend,omitempty"`
// 	// Time setting for the widget.
// 	Time *WidgetTime `json:"time,omitempty"`
// 	// Title of the widget.
// 	Title *string `json:"title,omitempty"`
// 	// How to align the text on the widget.
// 	TitleAlign *datadogV1.WidgetTextAlign `json:"title_align,omitempty"`
// 	// Size of the title.
// 	TitleSize *string `json:"title_size,omitempty"`
// 	// Type of the heat map widget.
// 	Type datadogV1.HeatMapWidgetDefinitionType `json:"type"`
// 	// Axis controls for the widget.
// 	Yaxis *WidgetAxis `json:"yaxis,omitempty"`
// }

// // WidgetEvent Event overlay control options.
// //
// // See the dedicated [Events JSON schema documentation](https://docs.datadoghq.com/dashboards/graphing_json/widget_json/#events-schema)
// // to learn how to build the `<EVENTS_SCHEMA>`.
// type WidgetEvent struct {
// 	// Query definition.
// 	Q string `json:"q"`
// 	// The execution method for multi-value filters.
// 	TagsExecution *string `json:"tags_execution,omitempty"`
// }

// // HeatMapWidgetRequest Updated heat map widget.
// type HeatMapWidgetRequest struct {
// 	// The log query.
// 	ApmQuery *LogQueryDefinition `json:"apm_query,omitempty"`
// 	// The event query.
// 	EventQuery *EventQueryDefinition `json:"event_query,omitempty"`
// 	// List of formulas that operate on queries.
// 	Formulas []WidgetFormula `json:"formulas,omitempty"`
// 	// The log query.
// 	LogQuery *LogQueryDefinition `json:"log_query,omitempty"`
// 	// The log query.
// 	NetworkQuery *LogQueryDefinition `json:"network_query,omitempty"`
// 	// The process query to use in the widget.
// 	ProcessQuery *ProcessQueryDefinition `json:"process_query,omitempty"`
// 	// The log query.
// 	ProfileMetricsQuery *LogQueryDefinition `json:"profile_metrics_query,omitempty"`
// 	// Widget query.
// 	Q *string `json:"q,omitempty"`
// 	// List of queries that can be returned directly or used in formulas.
// 	Queries []FormulaAndFunctionQueryDefinition `json:"queries,omitempty"`
// 	// Timeseries, scalar, or event list response. Event list response formats are supported by Geomap widgets.
// 	ResponseFormat *datadogV1.FormulaAndFunctionResponseFormat `json:"response_format,omitempty"`
// 	// The log query.
// 	RumQuery *LogQueryDefinition `json:"rum_query,omitempty"`
// 	// The log query.
// 	SecurityQuery *LogQueryDefinition `json:"security_query,omitempty"`
// 	// Widget style definition.
// 	Style *WidgetStyle `json:"style,omitempty"`
// }

// // EventQueryDefinition The event query.
// type EventQueryDefinition struct {
// 	// The query being made on the event.
// 	Search string `json:"search"`
// 	// The execution method for multi-value filters. Can be either and or or.
// 	TagsExecution string `json:"tags_execution"`
// }

// // WidgetAxis Axis controls for the widget.
// type WidgetAxis struct {
// 	// Set to `true` to include zero.
// 	IncludeZero *bool `json:"include_zero,omitempty"`
// 	// The label of the axis to display on the graph. Only usable on Scatterplot Widgets.
// 	Label *string `json:"label,omitempty"`
// 	// Specifies maximum numeric value to show on the axis. Defaults to `auto`.
// 	Max *string `json:"max,omitempty"`
// 	// Specifies minimum numeric value to show on the axis. Defaults to `auto`.
// 	Min *string `json:"min,omitempty"`
// 	// Specifies the scale type. Possible values are `linear`, `log`, `sqrt`, and `pow##` (for example `pow2` or `pow0.5`).
// 	Scale *string `json:"scale,omitempty"`
// }

// // HostMapWidgetDefinition The host map widget graphs any metric across your hosts using the same visualization available from the main Host Map page.
// type HostMapWidgetDefinition struct {
// 	// List of custom links.
// 	CustomLinks []WidgetCustomLink `json:"custom_links,omitempty"`
// 	// List of tag prefixes to group by.
// 	Group []string `json:"group,omitempty"`
// 	// Whether to show the hosts that don’t fit in a group.
// 	NoGroupHosts *bool `json:"no_group_hosts,omitempty"`
// 	// Whether to show the hosts with no metrics.
// 	NoMetricHosts *bool `json:"no_metric_hosts,omitempty"`
// 	// Which type of node to use in the map.
// 	NodeType *datadogV1.WidgetNodeType `json:"node_type,omitempty"`
// 	// Notes on the title.
// 	Notes *string `json:"notes,omitempty"`
// 	// List of definitions.
// 	Requests HostMapWidgetDefinitionRequests `json:"requests"`
// 	// List of tags used to filter the map.
// 	Scope []string `json:"scope,omitempty"`
// 	// The style to apply to the widget.
// 	Style *HostMapWidgetDefinitionStyle `json:"style,omitempty"`
// 	// Title of the widget.
// 	Title *string `json:"title,omitempty"`
// 	// How to align the text on the widget.
// 	TitleAlign *datadogV1.WidgetTextAlign `json:"title_align,omitempty"`
// 	// Size of the title.
// 	TitleSize *string `json:"title_size,omitempty"`
// 	// Type of the host map widget.
// 	Type datadogV1.HostMapWidgetDefinitionType `json:"type"`
// }

// // HostMapWidgetDefinitionRequests List of definitions.
// type HostMapWidgetDefinitionRequests struct {
// 	// Updated host map.
// 	Fill *HostMapRequest `json:"fill,omitempty"`
// 	// Updated host map.
// 	Size *HostMapRequest `json:"size,omitempty"`
// }

// // HostMapRequest Updated host map.
// type HostMapRequest struct {
// 	// The log query.
// 	ApmQuery *LogQueryDefinition `json:"apm_query,omitempty"`
// 	// The log query.
// 	EventQuery *LogQueryDefinition `json:"event_query,omitempty"`
// 	// The log query.
// 	LogQuery *LogQueryDefinition `json:"log_query,omitempty"`
// 	// The log query.
// 	NetworkQuery *LogQueryDefinition `json:"network_query,omitempty"`
// 	// The process query to use in the widget.
// 	ProcessQuery *ProcessQueryDefinition `json:"process_query,omitempty"`
// 	// The log query.
// 	ProfileMetricsQuery *LogQueryDefinition `json:"profile_metrics_query,omitempty"`
// 	// Query definition.
// 	Q *string `json:"q,omitempty"`
// 	// The log query.
// 	RumQuery *LogQueryDefinition `json:"rum_query,omitempty"`
// 	// The log query.
// 	SecurityQuery *LogQueryDefinition `json:"security_query,omitempty"`
// }

// // HostMapWidgetDefinitionStyle The style to apply to the widget.
// type HostMapWidgetDefinitionStyle struct {
// 	// Max value to use to color the map.
// 	FillMax *string `json:"fill_max,omitempty"`
// 	// Min value to use to color the map.
// 	FillMin *string `json:"fill_min,omitempty"`
// 	// Color palette to apply to the widget.
// 	Palette *string `json:"palette,omitempty"`
// 	// Whether to flip the palette tones.
// 	PaletteFlip *bool `json:"palette_flip,omitempty"`
// }

// // IFrameWidgetDefinition The iframe widget allows you to embed a portion of any other web page on your dashboard. Only available on FREE layout dashboards.
// type IFrameWidgetDefinition struct {
// 	// Type of the iframe widget.
// 	Type datadogV1.IFrameWidgetDefinitionType `json:"type"`
// 	// URL of the iframe.
// 	Url string `json:"url"`
// }

// // ImageWidgetDefinition The image widget allows you to embed an image on your dashboard. An image can be a PNG, JPG, or animated GIF. Only available on FREE layout dashboards.
// type ImageWidgetDefinition struct {
// 	// Whether to display a background or not.
// 	HasBackground *bool `json:"has_background,omitempty"`
// 	// Whether to display a border or not.
// 	HasBorder *bool `json:"has_border,omitempty"`
// 	// Horizontal alignment.
// 	HorizontalAlign *datadogV1.WidgetHorizontalAlign `json:"horizontal_align,omitempty"`
// 	// Size of the margins around the image.
// 	// **Note**: `small` and `large` values are deprecated.
// 	Margin *datadogV1.WidgetMargin `json:"margin,omitempty"`
// 	// How to size the image on the widget. The values are based on the image `object-fit` CSS properties.
// 	// **Note**: `zoom`, `fit` and `center` values are deprecated.
// 	Sizing *datadogV1.WidgetImageSizing `json:"sizing,omitempty"`
// 	// Type of the image widget.
// 	Type datadogV1.ImageWidgetDefinitionType `json:"type"`
// 	// URL of the image.
// 	Url string `json:"url"`
// 	// URL of the image in dark mode.
// 	UrlDarkTheme *string `json:"url_dark_theme,omitempty"`
// 	// Vertical alignment.
// 	VerticalAlign *datadogV1.WidgetVerticalAlign `json:"vertical_align,omitempty"`
// }

// WidgetDefinition - [Definition of the widget](https://docs.datadoghq.com/dashboards/widgets/).
// +k8s:openapi-gen=true
type WidgetDefinition struct {
	// AlertGraphWidgetDefinition    *AlertGraphWidgetDefinition
	// AlertValueWidgetDefinition    *AlertValueWidgetDefinition
	// ChangeWidgetDefinition        *ChangeWidgetDefinition
	// CheckStatusWidgetDefinition   *CheckStatusWidgetDefinition
	// DistributionWidgetDefinition  *DistributionWidgetDefinition
	// EventStreamWidgetDefinition   *EventStreamWidgetDefinition
	// EventTimelineWidgetDefinition *EventTimelineWidgetDefinition
	// FreeTextWidgetDefinition      *FreeTextWidgetDefinition
	// FunnelWidgetDefinition        *FunnelWidgetDefinition
	// GeomapWidgetDefinition        *GeomapWidgetDefinition
	// GroupWidgetDefinition         *GroupWidgetDefinition
	// HeatMapWidgetDefinition       *HeatMapWidgetDefinition
	// HostMapWidgetDefinition       *HostMapWidgetDefinition
	// IFrameWidgetDefinition        *IFrameWidgetDefinition
	// ImageWidgetDefinition         *ImageWidgetDefinition
	// ListStreamWidgetDefinition     *ListStreamWidgetDefinition
	// LogStreamWidgetDefinition      *LogStreamWidgetDefinition
	// MonitorSummaryWidgetDefinition *MonitorSummaryWidgetDefinition
	// NoteWidgetDefinition           *NoteWidgetDefinition
	// PowerpackWidgetDefinition      *PowerpackWidgetDefinition
	// QueryValueWidgetDefinition     *QueryValueWidgetDefinition
	// RunWorkflowWidgetDefinition    *RunWorkflowWidgetDefinition
	// SLOListWidgetDefinition        *SLOListWidgetDefinition
	// SLOWidgetDefinition            *SLOWidgetDefinition
	// ScatterPlotWidgetDefinition    *ScatterPlotWidgetDefinition
	// ServiceMapWidgetDefinition     *ServiceMapWidgetDefinition
	// ServiceSummaryWidgetDefinition *ServiceSummaryWidgetDefinition
	// SplitGraphWidgetDefinition     *SplitGraphWidgetDefinition
	// SunburstWidgetDefinition       *SunburstWidgetDefinition
	// TableWidgetDefinition          *TableWidgetDefinition
	// TimeseriesWidgetDefinition     *TimeseriesWidgetDefinition
	// ToplistWidgetDefinition        *ToplistWidgetDefinition
	// TopologyMapWidgetDefinition    *TopologyMapWidgetDefinition
	// TreeMapWidgetDefinition        *TreeMapWidgetDefinition
}

// // AlertGraphWidgetDefinition Alert graphs are timeseries graphs showing the current status of any monitor defined on your system.
// type AlertGraphWidgetDefinition struct {
// 	// ID of the alert to use in the widget.
// 	AlertId string `json:"alert_id"`
// 	// Time setting for the widget.
// 	Time *WidgetTime `json:"time,omitempty"`
// 	// The title of the widget.
// 	Title *string `json:"title,omitempty"`
// 	// How to align the text on the widget.
// 	TitleAlign *datadogV1.WidgetTextAlign `json:"title_align,omitempty"`
// 	// Size of the title.
// 	TitleSize *string `json:"title_size,omitempty"`
// 	// Type of the alert graph widget.
// 	Type datadogV1.AlertGraphWidgetDefinitionType `json:"type"`
// 	// Whether to display the Alert Graph as a timeseries or a top list.
// 	VizType datadogV1.WidgetVizType `json:"viz_type"`
// }

// //

// // AlertValueWidgetDefinition Alert values are query values showing the current value of the metric in any monitor defined on your system.
// type AlertValueWidgetDefinition struct {
// 	// ID of the alert to use in the widget.
// 	AlertId string `json:"alert_id"`
// 	// Number of decimal to show. If not defined, will use the raw value.
// 	Precision *int64 `json:"precision,omitempty"`
// 	// How to align the text on the widget.
// 	TextAlign *datadogV1.WidgetTextAlign `json:"text_align,omitempty"`
// 	// Title of the widget.
// 	Title *string `json:"title,omitempty"`
// 	// NOTE: mistake in struct defintion?
// 	// // How to align the text on the widget.
// 	// TitleAlign *WidgetTextAlign `json:"title_align,omitempty"`
// 	// Size of value in the widget.
// 	TitleSize *string `json:"title_size,omitempty"`
// 	// Type of the alert value widget.
// 	Type datadogV1.AlertValueWidgetDefinitionType `json:"type"`
// 	// Unit to display with the value.
// 	Unit *string `json:"unit,omitempty"`
// }

// // ChangeWidgetDefinition The Change graph shows you the change in a value over the time period chosen.
// type ChangeWidgetDefinition struct {
// 	// List of custom links.
// 	CustomLinks []WidgetCustomLink `json:"custom_links,omitempty"`
// 	// Array of one request object to display in the widget.
// 	//
// 	// See the dedicated [Request JSON schema documentation](https://docs.datadoghq.com/dashboards/graphing_json/request_json)
// 	//  to learn how to build the `REQUEST_SCHEMA`.
// 	Requests []ChangeWidgetRequest `json:"requests"`
// 	// Time setting for the widget.
// 	Time *WidgetTime `json:"time,omitempty"`
// 	// Title of the widget.
// 	Title *string `json:"title,omitempty"`
// 	// How to align the text on the widget.
// 	TitleAlign *datadogV1.WidgetTextAlign `json:"title_align,omitempty"`
// 	// Size of the title.
// 	TitleSize *string `json:"title_size,omitempty"`
// 	// Type of the change widget.
// 	Type datadogV1.ChangeWidgetDefinitionType `json:"type"`
// }

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
