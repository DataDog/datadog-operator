// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package extendeddaemonset

import (
	ksmetric "k8s.io/kube-state-metrics/pkg/metric"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/DataDog/extendeddaemonset/controllers/extendeddaemonset/conditions"
	"github.com/DataDog/extendeddaemonset/pkg/controller/metrics"
	"github.com/DataDog/extendeddaemonset/pkg/controller/utils"
)

const (
	extendeddaemonsetCreated                        = "eds_created"
	extendeddaemonsetStatusDesired                  = "eds_status_desired"
	extendeddaemonsetStatusCurrent                  = "eds_status_current"
	extendeddaemonsetStatusReady                    = "eds_status_ready"
	extendeddaemonsetStatusAvailable                = "eds_status_available"
	extendeddaemonsetStatusUpToDate                 = "eds_status_uptodate"
	extendeddaemonsetStatusIgnoredUnresponsiveNodes = "eds_status_ignored_unresponsive_nodes"
	extendeddaemonsetStatusCanaryActivated          = "eds_status_canary_activated"
	extendeddaemonsetStatusCanaryNumberOfNodes      = "eds_status_canary_node_number"
	extendeddaemonsetStatusCanaryPaused             = "eds_status_canary_paused"
	extendeddaemonsetStatusRollingUpdatePaused      = "eds_status_rolling_update_paused"
	extendeddaemonsetStatusRolloutFrozen            = "eds_status_rollout_frozen"
	extendeddaemonsetLabels                         = "eds_labels"
)

func init() {
	metrics.RegisterHandlerFunc(addMetrics)
}

func addMetrics(mgr manager.Manager, h metrics.Handler) error {
	return metrics.AddMetrics(datadoghqv1alpha1.GroupVersion.WithKind("ExtendedDaemonSet"), mgr, h, generateMetricFamilies())
}

func generateMetricFamilies() []ksmetric.FamilyGenerator {
	return []ksmetric.FamilyGenerator{
		{
			Name: extendeddaemonsetLabels,
			Type: ksmetric.Gauge,
			Help: "Kubernetes labels converted to Prometheus labels",
			GenerateFunc: func(obj interface{}) *ksmetric.Family {
				eds := obj.(*datadoghqv1alpha1.ExtendedDaemonSet)

				labelKeys, labelValues := utils.GetLabelsValues(&eds.ObjectMeta)
				extraKeys, extraValues := utils.BuildInfoLabels(&eds.ObjectMeta)

				return &ksmetric.Family{
					Metrics: []*ksmetric.Metric{
						{
							Value:       1,
							LabelKeys:   append(labelKeys, extraKeys...),
							LabelValues: append(labelValues, extraValues...),
						},
					},
				}
			},
		},
		{
			Name: extendeddaemonsetCreated,
			Type: ksmetric.Gauge,
			Help: "Unix creation timestamp",
			GenerateFunc: func(obj interface{}) *ksmetric.Family {
				eds := obj.(*datadoghqv1alpha1.ExtendedDaemonSet)

				labelKeys, labelValues := utils.GetLabelsValues(&eds.ObjectMeta)

				return &ksmetric.Family{
					Metrics: []*ksmetric.Metric{
						{
							Value:       float64(eds.CreationTimestamp.Unix()),
							LabelKeys:   labelKeys,
							LabelValues: labelValues,
						},
					},
				}
			},
		},
		{
			Name: extendeddaemonsetStatusDesired,
			Type: ksmetric.Gauge,
			Help: "The number of nodes that should be running the daemon pod.",
			GenerateFunc: func(obj interface{}) *ksmetric.Family {
				eds := obj.(*datadoghqv1alpha1.ExtendedDaemonSet)
				labelKeys, labelValues := utils.GetLabelsValues(&eds.ObjectMeta)

				return &ksmetric.Family{
					Metrics: []*ksmetric.Metric{
						{
							Value:       float64(eds.Status.Desired),
							LabelKeys:   labelKeys,
							LabelValues: labelValues,
						},
					},
				}
			},
		},
		{
			Name: extendeddaemonsetStatusCurrent,
			Type: ksmetric.Gauge,
			Help: "The number of nodes running at least one daemon pod and are supposed to.",
			GenerateFunc: func(obj interface{}) *ksmetric.Family {
				eds := obj.(*datadoghqv1alpha1.ExtendedDaemonSet)
				labelKeys, labelValues := utils.GetLabelsValues(&eds.ObjectMeta)

				return &ksmetric.Family{
					Metrics: []*ksmetric.Metric{
						{
							Value:       float64(eds.Status.Current),
							LabelKeys:   labelKeys,
							LabelValues: labelValues,
						},
					},
				}
			},
		},
		{
			Name: extendeddaemonsetStatusReady,
			Type: ksmetric.Gauge,
			Help: "The number of nodes that should be running the daemon pod and have one or more of the daemon pod running and ready.",
			GenerateFunc: func(obj interface{}) *ksmetric.Family {
				eds := obj.(*datadoghqv1alpha1.ExtendedDaemonSet)
				labelKeys, labelValues := utils.GetLabelsValues(&eds.ObjectMeta)

				return &ksmetric.Family{
					Metrics: []*ksmetric.Metric{
						{
							Value:       float64(eds.Status.Ready),
							LabelKeys:   labelKeys,
							LabelValues: labelValues,
						},
					},
				}
			},
		},
		{
			Name: extendeddaemonsetStatusAvailable,
			Type: ksmetric.Gauge,
			Help: "The number of nodes that should be running the daemon pod and have one or more of the daemon pod running and available.",
			GenerateFunc: func(obj interface{}) *ksmetric.Family {
				eds := obj.(*datadoghqv1alpha1.ExtendedDaemonSet)
				labelKeys, labelValues := utils.GetLabelsValues(&eds.ObjectMeta)

				return &ksmetric.Family{
					Metrics: []*ksmetric.Metric{
						{
							Value:       float64(eds.Status.Available),
							LabelKeys:   labelKeys,
							LabelValues: labelValues,
						},
					},
				}
			},
		},
		{
			Name: extendeddaemonsetStatusUpToDate,
			Type: ksmetric.Gauge,
			Help: "The total number of nodes that are running updated daemon pod",
			GenerateFunc: func(obj interface{}) *ksmetric.Family {
				eds := obj.(*datadoghqv1alpha1.ExtendedDaemonSet)
				labelKeys, labelValues := utils.GetLabelsValues(&eds.ObjectMeta)

				return &ksmetric.Family{
					Metrics: []*ksmetric.Metric{
						{
							Value:       float64(eds.Status.UpToDate),
							LabelKeys:   labelKeys,
							LabelValues: labelValues,
						},
					},
				}
			},
		},
		{
			Name: extendeddaemonsetStatusIgnoredUnresponsiveNodes,
			Type: ksmetric.Gauge,
			Help: "The total number of nodes that are ignored by the rolling update strategy due to an unresponsive state",
			GenerateFunc: func(obj interface{}) *ksmetric.Family {
				eds := obj.(*datadoghqv1alpha1.ExtendedDaemonSet)
				labelKeys, labelValues := utils.GetLabelsValues(&eds.ObjectMeta)

				return &ksmetric.Family{
					Metrics: []*ksmetric.Metric{
						{
							Value:       float64(eds.Status.IgnoredUnresponsiveNodes),
							LabelKeys:   labelKeys,
							LabelValues: labelValues,
						},
					},
				}
			},
		},
		{
			Name: extendeddaemonsetStatusCanaryActivated,
			Type: ksmetric.Gauge,
			Help: "The status of the canary deployment, set to 1 if active, else 0",
			GenerateFunc: func(obj interface{}) *ksmetric.Family {
				eds := obj.(*datadoghqv1alpha1.ExtendedDaemonSet)
				labelKeys, labelValues := utils.GetLabelsValues(&eds.ObjectMeta)
				val := float64(0)
				rs := ""
				if eds.Status.Canary != nil {
					val = 1
					rs = eds.Status.Canary.ReplicaSet
				}
				Labelkeys := append(labelKeys, "replicaset")
				Labelvalues := append(labelValues, rs)

				return &ksmetric.Family{
					Metrics: []*ksmetric.Metric{
						{
							Value:       val,
							LabelKeys:   Labelkeys,
							LabelValues: Labelvalues,
						},
					},
				}
			},
		},
		{
			Name: extendeddaemonsetStatusCanaryPaused,
			Type: ksmetric.Gauge,
			Help: "The paused state of the canary deployment, set to 1 if paused, else 0",
			GenerateFunc: func(obj interface{}) *ksmetric.Family {
				eds := obj.(*datadoghqv1alpha1.ExtendedDaemonSet)
				labelKeys, labelValues := utils.GetLabelsValues(&eds.ObjectMeta)
				val := float64(0)

				if eds.Status.Canary != nil {
					rs := eds.Status.Canary.ReplicaSet
					labelKeys = append(labelKeys, "replicaset")
					labelValues = append(labelValues, rs)
					if conditions.IsConditionTrue(&eds.Status, datadoghqv1alpha1.ConditionTypeEDSCanaryPaused) {
						cond := conditions.GetExtendedDaemonSetStatusCondition(&eds.Status, datadoghqv1alpha1.ConditionTypeEDSCanaryPaused)

						val = 1
						labelKeys = append(labelKeys, "paused_reason")
						labelValues = append(labelValues, cond.Reason)
					}
				}

				return &ksmetric.Family{
					Metrics: []*ksmetric.Metric{
						{
							Value:       val,
							LabelKeys:   labelKeys,
							LabelValues: labelValues,
						},
					},
				}
			},
		},
		{
			Name: extendeddaemonsetStatusCanaryNumberOfNodes,
			Type: ksmetric.Gauge,
			Help: "The total number of nodes that are running a canary pod",
			GenerateFunc: func(obj interface{}) *ksmetric.Family {
				eds := obj.(*datadoghqv1alpha1.ExtendedDaemonSet)
				labelKeys, labelValues := utils.GetLabelsValues(&eds.ObjectMeta)
				val := float64(0)
				if eds.Status.Canary != nil {
					val = float64(len(eds.Status.Canary.Nodes))
				}

				return &ksmetric.Family{
					Metrics: []*ksmetric.Metric{
						{
							Value:       val,
							LabelKeys:   labelKeys,
							LabelValues: labelValues,
						},
					},
				}
			},
		},
		{
			Name: extendeddaemonsetStatusRollingUpdatePaused,
			Type: ksmetric.Gauge,
			Help: "The paused state of a rolling update, 1 if paused, 0 otherwise",
			GenerateFunc: func(obj interface{}) *ksmetric.Family {
				eds := obj.(*datadoghqv1alpha1.ExtendedDaemonSet)
				labelKeys, labelValues := utils.GetLabelsValues(&eds.ObjectMeta)
				val := float64(0)

				if eds.Status.State == datadoghqv1alpha1.ExtendedDaemonSetStatusStateRollingUpdatePaused {
					val = 1
				}

				return &ksmetric.Family{
					Metrics: []*ksmetric.Metric{
						{
							Value:       val,
							LabelKeys:   labelKeys,
							LabelValues: labelValues,
						},
					},
				}
			},
		},
		{
			Name: extendeddaemonsetStatusRolloutFrozen,
			Type: ksmetric.Gauge,
			Help: "The frozen state of a rollout, 1 if frozen, 0 otherwise",
			GenerateFunc: func(obj interface{}) *ksmetric.Family {
				eds := obj.(*datadoghqv1alpha1.ExtendedDaemonSet)
				labelKeys, labelValues := utils.GetLabelsValues(&eds.ObjectMeta)
				val := float64(0)

				if eds.Status.State == datadoghqv1alpha1.ExtendedDaemonSetStatusStateRolloutFrozen {
					val = 1
				}

				return &ksmetric.Family{
					Metrics: []*ksmetric.Metric{
						{
							Value:       val,
							LabelKeys:   labelKeys,
							LabelValues: labelValues,
						},
					},
				}
			},
		},
	}
}
