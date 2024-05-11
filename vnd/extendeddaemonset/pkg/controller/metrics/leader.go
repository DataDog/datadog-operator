package metrics

import (
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	leaderLabel = "leader"
)

var (
	isLeader  = false
	edsLeader = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "eds_controller_leader",
			Help: "Whether EDS instance is currently leader not",
		}, []string{
			leaderLabel,
		},
	)
)

// SetLeader sets the edsLeader gauge.
func SetLeader(leaderValue bool) {
	if isLeader != leaderValue {
		edsLeader.Delete(prometheus.Labels{leaderLabel: strconv.FormatBool(isLeader)})
	}

	isLeader = leaderValue
	edsLeader.With(prometheus.Labels{leaderLabel: strconv.FormatBool(isLeader)}).Set(1)
}

func init() {
	SetLeader(false)
	// Register custom metrics with the global prometheus registry
	metrics.Registry.MustRegister(edsLeader)
	// Register custom metrics with KSMetrics registry
	ksmExtraMetricsRegistry.MustRegister(edsLeader)
}
