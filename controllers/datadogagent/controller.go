// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	componentagent "github.com/DataDog/datadog-operator/controllers/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	// Use to register features
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/admissioncontroller"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/apm"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/asm"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/clusterchecks"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/cspm"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/cws"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/dogstatsd"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/dummy"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/ebpfcheck"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/enabledefault"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/eventcollection"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/externalmetrics"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/helmcheck"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/kubernetesstatecore"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/livecontainer"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/liveprocess"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/logcollection"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/npm"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/oomkill"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/orchestratorexplorer"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/otlp"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/processdiscovery"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/prometheusscrape"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/remoteconfig"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/sbom"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/tcpqueuelength"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/usm"
)

const (
	defaultRequeuePeriod = 15 * time.Second
)

// ReconcilerOptions provides options read from command line
type ReconcilerOptions struct {
	ExtendedDaemonsetOptions        componentagent.ExtendedDaemonsetOptions
	SupportCilium                   bool
	OperatorMetricsEnabled          bool
	IntrospectionEnabled            bool
	DatadogAgentProfileEnabled      bool
	ProcessChecksInCoreAgentEnabled bool
}

// Reconciler is the internal reconciler for Datadog Agent
type Reconciler struct {
	options      ReconcilerOptions
	client       client.Client
	versionInfo  *version.Info
	platformInfo kubernetes.PlatformInfo
	scheme       *runtime.Scheme
	log          logr.Logger
	recorder     record.EventRecorder
	forwarders   datadog.MetricForwardersManager
}

// NewReconciler returns a reconciler for DatadogAgent
func NewReconciler(options ReconcilerOptions, client client.Client, versionInfo *version.Info, platformInfo kubernetes.PlatformInfo,
	scheme *runtime.Scheme, log logr.Logger, recorder record.EventRecorder, metricForwarder datadog.MetricForwardersManager) (*Reconciler, error) {
	return &Reconciler{
		options:      options,
		client:       client,
		versionInfo:  versionInfo,
		platformInfo: platformInfo,
		scheme:       scheme,
		log:          log,
		recorder:     recorder,
		forwarders:   metricForwarder,
	}, nil
}

// Reconcile is similar to reconciler.Reconcile interface, but taking a context
func (r *Reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	var resp reconcile.Result
	var err error

	resp, err = r.internalReconcileV2(ctx, request)

	r.metricsForwarderProcessError(request, err)
	return resp, err
}

func reconcilerOptionsToFeatureOptions(opts *ReconcilerOptions, logger logr.Logger) *feature.Options {
	return &feature.Options{
		SupportExtendedDaemonset:        opts.ExtendedDaemonsetOptions.Enabled,
		Logger:                          logger,
		ProcessChecksInCoreAgentEnabled: opts.ProcessChecksInCoreAgentEnabled,
	}
}

// metricsForwarderProcessError convert the reconciler errors into metrics if metrics forwarder is enabled
func (r *Reconciler) metricsForwarderProcessError(req reconcile.Request, err error) {
	if r.options.OperatorMetricsEnabled {
		r.forwarders.ProcessError(getMonitoredObj(req), err)
	}
}
