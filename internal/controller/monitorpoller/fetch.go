// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package monitorpoller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"

	ctrutils "github.com/DataDog/datadog-operator/pkg/controller/utils"
)

const (
	// groupStates selects the monitor groups whose state is returned with each
	// monitor. Only triggered groups are reported in custom resource statuses.
	groupStates = "alert,warn,no data"
	// requestTimeout bounds each request so that a stalled response cannot
	// hold up the polling of every other monitor.
	requestTimeout = time.Minute
)

// scopeKey identifies the credentials and Datadog site that monitors are
// polled with. It is a hash so that credentials are not used as map keys.
type scopeKey string

// credentials are the values of a Datadog API context, as returned by
// config.CredentialManager.GetAuth, that select the credentials and site of a
// request.
type credentials struct {
	apiKeys         map[string]datadogapi.APIKey
	serverIndex     *int
	serverVariables map[string]string
}

func credentialsFrom(auth context.Context) credentials {
	creds := credentials{}
	if apiKeys, ok := auth.Value(datadogapi.ContextAPIKeys).(map[string]datadogapi.APIKey); ok {
		creds.apiKeys = apiKeys
	}
	if serverIndex, ok := auth.Value(datadogapi.ContextServerIndex).(int); ok {
		creds.serverIndex = &serverIndex
	}
	if serverVariables, ok := auth.Value(datadogapi.ContextServerVariables).(map[string]string); ok {
		creds.serverVariables = serverVariables
	}
	return creds
}

func (c credentials) scope() scopeKey {
	h := sha256.New()
	for _, name := range slices.Sorted(maps.Keys(c.apiKeys)) {
		writeHashField(h, name)
		writeHashField(h, c.apiKeys[name].Prefix)
		writeHashField(h, c.apiKeys[name].Key)
	}
	if c.serverIndex != nil {
		writeHashField(h, strconv.Itoa(*c.serverIndex))
	} else {
		writeHashField(h, "")
	}
	for _, name := range slices.Sorted(maps.Keys(c.serverVariables)) {
		writeHashField(h, name)
		writeHashField(h, c.serverVariables[name])
	}
	return scopeKey(hex.EncodeToString(h.Sum(nil)))
}

// writeHashField writes a length-prefixed value, so that the boundaries
// between values are part of the hash.
func writeHashField(h hash.Hash, value string) {
	fmt.Fprintf(h, "%d:%s;", len(value), value)
}

// context returns a context that carries the credentials, and the deadline and
// cancellation of ctx.
func (c credentials) context(ctx context.Context) context.Context {
	if c.apiKeys != nil {
		ctx = context.WithValue(ctx, datadogapi.ContextAPIKeys, c.apiKeys)
	}
	if c.serverIndex != nil {
		ctx = context.WithValue(ctx, datadogapi.ContextServerIndex, *c.serverIndex)
	}
	if c.serverVariables != nil {
		ctx = context.WithValue(ctx, datadogapi.ContextServerVariables, c.serverVariables)
	}
	return ctx
}

// fetch polls monitors in batches. A failed request only fails the monitors of
// its own batch.
func (p *Poller) fetch(ctx context.Context, creds credentials, ids []int64) map[int64]Result {
	ctx = creds.context(ctx)
	polledAt := p.now()
	results := make(map[int64]Result, len(ids))
	var requests, failedRequests int

	for batch := range slices.Chunk(ids, p.batchSize) {
		requests++
		monitors, err := listMonitors(ctx, p.client, batch)
		if err != nil {
			failedRequests++
			p.log.Error(err, "Failed to poll monitors", "monitors", len(batch))
			for _, id := range batch {
				results[id] = Result{Err: err, PolledAt: polledAt}
			}
			continue
		}

		listed := make(map[int64]datadogV1.Monitor, len(monitors))
		for _, monitor := range monitors {
			listed[monitor.GetId()] = monitor
		}
		var missing []int64
		for _, id := range batch {
			if monitor, ok := listed[id]; ok {
				results[id] = Result{Monitor: monitor, PolledAt: polledAt}
			} else {
				missing = append(missing, id)
			}
		}

		// A monitor missing from the list was most likely deleted, but only a
		// definitive answer may report it as such: subscribers recreate
		// monitors that do not exist. Confirm each one individually.
		if len(missing) > 0 {
			p.log.V(1).Info("Monitors missing from the list response; fetching them individually", "monitors", len(missing))
		}
		for _, id := range missing {
			requests++
			monitor, err := getMonitor(ctx, p.client, id)
			if err != nil && !strings.Contains(err.Error(), ctrutils.NotFoundString) {
				failedRequests++
			}
			results[id] = Result{Monitor: monitor, Err: err, PolledAt: polledAt}
		}
	}

	p.log.V(1).Info("Polled monitors", "monitors", len(ids), "requests", requests, "failedRequests", failedRequests)
	return results
}

// listMonitors fetches the monitors with the given IDs in a single request.
// The generated client does not support the monitor_ids filter of the list
// endpoint, so the request is built with the client's lower-level helpers.
func listMonitors(ctx context.Context, api *datadogV1.MonitorsApi, ids []int64) ([]datadogV1.Monitor, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	client := api.Client
	basePath, err := client.Cfg.ServerURLWithContext(ctx, "v1.MonitorsApi.ListMonitors")
	if err != nil {
		return nil, fmt.Errorf("error listing monitors: %w", err)
	}

	monitorIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		monitorIDs = append(monitorIDs, strconv.FormatInt(id, 10))
	}
	query := url.Values{}
	query.Set("monitor_ids", strings.Join(monitorIDs, ","))
	query.Set("group_states", groupStates)
	// A single page holds every requested monitor. The page size only applies
	// when a page is requested.
	query.Set("page", "0")
	query.Set("page_size", strconv.Itoa(len(ids)))

	headers := map[string]string{"Accept": "application/json"}
	if client.Cfg.DelegatedTokenConfig != nil {
		if err = datadogapi.UseDelegatedTokenAuth(ctx, &headers, client.Cfg.DelegatedTokenConfig); err != nil {
			return nil, fmt.Errorf("error listing monitors: %w", err)
		}
	} else {
		datadogapi.SetAuthKeys(ctx, &headers,
			[2]string{"apiKeyAuth", "DD-API-KEY"},
			[2]string{"appKeyAuth", "DD-APPLICATION-KEY"},
		)
	}

	req, err := client.PrepareRequest(ctx, basePath+"/api/v1/monitor", http.MethodGet, nil, headers, query, url.Values{}, nil)
	if err != nil {
		return nil, fmt.Errorf("error listing monitors: %w", err)
	}
	httpResp, err := client.CallAPI(req)
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		return nil, ctrutils.NewAPIError(fmt.Errorf("error listing monitors: %w", err), httpResp)
	}
	body, err := datadogapi.ReadBody(httpResp)
	if err != nil {
		return nil, ctrutils.NewAPIError(fmt.Errorf("error listing monitors: %w", err), httpResp)
	}
	if httpResp.StatusCode >= 300 {
		// Report the status code only: the message must not read as a missing
		// monitor, which only the individual fetch can establish.
		return nil, ctrutils.NewAPIError(fmt.Errorf("error listing monitors: HTTP status %d: %s", httpResp.StatusCode, body), httpResp)
	}

	var monitors []datadogV1.Monitor
	if err = client.Decode(&monitors, body, httpResp.Header.Get("Content-Type")); err != nil {
		return nil, fmt.Errorf("error decoding monitors: %w", err)
	}
	return monitors, nil
}

func getMonitor(ctx context.Context, api *datadogV1.MonitorsApi, id int64) (datadogV1.Monitor, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	monitor, httpResp, err := api.GetMonitor(ctx, id, *datadogV1.NewGetMonitorOptionalParameters().WithGroupStates(groupStates))
	if httpResp != nil {
		defer httpResp.Body.Close()
	}
	if err != nil {
		if apiErr, ok := errors.AsType[datadogapi.GenericOpenAPIError](err); ok {
			return datadogV1.Monitor{}, ctrutils.NewAPIError(fmt.Errorf("error getting monitor: %w: %s", err, apiErr.Body()), httpResp)
		}
		return datadogV1.Monitor{}, ctrutils.NewAPIError(fmt.Errorf("error getting monitor: %w", err), httpResp)
	}
	return monitor, nil
}
