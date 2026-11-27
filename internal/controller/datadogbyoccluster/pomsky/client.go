// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package pomsky is a client for the index and routing table REST API of Pomsky.
package pomsky

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// MainIndexName is the name that the Datadog UI shows for the default index.
	MainIndexName = "main"
	// MainIndexID is the ID under which Pomsky stores the default index.
	MainIndexID = "datadog"

	indexIDPrefix = "datadog-"

	requestTimeout       = 30 * time.Second
	maxErrorBodyBytes    = 1024
	maxResponseBodyBytes = 64 << 20
)

// ReservedIndexNamePrefixes are the name prefixes of the spans, metrics and sketches indexes,
// which cloudprem-bridge refuses to create as log indexes.
var ReservedIndexNamePrefixes = []string{"spans", "metrics", "sketches"}

// IndexID returns the ID under which Pomsky stores the index that the Datadog UI shows as name.
func IndexID(name string) string {
	if name == MainIndexName {
		return MainIndexID
	}
	return indexIDPrefix + name
}

// Index is the part of an index that the Operator manages.
type Index struct {
	// ID is the index ID in Pomsky.
	ID string
	// Retention is the retention period, empty when the index has no retention policy.
	Retention string
}

// RoutingRule routes the logs that match Filter to the index IndexID.
type RoutingRule struct {
	Filter  string `json:"filter"`
	IndexID string `json:"index_id"`
}

// Client manages the indexes and the routing table of a Pomsky cluster.
type Client interface {
	// ListIndexes returns every index of the cluster, including those that the Operator does not manage.
	ListIndexes(ctx context.Context) ([]Index, error)
	// CreateIndex creates an index with the configuration of the main index and the given retention.
	CreateIndex(ctx context.Context, id, retention string) error
	// UpdateIndexRetention sets the retention of an index and keeps the rest of its configuration.
	UpdateIndexRetention(ctx context.Context, id, retention string) error
	GetRoutingTable(ctx context.Context) ([]RoutingRule, error)
	// SetRoutingTable replaces the routing table.
	SetRoutingTable(ctx context.Context, rules []RoutingRule) error
}

// Factory creates a Client for the REST API served at baseURL.
type Factory func(baseURL string) Client

// NewClient returns a Client for the REST API served at baseURL.
func NewClient(baseURL string) Client {
	return &httpClient{baseURL: strings.TrimSuffix(baseURL, "/"), http: &http.Client{Timeout: requestTimeout}}
}

type httpClient struct {
	baseURL string
	http    *http.Client
}

// indexMetadata keeps the index configuration as it is, so that it can be cloned and written back unchanged.
type indexMetadata struct {
	IndexConfig map[string]any `json:"index_config"`
}

func (c *httpClient) ListIndexes(ctx context.Context) ([]Index, error) {
	var metadata []indexMetadata
	if err := c.do(ctx, http.MethodGet, "/api/v1/indexes", nil, &metadata); err != nil {
		return nil, fmt.Errorf("list indexes: %w", err)
	}
	indexes := make([]Index, 0, len(metadata))
	for _, m := range metadata {
		id, _ := m.IndexConfig["index_id"].(string)
		indexes = append(indexes, Index{ID: id, Retention: retentionPeriod(m.IndexConfig)})
	}
	return indexes, nil
}

func (c *httpClient) CreateIndex(ctx context.Context, id, retention string) error {
	config, err := c.indexConfig(ctx, MainIndexID)
	if err != nil {
		return fmt.Errorf("create index %s: %w", id, err)
	}
	config["index_id"] = id
	// Pomsky derives the URI from its default index root and the index ID.
	delete(config, "index_uri")
	setRetentionPeriod(config, retention)
	if err := c.do(ctx, http.MethodPost, "/api/v1/indexes", config, nil); err != nil {
		return fmt.Errorf("create index %s: %w", id, err)
	}
	return nil
}

func (c *httpClient) UpdateIndexRetention(ctx context.Context, id, retention string) error {
	config, err := c.indexConfig(ctx, id)
	if err != nil {
		return fmt.Errorf("update index %s: %w", id, err)
	}
	setRetentionPeriod(config, retention)
	if err := c.do(ctx, http.MethodPut, "/api/v1/indexes/"+url.PathEscape(id), config, nil); err != nil {
		return fmt.Errorf("update index %s: %w", id, err)
	}
	return nil
}

func (c *httpClient) GetRoutingTable(ctx context.Context) ([]RoutingRule, error) {
	var rules []RoutingRule
	if err := c.do(ctx, http.MethodGet, "/api/v1/index-routing-table", nil, &rules); err != nil {
		return nil, fmt.Errorf("get routing table: %w", err)
	}
	return rules, nil
}

func (c *httpClient) SetRoutingTable(ctx context.Context, rules []RoutingRule) error {
	if err := c.do(ctx, http.MethodPut, "/api/v1/index-routing-table", rules, nil); err != nil {
		return fmt.Errorf("set routing table: %w", err)
	}
	return nil
}

func (c *httpClient) indexConfig(ctx context.Context, id string) (map[string]any, error) {
	var metadata indexMetadata
	if err := c.do(ctx, http.MethodGet, "/api/v1/indexes/"+url.PathEscape(id), nil, &metadata); err != nil {
		return nil, fmt.Errorf("get index %s: %w", id, err)
	}
	if metadata.IndexConfig == nil {
		return nil, fmt.Errorf("get index %s: response has no index_config", id)
	}
	return metadata.IndexConfig, nil
}

func retentionPeriod(config map[string]any) string {
	retention, _ := config["retention"].(map[string]any)
	period, _ := retention["period"].(string)
	return period
}

func setRetentionPeriod(config map[string]any, period string) {
	retention, ok := config["retention"].(map[string]any)
	if !ok {
		retention = map[string]any{}
	}
	retention["period"] = period
	config["retention"] = retention
}

// StatusError is returned when Pomsky answers with an unsuccessful HTTP status.
type StatusError struct {
	Method     string
	Path       string
	StatusCode int
	Body       string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("%s %s: status %d: %s", e.Method, e.Path, e.StatusCode, e.Body)
}

func (c *httpClient) do(ctx context.Context, method, path string, requestBody, responseBody any) error {
	var body io.Reader
	if requestBody != nil {
		encoded, err := json.Marshal(requestBody)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	if requestBody != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode > 299 {
		message, _ := io.ReadAll(io.LimitReader(response.Body, maxErrorBodyBytes))
		return &StatusError{Method: method, Path: path, StatusCode: response.StatusCode, Body: strings.TrimSpace(string(message))}
	}
	if responseBody == nil {
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, maxResponseBodyBytes)).Decode(responseBody); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
