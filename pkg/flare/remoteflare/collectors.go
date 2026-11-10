// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteflare

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/prometheus/common/expfmt"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/pkg/flare"
	"github.com/DataDog/datadog-operator/pkg/version"
)

// redactedFlags are flags whose value is never collected: they can hold
// secret backend credentials.
var redactedFlags = map[string]struct{}{
	"secretBackendArgs":   {},
	"secretBackendConfig": {},
}

// collectedEnvPrefixes select the environment variables collected. Values of
// variables whose name looks like a credential are redacted.
var collectedEnvPrefixes = []string{"DD_", "WATCH_NAMESPACE", "POD_", "KUBERNETES_SERVICE_", "GOMEMLIMIT", "GOMAXPROCS", "GOGC", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy"}

// credentialWords redact environment variables that contain them anywhere in
// their name, such as DD_API_KEY_FILE.
var credentialWords = []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "PASSWD", "CREDENTIAL", "AUTH", "CERT"}

func (r *Runner) operatorCollectors() []flare.Collector {
	return []flare.Collector{
		{Name: "operator/version", Collect: r.collectVersion},
		{Name: "operator/config", Collect: r.collectConfig},
		{Name: "operator/leader-election", Collect: r.collectLeaderElection},
		{Name: "operator/remote-config", Collect: r.collectRemoteConfig},
		{Name: "operator/health", Collect: r.collectHealth},
		{Name: "operator/metrics", Collect: r.collectMetrics},
		{Name: "operator/logs", Timeout: 60 * time.Second, Collect: r.collectLogs},
	}
}

func (r *Runner) collectVersion(_ context.Context, c *flare.Collection) error {
	return c.AddYAML("operator/version.yaml", map[string]any{
		"version":    version.Version,
		"commit":     version.Commit,
		"buildTime":  version.BuildTime,
		"goVersion":  runtime.Version(),
		"platform":   runtime.GOOS + "/" + runtime.GOARCH,
		"goroutines": runtime.NumGoroutine(),
	})
}

func (r *Runner) collectConfig(_ context.Context, c *flare.Collection) error {
	flags := map[string]string{}
	r.cfg.Flags.VisitAll(func(f *flag.Flag) {
		value := f.Value.String()
		if _, redact := redactedFlags[f.Name]; (redact || flare.IsSensitiveKey(f.Name)) && value != "" {
			value = "********"
		}
		flags[f.Name] = value
	})

	env := map[string]string{}
	for _, kv := range r.cfg.Environ() {
		name, value, _ := strings.Cut(kv, "=")
		if !hasAnyPrefix(name, collectedEnvPrefixes) {
			continue
		}
		if isCredentialEnv(name) && value != "" {
			value = "********"
		}
		env[name] = value
	}

	return c.AddYAML("operator/config.yaml", map[string]any{
		"flags":    flags,
		"env":      env,
		"features": r.cfg.Features,
	})
}

func hasAnyPrefix(name string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func isCredentialEnv(name string) bool {
	if flare.IsSensitiveKey(name) {
		return true
	}
	upper := strings.ToUpper(name)
	for _, word := range credentialWords {
		if strings.Contains(upper, word) {
			return true
		}
	}
	return false
}

func (r *Runner) collectLeaderElection(ctx context.Context, c *flare.Collection) error {
	info := map[string]any{
		"enabled": r.cfg.LeaderElectionEnabled,
		"id":      r.cfg.LeaderElectionID,
		"pod":     r.cfg.PodName,
	}
	if r.cfg.IsLeader != nil {
		info["isLeader"] = r.cfg.IsLeader()
	}
	var leaseErr error
	if r.cfg.LeaderElectionEnabled && r.cfg.LeaderElectionID != "" {
		lease := &coordinationv1.Lease{}
		if err := r.cfg.Reader.Get(ctx, types.NamespacedName{Namespace: r.cfg.Namespace, Name: r.cfg.LeaderElectionID}, lease); err != nil {
			leaseErr = fmt.Errorf("unable to get the leader election lease: %w", err)
		} else {
			lease.ManagedFields = nil
			info["lease"] = lease.Spec
		}
	}
	c.Warn(leaseErr)
	return c.AddYAML("operator/leader-election.yaml", info)
}

func (r *Runner) collectRemoteConfig(_ context.Context, c *flare.Collection) error {
	if r.cfg.RemoteConfigState == nil {
		return errors.New("the Remote Configuration state is not available")
	}
	return c.AddYAML("operator/remote-config.yaml", r.cfg.RemoteConfigState())
}

func (r *Runner) collectHealth(ctx context.Context, c *flare.Collection) error {
	base, err := localURL("http", r.cfg.HealthProbeAddr)
	if err != nil {
		return fmt.Errorf("unable to read the health probes: %w", err)
	}
	failures := 0
	var out bytes.Buffer
	for _, endpoint := range []string{"/healthz?verbose", "/readyz?verbose"} {
		status, body, err := r.localGet(ctx, base+endpoint)
		if err != nil {
			failures++
			c.Warn(fmt.Errorf("unable to read %s: %w", endpoint, err))
			continue
		}
		fmt.Fprintf(&out, "GET %s: %s\n%s\n\n", endpoint, status, body)
	}
	if failures == 2 {
		return errors.New("unable to read the health probes")
	}
	return c.AddText("operator/health.txt", out.Bytes())
}

// collectMetrics reads the metrics from the configured metrics endpoint.
// When the endpoint is served with authentication, or can't be read, it
// reads the same registry in-process.
func (r *Runner) collectMetrics(ctx context.Context, c *flare.Collection) error {
	source := ""
	var data []byte
	if !r.cfg.SecureMetrics && r.cfg.MetricsAddr != "0" && r.cfg.MetricsAddr != "" {
		base, err := localURL("http", r.cfg.MetricsAddr)
		if err == nil {
			var status string
			status, data, err = r.localGet(ctx, base+"/metrics")
			if err == nil && !strings.HasPrefix(status, "200") {
				err = fmt.Errorf("HTTP %s", status)
			}
			source = base + "/metrics"
		}
		if err != nil {
			c.Warn(fmt.Errorf("unable to read the metrics endpoint %s, read the metrics in-process instead: %w", r.cfg.MetricsAddr, err))
			data = nil
		}
	}
	if data == nil {
		families, err := r.cfg.Gatherer.Gather()
		if err != nil && len(families) == 0 {
			return fmt.Errorf("unable to gather the metrics: %w", err)
		}
		c.Warn(err)
		var buf bytes.Buffer
		for _, family := range families {
			if _, err := expfmt.MetricFamilyToText(&buf, family); err != nil {
				return fmt.Errorf("unable to encode the metrics: %w", err)
			}
		}
		data = buf.Bytes()
		source = "in-process controller-runtime registry"
		if r.cfg.SecureMetrics {
			source += " (the metrics endpoint requires authentication)"
		}
	}
	header := fmt.Sprintf("# Datadog Operator flare: metrics read from %s at %s\n", source, r.cfg.Now().UTC().Format(time.RFC3339))
	return c.AddText("operator/metrics.txt", append([]byte(header), data...))
}

// collectLogs collects the logs of the Operator pods: this pod and the other
// replicas of its Deployment, in the Operator namespace only.
func (r *Runner) collectLogs(ctx context.Context, c *flare.Collection) error {
	pod := c.Env.OperatorPod
	if pod == nil {
		return errors.New("the Operator pod is unknown")
	}
	pods := []corev1.Pod{*pod}
	selector := client.MatchingLabels{}
	for key, value := range pod.Labels {
		if key != "pod-template-hash" {
			selector[key] = value
		}
	}
	if len(selector) > 0 {
		replicas := &corev1.PodList{}
		if err := r.cfg.Reader.List(ctx, replicas, client.InNamespace(pod.Namespace), selector); err != nil {
			c.Warn(fmt.Errorf("unable to list the other Operator replicas: %w", err))
		} else {
			for _, replica := range replicas.Items {
				if replica.Name != pod.Name && replica.Namespace == pod.Namespace {
					pods = append(pods, replica)
				}
			}
		}
	}
	sort.SliceStable(pods[1:], func(i, j int) bool { return pods[1+i].Name < pods[1+j].Name })

	summaries := make([]flare.PodSummary, 0, len(pods))
	for i := range pods {
		summaries = append(summaries, flare.SummarizePod(&pods[i]))
		flare.CollectPodLogs(ctx, c, "operator/logs", &pods[i])
	}
	return c.AddYAML("operator/pods.yaml", summaries)
}

func (r *Runner) operatorPod(ctx context.Context) (*corev1.Pod, error) {
	if r.cfg.Namespace == "" || r.cfg.PodName == "" {
		return nil, errors.New("the Operator pod name or namespace is unknown: set POD_NAME and POD_NAMESPACE")
	}
	pod := &corev1.Pod{}
	if err := r.cfg.Reader.Get(ctx, types.NamespacedName{Namespace: r.cfg.Namespace, Name: r.cfg.PodName}, pod); err != nil {
		return nil, fmt.Errorf("unable to get the Operator pod: %w", err)
	}
	return pod, nil
}

// localURL returns the URL of a local endpoint bound to addr, such as :8080.
func localURL(scheme, addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", fmt.Errorf("invalid address %q: %w", addr, err)
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
	}
	return scheme + "://" + net.JoinHostPort(host, port), nil
}

func (r *Runner) localGet(ctx context.Context, target string) (string, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, localHTTPTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", nil, err
	}
	resp, err := r.cfg.HTTPClient.Do(request)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, flare.DefaultLogLimitBytes))
	return resp.Status, body, err
}
