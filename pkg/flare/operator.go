// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	coordinationv1 "k8s.io/api/coordination/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Operator collectors and helpers of the remote flare.

const (
	// LeaderElectionID is the name of the Operator's leader election Lease.
	LeaderElectionID = "datadog-operator-lock"
	// DefaultHealthProbePort is the port of the Operator health probes.
	DefaultHealthProbePort = "8081"
)

// HealthEndpoints are the health probe endpoints collected in flares.
var HealthEndpoints = []string{"/healthz?verbose", "/readyz?verbose"}

// redactedFlags are flags whose value is never collected: they can hold
// secret backend credentials.
var redactedFlags = map[string]struct{}{
	"secretBackendArgs":   {},
	"secretBackendConfig": {},
}

// collectedEnvPrefixes select the environment variables of the Operator
// process collected by the remote flare.
var collectedEnvPrefixes = []string{"DD_", "WATCH_NAMESPACE", "POD_", "KUBERNETES_SERVICE_", "GOMEMLIMIT", "GOMAXPROCS", "GOGC", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy"}

// credentialWords redact environment variables that contain them anywhere in
// their name, such as DD_API_KEY_FILE.
var credentialWords = []string{"KEY", "TOKEN", "SECRET", "PASSWORD", "PASSWD", "CREDENTIAL", "AUTH", "CERT"}

func isSecretFlag(name string) bool {
	_, secret := redactedFlags[name]
	return secret || IsSensitiveKey(name)
}

// RedactFlagValue returns the value of an Operator flag as it's written in
// flares: credentials and secret backend settings are redacted.
func RedactFlagValue(name, value string) string {
	if isSecretFlag(name) && value != "" {
		return redacted
	}
	return value
}

// IsCollectedEnv reports whether an environment variable of the Operator
// process is collected by the remote flare.
func IsCollectedEnv(name string) bool {
	for _, prefix := range collectedEnvPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// RedactEnvValue returns the value of an environment variable as it's
// written in flares: variables whose name looks like a credential are
// redacted.
func RedactEnvValue(name, value string) string {
	if value != "" && isCredentialEnv(name) {
		return redacted
	}
	return value
}

func isCredentialEnv(name string) bool {
	if IsSensitiveKey(name) {
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

// GetLease returns the spec of the Operator's leader election Lease.
func GetLease(ctx context.Context, reader client.Reader, namespace, name string) (*coordinationv1.LeaseSpec, error) {
	lease := &coordinationv1.Lease{}
	if err := reader.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, lease); err != nil {
		return nil, fmt.Errorf("unable to get the leader election lease %s/%s: %w", namespace, name, err)
	}
	return &lease.Spec, nil
}

// HealthGetter reads a health probe endpoint, such as /readyz?verbose. It
// returns the HTTP status even when the endpoint reports a failure.
type HealthGetter func(ctx context.Context, endpoint string) (status string, body []byte, err error)

// CollectHealthEndpoints writes the Operator health probe endpoints to
// operator/health.txt.
func CollectHealthEndpoints(ctx context.Context, c *Collection, get HealthGetter) error {
	failures := 0
	var out bytes.Buffer
	for _, endpoint := range HealthEndpoints {
		status, body, err := get(ctx, endpoint)
		if err != nil {
			failures++
			c.Warn(fmt.Errorf("unable to read %s: %w", endpoint, err))
			continue
		}
		if strings.HasPrefix(status, "404") {
			// controller-runtime only serves the endpoints that have
			// checks: the Operator has no readiness checks.
			fmt.Fprintf(&out, "GET %s: %s (no checks are registered for this endpoint)\n\n", endpoint, status)
			continue
		}
		fmt.Fprintf(&out, "GET %s: %s\n%s\n\n", endpoint, status, body)
	}
	if failures == len(HealthEndpoints) {
		return errors.New("unable to read the health probes")
	}
	return c.AddText("operator/health.txt", out.Bytes())
}
