package main

import (
	"bufio"
	"encoding/json"
	"fmt"
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
			fmt.Println(string(line))
			continue
		}
		if event.Output != "" {
			fmt.Print(event.Output)
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "reading go test output: %v\n", err)
		os.Exit(1)
	}
}
