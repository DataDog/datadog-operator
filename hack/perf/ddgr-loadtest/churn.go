package main

import "math/rand"

// PickChurnTargets deterministically selects ceil(percent% of len(names)) entries
// using rand keyed by (seed + tick). Same (names, percent, seed, tick) always
// produces the same output. Different ticks produce different selections.
func PickChurnTargets(names []string, percent int, seed int64, tick int) []string {
	if percent <= 0 || len(names) == 0 {
		return nil
	}
	if percent >= 100 {
		out := make([]string, len(names))
		copy(out, names)
		return out
	}
	n := (len(names)*percent + 99) / 100 // ceil
	r := rand.New(rand.NewSource(seed + int64(tick)))
	indices := r.Perm(len(names))[:n]
	out := make([]string, n)
	for i, idx := range indices {
		out[i] = names[idx]
	}
	return out
}
