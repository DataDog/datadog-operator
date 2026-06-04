package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunPrintsGoTestOutputEvents(t *testing.T) {
	input := strings.Join([]string{
		`{"Action":"run","Package":"pkg","Test":"TestExample"}`,
		`{"Action":"output","Package":"pkg","Output":"=== RUN   TestExample\n"}`,
		`{"Action":"output","Package":"pkg","Output":"--- PASS: TestExample (0.00s)\n"}`,
		`{"Action":"pass","Package":"pkg","Test":"TestExample"}`,
	}, "\n")
	var output bytes.Buffer
	var errorOutput bytes.Buffer

	exitCode := run(strings.NewReader(input), &output, &errorOutput)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if got, want := output.String(), "=== RUN   TestExample\n--- PASS: TestExample (0.00s)\n"; got != want {
		t.Fatalf("unexpected output:\ngot:  %q\nwant: %q", got, want)
	}
	if got := errorOutput.String(); got != "" {
		t.Fatalf("expected no stderr, got %q", got)
	}
}

func TestRunPrintsInvalidJSONLines(t *testing.T) {
	var output bytes.Buffer
	var errorOutput bytes.Buffer

	exitCode := run(strings.NewReader("not json\n"), &output, &errorOutput)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if got, want := output.String(), "not json\n"; got != want {
		t.Fatalf("unexpected output:\ngot:  %q\nwant: %q", got, want)
	}
	if got := errorOutput.String(); got != "" {
		t.Fatalf("expected no stderr, got %q", got)
	}
}
