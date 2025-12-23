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
	"k8s.io/apimachinery/pkg/util/managedfields"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	// Use to register features
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/admissioncontroller"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/apm"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/appsec"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/asm"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/autoscaling"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/clusterchecks"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/controlplanemonitoring"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/cspm"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/cws"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/dogstatsd"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/dummy"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/ebpfcheck"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/enabledefault"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/eventcollection"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/externalmetrics"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/gpu"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/helmcheck"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/kubernetesstatecore"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/livecontainer"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/liveprocess"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/logcollection"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/npm"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/oomkill"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/orchestratorexplorer"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/otelagentgateway"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/otelcollector"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/otlp"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/processdiscovery"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/prometheusscrape"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/remoteconfig"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/sbom"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/servicediscovery"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/tcpqueuelength"
	_ "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/usm"
)

const (
	defaultRequeuePeriod = 15 * time.Second
)

// ReconcilerOptions provides options read from command line
type ReconcilerOptions struct {
	ExtendedDaemonsetOptions    componentagent.ExtendedDaemonsetOptions
	SupportCilium               bool
	OperatorMetricsEnabled      bool
	IntrospectionEnabled        bool
	DatadogAgentProfileEnabled  bool
	DatadogAgentInternalEnabled bool
}

// Reconciler is the internal reconciler for Datadog Agent
type Reconciler struct {
	options           ReconcilerOptions
	client            client.Client
	platformInfo      kubernetes.PlatformInfo
	scheme            *runtime.Scheme
	log               logr.Logger
	recorder          record.EventRecorder
	forwarders        datadog.MetricsForwardersManager
	fieldManager      *managedfields.FieldManager
	componentRegistry *ComponentRegistry
}

func (r *Reconciler) initializeComponentRegistry() {
	r.componentRegistry = NewComponentRegistry(r)
	// Register all components
	r.componentRegistry.Register(NewClusterAgentComponent(r))
	r.componentRegistry.Register(NewClusterChecksRunnerComponent(r))
}

// NewReconciler returns a reconciler for DatadogAgent
func NewReconciler(options ReconcilerOptions, client client.Client, platformInfo kubernetes.PlatformInfo,
	scheme *runtime.Scheme, log logr.Logger, recorder record.EventRecorder, metricForwardersMgr datadog.MetricsForwardersManager,
) (*Reconciler, error) {
	r := &Reconciler{
		options:      options,
		client:       client,
		platformInfo: platformInfo,
		scheme:       scheme,
		log:          log,
		recorder:     recorder,
		forwarders:   metricForwardersMgr,
	}

	// Initialize component registry
	r.initializeComponentRegistry()

	return r, nil
}

// Reconcile is similar to reconciler.Reconcile interface, but taking a context
func (r *Reconciler) Reconcile(ctx context.Context, dda *v2alpha1.DatadogAgent) (reconcile.Result, error) {
	var resp reconcile.Result
	var err error

	resp, err = r.internalReconcileV2(ctx, dda)

	r.metricsForwarderProcessError(dda, err)
	return resp, err
}

func reconcilerOptionsToFeatureOptions(opts *ReconcilerOptions, logger logr.Logger) *feature.Options {
	return &feature.Options{
		Logger: logger,
	}
}

// metricsForwarderProcessError convert the reconciler errors into metrics if metrics forwarder is enabled
func (r *Reconciler) metricsForwarderProcessError(dda *v2alpha1.DatadogAgent, err error) {
	if r.options.OperatorMetricsEnabled {
		r.forwarders.ProcessError(dda, err)
	}
}
