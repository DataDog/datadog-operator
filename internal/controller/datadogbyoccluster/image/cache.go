// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package image

import (
	"container/list"
	"sync"
	"time"
)

const (
	releaseCacheCapacity = 256
	releaseTagTTL        = 10 * time.Minute
)

type releaseCacheKey struct {
	repository string
	tag        string
	digest     string
}

type releaseCacheEntry struct {
	key       releaseCacheKey
	release   byocRelease
	expiresAt time.Time
}

// releaseCache holds validated releases before applying per-cluster overrides.
// Digest entries do not expire, but all entries are subject to LRU eviction.
type releaseCache struct {
	mu      sync.Mutex
	entries map[releaseCacheKey]*list.Element
	recency list.List
	now     func() time.Time
}

func newReleaseCache(now func() time.Time) *releaseCache {
	return &releaseCache{
		entries: make(map[releaseCacheKey]*list.Element),
		now:     now,
	}
}

func (c *releaseCache) get(key releaseCacheKey) (byocRelease, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	element, ok := c.entries[key]
	if !ok {
		return byocRelease{}, false
	}
	entry := element.Value.(releaseCacheEntry)
	if !entry.expiresAt.IsZero() && !c.now().Before(entry.expiresAt) {
		c.remove(element)
		return byocRelease{}, false
	}
	c.recency.MoveToFront(element)
	return entry.release, true
}

func (c *releaseCache) put(key releaseCacheKey, release byocRelease) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry := releaseCacheEntry{key: key, release: release}
	if key.tag != "" {
		entry.expiresAt = c.now().Add(releaseTagTTL)
	}
	if element, ok := c.entries[key]; ok {
		element.Value = entry
		c.recency.MoveToFront(element)
		return
	}
	c.entries[key] = c.recency.PushFront(entry)
	if c.recency.Len() > releaseCacheCapacity {
		c.remove(c.recency.Back())
	}
}

// remove must be called while holding mu.
func (c *releaseCache) remove(element *list.Element) {
	delete(c.entries, element.Value.(releaseCacheEntry).key)
	c.recency.Remove(element)
}
