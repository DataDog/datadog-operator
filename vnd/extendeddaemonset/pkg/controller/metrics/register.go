// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	"k8s.io/client-go/tools/cache"
	ksmetric "k8s.io/kube-state-metrics/pkg/metric"
	metricsstore "k8s.io/kube-state-metrics/pkg/metrics_store"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	metricsHandler          []func(manager.Manager, Handler) error
	ksmExtraMetricsRegistry = prometheus.NewRegistry()
	log                     = ctrl.Log.WithName("ksmetrics")
)

// RegisterHandlerFunc register a function to be added to endpoint when its registered.
func RegisterHandlerFunc(h func(manager.Manager, Handler) error) {
	metricsHandler = append(metricsHandler, h)
}

// RegisterEndpoint add custom metrics endpoint to existing HTTP Listener.
func RegisterEndpoint(mgr ctrl.Manager, register func(string, http.Handler) error) error {
	handler := &storesHandler{}
	for _, metricsHandler := range metricsHandler {
		if err := metricsHandler(mgr, handler); err != nil {
			return err
		}
	}

	return register("/ksmetrics", http.HandlerFunc(handler.serveKsmHTTP))
}

func (h *storesHandler) serveKsmHTTP(w http.ResponseWriter, r *http.Request) {
	resHeader := w.Header()
	// 0.0.4 is the exposition format version of prometheus
	// https://prometheus.io/docs/instrumenting/exposition_formats/#text-based-format
	resHeader.Set("Content-Type", `text/plain; version=`+"0.0.4")

	// Write KSM families
	for _, store := range h.stores {
		store.WriteAll(w)
	}

	// Write extra metrics
	metrics, err := ksmExtraMetricsRegistry.Gather()
	if err == nil {
		for _, m := range metrics {
			_, err = expfmt.MetricFamilyToText(w, m)
			if err != nil {
				log.Error(err, "Unable to write metrics", "metricFamily", m.GetName())
			}
		}
	} else {
		log.Error(err, "Unable to export extra metrics")
	}
}

func (h *storesHandler) RegisterStore(generators []ksmetric.FamilyGenerator, expectedType interface{}, lw cache.ListerWatcher) error {
	store := newMetricsStore(generators, expectedType, lw)
	h.stores = append(h.stores, store)

	return nil
}

type storesHandler struct {
	stores []*metricsstore.MetricsStore
}
