package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildMonitorJSON_IsValidJSON(t *testing.T) {
	payload := BuildMonitorJSON(42, 7)
	var m map[string]any
	if err := json.Unmarshal([]byte(payload), &m); err != nil {
		t.Fatalf("not valid JSON: %v\npayload:\n%s", err, payload)
	}
}

func TestBuildMonitorJSON_HasUniqueIndex(t *testing.T) {
	payload := BuildMonitorJSON(42, 0)
	if !strings.Contains(payload, "index:42") {
		t.Errorf("expected tag index:42, got: %s", payload)
	}
	if !strings.Contains(payload, "ddgr-loadtest 42") {
		t.Errorf("expected name to contain index, got: %s", payload)
	}
}

func TestBuildMonitorJSON_RevAppearsInMessage(t *testing.T) {
	payload := BuildMonitorJSON(0, 99)
	if !strings.Contains(payload, "rev=99") {
		t.Errorf("expected rev=99 in message, got: %s", payload)
	}
}
