// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package image

import (
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestReleaseCache_Get(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	release := byocRelease{
		Images: byocReleaseImages{
			Pomsky:                       releaseImage{Repository: "example.com/pomsky", Tag: "1.0.0"},
			ObservabilityPipelinesWorker: releaseImage{Repository: "example.com/worker", Tag: "2.0.0"},
		},
	}
	digestRelease := byocRelease{
		Images: byocReleaseImages{
			Pomsky:                       releaseImage{Repository: "example.com/pomsky", Tag: "1.1.0"},
			ObservabilityPipelinesWorker: releaseImage{Repository: "example.com/worker", Tag: "2.1.0"},
		},
	}

	tests := []struct {
		name        string
		initial     []releaseCacheEntry
		now         time.Time
		key         releaseCacheKey
		wantRelease byocRelease
		wantFound   bool
		wantEntries []releaseCacheEntry
	}{
		{
			name: "missing key leaves cache unchanged",
			initial: []releaseCacheEntry{
				{
					key:     releaseCacheKey{repository: "example.com/releases", digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
					release: digestRelease,
				},
				{
					key:       releaseCacheKey{repository: "example.com/releases", tag: "1.0.0"},
					release:   release,
					expiresAt: now.Add(releaseTagTTL),
				},
			},
			now: now,
			key: releaseCacheKey{repository: "example.com/releases", tag: "missing"},
			wantEntries: []releaseCacheEntry{
				{
					key:     releaseCacheKey{repository: "example.com/releases", digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
					release: digestRelease,
				},
				{
					key:       releaseCacheKey{repository: "example.com/releases", tag: "1.0.0"},
					release:   release,
					expiresAt: now.Add(releaseTagTTL),
				},
			},
		},
		{
			name: "tag hit before expiration moves to front without extending TTL",
			initial: []releaseCacheEntry{
				{
					key:     releaseCacheKey{repository: "example.com/releases", digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
					release: digestRelease,
				},
				{
					key:       releaseCacheKey{repository: "example.com/releases", tag: "1.0.0"},
					release:   release,
					expiresAt: now.Add(releaseTagTTL),
				},
			},
			now:         now.Add(releaseTagTTL - time.Nanosecond),
			key:         releaseCacheKey{repository: "example.com/releases", tag: "1.0.0"},
			wantRelease: release,
			wantFound:   true,
			wantEntries: []releaseCacheEntry{
				{
					key:       releaseCacheKey{repository: "example.com/releases", tag: "1.0.0"},
					release:   release,
					expiresAt: now.Add(releaseTagTTL),
				},
				{
					key:     releaseCacheKey{repository: "example.com/releases", digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
					release: digestRelease,
				},
			},
		},
		{
			name: "tag at expiration is removed",
			initial: []releaseCacheEntry{
				{
					key:     releaseCacheKey{repository: "example.com/releases", digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
					release: digestRelease,
				},
				{
					key:       releaseCacheKey{repository: "example.com/releases", tag: "1.0.0"},
					release:   release,
					expiresAt: now.Add(releaseTagTTL),
				},
			},
			now: now.Add(releaseTagTTL),
			key: releaseCacheKey{repository: "example.com/releases", tag: "1.0.0"},
			wantEntries: []releaseCacheEntry{
				{
					key:     releaseCacheKey{repository: "example.com/releases", digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
					release: digestRelease,
				},
			},
		},
		{
			name: "tag after expiration is removed",
			initial: []releaseCacheEntry{
				{
					key:     releaseCacheKey{repository: "example.com/releases", digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
					release: digestRelease,
				},
				{
					key:       releaseCacheKey{repository: "example.com/releases", tag: "1.0.0"},
					release:   release,
					expiresAt: now.Add(releaseTagTTL),
				},
			},
			now: now.Add(releaseTagTTL + time.Nanosecond),
			key: releaseCacheKey{repository: "example.com/releases", tag: "1.0.0"},
			wantEntries: []releaseCacheEntry{
				{
					key:     releaseCacheKey{repository: "example.com/releases", digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
					release: digestRelease,
				},
			},
		},
		{
			name: "digest does not expire and moves to front",
			initial: []releaseCacheEntry{
				{
					key:       releaseCacheKey{repository: "example.com/releases", tag: "1.0.0"},
					release:   release,
					expiresAt: now.Add(releaseTagTTL),
				},
				{
					key:     releaseCacheKey{repository: "example.com/releases", digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
					release: digestRelease,
				},
			},
			now:         now.Add(100 * releaseTagTTL),
			key:         releaseCacheKey{repository: "example.com/releases", digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
			wantRelease: digestRelease,
			wantFound:   true,
			wantEntries: []releaseCacheEntry{
				{
					key:     releaseCacheKey{repository: "example.com/releases", digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
					release: digestRelease,
				},
				{
					key:       releaseCacheKey{repository: "example.com/releases", tag: "1.0.0"},
					release:   release,
					expiresAt: now.Add(releaseTagTTL),
				},
			},
		},
		{
			name: "hit on front entry leaves order and count unchanged",
			initial: []releaseCacheEntry{
				{
					key:       releaseCacheKey{repository: "example.com/releases", tag: "1.0.0"},
					release:   release,
					expiresAt: now.Add(releaseTagTTL),
				},
				{
					key:     releaseCacheKey{repository: "example.com/releases", digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
					release: digestRelease,
				},
			},
			now:         now,
			key:         releaseCacheKey{repository: "example.com/releases", tag: "1.0.0"},
			wantRelease: release,
			wantFound:   true,
			wantEntries: []releaseCacheEntry{
				{
					key:       releaseCacheKey{repository: "example.com/releases", tag: "1.0.0"},
					release:   release,
					expiresAt: now.Add(releaseTagTTL),
				},
				{
					key:     releaseCacheKey{repository: "example.com/releases", digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
					release: digestRelease,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := newTestReleaseCache(tt.now, tt.initial)

			got, found := cache.get(tt.key)

			if found != tt.wantFound {
				t.Errorf("get() found = %v, want %v", found, tt.wantFound)
			}
			if diff := cmp.Diff(tt.wantRelease, got); diff != "" {
				t.Errorf("get() release mismatch (-want +got):\n%s", diff)
			}
			if got := len(cache.entries); got != len(tt.wantEntries) {
				t.Errorf("map size = %d, want %d", got, len(tt.wantEntries))
			}
			for element := cache.recency.Front(); element != nil; element = element.Next() {
				entry := element.Value.(releaseCacheEntry)
				if cache.entries[entry.key] != element {
					t.Errorf("map entry for %+v does not point to its LRU element", entry.key)
				}
			}
			if diff := cmp.Diff(tt.wantEntries, releaseCacheEntries(cache), cmp.AllowUnexported(releaseCacheEntry{}, releaseCacheKey{})); diff != "" {
				t.Errorf("cache entries in MRU order mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestReleaseCache_Put(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	release := byocRelease{Images: byocReleaseImages{
		Pomsky:                       releaseImage{Repository: "example.com/pomsky", Tag: "1.0.0"},
		ObservabilityPipelinesWorker: releaseImage{Repository: "example.com/worker", Tag: "2.0.0"},
	}}
	updatedRelease := byocRelease{Images: byocReleaseImages{
		Pomsky:                       releaseImage{Repository: "example.com/pomsky", Tag: "1.1.0"},
		ObservabilityPipelinesWorker: releaseImage{Repository: "example.com/worker", Tag: "2.1.0"},
	}}
	fullCache := make([]releaseCacheEntry, releaseCacheCapacity)
	for i := range fullCache {
		fullCache[i] = releaseCacheEntry{
			key:       releaseCacheKey{repository: "example.com/releases", tag: fmt.Sprintf("release-%d", i)},
			release:   release,
			expiresAt: now.Add(releaseTagTTL / 2),
		}
	}
	fullCache[len(fullCache)-1] = releaseCacheEntry{
		key:     releaseCacheKey{repository: "example.com/releases", digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		release: release,
	}

	tests := []struct {
		name        string
		initial     []releaseCacheEntry
		key         releaseCacheKey
		release     byocRelease
		wantEntries []releaseCacheEntry
	}{
		{
			name:    "new tag receives TTL",
			key:     releaseCacheKey{repository: "example.com/releases", tag: "new"},
			release: updatedRelease,
			wantEntries: []releaseCacheEntry{{
				key:       releaseCacheKey{repository: "example.com/releases", tag: "new"},
				release:   updatedRelease,
				expiresAt: now.Add(releaseTagTTL),
			}},
		},
		{
			name: "new digest has no expiration and is added at front",
			initial: []releaseCacheEntry{{
				key:       releaseCacheKey{repository: "example.com/releases", tag: "existing"},
				release:   release,
				expiresAt: now.Add(releaseTagTTL / 2),
			}},
			key:     releaseCacheKey{repository: "example.com/releases", digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
			release: release,
			wantEntries: []releaseCacheEntry{
				{
					key:     releaseCacheKey{repository: "example.com/releases", digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
					release: release,
				},
				{
					key:       releaseCacheKey{repository: "example.com/releases", tag: "existing"},
					release:   release,
					expiresAt: now.Add(releaseTagTTL / 2),
				},
			},
		},
		{
			name: "existing tag updates release and TTL and moves to front",
			initial: []releaseCacheEntry{
				{
					key:     releaseCacheKey{repository: "example.com/releases", digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
					release: release,
				},
				{
					key:       releaseCacheKey{repository: "example.com/releases", tag: "existing"},
					release:   release,
					expiresAt: now.Add(releaseTagTTL / 2),
				},
			},
			key:     releaseCacheKey{repository: "example.com/releases", tag: "existing"},
			release: updatedRelease,
			wantEntries: []releaseCacheEntry{
				{
					key:       releaseCacheKey{repository: "example.com/releases", tag: "existing"},
					release:   updatedRelease,
					expiresAt: now.Add(releaseTagTTL),
				},
				{
					key:     releaseCacheKey{repository: "example.com/releases", digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
					release: release,
				},
			},
		},
		{
			name:    "reaching capacity does not evict any entry",
			initial: fullCache[:releaseCacheCapacity-1],
			key:     releaseCacheKey{repository: "example.com/releases", tag: "new"},
			release: updatedRelease,
			wantEntries: append([]releaseCacheEntry{{
				key:       releaseCacheKey{repository: "example.com/releases", tag: "new"},
				release:   updatedRelease,
				expiresAt: now.Add(releaseTagTTL),
			}}, fullCache[:releaseCacheCapacity-1]...),
		},
		{
			name:    "exceeding capacity evicts oldest entry even without expiration",
			initial: fullCache,
			key:     releaseCacheKey{repository: "example.com/releases", tag: "new"},
			release: updatedRelease,
			wantEntries: append([]releaseCacheEntry{{
				key:       releaseCacheKey{repository: "example.com/releases", tag: "new"},
				release:   updatedRelease,
				expiresAt: now.Add(releaseTagTTL),
			}}, fullCache[:releaseCacheCapacity-1]...),
		},
		{
			name:    "updating existing entry at capacity does not evict",
			initial: fullCache,
			key:     releaseCacheKey{repository: "example.com/releases", tag: "release-1"},
			release: updatedRelease,
			wantEntries: append([]releaseCacheEntry{
				{
					key:       releaseCacheKey{repository: "example.com/releases", tag: "release-1"},
					release:   updatedRelease,
					expiresAt: now.Add(releaseTagTTL),
				},
				fullCache[0],
			}, fullCache[2:]...),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := newTestReleaseCache(now, tt.initial)

			cache.put(tt.key, tt.release)

			if got := len(cache.entries); got != len(tt.wantEntries) {
				t.Errorf("map size = %d, want %d", got, len(tt.wantEntries))
			}
			for element := cache.recency.Front(); element != nil; element = element.Next() {
				entry := element.Value.(releaseCacheEntry)
				if cache.entries[entry.key] != element {
					t.Errorf("map entry for %+v does not point to its LRU element", entry.key)
				}
			}
			if diff := cmp.Diff(tt.wantEntries, releaseCacheEntries(cache), cmp.AllowUnexported(releaseCacheEntry{}, releaseCacheKey{})); diff != "" {
				t.Errorf("cache entries in MRU order mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

// newTestReleaseCache seeds entries in most-to-least-recent order without calling put.
func newTestReleaseCache(now time.Time, entries []releaseCacheEntry) *releaseCache {
	cache := newReleaseCache(func() time.Time { return now })
	for _, entry := range entries {
		cache.entries[entry.key] = cache.recency.PushBack(entry)
	}
	return cache
}

// releaseCacheEntries returns entries in most-to-least-recent order without changing the cache.
func releaseCacheEntries(cache *releaseCache) []releaseCacheEntry {
	var entries []releaseCacheEntry
	for element := cache.recency.Front(); element != nil; element = element.Next() {
		entries = append(entries, element.Value.(releaseCacheEntry))
	}
	return entries
}
