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
	os.Exit(run(os.Stdin, os.Stdout, os.Stderr))
}

func run(input io.Reader, output, errorOutput io.Writer) int {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		var event testEvent
		line := scanner.Bytes()
		if err := json.Unmarshal(line, &event); err != nil {
			_, _ = output.Write(line)
			_, _ = output.Write([]byte("\n"))
			continue
		}
		if event.Output != "" {
			_, _ = io.WriteString(output, event.Output)
		}
	}

	if err := scanner.Err(); err != nil {
		_, _ = io.WriteString(errorOutput, "reading go test output: "+err.Error()+"\n")
		return 1
	}

	return 0
}
