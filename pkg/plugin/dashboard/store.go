// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dashboard

import "context"

// Store acquires the sources and serves snapshots of them.
type Store interface {
	// Start acquires the sources. A one-shot store returns once all sources
	// were read; a live store returns once it is running.
	Start(ctx context.Context) error
	// Snapshot returns an immutable copy of the current data. It is cheap.
	Snapshot() *Snapshot
	// Changed is signaled when the data changed since the last Snapshot.
	// It is nil for one-shot stores.
	Changed() <-chan struct{}
	// Close releases the store's resources.
	Close()
}
