// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleetsuite

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

type rcSubscription = func(map[string]state.RawConfig, func(string, state.ApplyStatus))

type fakeRCClient struct {
	mu            sync.RWMutex
	subscriptions map[string]rcSubscription
	subscribeCh   map[string]chan struct{}
	installer     []*pbgo.PackageState
}

func newFakeRCClient(initialState []*pbgo.PackageState) *fakeRCClient {
	return &fakeRCClient{
		subscriptions: make(map[string]rcSubscription),
		subscribeCh:   make(map[string]chan struct{}),
		installer:     initialState,
	}
}

func (c *fakeRCClient) Subscribe(product string, fn rcSubscription) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.subscriptions[product] = fn
	ch, ok := c.subscribeCh[product]
	if !ok {
		ch = make(chan struct{})
		c.subscribeCh[product] = ch
	}
	select {
	case <-ch:
	default:
		close(ch)
	}
}

func (c *fakeRCClient) GetInstallerState() []*pbgo.PackageState {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stateCopy := make([]*pbgo.PackageState, 0, len(c.installer))
	for _, pkg := range c.installer {
		stateCopy = append(stateCopy, &pbgo.PackageState{
			Package:                 pkg.GetPackage(),
			StableVersion:           pkg.GetStableVersion(),
			ExperimentVersion:       pkg.GetExperimentVersion(),
			Task:                    pkg.GetTask(),
			StableConfigVersion:     pkg.GetStableConfigVersion(),
			ExperimentConfigVersion: pkg.GetExperimentConfigVersion(),
		})
	}
	return stateCopy
}

func (c *fakeRCClient) SetInstallerState(packages []*pbgo.PackageState) {
	c.mu.Lock()
	defer c.mu.Unlock()

	stateCopy := make([]*pbgo.PackageState, 0, len(packages))
	for _, pkg := range packages {
		stateCopy = append(stateCopy, &pbgo.PackageState{
			Package:                 pkg.GetPackage(),
			StableVersion:           pkg.GetStableVersion(),
			ExperimentVersion:       pkg.GetExperimentVersion(),
			Task:                    pkg.GetTask(),
			StableConfigVersion:     pkg.GetStableConfigVersion(),
			ExperimentConfigVersion: pkg.GetExperimentConfigVersion(),
		})
	}
	c.installer = stateCopy
}

func (c *fakeRCClient) sendJSON(ctx context.Context, product string, path string, payload any) (state.ApplyStatus, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return state.ApplyStatus{}, err
	}
	return c.sendRaw(ctx, product, path, raw)
}

func (c *fakeRCClient) sendRaw(ctx context.Context, product string, path string, payload []byte) (state.ApplyStatus, error) {
	subscription, err := c.waitSubscription(ctx, product)
	if err != nil {
		return state.ApplyStatus{}, err
	}

	statuses := make(map[string]state.ApplyStatus)
	done := make(chan struct{})
	go func() {
		defer close(done)
		subscription(map[string]state.RawConfig{
			path: {
				Config: payload,
				Metadata: state.Metadata{
					Product: product,
					ID:      path,
					Name:    path,
				},
			},
		}, func(cfgPath string, status state.ApplyStatus) {
			statuses[cfgPath] = status
		})
	}()

	select {
	case <-done:
		status := statuses[path]
		if status.State != state.ApplyStateAcknowledged {
			return status, fmt.Errorf("remote config %s/%s was not acknowledged: state=%d error=%q", product, path, status.State, status.Error)
		}
		return status, nil
	case <-ctx.Done():
		return state.ApplyStatus{}, ctx.Err()
	}
}

func (c *fakeRCClient) waitSubscription(ctx context.Context, product string) (rcSubscription, error) {
	c.mu.Lock()
	if fn, ok := c.subscriptions[product]; ok {
		c.mu.Unlock()
		return fn, nil
	}
	ch, ok := c.subscribeCh[product]
	if !ok {
		ch = make(chan struct{})
		c.subscribeCh[product] = ch
	}
	c.mu.Unlock()

	select {
	case <-ch:
		c.mu.RLock()
		defer c.mu.RUnlock()
		return c.subscriptions[product], nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *fakeRCClient) packageState(packageName string) *pbgo.PackageState {
	for _, pkg := range c.GetInstallerState() {
		if pkg.GetPackage() == packageName {
			return pkg
		}
	}
	return nil
}
