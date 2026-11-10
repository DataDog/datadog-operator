// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flare

import (
	"context"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// listPageSize is the page size of list calls.
	listPageSize = 500
	// maxListItems caps the objects collected per kind.
	maxListItems = 2000
	// maxEventsPerKind caps the events collected per kind or workload.
	maxEventsPerKind = 500
	// managedByLabel and managedByOperator select the objects the Operator
	// creates.
	managedByLabel    = "app.kubernetes.io/managed-by"
	managedByOperator = "datadog-operator"
	datadogGroup      = "datadoghq.com"
)

// listUnstructured lists objects of a kind in all namespaces, up to
// maxListItems. found is false when the kind isn't installed.
func listUnstructured(ctx context.Context, reader client.Reader, gvk schema.GroupVersionKind, opts ...client.ListOption) (items []unstructured.Unstructured, found bool, truncated bool, err error) {
	continueToken := ""
	for {
		list := &unstructured.UnstructuredList{}
		list.SetGroupVersionKind(gvk.GroupVersion().WithKind(gvk.Kind + "List"))
		listOpts := append([]client.ListOption{client.Limit(listPageSize)}, opts...)
		if continueToken != "" {
			listOpts = append(listOpts, client.Continue(continueToken))
		}
		if err := reader.List(ctx, list, listOpts...); err != nil {
			if isNotInstalled(err) {
				return nil, false, false, nil
			}
			return items, true, false, err
		}
		items = append(items, list.Items...)
		if len(items) >= maxListItems {
			return items[:maxListItems], true, true, nil
		}
		continueToken = list.GetContinue()
		if continueToken == "" {
			return items, true, false, nil
		}
	}
}

// isNotInstalled reports whether err means a kind isn't served by the API
// server, for example because its CRD isn't installed.
func isNotInstalled(err error) bool {
	return meta.IsNoMatchError(err) || apierrors.IsNotFound(err) || strings.Contains(err.Error(), "no matches for kind")
}

// cleanObject removes noisy metadata before an object is written.
func cleanObject(obj map[string]any) map[string]any {
	if md, ok := obj["metadata"].(map[string]any); ok {
		delete(md, "managedFields")
		if annotations, ok := md["annotations"].(map[string]any); ok {
			// The last applied configuration repeats the spec, and may
			// hold values the structured redaction can't see.
			delete(annotations, "kubectl.kubernetes.io/last-applied-configuration")
		}
	}
	return obj
}

// cleanTyped strips managed fields and the last applied configuration from
// a typed object.
func cleanTyped(obj metav1.Object) {
	obj.SetManagedFields(nil)
	if annotations := obj.GetAnnotations(); annotations != nil {
		delete(annotations, "kubectl.kubernetes.io/last-applied-configuration")
		obj.SetAnnotations(annotations)
	}
}

// objectPath returns dir/<namespace>/<name>.yaml, or dir/<name>.yaml for a
// cluster-scoped object.
func objectPath(dir, namespace, name string) string {
	if namespace == "" {
		return path.Join(dir, safeName(name)+".yaml")
	}
	return path.Join(dir, safeName(namespace), safeName(name)+".yaml")
}

func safeName(name string) string {
	name = strings.NewReplacer("/", "_", "\\", "_", "..", "_").Replace(name)
	if name == "" {
		return "_"
	}
	return name
}

// EventSummary is a Kubernetes event in a flare.
type EventSummary struct {
	Namespace      string    `json:"namespace,omitempty"`
	InvolvedObject string    `json:"involvedObject"`
	Type           string    `json:"type,omitempty"`
	Reason         string    `json:"reason,omitempty"`
	Message        string    `json:"message,omitempty"`
	Count          int32     `json:"count,omitempty"`
	FirstTimestamp time.Time `json:"firstTimestamp,omitzero"`
	LastTimestamp  time.Time `json:"lastTimestamp,omitzero"`
	Source         string    `json:"source,omitempty"`
}

func summarizeEvent(event *corev1.Event) EventSummary {
	last := event.LastTimestamp.Time
	if last.IsZero() {
		last = event.EventTime.Time
	}
	source := event.Source.Component
	if source == "" {
		source = event.ReportingController
	}
	return EventSummary{
		Namespace:      event.Namespace,
		InvolvedObject: fmt.Sprintf("%s/%s", event.InvolvedObject.Kind, event.InvolvedObject.Name),
		Type:           event.Type,
		Reason:         event.Reason,
		Message:        event.Message,
		Count:          event.Count,
		FirstTimestamp: event.FirstTimestamp.Time,
		LastTimestamp:  last,
		Source:         source,
	}
}

// listEvents lists events in a namespace ("" for all namespaces) that match a
// field selector, and keeps the ones keep accepts, most recent last.
func listEvents(ctx context.Context, env Env, namespace string, selector fields.Selector, keep func(*corev1.Event) bool) ([]EventSummary, error) {
	opts := metav1.ListOptions{Limit: maxListItems}
	if selector != nil {
		opts.FieldSelector = selector.String()
	}
	events, err := env.Clientset.CoreV1().Events(namespace).List(ctx, opts)
	if err != nil {
		return nil, err
	}
	var summaries []EventSummary
	for i := range events.Items {
		if keep == nil || keep(&events.Items[i]) {
			summaries = append(summaries, summarizeEvent(&events.Items[i]))
		}
	}
	sort.SliceStable(summaries, func(i, j int) bool { return summaries[i].LastTimestamp.Before(summaries[j].LastTimestamp) })
	if len(summaries) > maxEventsPerKind {
		summaries = summaries[len(summaries)-maxEventsPerKind:]
	}
	return summaries, nil
}
