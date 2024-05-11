package metrics

import (
	"github.com/DataDog/extendeddaemonset/pkg/controller/utils"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	ersRollingUpdateStuck = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "ers_rolling_update_stuck",
			Help: "1 if the number of unavailable pods is higher than maxUnavailable, 0 otherwise",
		},
		[]string{
			utils.ResourceNamePromLabel,
			utils.ResourceNamespacePromLabel,
		},
	)

	ersMetrics = []*prometheus.GaugeVec{
		ersRollingUpdateStuck,
	}
)

func promLabels(name, namespace string) prometheus.Labels {
	return prometheus.Labels{
		utils.ResourceNamePromLabel:      name,
		utils.ResourceNamespacePromLabel: namespace,
	}
}

func boolToFloat64(b bool) float64 {
	if b {
		return 1.0
	}

	return 0.0
}

// SetRollingUpdateStuckMetric updates the ers_rolling_update_stuck metric value
// To be called by the ERS rolling update logic.
func SetRollingUpdateStuckMetric(ersName, ersNamespace string, isStuck bool) {
	ersRollingUpdateStuck.With(promLabels(ersName, ersNamespace)).Set(boolToFloat64(isStuck))
}

// DeleteERSMetrics removes metrics related to a specific ERS
// To be called by the EDS controller when removing ERS objects.
func DeleteERSMetrics(ersName, ersNamespace string) {
	for _, metric := range ersMetrics {
		metric.Delete(promLabels(ersName, ersNamespace))
	}
}

func init() {
	// Register custom metrics with the global prometheus registry
	for _, metric := range ersMetrics {
		metrics.Registry.MustRegister(metric)
	}
}
