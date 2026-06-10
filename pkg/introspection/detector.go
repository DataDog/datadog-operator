// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package introspection detects the cluster-level provider (e.g. "eks",
// "openshift-rhcos") on the elected leader and publishes it for lock-free reads
// by the DatadogAgent reconciler.
package introspection

import (
	"context"
	"slices"
	"sync/atomic"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

const (
	// initialBackoff is the first retry delay while the initial detection has
	// not yet succeeded.
	initialBackoff = 1 * time.Second
	// maxBackoff caps the retry delay for the initial detection.
	maxBackoff = 30 * time.Second
	// refreshInterval is how often the provider is re-detected after the first
	// success. The cluster provider is install-time stable; this only self-heals a
	// degraded first read and picks up the rare relabel.
	refreshInterval = 1 * time.Hour
)

// Detection sources.
const (
	sourcePlatform = "platform-api"
	sourceOwnNode  = "operator-node"
	sourceNodeList = "cluster-node-list"
)

// gkeAutopilotAllowlistKinds are the GKE-managed allowlist CRD kinds (group
// auto.gke.io) present only on GKE Autopilot clusters. GKE exposes the v1
// (AllowlistedWorkload) or v2 (AllowlistedV2Workload) kind depending on version;
// the presence of any of them indicates Autopilot. The detector owns this mapping —
// PlatformInfo only reports raw resource support, it does not know what Autopilot is.
var gkeAutopilotAllowlistKinds = []string{"AllowlistedV2Workload", "AllowlistedWorkload"}

// detection is a completed provider-detection result. A nil *detection (nothing
// published) means detection has not run yet and must not be read as "no
// provider"; a non-nil one may still carry "" or "default".
type detection struct {
	Provider   string
	Source     string
	DetectedAt time.Time
}

// Detector is the cluster environment-detection facade: it resolves a single
// cluster provider for lock-free reads.
type Detector struct {
	platformInfo kubernetes.PlatformInfo // platform-API classification
	apiReader    client.Reader           // uncached; operator-node read
	nodeClient   client.Client           // cached node lister; nil when node cache disabled
	nodeName     string                  // operator's own node (from DD_HOSTNAME); may be empty
	logger       logr.Logger

	current   atomic.Pointer[detection] // nil until first successful detection
	startedAt atomic.Int64              // unix-nanos of leader-start; 0 until Start runs
}

// Compile-time checks that Detector is a leader-only manager Runnable.
var (
	_ manager.Runnable               = &Detector{}
	_ manager.LeaderElectionRunnable = &Detector{}
)

// NewDetector builds a provider Detector from the manager. platformInfo supplies
// the Stage-0 platform-API classification. nodeName is the operator's own node
// (DD_HOSTNAME); empty skips the Stage-1 read. The Stage-2 cluster-node-list
// fallback is wired only when nodeCacheEnabled is true.
func NewDetector(mgr manager.Manager, platformInfo kubernetes.PlatformInfo, nodeName string, logger logr.Logger, nodeCacheEnabled bool) *Detector {
	d := &Detector{
		platformInfo: platformInfo,
		apiReader:    mgr.GetAPIReader(),
		nodeName:     nodeName,
		logger:       logger.WithName("provider-detector"),
	}
	if nodeCacheEnabled {
		d.nodeClient = mgr.GetClient()
	}
	return d
}

// NeedLeaderElection implements manager.LeaderElectionRunnable: detection runs
// only on the elected leader.
func (d *Detector) NeedLeaderElection() bool { return true }

// Start implements manager.Runnable. It records the leader-start time, retries
// the initial detection with bounded backoff until it succeeds, then re-detects
// periodically. It returns when ctx is cancelled (leadership lost or shutdown).
func (d *Detector) Start(ctx context.Context) error {
	d.logger = ctrl.LoggerFrom(ctx).WithName("provider-detector")
	d.startedAt.Store(time.Now().UnixNano())
	d.logger.Info("Starting cluster provider detector", "node", d.nodeName, "refresh", refreshInterval)

	// Initial detection with bounded retry/backoff.
	backoff := initialBackoff
	for d.current.Load() == nil {
		if det := d.detect(ctx); det != nil {
			d.publish(det)
			break
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
			if backoff < maxBackoff {
				if backoff *= 2; backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
		}
	}

	// Periodic re-detection; see refreshInterval.
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			d.logger.Info("Stopping cluster provider detector")
			return nil
		case <-ticker.C:
			if det := d.detect(ctx); det != nil {
				d.publish(det)
			}
		}
	}
}

// Provider returns the detected provider and whether detection has completed.
// It performs a single atomic load and is safe for the reconcile hot path.
func (d *Detector) Provider() (string, bool) {
	det := d.current.Load()
	if det == nil {
		return "", false
	}
	return det.Provider, true
}

// InGracePeriod reports whether the detector is still within its startup window
// on this leadership term, during which an absent signal should not be treated
// as final. A detector that has not started yet is always in the grace period.
func (d *Detector) InGracePeriod(window time.Duration) bool {
	ns := d.startedAt.Load()
	if ns == 0 {
		return true
	}
	return time.Since(time.Unix(0, ns)) < window
}

// StartedAt returns the leader-start time and whether Start has run this term.
func (d *Detector) StartedAt() (time.Time, bool) {
	ns := d.startedAt.Load()
	if ns == 0 {
		return time.Time{}, false
	}
	return time.Unix(0, ns), true
}

// detect resolves the cluster provider from the most specific available source,
// returning the first success or nil if all sources fail this round (leaving any
// prior result intact). In order:
//
//	Stage 0 — platform API: most specific and always ready (e.g. GKE Autopilot,
//	          detected from the CRD surface rather than node labels).
//	Stage 1 — operator's own node labels: authoritative on success, even when it
//	          resolves to the default provider.
//	Stage 2 — node-list fallback: used only when Stage 1 can't run or errors and
//	          the node cache is available.
func (d *Detector) detect(ctx context.Context) *detection {
	// Stage 0: platform API — a GKE allowlist CRD marks an Autopilot cluster.
	if d.isGKEAutopilot() {
		return &detection{
			Provider:   kubernetes.GKEAutopilotProvider,
			Source:     sourcePlatform,
			DetectedAt: time.Now(),
		}
	}

	// Stage 1: operator-node read.
	if d.nodeName != "" {
		node := &corev1.Node{}
		if err := d.apiReader.Get(ctx, types.NamespacedName{Name: d.nodeName}, node); err != nil {
			d.logger.V(1).Info("operator-node provider read failed; trying cluster-node-list fallback", "node", d.nodeName, "error", err)
		} else {
			return &detection{
				Provider:   kubernetes.ClusterProviderFromNodeLabels(node.Labels),
				Source:     sourceOwnNode,
				DetectedAt: time.Now(),
			}
		}
	}

	// Stage 2: node-list fallback.
	if d.nodeClient != nil {
		nodeList := &corev1.NodeList{}
		if err := d.nodeClient.List(ctx, nodeList); err != nil {
			d.logger.V(1).Info("cluster-node-list provider detection failed", "error", err)
		} else {
			return &detection{
				Provider:   kubernetes.GetClusterProviderFromNodeList(nodeList.Items),
				Source:     sourceNodeList,
				DetectedAt: time.Now(),
			}
		}
	}
	return nil
}

// isGKEAutopilot reports whether the platform API surface exposes a GKE Autopilot
// allowlist CRD (v1 or v2).
func (d *Detector) isGKEAutopilot() bool {
	return slices.ContainsFunc(gkeAutopilotAllowlistKinds, d.platformInfo.IsResourceSupported)
}

// publish stores the result and logs provider changes.
func (d *Detector) publish(det *detection) {
	prev := d.current.Swap(det)
	if prev == nil || prev.Provider != det.Provider {
		d.logger.Info("Cluster provider detected", "provider", det.Provider, "source", det.Source)
	}
}
