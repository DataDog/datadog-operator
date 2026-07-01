// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// PortClaimAnnotationPrefix namespaces per-(DDAI, feature) port claims written
// onto a shared resource (the node Agent local Service). The resource's single
// writer merges every claim, so it never has to read other DDAI objects.
// Full key shape: operator.datadoghq.com/port-claim.<ddai-name>.<feature-id>
const PortClaimAnnotationPrefix = "operator.datadoghq.com/port-claim."

// PortClaimAnnotationKey builds the annotation key for one (DDAI, feature)
// port claim.
func PortClaimAnnotationKey(ddaiName, featureID string) string {
	return PortClaimAnnotationPrefix + ddaiName + "." + featureID
}

// portClaimKeyDDAI returns the DDAI name encoded in a port-claim key, or "" if
// the key is not a port-claim key. The DDAI name is everything between the
// prefix and the final ".<feature-id>" segment.
func portClaimKeyDDAI(key string) string {
	rest, ok := strings.CutPrefix(key, PortClaimAnnotationPrefix)
	if !ok {
		return ""
	}
	idx := strings.LastIndex(rest, ".")
	if idx <= 0 {
		return ""
	}
	return rest[:idx]
}

// ClaimantDDAINames returns the distinct DatadogAgentInternal names that have a
// port claim in the given annotations.
func ClaimantDDAINames(annotations map[string]string) []string {
	seen := map[string]bool{}
	var names []string
	for key := range annotations {
		if name := portClaimKeyDDAI(key); name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	return names
}

// EncodeServicePorts serializes a port claim for storage in an annotation value.
func EncodeServicePorts(ports []corev1.ServicePort) (string, error) {
	b, err := json.Marshal(ports)
	if err != nil {
		return "", fmt.Errorf("encoding claimed service ports: %w", err)
	}
	return string(b), nil
}

// DecodeServicePorts parses a port-claim annotation value back into ports.
func DecodeServicePorts(value string) ([]corev1.ServicePort, error) {
	var ports []corev1.ServicePort
	if err := json.Unmarshal([]byte(value), &ports); err != nil {
		return nil, fmt.Errorf("decoding claimed service ports: %w", err)
	}
	return ports, nil
}

// PortClaimConflictError identifies the claimant whose declaration could not be
// merged, so the caller can surface the conflict on that owner (e.g. a DAP status).
type PortClaimConflictError struct {
	// DDAIName is the DatadogAgentInternal that claimed the conflicting ports.
	DDAIName string
	// Err is the underlying conflict (wraps ErrServicePortConflict).
	Err error
}

func (c *PortClaimConflictError) Error() string {
	return fmt.Sprintf("port claim %q: %v", c.DDAIName, c.Err)
}

func (c *PortClaimConflictError) Unwrap() error { return c.Err }

// MergePortClaims reads every port-claim.* declaration from the given
// annotations and returns their union. Claims are merged in a deterministic
// (sorted-key) order so conflict attribution is stable. The first claim whose
// ports collide with the accumulated set is returned as a *PortClaimConflictError.
func MergePortClaims(annotations map[string]string) ([]corev1.ServicePort, *PortClaimConflictError) {
	keys := make([]string, 0, len(annotations))
	for key := range annotations {
		if strings.HasPrefix(key, PortClaimAnnotationPrefix) {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	var merged []corev1.ServicePort
	for _, key := range keys {
		ports, err := DecodeServicePorts(annotations[key])
		if err != nil {
			return nil, &PortClaimConflictError{DDAIName: portClaimKeyDDAI(key), Err: err}
		}
		next, err := MergeServicePorts(merged, ports)
		if err != nil {
			return nil, &PortClaimConflictError{DDAIName: portClaimKeyDDAI(key), Err: err}
		}
		merged = next
	}
	return merged, nil
}
