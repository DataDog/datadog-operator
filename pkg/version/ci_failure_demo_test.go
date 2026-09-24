// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package version

import "testing"

// TestIntentionalCIFailureDemo exercises the consolidated CI job's failure path.
// This test is only for the demo PR and must not be merged.
func TestIntentionalCIFailureDemo(t *testing.T) {
	t.Fatal("INTENTIONAL CI FAILURE: demo of build_and_test failure output and automatic retries; do not merge")
}
