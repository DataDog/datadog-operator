// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/spf13/pflag"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
)

// selection describes which DatadogPodAutoscaler objects a command acts on.
type selection struct {
	names         []string
	labelSelector string
	all           bool
	allNamespaces bool
}

func (s *selection) addFlags(fs *pflag.FlagSet) {
	fs.StringVarP(&s.labelSelector, "label-selector", "l", "", "Select DatadogPodAutoscalers by label selector (e.g. -l app=web)")
	fs.BoolVar(&s.all, "all", false, "Select every DatadogPodAutoscaler in the namespace (or the cluster with -A)")
	fs.BoolVarP(&s.allNamespaces, "all-namespaces", "A", false, "Look in every namespace, with names, --label-selector or --all")
}

// validate requires an explicit selection: names, -l or --all; -A only widens it.
func (s *selection) validate() error {
	switch {
	case len(s.names) > 0 && (s.labelSelector != "" || s.all):
		return errors.New("DatadogPodAutoscaler names cannot be combined with --label-selector or --all")
	case s.labelSelector != "" && s.all:
		return errors.New("--label-selector and --all are mutually exclusive")
	case len(s.names) == 0 && s.labelSelector == "" && !s.all:
		return errors.New("select DatadogPodAutoscalers by name, with --label-selector or --all (-A widens them to every namespace)")
	}
	return nil
}

// resolve returns the selected objects, sorted by namespace and name.
func (s *selection) resolve(ctx context.Context, c client.Client, namespace string) ([]v1alpha2.DatadogPodAutoscaler, error) {
	var dpas []v1alpha2.DatadogPodAutoscaler
	if len(s.names) > 0 && !s.allNamespaces {
		for _, name := range s.names {
			dpa := v1alpha2.DatadogPodAutoscaler{}
			if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &dpa); err != nil {
				return nil, fmt.Errorf("unable to get DatadogPodAutoscaler %s/%s: %w", namespace, name, err)
			}
			dpas = append(dpas, dpa)
		}
	} else {
		var opts []client.ListOption
		if !s.allNamespaces {
			opts = append(opts, client.InNamespace(namespace))
		}
		if s.labelSelector != "" {
			selector, err := labels.Parse(s.labelSelector)
			if err != nil {
				return nil, fmt.Errorf("invalid --label-selector: %w", err)
			}
			opts = append(opts, client.MatchingLabelsSelector{Selector: selector})
		}
		list := v1alpha2.DatadogPodAutoscalerList{}
		if err := c.List(ctx, &list, opts...); err != nil {
			return nil, fmt.Errorf("unable to list DatadogPodAutoscalers: %w", err)
		}
		dpas = list.Items
		if len(s.names) > 0 {
			// Names with -A: the same name may match in several namespaces.
			dpas = slices.DeleteFunc(dpas, func(dpa v1alpha2.DatadogPodAutoscaler) bool {
				return !slices.Contains(s.names, dpa.Name)
			})
			for _, name := range s.names {
				if !slices.ContainsFunc(dpas, func(dpa v1alpha2.DatadogPodAutoscaler) bool { return dpa.Name == name }) {
					return nil, fmt.Errorf("DatadogPodAutoscaler %s not found in any namespace", name)
				}
			}
		}
	}

	slices.SortFunc(dpas, func(a, b v1alpha2.DatadogPodAutoscaler) int {
		return cmp.Or(cmp.Compare(a.Namespace, b.Namespace), cmp.Compare(a.Name, b.Name))
	})
	return dpas, nil
}
