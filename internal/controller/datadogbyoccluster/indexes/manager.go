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

// Manager converges the log indexes and the routing table of a Pomsky cluster.
type Manager struct {
	client pomsky.Client
}

// NewManager returns a Manager that calls the REST API of a Pomsky cluster through client.
func NewManager(client pomsky.Client) *Manager {
	return &Manager{client: client}
}

// Sync converges the indexes and the routing table of the cluster to desired.
// It does nothing when desired is empty, which leaves the indexes to the Datadog UI.
func (m *Manager) Sync(ctx context.Context, desired []datadoghqv1alpha1.DatadogBYOCClusterIndexSpec) error {
	if len(desired) == 0 {
		return nil
	}
	c, err := m.plan(ctx, desired)
	if err != nil {
		return fmt.Errorf("plan index changes: %w", err)
	}
	if err := m.apply(ctx, c); err != nil {
		return fmt.Errorf("apply index changes: %w", err)
	}
	return nil
}

// changes is the set of changes that bring a Pomsky cluster to the desired indexes and routing table.
type changes struct {
	// create lists the indexes that do not exist.
	create []pomsky.Index
	// updateRetention lists the indexes whose retention differs.
	updateRetention []pomsky.Index
	// routingTable is the routing table to set, nil when the current one is already the desired one.
	routingTable []pomsky.RoutingRule
	// delete lists the IDs of the managed indexes that are not desired.
	delete []string
}

// plan reads the indexes and the routing table of the cluster and compares them with desired.
//
// The routing table follows the order of desired. When desired does not list main, a catch-all rule routes the
// remaining logs to main. main is never created or deleted, as Pomsky owns its lifecycle.
// Indexes that the Datadog UI does not show as log indexes are left alone.
func (m *Manager) plan(ctx context.Context, desired []datadoghqv1alpha1.DatadogBYOCClusterIndexSpec) (changes, error) {
	current, err := m.client.ListIndexes(ctx)
	if err != nil {
		return changes{}, err
	}
	currentRoutingTable, err := m.client.GetRoutingTable(ctx)
	if err != nil {
		return changes{}, err
	}
	currentByID := make(map[string]pomsky.Index, len(current))
	for _, index := range current {
		currentByID[index.ID] = index
	}

	var c changes
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
			c.create = append(c.create, index)
		case exists && existing.Retention != index.Retention:
			c.updateRetention = append(c.updateRetention, index)
		}
	}
	if !mainListed {
		routingTable = append(routingTable, pomsky.RoutingRule{Filter: catchAllFilter, IndexID: pomsky.MainIndexID})
	}
	if !slices.Equal(routingTable, currentRoutingTable) {
		c.routingTable = routingTable
	}

	for _, index := range current {
		name, managed := pomsky.IndexName(index.ID)
		if _, wanted := desiredIDs[index.ID]; managed && !wanted && name != pomsky.MainIndexName {
			c.delete = append(c.delete, index.ID)
		}
	}
	return c, nil
}

// apply makes the changes. It creates the indexes before the routing table refers to them, and deletes the obsolete
// indexes after it stops routing to them. Deleting an index removes its routing rules.
func (m *Manager) apply(ctx context.Context, c changes) error {
	for _, index := range c.create {
		if err := m.client.CreateIndex(ctx, index.ID, index.Retention); err != nil {
			return err
		}
	}
	if c.routingTable != nil {
		if err := m.client.SetRoutingTable(ctx, c.routingTable); err != nil {
			return err
		}
	}
	for _, index := range c.updateRetention {
		if err := m.client.UpdateIndexRetention(ctx, index.ID, index.Retention); err != nil {
			return err
		}
	}
	for _, id := range c.delete {
		if err := m.client.DeleteIndex(ctx, id); err != nil {
			return err
		}
	}
	return nil
}
