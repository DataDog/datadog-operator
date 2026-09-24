// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package cichecks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func executable(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/usr/bin/env bash\nset -eu\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCIChecksStopAtFirstFailure(t *testing.T) {
	targets := []string{"ci-build", "update-golang", "verify-licenses", "generate", "fmt", "lint", "gotest", "integration-tests"}
	cases := []struct {
		name       string
		failTarget string
		failDiff   string
		completed  int
	}{
		{name: "success", completed: len(targets)},
		{name: "version_diff", failDiff: "1", completed: 2},
		{name: "generation_diff", failDiff: "2", completed: 4},
		{name: "formatting_diff", failDiff: "3", completed: 5},
	}
	for i, target := range targets {
		cases = append(cases, struct {
			name       string
			failTarget string
			failDiff   string
			completed  int
		}{name: target, failTarget: target, completed: i + 1})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			trace := filepath.Join(dir, "trace")
			makePath := executable(t, dir, "make", `target="${!#}"
echo "$target" >> "$TRACE"
if [[ "$target" == "${FAIL_TARGET:-}" ]]; then exit 7; fi
`)
			executable(t, dir, "git", `count=0
if [[ -f "$TRACE.diffs" ]]; then count=$(wc -l < "$TRACE.diffs"); fi
count=$((count + 1))
echo diff >> "$TRACE.diffs"
if [[ "$count" == "${FAIL_DIFF:-}" ]]; then exit 1; fi
`)
			cmd := exec.Command("bash", "../ci-checks.sh", makePath)
			cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "TRACE="+trace,
				"GITLAB_CI=true", "FAIL_TARGET="+tc.failTarget, "FAIL_DIFF="+tc.failDiff)
			out, err := cmd.CombinedOutput()
			wantFailure := tc.failTarget != "" || tc.failDiff != ""
			if (err != nil) != wantFailure {
				t.Fatalf("unexpected result: %v\n%s", err, out)
			}
			calls, err := os.ReadFile(trace)
			if err != nil {
				t.Fatal(err)
			}
			if want := strings.Join(targets[:tc.completed], "\n") + "\n"; string(calls) != want {
				t.Fatalf("calls = %q, want %q", calls, want)
			}
			if strings.Count(string(out), "section_start:") != strings.Count(string(out), "section_end:") {
				t.Fatalf("unclosed log section:\n%s", out)
			}
		})
	}
}

func TestMakeSetupFailures(t *testing.T) {
	makefile, err := filepath.Abs("../../Makefile")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, target, variable, output string
		exit                           string
		wantFailure                    bool
	}{
		{name: "openapi_error", target: "generate-openapi", variable: "OPENAPI_GEN", exit: "9", wantFailure: true},
		{name: "openapi_violation", target: "generate-openapi", variable: "OPENAPI_GEN", output: "API violation", exit: "0", wantFailure: true},
		{name: "openapi_success", target: "generate-openapi", variable: "OPENAPI_GEN", exit: "0"},
		{name: "envtest_error", target: "integration-tests", variable: "ENVTEST", exit: "9", wantFailure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tool := executable(t, dir, "tool", "echo '"+tc.output+"'\nexit "+tc.exit+"\n")
			// Make probes go env at parse time. Other Go commands must not run
			// when the generator or envtest setup has failed.
			executable(t, dir, "go", `if [[ "$1" == env ]]; then exit 0; fi
echo 'UNEXPECTED GO INVOCATION' >&2
exit 99
`)
			cmd := exec.Command("make", "-f", makefile, tc.target, tc.variable+"="+tool)
			cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"))
			out, err := cmd.CombinedOutput()
			if (err != nil) != tc.wantFailure || strings.Contains(string(out), "UNEXPECTED GO INVOCATION") {
				t.Fatalf("unexpected result: %v\n%s", err, out)
			}
		})
	}
}
