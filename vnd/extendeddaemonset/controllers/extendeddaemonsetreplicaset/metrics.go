// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package extendeddaemonsetreplicaset

import (
	ksmetric "k8s.io/kube-state-metrics/pkg/metric"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	"github.com/DataDog/extendeddaemonset/controllers/extendeddaemonsetreplicaset/conditions"
	"github.com/DataDog/extendeddaemonset/pkg/controller/metrics"
	"github.com/DataDog/extendeddaemonset/pkg/controller/utils"
)

const (
	ersCreated                        = "ers_created"
	ersStatusDesired                  = "ers_status_desired"
	ersStatusCurrent                  = "ers_status_current"
	ersStatusReady                    = "ers_status_ready"
	ersStatusAvailable                = "ers_status_available"
	ersStatusIgnoredUnresponsiveNodes = "ers_status_ignored_unresponsive_nodes"
	ersStatusCanaryFailed             = "ers_status_canary_failed"
	ersLabels                         = "ers_labels"
)

func init() {
	metrics.RegisterHandlerFunc(addMetrics)
}

func addMetrics(mgr manager.Manager, h metrics.Handler) error {
	return metrics.AddMetrics(datadoghqv1alpha1.GroupVersion.WithKind("ExtendedDaemonSetReplicaSet"), mgr, h, generateMetricFamilies())
}

func generateMetricFamilies() []ksmetric.FamilyGenerator {
	return []ksmetric.FamilyGenerator{
		{
			Name: ersLabels,
			Type: ksmetric.Gauge,
			Help: "Kubernetes labels converted to Prometheus labels",
			GenerateFunc: func(obj interface{}) *ksmetric.Family {
				ers := obj.(*datadoghqv1alpha1.ExtendedDaemonSetReplicaSet)
				labelKeys, labelValues := utils.GetLabelsValues(&ers.ObjectMeta)
				extraKeys, extraValues := utils.BuildInfoLabels(&ers.ObjectMeta)

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
			Name: ersCreated,
			Type: ksmetric.Gauge,
			Help: "Unix creation timestamp",
			GenerateFunc: func(obj interface{}) *ksmetric.Family {
				ers := obj.(*datadoghqv1alpha1.ExtendedDaemonSetReplicaSet)
				labelKeys, labelValues := utils.GetLabelsValues(&ers.ObjectMeta)

				return &ksmetric.Family{
					Metrics: []*ksmetric.Metric{
						{
							Value:       float64(ers.CreationTimestamp.Unix()),
							LabelKeys:   labelKeys,
							LabelValues: labelValues,
						},
					},
				}
			},
		},
		{
			Name: ersStatusDesired,
			Type: ksmetric.Gauge,
			Help: "The number of nodes that should be running the daemon pod.",
			GenerateFunc: func(obj interface{}) *ksmetric.Family {
				ers := obj.(*datadoghqv1alpha1.ExtendedDaemonSetReplicaSet)
				labelKeys, labelValues := utils.GetLabelsValues(&ers.ObjectMeta)

				return &ksmetric.Family{
					Metrics: []*ksmetric.Metric{
						{
							Value:       float64(ers.Status.Desired),
							LabelKeys:   labelKeys,
							LabelValues: labelValues,
						},
					},
				}
			},
		},
		{
			Name: ersStatusCurrent,
			Type: ksmetric.Gauge,
			Help: "The number of nodes running at least one daemon pod and are supposed to.",
			GenerateFunc: func(obj interface{}) *ksmetric.Family {
				ers := obj.(*datadoghqv1alpha1.ExtendedDaemonSetReplicaSet)
				labelKeys, labelValues := utils.GetLabelsValues(&ers.ObjectMeta)

				return &ksmetric.Family{
					Metrics: []*ksmetric.Metric{
						{
							Value:       float64(ers.Status.Current),
							LabelKeys:   labelKeys,
							LabelValues: labelValues,
						},
					},
				}
			},
		},
		{
			Name: ersStatusReady,
			Type: ksmetric.Gauge,
			Help: "The number of nodes that should be running the daemon pod and have one or more of the daemon pod running and ready.",
			GenerateFunc: func(obj interface{}) *ksmetric.Family {
				ers := obj.(*datadoghqv1alpha1.ExtendedDaemonSetReplicaSet)
				labelKeys, labelValues := utils.GetLabelsValues(&ers.ObjectMeta)

				return &ksmetric.Family{
					Metrics: []*ksmetric.Metric{
						{
							Value:       float64(ers.Status.Ready),
							LabelKeys:   labelKeys,
							LabelValues: labelValues,
						},
					},
				}
			},
		},
		{
			Name: ersStatusAvailable,
			Type: ksmetric.Gauge,
			Help: "The number of nodes that should be running the daemon pod and have one or more of the daemon pod running and available.",
			GenerateFunc: func(obj interface{}) *ksmetric.Family {
				ers := obj.(*datadoghqv1alpha1.ExtendedDaemonSetReplicaSet)
				labelKeys, labelValues := utils.GetLabelsValues(&ers.ObjectMeta)

				return &ksmetric.Family{
					Metrics: []*ksmetric.Metric{
						{
							Value:       float64(ers.Status.Available),
							LabelKeys:   labelKeys,
							LabelValues: labelValues,
						},
					},
				}
			},
		},
		{
			Name: ersStatusIgnoredUnresponsiveNodes,
			Type: ksmetric.Gauge,
			Help: "The total number of nodes that are ignored by the rolling update strategy due to an unresponsive state",
			GenerateFunc: func(obj interface{}) *ksmetric.Family {
				ers := obj.(*datadoghqv1alpha1.ExtendedDaemonSetReplicaSet)
				labelKeys, labelValues := utils.GetLabelsValues(&ers.ObjectMeta)

				return &ksmetric.Family{
					Metrics: []*ksmetric.Metric{
						{
							Value:       float64(ers.Status.IgnoredUnresponsiveNodes),
							LabelKeys:   labelKeys,
							LabelValues: labelValues,
						},
					},
				}
			},
		},
		{
			Name: ersStatusCanaryFailed,
			Type: ksmetric.Gauge,
			Help: "The failed state of the canary deployment, set to 1 if failed, else 0",
			GenerateFunc: func(obj interface{}) *ksmetric.Family {
				ers := obj.(*datadoghqv1alpha1.ExtendedDaemonSetReplicaSet)
				labelKeys, labelValues := utils.GetLabelsValues(&ers.ObjectMeta)

				val := float64(0)
				if conditions.IsConditionTrue(&ers.Status, datadoghqv1alpha1.ConditionTypeCanaryFailed) {
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
