// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package version

import (
	"encoding/json"
	"fmt"
	"io"
	"runtime"

	"github.com/go-logr/logr"
)

var (
	// Version Datadog Operator version
	Version = "0.0.0"
	// BuildTime binary build time
	BuildTime = ""
	// Commit current git commit
	Commit = ""
)

// JSON contains the json definition of the version payload
type JSON struct {
	Version   string `json:"version,omitempty"`
	BuildTime string `json:"build_time,omitempty"`
	Commit    string `json:"commit,omitempty"`
	Go        string `json:"go,omitempty"`
	Os        string `json:"os,omitempty"`
	Error     string `json:"error,omitempty"`
}

func newVersionJSON() []byte {
	bytes, err := json.Marshal(JSON{
		Version:   Version,
		BuildTime: BuildTime,
		Commit:    Commit,
		Go:        runtime.Version(),
		Os:        fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		Error:     "",
	})
	if err != nil {
		bytes, _ = json.Marshal(JSON{Error: fmt.Sprintf("cannot get version: %v", err)})
	}

	return bytes
}

// PrintVersionWriter print versions information in to writer interface
func PrintVersionWriter(writer io.Writer, format string) {
	switch format {
	case "text":
		fmt.Fprintf(writer, "Version:\n")
		for _, val := range printVersionSlice() {
			fmt.Fprintf(writer, "- %s\n", val)
		}
	case "json":
		versionBytes := newVersionJSON()
		fmt.Fprint(writer, string(versionBytes))
	default:
		fmt.Fprintf(writer, "Unknown format: %s", format)
	}
}

// PrintVersionLogs print versions information in logs
func PrintVersionLogs(logger logr.Logger) {
	for _, val := range printVersionSlice() {
		logger.Info(val)
	}
}

func printVersionSlice() []string {
	return []string{
		fmt.Sprintf("Version: %v", Version),
		fmt.Sprintf("Build time: %v", BuildTime),
		fmt.Sprintf("Git Commit: %v", Commit),
		"Go Version: " + runtime.Version(),
		fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH),
	}
}

// GetVersion returns the operator version
func GetVersion() string {
	return Version
}
