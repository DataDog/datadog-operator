// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"context"
	"fmt"
	"path"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	datadogAgentGVK         = schema.GroupVersionKind{Group: datadogGroup, Version: "v2alpha1", Kind: "DatadogAgent"}
	datadogAgentInternalGVK = schema.GroupVersionKind{Group: datadogGroup, Version: "v1alpha1", Kind: "DatadogAgentInternal"}
	datadogAgentProfileGVK  = schema.GroupVersionKind{Group: datadogGroup, Version: "v1alpha1", Kind: "DatadogAgentProfile"}

	// summarizedGVKs are the resources whose status, not spec, is
	// collected.
	summarizedGVKs = []schema.GroupVersionKind{
		{Group: datadogGroup, Version: "v1alpha1", Kind: "DatadogMonitor"},
		{Group: datadogGroup, Version: "v1alpha1", Kind: "DatadogSLO"},
		{Group: datadogGroup, Version: "v1alpha1", Kind: "DatadogDashboard"},
		{Group: datadogGroup, Version: "v1alpha1", Kind: "DatadogGenericResource"},
	}
)

// DatadogResourcesCollectors returns the collectors of the Datadog custom
// resources: every DatadogAgent, DatadogAgentInternal and DatadogAgentProfile
// with its full spec and status, a status summary of the DatadogMonitor,
// DatadogSLO, DatadogDashboard and DatadogGenericResource resources, and the
// Kubernetes events of all of them.
func DatadogResourcesCollectors() []Collector {
	return []Collector{
		fullResourceCollector("datadog/datadogagents", "datadog/datadogagents", datadogAgentGVK),
		fullResourceCollector("datadog/datadogagentinternals", "datadog/datadogagentinternals", datadogAgentInternalGVK),
		fullResourceCollector("datadog/datadogagentprofiles", "datadog/datadogagentprofiles", datadogAgentProfileGVK),
		{Name: "datadog/resources-summary", Collect: collectResourceSummaries},
		{Name: "datadog/events", Collect: collectDatadogEvents},
	}
}

func fullResourceCollector(name, dir string, gvk schema.GroupVersionKind) Collector {
	return Collector{
		Name: name,
		Collect: func(ctx context.Context, c *Collection) error {
			items, found, truncated, err := listUnstructured(ctx, c.Env.Reader, gvk)
			if err != nil {
				return fmt.Errorf("unable to list %s: %w", gvk.Kind, err)
			}
			if !found {
				return c.AddText(path.Join(dir, "NOT_INSTALLED.txt"), []byte(fmt.Sprintf("The %s resource is not installed in this cluster.\n", gvk.GroupKind())))
			}
			if truncated {
				c.Warn(fmt.Errorf("only the first %d %s were collected", maxListItems, gvk.Kind))
			}
			if len(items) == 0 {
				return c.AddText(path.Join(dir, "NONE.txt"), []byte(fmt.Sprintf("No %s found.\n", gvk.Kind)))
			}
			for i := range items {
				obj := cleanObject(items[i].Object)
				c.Warn(c.AddYAML(objectPath(dir, items[i].GetNamespace(), items[i].GetName()), obj))
			}
			return nil
		},
	}
}

// resourceSummary is the summary of a Datadog resource.
type resourceSummary struct {
	Namespace         string         `json:"namespace"`
	Name              string         `json:"name"`
	Generation        int64          `json:"generation,omitempty"`
	CreationTimestamp string         `json:"creationTimestamp,omitempty"`
	Deleting          bool           `json:"deleting,omitempty"`
	Status            map[string]any `json:"status,omitempty"`
}

func collectResourceSummaries(ctx context.Context, c *Collection) error {
	summaries := map[string]any{}
	for _, gvk := range summarizedGVKs {
		items, found, truncated, err := listUnstructured(ctx, c.Env.Reader, gvk)
		switch {
		case err != nil:
			c.Warn(fmt.Errorf("unable to list %s: %w", gvk.Kind, err))
			summaries[gvk.Kind] = "error: " + err.Error()
			continue
		case !found:
			summaries[gvk.Kind] = "not installed"
			continue
		case truncated:
			c.Warn(fmt.Errorf("only the first %d %s were collected", maxListItems, gvk.Kind))
		}
		list := make([]resourceSummary, 0, len(items))
		for i := range items {
			status, _, _ := unstructured.NestedMap(items[i].Object, "status")
			list = append(list, resourceSummary{
				Namespace:         items[i].GetNamespace(),
				Name:              items[i].GetName(),
				Generation:        items[i].GetGeneration(),
				CreationTimestamp: items[i].GetCreationTimestamp().UTC().Format("2006-01-02T15:04:05Z"),
				Deleting:          items[i].GetDeletionTimestamp() != nil,
				Status:            status,
			})
		}
		summaries[gvk.Kind] = list
	}
	return c.AddYAML("datadog/resources-summary.yaml", summaries)
}

func collectDatadogEvents(ctx context.Context, c *Collection) error {
	kinds := make([]string, 0, 3+len(summarizedGVKs))
	kinds = append(kinds, datadogAgentGVK.Kind, datadogAgentInternalGVK.Kind, datadogAgentProfileGVK.Kind)
	for _, gvk := range summarizedGVKs {
		kinds = append(kinds, gvk.Kind)
	}
	failed := 0
	for _, kind := range kinds {
		events, err := listEvents(ctx, c.Env, "", fields.OneTermEqualSelector("involvedObject.kind", kind), func(e *corev1.Event) bool {
			return e.InvolvedObject.Kind == kind && strings.HasPrefix(e.InvolvedObject.APIVersion, datadogGroup+"/")
		})
		if err != nil {
			failed++
			c.Warn(fmt.Errorf("unable to list %s events: %w", kind, err))
			continue
		}
		if len(events) == 0 {
			continue
		}
		c.Warn(c.AddYAML(path.Join("datadog/events", strings.ToLower(kind)+".yaml"), events))
	}
	if failed == len(kinds) {
		return fmt.Errorf("unable to list events")
	}
	return nil
}
