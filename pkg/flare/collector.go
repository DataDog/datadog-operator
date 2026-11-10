// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// DefaultTimeout is the default time limit to collect a flare.
	DefaultTimeout = 2 * time.Minute
	// defaultCollectorTimeout bounds a single collector, so that one slow
	// collector leaves time for the others.
	defaultCollectorTimeout = 30 * time.Second
	// maxErrorsPerCollector caps the errors recorded for one collector.
	maxErrorsPerCollector = 20
)

// Collector status values recorded in the manifest.
const (
	StatusOK      = "ok"
	StatusPartial = "partial"
	StatusError   = "error"
	StatusTimeout = "timeout"
	StatusSkipped = "skipped"
)

// Env is what collectors use to read the cluster. Collectors only read.
type Env struct {
	// Reader reads Kubernetes objects. The Operator passes its API reader,
	// which reads from the API server without starting informers.
	Reader client.Reader
	// Clientset reads pod logs, events and the server version.
	Clientset kubernetes.Interface
	// OperatorNamespace is the namespace the Operator runs in.
	OperatorNamespace string
	// OperatorPod is the Operator pod the flare is about (the one running
	// the flare, or the leader for the kubectl plugin). It may be nil.
	OperatorPod *corev1.Pod
	// ClusterName is the cluster name the Operator is configured with, if
	// any.
	ClusterName string
	// LogTailLines caps the lines collected per container log.
	LogTailLines int64
	// LogLimitBytes caps the bytes collected per container log.
	LogLimitBytes int64
}

// Collection is what a running collector writes to.
type Collection struct {
	*Archive
	Env    Env
	mu     sync.Mutex
	errors []string
}

// Warn records a non-fatal error: the collector goes on and its status is
// partial.
func (c *Collection) Warn(err error) {
	if err == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.errors) < maxErrorsPerCollector {
		c.errors = append(c.errors, c.Scrubber().ScrubString(err.Error()))
	} else if len(c.errors) == maxErrorsPerCollector {
		c.errors = append(c.errors, "more errors were not recorded")
	}
}

func (c *Collection) recordedErrors() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.errors...)
}

// Collector collects one part of a flare.
type Collector struct {
	// Name identifies the collector in the manifest.
	Name string
	// Timeout bounds the collector. 30 seconds if 0.
	Timeout time.Duration
	// Collect writes files to the collection. A returned error marks the
	// collector as failed. Use Collection.Warn for errors that don't stop it.
	Collect func(ctx context.Context, c *Collection) error
}

// Manifest describes a flare. It's written as manifest.json at the root of
// the archive.
type Manifest struct {
	FlareVersion    int               `json:"flare_version"`
	OperatorVersion string            `json:"operator_version,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
	Duration        string            `json:"duration"`
	Complete        bool              `json:"complete"`
	Timeout         string            `json:"timeout"`
	MaxBytes        int64             `json:"max_bytes"`
	TotalBytes      int64             `json:"total_bytes"`
	Info            map[string]string `json:"info,omitempty"`
	Collectors      []CollectorResult `json:"collectors"`
	FailedCollector []string          `json:"failed_collectors,omitempty"`
	Files           []FileInfo        `json:"files"`
}

// CollectorResult is the result of a collector in the manifest.
type CollectorResult struct {
	Name     string   `json:"name"`
	Status   string   `json:"status"`
	Duration string   `json:"duration"`
	Errors   []string `json:"errors,omitempty"`
}

// RunOptions configures Run.
type RunOptions struct {
	// Timeout bounds the whole collection. DefaultTimeout if 0.
	Timeout time.Duration
	// Now returns the current time. time.Now if nil.
	Now func() time.Time
}

// Run runs the collectors one after the other until the time limit, and
// returns the manifest. A collector that fails or times out doesn't stop the
// others: the flare is partial and the manifest says which collectors failed.
// Collectors that didn't start before the time limit are skipped.
func Run(ctx context.Context, env Env, archive *Archive, collectors []Collector, opts RunOptions) *Manifest {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	start := now()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	manifest := &Manifest{
		FlareVersion: 1,
		CreatedAt:    start.UTC(),
		Timeout:      timeout.String(),
		Complete:     true,
	}
	for _, collector := range collectors {
		result := runCollector(ctx, env, archive, collector, now)
		if result.Status != StatusOK {
			manifest.Complete = false
			manifest.FailedCollector = append(manifest.FailedCollector, collector.Name)
		}
		manifest.Collectors = append(manifest.Collectors, result)
	}
	manifest.Duration = now().Sub(start).Round(time.Millisecond).String()
	return manifest
}

func runCollector(ctx context.Context, env Env, archive *Archive, collector Collector, now func() time.Time) CollectorResult {
	result := CollectorResult{Name: collector.Name}
	if ctx.Err() != nil {
		result.Status = StatusSkipped
		result.Duration = "0s"
		result.Errors = []string{"the flare time limit was reached before this collector started"}
		return result
	}

	timeout := collector.Timeout
	if timeout <= 0 {
		timeout = defaultCollectorTimeout
	}
	collectorCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	collection := &Collection{Archive: archive, Env: env}
	start := now()
	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("collector panicked: %v", r)
			}
		}()
		done <- collector.Collect(collectorCtx, collection)
	}()

	var err error
	select {
	case err = <-done:
	case <-collectorCtx.Done():
		// The collector may still be running: it stops at its next API
		// call, and anything it writes after Close is dropped.
		err = collectorCtx.Err()
	}
	result.Duration = now().Sub(start).Round(time.Millisecond).String()
	result.Errors = collection.recordedErrors()

	switch {
	case errors.Is(err, context.DeadlineExceeded) || (err != nil && collectorCtx.Err() != nil):
		result.Status = StatusTimeout
		result.Errors = append(result.Errors, fmt.Sprintf("time limit reached after %s", result.Duration))
	case err != nil:
		result.Status = StatusError
		result.Errors = append(result.Errors, archive.Scrubber().ScrubString(err.Error()))
	case len(result.Errors) > 0:
		result.Status = StatusPartial
	default:
		result.Status = StatusOK
	}
	return result
}
