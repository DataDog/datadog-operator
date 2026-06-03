package main

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
)

type testEvent struct {
	Output string
}

func main() {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		var event testEvent
		line := scanner.Bytes()
		if err := json.Unmarshal(line, &event); err != nil {
			_, _ = os.Stdout.Write(line)
			_, _ = os.Stdout.Write([]byte("\n"))
			continue
		}
		if event.Output != "" {
			_, _ = io.WriteString(os.Stdout, event.Output)
		}
	}

	if err := scanner.Err(); err != nil {
		_, _ = io.WriteString(os.Stderr, "reading go test output: "+err.Error()+"\n")
		os.Exit(1)
	}
}
