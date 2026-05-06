package main

import (
	"reflect"
	"sort"
	"strconv"
	"testing"
)

func TestPickChurnTargets_PercentCalculation(t *testing.T) {
	names := make([]string, 100)
	for i := range names {
		names[i] = "monitor-" + strconv.Itoa(i)
	}
	got := PickChurnTargets(names, 10, 1, 0)
	if len(got) != 10 {
		t.Errorf("expected 10, got %d", len(got))
	}
}

func TestPickChurnTargets_DeterministicForSameSeedTick(t *testing.T) {
	names := []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}
	a := PickChurnTargets(names, 30, 7, 5)
	b := PickChurnTargets(names, 30, 7, 5)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("not deterministic: %v vs %v", a, b)
	}
}

func TestPickChurnTargets_VariesAcrossTicks(t *testing.T) {
	names := make([]string, 50)
	for i := range names {
		names[i] = "n-" + strconv.Itoa(i)
	}
	a := PickChurnTargets(names, 20, 1, 0)
	b := PickChurnTargets(names, 20, 1, 1)
	sort.Strings(a)
	sort.Strings(b)
	if reflect.DeepEqual(a, b) {
		t.Errorf("expected different selections for different ticks; got identical: %v", a)
	}
}

func TestPickChurnTargets_ZeroPercentReturnsEmpty(t *testing.T) {
	names := []string{"a", "b"}
	if got := PickChurnTargets(names, 0, 1, 0); len(got) != 0 {
		t.Errorf("expected empty for percent=0, got %v", got)
	}
}

func TestPickChurnTargets_HundredPercentReturnsAll(t *testing.T) {
	names := []string{"a", "b", "c"}
	got := PickChurnTargets(names, 100, 1, 0)
	if len(got) != 3 {
		t.Errorf("expected 3, got %d", len(got))
	}
}
