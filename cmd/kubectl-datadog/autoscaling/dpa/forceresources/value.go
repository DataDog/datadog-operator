// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package forceresources

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	datadoghqcommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/cmd/kubectl-datadog/autoscaling/dpa/common"
)

// forcedResources is the annotation value: a JSON list of container resources,
// as in the DPA, e.g. [{"name":"app","requests":{"cpu":"2"},"limits":{"cpu":"4"}}].
// datadog-agent ignores the whole value when it is not such a list
// (parseForceResourcesAnnotation in model/pod_autoscaler.go), and skips the
// values it cannot use (usableForcedResources in controller_vertical_helpers.go):
// the CLI rejects both.
type forcedResources []datadoghqcommon.DatadogPodAutoscalerContainerResources

// parseValue parses an annotation value; "" is an empty override.
func parseValue(value string) (forcedResources, error) {
	if value == "" {
		return forcedResources{}, nil
	}
	var f forcedResources
	decoder := json.NewDecoder(bytes.NewReader([]byte(value)))
	// A misspelled field would be silently ignored by the agent.
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&f); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if len(f) == 0 {
		return nil, errors.New("no container set")
	}
	return f, f.validate()
}

func (f forcedResources) validate() error {
	seen := map[string]bool{}
	for _, c := range f {
		if c.Name == "" {
			return errors.New("empty container name")
		}
		if seen[c.Name] {
			return fmt.Errorf("container %q is set more than once", c.Name)
		}
		seen[c.Name] = true
		if c.Runtime != nil {
			return fmt.Errorf("container %q: runtime cannot be forced", c.Name)
		}
		if len(c.Requests) == 0 && len(c.Limits) == 0 {
			return fmt.Errorf("container %q: no resource set", c.Name)
		}
		for _, list := range []corev1.ResourceList{c.Requests, c.Limits} {
			for name, q := range list {
				if name != corev1.ResourceCPU && name != corev1.ResourceMemory {
					return fmt.Errorf("container %q: unsupported resource %q, only cpu and memory are supported", c.Name, name)
				}
				if q.Sign() <= 0 {
					return fmt.Errorf("container %q: %s quantities must be positive", c.Name, name)
				}
			}
		}
		// The agent would lower the request to the limit: reject it instead.
		for name, request := range c.Requests {
			if limit, ok := c.Limits[name]; ok && request.Cmp(limit) > 0 {
				return fmt.Errorf("container %q: %s request %s is greater than limit %s", c.Name, name, request.String(), limit.String())
			}
		}
	}
	return nil
}

// merge overlays requests and limits on each container; containers that are
// not set yet are appended in order.
func (f forcedResources) merge(containers []string, requests, limits corev1.ResourceList) forcedResources {
	for _, container := range containers {
		i := slices.IndexFunc(f, func(c datadoghqcommon.DatadogPodAutoscalerContainerResources) bool { return c.Name == container })
		if i < 0 {
			f = append(f, datadoghqcommon.DatadogPodAutoscalerContainerResources{Name: container})
			i = len(f) - 1
		}
		f[i].Requests = overlay(f[i].Requests, requests)
		f[i].Limits = overlay(f[i].Limits, limits)
	}
	return f
}

func overlay(dst, src corev1.ResourceList) corev1.ResourceList {
	for name, q := range src {
		if dst == nil {
			dst = corev1.ResourceList{}
		}
		dst[name] = q.DeepCopy()
	}
	return dst
}

// remove drops the containers from f.
func (f forcedResources) remove(containers []string) forcedResources {
	return slices.DeleteFunc(f, func(c datadoghqcommon.DatadogPodAutoscalerContainerResources) bool {
		return slices.Contains(containers, c.Name)
	})
}

// value renders f as the annotation value, with sorted resource names; an
// empty override removes the annotation.
func (f forcedResources) value() (common.AnnotationValue, error) {
	if len(f) == 0 {
		return common.Removed, nil
	}
	raw, err := json.Marshal(f)
	if err != nil {
		return common.AnnotationValue{}, err
	}
	return common.Set(string(raw)), nil
}

// parseResourceList parses the "cpu=2,memory=1Gi" syntax of kubectl set
// resources. The resource names and values are checked by validate.
func parseResourceList(spec string) (corev1.ResourceList, error) {
	if spec == "" {
		return nil, nil
	}
	list := corev1.ResourceList{}
	for item := range strings.SplitSeq(spec, ",") {
		rawName, value, ok := strings.Cut(item, "=")
		if !ok {
			return nil, fmt.Errorf("invalid resource %q, expected <name>=<quantity>", item)
		}
		name := corev1.ResourceName(strings.TrimSpace(rawName))
		if _, found := list[name]; found {
			return nil, fmt.Errorf("duplicate resource %q", name)
		}
		q, err := resource.ParseQuantity(strings.TrimSpace(value))
		if err != nil {
			return nil, fmt.Errorf("invalid %s quantity %q: %w", name, value, err)
		}
		list[name] = q
	}
	return list, nil
}
