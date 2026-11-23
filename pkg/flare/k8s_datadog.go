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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	datadogAgentGVK         = schema.GroupVersionKind{Group: datadogGroup, Version: "v2alpha1", Kind: "DatadogAgent"}
	datadogAgentInternalGVK = schema.GroupVersionKind{Group: datadogGroup, Version: "v1alpha1", Kind: "DatadogAgentInternal"}
	datadogAgentProfileGVK  = schema.GroupVersionKind{Group: datadogGroup, Version: "v1alpha1", Kind: "DatadogAgentProfile"}
	datadogCSIDriverGVK     = schema.GroupVersionKind{Group: datadogGroup, Version: "v1alpha1", Kind: "DatadogCSIDriver"}

	// countedGVKs are the resources that are only counted: they can number
	// in the thousands, and their sync errors are in the Operator logs and
	// events.
	countedGVKs = []schema.GroupVersionKind{
		{Group: datadogGroup, Version: "v1alpha1", Kind: "DatadogMonitor"},
		{Group: datadogGroup, Version: "v1alpha1", Kind: "DatadogSLO"},
		{Group: datadogGroup, Version: "v1alpha1", Kind: "DatadogDashboard"},
		{Group: datadogGroup, Version: "v1alpha1", Kind: "DatadogGenericResource"},
	}
)

// maxSampledResources caps the resources collected in full per kind. The API
// server returns at most this many per request, so memory doesn't grow with
// the number of resources.
const maxSampledResources = 20

// DatadogResourcesCollectors returns the collectors of the Datadog custom
// resources (up to maxSampledResources in full for the Agent kinds, a count
// for the others) and their events.
func DatadogResourcesCollectors() []Collector {
	return []Collector{
		sampledResourceCollector("datadog/datadogagents", datadogAgentGVK),
		sampledResourceCollector("datadog/datadogagentinternals", datadogAgentInternalGVK),
		sampledResourceCollector("datadog/datadogagentprofiles", datadogAgentProfileGVK),
		sampledResourceCollector("datadog/datadogcsidrivers", datadogCSIDriverGVK),
		{Name: "datadog/resources-count", Collect: collectResourceCounts},
		{Name: "datadog/events", Collect: collectDatadogEvents},
	}
}

func sampledResourceCollector(dir string, gvk schema.GroupVersionKind) Collector {
	return Collector{
		Name: dir,
		Collect: func(ctx context.Context, c *Collection) error {
			list := &unstructured.UnstructuredList{}
			list.SetGroupVersionKind(gvk.GroupVersion().WithKind(gvk.Kind + "List"))
			if err := c.Env.Reader.List(ctx, list, client.Limit(maxSampledResources)); err != nil {
				if isNotInstalled(err) {
					return c.AddText(path.Join(dir, "NOT_INSTALLED.txt"), []byte(fmt.Sprintf("The %s resource is not installed in this cluster.\n", gvk.GroupKind())))
				}
				return fmt.Errorf("unable to list %s: %w", gvk.Kind, err)
			}
			items := list.Items
			if len(items) > maxSampledResources {
				items = items[:maxSampledResources]
			}
			if len(items) == 0 {
				return c.AddText(path.Join(dir, "NONE.txt"), []byte(fmt.Sprintf("No %s found.\n", gvk.Kind)))
			}
			for i := range items {
				obj := cleanObject(items[i].Object)
				c.Warn(c.AddYAML(objectPath(dir, items[i].GetNamespace(), items[i].GetName()), obj))
			}
			if len(list.Items) > len(items) || list.GetContinue() != "" {
				total, err := countResources(ctx, c.Env.Reader, gvk)
				if err != nil {
					c.Warn(fmt.Errorf("collected the first %d %s, unable to count the others: %w", len(items), gvk.Kind, err))
				} else {
					c.Warn(fmt.Errorf("collected the first %d of %d %s", len(items), total, gvk.Kind))
				}
			}
			return nil
		},
	}
}

// collectResourceCounts counts the resources of countedGVKs.
func collectResourceCounts(ctx context.Context, c *Collection) error {
	counts := map[string]any{}
	for _, gvk := range countedGVKs {
		count, err := countResources(ctx, c.Env.Reader, gvk)
		switch {
		case err != nil && isNotInstalled(err):
			counts[gvk.Kind] = "not installed"
		case err != nil:
			c.Warn(fmt.Errorf("unable to count %s: %w", gvk.Kind, err))
			counts[gvk.Kind] = "error: " + err.Error()
		default:
			counts[gvk.Kind] = count
		}
	}
	return c.AddYAML("datadog/resources-count.yaml", counts)
}

// countResources counts the resources of a kind in all namespaces. It lists
// metadata only, and uses the remaining item count of the first page when the
// API server returns it.
func countResources(ctx context.Context, reader client.Reader, gvk schema.GroupVersionKind) (int, error) {
	count := 0
	continueToken := ""
	for {
		list := &metav1.PartialObjectMetadataList{}
		list.SetGroupVersionKind(gvk.GroupVersion().WithKind(gvk.Kind + "List"))
		opts := []client.ListOption{client.Limit(crdMetadataPageSize)}
		if continueToken != "" {
			opts = append(opts, client.Continue(continueToken))
		}
		if err := reader.List(ctx, list, opts...); err != nil {
			return count, err
		}
		count += len(list.Items)
		if continueToken == "" && list.RemainingItemCount != nil {
			return count + int(*list.RemainingItemCount), nil
		}
		continueToken = list.Continue
		if continueToken == "" {
			return count, nil
		}
	}
}

func collectDatadogEvents(ctx context.Context, c *Collection) error {
	kinds := make([]string, 0, 4+len(countedGVKs))
	kinds = append(kinds, datadogAgentGVK.Kind, datadogAgentInternalGVK.Kind, datadogAgentProfileGVK.Kind, datadogCSIDriverGVK.Kind)
	for _, gvk := range countedGVKs {
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
		c.Warn(addEvents(c, path.Join("datadog/events", strings.ToLower(kind)+".yaml"), events, "for "+kind+" in all namespaces"))
	}
	if failed == len(kinds) {
		return fmt.Errorf("unable to list events")
	}
	return nil
}
