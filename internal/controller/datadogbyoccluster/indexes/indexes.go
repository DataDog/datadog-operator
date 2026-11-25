// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package indexes converges the log indexes and the routing table of a Pomsky cluster to spec.indexes.
package indexes

import (
	"context"
	"fmt"
	"slices"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogbyoccluster/pomsky"
)

// catchAllFilter matches every log.
const catchAllFilter = "*"

// Plan is the set of changes that bring a Pomsky cluster to the desired indexes and routing table.
type Plan struct {
	// Create lists the indexes that do not exist.
	Create []pomsky.Index
	// UpdateRetention lists the indexes whose retention differs.
	UpdateRetention []pomsky.Index
	// RoutingTable is the routing table to set, nil when the current one is already the desired one.
	RoutingTable []pomsky.RoutingRule
	// Delete lists the IDs of the managed indexes that are not desired.
	Delete []string
}

// BuildPlan compares the desired indexes with the indexes and the routing table of the cluster.
//
// The routing table follows the order of desired. When desired does not list main, a catch-all rule routes the
// remaining logs to main. main is never created or deleted, as Pomsky owns its lifecycle.
// Indexes that the Datadog UI does not show as log indexes are left alone.
func BuildPlan(desired []datadoghqv1alpha1.DatadogBYOCClusterIndexSpec, current []pomsky.Index, currentRoutingTable []pomsky.RoutingRule) Plan {
	currentByID := make(map[string]pomsky.Index, len(current))
	for _, index := range current {
		currentByID[index.ID] = index
	}

	var plan Plan
	desiredIDs := make(map[string]struct{}, len(desired))
	routingTable := make([]pomsky.RoutingRule, 0, len(desired)+1)
	mainListed := false
	for _, spec := range desired {
		id := pomsky.IndexID(spec.Name)
		desiredIDs[id] = struct{}{}
		routingTable = append(routingTable, pomsky.RoutingRule{Filter: spec.Filter, IndexID: id})
		mainListed = mainListed || spec.Name == pomsky.MainIndexName

		index := pomsky.Index{ID: id, Retention: spec.Retention}
		existing, exists := currentByID[id]
		switch {
		case !exists && spec.Name != pomsky.MainIndexName:
			plan.Create = append(plan.Create, index)
		case exists && existing.Retention != index.Retention:
			plan.UpdateRetention = append(plan.UpdateRetention, index)
		}
	}
	if !mainListed {
		routingTable = append(routingTable, pomsky.RoutingRule{Filter: catchAllFilter, IndexID: pomsky.MainIndexID})
	}
	if !slices.Equal(routingTable, currentRoutingTable) {
		plan.RoutingTable = routingTable
	}

	for _, index := range current {
		name, managed := pomsky.IndexName(index.ID)
		if _, wanted := desiredIDs[index.ID]; managed && !wanted && name != pomsky.MainIndexName {
			plan.Delete = append(plan.Delete, index.ID)
		}
	}
	return plan
}

// Apply runs the plan. It creates the indexes before the routing table refers to them, and deletes the obsolete
// indexes after it stops routing to them. Deleting an index removes its routing rules.
func (p Plan) Apply(ctx context.Context, client pomsky.Client) error {
	for _, index := range p.Create {
		if err := client.CreateIndex(ctx, index.ID, index.Retention); err != nil {
			return err
		}
	}
	if p.RoutingTable != nil {
		if err := client.SetRoutingTable(ctx, p.RoutingTable); err != nil {
			return err
		}
	}
	for _, index := range p.UpdateRetention {
		if err := client.UpdateIndexRetention(ctx, index.ID, index.Retention); err != nil {
			return err
		}
	}
	for _, id := range p.Delete {
		if err := client.DeleteIndex(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// Sync converges the indexes and the routing table of the cluster to desired.
// It does nothing when desired is empty, which leaves the indexes to the Datadog UI.
func Sync(ctx context.Context, client pomsky.Client, desired []datadoghqv1alpha1.DatadogBYOCClusterIndexSpec) error {
	if len(desired) == 0 {
		return nil
	}
	current, err := client.ListIndexes(ctx)
	if err != nil {
		return err
	}
	routingTable, err := client.GetRoutingTable(ctx)
	if err != nil {
		return err
	}
	if err := BuildPlan(desired, current, routingTable).Apply(ctx, client); err != nil {
		return fmt.Errorf("sync indexes: %w", err)
	}
	return nil
}
