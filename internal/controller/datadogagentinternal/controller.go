// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	// Use to register features
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/admissioncontroller"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/apm"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/asm"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/autoscaling"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/clusterchecks"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/cspm"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/cws"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/dogstatsd"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/dummy"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/ebpfcheck"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/enabledefault"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/eventcollection"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/externalmetrics"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/gpu"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/helmcheck"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/kubernetesstatecore"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/livecontainer"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/liveprocess"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/logcollection"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/npm"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/oomkill"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/orchestratorexplorer"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/otelcollector"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/otlp"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/processdiscovery"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/prometheusscrape"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/remoteconfig"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/sbom"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/servicediscovery"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/tcpqueuelength"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/usm"
)

const (
	defaultRequeuePeriod = 15 * time.Second
)

// ReconcilerOptions provides options read from command line
type ReconcilerOptions struct {
	ExtendedDaemonsetOptions componentagent.ExtendedDaemonsetOptions
	SupportCilium            bool
	OperatorMetricsEnabled   bool
}

// Reconciler is the internal reconciler for Datadog Agent
type Reconciler struct {
	options      ReconcilerOptions
	client       client.Client
	platformInfo kubernetes.PlatformInfo
	scheme       *runtime.Scheme
	log          logr.Logger
	recorder     record.EventRecorder
	forwarders   datadog.MetricForwardersManager
}

// NewReconciler returns a reconciler for DatadogAgent
func NewReconciler(options ReconcilerOptions, client client.Client, platformInfo kubernetes.PlatformInfo,
	scheme *runtime.Scheme, log logr.Logger, recorder record.EventRecorder, metricForwardersMgr datadog.MetricForwardersManager,
) (*Reconciler, error) {
	return &Reconciler{
		options:      options,
		client:       client,
		platformInfo: platformInfo,
		scheme:       scheme,
		log:          log,
		recorder:     recorder,
		forwarders:   metricForwardersMgr,
	}, nil
}

// Reconcile is similar to reconciler.Reconcile interface, but taking a context
func (r *Reconciler) Reconcile(ctx context.Context, ddai *v1alpha1.DatadogAgentInternal) (reconcile.Result, error) {
	var resp reconcile.Result
	var err error

	resp, err = r.internalReconcileV2(ctx, ddai)

	r.metricsForwarderProcessError(ddai, err)
	return resp, err
}

func reconcilerOptionsToFeatureOptions(opts *ReconcilerOptions, logger logr.Logger) *feature.Options {
	return &feature.Options{
		Logger: logger,
	}
}

// metricsForwarderProcessError convert the reconciler errors into metrics if metrics forwarder is enabled
func (r *Reconciler) metricsForwarderProcessError(ddai *v1alpha1.DatadogAgentInternal, err error) {
	if r.options.OperatorMetricsEnabled {
		r.forwarders.ProcessError(ddai, err)
	}
}
